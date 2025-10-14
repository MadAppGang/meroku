package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/samber/lo"
)

func nukeMenu() {
	var env string

	// Use already selected environment if available
	if selectedEnvironment != "" {
		env = selectedEnvironment
		fmt.Printf("\n⚠️  WARNING: You are about to DESTROY environment: %s\n", env)
	} else {
		// Only prompt for environment selection if none is selected
		envs, err := findFilesWithExts([]string{".yaml", ".yml"})
		if err != nil {
			panic(err)
		}
		// Filter out DNS config file
		var filteredEnvs []string
		for _, envFile := range envs {
			if envFile != "dns.yaml" {
				filteredEnvs = append(filteredEnvs, envFile)
			}
		}
		options := lo.Map(filteredEnvs, func(s string, _ int) huh.Option[string] {
			return huh.NewOption(fmt.Sprintf("💥 Destroy %s environment", s), s)
		})
		options = append(options, huh.NewOption("⬅️  Back to main menu", "go:back"))

		huh.NewSelect[string]().
			Title("⚠️  Select an environment to DESTROY").
			Description("This action will DELETE all infrastructure resources!").
			Options(
				options...,
			).
			Value(&env).
			Run()

		switch env {
		case "go:back":
			return
		case "":
			fmt.Println("No environment selected")
			return
		}
	}

	// Load environment to get project name
	e, err := loadEnv(env)
	if err != nil {
		fmt.Printf("❌ Error loading environment: %v\n", err)
		return
	}

	// Show version and environment details
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Printf("🤖 Meroku Version: %s\n", version)
	fmt.Printf("🔥 Environment: %s\n", env)
	fmt.Printf("📦 Project: %s\n", e.Project)
	fmt.Printf("☁️  Region: %s\n", e.Region)
	if e.AccountID != "" {
		fmt.Printf("🔑 AWS Account: %s\n", e.AccountID)
	}
	fmt.Println(strings.Repeat("=", 60))

	// First confirmation: Yes/No
	firstConfirm := false
	huh.NewConfirm().
		Title(fmt.Sprintf("⚠️  Are you ABSOLUTELY SURE you want to destroy environment '%s'?", env)).
		Description("This will DELETE all AWS resources for this environment!").
		Affirmative("Yes, I understand").
		Negative("No, cancel").
		Value(&firstConfirm).
		Run()

	if !firstConfirm {
		fmt.Println("✅ Cancelled. No resources were destroyed.")
		return
	}

	// Second confirmation: Type project name
	var projectNameInput string
	huh.NewInput().
		Title(fmt.Sprintf("⚠️  Type the project name '%s' to confirm destruction", e.Project)).
		Description("This is your final confirmation before destroying all resources.").
		Value(&projectNameInput).
		Validate(func(s string) error {
			if s != e.Project {
				return fmt.Errorf("project name does not match (expected: %s)", e.Project)
			}
			return nil
		}).
		Run()

	if projectNameInput != e.Project {
		fmt.Println("✅ Cancelled. Project name did not match.")
		return
	}

	// Final confirmation with exact phrase
	var finalConfirm string
	confirmPhrase := "destroy everything"
	huh.NewInput().
		Title(fmt.Sprintf("⚠️  Type '%s' to proceed with destruction", confirmPhrase)).
		Description("This is the FINAL step. All resources will be permanently deleted.").
		Value(&finalConfirm).
		Validate(func(s string) error {
			if strings.ToLower(strings.TrimSpace(s)) != confirmPhrase {
				return fmt.Errorf("confirmation phrase does not match (expected: %s)", confirmPhrase)
			}
			return nil
		}).
		Run()

	if strings.ToLower(strings.TrimSpace(finalConfirm)) != confirmPhrase {
		fmt.Println("✅ Cancelled. Confirmation phrase did not match.")
		return
	}

	// All confirmations passed, proceed with destruction
	fmt.Println("\n🔥 Starting infrastructure destruction...")
	fmt.Println("📝 This may take several minutes depending on your infrastructure size.")

	if err := runCommandToNuke(env); err != nil {
		fmt.Printf("\n❌ Error during destruction: %v\n", err)
		fmt.Println("⚠️  Some resources may have been destroyed. Check your AWS console.")
		return
	}

	fmt.Println("\n✅ Environment destroyed successfully!")
	fmt.Printf("💡 The configuration file '%s.yaml' has been preserved.\n", env)
	fmt.Println("💡 You can redeploy this environment later if needed.")
}

func runCommandToNuke(env string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("error getting current working directory: %w", err)
	}
	defer os.Chdir(wd)

	// Check if env directory exists
	envPath := filepath.Join("env", env)
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		return fmt.Errorf("environment directory not found: %s (has this environment been deployed?)", envPath)
	}

	// Change to environment directory
	err = os.Chdir(envPath)
	if err != nil {
		return fmt.Errorf("error changing directory to env folder: %w", err)
	}

	// Ensure terraform is initialized
	terraformInitIfNeeded()

	// Run terraform destroy
	return runTerraformDestroy()
}
