package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The bridge reads module.vpc, so it must expose the custom VPC outputs exactly
// when the template renders that module.
func TestBridgeCustomVPCOutputsFollowTemplate(t *testing.T) {
	tests := []struct {
		name          string
		set           bool
		value         interface{}
		wantCustomVPC bool
	}{
		{name: "true", set: true, value: true, wantCustomVPC: false},
		{name: "false", set: true, value: false, wantCustomVPC: true},
		{name: "absent", set: false, wantCustomVPC: true},
		{name: "null", set: true, value: nil, wantCustomVPC: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			m := qaCapabilityEnv()
			delete(m, "use_default_vpc")
			if tc.set {
				m["use_default_vpc"] = tc.value
			}
			qaCapabilityNoError(t, preprocessEnvMap(m))
			if err := os.MkdirAll(filepath.Join("env", "dev"), 0o755); err != nil {
				t.Fatal(err)
			}
			generateBridgeFile("dev", m)
			raw, err := os.ReadFile(filepath.Join("env", "dev", "_bridge.tf"))
			qaCapabilityNoError(t, err)
			rendered := string(raw)
			qaCapabilityParse(t, rendered)
			if got := strings.Contains(rendered, "vpc_cidr = module.vpc.vpc_cidr"); got != tc.wantCustomVPC {
				t.Errorf("bridge has custom VPC outputs = %v, want %v", got, tc.wantCustomVPC)
			}
		})
	}
}
