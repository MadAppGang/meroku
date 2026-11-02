package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/blang/semver/v4"
)

// infraDir is the project's copy of meroku's Terraform modules and templates.
const infraDir = "infrastructure"

// allowInfraMismatchEnv lets generation run against an older infrastructure/
// on purpose — bisecting a regression, say. It is an environment variable and
// not a flag so it cannot end up in a script by accident of tab completion.
const allowInfraMismatchEnv = "MEROKU_ALLOW_INFRA_MISMATCH"

// errInfraOutdated marks the one mismatch that silently breaks a deploy.
var errInfraOutdated = errors.New("infrastructure/ is older than meroku")

// checkInfrastructureVersion compares the binary's version with
// infrastructure/version.txt before anything is generated.
//
// The binary and infrastructure/ are versioned together but installed apart:
// brew upgrades the binary, and infrastructure/ changes only when the project
// runs an update. A newer binary migrates the YAML and renders main.tf with
// fields that an older module does not declare, and Terraform's object
// conversion drops undeclared fields without a word. The plan then reports "No
// changes" for a setting the user just made — which is how network_access
// (v4.10.0) first reached a project still on v4.9.0 modules. So an older
// infrastructure/ is refused, not warned about.
//
// A NEWER infrastructure/ than the binary is only a warning: the modules give
// every new field a default, so nothing is dropped, but the binary does not
// know the newest migrations and fields.
//
// A development build (version "dev") has no version to compare and is not
// checked.
func checkInfrastructureVersion() error {
	if version == "dev" {
		return nil
	}

	data, err := os.ReadFile(filepath.Join(infraDir, "version.txt"))
	if err != nil {
		// The template read that follows reports a missing infrastructure/
		// with a better message than a version check could.
		return nil
	}
	infraRaw := strings.TrimSpace(string(data))

	binVer, err := semver.ParseTolerant(version)
	if err != nil {
		return nil
	}
	infraVer, err := semver.ParseTolerant(infraRaw)
	if err != nil {
		fmt.Printf("⚠️  %s/version.txt holds %q, which is not a version; cannot check it against meroku %s\n", infraDir, infraRaw, version)
		return nil
	}

	switch {
	case infraVer.LT(binVer):
		if os.Getenv(allowInfraMismatchEnv) == "1" {
			fmt.Printf("⚠️  %s/ is %s, older than meroku %s. Generating anyway because %s=1.\n", infraDir, infraRaw, version, allowInfraMismatchEnv)
			fmt.Println("   Settings the older modules do not know are dropped silently.")
			return nil
		}
		return fmt.Errorf("%w: %s/ is %s, meroku is %s.\n\n"+
			"The older modules do not know every setting this meroku writes, and Terraform\n"+
			"drops unknown settings silently: the plan would show no change for them.\n\n"+
			"Update it to match this binary:\n\n"+
			"    meroku update-infra\n\n"+
			"(or set %s=1 to generate against the old modules on purpose)",
			errInfraOutdated, infraDir, infraRaw, version, allowInfraMismatchEnv)
	case infraVer.GT(binVer):
		fmt.Printf("⚠️  %s/ is %s, newer than meroku %s. Upgrade meroku (brew upgrade meroku) to use its newest settings and migrations.\n", infraDir, infraRaw, version)
	}
	return nil
}

// handleUpdateInfraCommand is `meroku update-infra [ref]`. Non-interactive, for
// scripts, CI and agents, which the TUI's "Check for updates" cannot serve.
//
// With no ref it installs the release matching this binary (tag v<version>), so
// the binary and the modules agree by construction. A ref may be any tag or
// branch, e.g. `main`.
func handleUpdateInfraCommand(args []string) error {
	ref := ""
	if len(args) > 0 {
		ref = args[0]
	}
	if ref == "" {
		if version == "dev" {
			return fmt.Errorf("this is a development build with no release tag; name one: meroku update-infra <tag|branch>")
		}
		ref = "v" + strings.TrimPrefix(version, "v")
	}

	before := "none"
	if data, err := os.ReadFile(filepath.Join(infraDir, "version.txt")); err == nil {
		before = strings.TrimSpace(string(data))
	}

	fmt.Printf("Updating %s/ (%s) to %s from %s ...\n", infraDir, before, ref, infrastructureCloneURL)
	if err := replaceInfrastructure(ref); err != nil {
		return err
	}

	after := "unknown"
	if data, err := os.ReadFile(filepath.Join(infraDir, "version.txt")); err == nil {
		after = strings.TrimSpace(string(data))
	}
	fmt.Printf("✓ %s/ is now %s (was %s). Run `meroku generate <env>` to regenerate.\n", infraDir, after, before)
	return nil
}

// replaceInfrastructure installs ref as infrastructure/, keeping the old copy
// until the new one is complete.
//
// The previous initProject deleted infrastructure/ first and cloned second, so
// a failed clone (no network, a ref that does not exist) left the project with
// no modules at all. Here the clone goes to a sibling directory and the swap
// happens only after it succeeded.
//
// A symlinked infrastructure/ is refused: that is a developer pointing a
// project at a meroku checkout, and replacing it would throw the link away.
func replaceInfrastructure(ref string) error {
	if info, err := os.Lstat(infraDir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		target, _ := os.Readlink(infraDir)
		return fmt.Errorf("%s/ is a symlink to %s; update that checkout with git instead", infraDir, target)
	}

	stamp := time.Now().Format("20060102-150405")
	staging := fmt.Sprintf("%s.new-%s", infraDir, stamp)
	backup := fmt.Sprintf("%s.old-%s", infraDir, stamp)

	cmd := exec.Command("git", "clone", "--quiet", "--depth=1", "--branch="+ref, infrastructureCloneURL, staging)
	if output, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(staging)
		return fmt.Errorf("cloning %s at %s: %w\n%s", infrastructureCloneURL, ref, err, strings.TrimSpace(string(output)))
	}
	if err := os.RemoveAll(filepath.Join(staging, ".git")); err != nil {
		_ = os.RemoveAll(staging)
		return fmt.Errorf("removing .git from the new copy: %w", err)
	}

	hadOld := false
	if _, err := os.Stat(infraDir); err == nil {
		if err := os.Rename(infraDir, backup); err != nil {
			_ = os.RemoveAll(staging)
			return fmt.Errorf("moving the old %s/ aside: %w", infraDir, err)
		}
		hadOld = true
	}
	if err := os.Rename(staging, infraDir); err != nil {
		if hadOld {
			_ = os.Rename(backup, infraDir)
		}
		_ = os.RemoveAll(staging)
		return fmt.Errorf("installing the new %s/: %w", infraDir, err)
	}
	if hadOld {
		_ = os.RemoveAll(backup)
	}
	return nil
}
