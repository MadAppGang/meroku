package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	pricingpkg "madappgang.com/meroku/pricing"
)

// version will be set at compile time using ldflags
var version = "dev"

// cachedVersion stores the computed version to avoid re-reading files
var cachedVersion string

// globalPricingService is the centralized pricing service instance
// Initialized once at startup and used by all API endpoints
var globalPricingService *pricingpkg.Service

var (
	profileFlag    = flag.String("profile", "", "AWS profile to use (skips profile selection)")
	webFlag        = flag.Bool("web", false, "Open web app immediately")
	envFlag        = flag.String("env", "", "Environment to use (e.g., dev, prod)")
	versionFlag    = flag.Bool("version", false, "Show version information")
	renderDiffFlag = flag.String("renderdiff", "", "Render terraform plan diff view from JSON file (for testing)")
	debugFlag      = flag.String("debug", "", "Debug mode to test screens (e.g., api_missing_key)")
	awsConfigFlag  = flag.String("aws-config", "", "Custom AWS config file path (for testing different scenarios)")
)

// GetVersion returns the actual version, reading from infrastructure/version.txt
func GetVersion() string {
	// Return cached version if already computed
	if cachedVersion != "" {
		return cachedVersion
	}

	// If version was set at compile time (ldflags), use it
	if version != "dev" {
		cachedVersion = version
		return cachedVersion
	}

	// Read from infrastructure/version.txt (standard project structure)
	if data, err := os.ReadFile("infrastructure/version.txt"); err == nil {
		cachedVersion = strings.TrimSpace(string(data))
		return cachedVersion
	}

	// No version file found, use global default
	cachedVersion = version
	return cachedVersion
}

