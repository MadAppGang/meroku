package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// AWS regions for dropdown selection
var awsRegions = []string{
	"us-east-1",      // US East (N. Virginia)
	"us-east-2",      // US East (Ohio)
	"us-west-1",      // US West (N. California)
	"us-west-2",      // US West (Oregon)
	"af-south-1",     // Africa (Cape Town)
	"ap-east-1",      // Asia Pacific (Hong Kong)
	"ap-south-1",     // Asia Pacific (Mumbai)
	"ap-northeast-1", // Asia Pacific (Tokyo)
	"ap-northeast-2", // Asia Pacific (Seoul)
	"ap-northeast-3", // Asia Pacific (Osaka)
	"ap-southeast-1", // Asia Pacific (Singapore)
	"ap-southeast-2", // Asia Pacific (Sydney)
	"ca-central-1",   // Canada (Central)
	"eu-central-1",   // Europe (Frankfurt)
	"eu-west-1",      // Europe (Ireland)
	"eu-west-2",      // Europe (London)
	"eu-west-3",      // Europe (Paris)
	"eu-south-1",     // Europe (Milan)
	"eu-north-1",     // Europe (Stockholm)
	"me-south-1",     // Middle East (Bahrain)
	"sa-east-1",      // South America (São Paulo)
}

// Common IAM roles for dropdown selection
var commonIAMRoles = []string{
	"AdministratorAccess",
	"PowerUserAccess",
	"ReadOnlyAccess",
	"ViewOnlyAccess",
	"SecurityAudit",
	"DatabaseAdministrator",
	"NetworkAdministrator",
	"SystemAdministrator",
}

// SSOWizardState represents the current step in the setup wizard
type SSOWizardState int

const (
	WizardStateInit SSOWizardState = iota
	WizardStateCheckCLI
	WizardStateAnalyzeProfile
	WizardStateSelectMode
	WizardStateEnterStartURL
	WizardStateEnterSSORegion
	WizardStateEnterAccountID
	WizardStateEnterRole
	WizardStateEnterRegion
	WizardStateEnterOutput
	WizardStateConfirm
	WizardStateWritingConfig
	WizardStateLoggingIn
	WizardStateValidating
	WizardStateSuccess
	WizardStateError
)

// SSOWizardModel is the Bubble Tea model for the setup wizard
type SSOWizardModel struct {
	profileName string
	yamlEnv     *Env // Pre-filled from YAML

	state        SSOWizardState
	inspector    *ProfileInspector
	profileInfo  *ProfileInfo

	// Mode selection
	setupMode    string // "wizard" or "agent"
	useModernSSO bool   // true = modern, false = legacy

	// Input fields
	startURLInput     string
	startURLCursor    int
	ssoRegionInput    string
	ssoRegionCursor   int
	accountIDInput    string
	accountIDCursor   int
	roleInput         string
	roleCursor        int
	regionInput       string
	regionCursor      int
	outputInput       string
	outputCursor      int

	// Current focus
	focusedField string

	// Status
	errorMsg     string
	statusMsg    string
	validationResult *ValidationResult

	// UI
	width  int
	height int
}

// NewSSOWizardModel creates a new wizard model
func NewSSOWizardModel(profileName string, yamlEnv *Env) SSOWizardModel {
	inspector, _ := NewProfileInspector()

	m := SSOWizardModel{
		profileName:  profileName,
		yamlEnv:      yamlEnv,
		state:        WizardStateInit,
		inspector:    inspector,
		useModernSSO: true, // Default to modern SSO

		// Pre-fill from YAML if available
		accountIDInput: func() string {
			if yamlEnv != nil && yamlEnv.AccountID != "" {
				return yamlEnv.AccountID
			}
			return ""
		}(),
		regionInput: func() string {
			if yamlEnv != nil && yamlEnv.Region != "" {
				return yamlEnv.Region
			}
			return "us-east-1"
		}(),
		ssoRegionInput: "us-east-1", // SSO Region always defaults to us-east-1 (IAM Identity Center location)
		outputInput: "json",
		roleInput:   "AdministratorAccess",
	}

	return m
}

func (m SSOWizardModel) Init() tea.Cmd {
	return func() tea.Msg {
		return wizardStepMsg{step: WizardStateCheckCLI}
	}
}

