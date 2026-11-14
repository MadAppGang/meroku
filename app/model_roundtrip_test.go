package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v2"
)

// envRoundTripFixture is a full-feature environment config. Every YAML key in it
// must be claimed by a field on Env or one of the structs it contains, because
// the test below loads it and saves it again through the same pair of functions
// the TUI, the web API and the selectors use.
//
// A key no struct declares is dropped by loadEnv and never comes back out of
// saveEnvToFile, so the saved file differs from the fixture and the test fails
// naming the missing key. That is the whole point of the fixture: it is the only
// thing that turns "someone added a YAML key and forgot the struct field" from a
// silent data loss on the user's next save into a red test on the commit that
// introduced it. A feature commit adds its keys here and its fields to the model
// in the same commit.
const (
	envRoundTripFixture     = "testdata/env_roundtrip.yaml"
	envRoundTripFixtureBase = "testdata/env_roundtrip"
)

// loadRoundTripFixture returns the fixture's bytes and the Env they load into.
//
// The schema version has to be checked on the bytes, before loadEnv sees the
// file. loadEnvWithMigration backs a stale config up under testdata/backup/,
// applies the migration chain and writes the result over the path it read from,
// so the version it hands back is CurrentSchemaVersion exactly when the fixture
// was behind. Checking the loaded struct therefore cannot catch the case the
// check exists for, and by the time it could, a test run has already rewritten a
// committed fixture and left a backup directory beside it. A stale fixture also
// makes the byte comparison report migration formatting instead of a dropped key.
func loadRoundTripFixture(t *testing.T) ([]byte, Env) {
	t.Helper()

	fixture, err := os.ReadFile(envRoundTripFixture)
	if err != nil {
		t.Fatalf("reading %s: %v", envRoundTripFixture, err)
	}

	var declared struct {
		SchemaVersion int `yaml:"schema_version"`
	}
	if err := yaml.Unmarshal(fixture, &declared); err != nil {
		t.Fatalf("parsing %s: %v", envRoundTripFixture, err)
	}
	if declared.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("%s declares schema_version %d while CurrentSchemaVersion is v%d: a load would migrate the fixture and rewrite it. Bump schema_version in the fixture and add whatever the newer migrations expect.",
			envRoundTripFixture, declared.SchemaVersion, CurrentSchemaVersion)
	}

	loaded, err := loadEnv(envRoundTripFixtureBase)
	if err != nil {
		t.Fatalf("loading %s: %v", envRoundTripFixture, err)
	}
	return fixture, loaded
}

func TestEnvRoundTrip_SavedFileMatchesFixtureByteForByte(t *testing.T) {
	fixture, loaded := loadRoundTripFixture(t)

	savedPath := filepath.Join(t.TempDir(), loaded.Env+".yaml")
	if err := saveEnvToFile(loaded, savedPath); err != nil {
		t.Fatalf("saving the loaded environment: %v", err)
	}
	saved, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatalf("reading the saved file: %v", err)
	}

	if !bytes.Equal(fixture, saved) {
		t.Errorf("saveEnvToFile did not reproduce %s byte for byte.\n\nEach difference is either a YAML key no struct field claims -- dropped on load, so gone from the saved file -- or fixture formatting that yaml.Marshal does not produce. Reconcile the formatting in the fixture; never by deleting a feature key from it.\n\n--- fixture ---\n%s\n--- saved ---\n%s",
			envRoundTripFixture, fixture, saved)
	}
}

