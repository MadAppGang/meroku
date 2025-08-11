package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
		
		// Get the region for this profile
		region, err := getAWSRegion(selectedProfile)
		if err != nil {
			fmt.Printf("Warning: failed to get AWS region: %v\n", err)
			region = "us-east-1" // Default fallback
		}
		
		// Save the account_id to the environment file if envName is provided
		if envName != "" {
			env, err := loadEnv(envName)
			if err != nil {
				return fmt.Errorf("failed to load environment %s: %w", envName, err)
			}
			
			// Check for region mismatch if environment already has a region configured
			if env.Region != "" && env.Region != region {
				huh.NewNote().
					Title("Region Mismatch Error").
					Description(fmt.Sprintf("The AWS profile '%s' is configured for region '%s', but the environment '%s' requires region '%s'.\n\nPlease select a profile configured for the correct region.", selectedProfile, region, envName, env.Region)).
					Run()
				return fmt.Errorf("region mismatch: profile region %s != environment region %s", region, env.Region)
			}
			
			env.AccountID = accountID
			env.AWSProfile = selectedProfile
			// Only update region if it was empty
			if env.Region == "" {
				env.Region = region
			}
			
			// Save the updated environment
			if err := saveEnvToFile(env, envName+".yaml"); err != nil {
				return fmt.Errorf("failed to save environment: %w", err)
			}
			
			fmt.Printf("AWS profile '%s' selected successfully (Account: %s, Region: %s) and saved to %s.yaml\n", selectedProfile, accountID, region, envName)
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
					// Check for region mismatch if environment already has a region configured
					if env.Region != "" && env.Region != region {
						fmt.Printf("Warning: Skipping %s - region mismatch (profile: %s, env: %s)\n", envName, region, env.Region)
						continue
					}
					env.AccountID = accountID
					env.AWSProfile = selectedProfile
					// Only update region if it was empty
					if env.Region == "" {
						env.Region = region
					}
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
		// Check if this is an SSO-related error and try to login automatically
		if strings.Contains(err.Error(), "SSO") || strings.Contains(err.Error(), "token") || strings.Contains(err.Error(), "expired") {
			fmt.Printf("AWS SSO session expired for profile '%s'. Attempting automatic login...\n", profile)
			
			// Try to run aws sso login
			loginErr := runAWSSSO(profile)
			if loginErr != nil {
				return "", fmt.Errorf("failed to refresh SSO login: %w", loginErr)
			}
			
			// Retry the identity call after login
			result, err = stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
			if err != nil {
				return "", fmt.Errorf("failed to get caller identity after SSO login: %w", err)
			}
		} else {
			return "", fmt.Errorf("failed to get caller identity: %w", err)
		}
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

func runAWSSSO(profile string) error {
	cmd := exec.Command("aws", "sso", "login", "--profile", profile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("aws sso login failed: %w", err)
	}
	
	return nil
}

// getAWSRegion retrieves the region configured for the given AWS profile
func getAWSRegion(profile string) (string, error) {
	// Set the profile temporarily
	oldProfile := os.Getenv("AWS_PROFILE")
	os.Setenv("AWS_PROFILE", profile)
	defer os.Setenv("AWS_PROFILE", oldProfile)
	
	ctx := context.TODO()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config: %w", err)
	}
	
	return cfg.Region, nil
}

