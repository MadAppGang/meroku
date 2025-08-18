package main

import (
	"fmt"
	"strings"
	
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type AWSProfileCreationState int

const (
	StateSelectSSO AWSProfileCreationState = iota
	StateEnterSSOURL
	StateEnterSSORegion  
	StateEnterProfileName
	StateEnterRole
	StateCreatingProfile
	StateProfileCreated
	StateProfileError
)

type AWSProfileCreationModel struct {
	accountID     string
	suggestedName string
	state         AWSProfileCreationState
	
	// SSO session selection
	ssoSessions     []SSOSession
	selectedSession int
	createNewSSO    bool
	
	// Input fields
	ssoURLInput      string
	ssoURLCursor     int
	ssoRegionInput   string
	ssoRegionCursor  int
	profileNameInput string
	profileNameCursor int
	roleInput        string
	roleCursor       int
	
	// Status
	createdProfile string
	errorMsg       string
	
	// UI
	width  int
	height int
}

func NewAWSProfileCreationModel(accountID, suggestedName string) AWSProfileCreationModel {
	m := AWSProfileCreationModel{
		accountID:        accountID,
		suggestedName:    suggestedName,
		profileNameInput: suggestedName,
		roleInput:        "AdministratorAccess",
		ssoRegionInput:   "us-east-1",
		state:           StateSelectSSO,
	}
	
	// Check for existing SSO sessions
	sessions, err := checkSSOSessions()
	if err == nil && len(sessions) > 0 {
		m.ssoSessions = sessions
		m.state = StateSelectSSO
	} else {
		// No sessions, go straight to creating one
		m.createNewSSO = true
		m.state = StateEnterSSOURL
	}
	
	return m
}

func (m AWSProfileCreationModel) Init() tea.Cmd {
	return nil
}

func (m AWSProfileCreationModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
		
	case profileCreatedMsg:
		if msg.error != nil {
			m.state = StateProfileError
			m.errorMsg = msg.error.Error()
		} else {
			m.state = StateProfileCreated
			m.createdProfile = msg.profileName
		}
		return m, nil
		
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			if m.state != StateProfileCreated {
				return m, tea.Quit
			}
			
		case tea.KeyEnter:
			return m.handleEnter()
			
		case tea.KeyUp:
			if m.state == StateSelectSSO && m.selectedSession > 0 {
				m.selectedSession--
			}
			
		case tea.KeyDown:
			if m.state == StateSelectSSO && m.selectedSession < len(m.ssoSessions) {
				m.selectedSession++
			}
			
		case tea.KeyBackspace:
			return m.handleBackspace()
			
		case tea.KeyLeft:
			return m.handleCursorLeft()
			
		case tea.KeyRight:
			return m.handleCursorRight()
			
		default:
			if msg.Type == tea.KeyRunes {
				return m.handleTextInput(string(msg.Runes))
			}
		}
	}
	
	return m, nil
}

func (m AWSProfileCreationModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateSelectSSO:
		if m.selectedSession < len(m.ssoSessions) {
			// Use existing session
			m.state = StateEnterProfileName
		} else {
			// Create new session
			m.createNewSSO = true
			m.state = StateEnterSSOURL
		}
		
	case StateEnterSSOURL:
		if m.ssoURLInput == "" {
			m.errorMsg = "SSO URL cannot be empty"
			return m, nil
		}
		m.errorMsg = ""
		m.state = StateEnterSSORegion
		
	case StateEnterSSORegion:
		if m.ssoRegionInput == "" {
			m.errorMsg = "SSO region cannot be empty"
			return m, nil
		}
		m.errorMsg = ""
		m.state = StateEnterProfileName
		
	case StateEnterProfileName:
		if m.profileNameInput == "" {
			m.errorMsg = "Profile name cannot be empty"
			return m, nil
		}
		m.errorMsg = ""
		m.state = StateEnterRole
		
	case StateEnterRole:
		if m.roleInput == "" {
			m.errorMsg = "Role name cannot be empty"
			return m, nil
		}
		m.errorMsg = ""
		m.state = StateCreatingProfile
		return m, m.createProfile
		
	case StateProfileCreated:
		return m, tea.Quit
		
	case StateProfileError:
		// Retry
		m.state = StateEnterProfileName
		m.errorMsg = ""
	}
	
	return m, nil
}

func (m AWSProfileCreationModel) handleBackspace() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateEnterSSOURL:
		if m.ssoURLCursor > 0 {
			m.ssoURLInput = m.ssoURLInput[:m.ssoURLCursor-1] + m.ssoURLInput[m.ssoURLCursor:]
			m.ssoURLCursor--
		}
	case StateEnterSSORegion:
		if m.ssoRegionCursor > 0 {
			m.ssoRegionInput = m.ssoRegionInput[:m.ssoRegionCursor-1] + m.ssoRegionInput[m.ssoRegionCursor:]
			m.ssoRegionCursor--
		}
	case StateEnterProfileName:
		if m.profileNameCursor > 0 {
			m.profileNameInput = m.profileNameInput[:m.profileNameCursor-1] + m.profileNameInput[m.profileNameCursor:]
			m.profileNameCursor--
		}
	case StateEnterRole:
		if m.roleCursor > 0 {
			m.roleInput = m.roleInput[:m.roleCursor-1] + m.roleInput[m.roleCursor:]
			m.roleCursor--
		}
	}
	return m, nil
}

