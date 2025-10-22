package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AWSProfileValidator validates AWS profiles before operations
type AWSProfileValidator struct {
	inspector *ProfileInspector
}

// NewAWSProfileValidator creates a new validator
func NewAWSProfileValidator() (*AWSProfileValidator, error) {
	inspector, err := NewProfileInspector()
	if err != nil {
		return nil, err
	}

	return &AWSProfileValidator{
		inspector: inspector,
	}, nil
}

// ValidateAllProfiles validates all profiles from YAML files
func (v *AWSProfileValidator) ValidateAllProfiles() ([]ProfileValidationResult, error) {
	// Find all YAML files in project directory
	yamlFiles, err := findYAMLFiles("project")
	if err != nil {
		return nil, fmt.Errorf("failed to find YAML files: %w", err)
	}

	var results []ProfileValidationResult

	for _, yamlPath := range yamlFiles {
		// Load YAML
		envName := strings.TrimSuffix(filepath.Base(yamlPath), ".yaml")
		env, err := loadEnv(envName)
		if err != nil {
			results = append(results, ProfileValidationResult{
				YAMLPath: yamlPath,
				EnvName:  envName,
				Error:    fmt.Errorf("failed to load YAML: %w", err),
			})
			continue
		}

		// Validate profile
		result := v.ValidateProfile(&env)
		result.YAMLPath = yamlPath
		results = append(results, result)
	}

	return results, nil
}

// ValidateProfile validates a single profile from environment config
func (v *AWSProfileValidator) ValidateProfile(env *Env) ProfileValidationResult {
	result := ProfileValidationResult{
		EnvName:    env.Env,
		Profile:    env.AWSProfile,
		Success:    true,
		Warnings:   []string{},
		Errors:     []string{},
	}

	// Check if profile is specified
	if env.AWSProfile == "" {
		result.Success = false
		result.Errors = append(result.Errors, "No AWS profile specified in YAML (aws_profile field missing)")
		return result
	}

	// Inspect profile
	info, err := v.inspector.InspectProfile(env.AWSProfile)
	if err != nil {
		result.Success = false
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to inspect profile: %v", err))
		return result
	}

	result.ProfileInfo = info

	// Check if profile exists
	if !info.Exists {
		result.Success = false
		result.Errors = append(result.Errors, fmt.Sprintf("Profile '%s' not found in AWS config", env.AWSProfile))
		result.Fixable = true
		result.FixSuggestion = "Run the AWS SSO Setup Wizard or AI Agent to create this profile"
		return result
	}

	// Check if profile is complete
	if !info.Complete {
		result.Success = false
		result.Errors = append(result.Errors, fmt.Sprintf("Profile '%s' is incomplete. Missing: %s",
			env.AWSProfile, strings.Join(info.MissingFields, ", ")))
		result.Fixable = true
		result.FixSuggestion = "Run the AWS SSO Setup Wizard to complete the profile configuration"
		return result
	}

	// Check SSO token status (if SSO profile)
	if info.Type == ProfileTypeModernSSO || info.Type == ProfileTypeLegacySSO {
		tokenValid, err := CheckSSOTokenStatus(env.AWSProfile)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Could not check SSO token status: %v", err))
		} else if !tokenValid {
			result.Success = false
			result.Errors = append(result.Errors, "SSO token expired or missing")
			result.Fixable = true
			result.FixSuggestion = fmt.Sprintf("Run: aws sso login --profile %s", env.AWSProfile)
		}
	}

	// Validate credentials work
	if result.Success {
		autoLogin := NewAutoLogin(env.AWSProfile)
		validationResult, err := autoLogin.ValidateCredentials(env.AccountID, env.Region)
		if err != nil {
			result.Success = false
			result.Errors = append(result.Errors, fmt.Sprintf("Credential validation failed: %v", err))
			result.Fixable = true
			result.FixSuggestion = fmt.Sprintf("Run: aws sso login --profile %s", env.AWSProfile)
		} else if validationResult.AccountIDMismatch {
			result.Success = false
			result.Errors = append(result.Errors, fmt.Sprintf(
				"Account ID mismatch: YAML says %s, AWS returns %s",
				validationResult.ExpectedAccountID,
				validationResult.AccountID))
			result.Fixable = true
			result.FixSuggestion = "Update the account_id in your YAML file or reconfigure the AWS profile"
		}
	}

	return result
}

