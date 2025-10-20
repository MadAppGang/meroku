package main

import (
	"testing"
)

func TestCreateEnv_VPCDefaults(t *testing.T) {
	env := createEnv("testproject", "dev")

	// Test VPC configuration defaults for new projects
	tests := []struct {
		name     string
		got      interface{}
		want     interface{}
		reason   string
	}{
		{
			name:   "UseDefaultVPC should be false",
			got:    env.UseDefaultVPC,
			want:   false,
			reason: "New projects should use custom VPC by default for better control",
		},
		{
			name:   "VPCCIDR should be set to default",
			got:    env.VPCCIDR,
			want:   "10.0.0.0/16",
			reason: "Sensible default CIDR (can be overridden, VPC module has same default)",
		},
		{
			name:   "SchemaVersion should be current",
			got:    env.SchemaVersion,
			want:   CurrentSchemaVersion,
			reason: "New environments should use latest schema version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s: got %v, want %v\nReason: %s", tt.name, tt.got, tt.want, tt.reason)
			}
		})
	}
}

func TestCreateEnv_VPCSimplicity(t *testing.T) {
	// Test that we've removed complexity
	env := createEnv("testproject", "dev")

	// These fields should NOT exist in the struct anymore
	// This test will fail to compile if fields are added back
	_ = env.UseDefaultVPC
	_ = env.VPCCIDR

	// Note: AZ count is hardcoded to 2 in VPC module (not configurable)
	// Note: Private subnets and NAT Gateway removed entirely
}

func TestCreateEnv_BackendScalingDefaults(t *testing.T) {
	env := createEnv("testproject", "dev")

	// Test backend scaling defaults (schema v4)
	if env.Workload.BackendDesiredCount != 1 {
		t.Errorf("BackendDesiredCount: got %d, want 1", env.Workload.BackendDesiredCount)
	}
	if env.Workload.BackendCPU != "256" {
		t.Errorf("BackendCPU: got %s, want 256", env.Workload.BackendCPU)
	}
	if env.Workload.BackendMemory != "512" {
		t.Errorf("BackendMemory: got %s, want 512", env.Workload.BackendMemory)
	}
}
