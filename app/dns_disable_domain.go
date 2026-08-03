package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// disableCustomDomain sets domain.enabled to false in <env>.yaml.
//
// It rewrites the single line rather than round-tripping through the Env struct.
// A marshal/unmarshal cycle silently drops every key the Go type does not model
// and reformats the rest, which is far too much collateral change for flipping
// one boolean — and this file is the one thing in a project that cannot be
// regenerated.
//
// Returns the path of the backup it wrote.
func disableCustomDomain(envName string) (string, error) {
	path := envName + ".yaml"

	original, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("could not read %s: %w", path, err)
	}

	updated, changed := setDomainEnabledFalse(string(original))
	if !changed {
		return "", fmt.Errorf("could not find domain.enabled in %s", path)
	}

	backup := fmt.Sprintf("%s.backup_%s", path, time.Now().Format("20060102_150405"))
	if err := os.WriteFile(backup, original, 0o644); err != nil {
		return "", fmt.Errorf("could not back up %s: %w", path, err)
	}

	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return backup, fmt.Errorf("could not write %s: %w", path, err)
	}
	return backup, nil
}

// applyDNSOutcome acts on what the DNS setup screen returned, and reports
// whether the environment config changed and so needs regenerating.
//
// It exits the process on the paths where continuing would produce a deploy the
// operator did not ask for. Keeping that decision here rather than in the model
// keeps the TUI free of process control, which is what makes it testable.
func applyDNSOutcome(envName string, e *Env, outcome dnsSetupOutcome) bool {
	switch {
	case outcome.Delegated:
		return false

	case outcome.SkipDomain:
		backup, err := disableCustomDomain(envName)
		if err != nil {
			fmt.Printf("\n❌ Could not disable the custom domain: %v\n", err)
			fmt.Printf("   Set domain.enabled to false in %s.yaml by hand, then re-run.\n", envName)
			os.Exit(1)
		}
		e.Domain.Enabled = false
		fmt.Printf("\n🌐 Custom domain disabled in %s.yaml (backup: %s)\n", envName, backup)
		fmt.Println("   The certificate and hosted zone leave the plan — review it before applying.")
		fmt.Println("   Re-enable it any time by setting domain.enabled back to true.")
		return true

	case outcome.ContinueAnyway:
		fmt.Println("\n⚠️  Continuing without delegation, as requested.")
		fmt.Println("   Certificate validation cannot succeed, so the apply is expected to")
		fmt.Println("   stall on it for 20 minutes and then fail.")
		return false

	default:
		// Cancelled.
		fmt.Println("\n⏸  Delegation is not in place yet — stopping before the apply stalls.")
		fmt.Println("   Re-run the deploy once it resolves. Check with: meroku dns validate")
		os.Exit(0)
		return false
	}
}

// setDomainEnabledFalse flips the `enabled` key inside the top-level `domain`
// block, leaving every other byte of the document alone.
//
// Scoping to that block matters: `enabled: true` appears under postgres,
// cognito, ses, sqs, alb and pubsub_appsync as well, and a naive replace would
// silently disable whichever came first.
func setDomainEnabledFalse(doc string) (string, bool) {
	lines := strings.Split(doc, "\n")
	inDomain := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// A top-level key is unindented. Reaching one ends the domain block.
		if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inDomain = trimmed == "domain:"
			continue
		}

		if !inDomain {
			continue
		}

		if key, value, found := strings.Cut(trimmed, ":"); found && strings.TrimSpace(key) == "enabled" {
			if strings.TrimSpace(value) == "false" {
				return doc, true // already off; nothing to change but not an error
			}
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = indent + "enabled: false"
			return strings.Join(lines, "\n"), true
		}
	}

	return doc, false
}
