package main

import (
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"strings"
	"testing"
)

// TEST-1; REQ-1/R1.1, REQ-2/R1.2, REQ-3/R1.3, REQ-8/R2.
// Picker exclusions are not validator exclusions for an existing deployment.
func TestValidateVPCCIDR_AcceptsPrivateNetworks(t *testing.T) {
	tests := []struct {
		name, value, want string
	}{
		{"legacy default is valid", "10.0.0.0/16", "10.0.0.0/16"},
		{"second tool default is valid when explicit", "10.1.0.0/16", "10.1.0.0/16"},
		{"Porter default is valid when explicit", "10.99.0.0/16", "10.99.0.0/16"},
		{"first ordinary pool member", "10.2.0.0/16", "10.2.0.0/16"},
		{"last ten range", "10.255.0.0/16", "10.255.0.0/16"},
		{"seventeen", "10.42.128.0/17", "10.42.128.0/17"},
		{"eighteen", "10.42.192.0/18", "10.42.192.0/18"},
		{"Go smallest range", "10.42.224.0/19", "10.42.224.0/19"},
		{"192 lower endpoint", "192.168.0.0/16", "192.168.0.0/16"},
		{"192 upper network", "192.168.224.0/19", "192.168.224.0/19"},
		{"surrounding input whitespace", " \t10.20.0.0/16\n", "10.20.0.0/16"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateVPCCIDR(tt.value)
			if err != nil {
				t.Fatalf("validateVPCCIDR(%q): %v", tt.value, err)
			}
			if want := netip.MustParsePrefix(tt.want); got != want {
				t.Errorf("range = %s, want %s", got, want)
			}
		})
	}
}

// TEST-2; REQ-1/R1.1, REQ-2/R1.2, REQ-3/R1.3, REQ-4/R1.4.
func TestValidateVPCCIDR_RejectsRuleViolations(t *testing.T) {
	tests := []struct {
		name, value, code string
	}{
		{"empty", "", "invalid_format"},
		{"whitespace", " \t\n", "invalid_format"},
		{"missing size", "10.42.0.0", "invalid_format"},
		{"non-IP", "not-a-network/16", "invalid_format"},
		{"invalid octet", "10.256.0.0/16", "invalid_format"},
		{"invalid size", "10.42.0.0/33", "invalid_format"},
		{"IPv6", "fd00::/16", "invalid_format"},
		{"IPv4 mapped IPv6", "::ffff:10.42.0.0/112", "invalid_format"},
		{"before ten private range", "9.255.0.0/16", "not_private"},
		{"after ten private range", "11.0.0.0/16", "not_private"},
		{"before 192 private range", "192.167.0.0/16", "not_private"},
		{"after 192 private range", "192.169.0.0/16", "not_private"},
		{"range extends beyond 192 private space", "192.168.0.0/15", "not_private"},
		{"before reserved 172 range", "172.15.0.0/16", "not_private"},
		{"first 172 reserved network", "172.16.0.0/16", "reserved_range"},
		{"last 172 reserved network", "172.31.0.0/16", "reserved_range"},
		{"after reserved 172 range", "172.32.0.0/16", "not_private"},
		{"before metadata", "169.253.0.0/16", "not_private"},
		{"metadata", "169.254.0.0/16", "reserved_range"},
		{"after metadata", "169.255.0.0/16", "not_private"},
		{"before AWS restricted network", "198.18.0.0/16", "not_private"},
		{"AWS restricted network", "198.19.0.0/16", "reserved_range"},
		{"after AWS restricted network", "198.20.0.0/16", "not_private"},
		{"one step too large", "10.42.0.0/15", "size"},
		{"one step too small in Go", "10.42.0.0/20", "size"},
		{"smaller than both Go and Terraform", "10.42.0.0/21", "size"},
		{"single private address", "10.42.0.0/32", "size"},
		{"host bits in sixteen", "10.42.1.0/16", "not_network_address"},
		{"host bits in nineteen", "10.5.33.0/19", "not_network_address"},
		{"reserved wins over host correction", "172.20.1.0/16", "reserved_range"},
		{"public wins over host correction", "8.8.8.8/16", "not_private"},
		{"size wins over host correction", "10.42.1.0/20", "size"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateVPCCIDR(tt.value)
			var issue *vpcCIDRError
			if !errors.As(err, &issue) || issue == nil {
				t.Fatalf("error = %v, want *vpcCIDRError", err)
			}
			if issue.Code != tt.code {
				t.Errorf("code = %q, want %q for %q", issue.Code, tt.code, tt.value)
			}
			if issue.Message == "" || issue.Error() != issue.Message {
				t.Errorf("issue Error() = %q, Message = %q; want the non-empty technical message", issue.Error(), issue.Message)
			}
		})
	}
}

