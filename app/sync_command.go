package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// `meroku sync` — the explicit form of the check in state_reconnect.go.
//
// The automatic check is deliberately quiet: it says nothing when an environment
// is healthy, because it runs as a side effect of generating or of picking an
// environment from a menu, and nobody asked it a question. This command is the
// opposite. Someone typed it to find out where they stand, so it reports what it
// found whatever that is, and ends with a verdict. A command you ran on purpose
// that prints nothing feels broken.
//
// It never asks for permission. Typing the command is the permission, and a
// tool that asks you to confirm the thing you just typed trains you to stop
// reading its questions.
//
// What it will never do: apply, destroy, or move state. `terraform init` here is
// plain — no -reconfigure, no -migrate-state — and `terraform plan` is read-only.
// Drift is reported, never acted on.

// syncDeps is the injectable half of the command: everything that reads AWS,
// writes files or runs terraform. Tests supply fakes; production supplies the
// package defaults.
type syncDeps struct {
	lookup remoteStateLookup
	gen    environmentGenerator
	init   terraformInitializer
	plan   terraformPlanner

	// decide is never consulted by this command — running it is the consent —
	// but it is carried so tests can arm it as a fuse and prove that.
	decide syncConsent
}

func defaultSyncDeps() syncDeps {
	return syncDeps{
		lookup: defaultRemoteStateLookup,
		gen:    defaultEnvironmentGenerator,
		init:   defaultTerraformInitializer,
		plan:   defaultTerraformPlanner,
	}
}

// syncRequestFor builds the request this command runs. ask is always false:
// `meroku sync` was typed on purpose, and asking a second time for the thing
// you just asked for is how a tool teaches people to stop reading its prompts.
func (d syncDeps) syncRequestFor(c stateConnection, envDir string) syncRequest {
	return syncRequest{
		conn:   c,
		envDir: envDir,
		ask:    false,
		// Somebody typed this command to find out where they stand, and
		// "linked" alone is only half an answer — linked to what, and does the
		// configuration still match it?
		compareWhenLinked: true,
		gen:               d.gen,
		init:              d.init,
		plan:              d.plan,
		decide:            d.decide,
	}
}

// handleSyncCommand is the main.go entry point for `meroku sync [env]`.
//
// Exit status is about the command, not about the infrastructure: a plan full of
// drift exits 0, because being told about drift is the command working. Only a
// step that could not be completed — an environment that cannot be resolved or
// read, a backend that cannot be reached, a failed generate or init — exits 1.
func handleSyncCommand(args []string) {
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "help" {
			printSyncUsage()
			return
		}
	}

	envName, err := resolveSyncEnvironment(args, *envFlag, listEnvironmentNames)
	if err != nil {
		fmt.Printf("\n❌ %v\n", err)
		os.Exit(1)
	}

	cfg, err := loadEnv(envName)
	if err != nil {
		fmt.Printf("\n❌ Could not read %s.yaml: %v\n", envName, err)
		fmt.Println("   meroku sync needs the environment config to find the state backend.")
		os.Exit(1)
	}

	if err := runSync(context.Background(), envName, cfg, filepath.Join("env", envName), defaultSyncDeps()); err != nil {
		os.Exit(1)
	}
}

func printSyncUsage() {
	fmt.Println("Usage: meroku sync [environment]")
	fmt.Println("Example: meroku sync dev")
	fmt.Println("")
	fmt.Println("Links an environment to the Terraform state in its S3 backend: writes")
	fmt.Println("env/<env>/ from <env>.yaml when it is missing, runs terraform init, then")
	fmt.Println("compares the configuration with what is deployed.")
	fmt.Println("")
	fmt.Println("Never applies, never destroys and never migrates state.")
	fmt.Println("With no argument the environment is taken from --env, or from the only")
	fmt.Println("environment in the project if there is exactly one.")
}

