package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/charmbracelet/huh"
)

// Global variable to store the selected AWS profile
var selectedAWSProfile string

func selectAWSProfile() error {
	return selectAWSProfileForEnv("")
}

func selectAWSProfileForEnv(envName string) error {
	profiles, err := getLocalAWSProfiles()
	if err != nil {
		return fmt.Errorf("failed to get AWS profiles: %w", err)
	}

	if len(profiles) == 0 {
		return fmt.Errorf("no AWS profiles found")
	}

	var selectedProfile string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select an AWS Profile").
				Options(huh.NewOptions(profiles...)...).
				Value(&selectedProfile),
		),
	)

	err = form.Run()
	if err != nil {
		return fmt.Errorf("error running form: %w", err)
	}

	if selectedProfile != "" {
		// Store the selected profile globally
		selectedAWSProfile = selectedProfile
		
		err = os.Setenv("AWS_PROFILE", selectedProfile)
		if err != nil {
			return fmt.Errorf("failed to set AWS_PROFILE: %w", err)
		}
		
		// Get the account ID for this profile
		accountID, err := getAWSAccountID(selectedProfile)
		if err != nil {
			// Handle SSO login if needed
			if strings.Contains(err.Error(), "the SSO session has expired or is invalid") || strings.Contains(err.Error(), "unable to refresh SSO token") {
				fmt.Println("SSO session has expired or is invalid. Attempting to log in...")
				_, err = runCommandWithOutput("aws", "sso", "login", "--profile", selectedProfile)
				if err != nil {
					return fmt.Errorf("failed to run 'aws sso login': %w", err)
				}
				fmt.Println("SSO login successful. Retrying...")
				accountID, err = getAWSAccountID(selectedProfile)
				if err != nil {
					return fmt.Errorf("failed to get account ID after SSO login: %w", err)
				}
			} else {
				return fmt.Errorf("failed to get account ID: %w", err)
			}
		}
		
		// Save the account_id to the environment file if envName is provided
		if envName != "" {
			env, err := loadEnv(envName)
			if err != nil {
				return fmt.Errorf("failed to load environment %s: %w", envName, err)
			}
			
			env.AccountID = accountID
			env.AWSProfile = selectedProfile
			
			// Save the updated environment
			if err := saveEnvToFile(env, envName+".yaml"); err != nil {
				return fmt.Errorf("failed to save environment: %w", err)
			}
			
			fmt.Printf("AWS profile '%s' selected successfully (Account: %s) and saved to %s.yaml\n", selectedProfile, accountID, envName)
		} else {
			// If no specific environment, try to update all environments that don't have account_id
			envFiles, _ := findFilesWithExts([]string{".yaml", ".yml"})
			updatedEnvs := []string{}
			
			for _, envFile := range envFiles {
				// Only process files in current directory
				if strings.Contains(envFile, "/") {
					continue
				}
				
				envName := strings.TrimSuffix(envFile, ".yaml")
				envName = strings.TrimSuffix(envName, ".yml")
				env, err := loadEnv(envName)
				if err != nil {
					continue
				}
				
				// Only update if account_id is empty
				if env.AccountID == "" {
					env.AccountID = accountID
					env.AWSProfile = selectedProfile
					if err := saveEnvToFile(env, envName+".yaml"); err == nil {
						updatedEnvs = append(updatedEnvs, envName)
					}
				}
			}
			
			if len(updatedEnvs) > 0 {
				fmt.Printf("AWS profile '%s' selected successfully (Account: %s) and saved to: %s\n", 
					selectedProfile, accountID, strings.Join(updatedEnvs, ", "))
			} else {
				fmt.Printf("AWS profile '%s' selected successfully (Account: %s)\n", selectedProfile, accountID)
			}
		}
		
		return nil
	} else {
		fmt.Println("No profile selected")
	}

	return nil
}

func getLocalAWSProfiles() ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".aws", "config")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read AWS config file: %w", err)
	}

	var profiles []string
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[profile ") && strings.HasSuffix(line, "]") {
			profile := strings.TrimPrefix(line, "[profile ")
			profile = strings.TrimSuffix(profile, "]")
			profiles = append(profiles, profile)
		}
	}

	return profiles, nil
}

func getAWSAccountID(profile string) (string, error) {
	// Set the profile temporarily
	oldProfile := os.Getenv("AWS_PROFILE")
	os.Setenv("AWS_PROFILE", profile)
	defer os.Setenv("AWS_PROFILE", oldProfile)
	
	ctx := context.TODO()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config: %w", err)
	}
	
	stsClient := sts.NewFromConfig(cfg)
	result, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("failed to get caller identity: %w", err)
	}
	
	if result.Account == nil {
		return "", fmt.Errorf("account ID is nil")
	}
	
	return *result.Account, nil
}

func findAWSProfileByAccountID(targetAccountID string) (string, error) {
	profiles, err := getLocalAWSProfiles()
	if err != nil {
		return "", fmt.Errorf("failed to get AWS profiles: %w", err)
	}
	
	for _, profile := range profiles {
		accountID, err := getAWSAccountID(profile)
		if err != nil {
			// Skip profiles that can't be accessed (might need SSO login)
			continue
		}
		
		if accountID == targetAccountID {
			return profile, nil
		}
	}
	
	return "", fmt.Errorf("no AWS profile found for account ID: %s", targetAccountID)
}

