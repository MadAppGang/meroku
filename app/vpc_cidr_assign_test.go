package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Append settings only to a synthetic test fixture made by writeStaleConfig.
func appendCIDRQASettings(path, text string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(text); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func cidrQAAssertBackup(t *testing.T, dir, backupPath string) {
	t.Helper()
	if backupPath == "" {
		t.Fatal("assignment wrote YAML without returning a backup path")
	}
	if !filepath.IsAbs(backupPath) {
		backupPath = filepath.Join(dir, backupPath)
	}
	rel, err := filepath.Rel(filepath.Join(dir, "backup"), backupPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("backup path %q is not under the fixture's backup directory: %v", backupPath, err)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("returned backup does not exist: %v", err)
	}
}

// TEST-25; REQ-9/R3, REQ-31.
func TestAssignVPCCIDR_WritesFreeRangeAndBackup(t *testing.T) {
	tests := []struct {
		name, configured string
	}{
		{"unset is explicitly assigned", ""},
		{"invalid value is replaced only on undeployed env", "10.77.0.0/20"},
		{"valid overlapping value is replaced", "10.5.0.0/16"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			extra := "use_default_vpc: false\n"
			if tt.configured != "" {
				extra += "vpc_cidr: " + tt.configured + "\n"
			}
			path := cidrQAWriteEnv(t, dir, "file-dev.yaml", "yaml-runtime-dev", "", extra)
			env := cidrQAEnv(t, cidrQAReadMap(t, path))
			otherDir := t.TempDir()
			chdir(t, otherDir)
			otherBefore := cidrQASnapshot(t, otherDir)

			result, err := assignVPCCIDR(context.Background(), cidrQADeps(cidrQAPoolUsesExcept(42)), env,
				vpcCIDRAssignOptions{EnvName: "file-dev", Dir: dir})
			if err != nil {
				t.Fatalf("assign undeployed synthetic env: %v", err)
			}
			if !result.Changed || result.CIDR != "10.42.0.0/16" || result.Previous != tt.configured {
				t.Errorf("assignment result = %+v, want Changed, exact 10.42/16 and Previous %q", result, tt.configured)
			}
			if result.Report.Deployed != "no" {
				t.Errorf("no-target synthetic env reported deployed %q", result.Report.Deployed)
			}
			m := cidrQAReadMap(t, path)
			if m["vpc_cidr"] != "10.42.0.0/16" || m["env"] != "yaml-runtime-dev" || m["project"] != "qa-project" || m["account_id"] != "" {
				t.Errorf("assignment changed unrelated YAML identity or did not write the choice: %v", m)
			}
			cidrQAAssertBackup(t, dir, result.BackupPath)
			if _, err := os.Stat(filepath.Join(dir, "yaml-runtime-dev.yaml")); !os.IsNotExist(err) {
				t.Errorf("assignment used YAML env as filename instead of the file stem: %v", err)
			}
			cidrQAAssertUnchanged(t, otherDir, otherBefore)
		})
	}
}

// TEST-26; REQ-8/R2, REQ-9/R3, REQ-31.
func TestAssignVPCCIDR_KeepsFreeValuesAndIsIdempotent(t *testing.T) {
	for _, value := range []string{"10.0.0.0/16", "10.99.0.0/16", "192.168.32.0/19", "10.77.32.0/19"} {
		t.Run("free-"+value, func(t *testing.T) {
			dir := t.TempDir()
			path := cidrQAWriteEnv(t, dir, "dev.yaml", "yaml-dev", "000000000000",
				"use_default_vpc: false\naws_profile: fixture-profile\nvpc_cidr: "+value+"\n")
			before := cidrQASnapshot(t, dir)
			result, err := assignVPCCIDR(context.Background(), cidrQADeps(nil), cidrQAEnv(t, cidrQAReadMap(t, path)),
				vpcCIDRAssignOptions{EnvName: "dev", Dir: dir})
			if err != nil || result.Changed || result.CIDR != value || result.BackupPath != "" {
				t.Errorf("free range assignment = %+v, %v; want unchanged %s and no backup", result, err, value)
			}
			cidrQAAssertUnchanged(t, dir, before)
		})
	}
	t.Run("second assignment does not rewrite or add a backup", func(t *testing.T) {
		dir := t.TempDir()
		path := cidrQAWriteEnv(t, dir, "dev.yaml", "yaml-dev", "", "use_default_vpc: false\n")
		deps := cidrQADeps(cidrQAPoolUsesExcept(42))
		first, err := assignVPCCIDR(context.Background(), deps, cidrQAEnv(t, cidrQAReadMap(t, path)),
			vpcCIDRAssignOptions{EnvName: "dev", Dir: dir})
		if err != nil || !first.Changed {
			t.Fatalf("first assignment = %+v, %v; want a successful write", first, err)
		}
		before := cidrQASnapshot(t, dir)
		second, err := assignVPCCIDR(context.Background(), cidrQADeps(cidrQAPoolUsesExcept(42)), cidrQAEnv(t, cidrQAReadMap(t, path)),
			vpcCIDRAssignOptions{EnvName: "dev", Dir: dir})
		if err != nil || second.Changed || second.CIDR != "10.42.0.0/16" || second.BackupPath != "" {
			t.Errorf("second assignment = %+v, %v; want unchanged 10.42/16 without backup", second, err)
		}
		cidrQAAssertUnchanged(t, dir, before)
	})
}