// TEST-3; REQ-1/R1.1, REQ-22/L1, REQ-23/L2.
func TestValidateVPCCIDR_ReturnsSafeFixesAndPlainHints(t *testing.T) {
	tests := []struct {
		value, code, fix string
	}{
		{"10.5.1.0/16", "not_network_address", "10.5.0.0/16"},
		{"10.5.33.0/19", "not_network_address", "10.5.32.0/19"},
		{"172.20.1.0/16", "reserved_range", ""},
		{"169.254.1.0/16", "reserved_range", ""},
		{"198.19.1.0/16", "reserved_range", ""},
		{"8.8.8.8/16", "not_private", ""},
		{"10.5.1.0/20", "size", ""},
		{"10.5.0.0/not-a-number", "invalid_format", ""},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			_, err := validateVPCCIDR(tt.value)
			var issue *vpcCIDRError
			if !errors.As(err, &issue) || issue == nil {
				t.Fatalf("error = %v, want typed issue", err)
			}
			if issue.Code != tt.code || issue.Fix != tt.fix {
				t.Fatalf("issue = %+v, want code %q, fix %q", issue, tt.code, tt.fix)
			}
			cidrQAAssertPlainHint(t, issue.Hint)
			if tt.fix != "" {
				if !strings.Contains(issue.Hint, tt.fix) {
					t.Errorf("hint %q does not offer exact masked value %s", issue.Hint, tt.fix)
				}
				if _, err := validateVPCCIDR(issue.Fix); err != nil {
					t.Errorf("offered Fix %q itself fails the rules: %v", issue.Fix, err)
				}
			}
		})
	}
}

// TEST-4; REQ-8/R2, REQ-32.
func TestEffectiveVPCCIDR_PreservesExistingDefaults(t *testing.T) {
	tests := []struct {
		name             string
		useDefault       bool
		configured, want string
	}{
		{"unset custom retains module default", false, "", "10.0.0.0/16"},
		{"explicit custom retained", false, "192.168.32.0/19", "192.168.32.0/19"},
		{"explicit legacy retained", false, "10.99.0.0/16", "10.99.0.0/16"},
		{"default VPC ignores configured range", true, "10.42.0.0/16", ""},
		{"default VPC with no range", true, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveVPCCIDR(tt.useDefault, tt.configured); got != tt.want {
				t.Errorf("effective = %q, want %q", got, tt.want)
			}
		})
	}
}

// TEST-5; REQ-5/R1.5, REQ-20/M6, REQ-22/L1.
func TestVPCCIDROverlapWarnings_ReportOnlyIntersections(t *testing.T) {
	p := netip.MustParsePrefix("10.42.0.0/16")
	uses := []vpcCIDRUse{
		{Source: `env "staging"`, Name: "staging", File: "staging.yaml", Prefix: netip.MustParsePrefix("10.42.0.0/16")},
		{Source: `env "nested"`, Name: "nested", File: "nested.yml", Prefix: netip.MustParsePrefix("10.42.224.0/19")},
		{Source: `env "wide"`, Name: "wide", File: "wide.yaml", Prefix: netip.MustParsePrefix("10.0.0.0/8")},
		{Source: `env "next"`, Name: "next", File: "next.yaml", Prefix: netip.MustParsePrefix("10.43.0.0/16")},
		{Source: `env "previous"`, Name: "previous", File: "previous.yaml", Prefix: netip.MustParsePrefix("10.41.0.0/16")},
		{Source: `env "other"`, Name: "other", File: "other.yaml", Prefix: netip.MustParsePrefix("192.168.0.0/16")},
	}
	overlaps := overlappingUses(p, uses)
	got := map[string]bool{}
	for _, u := range overlaps {
		got[u.Name] = true
	}
	want := map[string]bool{"staging": true, "nested": true, "wide": true}
	if len(overlaps) != 3 || !reflect.DeepEqual(got, want) {
		t.Errorf("overlaps = %+v, want only staging/nested/wide", overlaps)
	}
	warnings := vpcCIDROverlapWarnings(p, uses)
	if len(warnings) != 3 {
		t.Fatalf("warnings = %+v, want 3", warnings)
	}
	bySource := map[string]vpcCIDRWarning{}
	for _, w := range warnings {
		bySource[w.Source] = w
		cidrQAAssertPlainHint(t, w.Hint)
	}
	staging, ok := bySource[`env "staging"`]
	if !ok || !strings.Contains(staging.Message, "staging.yaml") || !strings.Contains(staging.Message, "10.42.0.0/16") || !strings.Contains(staging.Hint, "staging") {
		t.Errorf("staging warning lacks technical file or plain name: %+v", staging)
	}
	if empty := vpcCIDROverlapWarnings(netip.MustParsePrefix("192.168.0.0/16"), uses[:5]); empty == nil || len(empty) != 0 {
		t.Errorf("non-overlap warnings = %#v, want non-nil empty list", empty)
	}
}

