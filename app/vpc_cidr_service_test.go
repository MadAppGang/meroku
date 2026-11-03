package main

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"testing"
)

// TEST-15; REQ-1/R1.1, REQ-2/R1.2, REQ-3/R1.3, REQ-4/R1.4,
// REQ-14/H3, REQ-18/M4.
func TestValidateVPCCIDRMap_GatesRulesAndProtectsDeployedEnvs(t *testing.T) {
	tests := []struct {
		name      string
		fields    map[string]interface{}
		wantError bool
	}{
		{"valid", map[string]interface{}{"use_default_vpc": false, "vpc_cidr": "10.42.0.0/16"}, false},
		{"unset retains implicit legacy default", map[string]interface{}{"use_default_vpc": false}, false},
		{"empty retains implicit legacy default", map[string]interface{}{"use_default_vpc": false, "vpc_cidr": ""}, false},
		{"default VPC skips even malformed custom input", map[string]interface{}{"use_default_vpc": true, "vpc_cidr": "bad/16"}, false},
		{"override skips rule errors", map[string]interface{}{"use_default_vpc": false, "vpc_cidr": "172.20.0.0/16", "allow_vpc_cidr_override": true}, false},
		{"format error", map[string]interface{}{"use_default_vpc": false, "vpc_cidr": "bad/16"}, true},
		{"network error", map[string]interface{}{"use_default_vpc": false, "vpc_cidr": "10.42.1.0/16"}, true},
		{"private error", map[string]interface{}{"use_default_vpc": false, "vpc_cidr": "8.8.0.0/16"}, true},
		{"reserved error", map[string]interface{}{"use_default_vpc": false, "vpc_cidr": "172.20.0.0/16"}, true},
		{"size error", map[string]interface{}{"use_default_vpc": false, "vpc_cidr": "10.42.0.0/20"}, true},
		{"explicit override false still validates", map[string]interface{}{"use_default_vpc": false, "vpc_cidr": "10.42.0.0/20", "allow_vpc_cidr_override": false}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fields["env"] = "yaml-env-not-the-filename"
			err := validateVPCCIDRMap("file-dev", tt.fields)
			if !tt.wantError {
				if err != nil {
					t.Errorf("unexpected rule error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid applicable custom range was accepted")
			}
			for _, text := range []string{
				"allow_vpc_cidr_override", "true", "do NOT change vpc_cidr",
				"meroku vpc-cidr assign file-dev",
			} {
				if !strings.Contains(err.Error(), text) {
					t.Errorf("unsafe or incomplete guidance: error %q lacks %q", err, text)
				}
			}
			if strings.Contains(err.Error(), "assign yaml-env-not-the-filename") {
				t.Errorf("guidance uses YAML env instead of the CLI's file stem: %v", err)
			}
		})
	}
}

// TEST-16; REQ-5/R1.5, REQ-16/M2.
func TestVPCCIDROverlapWarningText_ShowsTipOnlyWithOverlap(t *testing.T) {
	uses := []vpcCIDRUse{
		{Source: `env "staging"`, Name: "staging", File: "staging.yaml", Prefix: netip.MustParsePrefix("10.42.0.0/16")},
		{Source: `env "legacy"`, Name: "legacy", File: "legacy.yml", Prefix: netip.MustParsePrefix("10.0.0.0/16")},
	}
	const tip = "Tip: `meroku vpc-cidr check file-dev --aws` also checks VPCs already in your AWS account"
	tests := []struct {
		name        string
		fields      map[string]interface{}
		wantWarning bool
	}{
		{"explicit overlap", map[string]interface{}{"use_default_vpc": false, "vpc_cidr": "10.42.0.0/16"}, true},
		{"unset overlaps implicit default", map[string]interface{}{"use_default_vpc": false}, true},
		{"override keeps overlap warnings", map[string]interface{}{"use_default_vpc": false, "vpc_cidr": "10.42.0.0/16", "allow_vpc_cidr_override": true}, true},
		{"free range", map[string]interface{}{"use_default_vpc": false, "vpc_cidr": "10.77.0.0/16"}, false},
		{"default VPC", map[string]interface{}{"use_default_vpc": true, "vpc_cidr": "10.42.0.0/16"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := vpcCIDROverlapWarningText("file-dev", tt.fields, uses)
			if !tt.wantWarning {
				if text != "" {
					t.Errorf("non-overlap returned warning/tip: %q", text)
				}
				return
			}
			if !strings.Contains(text, tip) || strings.Count(text, "Tip:") != 1 {
				t.Errorf("warning lacks the single specified M2 tip: %q", text)
			}
			if !strings.Contains(strings.ToLower(text), "overlap") {
				t.Errorf("tip is not accompanied by an overlap warning: %q", text)
			}
		})
	}
}

