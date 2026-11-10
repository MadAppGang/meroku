package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
)

// An environment initialised under one major of hashicorp/aws keeps that major
// in .terraform.lock.hcl. When the generated main.tf asks for a newer major,
// `terraform plan` fails with "locked provider ... does not match configured
// version constraint", and plain `terraform init` does not help: only
// `init -upgrade` may select a version outside the lock. terraformInitIfNeeded
// skips init entirely when .terraform exists, so it checks this first.

var (
	lockAWSVersion = regexp.MustCompile(`(?s)provider "registry\.terraform\.io/hashicorp/aws" \{\s*version\s*=\s*"(\d+)\.`)
	// The aws entry of required_providers: source first, then version, which
	// is how env/main.hbs writes it.
	mainTFAWSConstraint = regexp.MustCompile(`(?s)source\s*=\s*"hashicorp/aws"\s*version\s*=\s*"[~>=< ]*(\d+)\.`)
)

// lockedProviderMajorBehind reports whether the lock file pins hashicorp/aws
// to a lower major than main.tf requires. Any file it cannot read or parse
// counts as "not behind": the caller then does what it did before.
func lockedProviderMajorBehind(lockPath, mainTFPath string) (behind bool, locked, wanted int) {
	lock, err := os.ReadFile(lockPath)
	if err != nil {
		return false, 0, 0
	}
	mainTF, err := os.ReadFile(mainTFPath)
	if err != nil {
		return false, 0, 0
	}
	lm := lockAWSVersion.FindSubmatch(lock)
	cm := mainTFAWSConstraint.FindSubmatch(mainTF)
	if lm == nil || cm == nil {
		return false, 0, 0
	}
	locked, _ = strconv.Atoi(string(lm[1]))
	wanted, _ = strconv.Atoi(string(cm[1]))
	return locked < wanted, locked, wanted
}

// lockUpgradeInit is the init run when the lock is a major behind. A variable
// so tests can replace it.
var lockUpgradeInit = func() error {
	_, err := terraformInit("-upgrade", "-input=false")
	return err
}

// lockSelectsAWS reports whether the lock file selects a hashicorp/aws version.
// A .terraform directory can outlive its lock: an init stopped before it wrote
// one, or the lock was deleted. Every terraform command then fails with
// "Inconsistent dependency lock file ... no version is selected", and a plain
// init writes the lock again.
func lockSelectsAWS(lockPath string) bool {
	lock, err := os.ReadFile(lockPath)
	if err != nil {
		return false
	}
	return lockAWSVersion.Match(lock)
}

// lockInit is the init run when the lock selects no aws version. A variable
// so tests can replace it.
var lockInit = func() error {
	_, err := terraformInit("-input=false")
	return err
}

// upgradeLockedProviderIfBehind runs `terraform init` in the current directory
// when the lock file is missing or selects no aws version, and
// `terraform init -upgrade` when it is a provider major behind main.tf.
func upgradeLockedProviderIfBehind() error {
	if !lockSelectsAWS(".terraform.lock.hcl") {
		fmt.Println("🔄 .terraform.lock.hcl is missing or selects no AWS provider version.")
		fmt.Println("   Running terraform init to write it.")
		return lockInit()
	}
	behind, locked, wanted := lockedProviderMajorBehind(".terraform.lock.hcl", "main.tf")
	if !behind {
		return nil
	}
	fmt.Printf("🔄 The AWS provider lock is on version %d.x and this meroku generates for %d.x.\n", locked, wanted)
	fmt.Println("   Running terraform init -upgrade to move the lock to the new version.")
	return lockUpgradeInit()
}
