package main

// `meroku vpc-cidr check|suggest|assign <env> [--aws] [--force]`

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// vpcCIDRAWSTimeout bounds the AWS half of every vpc-cidr command.
const vpcCIDRAWSTimeout = 15 * time.Second

func printVPCCIDRUsage() {
	fmt.Println("Usage: meroku vpc-cidr <check|suggest|assign> <env> [--aws] [--force]")
	fmt.Println("")
	fmt.Println("  check <env>    Validate <env>'s vpc_cidr and report overlaps with the other")
	fmt.Println("                 environments (exit 1 when the range breaks a rule)")
	fmt.Println("  suggest <env>  Print a free range for <env> without writing anything")
	fmt.Println("  assign <env>   Keep <env>'s range if it is valid and free, otherwise write a")
	fmt.Println("                 free one into <env>.yaml (refused for deployed environments)")
	fmt.Println("")
	fmt.Println("  --aws    Also check the VPCs in <env>'s AWS account and region (uses the")
	fmt.Println("           aws_profile and region from <env>.yaml, never $AWS_PROFILE)")
	fmt.Println("  --force  assign only: write even if <env> is or may be deployed. Changing")
	fmt.Println("           vpc_cidr replaces the VPC and everything in it")
}

// vpcCIDRCommandArgs is the parsed command line.
type vpcCIDRCommandArgs struct {
	Action  string
	EnvName string
	AWS     bool
	Force   bool
}

// parseVPCCIDRArgs accepts flags before or after the environment.
func parseVPCCIDRArgs(args []string) (vpcCIDRCommandArgs, error) {
	var out vpcCIDRCommandArgs
	var positional []string
	for _, a := range args {
		switch a {
		case "--aws", "-aws":
			out.AWS = true
		case "--force", "-force":
			out.Force = true
		default:
			if strings.HasPrefix(a, "-") {
				return out, fmt.Errorf("unknown flag %s", a)
			}
			positional = append(positional, a)
		}
	}
	if len(positional) != 2 {
		return out, errors.New("expected an action and an environment: meroku vpc-cidr <check|suggest|assign> <env>")
	}
	out.Action, out.EnvName = positional[0], positional[1]
	switch out.Action {
	case "check", "suggest", "assign":
	default:
		return out, fmt.Errorf("unknown action %q: expected check, suggest or assign", out.Action)
	}
	if out.Force && out.Action != "assign" {
		return out, errors.New("--force only applies to assign")
	}
	return out, nil
}

// handleVPCCIDRCommand runs `meroku vpc-cidr`. Non-interactive: errors are
// returned for main to print and exit 1 on.
func handleVPCCIDRCommand(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printVPCCIDRUsage()
		return nil
	}
	parsed, err := parseVPCCIDRArgs(args)
	if err != nil {
		printVPCCIDRUsage()
		return err
	}

	names, err := listEnvironmentNames()
	if err != nil {
		return err
	}
	if !slices.Contains(names, parsed.EnvName) {
		return fmt.Errorf("environment %q not found — expected one of: %s (run from the project root)",
			parsed.EnvName, strings.Join(names, ", "))
	}
	env, err := loadEnv(parsed.EnvName)
	if err != nil {
		return fmt.Errorf("failed to load %s: %w", parsed.EnvName, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), vpcCIDRAWSTimeout)
	defer cancel()
	deps := defaultVPCCIDRDeps()

	switch parsed.Action {
	case "check":
		report := inspectVPCCIDR(ctx, deps, env, vpcCIDRInspectOptions{
			EnvName: parsed.EnvName, Dir: ".", WithAWS: parsed.AWS,
		})
		printVPCCIDRReport(report)
		if report.Error != nil {
			return fmt.Errorf("vpc_cidr for %s breaks a rule. %s", parsed.EnvName, vpcCIDROverrideGuidance(parsed.EnvName))
		}
		return nil

	case "suggest":
		report := inspectVPCCIDR(ctx, deps, env, vpcCIDRInspectOptions{
			EnvName: parsed.EnvName, Dir: ".", WithAWS: parsed.AWS, Count: 1,
		})
		printVPCCIDRAWSStatus(report)
		if len(report.Suggestions) == 0 {
			return errVPCCIDRPoolExhausted
		}
		fmt.Printf("Suggested vpc_cidr for %s: %s\n", parsed.EnvName, report.Suggestions[0])
		fmt.Printf("Write it with: meroku vpc-cidr assign %s", parsed.EnvName)
		if parsed.AWS {
			fmt.Print(" --aws")
		}
		fmt.Println()
		return nil

	default: // assign
		result, err := assignVPCCIDR(ctx, deps, env, vpcCIDRAssignOptions{
			EnvName: parsed.EnvName, Dir: ".", WithAWS: parsed.AWS, Force: parsed.Force,
		})
		printVPCCIDRAWSStatus(result.Report)
		if err != nil {
			if errors.Is(err, errVPCCIDRAssignRefused) && result.CIDR != "" {
				fmt.Printf("Suggested vpc_cidr for %s: %s (not written)\n", parsed.EnvName, result.CIDR)
			}
			return err
		}
		printVPCCIDRAssignResult(parsed.EnvName, result)
		return nil
	}
}

