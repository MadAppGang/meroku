package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/huh/spinner"
)

func terraformInit(flags ...string) (string, error) {
	var e error
	action := func() {
		cmd, err := newTerraformCmd(context.Background(), tfState, workingDir(), append([]string{"init"}, flags...)...)
		if err != nil {
			e = fmt.Errorf("error initializing terraform: %w", err)
			return
		}
		output, err := cmd.CombinedOutput()
		if err != nil {
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				fmt.Println(strings.TrimSpace(line))
			}
			e = fmt.Errorf("error initializing terraform: %w", err)
		}
	}
	_ = spinner.New().Title("Initializing terraform for your environment...").Action(action).Run()
	if e != nil {
		return "", e
	}
	fmt.Println("✅ Terraform initialized successfully.")
	return "", nil
}

// terraformInitIfNeeded runs the pre-flight and `terraform init` in env/<env>.
// Failures come back tagged "preflight" or "init" for the deploy to report.
func terraformInitIfNeeded() error {
	// Get the environment from selectedEnvironment global variable or current directory
	envName := selectedEnvironment
	if envName == "" {
		// Try to extract environment name from current working directory
		// Expected format: .../env/dev or .../env/prod
		wd, err := os.Getwd()
		if err == nil {
			parts := strings.Split(wd, string(os.PathSeparator))
			if len(parts) >= 2 && parts[len(parts)-2] == "env" {
				envName = parts[len(parts)-1]
			}
		}
	}

	// Run comprehensive AWS pre-flight checks before terraform init
	var envConfig Env
	haveEnvConfig := false
	if envName != "" {
		env, err := loadEnvFromPath(envName)
		if err == nil && env.StateBucket != "" && env.Region != "" {
			envConfig, haveEnvConfig = env, true
			fmt.Printf("\n🚀 Preparing environment: %s\n", envName)

			// CRITICAL: Run pre-flight checks with auto-recovery
			if err := AWSPreflightCheck(env); err != nil {
				return stepErr("preflight", err)
			}
		}
	}

	if _, err := os.Stat(".terraform"); os.IsNotExist(err) {
		// About to initialise. If the backend already holds a deployment, say so
		// first — this is the moment someone whose env/ directory went missing is
		// wondering whether their infrastructure went with it. Announce only: the
		// init below does the reconnecting.
		if haveEnvConfig {
			announceStateConnection(context.Background(), envName, envConfig, ".")
		}
		_, err = terraformInit()
		if err != nil {
			fmt.Printf("\n❌ Terraform initialization failed: %v\n", err)
			fmt.Println("\n💡 Recovery suggestions:")
			fmt.Println("• Run 'terraform init -reconfigure' manually")
			fmt.Println("• Check your AWS credentials: aws sts get-caller-identity")
			fmt.Println("• Verify S3 backend configuration in main.tf")
			return stepErr("init", err)
		}
	} else if err != nil {
		return stepErr("init", fmt.Errorf("checking the .terraform directory: %w", err))
	} else {
		fmt.Println("✅ Terraform already initialized.")
		if err := upgradeLockedProviderIfBehind(); err != nil {
			return stepErr("init", err)
		}
	}
	return nil
}

// errHandedOff marks a failure the user handed to Claude Code with [a].
// meroku then leaves the environment alone: no init/-reconfigure recovery or
// re-plan racing Claude for the state lock and .terraform/.
var errHandedOff = errors.New("handed off to Claude Code")

// errApplyFailed is an apply that ran and failed. Its error screen has
// already shown Terraform's diagnostics and offered the handoff, so callers
// stop without printing it again.
var errApplyFailed = errors.New("terraform apply failed")

// applyPlanStep and applyPlanRecovery are the plan and the recovery-command
// lookup runTerraformApply uses. Variables so tests can drive the flow without
// running Terraform.
var (
	applyPlanStep     = runTerraformPlanWithProgress
	applyPlanRecovery = terraformError
)

// runTerraformApply plans and applies the environment in the current
// directory (env/<env>). target names that environment and the project root;
// it is captured before the chdir and only feeds the Claude Code handoff.
func runTerraformApply(target handoffTarget) error {
	return runTerraformApplyWithOptions(target, applyRunOptions{})
}

// applyRunOptions shape one plan-and-apply. Zero is today's untargeted run
// on tfplan. Title names the run on both screens.
type applyRunOptions struct {
	Targets []string
	Title   string
}