// TestEnvRoundTrip_FixtureCoversTheGuardedFields keeps the test above honest. A
// byte-exact fixed point is also satisfied by a smaller fixture, so without this
// the protection could be undone by editing the fixture instead of the model:
// drop a key from it and the round trip goes green again while the struct field
// silently rots. These are the assertions that fail in that case.
func TestEnvRoundTrip_FixtureCoversTheGuardedFields(t *testing.T) {
	_, loaded := loadRoundTripFixture(t)

	t.Run("workload policy", func(t *testing.T) {
		policy := loaded.Workload.Policy
		if len(policy) != 2 {
			t.Fatalf("want 2 statements under workload.policy, got %d", len(policy))
		}
		if policy[0].Effect != "Allow" || len(policy[0].Actions) != 1 || len(policy[0].Resources) != 1 {
			t.Errorf("the Allow statement did not load whole: %+v", policy[0])
		}
		if policy[1].Effect != "Deny" {
			t.Errorf("the Deny statement lost its effect: %+v", policy[1])
		}
		if len(policy[1].Conditions) != 1 {
			t.Fatalf("want 1 condition on the Deny statement, got %d", len(policy[1].Conditions))
		}
		condition := policy[1].Conditions[0]
		if condition.Test == "" || condition.Variable == "" || len(condition.Values) != 1 {
			t.Errorf("the condition did not load whole: %+v", condition)
		}
	})

	t.Run("service policy", func(t *testing.T) {
		if len(loaded.Services) != 1 {
			t.Fatalf("want 1 service in the fixture, got %d", len(loaded.Services))
		}
		if len(loaded.Services[0].Policy) != 1 {
			t.Errorf("services[0].policy did not load: %+v", loaded.Services[0].Policy)
		}
	})

	t.Run("scheduled task policy and environment_variables", func(t *testing.T) {
		if len(loaded.ScheduledTasks) != 1 {
			t.Fatalf("want 1 scheduled task in the fixture, got %d", len(loaded.ScheduledTasks))
		}
		task := loaded.ScheduledTasks[0]
		if len(task.Policy) != 1 {
			t.Errorf("scheduled_tasks[0].policy did not load: %+v", task.Policy)
		}
		if len(task.EnvironmentVariables) != 1 {
			t.Errorf("scheduled_tasks[0].environment_variables did not load: %+v", task.EnvironmentVariables)
		}
	})

	t.Run("event processor task policy and environment_variables", func(t *testing.T) {
		if len(loaded.EventProcessorTasks) != 1 {
			t.Fatalf("want 1 event processor task in the fixture, got %d", len(loaded.EventProcessorTasks))
		}
		task := loaded.EventProcessorTasks[0]
		if len(task.Policy) != 1 {
			t.Fatalf("event_processor_tasks[0].policy did not load: %+v", task.Policy)
		}
		if len(task.Policy[0].Conditions) != 1 {
			t.Errorf("event_processor_tasks[0].policy[0].conditions did not load: %+v", task.Policy[0])
		}
		if len(task.EnvironmentVariables) != 1 {
			t.Errorf("event_processor_tasks[0].environment_variables did not load: %+v", task.EnvironmentVariables)
		}
	})

	t.Run("efs", func(t *testing.T) {
		if len(loaded.EFS) != 2 {
			t.Fatalf("want 2 filesystems under efs, got %d", len(loaded.EFS))
		}
		// The minimal entry has to stay minimal: a nil uid is what lets
		// modules/efs apply its own default of 1000, so a struct that cannot
		// tell absent from 0 turns this into an ownership change on save.
		if minimal := loaded.EFS[0]; minimal.Name != "shared" || minimal.UID != nil || minimal.GID != nil ||
			minimal.Permissions != "" || minimal.Path != "" {
			t.Errorf("the filesystem that sets only a name loaded extra values: %+v", minimal)
		}
		full := loaded.EFS[1]
		if full.Name != "uploads" || full.Permissions != "770" || full.Path != "/uploads" {
			t.Errorf("the fully specified filesystem did not load whole: %+v", full)
		}
		// uid: 0 and gid: 0 mean root, a value that has to survive as 0 rather
		// than be dropped and replaced by the module's default of 1000.
		if full.UID == nil {
			t.Error("efs[1].uid is 0 in the fixture and loaded as absent, so the module would apply its default of 1000")
		} else if *full.UID != 0 {
			t.Errorf("efs[1].uid is 0 in the fixture, got %d", *full.UID)
		}
		if full.GID == nil {
			t.Error("efs[1].gid is 0 in the fixture and loaded as absent, so the module would apply its default of 1000")
		} else if *full.GID != 0 {
			t.Errorf("efs[1].gid is 0 in the fixture, got %d", *full.GID)
		}

		if len(loaded.Workload.EFS) != 1 {
			t.Fatalf("want 1 mount under workload.efs, got %d", len(loaded.Workload.EFS))
		}
		mount := loaded.Workload.EFS[0]
		if mount.Name != "uploads" || mount.MountPoint != "/data" {
			t.Errorf("workload.efs[0] did not load whole: %+v", mount)
		}
		// true, not the module's default of false: a mount that asked to be
		// read-only has to still be read-only after a load.
		if mount.ReadOnly == nil {
			t.Error("workload.efs[0].read_only is true in the fixture and loaded as absent, so the container would mount the filesystem read-write")
		} else if !*mount.ReadOnly {
			t.Error("workload.efs[0].read_only is true in the fixture, got false")
		}
	})

	t.Run("backend_remote_access", func(t *testing.T) {
		// A pointer because the template defaults an absent key to true, so an
		// explicit false has to survive a save as false rather than become
		// absent and re-enable itself.
		if loaded.Workload.BackendRemoteAccess == nil {
			t.Fatal("workload.backend_remote_access did not load")
		}
		if *loaded.Workload.BackendRemoteAccess {
			t.Error("the fixture sets workload.backend_remote_access to false; true here means the value did not survive")
		}
	})

	t.Run("backend autoscaling targets", func(t *testing.T) {
		// 55 and 65, not the template's 70 and 80: a fixture that repeated the
		// defaults could not tell a preserved value from a reset one.
		cpu := loaded.Workload.BackendAutoscalingTargetCPU
		if cpu == nil {
			t.Error("workload.backend_autoscaling_target_cpu did not load")
		} else if *cpu != 55 {
			t.Errorf("workload.backend_autoscaling_target_cpu is 55 in the fixture, got %d", *cpu)
		}
		memory := loaded.Workload.BackendAutoscalingTargetMemory
		if memory == nil {
			t.Error("workload.backend_autoscaling_target_memory did not load")
		} else if *memory != 65 {
			t.Errorf("workload.backend_autoscaling_target_memory is 65 in the fixture, got %d", *memory)
		}
	})
}