func (m AWSProfileCreationModel) handleCursorLeft() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateEnterSSOURL:
		if m.ssoURLCursor > 0 {
			m.ssoURLCursor--
		}
	case StateEnterSSORegion:
		if m.ssoRegionCursor > 0 {
			m.ssoRegionCursor--
		}
	case StateEnterProfileName:
		if m.profileNameCursor > 0 {
			m.profileNameCursor--
		}
	case StateEnterRole:
		if m.roleCursor > 0 {
			m.roleCursor--
		}
	}
	return m, nil
}

func (m AWSProfileCreationModel) handleCursorRight() (tea.Model, tea.Cmd) {
	switch m.state {
	case StateEnterSSOURL:
		if m.ssoURLCursor < len(m.ssoURLInput) {
			m.ssoURLCursor++
		}
	case StateEnterSSORegion:
		if m.ssoRegionCursor < len(m.ssoRegionInput) {
			m.ssoRegionCursor++
		}
	case StateEnterProfileName:
		if m.profileNameCursor < len(m.profileNameInput) {
			m.profileNameCursor++
		}
	case StateEnterRole:
		if m.roleCursor < len(m.roleInput) {
			m.roleCursor++
		}
	}
	return m, nil
}

func (m AWSProfileCreationModel) handleTextInput(text string) (tea.Model, tea.Cmd) {
	switch m.state {
	case StateEnterSSOURL:
		m.ssoURLInput = m.ssoURLInput[:m.ssoURLCursor] + text + m.ssoURLInput[m.ssoURLCursor:]
		m.ssoURLCursor += len(text)
		
	case StateEnterSSORegion:
		m.ssoRegionInput = m.ssoRegionInput[:m.ssoRegionCursor] + text + m.ssoRegionInput[m.ssoRegionCursor:]
		m.ssoRegionCursor += len(text)
		
	case StateEnterProfileName:
		m.profileNameInput = m.profileNameInput[:m.profileNameCursor] + text + m.profileNameInput[m.profileNameCursor:]
		m.profileNameCursor += len(text)
		
	case StateEnterRole:
		m.roleInput = m.roleInput[:m.roleCursor] + text + m.roleInput[m.roleCursor:]
		m.roleCursor += len(text)
	}
	
	return m, nil
}

func (m AWSProfileCreationModel) View() string {
	switch m.state {
	case StateSelectSSO:
		return m.viewSelectSSO()
	case StateEnterSSOURL:
		return m.viewEnterSSOURL()
	case StateEnterSSORegion:
		return m.viewEnterSSORegion()
	case StateEnterProfileName:
		return m.viewEnterProfileName()
	case StateEnterRole:
		return m.viewEnterRole()
	case StateCreatingProfile:
		return m.viewCreatingProfile()
	case StateProfileCreated:
		return m.viewProfileCreated()
	case StateProfileError:
		return m.viewProfileError()
	default:
		return "Unknown state"
	}
}

func (m AWSProfileCreationModel) viewSelectSSO() string {
	var b strings.Builder
	
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("214"))
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))
	selectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("226"))
	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))
	
	b.WriteString(headerStyle.Render("Select SSO Session") + "\n")
	b.WriteString(descStyle.Render(fmt.Sprintf("For AWS Account %s", m.accountID)) + "\n\n")
	
	for i, session := range m.ssoSessions {
		prefix := "  "
		style := normalStyle
		if i == m.selectedSession {
			prefix = "> "
			style = selectedStyle
		}
		b.WriteString(prefix + style.Render(fmt.Sprintf("%s (https://%s)", session.Name, session.StartURL)) + "\n")
	}
	
	// Add option to create new
	prefix := "  "
	style := normalStyle
	if m.selectedSession == len(m.ssoSessions) {
		prefix = "> "
		style = selectedStyle
	}
	b.WriteString(prefix + style.Render("+ Create new SSO session") + "\n")
	
	b.WriteString("\n" + descStyle.Render("↑/↓ to navigate • Enter to select • Esc to cancel"))
	
	return b.String()
}

func (m AWSProfileCreationModel) viewEnterSSOURL() string {
	return m.renderInputView(
		"Enter SSO Start URL",
		"For AWS Account "+m.accountID,
		"SSO URL",
		m.ssoURLInput,
		m.ssoURLCursor,
		"https://myorg.awsapps.com/start",
		m.errorMsg,
	)
}

func (m AWSProfileCreationModel) viewEnterSSORegion() string {
	return m.renderInputView(
		"Enter SSO Region",
		"For AWS Account "+m.accountID,
		"Region",
		m.ssoRegionInput,
		m.ssoRegionCursor,
		"us-east-1",
		m.errorMsg,
	)
}