// resolveSyncEnvironment works out which environment to operate on.
//
// The order follows the rest of meroku rather than adding a convention: an
// explicit argument like `meroku generate dev`, then the --env flag that the
// interactive path honours, then the environments the selector menu would list.
// One environment is not a guess. More than one is genuine ambiguity, and the
// answer to ambiguity is to show the options, not to pick one.
func resolveSyncEnvironment(args []string, envFlag string, list func() ([]string, error)) (string, error) {
	if len(args) > 0 && args[0] != "" {
		return args[0], nil
	}
	if envFlag != "" {
		return envFlag, nil
	}

	envs, err := list()
	if err != nil {
		return "", fmt.Errorf("could not list environments: %w", err)
	}

	switch len(envs) {
	case 0:
		return "", errors.New("no environment configuration found in this directory.\n" +
			"   meroku sync expects to run from a project root containing dev.yaml (or\n" +
			"   prod.yaml, staging.yaml...).\n" +
			"   Usage: meroku sync <environment>")
	case 1:
		fmt.Printf("Using the only environment in this project: %s\n", envs[0])
		return envs[0], nil
	default:
		return "", fmt.Errorf("this project has %d environments, so meroku cannot tell which one you mean.\n"+
			"   Found: %s\n"+
			"   Run: meroku sync <environment>", len(envs), strings.Join(envs, ", "))
	}
}

// runSync performs the check and reports it. The returned error means the
// command failed, not that the infrastructure has a problem.
func runSync(ctx context.Context, envName string, env Env, envDir string, deps syncDeps) error {
	applyAWSEnvFromConfig(env)
	printSyncHeader(envName, env, envDir)

	c := probeStateConnection(ctx, envName, env, envDir, deps.lookup)

	switch c.Status {
	case stateSkipped:
		// The opt-out is honoured even here. It exists so that meroku never
		// reaches out to S3 in an environment where that is unwanted — usually CI
		// — and quietly overriding it because this particular command is explicit
		// would make the setting untrustworthy. Say exactly why nothing happened
		// and how to run the check anyway; nothing has failed, so exit 0.
		fmt.Printf("\n⏭️  Skipped: %s is set, so meroku did not read the remote state.\n", merokuSkipStateReconnect)
		printSyncVerdict(fmt.Sprintf("Nothing was checked. Unset %s to run this check.", merokuSkipStateReconnect))
		return nil

	case stateNoBackendConfigured:
		fmt.Printf("\n📭 No S3 backend is configured for '%s'.\n", envName)
		fmt.Printf("   %s.yaml needs state_bucket, state_file and region before there can be\n", envName)
		fmt.Println("   any remote state to link to.")
		printSyncVerdict(fmt.Sprintf("Nothing to sync: '%s' has no state backend configured.", envName))
		return nil

	case stateFresh:
		// The backend answered and holds nothing. Whether env/<env>/ is here or
		// not, this environment has never been deployed.
		fmt.Printf("\n🌱 '%s' is a new environment.\n\n", envName)
		fmt.Printf("   The backend s3://%s/%s holds no Terraform state with resources in\n", env.StateBucket, env.StateFile)
		fmt.Println("   it, so there is nothing deployed and nothing to link to.")
		if !c.Generated {
			fmt.Printf("\n   env/%s/ has not been written, and nothing was written now: meroku\n", envName)
			fmt.Println("   does not generate for an environment that has never been deployed.")
			fmt.Printf("   When you are ready:  meroku generate %s\n", envName)
		}
		printSyncVerdict(fmt.Sprintf("'%s' has never been deployed — nothing to sync.", envName))
		return nil

	case stateUnreadable:
		// Not a verdict about the infrastructure — meroku could not complete the
		// one thing it was asked to do, so this is the command failing.
		fmt.Printf("\n⚠️  Could not read the remote Terraform state for '%s'.\n\n", envName)
		fmt.Printf("   %v\n\n", c.Err)
		fmt.Println("   This says nothing about your infrastructure — only that meroku could")
		fmt.Println("   not look. The usual causes are an expired SSO session, the wrong")
		fmt.Println("   profile, or a bucket in another account or region.")
		fmt.Println("   Check with:  aws sts get-caller-identity")
		printSyncVerdict("Could not reach the state backend, so the connection is unknown. Fix AWS access and run this again.")
		return c.Err

	case stateAlreadyInitialised:
		return syncAlreadyConnected(ctx, envName, env, envDir, deps)

	case stateDeployedButDisconnected:
		outcome, err := performSync(deps.syncRequestFor(c, envDir))
		printSyncVerdict(outcome.Verdict)
		return err
	}

	return nil
}

