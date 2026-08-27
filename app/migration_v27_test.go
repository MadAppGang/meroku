package main

import (
	"strings"
	"testing"
)

// v27 has to stay in the registry, or a file that is behind migrates past it
// without it ever running.
//
// This used to also assert CurrentSchemaVersion == 27. That assertion belongs
// to whichever version is current, not to v27, so it now lives in
// TestV28IsRegisteredAtTheCurrentVersion and moves again with the next bump.
func TestV27IsRegistered(t *testing.T) {
	for _, migration := range AllMigrations {
		if migration.Version == 27 {
			if migration.Apply == nil {
				t.Fatal("the v27 entry has no Apply function")
			}
			return
		}
	}
	t.Fatal("AllMigrations has no entry for version 27")
}

func TestMigrateToV27_DefaultsToPublicIP(t *testing.T) {
	doc := map[string]interface{}{"project": "demo"}

	if err := migrateToV27(doc); err != nil {
		t.Fatalf("migrateToV27: %v", err)
	}

	got, ok := doc["egress_strategy"].(string)
	if !ok {
		t.Fatalf("egress_strategy = %#v, want a string", doc["egress_strategy"])
	}
	if got != string(EgressPublicIP) {
		t.Errorf("egress_strategy = %q, want %q — existing environments must keep today's behaviour",
			got, EgressPublicIP)
	}
}

func TestMigrateToV27_IsIdempotent(t *testing.T) {
	// A re-run, or a file someone already set by hand, must not be overwritten.
	doc := map[string]interface{}{"egress_strategy": string(EgressNATGatewayHA)}

	if err := migrateToV27(doc); err != nil {
		t.Fatalf("migrateToV27: %v", err)
	}

	if got := doc["egress_strategy"]; got != string(EgressNATGatewayHA) {
		t.Errorf("egress_strategy = %v, want the existing value left alone", got)
	}
}

func TestMigrateToV27_DefaultVPCStillGetsPublicIP(t *testing.T) {
	doc := map[string]interface{}{"use_default_vpc": true}

	if err := migrateToV27(doc); err != nil {
		t.Fatalf("migrateToV27: %v", err)
	}

	if got := doc["egress_strategy"]; got != string(EgressPublicIP) {
		t.Errorf("egress_strategy = %v, want public_ip (the default VPC has no private subnets)", got)
	}
}

func TestValidateEgressStrategy(t *testing.T) {
	tests := []struct {
		name    string
		env     *Env
		wantErr bool
	}{
		{"nil env", nil, false},
		{"empty means public_ip", &Env{}, false},
		{"public_ip", &Env{EgressStrategy: "public_ip"}, false},
		{"nat_gateway", &Env{EgressStrategy: "nat_gateway"}, false},
		{"nat_gateway_ha", &Env{EgressStrategy: "nat_gateway_ha"}, false},
		{"unknown value", &Env{EgressStrategy: "magic"}, true},
		{"typo", &Env{EgressStrategy: "nat-gateway"}, true},
		{"NAT on default VPC", &Env{EgressStrategy: "nat_gateway", UseDefaultVPC: true}, true},
		{"public_ip on default VPC is fine", &Env{EgressStrategy: "public_ip", UseDefaultVPC: true}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateEgressStrategy(tc.env)
			if tc.wantErr && err == nil {
				t.Error("expected an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestEffectiveEgressStrategy(t *testing.T) {
	if got := EffectiveEgressStrategy(nil); got != EgressPublicIP {
		t.Errorf("nil env = %q, want %q", got, EgressPublicIP)
	}
	if got := EffectiveEgressStrategy(&Env{}); got != EgressPublicIP {
		t.Errorf("unset = %q, want %q (pre-v27 files have no field)", got, EgressPublicIP)
	}
	if got := EffectiveEgressStrategy(&Env{EgressStrategy: "nat_gateway"}); got != EgressNATGateway {
		t.Errorf("nat_gateway = %q, want %q", got, EgressNATGateway)
	}
}

func TestAdviseEgress_AlreadyOnNATIsNotToldToSwitch(t *testing.T) {
	var services []Service
	for i := 0; i < 9; i++ {
		services = append(services, Service{Name: string(rune('a' + i))})
	}
	env := &Env{EgressStrategy: "nat_gateway", Services: services}
	env.Workload.BackendDesiredCount = 1

	advice := AdviseEgress(env) // 10 services, non-prod, already on NAT

	if advice.ShouldSwitch {
		t.Errorf("already on the recommended strategy, should not advise a switch: %q", advice.Summary)
	}
	if advice.Current != EgressNATGateway {
		t.Errorf("Current = %q, want %q", advice.Current, EgressNATGateway)
	}
}

func TestAdviseEgress_DefaultVPCNeverRecommendsNAT(t *testing.T) {
	// Twenty services on the default VPC: a NAT would be cheaper, but there is
	// nowhere to put it, so advising one would be useless.
	var services []Service
	for i := 0; i < 19; i++ {
		services = append(services, Service{Name: string(rune('a' + i%26))})
	}
	env := &Env{UseDefaultVPC: true, Services: services}
	env.Workload.BackendDesiredCount = 1

	advice := AdviseEgress(env)

	if advice.Recommended != EgressPublicIP {
		t.Errorf("Recommended = %q, want %q on the default VPC", advice.Recommended, EgressPublicIP)
	}
	if advice.ShouldSwitch {
		t.Error("should not advise a switch that cannot be performed")
	}
	if !strings.Contains(advice.Summary, "use_default_vpc") {
		t.Errorf("summary should name the blocker, got %q", advice.Summary)
	}
}
