package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// This file implements the three-step delegation handling described in the
// deployment plan:
//
//	1. Check   - is the zone already delegated to us? (dns_preflight.go)
//	2. Fix     - ask which profile owns the parent zone, verify that answer, write
//	             the NS record ourselves.
//	3. Fallback- show the records for a human to add elsewhere.
//
// The design deliberately asks rather than infers. AWS offers no way to ask "who
// owns this domain", and guessing from an Organizations tree would be a
// plausible-but-unverifiable answer to a question where being wrong means
// writing DNS into somebody else's zone. So the operator supplies the intent and
// nameserverSetsMatch supplies the proof.

// parentZoneCandidate is one profile's answer to "do you hold the parent zone?".
type parentZoneCandidate struct {
	Profile   string
	AccountID string
	ZoneID    string

	// Nameservers is the candidate zone's delegation set.
	Nameservers []string

	// Authoritative is true when Nameservers matches what the public internet
	// returns for the parent domain — i.e. this really is the live zone, not a
	// same-named zone in an unrelated account.
	Authoritative bool

	// Err records why this profile could not be inspected (expired SSO, no
	// permission, and so on). Candidates with an error are still listed so the
	// operator can see the profile was tried.
	Err error
}

// Label renders the candidate for a selection list.
func (c parentZoneCandidate) Label(parentDomain string) string {
	switch {
	case c.Err != nil:
		return fmt.Sprintf("%s — could not check (%s)", c.Profile, shortError(c.Err))
	case c.ZoneID == "":
		return fmt.Sprintf("%s — no %s zone", c.Profile, parentDomain)
	case c.Authoritative:
		return fmt.Sprintf("%s — has %s, matches public DNS ✅ (account %s)", c.Profile, parentDomain, c.AccountID)
	default:
		return fmt.Sprintf("%s — has %s, but does NOT match public DNS ⚠️ (account %s)", c.Profile, parentDomain, c.AccountID)
	}
}

func shortError(err error) string {
	msg := err.Error()
	if i := strings.IndexAny(msg, "\n"); i > 0 {
		msg = msg[:i]
	}
	if len(msg) > 60 {
		msg = msg[:57] + "..."
	}
	return msg
}

// probeProfileForParentZone asks one profile whether it holds the parent zone,
// and whether that zone is the one the internet actually consults.
//
// Never returns an error: an unreachable profile is a candidate carrying an
// error, because "this profile was tried and here is why it did not work" is
// information the operator needs.
func probeProfileForParentZone(ctx context.Context, profile, parentDomain string, publicNS []string) parentZoneCandidate {
	c := parentZoneCandidate{Profile: profile}

	// Bound each profile: an unreachable or unauthenticated one must not stall
	// the whole scan.
	profileCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	zoneID, ns, err := findHostedZoneByName(profileCtx, profile, parentDomain)
	if err != nil {
		c.Err = err
		return c
	}

	c.ZoneID = zoneID
	c.Nameservers = ns
	if zoneID != "" {
		c.Authoritative = nameserverSetsMatch(ns, publicNS)
		c.AccountID = lookupAccountID(profileCtx, profile)
	}
	return c
}

// scanProfilesForParentZoneStream probes every profile concurrently and emits
// each result the moment it lands, closing the channel when all are done.
//
// Streaming rather than batching is what stops one profile with an expired SSO
// token from holding the whole picker hostage for its full 20-second budget:
// the profiles that answered in 300ms are selectable while the slow one is
// still being waited on.
//
// Cancel ctx to stop early. Outstanding probes then drop their results rather
// than blocking forever on a channel nobody is draining.
func scanProfilesForParentZoneStream(ctx context.Context, profiles []string, parentDomain string, publicNS []string) <-chan parentZoneCandidate {
	out := make(chan parentZoneCandidate)

	go func() {
		defer close(out)

		var wg sync.WaitGroup
		// Modest concurrency: each profile may trigger an SSO token refresh.
		sem := make(chan struct{}, 6)

		for _, profile := range profiles {
			wg.Add(1)
			go func(profile string) {
				defer wg.Done()

				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					return
				}
				defer func() { <-sem }()

				c := probeProfileForParentZone(ctx, profile, parentDomain, publicNS)
				select {
				case out <- c:
				case <-ctx.Done():
				}
			}(profile)
		}
		wg.Wait()
	}()

	return out
}