func (m SSOWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case wizardStepMsg:
		m.state = msg.step
		switch m.state {
		case WizardStateCheckCLI:
			return m, m.checkAWSCLI()
		case WizardStateAnalyzeProfile:
			return m, m.analyzeProfile()
		case WizardStateWritingConfig:
			return m, m.writeConfig()
		case WizardStateLoggingIn:
			return m, m.performLogin()
		case WizardStateValidating:
			return m, m.validateCredentials()
		}
		return m, nil

	case wizardErrorMsg:
		m.state = WizardStateError
		m.errorMsg = msg.error.Error()
		return m, nil

	case wizardStatusMsg:
		m.statusMsg = msg.message
		if msg.nextStep != m.state {
			return m, func() tea.Msg {
				return wizardStepMsg{step: msg.nextStep}
			}
		}
		return m, nil

	case wizardValidationMsg:
		m.validationResult = msg.result
		if msg.result.Success {
			m.state = WizardStateSuccess
		} else {
			m.state = WizardStateError
			m.errorMsg = "Credential validation failed"
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyPress(msg)
	}

	return m, nil
}

func (m SSOWizardModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit

	case tea.KeyEsc:
		if m.state == WizardStateSuccess || m.state == WizardStateError {
			return m, tea.Quit
		}
		// Go back one step
		return m.previousStep()

	case tea.KeyEnter:
		return m.nextStep()

	case tea.KeyBackspace:
		m.handleBackspace()

	case tea.KeyRunes:
		m.handleInput(msg.Runes)
	}

	return m, nil
}

func (m *SSOWizardModel) handleInput(runes []rune) {
	char := string(runes)

	switch m.state {
	case WizardStateEnterStartURL:
		m.startURLInput += char
		m.startURLCursor++
	case WizardStateEnterSSORegion:
		m.ssoRegionInput += char
		m.ssoRegionCursor++
	case WizardStateEnterAccountID:
		m.accountIDInput += char
		m.accountIDCursor++
	case WizardStateEnterRole:
		m.roleInput += char
		m.roleCursor++
	case WizardStateEnterRegion:
		m.regionInput += char
		m.regionCursor++
	case WizardStateEnterOutput:
		m.outputInput += char
		m.outputCursor++
	}
}

func (m *SSOWizardModel) handleBackspace() {
	switch m.state {
	case WizardStateEnterStartURL:
		if len(m.startURLInput) > 0 {
			m.startURLInput = m.startURLInput[:len(m.startURLInput)-1]
			m.startURLCursor--
		}
	case WizardStateEnterSSORegion:
		if len(m.ssoRegionInput) > 0 {
			m.ssoRegionInput = m.ssoRegionInput[:len(m.ssoRegionInput)-1]
			m.ssoRegionCursor--
		}
	case WizardStateEnterAccountID:
		if len(m.accountIDInput) > 0 {
			m.accountIDInput = m.accountIDInput[:len(m.accountIDInput)-1]
			m.accountIDCursor--
		}
	case WizardStateEnterRole:
		if len(m.roleInput) > 0 {
			m.roleInput = m.roleInput[:len(m.roleInput)-1]
			m.roleCursor--
		}
	case WizardStateEnterRegion:
		if len(m.regionInput) > 0 {
			m.regionInput = m.regionInput[:len(m.regionInput)-1]
			m.regionCursor--
		}
	case WizardStateEnterOutput:
		if len(m.outputInput) > 0 {
			m.outputInput = m.outputInput[:len(m.outputInput)-1]
			m.outputCursor--
		}
	}
}

func (m SSOWizardModel) nextStep() (tea.Model, tea.Cmd) {
	switch m.state {
	case WizardStateSelectMode:
		// For now, always go to wizard mode (AI agent will be separate entry point)
		m.state = WizardStateEnterStartURL
		return m, nil

	case WizardStateEnterStartURL:
		// Validate URL
		if err := validateSSOStartURL(m.startURLInput); err != nil {
			m.errorMsg = err.Error()
			return m, nil
		}
		m.errorMsg = ""
		m.state = WizardStateEnterSSORegion
		return m, nil

	case WizardStateEnterSSORegion:
		// Validate region
		if err := validateAWSRegion(m.ssoRegionInput); err != nil {
			m.errorMsg = err.Error()
			return m, nil
		}
		m.errorMsg = ""
		m.state = WizardStateEnterAccountID
		return m, nil

	case WizardStateEnterAccountID:
		// Validate account ID
		if err := validateAccountID(m.accountIDInput); err != nil {
			m.errorMsg = err.Error()
			return m, nil
		}
		m.errorMsg = ""
		m.state = WizardStateEnterRole
		return m, nil

	case WizardStateEnterRole:
		// Validate role name
		if err := validateRoleName(m.roleInput); err != nil {
			m.errorMsg = err.Error()
			return m, nil
		}
		m.errorMsg = ""
		m.state = WizardStateEnterRegion
		return m, nil

	case WizardStateEnterRegion:
		// Validate region
		if err := validateAWSRegion(m.regionInput); err != nil {
			m.errorMsg = err.Error()
			return m, nil
		}
		m.errorMsg = ""
		m.state = WizardStateEnterOutput
		return m, nil

	case WizardStateEnterOutput:
		// Validate output format
		if err := validateOutputFormat(m.outputInput); err != nil {
			m.errorMsg = err.Error()
			return m, nil
		}
		m.errorMsg = ""
		m.state = WizardStateConfirm
		return m, nil

	case WizardStateConfirm:
		return m, func() tea.Msg {
			return wizardStepMsg{step: WizardStateWritingConfig}
		}

	case WizardStateSuccess, WizardStateError:
		return m, tea.Quit
	}

	return m, nil
}

