package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/charmbracelet/huh"
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
	// Run plan and save to file
	errString, err := runCommandWithOutput("terraform", "plan", "-out=tfplan")
	if err != nil {
		slog.Error("error running terraform plan", "error", err, "output", errString)
		// trying to recover:
		var recoverErr error
		retryCount := 0
		maxRetries := 5
		var commands []string

		for recoverErr == nil && retryCount < maxRetries {
			commands, recoverErr = terraformError(errString)
			if recoverErr != nil {
				fmt.Println("🛑 terraform error recovery unable to handle output with error: ", recoverErr.Error())
				fmt.Println(errString)
				return fmt.Errorf("error checking terraform error: %w", recoverErr)
			}
			fmt.Printf("✳️ terraform error recovery attempt %d/%d suggests to run: %v\n", retryCount+1, maxRetries, commands)
			errString, err = runCommandWithOutput(commands[0], commands[1:]...)
			if err != nil {
				fmt.Printf("❌ Attempt %d failed. Error: %v\n", retryCount+1, err)
			} else {
				fmt.Printf("✅ Attempt %d succeeded.\n", retryCount+1)
				break
			}
			retryCount++
		}

		if err != nil {
			return fmt.Errorf("error running terraform command after %d attempts: %w", retryCount, err)
		}

		return runTerraformApply()
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

				// Update summary
				switch change.Change.Actions[0] {
				case "create":
					filteredPlan.Summary.Create++
				case "update":
					filteredPlan.Summary.Update++
				case "delete":
					filteredPlan.Summary.Delete++
				case "replace":
					filteredPlan.Summary.Replace++
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

	// Still save the full plan for reference
	err = os.WriteFile("terraform-plan-full.json", jsonOutput, 0o644)
	if err != nil {
		return fmt.Errorf("error saving plan JSON: %w", err)
	}
	fmt.Println("📄 Full plan saved to terraform-plan-full.json")

	// Skip the text formatting and go straight to interactive view
	fmt.Println("✅ Terraform plan generated successfully")

	// Show interactive TUI
	err = showModernTerraformPlanTUI(string(jsonOutput))
	if err != nil {
		return fmt.Errorf("error showing plan TUI: %w", err)
	}

	// Ask the user if they want to apply or return to the main menu
	result := false
	huh.NewConfirm().
		Title("Do you want to apply the Terraform changes?").
		Description("Select 'Yes' to apply, 'No' to return to the main menu.").
		Affirmative("Yes").
		Negative("No").
		Value(&result).
		Run()

	if !result {
		fmt.Println("Returning to main menu...")
		return nil
	}

	fmt.Println("Applying Terraform changes...")
	_, err = runCommandWithOutput("terraform", "apply", "tfplan")
	// Clean up the plan file
	os.Remove("tfplan")
	return err
}

func terraformError(output string) ([]string, error) {
	fmt.Println("Recovering from error ... ")

	clean := stripAnsiEscapeCodes(output)
	if strings.Contains(clean, "Error: Backend configuration changed") {
		return []string{"terraform", "init", "-reconfigure"}, nil
	} else if strings.Contains(clean, "Error: Backend initialization required: please run \"terraform init\"") ||
		strings.Contains(clean, "Reason: Backend configuration block has changed") ||
		strings.Contains(clean, "Error: Module not installed") {
		return []string{"terraform", "init"}, nil
	}
	return []string{}, errors.New("unknown error, I could not check it. please provide the output to us–....")
}

func stripAnsiEscapeCodes(input string) string {
	// This regex matches ANSI escape codes
	re := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	return re.ReplaceAllString(input, "")
}
