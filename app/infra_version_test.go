package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// withBinaryVersion sets the ldflags version for one test.
func withBinaryVersion(t *testing.T, v string) {
	t.Helper()
	old := version
	version = v
	t.Cleanup(func() { version = old })
}

// projectWithInfra makes a project root whose infrastructure/version.txt holds
// infraVersion, and chdirs into it.
func projectWithInfra(t *testing.T, infraVersion string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, infraDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, infraDir, "version.txt"), []byte(infraVersion+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, root)
	return root
}

// The case that shipped: meroku v4.10.0 against v4.9.0 modules rendered
// network_access, the module dropped it, and the plan said "No changes".
func TestCheckInfrastructureVersion_RefusesOlderModules(t *testing.T) {
	withBinaryVersion(t, "v4.10.0")
	projectWithInfra(t, "v4.9.0")
	t.Setenv(allowInfraMismatchEnv, "")

	err := checkInfrastructureVersion()
	if !errors.Is(err, errInfraOutdated) {
		t.Fatalf("want errInfraOutdated, got %v", err)
	}
	for _, want := range []string{"v4.9.0", "v4.10.0", "meroku update", allowInfraMismatchEnv} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not mention %q:\n%s", want, err)
		}
	}
}

// Semver, not string order: "v4.10.0" < "v4.9.0" as strings.
func TestCheckInfrastructureVersion_ComparesAsSemver(t *testing.T) {
	withBinaryVersion(t, "v4.9.0")
	projectWithInfra(t, "v4.10.0")
	if err := checkInfrastructureVersion(); err != nil {
		t.Fatalf("newer modules must only warn, got %v", err)
	}
}

func TestCheckInfrastructureVersion_PassesWhenEqualDevOrOverridden(t *testing.T) {
	t.Run("equal", func(t *testing.T) {
		withBinaryVersion(t, "4.10.2") // ldflags may omit the v
		projectWithInfra(t, "v4.10.2")
		if err := checkInfrastructureVersion(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("dev build", func(t *testing.T) {
		withBinaryVersion(t, "dev")
		projectWithInfra(t, "v1.0.0")
		if err := checkInfrastructureVersion(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("override", func(t *testing.T) {
		withBinaryVersion(t, "v4.10.0")
		projectWithInfra(t, "v4.9.0")
		t.Setenv(allowInfraMismatchEnv, "1")
		if err := checkInfrastructureVersion(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("no infrastructure", func(t *testing.T) {
		withBinaryVersion(t, "v4.10.0")
		chdir(t, t.TempDir())
		if err := checkInfrastructureVersion(); err != nil {
			t.Fatalf("a missing infrastructure/ is reported by the template read, not here: %v", err)
		}
	})
}

// The refusal sits in front of generation, so `meroku generate` stops before
// writing a main.tf the modules would half-ignore.
func TestApplyTemplate_StopsOnOutdatedInfrastructure(t *testing.T) {
	withBinaryVersion(t, "v4.10.0")
	projectWithInfra(t, "v4.9.0")
	t.Setenv(allowInfraMismatchEnv, "")

	if err := applyTemplate("dev"); !errors.Is(err, errInfraOutdated) {
		t.Fatalf("applyTemplate must refuse outdated infrastructure/, got %v", err)
	}
}

// localInfraRepo makes a git repo with a tag holding version.txt, and points
// infrastructureCloneURL at it.
func localInfraRepo(t *testing.T, tag string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		// Isolated from the developer's git config: a global tag.gpgSign or
		// commit.gpgSign makes `git tag` / `git commit` fail here.
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "version.txt"), []byte(tag+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "release")
	run("tag", tag)

	old := infrastructureCloneURL
	infrastructureCloneURL = repo
	t.Cleanup(func() { infrastructureCloneURL = old })
}

func TestUpdate_InstallsTheTagMatchingTheBinary(t *testing.T) {
	localInfraRepo(t, "v4.10.2")
	withBinaryVersion(t, "4.10.2")
	root := projectWithInfra(t, "v4.9.0")

	if err := handleUpdateCommand(nil); err != nil {
		t.Fatalf("update: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, infraDir, "version.txt"))
	if err != nil || strings.TrimSpace(string(data)) != "v4.10.2" {
		t.Fatalf("infrastructure/version.txt = %q (%v), want v4.10.2", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, infraDir, ".git")); !os.IsNotExist(err) {
		t.Error("the installed copy must have no .git")
	}
	leftovers, _ := filepath.Glob(filepath.Join(root, infraDir+".*"))
	if len(leftovers) > 0 {
		t.Errorf("staging or backup directories left behind: %v", leftovers)
	}
}

// A failed clone must leave the old modules in place. The old initProject
// deleted them first, so a typo in a ref left the project with nothing.
func TestUpdate_FailedCloneKeepsTheOldCopy(t *testing.T) {
	localInfraRepo(t, "v4.10.2")
	root := projectWithInfra(t, "v4.9.0")

	if err := handleUpdateCommand([]string{"v9.9.9-does-not-exist"}); err == nil {
		t.Fatal("a ref that does not exist must fail")
	}
	data, err := os.ReadFile(filepath.Join(root, infraDir, "version.txt"))
	if err != nil || strings.TrimSpace(string(data)) != "v4.9.0" {
		t.Fatalf("the old infrastructure/ must survive a failed clone; version.txt = %q (%v)", data, err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(root, infraDir+".*"))
	if len(leftovers) > 0 {
		t.Errorf("staging directory left behind: %v", leftovers)
	}
}

func TestUpdate_RefusesASymlinkedCheckout(t *testing.T) {
	localInfraRepo(t, "v4.10.2")
	checkout := t.TempDir()
	root := t.TempDir()
	if err := os.Symlink(checkout, filepath.Join(root, infraDir)); err != nil {
		t.Fatal(err)
	}
	chdir(t, root)

	err := handleUpdateCommand([]string{"v4.10.2"})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("a symlinked infrastructure/ must be refused, got %v", err)
	}
	if info, _ := os.Lstat(filepath.Join(root, infraDir)); info == nil || info.Mode()&os.ModeSymlink == 0 {
		t.Error("the symlink must be left as it was")
	}
}

func TestUpdate_DevBuildNeedsAnExplicitRef(t *testing.T) {
	withBinaryVersion(t, "dev")
	chdir(t, t.TempDir())
	if err := handleUpdateCommand(nil); err == nil {
		t.Fatal("a dev build has no release tag to default to")
	}
}