func (m SSOWizardModel) previousStep() (tea.Model, tea.Cmd) {
	switch m.state {
	case WizardStateEnterSSORegion:
		m.state = WizardStateEnterStartURL
	case WizardStateEnterAccountID:
		m.state = WizardStateEnterSSORegion
	case WizardStateEnterRole:
		m.state = WizardStateEnterAccountID
	case WizardStateEnterRegion:
		m.state = WizardStateEnterRole
	case WizardStateEnterOutput:
		m.state = WizardStateEnterRegion
	case WizardStateConfirm:
		m.state = WizardStateEnterOutput
	default:
		return m, tea.Quit
	}
	return m, nil
}

// Commands

func (m SSOWizardModel) checkAWSCLI() tea.Cmd {
	return func() tea.Msg {
		if m.inspector == nil {
			return wizardErrorMsg{error: fmt.Errorf("inspector not initialized")}
		}

		if err := m.inspector.CheckAWSCLI(); err != nil {
			return wizardErrorMsg{error: err}
		}

		return wizardStatusMsg{
			message:  "✅ AWS CLI found",
			nextStep: WizardStateAnalyzeProfile,
		}
	}
}

func (m SSOWizardModel) analyzeProfile() tea.Cmd {
	return func() tea.Msg {
		info, err := m.inspector.InspectProfile(m.profileName)
		if err != nil {
			return wizardErrorMsg{error: err}
		}

		m.profileInfo = info

		// If profile exists and complete, show status
		if info.Exists && info.Complete {
			return wizardStatusMsg{
				message:  fmt.Sprintf("✅ Profile '%s' already configured", m.profileName),
				nextStep: WizardStateSelectMode,
			}
		}

		// Profile missing or incomplete, proceed to setup
		return wizardStepMsg{step: WizardStateEnterStartURL}
	}
}

func (m SSOWizardModel) writeConfig() tea.Cmd {
	return func() tea.Msg {
		writer := NewConfigWriter()

		opts := ModernSSOProfileOptions{
			ProfileName:           m.profileName,
			SSOSessionName:        "default-sso",
			SSOStartURL:           m.startURLInput,
			SSORegion:             m.ssoRegionInput,
			SSOAccountID:          m.accountIDInput,
			SSORoleName:           m.roleInput,
			SSORegistrationScopes: "sso:account:access",
			Region:                m.regionInput,
			Output:                m.outputInput,
		}

		if err := writer.WriteModernSSOProfile(opts); err != nil {
			return wizardErrorMsg{error: err}
		}

		return wizardStatusMsg{
			message:  "✅ Configuration written",
			nextStep: WizardStateLoggingIn,
		}
	}
}

func (m SSOWizardModel) performLogin() tea.Cmd {
	return func() tea.Msg {
		autoLogin := NewAutoLogin(m.profileName)

		if err := autoLogin.Login(); err != nil {
			return wizardErrorMsg{error: err}
		}

		return wizardStatusMsg{
			message:  "✅ SSO login successful",
			nextStep: WizardStateValidating,
		}
	}
}

func (m SSOWizardModel) validateCredentials() tea.Cmd {
	return func() tea.Msg {
		autoLogin := NewAutoLogin(m.profileName)

		result, err := autoLogin.ValidateCredentials(m.accountIDInput, m.regionInput)
		if err != nil {
			return wizardErrorMsg{error: err}
		}

		return wizardValidationMsg{result: result}
	}
}