// TEST-17; REQ-5/R1.5, REQ-13/H2, REQ-18/M4, REQ-32.
func TestInspectVPCCIDR_ReportsEffectiveConfiguration(t *testing.T) {
	uses := []vpcCIDRUse{
		{Source: `env "staging"`, Name: "staging", File: "staging.yaml", Prefix: netip.MustParsePrefix("10.42.0.0/16")},
		{Source: `env "legacy"`, Name: "legacy", File: "legacy.yaml", Prefix: netip.MustParsePrefix("10.0.0.0/16")},
		{Source: `env "reserved"`, Name: "reserved", File: "reserved.yaml", Prefix: netip.MustParsePrefix("172.20.0.0/16")},
	}
	tests := []struct {
		name                        string
		fields                      map[string]interface{}
		configured, effective, code string
		warnings                    int
		override, defaultVPC        bool
	}{
		{"free custom", map[string]interface{}{"vpc_cidr": "10.77.0.0/16"}, "10.77.0.0/16", "10.77.0.0/16", "", 0, false, false},
		{"overlap is a warning", map[string]interface{}{"vpc_cidr": "10.42.0.0/16"}, "10.42.0.0/16", "10.42.0.0/16", "", 1, false, false},
		{"unset warns against actual module default", nil, "", "10.0.0.0/16", "", 1, false, false},
		{"invalid configured range", map[string]interface{}{"vpc_cidr": "10.77.0.0/20"}, "10.77.0.0/20", "10.77.0.0/20", "size", 0, false, false},
		{"override suppresses rules not overlaps", map[string]interface{}{"vpc_cidr": "172.20.0.0/16", "allow_vpc_cidr_override": true}, "172.20.0.0/16", "172.20.0.0/16", "", 1, true, false},
		{"default VPC ignores configured range", map[string]interface{}{"use_default_vpc": true, "vpc_cidr": "10.42.0.0/16"}, "10.42.0.0/16", "", "", 0, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := inspectVPCCIDR(context.Background(), cidrQADeps(uses), cidrQAEnv(t, tt.fields),
				vpcCIDRInspectOptions{EnvName: "file-dev", Dir: t.TempDir(), Count: 3})
			if report.Env != "file-dev" || report.Configured != tt.configured || report.Effective != tt.effective {
				t.Errorf("report identity/ranges = %+v, want file-dev, configured %q, effective %q", report, tt.configured, tt.effective)
			}
			if report.Override != tt.override || report.UseDefaultVPC != tt.defaultVPC {
				t.Errorf("report flags = override %v default %v, want %v/%v", report.Override, report.UseDefaultVPC, tt.override, tt.defaultVPC)
			}
			if tt.code == "" {
				if report.Error != nil {
					t.Errorf("unexpected configured error: %+v", report.Error)
				}
			} else if report.Error == nil || report.Error.Code != tt.code {
				t.Errorf("configured error = %+v, want code %q", report.Error, tt.code)
			}
			if report.Warnings == nil || len(report.Warnings) != tt.warnings {
				t.Errorf("warnings = %#v, want non-nil length %d", report.Warnings, tt.warnings)
			}
			if report.AWSChecked || report.Deployed != "no" {
				t.Errorf("offline no-target report unexpectedly used/required AWS: %+v", report)
			}
			if !tt.defaultVPC && len(report.Suggestions) != 3 {
				t.Errorf("suggestion count = %d, want 3", len(report.Suggestions))
			}
			cidrQAAssertSuggestions(t, report.Suggestions, uses)
		})
	}
}

