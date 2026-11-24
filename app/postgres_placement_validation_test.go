package main

import (
	"strings"
	"testing"
)

// postgres.private_subnets puts a database that is not yet created in the VPC's
// private subnets. Those exist only in an own VPC with a NAT egress strategy,
// and a public address there has no route from the internet, so every other
// combination is refused at generate rather than failing part-way through an
// apply.
func TestValidatePostgresPlacementMap(t *testing.T) {
	postgres := func(kv ...interface{}) map[string]interface{} {
		m := map[string]interface{}{"enabled": true}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i].(string)] = kv[i+1]
		}
		return m
	}

	tests := []struct {
		name    string
		envMap  map[string]interface{}
		wantErr bool
		// Every refusal names the key to change.
		wantInErr []string
	}{
		{
			name: "key absent",
			envMap: map[string]interface{}{
				"use_default_vpc": false,
				"egress_strategy": "public_ip",
				"postgres":        postgres(),
			},
		},
		{
			name: "key nil",
			envMap: map[string]interface{}{
				"use_default_vpc": false,
				"postgres":        postgres("private_subnets", nil),
			},
		},
		{
			name: "false",
			envMap: map[string]interface{}{
				"use_default_vpc": true,
				"postgres":        postgres("private_subnets", false, "public_access", true),
			},
		},
		{
			name: "no postgres section",
			envMap: map[string]interface{}{
				"use_default_vpc": true,
			},
		},
		{
			name: "non-bool value",
			envMap: map[string]interface{}{
				"use_default_vpc": false,
				"egress_strategy": "nat_gateway",
				"postgres":        postgres("private_subnets", "yes"),
			},
			wantErr:   true,
			wantInErr: []string{"private_subnets"},
		},
		{
			name: "true with use_default_vpc true",
			envMap: map[string]interface{}{
				"use_default_vpc": true,
				"egress_strategy": "nat_gateway",
				"postgres":        postgres("private_subnets", true, "public_access", false),
			},
			wantErr:   true,
			wantInErr: []string{"private_subnets", "use_default_vpc"},
		},
		{
			name: "true with egress_strategy absent",
			envMap: map[string]interface{}{
				"use_default_vpc": false,
				"postgres":        postgres("private_subnets", true, "public_access", false),
			},
			wantErr:   true,
			wantInErr: []string{"private_subnets", "egress_strategy"},
		},
		{
			name: "true with egress_strategy public_ip",
			envMap: map[string]interface{}{
				"use_default_vpc": false,
				"egress_strategy": "public_ip",
				"postgres":        postgres("private_subnets", true, "public_access", false),
			},
			wantErr:   true,
			wantInErr: []string{"private_subnets", "egress_strategy"},
		},
		{
			name: "true with public_access true",
			envMap: map[string]interface{}{
				"use_default_vpc": false,
				"egress_strategy": "nat_gateway",
				"postgres":        postgres("private_subnets", true, "public_access", true),
			},
			wantErr:   true,
			wantInErr: []string{"private_subnets", "public_access"},
		},
		{
			name: "true with nat_gateway",
			envMap: map[string]interface{}{
				"use_default_vpc": false,
				"egress_strategy": "nat_gateway",
				"postgres":        postgres("private_subnets", true, "public_access", false),
			},
		},
		{
			name: "true with nat_gateway_ha",
			envMap: map[string]interface{}{
				"use_default_vpc": false,
				"egress_strategy": "nat_gateway_ha",
				"postgres":        postgres("private_subnets", true, "public_access", false),
			},
		},
		{
			name: "true with nat_gateway and public_access absent",
			envMap: map[string]interface{}{
				"use_default_vpc": false,
				"egress_strategy": "nat_gateway",
				"postgres":        postgres("private_subnets", true),
			},
		},
		{
			name: "true but postgres disabled is ignored",
			envMap: map[string]interface{}{
				"use_default_vpc": true,
				"egress_strategy": "public_ip",
				"postgres": map[string]interface{}{
					"enabled":         false,
					"private_subnets": true,
					"public_access":   true,
				},
			},
		},
		{
			name: `quoted use_default_vpc "true"`,
			envMap: map[string]interface{}{
				"use_default_vpc": "true",
				"egress_strategy": "nat_gateway",
				"postgres":        postgres("private_subnets", true, "public_access", false),
			},
			wantErr:   true,
			wantInErr: []string{"private_subnets", "use_default_vpc"},
		},
		{
			name: `quoted public_access "true"`,
			envMap: map[string]interface{}{
				"use_default_vpc": false,
				"egress_strategy": "nat_gateway",
				"postgres":        postgres("private_subnets", true, "public_access", "true"),
			},
			wantErr:   true,
			wantInErr: []string{"private_subnets", "public_access"},
		},
		{
			name: `quoted use_default_vpc "True"`,
			envMap: map[string]interface{}{
				"use_default_vpc": "True",
				"egress_strategy": "nat_gateway",
				"postgres":        postgres("private_subnets", true, "public_access", false),
			},
			wantErr:   true,
			wantInErr: []string{"private_subnets", "use_default_vpc"},
		},
		{
			name: `quoted public_access "TRUE"`,
			envMap: map[string]interface{}{
				"use_default_vpc": false,
				"egress_strategy": "nat_gateway",
				"postgres":        postgres("private_subnets", true, "public_access", "TRUE"),
			},
			wantErr:   true,
			wantInErr: []string{"private_subnets", "public_access"},
		},
		{
			name: `quoted "false" for use_default_vpc and public_access`,
			envMap: map[string]interface{}{
				"use_default_vpc": "false",
				"egress_strategy": "nat_gateway",
				"postgres":        postgres("private_subnets", true, "public_access", "false"),
			},
		},
		{
			name: "YAML-decoded nested map is read the same way",
			envMap: map[string]interface{}{
				"use_default_vpc": true,
				"egress_strategy": "nat_gateway",
				"postgres": map[interface{}]interface{}{
					"enabled":         true,
					"private_subnets": true,
				},
			},
			wantErr:   true,
			wantInErr: []string{"use_default_vpc"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePostgresPlacementMap(tc.envMap)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			for _, want := range tc.wantInErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error should mention %q, got: %v", want, err)
				}
			}
		})
	}
}