func (m SSOWizardModel) View() string {
	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		MarginBottom(1)

	b.WriteString(titleStyle.Render("🔐 AWS SSO Setup Wizard"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Profile: %s\n\n", m.profileName))

	// Render based on state
	switch m.state {
	case WizardStateInit:
		b.WriteString("Initializing...\n")

	case WizardStateCheckCLI:
		b.WriteString("🔍 Checking AWS CLI installation...\n")

	case WizardStateAnalyzeProfile:
		b.WriteString("🔍 Analyzing existing configuration...\n")

	case WizardStateSelectMode:
		b.WriteString("Profile analysis complete.\n\n")
		b.WriteString("Press Enter to continue with setup.\n")

	case WizardStateEnterStartURL:
		b.WriteString("SSO Start URL:\n")
		b.WriteString("(Example: https://mycompany.awsapps.com/start)\n\n")
		b.WriteString("> " + m.startURLInput + "█\n\n")
		if m.errorMsg != "" {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("❌ " + m.errorMsg + "\n"))
		}
		b.WriteString("\nPress Enter to continue, Esc to go back\n")

	case WizardStateEnterSSORegion:
		prefillNote := ""
		if m.yamlEnv != nil && m.yamlEnv.Region != "" {
			prefillNote = " (from YAML)"
		}
		b.WriteString("SSO Region" + prefillNote + ":\n")
		b.WriteString("(The AWS region hosting your IAM Identity Center)\n")
		b.WriteString("Common: us-east-1, us-west-2, eu-west-1, ap-southeast-1\n\n")
		b.WriteString("> " + m.ssoRegionInput + "█\n\n")
		if m.errorMsg != "" {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("❌ " + m.errorMsg + "\n"))
		}
		b.WriteString("\nPress Enter to continue, Esc to go back\n")

	case WizardStateEnterAccountID:
		prefillNote := ""
		if m.yamlEnv != nil && m.yamlEnv.AccountID != "" {
			prefillNote = " (from YAML)"
		}
		b.WriteString("AWS Account ID" + prefillNote + ":\n")
		b.WriteString("(12-digit AWS account number)\n\n")
		b.WriteString("> " + m.accountIDInput + "█\n\n")
		if m.errorMsg != "" {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("❌ " + m.errorMsg + "\n"))
		}
		b.WriteString("\nPress Enter to continue, Esc to go back\n")

	case WizardStateEnterRole:
		b.WriteString("IAM Role Name:\n")
		b.WriteString("(Common: AdministratorAccess, PowerUserAccess, ReadOnlyAccess)\n\n")
		b.WriteString("> " + m.roleInput + "█\n\n")
		if m.errorMsg != "" {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("❌ " + m.errorMsg + "\n"))
		}
		b.WriteString("\nPress Enter to continue, Esc to go back\n")

	case WizardStateEnterRegion:
		prefillNote := ""
		if m.yamlEnv != nil && m.yamlEnv.Region != "" {
			prefillNote = " (from YAML)"
		}
		b.WriteString("Default AWS Region" + prefillNote + ":\n")
		b.WriteString("(Region for AWS CLI commands)\n")
		b.WriteString("Common: us-east-1, us-west-2, eu-west-1, ap-southeast-1\n\n")
		b.WriteString("> " + m.regionInput + "█\n\n")
		if m.errorMsg != "" {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("❌ " + m.errorMsg + "\n"))
		}
		b.WriteString("\nPress Enter to continue, Esc to go back\n")

	case WizardStateEnterOutput:
		b.WriteString("Output Format:\n")
		b.WriteString("(Options: json, yaml, text, table)\n\n")
		b.WriteString("> " + m.outputInput + "█\n\n")
		if m.errorMsg != "" {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("❌ " + m.errorMsg + "\n"))
		}
		b.WriteString("\nPress Enter to continue, Esc to go back\n")

	case WizardStateConfirm:
		b.WriteString("Confirm Configuration:\n\n")
		b.WriteString(fmt.Sprintf("  SSO Start URL:  %s\n", m.startURLInput))
		b.WriteString(fmt.Sprintf("  SSO Region:     %s\n", m.ssoRegionInput))
		b.WriteString(fmt.Sprintf("  Account ID:     %s\n", m.accountIDInput))
		b.WriteString(fmt.Sprintf("  Role Name:      %s\n", m.roleInput))
		b.WriteString(fmt.Sprintf("  Default Region: %s\n", m.regionInput))
		b.WriteString(fmt.Sprintf("  Output Format:  %s\n\n", m.outputInput))
		b.WriteString("Press Enter to create profile, Esc to go back\n")

	case WizardStateWritingConfig:
		b.WriteString("🔧 Writing configuration...\n")
		if m.statusMsg != "" {
			b.WriteString(m.statusMsg + "\n")
		}

	case WizardStateLoggingIn:
		b.WriteString("🔐 Logging in to AWS SSO...\n")
		b.WriteString("(Browser window should open for authentication)\n")
		if m.statusMsg != "" {
			b.WriteString(m.statusMsg + "\n")
		}

	case WizardStateValidating:
		b.WriteString("✓ Validating credentials...\n")
		if m.statusMsg != "" {
			b.WriteString(m.statusMsg + "\n")
		}

	case WizardStateSuccess:
		successStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true)
		b.WriteString(successStyle.Render("✅ SUCCESS!") + "\n\n")
		b.WriteString(fmt.Sprintf("AWS SSO profile '%s' is ready to use!\n\n", m.profileName))
		if m.validationResult != nil {
			b.WriteString(fmt.Sprintf("Account: %s\n", m.validationResult.AccountID))
			b.WriteString(fmt.Sprintf("User:    %s\n\n", m.validationResult.ARN))
		}
		b.WriteString("Press Enter or Esc to exit\n")

	case WizardStateError:
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
		b.WriteString(errorStyle.Render("❌ ERROR") + "\n\n")
		b.WriteString(m.errorMsg + "\n\n")
		b.WriteString("Press Enter or Esc to exit\n")
	}

	return b.String()
}