// TEST-18; REQ-6/R1.6, REQ-9/R3, REQ-20/M6.
// The external fake is target-dependent; assertions concern networks in the
// returned report, not which dependency functions were invoked.
func TestInspectVPCCIDR_AvoidsTargetAccountNetworks(t *testing.T) {
	t.Setenv("AWS_PROFILE", "ambient-profile-do-not-use")
	env := cidrQAEnv(t, map[string]interface{}{
		"account_id": "000000000000", "aws_profile": "fixture-profile", "vpc_cidr": "10.77.0.0/16",
	})
	siblings := []vpcCIDRUse{{Source: `env "staging"`, Name: "staging", File: "staging.yaml", Prefix: netip.MustParsePrefix("10.42.0.0/16")}}
	deps := cidrQADeps(siblings)
	deps.aws = func(ctx context.Context, target Env) (vpcCIDRAWS, error) {
		m := cidrQAEnvMap(t, target)
		networks := []netip.Prefix{netip.MustParsePrefix("10.200.0.0/16")}
		if m["aws_profile"] == "fixture-profile" && m["region"] == "us-east-1" {
			networks = []netip.Prefix{netip.MustParsePrefix("10.41.0.0/16"), netip.MustParsePrefix("10.77.0.0/16")}
		}
		return cidrQAFakeAWS([]awsVPC{{
			ID: "vpc-synthetic-target", CIDRs: networks,
			Tags: map[string]string{"Name": "synthetic-target-network"},
		}}, nil)(ctx, target)
	}
	report := inspectVPCCIDR(context.Background(), deps, env,
		vpcCIDRInspectOptions{EnvName: "file-dev", Dir: t.TempDir(), WithAWS: true, Count: 10})
	if !report.AWSChecked || report.Error != nil {
		t.Fatalf("AWS report did not complete as a warning-only check: %+v", report)
	}
	if len(report.Warnings) != 1 || report.Warnings[0].Source != "another network in this AWS account" {
		t.Errorf("expected the YAML-target secondary-network warning, got %+v", report.Warnings)
	}
	used := append(append([]vpcCIDRUse(nil), siblings...),
		vpcCIDRUse{Source: "target primary", Prefix: netip.MustParsePrefix("10.41.0.0/16")},
		vpcCIDRUse{Source: "target secondary", Prefix: netip.MustParsePrefix("10.77.0.0/16")})
	if len(report.Suggestions) != 10 {
		t.Errorf("suggestion count = %d, want 10", len(report.Suggestions))
	}
	cidrQAAssertSuggestions(t, report.Suggestions, used)
	for _, warning := range report.Warnings {
		if strings.Contains(warning.Message+warning.Hint+warning.Source, "vpc-synthetic") || strings.Contains(warning.Message+warning.Hint, "synthetic-target-network") {
			t.Errorf("AWS warning leaked VPC identity: %+v", warning)
		}
	}
}

