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

// scanProfilesForParentZone inspects every local AWS profile for a public hosted
// zone matching parentDomain, and marks the ones whose delegation set matches
// what the internet actually returns.
//
// This exists to make the right choice obvious in the picker, not to make the
// choice automatically — several profiles can legitimately hold a zone of the
// same name.
func scanProfilesForParentZone(ctx context.Context, profiles []string, parentDomain string, publicNS []string) []parentZoneCandidate {
	results := make([]parentZoneCandidate, len(profiles))

	var wg sync.WaitGroup
	// Modest concurrency: each profile may trigger an SSO token refresh.
	sem := make(chan struct{}, 6)

	for i, profile := range profiles {
		wg.Add(1)
		go func(i int, profile string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			c := parentZoneCandidate{Profile: profile}

			// Bound each profile: an unreachable or unauthenticated profile must
			// not stall the whole scan.
			profileCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()

			zoneID, ns, err := findHostedZoneByName(profileCtx, profile, parentDomain)
			if err != nil {
				c.Err = err
				results[i] = c
				return
			}

			c.ZoneID = zoneID
			c.Nameservers = ns
			if zoneID != "" {
				c.Authoritative = nameserverSetsMatch(ns, publicNS)
				c.AccountID = lookupAccountID(profileCtx, profile)
			}
			results[i] = c
		}(i, profile)
	}
	wg.Wait()

	// Most useful first: authoritative, then has-a-zone, then the rest.
	sort.SliceStable(results, func(a, b int) bool {
		ra, rb := results[a], results[b]
		rank := func(c parentZoneCandidate) int {
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
		return rank(ra) < rank(rb)
	})

	return results
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
func applyDelegation(req delegationRequest) error {
	if err := req.Validate(); err != nil {
		return err
	}

	err := createNSRecordDelegation("", req.ParentProfile, req.ParentZoneID, req.Subdomain, req.Nameservers)
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

// recordDelegation remembers a successful delegation in dns.yaml so that other
// environments of the same project never have to ask again.
func recordDelegation(rootDomain, subdomain, accountID, zoneID string, nameservers []string) error {
	cfg, err := loadDNSConfig()
	if err != nil {
		return err
	}
	if cfg == nil {
		cfg = &DNSConfig{RootDomain: rootDomain}
	}
	if cfg.RootDomain == "" {
		cfg.RootDomain = rootDomain
	}

	addOrUpdateDelegatedZone(cfg, DelegatedZone{
		Subdomain: subdomain,
		AccountID: accountID,
		ZoneID:    zoneID,
		NSRecords: nameservers,
		Status:    "delegated",
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
