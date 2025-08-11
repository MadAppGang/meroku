package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
)

// Global variable to store the selected environment
var selectedEnvironment string

func selectEnvironment() error {
	// Find all environment files in current directory
	envFiles, err := findFilesWithExts([]string{".yaml", ".yml"})
	if err != nil {
		return fmt.Errorf("failed to find environment files: %w", err)
	}

	var environments []string
	for _, envFile := range envFiles {
		// Only include YAML files in the root directory (not in subdirectories)
		if !strings.Contains(envFile, "/") {
			envName := strings.TrimSuffix(envFile, ".yaml")
			envName = strings.TrimSuffix(envName, ".yml")
			environments = append(environments, envName)
		}
	}

	// Add option to create new environment
	options := []huh.Option[string]{}
	for _, env := range environments {
		options = append(options, huh.NewOption(fmt.Sprintf("Use existing: %s", env), env))
	}
	options = append(options, huh.NewOption("Create new environment", "create-new"))

	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select an environment").
				Options(options...).
				Value(&selected),
		),
	)

	err = form.Run()
	if err != nil {
		return fmt.Errorf("error selecting environment: %w", err)
	}

	if selected == "create-new" {
		// Create new environment
		envName := createEnvMenu()
		if envName == "" {
			return fmt.Errorf("failed to create environment")
		}
		selected = envName
	}

	// Load the selected environment
	env, err := loadEnv(selected)
	if err != nil {
		return fmt.Errorf("failed to load environment %s: %w", selected, err)
	}

	selectedEnvironment = selected
	fmt.Printf("Selected environment: %s\n", selected)

	// Check if this environment has account_id
	if env.AccountID == "" {
		fmt.Printf("\nNo AWS account configured for '%s' environment.\n", selected)
		err = selectAWSProfileForEnv(selected)
		if err != nil {
			return fmt.Errorf("failed to configure AWS profile: %w", err)
		}
		// Reload environment to get the updated account_id
		env, _ = loadEnv(selected)
	} else {
		// Environment has account_id, try to find matching profile
		if env.AWSProfile != "" {
			// First try the saved profile
			accountID, err := getAWSAccountID(env.AWSProfile)
			if err != nil || accountID != env.AccountID {
				// Saved profile doesn't work or doesn't match, find the correct one
				profile, err := findAWSProfileByAccountID(env.AccountID)
				if err != nil {
					huh.NewNote().
						Title("AWS Profile Not Found").
						Description(fmt.Sprintf("No AWS profile found for account ID: %s\n\nPlease configure AWS access for this account or select a different environment.", env.AccountID)).
						Run()
					return fmt.Errorf("no AWS profile found for account ID: %s", env.AccountID)
				}
				// Validate region if environment already has one configured
				if env.Region != "" {
					profileRegion, err := getAWSRegion(profile)
					if err == nil && profileRegion != "" && profileRegion != env.Region {
						huh.NewNote().
							Title("Region Mismatch Error").
							Description(fmt.Sprintf("The AWS profile '%s' is configured for region '%s', but the environment '%s' requires region '%s'.\n\nPlease use a profile configured for the correct region or update the environment configuration.", profile, profileRegion, selected, env.Region)).
							Run()
						return fmt.Errorf("region mismatch: profile region %s != environment region %s", profileRegion, env.Region)
					}
				}
				// Update the environment with the correct profile
				env.AWSProfile = profile
				// Set region if it's empty
				if env.Region == "" {
					region, err := getAWSRegion(profile)
					if err == nil && region != "" {
						env.Region = region
					}
				}
				saveEnvToFile(env, selected+".yaml")
			}
		} else {
			// No profile saved, try to find one
			profile, err := findAWSProfileByAccountID(env.AccountID)
			if err != nil {
				huh.NewNote().
					Title("AWS Profile Not Found").
					Description(fmt.Sprintf("No AWS profile found for account ID: %s\n\nPlease configure AWS access for this account or select a different environment.", env.AccountID)).
					Run()
				return fmt.Errorf("no AWS profile found for account ID: %s", env.AccountID)
			}
			// Validate region if environment already has one configured
			if env.Region != "" {
				profileRegion, err := getAWSRegion(profile)
				if err == nil && profileRegion != "" && profileRegion != env.Region {
					huh.NewNote().
						Title("Region Mismatch Error").
						Description(fmt.Sprintf("The AWS profile '%s' is configured for region '%s', but the environment '%s' requires region '%s'.\n\nPlease use a profile configured for the correct region or update the environment configuration.", profile, profileRegion, selected, env.Region)).
						Run()
					return fmt.Errorf("region mismatch: profile region %s != environment region %s", profileRegion, env.Region)
				}
			}
			// Update the environment with the found profile
			env.AWSProfile = profile
			// Set region if it's empty
			if env.Region == "" {
				region, err := getAWSRegion(profile)
				if err == nil && region != "" {
					env.Region = region
				}
			}
			saveEnvToFile(env, selected+".yaml")
		}
	}

	// Check if region is empty and try to get it from the profile
	if env.Region == "" && env.AWSProfile != "" {
		region, err := getAWSRegion(env.AWSProfile)
		if err == nil && region != "" {
			env.Region = region
			saveEnvToFile(env, selected+".yaml")
		}
	}
	
	// Set AWS_PROFILE environment variable
	if env.AWSProfile != "" {
		os.Setenv("AWS_PROFILE", env.AWSProfile)
		selectedAWSProfile = env.AWSProfile
		fmt.Printf("Using AWS Profile: %s (Account: %s, Region: %s)\n", env.AWSProfile, env.AccountID, env.Region)
	}

	return nil
}