package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Real values captured from a live environment: Route53's DelegationSet returns
// bare names in arbitrary order, public resolvers return FQDNs (trailing dot) in
// a different order. Comparing these raw reports "not delegated" for a zone that
// is delegated perfectly well.
func TestNameserverSetsMatch_IgnoresDotsCaseAndOrder(t *testing.T) {
	route53Style := []string{
		"ns-1930.awsdns-49.co.uk",
		"ns-1050.awsdns-03.org",
		"ns-678.awsdns-20.net",
		"ns-247.awsdns-30.com",
	}
	resolverStyle := []string{
		"ns-247.awsdns-30.com.",
		"NS-678.AWSDNS-20.NET.",
		"ns-1930.awsdns-49.co.uk.",
		"ns-1050.awsdns-03.org.",
	}

	if !nameserverSetsMatch(route53Style, resolverStyle) {
		t.Error("expected match despite trailing dots, case and ordering differences")
	}
}

// The safety property: a zone with the same name in a different AWS account has
// a different delegation set, and must never be treated as ours.
func TestNameserverSetsMatch_RejectsDifferentZone(t *testing.T) {
	ours := []string{
		"ns-1930.awsdns-49.co.uk",
		"ns-1050.awsdns-03.org",
		"ns-678.awsdns-20.net",
		"ns-247.awsdns-30.com",
	}
	somebodyElses := []string{
		"ns-523.awsdns-01.net.",
		"ns-1484.awsdns-57.org.",
		"ns-1800.awsdns-33.co.uk.",
		"ns-346.awsdns-43.com.",
	}

	if nameserverSetsMatch(ours, somebodyElses) {
		t.Error("must not match a different account's zone — this guards against writing delegation into the wrong zone")
	}
}

// An empty public result means "no delegation exists", never "matches".
// Without this guard two empty lists would compare equal and an undelegated
// zone would be reported as healthy.
func TestNameserverSetsMatch_EmptyNeverMatches(t *testing.T) {
	ns := []string{"ns-1.example.com"}

	if nameserverSetsMatch(nil, nil) {
		t.Error("two empty sets must not count as a match")
	}
	if nameserverSetsMatch(ns, nil) {
		t.Error("empty public nameservers must not match a populated zone")
	}
	if nameserverSetsMatch(nil, ns) {
		t.Error("empty zone nameservers must not match populated public records")
	}
}

// A partial overlap is not a delegation.
func TestNameserverSetsMatch_RejectsSubset(t *testing.T) {
	full := []string{"ns-1.example.com", "ns-2.example.com", "ns-3.example.com", "ns-4.example.com"}
	partial := []string{"ns-1.example.com", "ns-2.example.com"}

	if nameserverSetsMatch(full, partial) {
		t.Error("a subset of nameservers must not count as a match")
	}
}

// Whitespace and empty entries must not throw the comparison off.
func TestNormalizeNameservers_DropsBlanksAndTrims(t *testing.T) {
	got := normalizeNameservers([]string{"  ns-2.example.com. ", "", "ns-1.EXAMPLE.com", "   "})

	want := []string{"ns-1.example.com", "ns-2.example.com"}
	if len(got) != len(want) {
		t.Fatalf("expected %d entries, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

// expectedZoneName must reproduce what env/main.hbs generates.
//
// The regression this pins down: the template writes
// `add_env_domain_prefix = {{default domain.add_env_domain_prefix true}}`, so an
// absent YAML key means TRUE. Go's zero value for the bool is false, so reading
// the struct field directly inverted the common case — and most project YAML
// files (including coretechx's) omit the key. The live endpoint check caught
// this by reporting "coretechx.dev" where the generated Terraform used
// "dev.coretechx.dev".
func TestExpectedZoneName(t *testing.T) {
	cases := []struct {
		name string
		env  Env
		want string
	}{
		{
			name: "non-prod defaults to prefixed when the YAML key is absent",
			env:  Env{Env: "dev", Domain: Domain{DomainName: "coretechx.dev"}},
			want: "dev.coretechx.dev",
		},
		{
			name: "prod never prefixes",
			env:  Env{Env: "prod", Domain: Domain{DomainName: "coretechx.dev", AddEnvDomainPrefix: true}},
			want: "coretechx.dev",
		},
		{
			name: "cross-account delegation branch is prefixed",
			env:  Env{Env: "staging", Domain: Domain{DomainName: "coretechx.dev", RootZoneID: "Z123"}},
			want: "staging.coretechx.dev",
		},
		{
			name: "trailing dot and spaces are trimmed",
			env:  Env{Env: "staging", Domain: Domain{DomainName: " coretechx.dev. "}},
			want: "staging.coretechx.dev",
		},
		{
			name: "empty domain name",
			env:  Env{Env: "dev", Domain: Domain{DomainName: ""}},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := expectedZoneName(tc.env); got != tc.want {
				t.Errorf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

// An explicit `add_env_domain_prefix: false` in the YAML must suppress the
// prefix, while an absent key must not.
func TestEnvDomainPrefixEnabled_ReadsYAMLKeyPresence(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(wd)

	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	write("explicitfalse.yaml", "domain:\n  enabled: true\n  domain_name: example.com\n  add_env_domain_prefix: false\n")
	write("explicittrue.yaml", "domain:\n  enabled: true\n  domain_name: example.com\n  add_env_domain_prefix: true\n")
	write("absent.yaml", "domain:\n  enabled: true\n  domain_name: example.com\n")

	cases := map[string]bool{
		"explicitfalse": false,
		"explicittrue":  true,
		"absent":        true, // template default
		"nosuchfile":    true, // unreadable -> template default, never the other branch
	}

	for envName, want := range cases {
		t.Run(envName, func(t *testing.T) {
			e := Env{Env: envName, Domain: Domain{DomainName: "example.com"}}
			if got := envDomainPrefixEnabled(e); got != want {
				t.Errorf("expected %v, got %v", want, got)
			}

			wantZone := "example.com"
			if want {
				wantZone = envName + ".example.com"
			}
			if got := expectedZoneName(e); got != wantZone {
				t.Errorf("expected zone %q, got %q", wantZone, got)
			}
		})
	}
}

// domain.enabled=false must short-circuit before any AWS call.
func TestCheckDNSPreflight_SkipsWhenDomainDisabled(t *testing.T) {
	res, err := checkDNSPreflight(context.Background(), Env{Env: "dev", Domain: Domain{Enabled: false}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Plan != dnsPlanSkip {
		t.Errorf("expected dnsPlanSkip, got %v", res.Plan)
	}
	if res.NeedsDelegation() {
		t.Error("a skipped environment must not need delegation")
	}
}

// Enabled but no domain name is a config error, caught without touching AWS.
func TestCheckDNSPreflight_MissingDomainName(t *testing.T) {
	res, err := checkDNSPreflight(context.Background(), Env{Env: "dev", Domain: Domain{Enabled: true}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Plan != dnsPlanMissingZone {
		t.Errorf("expected dnsPlanMissingZone, got %v", res.Plan)
	}
}

func TestNeedsDelegation(t *testing.T) {
	cases := map[dnsPlan]bool{
		dnsPlanSkip:        false,
		dnsPlanNormal:      false,
		dnsPlanBootstrap:   true,
		dnsPlanBlocked:     true,
		dnsPlanMissingZone: false,
	}

	for plan, want := range cases {
		if got := (dnsPreflightResult{Plan: plan}).NeedsDelegation(); got != want {
			t.Errorf("plan %v: expected NeedsDelegation=%v, got %v", plan, want, got)
		}
	}
}
