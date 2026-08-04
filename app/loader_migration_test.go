package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v2"
)

// Every loader that reads a config must see it at the current schema version.
//
// This is a regression guard for a real defect: migration used to live on the
// caller's side of the boundary, so `meroku deploy` migrated (it called loadEnv
// first) while `meroku generate` did not (it went straight to applyTemplate).
// Generation then rendered Terraform from a config several versions behind --
// successfully, because an old config parses fine and is merely missing keys the
// template substitutes defaults for. Nothing failed and nothing warned.
//
// The test writes a deliberately stale file and asserts the loader leaves it
// current. It fails if anyone reintroduces a read path that skips migration.

func writeStaleConfig(t *testing.T, dir, env string, version int) string {
	t.Helper()

	cfg := map[string]interface{}{
		"schema_version": version,
		"env":            env,
		"project":        "regression",
		"region":         "ap-southeast-2",
		"account_id":     "000000000000",
		"aws_profile":    "none",
		"workload":       map[string]interface{}{"backend_image_port": 8080},
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	path := filepath.Join(dir, env+".yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func schemaVersionOnDisk(t *testing.T, path string) int {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var m map[string]interface{}
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal read back: %v", err)
	}
	return detectSchemaVersion(m)
}

// chdir moves into dir for the duration of the test. The loaders resolve config
// paths relative to the working directory, which is how meroku is actually run.
func chdir(t *testing.T, dir string) {
	t.Helper()

	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

// loadEnvToMap is the loader template rendering uses, and the one that used to
// skip migration.
func TestLoadEnvToMapMigratesAStaleConfig(t *testing.T) {
	dir := t.TempDir()
	path := writeStaleConfig(t, dir, "dev", 20)
	chdir(t, dir)

	if _, err := loadEnvToMap("dev.yaml"); err != nil {
		t.Fatalf("loadEnvToMap: %v", err)
	}

	if got := schemaVersionOnDisk(t, path); got != CurrentSchemaVersion {
		t.Errorf("schema_version on disk = %d, want %d — the template loader read a stale config", got, CurrentSchemaVersion)
	}
}

// The typed loader has always migrated; assert it still does, so the two loaders
// cannot drift apart again in the other direction.
func TestLoadEnvMigratesAStaleConfig(t *testing.T) {
	dir := t.TempDir()
	path := writeStaleConfig(t, dir, "dev", 20)
	chdir(t, dir)

	if _, err := loadEnv("dev"); err != nil {
		t.Fatalf("loadEnv: %v", err)
	}

	if got := schemaVersionOnDisk(t, path); got != CurrentSchemaVersion {
		t.Errorf("schema_version on disk = %d, want %d", got, CurrentSchemaVersion)
	}
}

// A current file must not be rewritten. Migration creates a backup every time it
// runs, so a loader that migrated unconditionally would litter a backup per
// render and churn the file's mtime.
func TestLoadersLeaveACurrentConfigAlone(t *testing.T) {
	dir := t.TempDir()
	path := writeStaleConfig(t, dir, "dev", CurrentSchemaVersion)
	chdir(t, dir)

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}

	if _, err := loadEnvToMap("dev.yaml"); err != nil {
		t.Fatalf("loadEnvToMap: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(before) != string(after) {
		t.Error("a config already at the current version was rewritten")
	}

	if entries, err := os.ReadDir(filepath.Join(dir, "backup")); err == nil && len(entries) > 0 {
		t.Errorf("migration created %d backup(s) for a config that needed no migration", len(entries))
	}
}

// A missing config must not be turned into a migration error. The caller
// produces a far better message about the missing file, and is about to.
func TestMigrateFileIfNeededIgnoresAMissingFile(t *testing.T) {
	if err := migrateFileIfNeeded(filepath.Join(t.TempDir(), "does-not-exist.yaml")); err != nil {
		t.Errorf("migrateFileIfNeeded on a missing file = %v, want nil", err)
	}
}
