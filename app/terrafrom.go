package main

import (
	"encoding/json"
	"errors"
	"fmt"
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

func terraformInitIfNeeded() {
	if _, err := os.Stat(".terraform"); os.IsNotExist(err) {
		_, err = terraformInit()
		if err != nil {
			fmt.Printf("Error initializing terraform: %v\n", err)
			os.Exit(1)
		}
	} else if err != nil {
		fmt.Printf("Error checking .terraform directory: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Terraform already initialized.")
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
	if strings.Contains(clean, "Error: Backend configuration changed") {
		return []string{"terraform", "init", "-reconfigure"}, nil
	} else if strings.Contains(clean, "Error: Backend initialization required: please run \"terraform init\"") ||
		strings.Contains(clean, "Backend initialization required, please run \"terraform init\"") ||
		strings.Contains(clean, "Reason: Backend configuration block has changed") ||
		strings.Contains(clean, "Reason: Initial configuration of the requested backend") ||
		strings.Contains(clean, "Error: Module not installed") {
		// Check if we need -reconfigure or -migrate-state flags
		if strings.Contains(clean, "Reason: Initial configuration of the requested backend") &&
			(strings.Contains(clean, "\"-reconfigure\"") || strings.Contains(clean, "\"-migrate-state\"")) {
			return []string{"terraform", "init", "-reconfigure"}, nil
		}
		return []string{"terraform", "init"}, nil
	}
	return []string{}, errors.New("unknown error, I could not check it. please provide the output to us–....")
}

func stripAnsiEscapeCodes(input string) string {
	// This regex matches ANSI escape codes
	re := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	return re.ReplaceAllString(input, "")
}
