package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
)

func terraformError(err error, output string) ([]string, error) {
	// check different error types and run the specific command to fix them
	return []string{}, nil
}

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
	if err != nil {
		slog.Error("error running terraform plan", "error", err, "output", errString)
		if strings.Contains(errString, "Backend initialization required: please run \"terraform init\"") {
			runInit := false
			huh.NewForm(
				huh.NewGroup(
					huh.NewNote().
						Title("Terraform init required").
						Description("Terraform init is required to run this command. Do you want to run it now?"),
					huh.NewConfirm().
						Title("Do you want to run terraform init?").
						Value(&runInit),
				),
			).Run()
			if runInit {
				_, err = terraformInit()
				if err != nil {
					return fmt.Errorf("error initializing terraform, you need to run it manually: %w", err)
				}
				return runTerraformApply()
			}
			return nil
		}
		return err
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