// runTerraformApplyWithOptions is runTerraformApply limited to Targets and
// titled Title. In a Neon dir [r] is hidden, a changed role search_path is
// explained after the apply, and a role Postgres could not drop gets its
// remedy printed.
func runTerraformApplyWithOptions(target handoffTarget, opts applyRunOptions) error {
	dir := workingDir()
	// The plan file can hold provider-stored secrets: it never outlives this
	// call, whatever path the call returns by.
	defer os.Remove(filepath.Join(dir, "tfplan"))

	planStep := applyPlanStep
	if len(opts.Targets) > 0 || opts.Title != "" {
		planStep = func(t handoffTarget) error {
			return runTerraformPlanWithOptions(t, planStepOptions{Targets: opts.Targets, Title: opts.Title, Dir: dir})
		}
	}

	// Run the new progress TUI for terraform plan
	err := planStep(target)
	if errors.Is(err, errHandedOff) {
		// The user chose Claude Code over automatic recovery.
		return err
	}
	if err != nil {
		// Check if we can recover from the error
		errString := err.Error()
		var recoverErr error
		retryCount := 0
		maxRetries := 5
		var commands []string

		for recoverErr == nil && retryCount < maxRetries {
			commands, recoverErr = applyPlanRecovery(errString)
			if recoverErr != nil {
				// Cannot recover, error was already shown in TUI
				return fmt.Errorf("terraform plan failed: %w", err)
			}
			fmt.Printf("✳️ terraform error recovery attempt %d/%d suggests to run: terraform %s\n", retryCount+1, maxRetries, strings.Join(commands, " "))
			recoveryOutput, err2 := runTerraformRecovery(dir, commands)
			if err2 != nil {
				fmt.Printf("❌ Attempt %d failed. Error: %v\n", retryCount+1, err2)
				err = err2
				errString = recoveryOutput // Update errString for next iteration
			} else {
				fmt.Printf("✅ Attempt %d succeeded.\n", retryCount+1)
				// Retry the plan with progress TUI
				return runTerraformApplyWithOptions(target, opts)
			}
			retryCount++
		}

		if err != nil {
			return fmt.Errorf("error running terraform command after %d attempts: %w", retryCount, err)
		}
	}

	// Parse and format the plan
	// Run terraform show to get JSON output
	cmd, err := newTerraformCmd(context.Background(), tfState, dir, "show", "-json", "tfplan")
	if err != nil {
		return fmt.Errorf("error running terraform show: %w", err)
	}
	jsonOutput, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("error running terraform show: %w", err)
	}

	// Parse the JSON to filter only changes
	var fullPlan TerraformPlanVisual
	err = json.Unmarshal(jsonOutput, &fullPlan)
	// rawChanges keeps the values as Terraform reported them, for checks that
	// read attributes after the plan screen. maskPlanSensitive replaces masked
	// maps with copies, so it never reaches into these.
	var rawChanges []ResourceChange
	if err == nil {
		rawChanges = append([]ResourceChange(nil), fullPlan.ResourceChanges...)
		for i := range fullPlan.ResourceChanges {
			maskPlanSensitive(&fullPlan.ResourceChanges[i])
		}

		// Create a filtered version with only changes
		filteredPlan := struct {
			TerraformVersion string           `json:"terraform_version"`
			ResourceChanges  []ResourceChange `json:"resource_changes"`
			Summary          struct {
				Total   int `json:"total"`
				Create  int `json:"create"`
				Update  int `json:"update"`
				Delete  int `json:"delete"`
				Replace int `json:"replace"`
			} `json:"summary"`
		}{
			TerraformVersion: fullPlan.TerraformVersion,
		}

		// Filter only actual changes
		for _, change := range fullPlan.ResourceChanges {
			if len(change.Change.Actions) > 0 &&
				change.Change.Actions[0] != "no-op" &&
				change.Change.Actions[0] != "read" {
				filteredPlan.ResourceChanges = append(filteredPlan.ResourceChanges, change)

				// Update summary - handle replace operations specially
				// Replace operations have ["delete", "create"] actions
				if len(change.Change.Actions) == 2 &&
					change.Change.Actions[0] == "delete" &&
					change.Change.Actions[1] == "create" {
					// This is a replace operation
					filteredPlan.Summary.Replace++
					filteredPlan.Summary.Delete++
					filteredPlan.Summary.Create++
				} else {
					// Single action
					switch change.Change.Actions[0] {
					case "create":
						filteredPlan.Summary.Create++
					case "update":
						filteredPlan.Summary.Update++
					case "delete":
						filteredPlan.Summary.Delete++
					}
				}
			}
		}
		filteredPlan.Summary.Total = len(filteredPlan.ResourceChanges)

		// Save the filtered JSON: sensitive values already masked, registered
		// secrets redacted, readable only by the user.
		filteredJSON, _ := json.MarshalIndent(filteredPlan, "", "  ")
		err = writePrivateFile(filepath.Join(dir, "terraform-plan-changes.json"), []byte(redactSecrets(string(filteredJSON))))
		if err == nil {
			fmt.Printf("💾 Changes saved to terraform-plan-changes.json (%d resources)\n", filteredPlan.Summary.Total)
		}
	}

	// Skip the text formatting and go straight to interactive view
	fmt.Println("✅ Terraform plan generated successfully")

	// Show interactive TUI
	result, err := showModernTerraformPlanTUIWithOptions(string(jsonOutput), target, planTUIOptions{
		Targets:      opts.Targets,
		AllowReplace: !isNeonDir(dir),
		Title:        opts.Title,
	})
	if err != nil {
		return fmt.Errorf("error showing plan TUI: %w", err)
	}
	ownerRole := ""
	if n := normalizeNeon(target.Env.Neon); n != nil {
		ownerRole = n.OwnerRole
	}
	if result.Applied {
		if notice := neonSearchPathNotice(neonSearchPathChanges(rawChanges), ownerRole); notice != "" {
			fmt.Println()
			fmt.Print(notice)
		}
	}
	if result.Failed {
		if remedy := neonRoleInUseRemedy(result.FailureOutput, ownerRole); remedy != "" {
			fmt.Println()
			fmt.Print(remedy)
		}
	}
	fmt.Println("Returning to main menu...")
	if result.Failed {
		return errApplyFailed
	}
	return nil
}