// TEST-19; REQ-6/R1.6, REQ-13/H2, REQ-32.
func TestInspectVPCCIDR_DegradesAWSFailures(t *testing.T) {
	tests := []struct {
		name                       string
		factoryError, listingError error
		cancel                     bool
	}{
		{"credentials unavailable", errors.New("synthetic credential diagnostic"), nil, false},
		{"DescribeVpcs unavailable", nil, errors.New("synthetic endpoint diagnostic"), false},
		{"already cancelled", nil, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := cidrQAEnv(t, map[string]interface{}{
				"account_id": "000000000000", "aws_profile": "fixture-profile", "vpc_cidr": "10.42.0.0/16",
			})
			siblings := []vpcCIDRUse{{Source: `env "staging"`, Name: "staging", File: "staging.yaml", Prefix: netip.MustParsePrefix("10.42.0.0/16")}}
			deps := cidrQADeps(siblings)
			if tt.factoryError != nil {
				deps.aws = func(context.Context, Env) (vpcCIDRAWS, error) { return vpcCIDRAWS{}, tt.factoryError }
			} else {
				deps.aws = cidrQAFakeAWS(nil, tt.listingError)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.cancel {
				cancel()
			}
			report := inspectVPCCIDR(ctx, deps, env,
				vpcCIDRInspectOptions{EnvName: "dev", Dir: t.TempDir(), WithAWS: true, Count: 5})
			if report.AWSChecked || report.AWSNote == "" || report.Deployed != "unknown" {
				t.Errorf("AWS failure did not degrade to noted, unconfirmed inspection: %+v", report)
			}
			if report.Error != nil || len(report.Warnings) != 1 || len(report.Suggestions) != 5 {
				t.Errorf("offline sibling result was lost on AWS failure: %+v", report)
			}
			cidrQAAssertSuggestions(t, report.Suggestions, siblings)
		})
	}
}

// TEST-20; REQ-9/R3, REQ-13/H2.
// No fabricated remote-state signature or guessed own-VPC tag convention.
func TestVPCCIDRDeployed_DistinguishesConfirmedAndUnconfirmedStates(t *testing.T) {
	tests := []struct {
		name                   string
		fields                 map[string]interface{}
		withAWS, ready, listed bool
		own                    []awsVPC
		awsError, want         string
	}{
		{"no target offline", nil, false, false, false, nil, "", "no"},
		{"no target wins even if AWS failed", nil, true, false, false, nil, "synthetic failure", "no"},
		{"account alone is unchecked offline", map[string]interface{}{"account_id": "000000000000"}, false, false, false, nil, "", "unchecked"},
		{"profile alone is unchecked offline", map[string]interface{}{"aws_profile": "fixture-profile"}, false, false, false, nil, "", "unchecked"},
		{"own listed VPC confirms deployment", map[string]interface{}{"account_id": "000000000000"}, true, true, true, []awsVPC{{ID: "vpc-synthetic-own"}}, "", "yes"},
		{"credentials failed", map[string]interface{}{"account_id": "000000000000"}, true, false, false, nil, "synthetic failure", "unknown"},
		{"nil state reader cannot confirm absence", map[string]interface{}{"aws_profile": "fixture-profile"}, true, true, true, nil, "", "unknown"},
		{"no successful listing", map[string]interface{}{"aws_profile": "fixture-profile"}, true, true, false, nil, "synthetic listing failure", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, reason := vpcCIDRDeployed(context.Background(), cidrQAEnv(t, tt.fields),
				tt.withAWS, tt.ready, vpcCIDRAWS{}, tt.listed, tt.own, tt.awsError)
			if status != tt.want {
				t.Errorf("deployed = %q, want %q (reason %q)", status, tt.want, reason)
			}
			if strings.TrimSpace(reason) == "" {
				t.Error("deployment reason is empty")
			}
		})
	}
}