// ProfileValidationResult contains the validation result for a profile
type ProfileValidationResult struct {
	YAMLPath      string
	EnvName       string
	Profile       string
	Success       bool
	ProfileInfo   *ProfileInfo
	Warnings      []string
	Errors        []string
	Fixable       bool
	FixSuggestion string
	Error         error // Fatal error (couldn't even validate)
}

// String returns a formatted string representation
func (r ProfileValidationResult) String() string {
	var b strings.Builder

	if r.Error != nil {
		b.WriteString(fmt.Sprintf("❌ %s: %v\n", r.EnvName, r.Error))
		return b.String()
	}

	if r.Success {
		b.WriteString(fmt.Sprintf("✅ %s (profile: %s)\n", r.EnvName, r.Profile))
		if len(r.Warnings) > 0 {
			for _, warning := range r.Warnings {
				b.WriteString(fmt.Sprintf("   ⚠️  %s\n", warning))
			}
		}
	} else {
		b.WriteString(fmt.Sprintf("❌ %s (profile: %s)\n", r.EnvName, r.Profile))
		for _, err := range r.Errors {
			b.WriteString(fmt.Sprintf("   • %s\n", err))
		}
		if r.Fixable && r.FixSuggestion != "" {
			b.WriteString(fmt.Sprintf("   💡 Fix: %s\n", r.FixSuggestion))
		}
	}

	return b.String()
}

// PrintValidationResults prints all validation results
func PrintValidationResults(results []ProfileValidationResult) {
	fmt.Println("\n🔍 AWS Profile Validation Results:")
	fmt.Println("══════════════════════════════════════════════════════════════════")

	for _, result := range results {
		fmt.Print(result.String())
	}

	fmt.Println("══════════════════════════════════════════════════════════════════")
}

// OfferFix presents fix options to the user
func OfferFix(results []ProfileValidationResult) error {
	// Find profiles that need fixing
	var needsFix []ProfileValidationResult
	for _, result := range results {
		if !result.Success && result.Fixable {
			needsFix = append(needsFix, result)
		}
	}

	if len(needsFix) == 0 {
		return nil
	}

	fmt.Println("\n⚠️  Some profiles need configuration.")
	fmt.Println()
	fmt.Println("How would you like to fix this?")
	fmt.Println("  [1] Interactive Wizard (step-by-step)")
	fmt.Println("  [2] AI Agent (automatic)")
	fmt.Println("  [3] Skip for now")
	fmt.Println()
	fmt.Print("Choice (1-3): ")

	var choice string
	fmt.Scanln(&choice)
	choice = strings.TrimSpace(choice)

	switch choice {
	case "1":
		// Run wizard for each broken profile
		for _, result := range needsFix {
			fmt.Printf("\n═══ Setting up profile: %s ═══\n\n", result.Profile)
			envName := strings.TrimSuffix(filepath.Base(result.YAMLPath), ".yaml")
			yamlEnv, _ := loadEnv(envName)
			if err := RunSSOWizard(result.Profile, &yamlEnv); err != nil {
				return fmt.Errorf("wizard failed: %w", err)
			}
		}
		return nil

	case "2":
		// Run AI agent for each broken profile
		for _, result := range needsFix {
			fmt.Printf("\n═══ AI Agent: Setting up profile %s ═══\n\n", result.Profile)
			envName := strings.TrimSuffix(filepath.Base(result.YAMLPath), ".yaml")
			yamlEnv, _ := loadEnv(envName)
			if err := RunSSOAgent(result.Profile, &yamlEnv); err != nil {
				return fmt.Errorf("AI agent failed: %w", err)
			}
		}
		return nil

	case "3":
		fmt.Println("\n⚠️  Skipping AWS profile configuration.")
		fmt.Println("Note: Some operations may fail without valid AWS credentials.")
		return nil

	default:
		fmt.Println("\nInvalid choice, skipping.")
		return nil
	}
}

// findYAMLFiles finds all YAML environment files
func findYAMLFiles(dir string) ([]string, error) {
	var yamlFiles []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Look for environment YAML files (dev.yaml, staging.yaml, prod.yaml)
		if strings.HasSuffix(name, ".yaml") && !strings.HasPrefix(name, ".") {
			yamlFiles = append(yamlFiles, filepath.Join(dir, name))
		}
	}

	return yamlFiles, nil
}