// runTerraformRecovery runs the terraform arguments terraformError suggested
// in dir. In a Neon dir, newTerraformCmd refuses init -upgrade unless the
// Neon providers are pinned to their exact releases.
func runTerraformRecovery(dir string, args []string) (string, error) {
	cmd, err := newTerraformCmd(context.Background(), tfState, dir, args...)
	if err != nil {
		return err.Error(), err
	}
	return runCmdWithOutput(cmd)
}

// writePrivateFile writes data to path with mode 0600, also tightening the
// mode of a file an earlier version wrote world-readable.
func writePrivateFile(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

// terraformError maps a failed command's output to the terraform arguments
// that recover from it (no "terraform" element), or an error when there is no
// known recovery. It only ever suggests init variants: never apply, plan or
// destroy.
func terraformError(output string) ([]string, error) {
	fmt.Println("Recovering from error ... ")

	clean := stripAnsiEscapeCodes(output)

	// A lock file that pins an older provider version than main.tf allows.
	// Only -upgrade may select a version outside the lock, and the message
	// also contains "terraform init", which the patterns below would answer
	// with a plain init that fails the same way.
	if strings.Contains(clean, "does not match configured version constraint") ||
		strings.Contains(clean, "terraform init -upgrade") {
		return []string{"init", "-upgrade", "-input=false"}, nil
	}

	// Check for errors that require terraform init -reconfigure
	reconfigurePatterns := []string{
		"Error: Backend configuration changed",
		"backend configuration changed",
		"Backend configuration has changed",
		"backend type changed",
		"Backend type has changed",
		"-reconfigure", // Terraform suggests using -reconfigure
		"terraform init -reconfigure",
		"run \"terraform init -reconfigure\"",
	}

	for _, pattern := range reconfigurePatterns {
		if strings.Contains(clean, pattern) {
			return []string{"init", "-reconfigure"}, nil
		}
	}

	// Check for errors that require standard terraform init
	initPatterns := []string{
		"Error: Backend initialization required: please run \"terraform init\"",
		"Backend initialization required, please run \"terraform init\"",
		"Backend initialization required",
		"Reason: Backend configuration block has changed",
		"Reason: Initial configuration of the requested backend",
		"Error: Module not installed",
		"terraform init",
		"run \"terraform init\"",
		"Terraform has been successfully initialized",
		"Backend has not been initialized",
		"No backend is configured",
		"Error: Could not load plugin",
		"Provider requirements cannot be satisfied",
		"Required plugins are not installed",
		"terraform providers lock",
	}

	for _, pattern := range initPatterns {
		if strings.Contains(clean, pattern) {
			// Check if terraform suggests -reconfigure specifically
			if strings.Contains(clean, "\"-reconfigure\"") || strings.Contains(clean, "\"-migrate-state\"") {
				return []string{"init", "-reconfigure"}, nil
			}
			return []string{"init"}, nil
		}
	}

	// Check for inline_policy deprecation warnings (can be safely ignored during destroy)
	if strings.Contains(clean, "inline_policy is deprecated") {
		fmt.Println("⚠️  Detected deprecation warning (safe to ignore during destroy)")
		// Return an empty error to indicate we should continue without recovery
		return []string{}, errors.New("deprecation warning, continuing without recovery")
	}

	return []string{}, errors.New("unknown error, unable to analyze. Please provide the output to us for debugging")
}

func stripAnsiEscapeCodes(input string) string {
	// This regex matches ANSI escape codes
	re := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	return re.ReplaceAllString(input, "")
}
