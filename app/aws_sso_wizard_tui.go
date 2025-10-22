package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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
		ssoRegionInput: "us-east-1",
		outputInput:    "json",
		roleInput:      "AdministratorAccess",
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
		b.WriteString("SSO Region:\n")
		b.WriteString("(The AWS region hosting your IAM Identity Center)\n\n")
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
		b.WriteString("(Region for AWS CLI commands)\n\n")
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
	model := NewSSOWizardModel(profileName, yamlEnv)
	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("wizard error: %w", err)
	}
	return nil
}