// Messages

type wizardStepMsg struct {
	step SSOWizardState
}

type wizardErrorMsg struct {
	error error
}

type wizardStatusMsg struct {
	message  string
	nextStep SSOWizardState
}

type wizardValidationMsg struct {
	result *ValidationResult
}

// RunSSOWizard runs the interactive setup wizard
func RunSSOWizard(profileName string, yamlEnv *Env) error {
	// Use new huh-based wizard implementation
	return RunSSOWizardHuh(profileName, yamlEnv)
}

// RunSSOWizardHuh is the new huh-based wizard with dropdown selections
func RunSSOWizardHuh(profileName string, yamlEnv *Env) error {
	fmt.Println("\n🔐 AWS SSO Setup Wizard")
	fmt.Printf("Profile: %s\n\n", profileName)

	// Check AWS CLI
	inspector, err := NewProfileInspector()
	if err != nil {
		return fmt.Errorf("failed to create inspector: %w", err)
	}

	if err := inspector.CheckAWSCLI(); err != nil {
		fmt.Println("❌ AWS CLI v2+ is required for SSO")
		fmt.Println("Install from: https://aws.amazon.com/cli/")
		return fmt.Errorf("AWS CLI not available")
	}

	// Inspect current profile
	profileInfo, err := inspector.InspectProfile(profileName)
	if err != nil {
		return fmt.Errorf("failed to inspect profile: %w", err)
	}

	// Pre-fill values from YAML or existing profile
	var (
		ssoStartURL  string
		ssoRegion    string
		accountID    string
		roleName     string
		region       string
		output       string
	)

	// Pre-fill from existing profile (fallback values)
	if profileInfo.Exists {
		if profileInfo.SSOStartURL != "" {
			ssoStartURL = profileInfo.SSOStartURL
		}
		if profileInfo.SSORegion != "" {
			ssoRegion = profileInfo.SSORegion
		}
		if profileInfo.SSOAccountID != "" {
			accountID = profileInfo.SSOAccountID
		}
		if profileInfo.SSORoleName != "" {
			roleName = profileInfo.SSORoleName
		}
		if profileInfo.Region != "" {
			region = profileInfo.Region
		}
		if profileInfo.Output != "" {
			output = profileInfo.Output
		}
	}

	// Pre-fill from YAML (source of truth - overrides profile values)
	if yamlEnv != nil {
		if yamlEnv.AccountID != "" {
			accountID = yamlEnv.AccountID
		}
		if yamlEnv.Region != "" {
			region = yamlEnv.Region
			// Note: SSO Region is NOT set from YAML - it defaults to us-east-1
			// SSO Region is where IAM Identity Center is hosted (usually us-east-1)
			// Default Region is where your resources run (from YAML)
		}
	}

	// Set defaults if still empty
	if ssoRegion == "" {
		ssoRegion = "us-east-1"
	}
	if region == "" {
		region = "us-east-1"
	}
	if roleName == "" {
		roleName = "AdministratorAccess"
	}
	if output == "" {
		output = "json"
	}

	// Create region options for dropdown
	regionOptions := make([]huh.Option[string], len(awsRegions))
	for i, r := range awsRegions {
		regionOptions[i] = huh.NewOption(r, r)
	}

	// Create role options for dropdown
	roleOptions := make([]huh.Option[string], len(commonIAMRoles))
	for i, r := range commonIAMRoles {
		roleOptions[i] = huh.NewOption(r, r)
	}
	// Add "Other" option for custom role
	roleOptions = append(roleOptions, huh.NewOption("Other (enter custom)", "custom"))

	// Validate Account ID from YAML
	if accountID == "" {
		return fmt.Errorf("account ID not found in YAML file - please add account_id to %s", selectedEnvironment+".yaml")
	}
	if len(accountID) != 12 {
		return fmt.Errorf("invalid account ID in YAML file: must be exactly 12 digits")
	}

	// Build the form (simplified - only ask for SSO-specific fields)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("SSO Start URL").
				Description("Example: https://mycompany.awsapps.com/start").
				Value(&ssoStartURL).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("SSO start URL is required")
					}
					if !strings.HasPrefix(s, "https://") {
						return fmt.Errorf("must start with https://")
					}
					return nil
				}),
		),

		huh.NewGroup(
			huh.NewSelect[string]().
				Title("SSO Region").
				Description("The AWS region hosting your IAM Identity Center").
				Options(regionOptions...).
				Value(&ssoRegion),
		),

		huh.NewGroup(
			huh.NewSelect[string]().
				Title("IAM Role Name").
				Description("Select the IAM role for this profile").
				Options(roleOptions...).
				Value(&roleName),
		),

		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Default AWS Region").
				Description("Region for AWS CLI commands (from YAML: "+region+")").
				Options(regionOptions...).
				Value(&region),
		),
	)

	// Run the form
	err = form.Run()
	if err != nil {
		return fmt.Errorf("form cancelled: %w", err)
	}

	// Handle custom role name
	if roleName == "custom" {
		var customRole string
		err := huh.NewInput().
			Title("Enter custom IAM role name").
			Value(&customRole).
			Run()
		if err != nil {
			return fmt.Errorf("cancelled: %w", err)
		}
		roleName = customRole
	}

	// Confirm before writing
	var confirm bool
	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Confirm AWS SSO Configuration").
				Description(fmt.Sprintf(
					"Profile: %s\nSSO URL: %s\nSSO Region: %s\nAccount: %s\nRole: %s\nRegion: %s\nOutput: %s",
					profileName, ssoStartURL, ssoRegion, accountID, roleName, region, output,
				)).
				Value(&confirm),
		),
	)

	err = confirmForm.Run()
	if err != nil || !confirm {
		fmt.Println("❌ Setup cancelled")
		return fmt.Errorf("setup cancelled")
	}

	// Write the configuration
	writer := NewConfigWriter()

	options := ModernSSOProfileOptions{
		ProfileName:           profileName,
		SSOSessionName:        "sso-session-" + profileName,
		SSOStartURL:           ssoStartURL,
		SSORegion:             ssoRegion,
		SSOAccountID:          accountID,
		SSORoleName:           roleName,
		SSORegistrationScopes: "sso:account:access",
		Region:                region,
		Output:                output,
	}

	fmt.Println("\n💾 Writing configuration...")
	err = writer.WriteModernSSOProfile(options)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Println("✅ Configuration written successfully")

	// Attempt automatic login
	fmt.Println("\n🔐 Attempting AWS SSO login...")
	autoLogin := NewAutoLogin(profileName)
	loginErr := autoLogin.Login()
	if loginErr != nil {
		fmt.Printf("⚠️  Login failed: %v\n", loginErr)
		fmt.Println("You can login manually with: aws sso login --profile " + profileName)
		return nil // Don't fail the wizard
	}

	// Validate credentials
	fmt.Println("\n✓ Validating credentials...")
	validationResult, err := autoLogin.ValidateCredentials(accountID, region)
	if err != nil {
		fmt.Printf("⚠️  Validation failed: %v\n", err)
		return nil // Don't fail the wizard
	}

	// Success!
	fmt.Println("\n✅ SUCCESS!")
	fmt.Printf("AWS SSO profile '%s' is ready to use!\n\n", profileName)
	fmt.Printf("Account: %s\n", validationResult.AccountID)
	fmt.Printf("User:    %s\n", validationResult.ARN)

	return nil
}
