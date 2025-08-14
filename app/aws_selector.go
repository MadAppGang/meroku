package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
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

	// Add option to create new profile
	options := append([]string{"+ Create New Profile"}, profiles...)

	var selectedProfile string

	// Show the help resources using lipgloss table
	displayAWSResourcesTable()

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select an AWS Profile").
				Options(huh.NewOptions(options...)...).
				Value(&selectedProfile),
		),
	)

	err = form.Run()
	if err != nil {
		return fmt.Errorf("error running form: %w", err)
	}

	// Handle profile creation
	if selectedProfile == "+ Create New Profile" {
		newProfile, err := createAWSProfile()
		if err != nil {
			return fmt.Errorf("failed to create AWS profile: %w", err)
		}
		selectedProfile = newProfile
	}

	return processSelectedProfile(selectedProfile, envName)
}

// processSelectedProfile handles the selected profile configuration
func processSelectedProfile(selectedProfile string, envName string) error {
	if selectedProfile != "" {
		// Store the selected profile globally
		selectedAWSProfile = selectedProfile
		
		err := os.Setenv("AWS_PROFILE", selectedProfile)
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
				// Skip DNS config file
				if strings.Contains(envFile, "/") || envFile == "dns.yaml" {
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

// SSOSession represents an AWS SSO session configuration
type SSOSession struct {
	Name               string
	StartURL           string
	Region             string
	RegistrationScopes string
}

// AWSRegion represents an AWS region with its code and name
type AWSRegion struct {
	Code string
	Name string
}

// displayAWSResourcesTable displays AWS SSO resources in a formatted table
func displayAWSResourcesTable() {
	// Define styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("220")).
		Align(lipgloss.Center).
		MarginTop(1).
		MarginBottom(1)

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86"))

	// Create the main content sections
	fmt.Println(titleStyle.Render("📚 AWS SSO Resources 📚"))

	// Define URL style for links to pop
	urlStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).  // Bright blue
		Background(lipgloss.Color("235")). // Dark background
		Padding(0, 1).
		Underline(true)
	
	resourceLabelStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("220"))

	// Resources section
	resourcesTable := table.New().
		Border(lipgloss.ThickBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("220"))). // Yellow border to pop
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == 0 {
				return headerStyle.Align(lipgloss.Center)
			}
			if col == 0 {
				return resourceLabelStyle.PaddingLeft(1).PaddingRight(1)
			}
			// Make URLs pop with styling
			return lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
		}).
		Headers("Resource", "Click to Open").
		Row(
			"📖 Documentation",
			urlStyle.Render("https://docs.aws.amazon.com/cli/latest/userguide/sso-configure-profile-token.html"),
		).
		Row(
			"🎥 Video Tutorial",
			urlStyle.Render("https://www.youtube.com/watch?v=bVjwu1WN42I"),
		)

	fmt.Println(resourcesTable)

	// Example configurations
	fmt.Println()
	examplesTitle := headerStyle.Render("📝 Example Configurations:")
	fmt.Println(examplesTitle)
	fmt.Println()

	// Define styles for syntax highlighting
	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("220")) // Yellow for [sections]
	
	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")) // Blue for keys
	
	equalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")) // Gray for =
	
	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("82")) // Green for values

	// Helper function to format configuration lines
	formatConfigLine := func(line string) string {
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			return sectionStyle.Render(line)
		}
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				return keyStyle.Render(key) + equalStyle.Render(" = ") + valueStyle.Render(value)
			}
		}
		return line
	}

	// Create single column example table showing the complete config file
	exampleTable := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("86"))).
		Width(100).
		Headers(headerStyle.Render("~/.aws/config")).
		Row(formatConfigLine("[sso-session my-company]")).
		Row(formatConfigLine("sso_start_url = https://my-company.awsapps.com/start")).
		Row(formatConfigLine("sso_region = us-east-1")).
		Row(formatConfigLine("sso_registration_scopes = sso:account:access")).
		Row("").
		Row(formatConfigLine("[profile dev-account]")).
		Row(formatConfigLine("credential_process = aws configure export-credentials --profile dev-account")).
		Row(formatConfigLine("sso_session = my-company")).
		Row(formatConfigLine("sso_account_id = 123456789012")).
		Row(formatConfigLine("sso_role_name = AdministratorAccess")).
		Row(formatConfigLine("region = us-west-2"))

	fmt.Println(exampleTable)
	fmt.Println()
}

