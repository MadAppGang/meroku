package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A project name long enough that both cascade form 1
// ("<project>-dev-github-actions-role") and form 2
// ("<project>-github-actions-role-dev") exceed the 64-character IAM role limit,
// forcing form 3.
const cascade3Project = "extremely-long-project-nameextremely-long-project-name"

var cascade3RoleName = regexp.MustCompile(`^github-actions-role-[0-9a-f]{8}$`)

func TestOwnGitHubRoleName(t *testing.T) {
	// Form 1: the legacy name fits, so the deployed name comes back verbatim.
	if got, want := ownGitHubRoleName("acme", "dev"), "acme-dev-github-actions-role"; got != want {
		t.Errorf("ownGitHubRoleName(acme, dev) = %q, want %q", got, want)
	}

	// Form 3: the regression test for the naming trap. Neither the project nor
	// the environment survives into the name, so no suffix or prefix filter can
	// recognise this role as ours — only recomputing the cascade can.
	got := ownGitHubRoleName(cascade3Project, "dev")
	if len(got) > 64 {
		t.Fatalf("ownGitHubRoleName(%q, dev) = %q, %d characters, over the 64-character IAM limit",
			cascade3Project, got, len(got))
	}
	if !cascade3RoleName.MatchString(got) {
		t.Fatalf("ownGitHubRoleName(%q, dev) = %q, want github-actions-role-<8 hex>", cascade3Project, got)
	}
	if strings.Contains(got, cascade3Project) || strings.Contains(got, "dev") {
		t.Errorf("cascade form 3 name %q still carries project or env; the test project is not long enough", got)
	}

	// Pinned to awsName itself, so a divergence between this helper and the
	// naming cascade fails here rather than in AWS.
	want := awsName(cascade3Project, "dev", []string{"github", "actions", "role"}, 64, "", "",
		cascade3Project+"-dev-github-actions-role")
	if got != want {
		t.Errorf("ownGitHubRoleName = %q, awsName = %q; the helper no longer reproduces modules/workloads/naming.tf", got, want)
	}
}

// envYAML writes a sibling environment config the way a real one looks: more keys
// than the probe struct models, so a loader that round-tripped the file would
// visibly lose some.
func envYAML(t *testing.T, dir, file, project, env, accountID string) {
	t.Helper()
	body := fmt.Sprintf(`schema_version: %d
project: %s
env: %s
account_id: "%s"
region: us-east-1
workload:
  enable_github_oidc: true
  github_oidc_subjects:
    - "repo:acme/api:*"
`, CurrentSchemaVersion, project, env, accountID)
	if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", file, err)
	}
}

