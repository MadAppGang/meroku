package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
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
		if missing := lockMissingProviders("."); len(missing) > 0 || modulesMissing(".") {
			fmt.Println("🔄 The provider lock or installed modules do not cover main.tf yet. Running terraform init.")
			return lockInit()
		}
		return nil
	}
	fmt.Printf("🔄 The AWS provider lock is on version %d.x and this meroku generates for %d.x.\n", locked, wanted)
	fmt.Println("   Running terraform init -upgrade to move the lock to the new version.")
	return lockUpgradeInit()
}

var (
	reRequiredSource = regexp.MustCompile(`source\s*=\s*"([^"]+)"`)
	reModuleBlock    = regexp.MustCompile(`(?m)^\s*module\s+"([^"]+)"`)
)

// lockMissingProviders lists the provider sources dir's configuration
// (main.tf and the deploy-time Neon files) requires that the lock file does
// not select. A fresh init picks the newest version of a provider only state
// requires, so this runs after the deploy-time files with their exact pins
// are written.
func lockMissingProviders(dir string) []string {
	lock, _ := os.ReadFile(filepath.Join(dir, ".terraform.lock.hcl"))
	lockText := strings.ToLower(string(lock))
	seen := map[string]bool{}
	var missing []string
	for _, name := range []string{"main.tf", neonRetiredFile, neonRetiredOverrideFile} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		for _, m := range reRequiredSource.FindAllSubmatch(requiredProvidersBlocks(data), -1) {
			src := strings.ToLower(string(m[1]))
			if strings.Count(src, "/") == 1 {
				src = "registry.terraform.io/" + src
			}
			if seen[src] || strings.Count(src, "/") != 2 {
				continue
			}
			seen[src] = true
			if !strings.Contains(lockText, `provider "`+src+`"`) {
				missing = append(missing, src)
			}
		}
	}
	sort.Strings(missing)
	return missing
}

var reRequiredProviders = regexp.MustCompile(`required_providers\s*\{`)

// requiredProvidersBlocks is the bodies of every required_providers block in
// data, so module sources never count as providers.
func requiredProvidersBlocks(data []byte) []byte {
	var out []byte
	for _, loc := range reRequiredProviders.FindAllIndex(data, -1) {
		depth := 0
		for i := loc[1] - 1; i < len(data); i++ {
			switch data[i] {
			case '{':
				depth++
			case '}':
				depth--
			}
			if depth == 0 {
				out = append(out, data[loc[1]:i]...)
				out = append(out, '\n')
				break
			}
		}
	}
	return out
}

// modulesMissing reports whether main.tf in dir declares a module that
// .terraform/modules/modules.json does not list. Without main.tf or a
// .terraform directory it reports false: init decides those cases elsewhere.
func modulesMissing(dir string) bool {
	mainTF, err := os.ReadFile(filepath.Join(dir, "main.tf"))
	if err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, ".terraform")); err != nil {
		return false
	}
	installed := map[string]bool{}
	if data, err := os.ReadFile(filepath.Join(dir, ".terraform", "modules", "modules.json")); err == nil {
		var manifest struct {
			Modules []struct {
				Key string `json:"Key"`
			} `json:"Modules"`
		}
		if json.Unmarshal(data, &manifest) == nil {
			for _, m := range manifest.Modules {
				installed[m.Key] = true
			}
		}
	}
	for _, m := range reModuleBlock.FindAllSubmatch(mainTF, -1) {
		if !installed[string(m[1])] {
			return true
		}
	}
	return false
}

// initIfLockIncomplete runs a plain `terraform init -input=false` in dir
// (never -upgrade) when the lock lacks a required provider or a module is not
// installed.
func initIfLockIncomplete(ctx context.Context, dir string, out io.Writer) error {
	missing := lockMissingProviders(dir)
	if len(missing) == 0 && !modulesMissing(dir) {
		return nil
	}
	if len(missing) > 0 {
		fmt.Fprintf(out, "🔄 The provider lock does not select %s yet. Running terraform init.\n", strings.Join(missing, ", "))
	} else {
		fmt.Fprintln(out, "🔄 A module is not installed yet. Running terraform init.")
	}
	cmd, err := newTerraformCmd(ctx, tfState, dir, "init", "-input=false", "-no-color")
	if err != nil {
		return err
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("terraform init: %w\n%s", err, lastLines(redactSecrets(string(output)), 12))
	}
	return nil
}