// TEST-27; REQ-9/R3, REQ-13/H2, REQ-31.
func TestAssignVPCCIDR_RefusesUnconfirmedDeployments(t *testing.T) {
	tests := []struct {
		name                  string
		withAWS, listSucceeds bool
		want                  string
	}{
		{"offline configured target", false, false, "unchecked"},
		{"AWS configuration failed", true, false, "unknown"},
		{"AWS listed but state reader missing", true, true, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := cidrQAWriteEnv(t, dir, "dev.yaml", "yaml-dev", "000000000000",
				"use_default_vpc: false\naws_profile: fixture-profile\nvpc_cidr: 10.77.0.0/20\n")
			before := cidrQASnapshot(t, dir)
			deps := cidrQADeps(cidrQAPoolUsesExcept(42))
			if tt.listSucceeds {
				deps.aws = cidrQAFakeAWS(nil, nil)
			}
			result, err := assignVPCCIDR(context.Background(), deps, cidrQAEnv(t, cidrQAReadMap(t, path)),
				vpcCIDRAssignOptions{EnvName: "dev", Dir: dir, WithAWS: tt.withAWS})
			if !errors.Is(err, errVPCCIDRAssignRefused) {
				t.Fatalf("error = %v, want wrapped errVPCCIDRAssignRefused", err)
			}
			if result.CIDR != "10.42.0.0/16" || result.Changed || result.BackupPath != "" || result.Report.Deployed != tt.want {
				t.Errorf("refusal lost suggestion or wrote data: %+v; want suggested 10.42/16, unchanged, %s", result, tt.want)
			}
			if tt.want == "unchecked" {
				text := strings.ToLower(err.Error())
				if !strings.Contains(text, "rerun with --aws") || !strings.Contains(text, "--force") {
					t.Errorf("unchecked refusal lacks explicit rerun/force guidance: %v", err)
				}
			}
			cidrQAAssertUnchanged(t, dir, before)
		})
	}
}

// TEST-28; REQ-9/R3, REQ-13/H2, REQ-31.
func TestAssignVPCCIDR_ForceAllowsUnconfirmedCustomEnv(t *testing.T) {
	for _, withAWS := range []bool{false, true} {
		name := "offline unchecked"
		if withAWS {
			name = "AWS unavailable unknown"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := cidrQAWriteEnv(t, dir, "dev.yaml", "yaml-dev", "000000000000",
				"use_default_vpc: false\naws_profile: fixture-profile\nvpc_cidr: 10.77.0.0/20\n")
			result, err := assignVPCCIDR(context.Background(), cidrQADeps(cidrQAPoolUsesExcept(42)), cidrQAEnv(t, cidrQAReadMap(t, path)),
				vpcCIDRAssignOptions{EnvName: "dev", Dir: dir, WithAWS: withAWS, Force: true})
			if err != nil || !result.Changed || result.CIDR != "10.42.0.0/16" {
				t.Fatalf("forced assignment = %+v, %v; want successful 10.42/16 write", result, err)
			}
			if got := cidrQAReadMap(t, path)["vpc_cidr"]; got != "10.42.0.0/16" {
				t.Errorf("forced range on disk = %v, want 10.42.0.0/16", got)
			}
			cidrQAAssertBackup(t, dir, result.BackupPath)
		})
	}
}

// TEST-29; REQ-8/R2, REQ-9/R3, REQ-31.
func TestAssignVPCCIDR_RefusesDefaultVPCAndExhaustion(t *testing.T) {
	tests := []struct {
		name, extra string
		force       bool
		want        error
	}{
		{"default VPC refuses even force", "use_default_vpc: true\nvpc_cidr: 10.42.0.0/16\n", true, errVPCCIDRAssignRefused},
		{"custom exhausted pool never falls back", "use_default_vpc: false\n", false, errVPCCIDRPoolExhausted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := cidrQAWriteEnv(t, dir, "dev.yaml", "yaml-dev", "", tt.extra)
			before := cidrQASnapshot(t, dir)
			result, err := assignVPCCIDR(context.Background(), cidrQADeps(cidrQAPoolUsesExcept()), cidrQAEnv(t, cidrQAReadMap(t, path)),
				vpcCIDRAssignOptions{EnvName: "dev", Dir: dir, Force: tt.force})
			if !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
			if result.Changed || result.BackupPath != "" {
				t.Errorf("failed assignment claims a write or backup: %+v", result)
			}
			cidrQAAssertUnchanged(t, dir, before)
		})
	}
}
