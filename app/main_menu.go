package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
)

// returns env to edit
func mainMenu() string {
	// check if project is init
	initProjectIfNeeded()

	// Show current environment and profile in the menu
	menuTitle := "Select an action"
	if selectedEnvironment != "" && selectedAWSProfile != "" {
		menuTitle = fmt.Sprintf("Select an action (Environment: %s | AWS Profile: %s)", selectedEnvironment, selectedAWSProfile)
	} else if selectedEnvironment != "" {
		menuTitle = fmt.Sprintf("Select an action (Environment: %s)", selectedEnvironment)
	}
	
	options := []huh.Option[string]{
		huh.NewOption("🌐 Edit environment with web UI", "api"),
		huh.NewOption("🚀 Deploy environment", "deploy"),
		huh.NewOption("✨ Create new environment", "create"),
		huh.NewOption("🔄 Change Environment", "change-env"),
		huh.NewOption("💥 Nuke/Destroy Environment", "nuke"),
		huh.NewOption("🤖 AI Agent - Troubleshoot Issues", "ai-agent"),
		huh.NewOption("🔐 AWS SSO Tools", "sso-menu"),
		huh.NewOption("🔍 Check for updates", "update"),
		huh.NewOption("👋 Exit", "exit"),
	}

	action := ""

	err := huh.NewSelect[string]().
		Title(menuTitle).
		Options(
			options...,
		).
		Value(&action).
		Run()

	// Handle Ctrl+C or other interrupts
	if err != nil {
		fmt.Println("\nExiting...")
		os.Exit(0)
	}

	switch {
	case strings.HasPrefix(action, "env:"):
		return strings.TrimPrefix(action, "env:")
	case action == "create":
		return createEnvMenu()
	case action == "deploy":
		deployMenu()
		return mainMenu()
	case action == "nuke":
		nukeMenu()
		return mainMenu()
	case action == "update":
		err := updateInfrastructure()
		if err != nil {
			fmt.Println("Error updating infrastructure:", err)
			os.Exit(1)
		}
		return mainMenu()
	case action == "api":
		startSPAServer("8080")
		return mainMenu()
	case action == "change-env":
		// Change environment
		err := selectEnvironment()
		if err != nil {
			fmt.Printf("Error selecting environment: %v\n", err)
		}
		return mainMenu()
	case action == "ai-agent":
		// Run AI agent for troubleshooting
		offerAIAgentFromMenu()
		return mainMenu()
	case action == "sso-menu":
		// Open SSO tools submenu
		ssoToolsMenu()
		return mainMenu()
	case action == "exit":
		os.Exit(0)
	}
	return ""
}

// ssoToolsMenu shows the AWS SSO tools submenu
func ssoToolsMenu() {
	var action string
	err := huh.NewSelect[string]().
		Title("AWS SSO Tools").
		Options(
			huh.NewOption("🔐 SSO Setup Wizard", "wizard"),
			huh.NewOption("🤖 SSO AI Agent (Enhanced)", "agent"),
			huh.NewOption("✓ Validate Configuration", "validate"),
			huh.NewOption("← Back to Main Menu", "back"),
		).
		Value(&action).
		Run()

	if err != nil {
		return
	}

	switch action {
	case "wizard":
		runSSOWizardFromMenu()
		ssoToolsMenu() // Return to SSO menu
	case "agent":
		runEnhancedSSOAgentFromMenu()
		ssoToolsMenu() // Return to SSO menu
	case "validate":
		validateAWSFromMenu()
		ssoToolsMenu() // Return to SSO menu
	case "back":
		return
	}
}

func createEnvMenu() string {
	projectName := getProjectName()

	var name string
	err := huh.NewInput().
		Title("What is the name of the environment?").
		Value(&name).
		Run()

	// Handle Ctrl+C or other interrupts
	if err != nil {
		fmt.Println("\nCancelled")
		return ""
	}

	r := regexp.MustCompile(`^[a-z]{2,}$`)
	if !r.MatchString(name) {
		fmt.Println("Invalid environment name")
		fmt.Println("minimum 2 characters, all lowercases, only letters from a-z")
		return createEnvMenu()
	}

	envs, _ := findFilesWithExts([]string{".yaml", ".yml"})
	if slices.Contains(envs, name+".yaml") || slices.Contains(envs, name+".yml") {
		fmt.Println("Environment already exists, try another name")
		return createEnvMenu()
	}

	e := createEnv(projectName, name)

	// Save to current directory
	err = saveEnvToFile(e, name+".yaml")
	if err != nil {
		fmt.Println("Error saving environment:", err)
		os.Exit(1)
	}
	
	fmt.Printf("Environment '%s' created successfully.\n", name)
	return name
}

