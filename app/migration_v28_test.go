package main

import "testing"

// v28 has to stay in the registry, or a file that is behind migrates past it
// without it ever running. The current-version check moved to
// TestV29IsRegisteredAtTheCurrentVersion with the v29 bump.
func TestV28IsRegistered(t *testing.T) {
	for _, migration := range AllMigrations {
		if migration.Version == 28 {
			if migration.Apply == nil {
				t.Fatal("the v28 entry has no Apply function")
			}
			return
		}
	}
	t.Fatal("AllMigrations has no entry for version 28")
}

// workloadDoc builds the map shape yaml.v2 produces for a nested block: string
// keys at the top level, interface keys below it.
func workloadDoc(workload map[interface{}]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"project":  "demo",
		"workload": workload,
	}
}

func TestMigrateToV28_EnabledProjectKeepsCreatingTheProvider(t *testing.T) {
	doc := workloadDoc(map[interface{}]interface{}{
		"enable_github_oidc": true,
	})

	if err := migrateToV28(doc); err != nil {
		t.Fatalf("migrateToV28: %v", err)
	}

	workload := doc["workload"].(map[interface{}]interface{})
	got, ok := workload["github_oidc_create_provider"].(bool)
	if !ok {
		t.Fatalf("github_oidc_create_provider = %#v, want a bool", workload["github_oidc_create_provider"])
	}
	if !got {
		t.Error("github_oidc_create_provider = false, want true — every project created the provider before this flag existed, and migrating must not change any plan")
	}
}

func TestMigrateToV28_DisabledProjectGetsNoKey(t *testing.T) {
	// Writing the key here would describe a resource the config never creates,
	// and would add a line about GitHub OIDC to environments that do not use it.
	doc := workloadDoc(map[interface{}]interface{}{
		"enable_github_oidc": false,
	})

	if err := migrateToV28(doc); err != nil {
		t.Fatalf("migrateToV28: %v", err)
	}

	workload := doc["workload"].(map[interface{}]interface{})
	if _, has := workload["github_oidc_create_provider"]; has {
		t.Error("github_oidc_create_provider was added to a config with GitHub OIDC disabled")
	}
}

func TestMigrateToV28_AbsentOIDCKeyCountsAsDisabled(t *testing.T) {
	doc := workloadDoc(map[interface{}]interface{}{"backend_image_port": 8080})

	if err := migrateToV28(doc); err != nil {
		t.Fatalf("migrateToV28: %v", err)
	}

	workload := doc["workload"].(map[interface{}]interface{})
	if _, has := workload["github_oidc_create_provider"]; has {
		t.Error("github_oidc_create_provider was added to a config that never mentions GitHub OIDC")
	}
}

func TestMigrateToV28_IsIdempotent(t *testing.T) {
	// The resolution meroku writes when another project owns the provider is an
	// explicit false. A re-run must not undo it and re-break the deploy.
	doc := workloadDoc(map[interface{}]interface{}{
		"enable_github_oidc":          true,
		"github_oidc_create_provider": false,
	})

	if err := migrateToV28(doc); err != nil {
		t.Fatalf("migrateToV28: %v", err)
	}

	workload := doc["workload"].(map[interface{}]interface{})
	if got := workload["github_oidc_create_provider"]; got != false {
		t.Errorf("github_oidc_create_provider = %v, want the existing false left alone", got)
	}
}

func TestMigrateToV28_NoWorkloadSection(t *testing.T) {
	// A minimal config has no workload block at all. The migration must return
	// cleanly rather than panicking on the type assertion.
	doc := map[string]interface{}{"project": "demo"}

	if err := migrateToV28(doc); err != nil {
		t.Fatalf("migrateToV28: %v", err)
	}

	if _, has := doc["workload"]; has {
		t.Error("migrateToV28 created a workload section that was not there")
	}
}