// TEST-21; REQ-18/M4, REQ-22/L1, REQ-23/L2, REQ-26/L5.
func TestInspectVPCCIDRCandidate_ValidatesLiveInput(t *testing.T) {
	siblings := []vpcCIDRUse{{Source: `env "staging"`, Name: "staging", File: "staging.yaml", Prefix: netip.MustParsePrefix("10.42.0.0/16")}}
	tests := []struct {
		name, value       string
		incomplete, valid bool
		code, fix         string
		warnings          int
	}{
		{"empty", "", true, false, "", "", 0},
		{"first number", "10", true, false, "", "", 0},
		{"two numbers", "10.42", true, false, "", "", 0},
		{"fewer than four numbers with slash", "10.42/16", true, false, "", "", 0},
		{"no slash yet", "10.42.0.0", true, false, "", "", 0},
		{"complete free range", "10.77.0.0/16", false, true, "", "", 0},
		{"complete overlap is still valid", "10.42.0.0/16", false, true, "", "", 1},
		{"slash typed, size not yet", "10.77.0.0/", true, false, "", "", 0},
		{"complete invalid size text", "10.77.0.0/x", false, false, "invalid_format", "", 0},
		{"complete too small", "10.77.0.0/20", false, false, "size", "", 0},
		{"host bits corrected to nonzero nineteen", "10.5.33.0/19", false, false, "not_network_address", "10.5.32.0/19", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := vpcCIDRInputIncomplete(tt.value); got != tt.incomplete {
				t.Errorf("incomplete(%q) = %v, want %v", tt.value, got, tt.incomplete)
			}
			got := inspectVPCCIDRCandidate(tt.value, siblings)
			if got == nil {
				t.Fatal("candidate result is nil")
			}
			if got.Value != tt.value || got.Incomplete != tt.incomplete || got.Valid != tt.valid {
				t.Errorf("candidate = %+v, want value %q incomplete %v valid %v", got, tt.value, tt.incomplete, tt.valid)
			}
			if tt.code == "" {
				if got.Error != nil {
					t.Errorf("valid/incomplete input has a rule error: %+v", got.Error)
				}
			} else if got.Error == nil || got.Error.Code != tt.code || got.Error.Fix != tt.fix {
				t.Errorf("candidate issue = %+v, want %s fix %q", got.Error, tt.code, tt.fix)
			} else {
				cidrQAAssertPlainHint(t, got.Error.Hint)
			}
			if got.Warnings == nil || len(got.Warnings) != tt.warnings {
				t.Errorf("candidate warnings = %#v, want non-nil length %d", got.Warnings, tt.warnings)
			}
		})
	}
}

// TEST-22; REQ-13/H2, REQ-18/M4, REQ-26/L5.
func TestInspectVPCCIDR_CandidateRemainsOfflineAndValidatesOverride(t *testing.T) {
	tests := []struct {
		candidate string
		code      string
	}{
		{"10.77.0.0/16", ""},
		{"10.77.0.0/20", "size"},
	}
	for _, tt := range tests {
		t.Run(tt.candidate, func(t *testing.T) {
			env := cidrQAEnv(t, map[string]interface{}{
				"account_id": "000000000000", "aws_profile": "fixture-profile",
				"vpc_cidr": "10.77.0.0/20", "allow_vpc_cidr_override": true,
			})
			deps := cidrQADeps(nil)
			deps.aws = cidrQAFakeAWS([]awsVPC{{ID: "vpc-synthetic-candidate", CIDRs: []netip.Prefix{netip.MustParsePrefix("10.77.0.0/16")}}}, nil)
			value := tt.candidate
			report := inspectVPCCIDR(context.Background(), deps, env,
				vpcCIDRInspectOptions{EnvName: "dev", Dir: t.TempDir(), WithAWS: true, Count: 5, Candidate: &value})
			if !report.Override || report.Error != nil {
				t.Errorf("configured override was not reported: %+v", report)
			}
			if report.AWSChecked || report.Deployed != "unchecked" {
				t.Errorf("candidate returned AWS-derived deployment/network data: %+v", report)
			}
			if report.Candidate == nil {
				t.Fatal("candidate field is absent")
			}
			if len(report.Candidate.Warnings) != 0 {
				t.Errorf("AWS-only candidate overlap leaked into local candidate check: %+v", report.Candidate)
			}
			if tt.code != "" {
				if report.Candidate.Error == nil || report.Candidate.Error.Code != tt.code || report.Candidate.Valid {
					t.Errorf("override incorrectly suppressed the typed candidate issue: %+v", report.Candidate)
				}
			} else if !report.Candidate.Valid || report.Candidate.Error != nil {
				t.Errorf("free candidate was not valid: %+v", report.Candidate)
			}
		})
	}
}

