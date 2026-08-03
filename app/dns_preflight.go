package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"gopkg.in/yaml.v2"
)

// dnsDebugEnabled gates the verbose resolver tracing in dns_api.go.
//
// Those messages are genuinely useful in the interactive DNS wizard, where the
// operator is actively diagnosing propagation. They are just noise during a
// deploy preflight, which runs every single time. Default off; the wizard turns
// it on.
var dnsDebugEnabled = false

// dnsDebugf prints resolver tracing only when dnsDebugEnabled is set.
func dnsDebugf(format string, args ...interface{}) {
	if dnsDebugEnabled {
		fmt.Printf(format, args...)
	}
}

// dnsPlan selects which deployment plan to run for an environment.
//
// A certificate can only be issued once its DNS validation record resolves on
// the public internet, which requires the zone to be delegated from its parent.
// modules/domain exports the certificate ARNs from aws_acm_certificate_validation
// (not from aws_acm_certificate), and module.workloads consumes those ARNs — so
// an undelegated zone does not merely fail the domain, it parks the entire apply
// until the validation times out. Deciding the plan up front turns a 20-75 minute
// silent hang into a two-second answer.
type dnsPlan int

const (
	// dnsPlanSkip - the environment has no custom domain. Normal single-phase apply.
	dnsPlanSkip dnsPlan = iota

	// dnsPlanNormal - zone exists and public delegation points at it. Normal apply.
	dnsPlanNormal

	// dnsPlanBootstrap - we are meant to create the zone and it does not exist yet.
	// Run phase 1 (zone only), hand off to the delegation flow, then phase 2.
	dnsPlanBootstrap

	// dnsPlanBlocked - the zone exists but the parent does not delegate to it.
	// Certificate validation cannot succeed; go to the delegation flow instead
	// of starting an apply that is guaranteed to stall.
	dnsPlanBlocked

	// dnsPlanMissingZone - create_domain_zone is false but no such zone exists in
	// this account. Today this surfaces as an opaque data.aws_route53_zone error.
	dnsPlanMissingZone
)

func (p dnsPlan) String() string {
	switch p {
	case dnsPlanSkip:
		return "skip"
	case dnsPlanNormal:
		return "normal"
	case dnsPlanBootstrap:
		return "bootstrap"
	case dnsPlanBlocked:
		return "blocked"
	case dnsPlanMissingZone:
		return "missing-zone"
	}
	return "unknown"
}

// dnsPreflightResult is the machine-readable outcome of the check.
//
// Deliberately a value rather than a print-and-exit, unlike AWSPreflightCheck:
// the caller has to branch on the plan, so it cannot be a bool or an os.Exit.
type dnsPreflightResult struct {
	Plan dnsPlan

	// ZoneName is the zone this environment needs, e.g. "dev.example.com".
	ZoneName string

	// ParentDomain is the zone that must carry the NS delegation record,
	// e.g. "example.com". Empty when ZoneName is already a root domain.
	ParentDomain string

	// ZoneID and ZoneNameservers are populated when the zone exists in this account.
	ZoneID          string
	ZoneNameservers []string

	// PublicNameservers is what the public internet currently returns for ZoneName.
	// Empty means no delegation exists at all.
	PublicNameservers []string

	// Reason is a human-readable explanation of the decision.
	Reason string
}

// NeedsDelegation reports whether the operator (or meroku) must write an NS
// record into the parent zone before the apply can succeed.
func (r dnsPreflightResult) NeedsDelegation() bool {
	return r.Plan == dnsPlanBootstrap || r.Plan == dnsPlanBlocked
}

