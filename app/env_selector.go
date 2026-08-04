package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"gopkg.in/yaml.v2"
)

// Global variables to store the selected environment and AWS region
var selectedEnvironment string
var selectedAWSRegion string

// listEnvironmentNames returns the environments meroku offers: every YAML file
// in the project root except the DNS config.
//
// Extracted from selectEnvironment so that commands which have to resolve an
// environment without a menu — `meroku sync` — use the same definition of "an
// environment exists" as the menu does, rather than inventing a second one.
func listEnvironmentNames() ([]string, error) {
	// findFilesWithExts reads the project root only and returns names with the
	// extension already stripped, so a name can never contain a separator.
	envFiles, err := findFilesWithExts([]string{".yaml", ".yml"})
	if err != nil {
		return nil, fmt.Errorf("failed to find environment files: %w", err)
	}

	var environments []string
	for _, envName := range envFiles {
		if strings.Contains(envName, "/") || envName == "dns" {
			continue
		}
		if !looksLikeEnvironmentConfig(envName) {
			continue
		}
		environments = append(environments, envName)
	}
	return environments, nil
}

// looksLikeEnvironmentConfig decides whether a root YAML file is a meroku
// environment, by reading it rather than by trusting its name.
//
// Filtering on filename alone offered every stray YAML in the project root as a
// deployable environment: `meroku sync` in this repo picked "Taskfile" and then
// failed trying to read a state backend out of it. docker-compose.yml,
// .golangci.yml and any CI config would do the same. Worse than the noise, an
// operator scanning the picker for "prod" should not have to know which entries
// are real.
//
// Every meroku environment declares both `project` and `env` -- they are what the
// resource names are built from, so a config without them cannot deploy anything.
// That makes them a reliable signature, and it costs one small read of a handful
// of root files.
//
// Anything unreadable or unparseable is simply not an environment here. This
// function answers "should this appear in the list", and a file we cannot read is
// not one the operator can pick.
func looksLikeEnvironmentConfig(name string) bool {
	for _, ext := range []string{".yaml", ".yml"} {
		data, err := os.ReadFile(name + ext)
		if err != nil {
			continue
		}

		var probe struct {
			Project string `yaml:"project"`
			Env     string `yaml:"env"`
		}
		if err := yaml.Unmarshal(data, &probe); err != nil {
			continue
		}
		if probe.Project != "" && probe.Env != "" {
			return true
		}
	}
	return false
}

func selectEnvironment() error {
	environments, err := listEnvironmentNames()
	if err != nil {
		return err
	}

	// Add environment options
	options := []huh.Option[string]{}

	for _, env := range environments {
		options = append(options, huh.NewOption(fmt.Sprintf("Use existing: %s", env), env))
	}
	options = append(options, huh.NewOption("Create new environment", "create-new"))

	// Check DNS configuration status and add DNS option at the end
	dnsConfig, _ := loadDNSConfig()
	dnsLabel := "🌐 DNS Setup - Configure custom domain"
	if dnsConfig != nil && dnsConfig.RootDomain != "" {
		dnsLabel = fmt.Sprintf("🌐 DNS Setup - Domain: %s ✓", dnsConfig.RootDomain)
	}
	options = append(options, huh.NewOption(dnsLabel, "dns-setup"))

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

	if selected == "dns-setup" {
		// Run DNS setup wizard
		runDNSSetupWizard()
		// After DNS setup, return to environment selection
		return selectEnvironment()
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

	// Validate AWS SSO configuration BEFORE attempting any AWS API calls
	// This prevents auto-login from hiding incomplete profile issues
	if env.AWSProfile != "" {
		selectedAWSProfile = env.AWSProfile // Set for validation function
		if err := performAutoSSOValidation(); err != nil {
			fmt.Printf("Warning: SSO validation encountered an issue: %v\n", err)
			// Don't fail - allow user to continue
		}
	}

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

	// Set AWS_PROFILE and AWS_REGION environment variables
	if env.AWSProfile != "" {
		os.Setenv("AWS_PROFILE", env.AWSProfile)
		selectedAWSProfile = env.AWSProfile
		fmt.Printf("Using AWS Profile: %s (Account: %s, Region: %s)\n", env.AWSProfile, env.AccountID, env.Region)
	}

	// Set selected AWS region for global access
	if env.Region != "" {
		selectedAWSRegion = env.Region
		os.Setenv("AWS_REGION", env.Region)
		os.Setenv("AWS_DEFAULT_REGION", env.Region)
	}

	// Picking an environment is when meroku tells you what is deployed, so a
	// directory that has just been written (or checked out) but never initialised
	// reads here as "nothing deployed". Offer to link it now, before the main menu
	// makes that claim.
	//
	// This runs last on purpose: the profile and region above are what the state
	// lookup needs. Selection has already made AWS calls (STS, profile matching),
	// so one read of the state object is in keeping with what this path does — and
	// it only happens for an environment that is written but not initialised.
	if err := reconnectStateIfNeeded(context.Background(), selected, env, filepath.Join("env", selected)); err != nil {
		// Selection succeeded; a failed sync is information, not a blocker.
		fmt.Printf("⚠️  Could not finish syncing with the deployed state: %v\n", err)
	}

	return nil
}