func printVPCCIDRAWSStatus(r vpcCIDRReport) {
	if r.AWSNote != "" {
		fmt.Printf("Note: %s", r.AWSNote)
		if r.AWSError != "" {
			fmt.Printf(" (%s)", r.AWSError)
		}
		fmt.Println()
	}
	if r.StateError != "" {
		fmt.Printf("Remote state could not be read: %s\n", r.StateError)
	}
}

func printVPCCIDRReport(r vpcCIDRReport) {
	fmt.Printf("Environment: %s\n", r.Env)
	if r.UseDefaultVPC {
		fmt.Println("Uses the AWS default VPC (use_default_vpc: true); vpc_cidr is not used.")
		return
	}
	configured := r.Configured
	if configured == "" {
		configured = "(not set)"
	}
	fmt.Printf("vpc_cidr:    %s\n", configured)
	fmt.Printf("Effective:   %s\n", r.Effective)
	if r.Override {
		fmt.Println("Rule checks: off (allow_vpc_cidr_override: true)")
	}
	if r.Error != nil {
		fmt.Printf("\n❌ %s\n", r.Error.Message)
	}
	if len(r.Warnings) > 0 {
		fmt.Println()
		for _, w := range r.Warnings {
			fmt.Printf("⚠️  %s\n", w.Message)
		}
	}
	if r.Error == nil && len(r.Warnings) == 0 {
		if r.AWSChecked {
			fmt.Println("\n✅ Valid, and no overlap with other environments or VPCs in the account.")
		} else {
			fmt.Println("\n✅ Valid, and no overlap with other environments.")
		}
	}
	fmt.Println()
	printVPCCIDRAWSStatus(r)
	fmt.Printf("Deployed: %s (%s)\n", r.Deployed, r.DeployedReason)
	// Only after a sibling overlap was shown (plan review M2), as generate does.
	if len(r.Warnings) > 0 && !r.AWSChecked && r.AWSNote == "" {
		fmt.Printf("Tip: `meroku vpc-cidr check %s --aws` also checks VPCs already in your AWS account.\n", r.Env)
	}
}

func printVPCCIDRAssignResult(envName string, result vpcCIDRAssignResult) {
	if !result.Changed {
		fmt.Printf("%s keeps vpc_cidr %s — it is valid and does not overlap anything checked.\n", envName, result.CIDR)
		return
	}
	previous := result.Previous
	if previous == "" {
		previous = "(not set)"
	}
	fmt.Printf("%s: vpc_cidr %s → %s\n", envName, previous, result.CIDR)
	if result.BackupPath != "" {
		fmt.Printf("Backup: %s\n", result.BackupPath)
	}
}

// offerVPCCIDRCheckForNewAccount is the TUI hook run right after an AWS
// account is first bound to an environment: offer to check that account for
// existing VPCs and move the env's range if it overlaps. Every failure is
// printed and swallowed — this must never stop environment selection.
func offerVPCCIDRCheckForNewAccount(envName string, confirm func(title string) (bool, error)) {
	env, err := loadEnv(envName)
	if err != nil || env.UseDefaultVPC || env.AWSProfile == "" || env.Region == "" {
		return
	}
	title := fmt.Sprintf("Check %s/%s for existing VPCs and move this env's range if it overlaps?", env.AccountID, env.Region)
	ok, err := confirm(title)
	if err != nil || !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), vpcCIDRAWSTimeout)
	defer cancel()
	result, err := assignVPCCIDR(ctx, defaultVPCCIDRDeps(), env, vpcCIDRAssignOptions{
		EnvName: envName, Dir: ".", WithAWS: true,
	})
	printVPCCIDRAWSStatus(result.Report)
	if err != nil {
		fmt.Printf("%v\n", err)
		return
	}
	printVPCCIDRAssignResult(envName, result)
}