// normalizeNameservers makes nameserver lists comparable across sources.
//
// Route53's DelegationSet returns "ns-1930.awsdns-49.co.uk" while public
// resolvers return "ns-1930.awsdns-49.co.uk." — and neither guarantees order.
// Comparing them raw reports "not delegated" for a correctly delegated zone.
func normalizeNameservers(ns []string) []string {
	out := make([]string, 0, len(ns))
	for _, n := range ns {
		n = strings.ToLower(strings.TrimSpace(n))
		n = strings.TrimSuffix(n, ".")
		if n != "" {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// nameserverSetsMatch reports whether two nameserver lists describe the same
// delegation, ignoring order, trailing dots and case.
//
// This is the load-bearing safety check: it is what proves a zone we found in
// some AWS account really is the zone the internet is using, so that we never
// write a delegation record into a same-named zone belonging to someone else.
func nameserverSetsMatch(a, b []string) bool {
	na, nb := normalizeNameservers(a), normalizeNameservers(b)
	if len(na) == 0 || len(na) != len(nb) {
		return false
	}
	for i := range na {
		if na[i] != nb[i] {
			return false
		}
	}
	return true
}

// expectedZoneName reproduces the zone name that env/main.hbs will actually
// generate. It must match, or the preflight inspects the wrong zone.
//
// The template has three branches (env/main.hbs):
//
//	prod                     -> add_env_domain_prefix = false          (zone = domain_name)
//	non-prod + root_zone_id  -> domain_zone = "<env>.<domain_name>"    (zone = env.domain_name)
//	fallback                 -> add_env_domain_prefix =
//	                            {{default domain.add_env_domain_prefix true}}
//
// The last one is the trap: the template defaults the flag to TRUE when the YAML
// key is absent, while Go's zero value for the bool is false. Reading
// e.Domain.AddEnvDomainPrefix directly therefore gets the common case exactly
// backwards — most project YAML files omit the key entirely.
func expectedZoneName(e Env) string {
	base := strings.TrimSuffix(strings.TrimSpace(e.Domain.DomainName), ".")
	if base == "" {
		return ""
	}

	// Production always uses the root domain.
	if e.Env == "prod" {
		return base
	}

	// Cross-account delegation branch: the subdomain is baked into domain_zone.
	if e.Domain.RootZoneID != "" {
		return e.Env + "." + base
	}

	if envDomainPrefixEnabled(e) {
		return e.Env + "." + base
	}
	return base
}

// envDomainPrefixEnabled resolves add_env_domain_prefix with the template's
// semantics: present-and-false means no prefix, anything else means prefix.
//
// A plain bool cannot distinguish "absent" from "false", so the raw YAML is
// consulted for key presence. If it cannot be read we fall back to the
// template's default of true rather than silently choosing the other branch.
func envDomainPrefixEnabled(e Env) bool {
	set, ok := yamlBoolIsSet(e.Env+".yaml", "domain", "add_env_domain_prefix")
	if !ok {
		return true // template default
	}
	return set
}

// yamlBoolIsSet reports the value of a nested boolean key and whether it was
// present at all.
func yamlBoolIsSet(path string, section, key string) (value bool, present bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, false
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return false, false
	}

	raw, ok := doc[section]
	if !ok {
		return false, false
	}

	// gopkg.in/yaml.v2 decodes nested maps as map[interface{}]interface{}.
	var field interface{}
	switch m := raw.(type) {
	case map[string]interface{}:
		field, ok = m[key]
	case map[interface{}]interface{}:
		field, ok = m[key]
	default:
		return false, false
	}
	if !ok || field == nil {
		return false, false
	}

	b, isBool := field.(bool)
	if !isBool {
		return false, false
	}
	return b, true
}

// findHostedZoneByName looks up a public hosted zone by exact name in the
// account reachable through profile. Returns an empty zoneID when absent.
func findHostedZoneByName(ctx context.Context, profile, zoneName string) (zoneID string, nameservers []string, err error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(profile))
	if err != nil {
		return "", nil, fmt.Errorf("failed to load AWS config for profile %q: %w", profile, err)
	}
	client := route53.NewFromConfig(cfg)

	want := strings.TrimSuffix(strings.ToLower(zoneName), ".")
	resp, err := client.ListHostedZonesByName(ctx, &route53.ListHostedZonesByNameInput{
		DNSName: aws.String(zoneName),
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to list hosted zones: %w", err)
	}

	for _, zone := range resp.HostedZones {
		if zone.Name == nil || zone.Id == nil {
			continue
		}
		// Private zones never serve the public internet, so they can never
		// satisfy certificate validation.
		if zone.Config != nil && zone.Config.PrivateZone {
			continue
		}
		if strings.TrimSuffix(strings.ToLower(*zone.Name), ".") != want {
			continue
		}

		fullZoneID := *zone.Id
		ns, nsErr := getZoneNameservers(ctx, client, fullZoneID)
		if nsErr != nil {
			return "", nil, fmt.Errorf("found zone %s but could not read its nameservers: %w", want, nsErr)
		}
		return strings.TrimPrefix(fullZoneID, "/hostedzone/"), ns, nil
	}

	return "", nil, nil
}

// checkDNSPreflight decides which deployment plan an environment needs.
//
// It performs read-only AWS and DNS lookups and never mutates anything.
func checkDNSPreflight(ctx context.Context, e Env) (dnsPreflightResult, error) {
	res := dnsPreflightResult{Plan: dnsPlanSkip}

	if !e.Domain.Enabled {
		res.Reason = "domain.enabled is false — no custom domain for this environment"
		return res, nil
	}

	res.ZoneName = expectedZoneName(e)
	if res.ZoneName == "" {
		res.Plan = dnsPlanMissingZone
		res.Reason = "domain.enabled is true but domain.domain_name is empty"
		return res, nil
	}
	res.ParentDomain = strings.TrimSuffix(getParentDomain(res.ZoneName), ".")

	zoneID, zoneNS, err := findHostedZoneByName(ctx, e.AWSProfile, res.ZoneName)
	if err != nil {
		return res, err
	}
	res.ZoneID = zoneID
	res.ZoneNameservers = zoneNS

	// Zone absent.
	if zoneID == "" {
		if e.Domain.CreateDomainZone {
			res.Plan = dnsPlanBootstrap
			res.Reason = fmt.Sprintf(
				"zone %s does not exist yet — create it first, delegate it, then deploy the rest",
				res.ZoneName)
			return res, nil
		}
		res.Plan = dnsPlanMissingZone
		res.Reason = fmt.Sprintf(
			"create_domain_zone is false but no public hosted zone named %s exists in account %s",
			res.ZoneName, e.AccountID)
		return res, nil
	}

	// Zone present — is the internet actually pointed at it?
	// A resolution error here means "cannot prove delegation", which we treat the
	// same as absent: better to stop and show the records than to stall an apply.
	publicNS, nsErr := queryNameservers(res.ZoneName)
	if nsErr == nil {
		res.PublicNameservers = publicNS
	}

	// A resolver agreeing is not proof. It answers from cache, and a cache can
	// outlive the record that put it there: a domain that moves DNS provider
	// leaves resolvers serving its old subdomain delegations for as long as the
	// TTL allows. Where we know which zone should carry the record, read that
	// zone instead — the deploy is about to spend twenty minutes on a certificate
	// that depends on the answer being true rather than merely cached.
	if profile, ok := cachedParentProfile(res.ParentDomain); ok && res.ParentDomain != "" {
		if parentZoneID, _, zErr := findHostedZoneByName(ctx, profile, res.ParentDomain); zErr == nil && parentZoneID != "" {
			if check, cErr := verifyDelegationInParentZone(ctx, profile, parentZoneID, res.ZoneName, zoneNS); cErr == nil && !(check.Present && check.Matches) {
				res.Plan = dnsPlanBlocked
				res.Reason = describeDelegationCheck(check, res.ZoneName, res.ParentDomain)
				return res, nil
			}
		}
	}

	switch {
	case nameserverSetsMatch(zoneNS, res.PublicNameservers):
		res.Plan = dnsPlanNormal
		res.Reason = fmt.Sprintf("zone %s is delegated to this account's Route53 zone %s", res.ZoneName, zoneID)

	case len(res.PublicNameservers) == 0:
		res.Plan = dnsPlanBlocked
		res.Reason = fmt.Sprintf(
			"zone %s exists in this account but the public internet has no NS delegation for it",
			res.ZoneName)

	default:
		res.Plan = dnsPlanBlocked
		res.Reason = fmt.Sprintf(
			"zone %s is delegated to %s, which is not this account's Route53 zone (%s)",
			res.ZoneName,
			strings.Join(normalizeNameservers(res.PublicNameservers), ", "),
			strings.Join(normalizeNameservers(zoneNS), ", "))
	}

	return res, nil
}

// describeDNSPreflight renders the decision for the terminal.
func describeDNSPreflight(res dnsPreflightResult) string {
	var b strings.Builder

	switch res.Plan {
	case dnsPlanSkip:
		return "🌐 DNS: skipped (no custom domain)\n"
	case dnsPlanNormal:
		b.WriteString("🌐 DNS: ✅ delegation verified\n")
		b.WriteString(fmt.Sprintf("   %s → zone %s\n", res.ZoneName, res.ZoneID))
		return b.String()
	case dnsPlanBootstrap:
		b.WriteString("🌐 DNS: zone does not exist yet — deploying in two phases\n")
		b.WriteString(fmt.Sprintf("   %s\n", res.Reason))
		b.WriteString("   Phase 1 creates the zone, then you delegate it, then the full deploy runs.\n")
		return b.String()
	case dnsPlanBlocked:
		b.WriteString("🌐 DNS: ⛔ delegation missing — stopping before the apply stalls\n")
		b.WriteString(fmt.Sprintf("   %s\n", res.Reason))
		if len(res.ZoneNameservers) > 0 && res.ParentDomain != "" {
			b.WriteString(fmt.Sprintf("\n   Add an NS record for %s in the %s zone, pointing at:\n",
				res.ZoneName, res.ParentDomain))
			for _, ns := range res.ZoneNameservers {
				b.WriteString(fmt.Sprintf("     %s\n", ns))
			}
		}
		return b.String()
	case dnsPlanMissingZone:
		b.WriteString("🌐 DNS: ⛔ configuration error\n")
		b.WriteString(fmt.Sprintf("   %s\n", res.Reason))
		return b.String()
	}
	return ""
}
