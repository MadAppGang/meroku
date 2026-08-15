package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Region drift: the YAML `region` and the region the AWS provider would resolve
// on its own disagreeing.
//
// This used to be invisible. env/main.hbs pinned no region on the default
// provider, so one value had two sources — the S3 backend read `region` from
// the config, every resource read it from AWS_REGION or the profile. A shell
// pointed at a different region put the state in one place and the
// infrastructure in another, silently, for as long as nobody looked.
//
// The template now pins the region, which fixes new deployments and reveals old
// ones: a stack that has been running mismatched will plan a move to the YAML
// region, and for real infrastructure that plan is a destroy-and-recreate. That
// is the correct end state but a terrible way to find out, so generation warns
// first. It does not fail — a mismatch is harmless on a stack that does not
// exist yet, and we cannot tell the two apart without reading state.

// resolveAmbientRegion returns the region the AWS provider would use if the
// template pinned nothing, and where that value came from. Empty region means
// nothing configured it.
//
// This mirrors the provider's own precedence: AWS_REGION, then
// AWS_DEFAULT_REGION, then the `region` key of the active profile in the shared
// config file. It is a read of local configuration only — no API calls, because
// this runs on every generate.
func resolveAmbientRegion(profile string) (region, source string) {
	if v := strings.TrimSpace(os.Getenv("AWS_REGION")); v != "" {
		return v, "AWS_REGION"
	}
	if v := strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION")); v != "" {
		return v, "AWS_DEFAULT_REGION"
	}

	if profile == "" {
		profile = strings.TrimSpace(os.Getenv("AWS_PROFILE"))
	}
	if profile == "" {
		profile = "default"
	}

	configPath := strings.TrimSpace(os.Getenv("AWS_CONFIG_FILE"))
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", ""
		}
		configPath = filepath.Join(home, ".aws", "config")
	}

	if v := regionFromSharedConfig(configPath, profile); v != "" {
		return v, fmt.Sprintf("profile %q in %s", profile, configPath)
	}
	return "", ""
}

// regionFromSharedConfig reads the `region` key of one profile out of an AWS
// shared config file. Sections are `[default]` or `[profile name]`.
func regionFromSharedConfig(path, profile string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	want := "[profile " + profile + "]"
	if profile == "default" {
		want = "[default]"
	}

	inSection := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inSection = line == want
			continue
		}
		if !inSection {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || strings.TrimSpace(key) != "region" {
			continue
		}
		return strings.TrimSpace(value)
	}
	return ""
}

// regionDriftWarning returns the operator-facing warning for a YAML region that
// disagrees with the ambient one, or "" when they agree or nothing is ambient.
//
// Split from the printing so it can be tested without capturing stdout.
func regionDriftWarning(yamlRegion, ambientRegion, source string) string {
	yamlRegion = strings.TrimSpace(yamlRegion)
	ambientRegion = strings.TrimSpace(ambientRegion)

	if yamlRegion == "" || ambientRegion == "" || yamlRegion == ambientRegion {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n⚠️  Region mismatch\n\n")
	fmt.Fprintf(&b, "    config region:  %s\n", yamlRegion)
	fmt.Fprintf(&b, "    your shell:     %s  (from %s)\n\n", ambientRegion, source)
	fmt.Fprintf(&b, "Terraform will use %s, because the config is authoritative.\n\n", yamlRegion)
	fmt.Fprintf(&b, "If this environment was last applied from a shell set to %s, its\n", ambientRegion)
	fmt.Fprintf(&b, "resources are there while its state is in %s, and the next plan will\n", yamlRegion)
	fmt.Fprintf(&b, "move them — as a destroy and recreate, not a migration.\n\n")
	fmt.Fprintf(&b, "Read the plan before applying. If %s is the region you want, set it as\n", yamlRegion)
	fmt.Fprintf(&b, "`region` in the config and clear AWS_REGION so the two agree.\n")
	return b.String()
}

// warnOnRegionDrift prints the warning, if there is one, during generation.
func warnOnRegionDrift(yamlRegion, profile string) {
	ambient, source := resolveAmbientRegion(profile)
	if w := regionDriftWarning(yamlRegion, ambient, source); w != "" {
		fmt.Print(w)
	}
}

// stringFromMap reads a string value out of a rendered env map, tolerating a
// missing key and a non-string value rather than asserting: this runs on config
// that has already been through migration, but a hand-edited YAML can still put
// anything anywhere, and a warning is not worth a panic.
func stringFromMap(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
