package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

// deployStepError says which step of a deploy failed before Terraform's own
// screens took over. Step doubles as HandoffContext.Operation, so the Claude
// Code briefing is named after it: setup, generate, migration, preflight,
// oidc, dns or init.
type deployStepError struct {
	Step string
	Err  error
}

func (e *deployStepError) Error() string {
	return fmt.Sprintf("%s failed: %v", deployStepLabel(e.Step), e.Err)
}

func (e *deployStepError) Unwrap() error { return e.Err }

// errDeployStopped is a deploy the operator chose to stop (cancelling the DNS
// setup screen, say). The reason has already been printed; it is not a
// failure, so it gets no error line and no handoff offer.
var errDeployStopped = errors.New("deploy stopped")

// stepErr tags err with step. An error already tagged keeps its tag: the
// innermost step is the one that knows what actually failed (a YAML that does
// not load inside generation is a migration failure, not a render failure).
func stepErr(step string, err error) error {
	if err == nil {
		return nil
	}
	var se *deployStepError
	if errors.As(err, &se) {
		return err
	}
	return &deployStepError{Step: step, Err: err}
}

func deployStepLabel(step string) string {
	switch step {
	case "setup":
		return "Deploy setup"
	case "generate":
		return "Terraform generation"
	case "migration":
		return "Loading and migrating the env YAML"
	case "preflight":
		return "AWS pre-flight check"
	case "oidc":
		return "GitHub OIDC check"
	case "dns":
		return "DNS pre-flight"
	case "init":
		return "Terraform init"
	default:
		return step
	}
}

// deployPreflight is the AWS pre-flight runCommandToDeploy runs before it
// touches Terraform. A variable so tests can fail it without reaching AWS.
var deployPreflight = AWSPreflightCheckContext

func deployHandoffGoal(step string) string {
	return fmt.Sprintf("meroku's deploy stopped at the %q step (%s) before Terraform planned anything. "+
		"Find out why, explain the cause, and propose a fix that follows the rules below.",
		step, deployStepLabel(step))
}

// deployInteractive is the menu's deploy: run it, and on a failure meroku
// itself detected, offer the Claude Code handoff. The project root is taken
// here, before runCommandToDeploy changes into env/<env>.
func deployInteractive(env string, in io.Reader, offer func(HandoffContext)) {
	root := deployProjectRoot(os.Getwd, os.Stdout)
	err := runCommandToDeploy(env)
	if err == nil {
		return
	}
	reportDeployFailure(err, handoffTarget{ProjectRoot: root, EnvName: env, Env: peekEnv(env)}, in, offer)
}

// deployProjectRoot is the working directory, or "" with the cause printed
// when it cannot be read: the handoff later fails on an empty project root,
// and that message alone would not say why.
func deployProjectRoot(getwd func() (string, error), out io.Writer) string {
	root, err := getwd()
	if err != nil {
		fmt.Fprintf(out, "⚠️  Could not read the current directory (the Claude Code handoff will be unavailable): %v\n", err)
		return ""
	}
	return root
}

// reportDeployFailure prints err and, for a failure in one of meroku's own
// deploy steps, offers the handoff. Plan and apply failures are not offered
// again: their TUI error screens already did, with Terraform's diagnostics.
func reportDeployFailure(err error, t handoffTarget, in io.Reader, offer func(HandoffContext)) {
	if errors.Is(err, errDeployStopped) {
		return
	}
	fmt.Printf("\n❌ %v\n", err)

	var se *deployStepError
	if !errors.As(err, &se) {
		return
	}

	fmt.Println()
	fmt.Println("  [a] Fix with AI (Claude Code)")
	fmt.Println("  [Enter] Continue")
	fmt.Print("\nChoice: ")
	line, _ := bufio.NewReader(in).ReadString('\n')
	if strings.ToLower(strings.TrimSpace(line)) != "a" {
		return
	}

	ctx := t.context(se.Step, deployHandoffGoal(se.Step))
	ctx.ErrorOutput = err.Error()
	offer(ctx)
}

// peekEnv reads the env YAML for the handoff's profile, region and account,
// without migrating or writing it: the deploy that just failed may have
// failed on exactly that. Fields that do not parse are left blank, and the
// briefing says they were not recorded.
func peekEnv(env string) Env {
	e, _ := peekEnvErr(env)
	return e
}

// peekEnvErr is peekEnv that also says why the YAML could not be read or
// parsed. The Env holds whatever did parse, even when err is set.
func peekEnvErr(env string) (Env, error) {
	var e Env
	path, err := resolveEnvFilePath(env)
	if err != nil {
		return e, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return e, err
	}
	if err := yaml.Unmarshal(data, &e); err != nil {
		return e, fmt.Errorf("parsing %s: %w", path, err)
	}
	return e, nil
}

// cliExitCode prints a non-interactive subcommand's error and returns the
// process exit status. Subcommands never prompt: they may run in CI.
func cliExitCode(err error) int {
	if err == nil {
		return 0
	}
	fmt.Printf("Error: %v\n", err)
	return 1
}
