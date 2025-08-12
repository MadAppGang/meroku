package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// version will be set at compile time using ldflags
var version = "dev"

var (
	profileFlag = flag.String("profile", "", "AWS profile to use (skips profile selection)")
	webFlag     = flag.Bool("web", false, "Open web app immediately")
	envFlag     = flag.String("env", "", "Environment to use (e.g., dev, prod)")
	versionFlag = flag.Bool("version", false, "Show version information")
)

func main() {
	// Parse command line flags
	flag.Parse()

	// Handle version flag
	if *versionFlag {
		fmt.Printf("meroku version %s\n", strings.TrimSpace(version))
		os.Exit(0)
	}

	registerCustomHelpers()
	file, err := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	jsonHandler := slog.NewJSONHandler(file, nil)
	logger := slog.New(jsonHandler)
	slog.SetDefault(logger)

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
		err = os.Setenv("AWS_PROFILE", selectedAWSProfile)
		if err != nil {
			fmt.Printf("Failed to set AWS_PROFILE: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Interactive environment selection
		err = selectEnvironment()
		if err != nil {
			fmt.Println("Error selecting environment:", err)
			os.Exit(1)
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