// displaySSOHelpTable displays help for SSO configuration
func displaySSOHelpTable() {
	// Define styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("220")).
		Align(lipgloss.Center).
		MarginTop(1).
		MarginBottom(1)

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86"))

	tipStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("82")).
		Bold(true)

	fmt.Println(titleStyle.Render("📚 AWS SSO Configuration Help 📚"))

	// Define URL style for links to pop
	urlStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).  // Bright blue
		Background(lipgloss.Color("235")). // Dark background
		Padding(0, 1).
		Underline(true)
	
	resourceLabelStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("220"))

	// Help resources table
	helpTable := table.New().
		Border(lipgloss.ThickBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("220"))). // Yellow border to pop
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == 0 {
				return headerStyle.Align(lipgloss.Center)
			}
			if col == 0 {
				return resourceLabelStyle.PaddingLeft(1).PaddingRight(1)
			}
			return lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
		}).
		Headers("Resource", "Click to Open").
		Row(
			"📖 Official Documentation",
			urlStyle.Render("https://docs.aws.amazon.com/cli/latest/userguide/sso-configure-profile-token.html"),
		).
		Row(
			"🎥 Video Tutorial",
			urlStyle.Render("https://www.youtube.com/watch?v=bVjwu1WN42I"),
		)

	fmt.Println(helpTable)

	// Example configuration
	fmt.Println()
	exampleTitle := headerStyle.Render("📝 Example Configuration:")
	fmt.Println(exampleTitle)
	fmt.Println()

	// Define styles for syntax highlighting
	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("220")) // Yellow for [sections]
	
	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")) // Blue for keys
	
	equalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")) // Gray for =
	
	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("82")) // Green for values

	// Helper function to format configuration lines
	formatConfigLine := func(line string) string {
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			return sectionStyle.Render(line)
		}
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				return keyStyle.Render(key) + equalStyle.Render(" = ") + valueStyle.Render(value)
			}
		}
		return line
	}

	// Combined example table showing complete config file with syntax highlighting
	configTable := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("86"))).
		Width(100).
		Headers(headerStyle.Render("~/.aws/config")).
		Row(formatConfigLine("[sso-session company-sso]")).
		Row(formatConfigLine("sso_start_url = https://your-org.awsapps.com/start")).
		Row(formatConfigLine("sso_region = us-east-1")).
		Row(formatConfigLine("sso_registration_scopes = sso:account:access")).
		Row("").
		Row(formatConfigLine("[profile production]")).
		Row(formatConfigLine("credential_process = aws configure export-credentials --profile production")).
		Row(formatConfigLine("sso_session = company-sso")).
		Row(formatConfigLine("sso_account_id = 987654321098")).
		Row(formatConfigLine("sso_role_name = AdministratorAccess")).
		Row(formatConfigLine("region = eu-west-1"))

	fmt.Println(configTable)

	// Tip
	tip := tipStyle.MarginTop(1).Render("💡 Tip: ") + 
		"Configure SSO manually, then return here to create your profile."
	fmt.Println(tip)
	fmt.Println()
}

