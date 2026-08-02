package main

import (
	"errors"
	"strings"
	"testing"
)

func TestLooksLikeRoute53(t *testing.T) {
	route53 := []string{
		"ns-523.awsdns-01.net.",
		"ns-1484.awsdns-57.org.",
		"ns-1800.awsdns-33.co.uk.",
		"ns-346.awsdns-43.com.",
	}
	if !looksLikeRoute53(route53) {
		t.Error("expected AWS nameservers to be recognised as Route53")
	}

	others := [][]string{
		{"ada.ns.cloudflare.com.", "rex.ns.cloudflare.com."},
		{"ns1.digitalocean.com.", "ns2.digitalocean.com."},
		{"ns01.domaincontrol.com.", "ns02.domaincontrol.com."},
		{},
	}
	for _, ns := range others {
		if looksLikeRoute53(ns) {
			t.Errorf("expected %v not to be recognised as Route53", ns)
		}
	}
}

// A request must be rejected before any AWS call when it is incomplete —
// especially an empty nameserver list, which would otherwise write an NS record
// with no values and break the parent zone.
func TestDelegationRequest_Validate(t *testing.T) {
	valid := delegationRequest{
		ParentProfile: "mag",
		ParentZoneID:  "Z03970472IHTTTNCP0DZD",
		Subdomain:     "dev.coretechx.dev",
		Nameservers:   []string{"ns-1930.awsdns-49.co.uk"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid request to pass, got %v", err)
	}

	cases := map[string]delegationRequest{
		"missing profile":     {ParentZoneID: "Z1", Subdomain: "a.b.com", Nameservers: []string{"ns-1"}},
		"missing zone":        {ParentProfile: "p", Subdomain: "a.b.com", Nameservers: []string{"ns-1"}},
		"missing subdomain":   {ParentProfile: "p", ParentZoneID: "Z1", Nameservers: []string{"ns-1"}},
		"missing nameservers": {ParentProfile: "p", ParentZoneID: "Z1", Subdomain: "a.b.com"},
		"blank profile":       {ParentProfile: "   ", ParentZoneID: "Z1", Subdomain: "a.b.com", Nameservers: []string{"ns-1"}},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			if err := req.Validate(); err == nil {
				t.Error("expected validation to fail")
			}
		})
	}
}

// applyDelegation must refuse invalid requests without reaching AWS.
func TestApplyDelegation_RejectsInvalidBeforeCallingAWS(t *testing.T) {
	err := applyDelegation(delegationRequest{ParentProfile: "p", ParentZoneID: "Z1", Subdomain: "a.b.com"})
	if err == nil {
		t.Fatal("expected an error for a request with no nameservers")
	}
	if !strings.Contains(err.Error(), "no nameservers") {
		t.Errorf("expected a validation error, got %v", err)
	}
}

// The picker must offer only profiles that actually hold a zone; profiles with
// errors or without the zone are noise and could be mis-selected.
func TestParentZoneCandidate_Label(t *testing.T) {
	const parent = "coretechx.dev"

	authoritative := parentZoneCandidate{
		Profile: "mag", AccountID: "891880437329", ZoneID: "Z039", Authoritative: true,
	}
	if got := authoritative.Label(parent); !strings.Contains(got, "matches public DNS") {
		t.Errorf("authoritative candidate should say it matches, got %q", got)
	}

	mismatched := parentZoneCandidate{
		Profile: "other", AccountID: "111", ZoneID: "Z999", Authoritative: false,
	}
	if got := mismatched.Label(parent); !strings.Contains(got, "does NOT match") {
		t.Errorf("mismatched candidate must be visibly flagged, got %q", got)
	}

	absent := parentZoneCandidate{Profile: "empty"}
	if got := absent.Label(parent); !strings.Contains(got, "no "+parent+" zone") {
		t.Errorf("absent candidate should say so, got %q", got)
	}

	broken := parentZoneCandidate{Profile: "expired", Err: errors.New("ExpiredToken: the token has expired")}
	if got := broken.Label(parent); !strings.Contains(got, "could not check") {
		t.Errorf("errored candidate should say so, got %q", got)
	}
}

func TestShortError_TruncatesAndStripsNewlines(t *testing.T) {
	err := errors.New("first line of the failure\nsecond line that should be dropped")
	if got := shortError(err); strings.Contains(got, "second line") {
		t.Errorf("expected only the first line, got %q", got)
	}

	long := errors.New(strings.Repeat("x", 200))
	if got := shortError(long); len(got) > 60 {
		t.Errorf("expected truncation to 60 chars, got %d", len(got))
	}
}

// The fallback must always state why meroku could not do it automatically, and
// must list every nameserver — a partial list produces a broken delegation.
func TestManualDelegationInstructions(t *testing.T) {
	ns := []string{
		"ns-1930.awsdns-49.co.uk",
		"ns-1050.awsdns-03.org",
		"ns-678.awsdns-20.net",
		"ns-247.awsdns-30.com",
	}
	out := manualDelegationInstructions("dev.coretechx.dev", "coretechx.dev", "parent is on Cloudflare", ns)

	if !strings.Contains(out, "parent is on Cloudflare") {
		t.Error("instructions must explain why we fell back")
	}
	for _, n := range ns {
		if !strings.Contains(out, n) {
			t.Errorf("instructions must list nameserver %s", n)
		}
	}
	if !strings.Contains(out, "dev.coretechx.dev") || !strings.Contains(out, "coretechx.dev") {
		t.Error("instructions must name both the subdomain and the parent zone")
	}
	if !strings.Contains(out, "meroku dns validate") {
		t.Error("instructions should tell the operator how to re-check")
	}
}