func (m AWSProfileCreationModel) viewEnterProfileName() string {
	return m.renderInputView(
		"Enter Profile Name",
		fmt.Sprintf("Name for AWS profile (account %s)", m.accountID),
		"Profile Name",
		m.profileNameInput,
		m.profileNameCursor,
		m.suggestedName,
		m.errorMsg,
	)
}

func (m AWSProfileCreationModel) viewEnterRole() string {
	return m.renderInputView(
		"Enter IAM Role",
		"For AWS Account "+m.accountID,
		"Role",
		m.roleInput,
		m.roleCursor,
		"AdministratorAccess",
		m.errorMsg,
	)
}

func (m AWSProfileCreationModel) renderInputView(title, desc, label, input string, cursor int, example, errorMsg string) string {
	var b strings.Builder
	
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("214"))
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("87"))
	inputStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFF")).
		Background(lipgloss.Color("235")).
		Padding(0, 1)
	exampleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true)
	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196"))
	linkStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Underline(true)
	
	// Header section with icon and title
	b.WriteString(headerStyle.Render("🔧 "+title) + "\n")
	b.WriteString(descStyle.Render(desc) + "\n")
	
	// Special case for SSO URL to show the link
	if title == "Enter SSO Start URL" {
		b.WriteString("> " + linkStyle.Render("mag") + " (" + linkStyle.Render("https://madappgang.awsapps.com/start") + ")\n")
	}
	
	b.WriteString("\n")
	
	// Input field with cursor
	inputLine := input
	if cursor < len(inputLine) {
		inputLine = inputLine[:cursor] + "█" + inputLine[cursor:]
	} else {
		inputLine = inputLine + "█"
	}
	
	b.WriteString(labelStyle.Render(label+": ") + inputStyle.Render(" "+inputLine+" ") + "\n")
	
	if example != "" {
		b.WriteString(exampleStyle.Render("Example: "+example) + "\n")
	}
	
	if errorMsg != "" {
		b.WriteString("\n" + errorStyle.Render("⚠️  "+errorMsg) + "\n")
	}
	
	b.WriteString("\n" + descStyle.Render("Enter to submit • ← → to move cursor • Esc to cancel"))
	
	return b.String()
}

func (m AWSProfileCreationModel) viewCreatingProfile() string {
	var b strings.Builder
	
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("214"))
	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))
	
	b.WriteString(headerStyle.Render("🔄 Creating AWS Profile...") + "\n\n")
	b.WriteString(statusStyle.Render("Setting up profile: ") + m.profileNameInput + "\n")
	b.WriteString(statusStyle.Render("For account: ") + m.accountID + "\n")
	
	return b.String()
}

func (m AWSProfileCreationModel) viewProfileCreated() string {
	var b strings.Builder
	
	successStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("82"))
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))
	
	b.WriteString(successStyle.Render("✅ AWS Profile Created Successfully!") + "\n\n")
	b.WriteString(descStyle.Render("Profile name: ") + m.createdProfile + "\n")
	b.WriteString(descStyle.Render("Account ID: ") + m.accountID + "\n\n")
	b.WriteString(descStyle.Render("Press Enter to continue..."))
	
	return b.String()
}

func (m AWSProfileCreationModel) viewProfileError() string {
	var b strings.Builder
	
	errorStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196"))
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))
	
	b.WriteString(errorStyle.Render("❌ Failed to Create Profile") + "\n\n")
	b.WriteString(descStyle.Render("Error: ") + m.errorMsg + "\n\n")
	b.WriteString(descStyle.Render("Press Enter to retry or Esc to cancel"))
	
	return b.String()
}

func (m AWSProfileCreationModel) createProfile() tea.Msg {
	var err error
	var profileName string
	
	// Create or use SSO session
	var sessionName string
	if m.createNewSSO {
		// Create new SSO session
		sessionName = fmt.Sprintf("sso-%s", m.profileNameInput)
		err = createSSOSessionDirectly(sessionName, m.ssoURLInput, m.ssoRegionInput)
		if err != nil {
			return profileCreatedMsg{error: fmt.Errorf("failed to create SSO session: %w", err)}
		}
	} else if m.selectedSession < len(m.ssoSessions) {
		// Use existing session
		sessionName = m.ssoSessions[m.selectedSession].Name
	}
	
	// Create the AWS profile
	profileName = m.profileNameInput
	err = createAWSProfileDirectly(profileName, sessionName, m.accountID, m.roleInput)
	if err != nil {
		return profileCreatedMsg{error: fmt.Errorf("failed to create AWS profile: %w", err)}
	}
	
	// Verify the profile works
	verifiedAccountID, err := getAWSAccountID(profileName)
	if err != nil || verifiedAccountID != m.accountID {
		return profileCreatedMsg{error: fmt.Errorf("profile verification failed")}
	}
	
	return profileCreatedMsg{
		profileName: profileName,
		error:       nil,
	}
}

type profileCreatedMsg struct {
	profileName string
	error       error
}