func main() {
	// Parse command line flags
	flag.Parse()

	// Handle version flag (early, before any initialization)
	if *versionFlag {
		fmt.Printf("meroku version %s\n", strings.TrimSpace(GetVersion()))
		os.Exit(0)
	}

	// Handle debug flag for testing screens (early, before any initialization)
	if *debugFlag != "" {
		handleDebugScreen(*debugFlag)
		// handleDebugScreen will exit, so this line is never reached
	}

	// Set custom AWS config path if provided (for testing)
	if *awsConfigFlag != "" {
		// Validate that the custom config file exists
		if _, err := os.Stat(*awsConfigFlag); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "❌ Error: Custom AWS config file does not exist: %s\n", *awsConfigFlag)
			fmt.Fprintf(os.Stderr, "\nPlease check the path and try again.\n")
			fmt.Fprintf(os.Stderr, "\nAvailable test configs:\n")
			fmt.Fprintf(os.Stderr, "  - test-configs/aws-config-empty\n")
			fmt.Fprintf(os.Stderr, "  - test-configs/aws-config-modern-sso\n")
			fmt.Fprintf(os.Stderr, "  - test-configs/aws-config-legacy-sso\n")
			fmt.Fprintf(os.Stderr, "  - test-configs/aws-config-incomplete\n")
			os.Exit(1)
		}
		SetCustomAWSConfigPath(*awsConfigFlag)
	}

	// Initialize pricing service early (needed for web API)
	// This runs in background and caches pricing data
	ctx := context.Background()
	regions := []string{"us-east-1", "us-west-2", "eu-west-1"}

	var err error
	globalPricingService, err = pricingpkg.NewService(ctx, regions)
	if err != nil {
		log.Printf("[Pricing] Warning: Failed to initialize pricing service: %v", err)
		log.Printf("[Pricing] Continuing with fallback prices only")
	}

	// Check for Anthropic API key (only in interactive mode)
	// Skip check if using flags that don't need AI (version, renderdiff, dns, migrate, generate, web)
	needsInteractiveCheck := !*versionFlag && *renderDiffFlag == "" && !*webFlag && *debugFlag == ""
	if needsInteractiveCheck && len(flag.Args()) == 0 {
		// Only show the screen in fully interactive mode (no commands)
		if !CheckAnthropicAPIKey() {
			ShowAPIKeyRequiredScreen()
		}
	}

	// Handle renderdiff flag for testing terraform plan diff view
	if *renderDiffFlag != "" {
		if err := runRenderDiff(*renderDiffFlag); err != nil {
			fmt.Printf("Error rendering diff: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Handle DNS commands (before environment selection)
	args := flag.Args()
	if len(args) > 0 && args[0] == "dns" {
		// DNS commands don't need environment selection
		handleDNSCommand(args[1:])
		os.Exit(0)
	}

	// Handle migrate commands (before environment selection)
	if len(args) > 0 && args[0] == "migrate" {
		handleMigrateCommand(args[1:])
		os.Exit(0)
	}

	// Handle generate commands (before environment selection)
	if len(args) > 0 && args[0] == "generate" {
		handleGenerateCommand(args[1:])
		os.Exit(0)
	}

	registerCustomHelpers()

	// Handle environment and profile selection
	if *envFlag != "" {
		// Use the provided environment directly
		selectedEnvironment = *envFlag
		fmt.Printf("Using environment: %s\n", selectedEnvironment)
		
		// Load the environment to check for account_id
		env, err := loadEnv(selectedEnvironment)
		if err != nil {
			fmt.Printf("Failed to load environment %s: %v\n", selectedEnvironment, err)
			os.Exit(1)
		}
		
		if env.AccountID == "" {
			// Need to select AWS profile for this environment
			err = selectAWSProfileForEnv(selectedEnvironment)
			if err != nil {
				fmt.Printf("Failed to configure AWS profile: %v\n", err)
				os.Exit(1)
			}
		} else {
			// Environment has account_id, find matching profile
			if env.AWSProfile != "" {
				// Verify the saved profile still works
				accountID, err := getAWSAccountID(env.AWSProfile)
				if err != nil || accountID != env.AccountID {
					// Saved profile doesn't work, find the correct one
					profile, err := findAWSProfileByAccountID(env.AccountID)
					if err != nil {
						fmt.Printf("Error: No AWS profile found for account ID: %s\n", env.AccountID)
						fmt.Println("Please configure AWS access for this account or select a different environment.")
						os.Exit(1)
					}
					env.AWSProfile = profile
					saveEnvToFile(env, selectedEnvironment+".yaml")
				}
			} else {
				// No profile saved, find one
				profile, err := findAWSProfileByAccountID(env.AccountID)
				if err != nil {
					fmt.Printf("Error: No AWS profile found for account ID: %s\n", env.AccountID)
					fmt.Println("Please configure AWS access for this account or select a different environment.")
					os.Exit(1)
				}
				env.AWSProfile = profile
				saveEnvToFile(env, selectedEnvironment+".yaml")
			}
			
			// Set the AWS profile and region
			os.Setenv("AWS_PROFILE", env.AWSProfile)
			selectedAWSProfile = env.AWSProfile
			if env.Region != "" {
				selectedAWSRegion = env.Region
				os.Setenv("AWS_REGION", env.Region)
				os.Setenv("AWS_DEFAULT_REGION", env.Region)
			}
			fmt.Printf("Using AWS Profile: %s (Account: %s, Region: %s)\n", env.AWSProfile, env.AccountID, env.Region)
		}
	} else if *profileFlag != "" {
		// Use the provided profile directly (backward compatibility)
		selectedAWSProfile = *profileFlag
		fmt.Printf("Using AWS profile: %s\n", selectedAWSProfile)
		os.Setenv("AWS_PROFILE", selectedAWSProfile)
	} else {
		// Interactive environment selection
		if err := selectEnvironment(); err != nil {
			fmt.Println("Error selecting environment:", err)
			os.Exit(1)
		}
	}

	// Check for updates at startup (only in interactive mode)
	versionCheckResult := checkVersionAtStartup()
	if versionCheckResult.Error == nil && versionCheckResult.UpdateAvailable {
		// Silently prompt for update if available
		if err := promptForUpdate(versionCheckResult); err != nil {
			// Don't exit on error, just log and continue
			fmt.Printf("Note: Update check encountered an issue: %v\n", err)
		}
	}
	// Silently ignore errors from version check to not disrupt startup

	// Automatic AWS SSO validation (only in interactive mode, not for --web)
	if !*webFlag && selectedEnvironment != "" && selectedAWSProfile != "" {
		if err := performAutoSSOValidation(); err != nil {
			// Error already displayed to user, just log and continue
			log.Printf("SSO validation error: %v", err)
		}
	}

	// If --web flag is set, open web app directly
	if *webFlag {
		startSPAServerWithAutoOpen("8080", true, false)
		// Keep the program running
		fmt.Println("\nWeb server is running. Press Ctrl+C to stop.")
		select {}
	} else {
		// Run normal interactive menu
		mainMenu()
	}

	os.Exit(0)
}

// handleDNSCommand handles DNS subcommands
func handleDNSCommand(args []string) {
	if len(args) == 0 {
		fmt.Println("DNS management commands:")
		fmt.Println("  dns setup    - Run DNS setup wizard")
		fmt.Println("  dns status   - Show DNS configuration status")
		fmt.Println("  dns validate - Validate DNS configuration")
		fmt.Println("  dns remove   - Remove subdomain delegation")
		return
	}

	switch args[0] {
	case "setup":
		runDNSSetupWizard()
	case "status":
		if err := runDNSStatus(nil, args[1:]); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "validate":
		if err := runDNSValidate(nil, args[1:]); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "remove":
		if len(args) < 2 {
			fmt.Println("Usage: dns remove [subdomain]")
			os.Exit(1)
		}
		if err := runDNSRemove(nil, args[1:]); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("Unknown DNS command: %s\n", args[0])
		fmt.Println("Available commands: setup, status, validate, remove")
		os.Exit(1)
	}
}

// handleMigrateCommand handles migration subcommands
func handleMigrateCommand(args []string) {
	if len(args) == 0 {
		fmt.Println("YAML Schema Migration Commands:")
		fmt.Println("  migrate all      - Migrate all YAML files in project directory")
		fmt.Println("  migrate <file>   - Migrate a specific YAML file")
		fmt.Println()
		fmt.Printf("Current schema version: v%d\n", CurrentSchemaVersion)
		return
	}

	switch args[0] {
	case "all":
		if err := MigrateAllYAMLFiles(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("\nAll migrations completed successfully!")
	default:
		// Treat as filename
		filename := args[0]
		if err := MigrateYAMLFile(filename); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Migration completed successfully!")
	}
}

// handleDebugScreen handles debug mode for testing screens
func handleDebugScreen(screenName string) {
	switch screenName {
	case "api_missing_key", "api":
		fmt.Println("Debug Mode: Displaying API key missing screen\n")
		ShowAPIKeyRequiredScreen()

	case "terraform_plan", "plan":
		fmt.Println("Debug Mode: Displaying Terraform Plan TUI with sample data\n")
		fmt.Println("Sample plan includes:")
		fmt.Println("  - VPC and networking resources (create)")
		fmt.Println("  - ECS cluster and services (create/update)")
		fmt.Println("  - RDS database (create)")
		fmt.Println("  - Task definitions (replace)")
		fmt.Println("  - Security groups (delete)")
		fmt.Println("  - Route53 records (update)")
		fmt.Println("\nStarting TUI...\n")
		time.Sleep(1 * time.Second)

		// Create and run the terraform plan TUI with sample data
		model, err := initModernTerraformPlanTUI(getSampleTerraformPlan())
		if err != nil {
			fmt.Printf("Error initializing TUI: %v\n", err)
			os.Exit(1)
		}

		p := tea.NewProgram(model, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Printf("Error running TUI: %v\n", err)
			os.Exit(1)
		}

	case "list", "help", "":
		fmt.Println("Debug Mode - Available Screens:")
		fmt.Println()
		fmt.Println("  api_missing_key, api    - Anthropic API key required screen")
		fmt.Println("                             Shows when ANTHROPIC_API_KEY is not set")
		fmt.Println()
		fmt.Println("  terraform_plan, plan    - Terraform plan viewer TUI")
		fmt.Println("                             Interactive plan review with sample data")
		fmt.Println("                             Navigate with arrows, press 'a' for AI help")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Println("  ./meroku --debug <screen_name>")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  ./meroku --debug api")
		fmt.Println("  ./meroku --debug terraform_plan")
		fmt.Println("  ./meroku --debug list")
		os.Exit(0)

	default:
		fmt.Printf("Unknown debug screen: %s\n", screenName)
		fmt.Println("\nRun './meroku --debug list' to see available screens")
		os.Exit(1)
	}
	os.Exit(0)
}

// runRenderDiff renders the terraform plan diff view from a JSON file
func runRenderDiff(planFile string) error {
	// Always run the interactive TUI when --renderdiff is used
	// The user explicitly wants to test the TUI view
	return runTerraformPlanTUI(planFile)
}

// runTerraformPlanTUI runs the interactive TUI for a terraform plan file
func runTerraformPlanTUI(planFile string) error {
	// Read the plan file
	planData, err := os.ReadFile(planFile)
	if err != nil {
		return fmt.Errorf("failed to read plan file: %w", err)
	}
	
	// Initialize the TUI model
	model, err := initModernTerraformPlanTUI(string(planData))
	if err != nil {
		return fmt.Errorf("failed to initialize TUI: %w", err)
	}
	
	// Create and run the tea program
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}

	return nil
}

// performAutoSSOValidation validates AWS SSO configuration at startup
// If validation fails, offers to fix via wizard or AI agent
func performAutoSSOValidation() error {
	// Load the current environment YAML
	yamlEnv, err := loadEnv(selectedEnvironment)
	if err != nil {
		return fmt.Errorf("failed to load environment: %w", err)
	}

	// Use the profile from YAML or fallback to environment name
	profileName := yamlEnv.AWSProfile
	if profileName == "" {
		profileName = selectedEnvironment
	}

	// Create profile inspector
	inspector, err := NewProfileInspector()
	if err != nil {
		return fmt.Errorf("failed to create profile inspector: %w", err)
	}

	// Check if AWS CLI is installed
	if err := inspector.CheckAWSCLI(); err != nil {
		fmt.Printf("\n⚠️  AWS CLI not installed or not v2+\n")
		fmt.Printf("AWS SSO requires AWS CLI v2.0 or later.\n")
		fmt.Printf("Install from: https://aws.amazon.com/cli/\n\n")
		return nil // Don't block startup for missing CLI
	}

	// Inspect the profile
	profileInfo, err := inspector.InspectProfile(profileName)
	if err != nil {
		return fmt.Errorf("failed to inspect profile: %w", err)
	}

	// If profile is complete and valid, nothing to do
	if profileInfo.Complete {
		fmt.Printf("✅ AWS SSO profile '%s' is properly configured\n\n", profileName)
		return nil
	}

	// Profile is incomplete - offer to fix
	fmt.Printf("\n⚠️  AWS SSO Configuration Issue Detected\n\n")

	if !profileInfo.Exists {
		fmt.Printf("Profile '%s' does not exist in AWS config\n", profileName)
	} else {
		fmt.Printf("Profile '%s' is missing required fields:\n", profileName)
		for _, field := range profileInfo.MissingFields {
			fmt.Printf("  - %s\n", field)
		}
	}

	fmt.Printf("\nWould you like to fix this now?\n\n")

	// Check if Anthropic API key is available
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	hasAPIKey := anthropicKey != ""

	// Build options based on API key availability
	options := []huh.Option[string]{
		huh.NewOption("🔐 Run Interactive Setup Wizard", "wizard"),
	}

	if hasAPIKey {
		options = append(options, huh.NewOption("🤖 Use AI Agent", "agent"))
	} else {
		options = append(options, huh.NewOption("🤖 AI Agent (API key not configured)", "agent_disabled"))
	}

	options = append(options, huh.NewOption("⏭  Skip for now (continue to main menu)", "skip"))

	// Offer fix options
	var choice string
	err = huh.NewSelect[string]().
		Title("Fix AWS SSO Configuration").
		Options(options...).
		Value(&choice).
		Run()

	if err != nil {
		return fmt.Errorf("failed to get user choice: %w", err)
	}

	switch choice {
	case "wizard":
		fmt.Println("\n🔐 Starting AWS SSO Setup Wizard...\n")
		if err := RunSSOWizard(profileName, &yamlEnv); err != nil {
			return fmt.Errorf("wizard failed: %w", err)
		}
	case "agent":
		fmt.Println("\n🤖 Starting AWS SSO AI Agent...\n")
		if err := RunSSOAgent(profileName, &yamlEnv); err != nil {
			return fmt.Errorf("AI agent failed: %w", err)
		}
	case "agent_disabled":
		fmt.Println("\n❌ AI Agent Not Available\n")
		fmt.Println("The AI Agent requires an Anthropic API key to function.")
		fmt.Println("Please set the ANTHROPIC_API_KEY environment variable:")
		fmt.Println("\n  export ANTHROPIC_API_KEY=your_key_here")
		fmt.Println("\nGet your API key from: https://console.anthropic.com/settings/keys\n")
		fmt.Println("Returning to main menu...")
		return nil
	case "skip":
		fmt.Println("\n⏭  Skipping AWS SSO configuration check\n")
		fmt.Println("Note: You can configure AWS SSO later from the main menu:\n")
		fmt.Println("  - 🔐 AWS SSO Setup Wizard")
		fmt.Println("  - 🤖 AWS SSO AI Agent")
		fmt.Println("  - ✓ Validate AWS Configuration\n")
	}

	return nil
}