func TestOwnRoleNames(t *testing.T) {
	const account = "000000000000"
	dir := t.TempDir()

	envYAML(t, dir, "dev.yaml", "acme", "dev", account)
	envYAML(t, dir, "prod.yaml", "acme", "prod", account)
	// Same project, a different AWS account: its role does not exist in the
	// account being scanned, so it must not be excluded.
	envYAML(t, dir, "other.yaml", "acme", "other", "111111111111")
	// The filename says staging, the config says stage. Terraform builds the
	// role name from the declared value.
	envYAML(t, dir, "staging.yaml", "acme", "stage", account)
	// A project long enough to force cascade form 3.
	envYAML(t, dir, "big.yaml", cascade3Project, "dev", account)

	// Neither of these is an environment. Both must be skipped without error.
	if err := os.WriteFile(filepath.Join(dir, "dns.yaml"),
		[]byte("root_domain: example.com\nroot_zone_id: Z123\n"), 0o644); err != nil {
		t.Fatalf("write dns.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "taskfile.yaml"),
		[]byte("version: '3'\ntasks:\n  build:\n    cmds:\n      - go build\n"), 0o644); err != nil {
		t.Fatalf("write taskfile.yaml: %v", err)
	}

	chdir(t, dir)

	own, err := ownRoleNames("acme", account)
	if err != nil {
		t.Fatalf("ownRoleNames: %v", err)
	}

	want := []string{
		"acme-dev-github-actions-role",
		"acme-prod-github-actions-role",
		"acme-stage-github-actions-role", // from `env: stage`, not from staging.yaml
	}
	assertSet(t, own, want)

	if own["acme-staging-github-actions-role"] {
		t.Error("excluded a role name built from the filename; ownRoleNames must use the declared env")
	}
	if own["acme-other-github-actions-role"] {
		t.Error("excluded a sibling in a different AWS account")
	}

	// The cascade-form-3 project, whose role name carries no trace of either
	// field ownRoleNames filters on.
	bigOwn, err := ownRoleNames(cascade3Project, account)
	if err != nil {
		t.Fatalf("ownRoleNames(%q): %v", cascade3Project, err)
	}
	assertSet(t, bigOwn, []string{ownGitHubRoleName(cascade3Project, "dev")})
	for name := range bigOwn {
		if !cascade3RoleName.MatchString(name) {
			t.Errorf("expected a cascade form 3 digest name, got %q", name)
		}
	}

	// A project with no environments in this directory excludes nothing, rather
	// than erroring or excluding everything.
	none, err := ownRoleNames("nosuchproject", account)
	if err != nil {
		t.Fatalf("ownRoleNames(nosuchproject): %v", err)
	}
	assertSet(t, none, nil)
}

func assertSet(t *testing.T, got map[string]bool, want []string) {
	t.Helper()
	var have []string
	for name, ok := range got {
		if ok {
			have = append(have, name)
		}
	}
	sort.Strings(have)
	sorted := append([]string(nil), want...)
	sort.Strings(sorted)
	if strings.Join(have, ",") != strings.Join(sorted, ",") {
		t.Errorf("own role names = %v, want %v", have, sorted)
	}
}

// TestOwnRoleNamesDoesNotRewriteSiblings is the regression test for this phase.
//
// loadEnv delegates to loadEnvWithMigration, which for any file below
// CurrentSchemaVersion writes a <file>.backup_<timestamp> and rewrites the
// environment file through a full marshal cycle, dropping every key the Env
// struct does not model. This scan's whole rule is "report and refuse, never
// rewrite", and it touches every sibling environment, not just the one the
// operator asked about.
//
// So anyone who later "simplifies" the raw probe read in readEnvIdentity into a
// loadEnv call fails here.
func TestOwnRoleNamesDoesNotRewriteSiblings(t *testing.T) {
	const account = "000000000000"
	dir := t.TempDir()

	// Well below the current schema version, so a migrating loader would rewrite
	// it. Derived from the constant so it cannot drift as the schema advances.
	oldVersion := CurrentSchemaVersion - 5
	if oldVersion < 1 {
		oldVersion = 1
	}

	// Deliberately full of things a marshal cycle would destroy: a comment, a
	// key the Env struct does not model, and non-canonical formatting.
	stale := []byte(fmt.Sprintf(`# hand-written, and it stays that way
schema_version: %d
project:  acme
env:      dev
account_id: "%s"
region: us-east-1
a_key_the_env_struct_does_not_model: keep me
`, oldVersion, account))

	path := filepath.Join(dir, "dev.yaml")
	if err := os.WriteFile(path, stale, 0o644); err != nil {
		t.Fatalf("write dev.yaml: %v", err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat dev.yaml: %v", err)
	}

	chdir(t, dir)

	own, err := ownRoleNames("acme", account)
	if err != nil {
		t.Fatalf("ownRoleNames: %v", err)
	}
	// The stale file is still readable, so it is still excluded — the point is
	// that reading it cost nothing.
	if !own["acme-dev-github-actions-role"] {
		t.Errorf("stale sibling was not excluded: %v", own)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("re-read dev.yaml: %v", err)
	}
	if string(after) != string(stale) {
		t.Errorf("sibling was rewritten.\n--- before ---\n%s\n--- after ---\n%s", stale, after)
	}

	afterStat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("re-stat dev.yaml: %v", err)
	}
	if !afterStat.ModTime().Equal(before.ModTime()) {
		t.Errorf("sibling ModTime changed: %v -> %v", before.ModTime(), afterStat.ModTime())
	}

	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(d.Name(), ".backup_") || (d.IsDir() && d.Name() == "backup") {
			t.Errorf("a backup was created at %s; something migrated the sibling", p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
}
