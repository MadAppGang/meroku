package main

import (
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
	errString, err := runCommandWithOutput("terraform", "plan")
	fmt.Println("errString", errString)
	fmt.Println("err", err)
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

	fmt.Println("✅ Terraform plan completed successfully.")
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
	_, err = runCommandWithOutput("terraform", "apply", "-auto-approve")
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