// scanProfilesForParentZone drains the stream into a sorted slice, for callers
// that cannot show partial results (the HTTP handlers and the text flow).
func scanProfilesForParentZone(ctx context.Context, profiles []string, parentDomain string, publicNS []string) []parentZoneCandidate {
	var results []parentZoneCandidate
	for c := range scanProfilesForParentZoneStream(ctx, profiles, parentDomain, publicNS) {
		results = append(results, c)
	}
	sortCandidates(results)
	return results
}

// candidateRank orders candidates by how useful they are: authoritative first,
// then has-a-zone, then reachable-but-empty, then errored.
func candidateRank(c parentZoneCandidate) int {
	switch {
	case c.Authoritative:
		return 0
	case c.ZoneID != "":
		return 1
	case c.Err == nil:
		return 2
	default:
		return 3
	}
}

// sortCandidates sorts by rank, then by profile name.
//
// The name tiebreak matters now that results arrive in completion order: without
// it the list would be ordered by how fast each profile's credentials resolved,
// which reshuffles between runs for no reason the operator can see.
func sortCandidates(candidates []parentZoneCandidate) {
	sort.SliceStable(candidates, func(a, b int) bool {
		ra, rb := candidateRank(candidates[a]), candidateRank(candidates[b])
		if ra != rb {
			return ra < rb
		}
		return candidates[a].Profile < candidates[b].Profile
	})
}

// cachedParentProfile returns the profile last known to manage parentDomain.
//
// The answer is a starting point, not a conclusion — the caller still probes it
// and still requires the public-DNS match before it can be used.
func cachedParentProfile(parentDomain string) (string, bool) {
	cfg, err := loadDNSConfig()
	if err != nil || cfg == nil {
		return "", false
	}
	ref := findParentZone(cfg, parentDomain)
	if ref == nil || ref.Profile == "" {
		return "", false
	}
	return ref.Profile, true
}

// lookupAccountID resolves a profile to its AWS account ID, best effort.
func lookupAccountID(ctx context.Context, profile string) string {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(profile))
	if err != nil {
		return ""
	}
	out, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil || out.Account == nil {
		return ""
	}
	return *out.Account
}

// delegationRequest is everything needed to write one NS delegation record.
type delegationRequest struct {
	// ParentProfile is the profile that can write to the parent zone.
	ParentProfile string
	// ParentZoneID is the zone that will carry the NS record, e.g. for coretechx.dev.
	ParentZoneID string
	// Subdomain is the delegated zone name, e.g. "dev.coretechx.dev".
	Subdomain string
	// Nameservers is our zone's delegation set.
	Nameservers []string
}

// Validate rejects requests that are obviously unsafe or incomplete before any
// AWS call is made.
func (r delegationRequest) Validate() error {
	switch {
	case strings.TrimSpace(r.ParentProfile) == "":
		return fmt.Errorf("no AWS profile selected for the parent zone")
	case strings.TrimSpace(r.ParentZoneID) == "":
		return fmt.Errorf("no parent hosted zone selected")
	case strings.TrimSpace(r.Subdomain) == "":
		return fmt.Errorf("no subdomain to delegate")
	case len(r.Nameservers) == 0:
		return fmt.Errorf("no nameservers to delegate %s to", r.Subdomain)
	}
	return nil
}

// applyDelegation writes the NS record into the parent zone.
//
// Write permission is verified by attempting the change and classifying the
// failure, rather than by iam:SimulatePrincipalPolicy. Route53 has no dry-run
// for ChangeResourceRecordSets, simulate needs its own permission (which is
// itself often denied), and the change is an idempotent UPSERT of exactly the
// record we want — so attempting it is the most accurate possible check.
// delegationWriter performs the actual Route53 write. It is a variable so that
// tests can assert what applyDelegation forwards without touching AWS — the
// original bug here was passing the operator's chosen profile in the wrong
// argument position, which no amount of AWS-free testing could catch while the
// call was hardcoded.
var delegationWriter = createNSRecordDelegation