// getAWSRegions returns a list of all AWS regions with their names
func getAWSRegions() []AWSRegion {
	return []AWSRegion{
		{Code: "us-east-1", Name: "US East (N. Virginia)"},
		{Code: "us-east-2", Name: "US East (Ohio)"},
		{Code: "us-west-1", Name: "US West (N. California)"},
		{Code: "us-west-2", Name: "US West (Oregon)"},
		{Code: "af-south-1", Name: "Africa (Cape Town)"},
		{Code: "ap-east-1", Name: "Asia Pacific (Hong Kong)"},
		{Code: "ap-south-1", Name: "Asia Pacific (Mumbai)"},
		{Code: "ap-south-2", Name: "Asia Pacific (Hyderabad)"},
		{Code: "ap-southeast-1", Name: "Asia Pacific (Singapore)"},
		{Code: "ap-southeast-2", Name: "Asia Pacific (Sydney)"},
		{Code: "ap-southeast-3", Name: "Asia Pacific (Jakarta)"},
		{Code: "ap-southeast-4", Name: "Asia Pacific (Melbourne)"},
		{Code: "ap-northeast-1", Name: "Asia Pacific (Tokyo)"},
		{Code: "ap-northeast-2", Name: "Asia Pacific (Seoul)"},
		{Code: "ap-northeast-3", Name: "Asia Pacific (Osaka)"},
		{Code: "ca-central-1", Name: "Canada (Central)"},
		{Code: "ca-west-1", Name: "Canada West (Calgary)"},
		{Code: "eu-central-1", Name: "Europe (Frankfurt)"},
		{Code: "eu-central-2", Name: "Europe (Zurich)"},
		{Code: "eu-west-1", Name: "Europe (Ireland)"},
		{Code: "eu-west-2", Name: "Europe (London)"},
		{Code: "eu-west-3", Name: "Europe (Paris)"},
		{Code: "eu-south-1", Name: "Europe (Milan)"},
		{Code: "eu-south-2", Name: "Europe (Spain)"},
		{Code: "eu-north-1", Name: "Europe (Stockholm)"},
		{Code: "il-central-1", Name: "Israel (Tel Aviv)"},
		{Code: "me-south-1", Name: "Middle East (Bahrain)"},
		{Code: "me-central-1", Name: "Middle East (UAE)"},
		{Code: "sa-east-1", Name: "South America (São Paulo)"},
	}
}

// checkSSOSessions reads the AWS config file and returns available SSO sessions
func checkSSOSessions() ([]SSOSession, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".aws", "config")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read AWS config file: %w", err)
	}

	var sessions []SSOSession
	lines := strings.Split(string(content), "\n")
	
	var currentSession *SSOSession
	ssoSessionPattern := regexp.MustCompile(`^\[sso-session\s+(.+)\]$`)
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		
		// Check for SSO session header
		if matches := ssoSessionPattern.FindStringSubmatch(line); matches != nil {
			if currentSession != nil {
				sessions = append(sessions, *currentSession)
			}
			currentSession = &SSOSession{Name: matches[1]}
			continue
		}
		
		// Parse SSO session properties
		if currentSession != nil {
			if strings.HasPrefix(line, "sso_start_url") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					currentSession.StartURL = strings.TrimSpace(parts[1])
				}
			} else if strings.HasPrefix(line, "sso_region") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					currentSession.Region = strings.TrimSpace(parts[1])
				}
			} else if strings.HasPrefix(line, "sso_registration_scopes") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					currentSession.RegistrationScopes = strings.TrimSpace(parts[1])
				}
			} else if strings.HasPrefix(line, "[") {
				// New section started, save current session
				if currentSession != nil {
					sessions = append(sessions, *currentSession)
					currentSession = nil
				}
			}
		}
	}
	
	// Save the last session if exists
	if currentSession != nil {
		sessions = append(sessions, *currentSession)
	}
	
	return sessions, nil
}