// TEST-6; REQ-8/R2, REQ-11/R5.
func TestPickVPCCIDRs_ReturnsExactlyCuratedPool(t *testing.T) {
	got, err := pickVPCCIDRs(cidrQARand(42), nil, 300)
	if err != nil {
		t.Fatalf("pick whole pool: %v", err)
	}
	want := map[netip.Prefix]bool{}
	for n := 2; n <= 255; n++ {
		if n != 99 {
			want[netip.MustParsePrefix(fmt.Sprintf("10.%d.0.0/16", n))] = true
		}
	}
	if len(got) != 253 {
		t.Fatalf("pool has %d suggestions, want 253", len(got))
	}
	for _, p := range got {
		cidrQAAssertCurated(t, p)
		if !want[p] {
			t.Errorf("unexpected or duplicate pool member %s", p)
		}
		delete(want, p)
	}
	if len(want) != 0 {
		t.Errorf("permitted pool members were omitted: %v", want)
	}
}

// TEST-7; REQ-5/R1.5, REQ-6/R1.6, REQ-8/R2, REQ-9/R3.
func TestPickVPCCIDRs_AvoidsUsedNetworks(t *testing.T) {
	used := []vpcCIDRUse{
		{Source: `env "large"`, Prefix: netip.MustParsePrefix("10.32.0.0/12")},
		{Source: `env "small"`, Prefix: netip.MustParsePrefix("10.80.128.0/17")},
		{Source: "another network in this AWS account", Prefix: netip.MustParsePrefix("10.120.0.0/15")},
		{Source: "another network in this AWS account", Prefix: netip.MustParsePrefix("10.200.224.0/19")},
	}
	got, err := pickVPCCIDRs(cidrQARand(7), used, 300)
	if err != nil {
		t.Fatalf("pick with used networks: %v", err)
	}
	// 16 + 1 + 2 + 1 distinct curated /16s are blocked.
	if len(got) != 233 {
		t.Errorf("free suggestions = %d, want 233", len(got))
	}
	values := make([]string, len(got))
	for i, p := range got {
		values[i] = p.String()
	}
	cidrQAAssertSuggestions(t, values, used)
}

// TEST-8; REQ-8/R2, REQ-11/R5.
func TestPickVPCCIDRs_UsesInjectedRandomness(t *testing.T) {
	a, err := pickVPCCIDRs(cidrQARand(0xc0ffee), nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	b, err := pickVPCCIDRs(cidrQARand(0xc0ffee), nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("same PCG seed did not reproduce suggestions: %v versus %v", a, b)
	}
	distinct := map[netip.Prefix]bool{}
	for seed := uint64(1); seed <= 32; seed++ {
		p, err := pickVPCCIDR(cidrQARand(seed), nil)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		cidrQAAssertCurated(t, p)
		distinct[p] = true
	}
	if len(distinct) < 2 {
		t.Errorf("all 32 fixed seeds returned the same range; default is not random: %v", distinct)
	}
}

// TEST-9; REQ-8/R2, REQ-9/R3, REQ-11/R5.
func TestPickVPCCIDRs_HandlesLimitsAndExhaustion(t *testing.T) {
	t.Run("one free range has an exact expected value", func(t *testing.T) {
		used := cidrQAPoolUsesExcept(42)
		p, err := pickVPCCIDR(cidrQARand(9), used)
		if err != nil {
			t.Fatal(err)
		}
		if p.String() != "10.42.0.0/16" {
			t.Errorf("single remaining range = %s, want 10.42.0.0/16", p)
		}
		ps, err := pickVPCCIDRs(cidrQARand(9), used, 5)
		if err != nil || len(ps) != 1 || ps[0].String() != "10.42.0.0/16" {
			t.Errorf("short pool = %v, %v; want [10.42.0.0/16], nil", ps, err)
		}
	})
	t.Run("two free ranges are distinct", func(t *testing.T) {
		ps, err := pickVPCCIDRs(cidrQARand(11), cidrQAPoolUsesExcept(42, 77), 5)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, p := range ps {
			got[p.String()] = true
		}
		if len(ps) != 2 || !reflect.DeepEqual(got, map[string]bool{"10.42.0.0/16": true, "10.77.0.0/16": true}) {
			t.Errorf("short pool = %v, want exactly the two free ranges", ps)
		}
	})
	exhausted := []vpcCIDRUse{{Source: "synthetic entire ten range", Prefix: netip.MustParsePrefix("10.0.0.0/8")}}
	t.Run("no free range returns sentinel", func(t *testing.T) {
		_, err := pickVPCCIDR(cidrQARand(3), exhausted)
		if !errors.Is(err, errVPCCIDRPoolExhausted) {
			t.Errorf("error = %v, want errVPCCIDRPoolExhausted", err)
		}
	})
	for _, n := range []int{0, -1} {
		t.Run(fmt.Sprintf("count %d is empty even when exhausted", n), func(t *testing.T) {
			ps, err := pickVPCCIDRs(cidrQARand(3), exhausted, n)
			if err != nil || len(ps) != 0 {
				t.Errorf("count %d = %v, %v; want empty, nil", n, ps, err)
			}
		})
	}
}