// TEST-23; REQ-8/R2, REQ-21/M7.
func TestNewEnvVPCCIDRWith_AvoidsSiblingsAndRefusesExhaustion(t *testing.T) {
	dir := t.TempDir()
	// An independently specified prefix cover leaves only 10.42/16 free.
	blocked := []string{"10.0.0.0/11", "10.32.0.0/13", "10.40.0.0/15", "10.43.0.0/16", "10.44.0.0/14", "10.48.0.0/12", "10.64.0.0/10", "10.128.0.0/9"}
	for i, value := range blocked {
		name := fmt.Sprintf("sibling-%02d", i)
		cidrQAWriteEnv(t, dir, name+".yaml", name, "000000000000",
			"use_default_vpc: false\nvpc_cidr: "+value+"\n")
	}
	deps := cidrQADeps(nil)
	deps.siblings = siblingVPCCIDRUses
	before := cidrQASnapshot(t, dir)
	value, err := newEnvVPCCIDRWith(deps, dir, "new-env")
	if err != nil || value != "10.42.0.0/16" {
		t.Errorf("new range = %q, %v; want exactly 10.42.0.0/16, nil", value, err)
	}
	cidrQAAssertUnchanged(t, dir, before)

	cidrQAWriteEnv(t, dir, "all.yaml", "all", "000000000000",
		"use_default_vpc: false\nvpc_cidr: 10.0.0.0/8\n")
	before = cidrQASnapshot(t, dir)
	value, err = newEnvVPCCIDRWith(deps, dir, "new-env")
	if !errors.Is(err, errVPCCIDRPoolExhausted) || value != "" {
		t.Errorf("exhausted creation = %q, %v; want empty value and pool exhaustion, never a legacy fallback", value, err)
	}
	cidrQAAssertUnchanged(t, dir, before)
}

// TEST-24; REQ-8/R2, REQ-24/L3.
func TestCreateEnvAndLoadEnv_PreserveExplicitVPCCIDR(t *testing.T) {
	t.Run("creation persists the supplied choice", func(t *testing.T) {
		for _, value := range []string{"10.42.0.0/16", "10.77.0.0/16"} {
			env := createEnv("qa-project", "dev", value)
			m := cidrQAEnvMap(t, env)
			if m["vpc_cidr"] != value || m["use_default_vpc"] != false {
				t.Errorf("created YAML fields = %v, want explicit %s and a custom VPC", m, value)
			}
		}
	})
	for _, version := range []int{CurrentSchemaVersion, CurrentSchemaVersion - 1} {
		for _, value := range []string{"10.0.0.0/16", "10.1.0.0/16", "10.99.0.0/16", "192.168.32.0/19", "172.20.0.0/16"} {
			t.Run(fmt.Sprintf("schema-%d-keeps-%s", version, value), func(t *testing.T) {
				dir := t.TempDir()
				path := writeStaleConfig(t, dir, "dev", version)
				// Extend the existing synthetic fixture without changing its helper.
				if err := appendCIDRQASettings(path, "use_default_vpc: false\nvpc_cidr: "+value+"\n"); err != nil {
					t.Fatalf("append synthetic range: %v", err)
				}
				chdir(t, dir)
				env, err := loadEnv("dev")
				if err != nil {
					t.Fatalf("load synthetic env: %v", err)
				}
				if got := cidrQAEnvMap(t, env)["vpc_cidr"]; got != value {
					t.Errorf("load changed explicit range to %v, want %s", got, value)
				}
				if got := cidrQAReadMap(t, path)["vpc_cidr"]; got != value {
					t.Errorf("migration changed range on disk to %v, want %s", got, value)
				}
			})
		}
	}
}