// createSSOSession creates a new SSO session configuration
func createSSOSession() (*SSOSession, error) {
	var sessionName, startURL, region string
	
	// Get session name
	nameForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter SSO Session Name").
				Description("e.g., 'mag' or 'mycompany'").
				Value(&sessionName).
				Validate(func(str string) error {
					if strings.TrimSpace(str) == "" {
						return fmt.Errorf("session name cannot be empty")
					}
					return nil
				}),
		),
	)
	
	if err := nameForm.Run(); err != nil {
		return nil, err
	}
	
	// Get SSO start URL
	urlForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter SSO Start URL").
				Description("e.g., https://madappgang.awsapps.com/start").
				Value(&startURL).
				Validate(func(str string) error {
					if !strings.HasPrefix(str, "https://") {
						return fmt.Errorf("SSO start URL must begin with https://")
					}
					return nil
				}),
		),
	)
	
	if err := urlForm.Run(); err != nil {
		return nil, err
	}
	
	// Get SSO region - use a searchable select list
	regions := getAWSRegions()
	
	regionOptions := make([]huh.Option[string], len(regions))
	for i, r := range regions {
		// Display format: "us-east-1 - US East (N. Virginia)"
		display := fmt.Sprintf("%s - %s", r.Code, r.Name)
		regionOptions[i] = huh.NewOption(display, r.Code)
	}
	
	// Default to us-east-1
	region = "us-east-1"
	regionForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select SSO Region").
				Description("Type to search for a region").
				Options(regionOptions...).
				Value(&region).
				Filtering(true), // Enable filtering/search
		),
	)
	
	if err := regionForm.Run(); err != nil {
		return nil, err
	}
	
	// Create the SSO session
	session := &SSOSession{
		Name:               sessionName,
		StartURL:           startURL,
		Region:             region,
		RegistrationScopes: "sso:account:access",
	}
	
	// Write to AWS config file
	if err := appendSSOSessionToConfig(session); err != nil {
		return nil, fmt.Errorf("failed to write SSO session to config: %w", err)
	}
	
	fmt.Printf("✅ SSO session '%s' created successfully\n", sessionName)
	return session, nil
}

// appendSSOSessionToConfig appends an SSO session to the AWS config file
func appendSSOSessionToConfig(session *SSOSession) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}
	
	configPath := filepath.Join(homeDir, ".aws", "config")
	
	// Open file in append mode
	file, err := os.OpenFile(configPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open AWS config file: %w", err)
	}
	defer file.Close()
	
	// Write SSO session configuration
	writer := bufio.NewWriter(file)
	fmt.Fprintf(writer, "\n[sso-session %s]\n", session.Name)
	fmt.Fprintf(writer, "sso_start_url = %s\n", session.StartURL)
	fmt.Fprintf(writer, "sso_region = %s\n", session.Region)
	fmt.Fprintf(writer, "sso_registration_scopes = %s\n", session.RegistrationScopes)
	
	return writer.Flush()
}

