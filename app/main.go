package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

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

	// Handle version flag
	if *versionFlag {
		fmt.Printf("meroku version %s\n", strings.TrimSpace(GetVersion()))
		os.Exit(0)
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
			
			// Set the AWS profile
			os.Setenv("AWS_PROFILE", env.AWSProfile)
			selectedAWSProfile = env.AWSProfile
			fmt.Printf("Using AWS Profile: %s (Account: %s)\n", env.AWSProfile, env.AccountID)
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
