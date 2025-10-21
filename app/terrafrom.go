package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/charmbracelet/huh/spinner"
)

func terraformInit(flags ...string) (string, error) {
	var e error
	action := func() {
		cmd := exec.Command("terraform", append([]string{"init"}, flags...)...)
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

// ensureLambdaBootstrapExists checks if the lambda bootstrap file exists
// and creates a dummy one if it doesn't. The bootstrap file is gitignored,
// so it won't exist in project copies. This prevents terraform errors during
// destroy operations when the archive_file data source tries to create an archive.
func ensureLambdaBootstrapExists() {
	bootstrapPath := "infrastructure/modules/workloads/ci_lambda/bootstrap"

	// Check if file exists
	if _, err := os.Stat(bootstrapPath); os.IsNotExist(err) {
		// Create dummy bootstrap file with random content
		dummyContent := []byte("# Dummy bootstrap file created by meroku\n# This file is created to prevent terraform errors\n")

		// Create the file
		if err := os.WriteFile(bootstrapPath, dummyContent, 0755); err != nil {
			fmt.Printf("⚠️  Warning: Failed to create dummy bootstrap file: %v\n", err)
		} else {
			fmt.Println("✅ Created dummy lambda bootstrap file")
		}
	}
}

func terraformInitIfNeeded() {
	// Ensure lambda bootstrap file exists before running terraform
	ensureLambdaBootstrapExists()

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
	if envName != "" {
		env, err := loadEnvFromPath(envName)
		if err == nil && env.StateBucket != "" && env.Region != "" {
			fmt.Printf("\n🚀 Preparing environment: %s\n", envName)

			// CRITICAL: Run pre-flight checks with auto-recovery
			if err := AWSPreflightCheck(env); err != nil {
				fmt.Printf("\n%v\n\n", err)
				fmt.Println("❌ Pre-flight checks failed. Please fix the issues above and try again.")
				os.Exit(1)
			}
		}
	}

	if _, err := os.Stat(".terraform"); os.IsNotExist(err) {
		_, err = terraformInit()
		if err != nil {
			fmt.Printf("\n❌ Terraform initialization failed: %v\n", err)
			fmt.Println("\n💡 Recovery suggestions:")
			fmt.Println("• Run 'terraform init -reconfigure' manually")
			fmt.Println("• Check your AWS credentials: aws sts get-caller-identity")
			fmt.Println("• Verify S3 backend configuration in main.tf")
			os.Exit(1)
		}
	} else if err != nil {
		fmt.Printf("Error checking .terraform directory: %v\n", err)
		os.Exit(1)
	} else {
		fmt.Println("✅ Terraform already initialized.")
	}
}

func runTerraformApply() error {
	// Run the new progress TUI for terraform plan
	err := runTerraformPlanWithProgress()
	if err != nil {
		// Check if we can recover from the error
		errString := err.Error()
		var recoverErr error
		retryCount := 0
		maxRetries := 5
		var commands []string

		for recoverErr == nil && retryCount < maxRetries {
			commands, recoverErr = terraformError(errString)
			if recoverErr != nil {
				// Cannot recover, error was already shown in TUI
				return fmt.Errorf("terraform plan failed: %w", err)
			}
			fmt.Printf("✳️ terraform error recovery attempt %d/%d suggests to run: %v\n", retryCount+1, maxRetries, commands)
			recoveryOutput, err2 := runCommandWithOutput(commands[0], commands[1:]...)
			if err2 != nil {
				fmt.Printf("❌ Attempt %d failed. Error: %v\n", retryCount+1, err2)
				err = err2
				errString = recoveryOutput // Update errString for next iteration
			} else {
				fmt.Printf("✅ Attempt %d succeeded.\n", retryCount+1)
				// Retry the plan with progress TUI
				return runTerraformApply()
			}
			retryCount++
		}

		if err != nil {
			return fmt.Errorf("error running terraform command after %d attempts: %w", retryCount, err)
		}
	}

	// Parse and format the plan
	// Run terraform show to get JSON output
	cmd := exec.Command("terraform", "show", "-json", "tfplan")
	jsonOutput, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("error running terraform show: %w", err)
	}

	// Parse the JSON to filter only changes
	var fullPlan TerraformPlanVisual
	err = json.Unmarshal(jsonOutput, &fullPlan)
	if err == nil {
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

		// Save the filtered JSON
		filteredJSON, _ := json.MarshalIndent(filteredPlan, "", "  ")
		err = os.WriteFile("terraform-plan-changes.json", filteredJSON, 0o644)
		if err == nil {
			fmt.Printf("💾 Changes saved to terraform-plan-changes.json (%d resources)\n", filteredPlan.Summary.Total)
		}
	}

	// Skip the text formatting and go straight to interactive view
	fmt.Println("✅ Terraform plan generated successfully")

	// Show interactive TUI
	err = showModernTerraformPlanTUI(string(jsonOutput))
	if err != nil {
		return fmt.Errorf("error showing plan TUI: %w", err)
	}
	os.Remove("tfplan")
	fmt.Println("Returning to main menu...")
	return nil
}

func terraformError(output string) ([]string, error) {
	fmt.Println("Recovering from error ... ")

	clean := stripAnsiEscapeCodes(output)

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
			return []string{"terraform", "init", "-reconfigure"}, nil
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
				return []string{"terraform", "init", "-reconfigure"}, nil
			}
			return []string{"terraform", "init"}, nil
		}
	}

	// Check for archive creation errors (missing files for lambda, etc.)
	// These typically occur during destroy when lambda source files have been moved/deleted
	archiveErrorPatterns := []string{
		"Error: Archive creation error",
		"error creating archive",
		"could not archive missing file",
		"error archiving file",
	}

	for _, pattern := range archiveErrorPatterns {
		if strings.Contains(clean, pattern) {
			fmt.Println("\n⚠️  Detected missing file error for archive creation.")
			fmt.Println("💡 This usually means a lambda function references files that no longer exist.")
			fmt.Println("🔄 Attempting destroy with -refresh=false to skip validation...")

			// During destroy, we don't need to validate/create archives
			// Skip refresh phase which tries to read these files
			return []string{"terraform", "apply", "-destroy", "-refresh=false", "-auto-approve"}, nil
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

func runTerraformDestroy() error {
	return runTerraformDestroyWithRetry(0)
}

func runTerraformDestroyWithRetry(retryCount int) error {
	const maxRetries = 3

	fmt.Println("\n🔍 Running terraform plan -destroy to preview changes...")

	// First, run terraform plan -destroy to show what will be destroyed
	// Use -refresh=false to skip data source reads and prevent errors from already-deleted resources
	// Use a combined output buffer to capture errors for recovery
	var stderrBuf bytes.Buffer
	cmd := exec.Command("terraform", "plan", "-destroy", "-out=destroy.tfplan", "-refresh=false")
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

	if err := cmd.Run(); err != nil {
		// Try to recover from errors
		stderrOutput := stderrBuf.String()

		// Check if this is a recoverable error
		if retryCount < maxRetries {
			commands, recoverErr := terraformError(stderrOutput)
			if recoverErr == nil {
				fmt.Printf("\n✳️ Detected recoverable error. Running recovery command (attempt %d/%d): %v\n", retryCount+1, maxRetries, commands)

				// Run the recovery command
				recoveryOutput, err2 := runCommandWithOutput(commands[0], commands[1:]...)
				if err2 != nil {
					fmt.Printf("❌ Recovery attempt %d failed: %v\n", retryCount+1, err2)
					fmt.Printf("Output: %s\n", recoveryOutput)
				} else {
					fmt.Printf("✅ Recovery successful. Retrying destroy operation...\n\n")
					// Retry the destroy operation
					return runTerraformDestroyWithRetry(retryCount + 1)
				}
			}
		}

		return fmt.Errorf("terraform plan -destroy failed: %w", err)
	}

	fmt.Println("\n📊 Destroy plan created successfully.")
	fmt.Println("🔥 Proceeding with destruction...")

	// Now run terraform apply with the destroy plan
	// Use -auto-approve since we already confirmed multiple times
	destroyCmd := exec.Command("terraform", "apply", "-auto-approve", "destroy.tfplan")
	destroyCmd.Stdout = os.Stdout
	destroyCmd.Stderr = os.Stderr

	if err := destroyCmd.Run(); err != nil {
		// Try to recover from errors
		errString := err.Error()
		var recoverErr error
		retryCount := 0
		maxRetries := 3
		var commands []string

		for recoverErr == nil && retryCount < maxRetries {
			commands, recoverErr = terraformError(errString)
			if recoverErr != nil {
				return fmt.Errorf("terraform destroy failed: %w", err)
			}
			fmt.Printf("✳️ terraform error recovery attempt %d/%d suggests to run: %v\n", retryCount+1, maxRetries, commands)
			recoveryOutput, err2 := runCommandWithOutput(commands[0], commands[1:]...)
			if err2 != nil {
				fmt.Printf("❌ Attempt %d failed. Error: %v\n", retryCount+1, err2)
				errString = recoveryOutput
			} else {
				fmt.Printf("✅ Attempt %d succeeded. Retrying destroy...\n", retryCount+1)
				return runTerraformDestroy()
			}
			retryCount++
		}

		return fmt.Errorf("terraform destroy failed after %d recovery attempts: %w", retryCount, err)
	}

	// Clean up the destroy plan file
	os.Remove("destroy.tfplan")

	return nil
}