// syncAlreadyConnected reports on an environment that is already wired to its
// backend.
//
// The automatic path stops at "already initialised" and touches nothing, which is
// right when nobody asked. Here somebody did ask, and "linked" on its own is only
// half an answer — linked to what, and does the local configuration still
// describe it? So this reads the state for the counts and runs the same
// read-only comparison the full sync ends with.
func syncAlreadyConnected(ctx context.Context, envName string, env Env, envDir string, deps syncDeps) error {
	fmt.Printf("\n🔗 '%s' is linked: env/%s/.terraform is here, so terraform is already\n", envName, envName)
	fmt.Println("   talking to the backend.")

	c := stateConnection{
		Env: envName, Status: stateAlreadyInitialised, Generated: true,
		AWSProfile: env.AWSProfile, AccountID: env.AccountID,
	}

	if env.StateBucket != "" && env.StateFile != "" && env.Region != "" {
		lookupCtx, cancel := context.WithTimeout(ctx, remoteStateTimeout)
		defer cancel()

		summary, err := deps.lookup(lookupCtx, env)
		switch {
		case err == nil:
			c.Summary = summary
			fmt.Printf("\n   State file:  s3://%s/%s\n", summary.Bucket, summary.Key)
			fmt.Printf("   Tracked:     %d resources (%d managed, %d data sources)\n",
				summary.ResourceCount, summary.ManagedCount, summary.DataCount)
			if summary.Serial > 0 {
				fmt.Printf("   Serial:      %d\n", summary.Serial)
			}
		case errors.Is(err, errRemoteStateAbsent):
			fmt.Println("\n   The backend holds no state object yet, so this directory is")
			fmt.Println("   initialised but nothing has been applied through it.")
		default:
			// Informational only here: the plan below is the real check, and it
			// talks to the same backend. Do not fail the command on it.
			fmt.Printf("\n   (Could not read the state object for a resource count: %v)\n", err)
		}
	}

	outcome, err := performSync(deps.syncRequestFor(c, envDir))
	printSyncVerdict(outcome.Verdict)
	return err
}

func printSyncHeader(envName string, env Env, envDir string) {
	fmt.Printf("\n🔄 meroku sync — environment '%s'\n\n", envName)
	fmt.Printf("   Config:      %s.yaml\n", envName)
	if env.AWSProfile != "" {
		fmt.Printf("   Profile:     %s\n", env.AWSProfile)
	}
	if env.AccountID != "" {
		fmt.Printf("   Account:     %s\n", env.AccountID)
	}
	if env.Region != "" {
		fmt.Printf("   Region:      %s\n", env.Region)
	}
	if env.StateBucket != "" && env.StateFile != "" {
		fmt.Printf("   Backend:     s3://%s/%s\n", env.StateBucket, env.StateFile)
	} else {
		fmt.Printf("   Backend:     not configured in %s.yaml\n", envName)
	}
	if info, err := os.Stat(envDir); err == nil && info.IsDir() {
		fmt.Printf("   Directory:   %s (present)\n", envDir)
	} else {
		fmt.Printf("   Directory:   %s (missing)\n", envDir)
	}
	fmt.Println("\n   This command never applies, never destroys and never migrates state.")
}

// printSyncVerdict closes every run with one line the user can act on.
func printSyncVerdict(verdict string) {
	if verdict == "" {
		verdict = "No conclusion could be reached."
	}
	fmt.Printf("\n%s\n", strings.Repeat("─", 72))
	fmt.Printf("Verdict: %s\n", verdict)
}
