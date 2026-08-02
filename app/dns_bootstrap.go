package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// zoneTargetAddress is the terraform address of the hosted zone created by
// modules/domain.
//
// It is deliberately the only thing phase 1 targets. The zone is a dependency
// free leaf — modules/domain/main.tf creates it with no references to anything
// else — so `-target` on it creates exactly one resource. Targeting the whole
// module would drag in the ACM certificates and their validation, which is the
// very thing that cannot succeed before delegation exists.
const zoneTargetAddress = "module.domain.aws_route53_zone.domain[0]"

// bootstrapDNSZone runs phase 1 of a two-phase deploy: create just the hosted
// zone, so that its nameservers exist and delegation can be set up.
//
// Must be called with the working directory already inside env/<env> and after
// terraform init, exactly like runTerraformApply.
func bootstrapDNSZone() error {
	fmt.Println("\n🌐 Phase 1 of 2: creating the DNS hosted zone only")
	fmt.Printf("   terraform apply -target=%s\n\n", zoneTargetAddress)

	args := []string{
		"apply",
		"-no-color",
		"-auto-approve",
		"-target=" + zoneTargetAddress,
	}

	cmd := exec.Command("terraform", args...)
	cmd.Stdin = os.Stdin

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to attach to terraform stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to attach to terraform stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start terraform: %w", err)
	}

	// One resource, so plain streaming is enough — a progress TUI would be noise.
	var captured strings.Builder
	done := make(chan struct{}, 2)
	relay := func(r io.Reader) {
		defer func() { done <- struct{}{} }()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			captured.WriteString(line + "\n")
			fmt.Println("   " + line)
		}
	}
	go relay(stdout)
	go relay(stderr)
	<-done
	<-done

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("zone-only apply failed: %w\n%s", err, captured.String())
	}

	fmt.Println("\n✅ Hosted zone created.")
	return nil
}

// runDNSBootstrapAndDelegate performs phase 1, then drives the delegation flow,
// then re-checks. It reports whether the environment is ready for the full apply.
//
// Returns true when delegation is verified and phase 2 can proceed immediately.
func runDNSBootstrapAndDelegate(ctx context.Context, e Env) (bool, error) {
	if err := bootstrapDNSZone(); err != nil {
		return false, err
	}

	// Re-read state from AWS: the zone now exists, so the preflight can report
	// its ID and nameservers for the delegation step.
	res, err := checkDNSPreflight(ctx, e)
	if err != nil {
		return false, fmt.Errorf("could not inspect the new zone: %w", err)
	}

	// Never trust -target. Terraform does NOT error on an address that matches
	// nothing — it prints "No changes" plus a generic targeting warning and exits
	// 0. So a drifted address (for example if modules/domain drops the count on
	// the zone, making the correct address ...domain rather than ...domain[0])
	// would silently create nothing here and surface later as an unrelated
	// failure. Verify against AWS instead of trusting the exit code.
	if res.ZoneID == "" {
		return false, fmt.Errorf(
			"phase 1 reported success but hosted zone %s still does not exist.\n"+
				"The -target address %q probably no longer matches modules/domain "+
				"(terraform does not report a target that matches nothing)",
			res.ZoneName, zoneTargetAddress)
	}

	if res.Plan == dnsPlanNormal {
		// Unlikely but possible: delegation was already in place, waiting for the
		// zone to exist.
		fmt.Print(describeDNSPreflight(res))
		return true, nil
	}

	if err := runDelegationFlow(ctx, e, res); err != nil {
		return false, err
	}

	// Did the delegation actually land?
	final, err := checkDNSPreflight(ctx, e)
	if err != nil {
		return false, fmt.Errorf("could not re-check delegation: %w", err)
	}
	return final.Plan == dnsPlanNormal, nil
}
