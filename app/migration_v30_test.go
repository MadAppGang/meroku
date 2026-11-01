package main

import (
	"regexp"
	"strings"
	"testing"
)

// The version constant, the registered migration list and the newest migration
// have to agree, or a file migrates to a version whose migration never ran.
//
// This check belongs to whichever migration is currently newest. When v31 is
// added, move it there and leave a registration-only check behind, the way
// TestV29IsRegistered was left.
func TestV30IsRegisteredAtTheCurrentVersion(t *testing.T) {
	if CurrentSchemaVersion != 30 {
		t.Fatalf("CurrentSchemaVersion = %d, want 30", CurrentSchemaVersion)
	}
	last := AllMigrations[len(AllMigrations)-1]
	if last.Version != CurrentSchemaVersion {
		t.Errorf("last registered migration is v%d, but CurrentSchemaVersion is %d",
			last.Version, CurrentSchemaVersion)
	}
	if last.Apply == nil {
		t.Fatal("the v30 entry has no Apply function")
	}
}

// Every pre-v30 service exposed its port to the internet. The template default
// for an absent value is public, so a service the migration missed would be
// narrowed on its next apply — the change nobody asked for.
func TestMigrateToV30_PinsEveryExistingServiceToFullOpen(t *testing.T) {
	doc := map[string]interface{}{
		"services": []interface{}{
			// The shape yaml.v2 produces from a file on disk.
			map[interface{}]interface{}{"name": "api"},
			// The shape a hand-built or JSON-converted document has.
			map[string]interface{}{"name": "bridge"},
		},
	}

	if err := migrateToV30(doc); err != nil {
		t.Fatalf("migrateToV30: %v", err)
	}

	services := doc["services"].([]interface{})
	if got := services[0].(map[interface{}]interface{})["network_access"]; got != "full_open" {
		t.Errorf("api: network_access = %v, want full_open", got)
	}
	if got := services[1].(map[string]interface{})["network_access"]; got != "full_open" {
		t.Errorf("bridge: network_access = %v, want full_open", got)
	}
}

func TestMigrateToV30_KeepsAValueTheUserAlreadyChose(t *testing.T) {
	doc := map[string]interface{}{
		"services": []interface{}{
			map[interface{}]interface{}{"name": "bridge", "network_access": "internal"},
		},
	}
	if err := migrateToV30(doc); err != nil {
		t.Fatalf("migrateToV30: %v", err)
	}
	if got := doc["services"].([]interface{})[0].(map[interface{}]interface{})["network_access"]; got != "internal" {
		t.Errorf("network_access = %v, want internal (left alone)", got)
	}
}

func TestMigrateToV30_ToleratesMissingAndOddShapes(t *testing.T) {
	for name, doc := range map[string]map[string]interface{}{
		"no services key":   {},
		"null services":     {"services": nil},
		"empty services":    {"services": []interface{}{}},
		"services not list": {"services": "oops"},
		"element not map":   {"services": []interface{}{"oops"}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := migrateToV30(doc); err != nil {
				t.Fatalf("migrateToV30 must skip, not fail: %v", err)
			}
		})
	}
}

// The migrated value has to reach the module call, or the pin is decoration
// and the service gets the Terraform default (public) anyway.
func TestNetworkAccessReachesTheWorkloadsModule(t *testing.T) {
	overlay := map[string]interface{}{
		"services": []interface{}{
			map[interface{}]interface{}{
				"name": "legacy", "container_port": 3000, "host_port": 3000,
				"network_access": "full_open",
			},
			map[interface{}]interface{}{
				"name": "bridge", "container_port": 8080, "host_port": 8080,
				"network_access":       "internal",
				"health_check_command": []interface{}{"CMD-SHELL", "wget -qO- http://localhost:8080/health/live || exit 1"},
			},
		},
	}
	block := workloadsModuleBlock(t, renderMainHBS(t, overlay))

	servicesLine := regexp.MustCompile(`(?m)^\s*services = (.*)$`).FindStringSubmatch(block)
	if servicesLine == nil {
		t.Fatalf("no services argument in the module call:\n%s", block)
	}
	for _, want := range []string{
		`"network_access":"full_open"`,
		`"network_access":"internal"`,
		`"health_check_command":["CMD-SHELL","wget -qO- http://localhost:8080/health/live || exit 1"]`,
	} {
		if !strings.Contains(servicesLine[1], want) {
			t.Errorf("services argument is missing %s:\n%s", want, servicesLine[1])
		}
	}
}