// we are getting the project name from the first yaml file in the current directory
// if there is no yaml file, we are asking user to input the project name
func getProjectName() string {
	envs, _ := findFilesWithExts([]string{".yaml", ".yml"})
	if len(envs) > 0 {
		e, err := loadEnv(envs[0])
		if err == nil {
			return e.Project
		}
	}

	var name string
	err := huh.NewInput().
		Title("What is the project name?").
		Value(&name).
		Run()

	// Handle Ctrl+C or other interrupts
	if err != nil {
		fmt.Println("\nCancelled")
		os.Exit(0)
	}

	return name
}

func initProject() {
	os.RemoveAll("./infrastructure/")
	cmd := exec.Command("git", "clone", "--depth=1", "--branch=main", "https://github.com/MadAppGang/infrastructure.git", "./infrastructure")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Error cloning infrastructure:", output)
		os.Exit(1)
	}
	os.RemoveAll("./infrastructure/.git")
}

func initProjectIfNeeded() {
	if _, err := os.Stat("infrastructure"); os.IsNotExist(err) {
		answer := false
		huh.NewConfirm().
			Title("The project is not initialized, do you want to initialize it?").
			Affirmative("Yes 🚀").
			Negative("No 🤷‍♂️").
			Value(&answer).
			Run()
		if !answer {
			fmt.Println("Aborting, 👋!")
			os.Exit(1)
		}

		initProject()

		_ = spinner.New().Title("Initializing the project...").Action(initProject).Run()
	}
}

// runSSOWizardFromMenu runs the AWS SSO Setup Wizard
func runSSOWizardFromMenu() {
	// Get environment selection
	envName, _, err := selectEnvironmentForSSO()
	if err != nil {
		fmt.Printf("Error selecting environment: %v\n", err)
		return
	}

	// Load YAML
	yamlEnv, err := loadEnv(envName)
	if err != nil {
		fmt.Printf("Error loading environment config: %v\n", err)
		return
	}

	// Determine profile name
	profileName := yamlEnv.AWSProfile
	if profileName == "" {
		profileName = envName
	}

	// Run wizard
	if err := RunSSOWizard(profileName, &yamlEnv); err != nil {
		fmt.Printf("Wizard error: %v\n", err)
	}
}

// runSSOAgentFromMenu runs the AWS SSO AI Agent
func runSSOAgentFromMenu() {
	// Get environment selection
	envName, _, err := selectEnvironmentForSSO()
	if err != nil {
		fmt.Printf("Error selecting environment: %v\n", err)
		return
	}

	// Load YAML
	yamlEnv, err := loadEnv(envName)
	if err != nil {
		fmt.Printf("Error loading environment config: %v\n", err)
		return
	}

	// Determine profile name
	profileName := yamlEnv.AWSProfile
	if profileName == "" {
		profileName = envName
	}

	// Run AI agent
	if err := RunSSOAgent(profileName, &yamlEnv); err != nil {
		fmt.Printf("AI Agent error: %v\n", err)
	}
}

// runEnhancedSSOAgentFromMenu runs the SSO AI Agent (uses existing agent for now)
func runEnhancedSSOAgentFromMenu() {
	// TODO: Create enhanced agent with tool support (read/write config, ask user, web search)
	// For now, use existing agent
	runSSOAgentFromMenu()
}

// validateAWSFromMenu validates AWS configuration
func validateAWSFromMenu() {
	validator, err := NewAWSProfileValidator()
	if err != nil {
		fmt.Printf("Error creating validator: %v\n", err)
		return
	}

	results, err := validator.ValidateAllProfiles()
	if err != nil {
		fmt.Printf("Validation error: %v\n", err)
		return
	}

	PrintValidationResults(results)

	// Check if any failed
	anyFailed := false
	for _, result := range results {
		if !result.Success {
			anyFailed = true
			break
		}
	}

	if anyFailed {
		fmt.Println()
		fmt.Println("Press Enter to continue...")
		fmt.Scanln()
	}
}

// selectEnvironmentForSSO helps user select environment for SSO setup
func selectEnvironmentForSSO() (string, string, error) {
	yamlFiles, err := findYAMLFiles("project")
	if err != nil {
		return "", "", fmt.Errorf("failed to find YAML files: %w", err)
	}

	if len(yamlFiles) == 0 {
		return "", "", fmt.Errorf("no environment YAML files found in project/ directory")
	}

	// If only one, use it
	if len(yamlFiles) == 1 {
		envName := strings.TrimSuffix(filepath.Base(yamlFiles[0]), ".yaml")
		return envName, yamlFiles[0], nil
	}

	// Multiple environments, ask user to select
	var options []huh.Option[string]
	for _, yamlPath := range yamlFiles {
		envName := strings.TrimSuffix(filepath.Base(yamlPath), ".yaml")
		options = append(options, huh.NewOption(envName, yamlPath))
	}

	var selectedPath string
	huh.NewSelect[string]().
		Title("Select environment to configure AWS SSO:").
		Options(options...).
		Value(&selectedPath).
		Run()

	envName := strings.TrimSuffix(filepath.Base(selectedPath), ".yaml")
	return envName, selectedPath, nil
}
