package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

// handleCertCommand runs `meroku cert check <env>`: ACM's own validation
// status for the environment's certificates, and why a pending one is pending.
func handleCertCommand(args []string) error {
	if len(args) != 2 || args[0] != "check" {
		fmt.Println("Usage: meroku cert check <env>")
		fmt.Println()
		fmt.Println("Shows ACM's validation status for the environment's certificates and, for")
		fmt.Println("each pending one, whether the validation record is the reason.")
		if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
			return nil
		}
		return fmt.Errorf("unknown cert command: %s", strings.Join(args, " "))
	}
	envName := args[1]
	names, err := listEnvironmentNames()
	if err != nil {
		return err
	}
	if !slices.Contains(names, envName) {
		return fmt.Errorf("environment %q not found — expected one of: %s (run from the project root)",
			envName, strings.Join(names, ", "))
	}
	env, err := loadEnv(envName)
	if err != nil {
		return fmt.Errorf("failed to load %s: %w", envName, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	checks, err := checkCertificateValidation(ctx, env, nil)
	if err != nil {
		return err
	}
	if len(checks) == 0 {
		fmt.Printf("No certificate for %s is waiting on validation.\n", expectedZoneName(env))
		return nil
	}
	for _, c := range checks {
		fmt.Printf("%s  %s  (%s)\n", verdictIcon(c.Verdict), c.Domain, c.Status)
		fmt.Printf("   %s\n", c.Detail)
		fmt.Printf("   %s\n", c.CertARN)
	}
	return nil
}

func verdictIcon(v certVerdict) string {
	switch v {
	case certIssued:
		return "✅"
	case certWaitingOnACM, certUnchecked:
		return "⏳"
	default:
		return "❌"
	}
}