// createAWSProfile creates a new AWS profile with SSO configuration
func createAWSProfile() (string, error) {
	// Check for existing SSO sessions
	sessions, err := checkSSOSessions()
	if err != nil {
		return "", fmt.Errorf("failed to check SSO sessions: %w", err)
	}
	
	var selectedSession *SSOSession
	
	if len(sessions) == 0 {
		// No SSO sessions found, offer to create one
		var createChoice string
		createForm := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("No SSO Sessions Found").
					Description("You need an SSO session to create a profile.").
					Options(
						huh.NewOption("Create SSO Session", "create"),
						huh.NewOption("View Documentation", "docs"),
						huh.NewOption("Cancel", "cancel"),
					).
					Value(&createChoice),
			),
		)
		
		if err := createForm.Run(); err != nil {
			return "", err
		}
		
		switch createChoice {
		case "docs":
			displaySSOHelpTable()
			return "", fmt.Errorf("user chose to view documentation")
		case "create":
			session, err := createSSOSession()
			if err != nil {
				return "", err
			}
			selectedSession = session
		case "cancel":
			return "", fmt.Errorf("user cancelled")
		}
	} else {
		// Select from existing SSO sessions
		var sessionName string
		sessionOptions := make([]huh.Option[string], len(sessions))
		for i, session := range sessions {
			sessionOptions[i] = huh.NewOption(
				fmt.Sprintf("%s (%s)", session.Name, session.StartURL),
				session.Name,
			)
		}
		
		selectForm := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Select SSO Session").
					Options(sessionOptions...).
					Value(&sessionName),
			),
		)
		
		if err := selectForm.Run(); err != nil {
			return "", err
		}
		
		// Find the selected session
		for i := range sessions {
			if sessions[i].Name == sessionName {
				selectedSession = &sessions[i]
				break
			}
		}
	}
	
	if selectedSession == nil {
		return "", fmt.Errorf("no SSO session selected")
	}
	
	// Get profile details
	var profileName, accountID, roleName, region string
	
	// Profile name
	profileForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter Profile Name").
				Description("e.g., 'mycomply' or 'dev-account'").
				Value(&profileName).
				Validate(func(str string) error {
					if strings.TrimSpace(str) == "" {
						return fmt.Errorf("profile name cannot be empty")
					}
					// Check if profile already exists
					profiles, _ := getLocalAWSProfiles()
					for _, p := range profiles {
						if p == str {
							return fmt.Errorf("profile '%s' already exists", str)
						}
					}
					return nil
				}),
		),
	)
	
	if err := profileForm.Run(); err != nil {
		return "", err
	}
	
	// Account ID
	accountForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter AWS Account ID").
				Description("12-digit account ID (e.g., 591726693874)").
				Value(&accountID).
				Validate(func(str string) error {
					if len(str) != 12 {
						return fmt.Errorf("account ID must be 12 digits")
					}
					for _, c := range str {
						if c < '0' || c > '9' {
							return fmt.Errorf("account ID must contain only digits")
						}
					}
					return nil
				}),
		),
	)
	
	if err := accountForm.Run(); err != nil {
		return "", err
	}
	
	// Role name - default to AdministratorAccess
	roleName = "AdministratorAccess"
	roleForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter SSO Role Name").
				Description("e.g., 'AdministratorAccess' or 'ReadOnlyAccess'").
				Value(&roleName).
				Placeholder("AdministratorAccess").
				Validate(func(str string) error {
					if strings.TrimSpace(str) == "" {
						return fmt.Errorf("role name cannot be empty")
					}
					return nil
				}),
		),
	)
	
	if err := roleForm.Run(); err != nil {
		return "", err
	}
	
	// Region - use a searchable select list
	regions := getAWSRegions()
	
	regionOptions := make([]huh.Option[string], len(regions))
	for i, r := range regions {
		// Display format: "us-east-1 - US East (N. Virginia)"
		display := fmt.Sprintf("%s - %s", r.Code, r.Name)
		regionOptions[i] = huh.NewOption(display, r.Code)
	}
	
	// Default to us-east-1
	region = "us-east-1"
	regionForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select AWS Region").
				Description("Type to search for a region").
				Options(regionOptions...).
				Value(&region).
				Filtering(true), // Enable filtering/search
		),
	)
	
	if err := regionForm.Run(); err != nil {
		return "", err
	}
	
	// Write the profile to AWS config
	if err := appendProfileToConfig(profileName, selectedSession.Name, accountID, roleName, region); err != nil {
		return "", fmt.Errorf("failed to write profile to config: %w", err)
	}
	
	fmt.Printf("✅ Profile '%s' created successfully\n", profileName)
	fmt.Println("\nYou may need to run 'aws sso login' to authenticate with this profile.")
	
	return profileName, nil
}

// appendProfileToConfig appends a profile to the AWS config file
func appendProfileToConfig(profileName, sessionName, accountID, roleName, region string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}
	
	configPath := filepath.Join(homeDir, ".aws", "config")
	
	// Open file in append mode
	file, err := os.OpenFile(configPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open AWS config file: %w", err)
	}
	defer file.Close()
	
	// Write profile configuration
	writer := bufio.NewWriter(file)
	fmt.Fprintf(writer, "\n[profile %s]\n", profileName)
	fmt.Fprintf(writer, "credential_process = aws configure export-credentials --profile %s\n", profileName)
	fmt.Fprintf(writer, "sso_session = %s\n", sessionName)
	fmt.Fprintf(writer, "sso_account_id = %s\n", accountID)
	fmt.Fprintf(writer, "sso_role_name = %s\n", roleName)
	fmt.Fprintf(writer, "region = %s\n", region)
	
	return writer.Flush()
}

