package main

import (
	"net/netip"
	"os"
	"regexp"
	"strings"
	"testing"
)

// TEST-35; REQ-29.
func TestParseVPCCIDRArgs_AcceptsDocumentedGrammar(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		action, env string
		aws, force  bool
	}{
		{"offline check", []string{"check", "dev"}, "check", "dev", false, false},
		{"offline suggest", []string{"suggest", "staging"}, "suggest", "staging", false, false},
		{"offline assign", []string{"assign", "dev"}, "assign", "dev", false, false},
		{"AWS after env", []string{"check", "dev", "--aws"}, "check", "dev", true, false},
		{"AWS before action", []string{"--aws", "suggest", "dev"}, "suggest", "dev", true, false},
		{"AWS between action and env", []string{"suggest", "--aws", "dev"}, "suggest", "dev", true, false},
		{"force after env", []string{"assign", "dev", "--force"}, "assign", "dev", false, true},
		{"both flags before action", []string{"--force", "--aws", "assign", "dev"}, "assign", "dev", true, true},
		{"mixed flags", []string{"assign", "--force", "dev", "--aws"}, "assign", "dev", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseVPCCIDRArgs(tt.args)
			if err != nil {
				t.Fatalf("parse %v: %v", tt.args, err)
			}
			if got.Action != tt.action || got.EnvName != tt.env || got.AWS != tt.aws || got.Force != tt.force {
				t.Errorf("parse %v = %+v, want action %s env %s aws %v force %v", tt.args, got, tt.action, tt.env, tt.aws, tt.force)
			}
		})
	}
}

// TEST-36; REQ-29.
func TestParseVPCCIDRArgs_RejectsInvalidGrammar(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no args", nil},
		{"flag without action", []string{"--aws"}},
		{"action without env", []string{"check"}},
		{"action and flag without env", []string{"assign", "--force"}},
		{"unknown action", []string{"delete", "dev"}},
		{"too many envs", []string{"suggest", "dev", "prod"}},
		{"unknown flag", []string{"check", "dev", "--unknown"}},
		{"force cannot bypass check", []string{"check", "dev", "--force"}},
		{"force cannot modify suggest", []string{"--force", "suggest", "dev"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := parseVPCCIDRArgs(tt.args); err == nil {
				t.Errorf("parse %v accepted %+v, want an error", tt.args, got)
			}
		})
	}
}

// TEST-37; REQ-5/R1.5, REQ-16/M2, REQ-29.
func TestHandleVPCCIDRCommand_CheckPrintsTipOnlyForOverlap(t *testing.T) {
	tests := []struct {
		name, siblingRange string
		wantTip            bool
	}{
		{"free range has no AWS tip", "10.77.0.0/16", false},
		{"sibling overlap has one AWS tip", "10.42.0.0/16", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inTempDir(t, func() {
				dir, err := os.Getwd()
				if err != nil {
					t.Fatal(err)
				}
				cidrQAWriteEnv(t, dir, "dev.yaml", "yaml-dev", "", "use_default_vpc: false\nvpc_cidr: 10.42.0.0/16\n")
				cidrQAWriteEnv(t, dir, "staging.yaml", "staging", "", "use_default_vpc: false\nvpc_cidr: "+tt.siblingRange+"\n")
				before := cidrQASnapshot(t, dir)
				output, err := cidrQACaptureStdout(t, func() error {
					return handleVPCCIDRCommand([]string{"check", "dev"})
				})
				if err != nil {
					t.Fatalf("offline check: %v; output: %s", err, output)
				}
				if strings.Contains(output, "Tip:") != tt.wantTip {
					t.Errorf("Tip presence = %v, want %v; output: %s", strings.Contains(output, "Tip:"), tt.wantTip, output)
				}
				if tt.wantTip && (!strings.Contains(output, "meroku vpc-cidr check dev --aws") || strings.Count(output, "Tip:") != 1 || !strings.Contains(output, "staging")) {
					t.Errorf("overlap check lacks the single env-specific AWS tip: %s", output)
				}
				if !strings.Contains(output, "10.42.0.0/16") {
					t.Errorf("check did not show the configured range: %s", output)
				}
				cidrQAAssertUnchanged(t, dir, before)
			})
		})
	}
}

// TEST-38; REQ-9/R3, REQ-29.
func TestHandleVPCCIDRCommand_SuggestDoesNotWrite(t *testing.T) {
	inTempDir(t, func() {
		dir, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		path := cidrQAWriteEnv(t, dir, "dev.yaml", "yaml-dev", "", "use_default_vpc: false\nvpc_cidr: 10.99.0.0/16\n")
		cidrQAWriteEnv(t, dir, "staging.yaml", "staging", "", "use_default_vpc: false\nvpc_cidr: 10.42.0.0/16\n")
		before := cidrQASnapshot(t, dir)
		output, err := cidrQACaptureStdout(t, func() error {
			return handleVPCCIDRCommand([]string{"suggest", "dev"})
		})
		if err != nil {
			t.Fatalf("offline suggest: %v; output: %s", err, output)
		}
		pattern := regexp.MustCompile(`10\.[0-9]+\.0\.0/16`)
		suggested := map[string]bool{}
		for _, value := range pattern.FindAllString(output, -1) {
			// A report may also display the configured legacy range and a
			// sibling warning. Neither is necessarily a suggestion.
			if value == "10.99.0.0/16" || value == "10.42.0.0/16" {
				continue
			}
			p, err := netip.ParsePrefix(value)
			if err != nil {
				t.Fatal(err)
			}
			cidrQAAssertCurated(t, p)
			suggested[value] = true
		}
		if len(suggested) == 0 {
			t.Errorf("suggest did not display any curated free range: %s", output)
		}
		if cidrQAReadMap(t, path)["vpc_cidr"] != "10.99.0.0/16" {
			t.Error("suggest changed the explicit current range")
		}
		cidrQAAssertUnchanged(t, dir, before)
	})
}

// TEST-39; REQ-9/R3, REQ-13/H2, REQ-29.
func TestHandleVPCCIDRCommand_AssignRefusesUnchecked(t *testing.T) {
	inTempDir(t, func() {
		dir, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		cidrQAWriteEnv(t, dir, "dev.yaml", "yaml-dev", "000000000000",
			"use_default_vpc: false\naws_profile: fixture-profile\nvpc_cidr: 10.77.0.0/20\n")
		before := cidrQASnapshot(t, dir)
		output, err := cidrQACaptureStdout(t, func() error {
			// No --aws: this test must never create a real AWS client.
			return handleVPCCIDRCommand([]string{"assign", "dev"})
		})
		if err == nil {
			t.Fatalf("unchecked assign succeeded; output: %s", output)
		}
		text := strings.ToLower(output + "\n" + err.Error())
		if !strings.Contains(text, "rerun with --aws") || !strings.Contains(text, "--force") {
			t.Errorf("unchecked CLI refusal lacks rerun/force guidance: %s", text)
		}
		cidrQAAssertUnchanged(t, dir, before)
	})
}