func applyDelegation(req delegationRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}

	err := delegationWriter(req.ParentProfile, req.ParentZoneID, req.Subdomain, req.Nameservers)
	if err == nil {
		return nil
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "AccessDenied"), strings.Contains(msg, "not authorized"):
		return fmt.Errorf(
			"profile %s cannot write to hosted zone %s (route53:ChangeResourceRecordSets denied). "+
				"Use a profile with write access, or add the record manually: %w",
			req.ParentProfile, req.ParentZoneID, err)
	case strings.Contains(msg, "NoSuchHostedZone"):
		return fmt.Errorf("hosted zone %s does not exist in profile %s: %w",
			req.ParentZoneID, req.ParentProfile, err)
	case strings.Contains(msg, "ExpiredToken"), strings.Contains(msg, "expired"):
		return fmt.Errorf("credentials for profile %s have expired — run `aws sso login --profile %s`: %w",
			req.ParentProfile, req.ParentProfile, err)
	}
	return err
}

// waitForDelegation polls public DNS until the delegation is visible, or the
// context is done. It returns the nameservers actually observed.
//
// Delegation is not instant: the parent zone's own TTL and resolver caches mean
// the record can take a few minutes to become globally visible, and ACM will not
// validate until it is.
func waitForDelegation(ctx context.Context, subdomain string, expected []string, poll time.Duration) ([]string, bool) {
	if poll <= 0 {
		poll = 10 * time.Second
	}

	for {
		observed, err := queryNameservers(subdomain)
		if err == nil && nameserverSetsMatch(expected, observed) {
			return observed, true
		}

		select {
		case <-ctx.Done():
			return observed, false
		case <-time.After(poll):
		}
	}
}

// delegationRecord is what gets written to dns.yaml after a successful
// delegation. Grouped into a struct because half of these fields describe the
// delegated zone and half describe its parent, and seven positional strings at
// the call site made it easy to swap them.
type delegationRecord struct {
	// The zone we created and delegated to.
	Subdomain   string
	AccountID   string
	ZoneID      string
	Nameservers []string

	// Where the NS record was written.
	ParentDomain    string
	ParentProfile   string
	ParentZoneID    string
	ParentAccountID string
}

// recordDelegation remembers a successful delegation in dns.yaml.
//
// Two things are saved for two different reasons: the delegated zone, so
// `meroku dns status` can report it, and the parent zone's profile, so the next
// environment of this project skips the profile scan entirely.
func recordDelegation(r delegationRecord) error {
	cfg, err := loadDNSConfig()
	if err != nil {
		return err
	}
	if cfg == nil {
		cfg = &DNSConfig{RootDomain: r.ParentDomain}
	}
	if cfg.RootDomain == "" {
		cfg.RootDomain = r.ParentDomain
	}

	addOrUpdateDelegatedZone(cfg, DelegatedZone{
		Subdomain: r.Subdomain,
		AccountID: r.AccountID,
		ZoneID:    r.ZoneID,
		NSRecords: r.Nameservers,
		Status:    "delegated",
	})

	addOrUpdateParentZone(cfg, ParentZoneRef{
		Domain:    r.ParentDomain,
		Profile:   r.ParentProfile,
		ZoneID:    r.ParentZoneID,
		AccountID: r.ParentAccountID,
	})

	return saveDNSConfig(cfg)
}

// manualDelegationInstructions renders the fallback: what a human must add, and
// crucially why meroku could not do it for them.
func manualDelegationInstructions(subdomain, parentDomain, reason string, nameservers []string) string {
	var b strings.Builder

	b.WriteString("🌐 Manual DNS delegation required\n\n")
	if reason != "" {
		b.WriteString(fmt.Sprintf("   Why: %s\n\n", reason))
	}
	b.WriteString(fmt.Sprintf("   Add this record to the %s zone, wherever it is hosted:\n\n", parentDomain))
	b.WriteString(fmt.Sprintf("     Name:  %s\n", subdomain))
	b.WriteString("     Type:  NS\n")
	b.WriteString("     TTL:   300\n")
	b.WriteString("     Value:\n")
	for _, ns := range nameservers {
		b.WriteString(fmt.Sprintf("       %s\n", ns))
	}
	b.WriteString("\n   Then check it has taken effect with:\n")
	b.WriteString(fmt.Sprintf("     dig +short NS %s\n", subdomain))
	b.WriteString("     meroku dns validate\n")

	return b.String()
}
