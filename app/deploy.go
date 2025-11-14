package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aymerick/raymond"
	"github.com/charmbracelet/huh"
	"github.com/samber/lo"
)

func deployMenu() {
	var env string

	// Use already selected environment if available
	if selectedEnvironment != "" {
		env = selectedEnvironment
		fmt.Printf("Deploying environment: %s\n", env)
	} else {
		// Only prompt for environment selection if none is selected
		envs, err := findFilesWithExts([]string{".yaml", ".yml"})
		if err != nil {
			panic(err)
		}
		// Filter out DNS config file
		var filteredEnvs []string
		for _, env := range envs {
			if env != "dns.yaml" {
				filteredEnvs = append(filteredEnvs, env)
			}
		}
		options := lo.Map(filteredEnvs, func(s string, _ int) huh.Option[string] {
			return huh.NewOption(fmt.Sprintf("Deploy %s environment", s), s)
		})
		options = append(options, huh.NewOption("Back to main menu", "go:back"))

		huh.NewSelect[string]().
			Title("Select an environment to deploy").
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
			os.Exit(1)
		}
	}
	
	runCommandToDeploy(env)
}

func runCommandToDeploy(env string) error {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting current working directory:", err)
		os.Exit(1)
	}
	defer os.Chdir(wd)

	createFolderIfNotExists("env")
	err = createFolderIfNotExists(filepath.Join("env", env))
	if err != nil {
		fmt.Println("Error creating folder for environment:", err)
		os.Exit(1)
	}
	//
	applyTemplate(env)
	buildDeploymentLambda(env)

	e, err := loadEnv(env)
	if err != nil {
		fmt.Println("Error loading environment:", err)
		os.Exit(1)
	}

	// Run comprehensive AWS pre-flight checks BEFORE changing directory
	// This validates credentials, checks/creates S3 bucket, and handles SSO refresh
	fmt.Printf("\n🚀 Starting deployment for environment: %s\n", env)
	if err := AWSPreflightCheck(e); err != nil {
		fmt.Printf("\n%v\n\n", err)
		fmt.Println("❌ Pre-flight checks failed. Please fix the issues above and try again.")
		os.Exit(1)
	}

	err = os.Chdir(filepath.Join("env", env))
	if err != nil {
		fmt.Println("Error changing directory to env folder:", err)
		os.Exit(1)
	}
	terraformInitIfNeeded()
	return runTerraformApply()
}

func handleGenerateCommand(args []string) {
	// Register custom Handlebars helpers
	registerCustomHelpers()

	if len(args) == 0 {
		fmt.Println("Usage: meroku generate <environment>")
		fmt.Println("Example: meroku generate dev")
		fmt.Println("")
		fmt.Println("Generates Terraform configuration files from YAML templates.")
		os.Exit(1)
	}

	env := args[0]
	fmt.Printf("Generating Terraform configuration for environment: %s\n", env)

	// Check if environment file exists
	envFile := env + ".yaml"
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		fmt.Printf("Error: Environment file '%s' not found\n", envFile)
		os.Exit(1)
	}

	// Create env directory structure
	createFolderIfNotExists("env")
	if err := createFolderIfNotExists(filepath.Join("env", env)); err != nil {
		fmt.Printf("Error creating environment directory: %v\n", err)
		os.Exit(1)
	}

	// Generate template
	applyTemplate(env)

	fmt.Printf("✓ Generated: env/%s/main.tf\n", env)
}

func applyTemplate(env string) {
	// Read the template file
	templateContent, err := os.ReadFile(filepath.Join("infrastructure", "env", "main.hbs"))
	if err != nil {
		fmt.Printf("error reading template file: %v", err)
		os.Exit(1)
	}

	envMap, err := loadEnvToMap(env + ".yaml")
	if err != nil {
		fmt.Printf("error loading environment: %v", err)
		os.Exit(1)
	}
	envMap["modules"] = "../../infrastructure/modules"
	envMap["custom_modules"] = "../../custom"
	if err != nil {
		fmt.Printf("error loading environment: %v", err)
		os.Exit(1)
	}
	// Create a new template and parse the content
	tmpl, err := raymond.Parse(string(templateContent))
	if err != nil {
		fmt.Printf("error parsing template: %v", err)
		os.Exit(1)
	}
	// Execute the template with the environment data
	result, err := tmpl.Exec(envMap)
	if err != nil {
		fmt.Printf("Error executing template: %+v\n", err)
		os.Exit(1)
	}

	os.WriteFile(filepath.Join("env", env, "main.tf"), []byte(result), 0o644)
}

func buildDeploymentLambda(env string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("error getting current working directory: %w", err)
	}
	defer os.Chdir(wd)

	os.RemoveAll(filepath.Join("env", env, "ci_lambda.zip"))
	os.Chdir("infrastructure/modules/workloads/ci_lambda")
	os.RemoveAll("bootstrap")

	os.Setenv("GOOS", "linux")
	os.Setenv("GOARCH", "amd64")
	if _, err := runCommandWithOutput("go", "build", "-o", "bootstrap", "."); err != nil {
		return fmt.Errorf("error building deployment lambda: %w", err)
	}

	return nil
}
