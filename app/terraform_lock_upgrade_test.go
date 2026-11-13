package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const lockFixture = `# This file is maintained automatically by "terraform init".
provider "registry.terraform.io/hashicorp/aws" {
  version     = "%s"
  constraints = "~> 5.0"
  hashes = [
    "h1:fake-hash-do-not-use=",
  ]
}
`

const mainTFFixture = `terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "%s"
    }
  }
}
`

func writeLockFixtures(t *testing.T, lockVersion, constraint string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	lock := filepath.Join(dir, ".terraform.lock.hcl")
	main := filepath.Join(dir, "main.tf")
	if lockVersion != "" {
		if err := os.WriteFile(lock, []byte(fmt.Sprintf(lockFixture, lockVersion)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(main, []byte(fmt.Sprintf(mainTFFixture, constraint)), 0o644); err != nil {
		t.Fatal(err)
	}
	return lock, main
}

func TestLockedProviderMajorBehind(t *testing.T) {
	cases := []struct {
		name       string
		lock       string
		constraint string
		want       bool
	}{
		{"5 locked, 6 wanted", "5.100.0", "~> 6.0", true},
		{"6 locked, 6 wanted", "6.72.0", "~> 6.0", false},
		{"5 locked, 5 wanted", "5.100.0", "~> 5.0", false},
		{"range constraint", "5.100.0", ">= 6.0.0, < 7.0.0", true},
		{"no lock file", "", "~> 6.0", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lock, main := writeLockFixtures(t, tc.lock, tc.constraint)
			got, _, _ := lockedProviderMajorBehind(lock, main)
			if got != tc.want {
				t.Errorf("lockedProviderMajorBehind = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUpgradeLockedProviderIfBehindRunsInitUpgrade(t *testing.T) {
	lock, _ := writeLockFixtures(t, "5.100.0", "~> 6.0")
	dir := filepath.Dir(lock)
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousDir) })

	calls := 0
	previous := lockUpgradeInit
	lockUpgradeInit = func() error { calls++; return nil }
	t.Cleanup(func() { lockUpgradeInit = previous })

	if err := upgradeLockedProviderIfBehind(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("init -upgrade ran %d times, want 1", calls)
	}

	if err := os.WriteFile(lock, []byte(fmt.Sprintf(lockFixture, "6.72.0")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := upgradeLockedProviderIfBehind(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("a lock on the wanted major must not re-run init; ran %d times", calls)
	}
}

func TestTerraformErrorLockedProviderAsksForUpgrade(t *testing.T) {
	out := "Error: Failed to query available provider packages\n\n" +
		"locked provider registry.terraform.io/hashicorp/aws 5.100.0 does not match\n" +
		"configured version constraint ~> 6.0; must use terraform init -upgrade to allow\n" +
		"selection of new versions"
	got, err := terraformError(out)
	if err != nil {
		t.Fatal(err)
	}
	// REQ-20 / TEST-17: recovery returns argv only; the caller owns the executable.
	want := []string{"init", "-upgrade", "-input=false"}
	if len(got) != len(want) {
		t.Fatalf("terraformError = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("terraformError = %v, want %v", got, want)
		}
	}
}

func TestUpgradeLockedProviderIfBehindInitsWithoutALock(t *testing.T) {
	for name, lockContent := range map[string]string{
		"no lock file":     "",
		"lock without aws": "# This file is maintained automatically by \"terraform init\".\n",
	} {
		t.Run(name, func(t *testing.T) {
			lock, _ := writeLockFixtures(t, "", "~> 6.0")
			if lockContent != "" {
				if err := os.WriteFile(lock, []byte(lockContent), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			previousDir, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(filepath.Dir(lock)); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chdir(previousDir) })

			inits, upgrades := 0, 0
			previousInit, previousUpgrade := lockInit, lockUpgradeInit
			lockInit = func() error { inits++; return nil }
			lockUpgradeInit = func() error { upgrades++; return nil }
			t.Cleanup(func() { lockInit, lockUpgradeInit = previousInit, previousUpgrade })

			if err := upgradeLockedProviderIfBehind(); err != nil {
				t.Fatal(err)
			}
			if inits != 1 || upgrades != 0 {
				t.Errorf("init ran %d times and init -upgrade %d times, want 1 and 0", inits, upgrades)
			}
		})
	}
}
