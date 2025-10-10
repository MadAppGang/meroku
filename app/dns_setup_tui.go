package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v2"
)

type DNSSetupState int

const (
	StateCheckExisting DNSSetupState = iota
	StateInputDomain
	StateSelectRootAccount
	StateCreateRootZone
	StateDisplayNameservers
	StateWaitPropagation
	StateCheckIAMPermissions // Check and fix IAM permissions
	StateDNSDetails          // New state for showing detailed DNS configuration
	StateComplete
	StateError
	StateValidateConfig
	StateSetupProduction   // New state for setting up production environment
	StateInputAccountID     // New state for inputting AWS account ID
	StateSetupAWSProfile    // New state for setting up AWS profile
	StateProcessingSetup    // New state for showing progress during setup
)

type DNSSetupModel struct {
	state                 DNSSetupState
	rootDomain            string
	domainInput           string
	cursorPos             int
	selectedProfile       string
	selectedAccountID     string
	environments          []DNSEnvironment
	nameservers           []string
	zoneID                string
	propagationStatus     map[string]bool
	spinner               spinner.Model
	progress              progress.Model
	currentStep           string
	currentStepIndex      int     // Track which step we're on
	setupSteps            []string // List of setup steps
	setupStepStatus       []bool   // Status of each step (completed or not)
	errorMsg              string
	dnsConfig             *DNSConfig
	envPermissions        map[string]bool // Maps environment name to whether it has IAM access
	missingPermissions    []string        // Environments missing IAM permissions
	checkingEnvironment   string          // Environment currently being checked for IAM permissions
	propagationCheckCount int
	delegationRoleArn     string
	width                 int
	height                int
	copiedToClipboard     bool
	copyFeedbackTimer     int
	animationFrame        int
	copiedNSIndex         int      // Which nameserver was copied (1-4, 0 = all)
	copiedNSTimer         int      // Timer for individual NS copy animation
	fixPermissions        bool     // Whether user wants to fix missing permissions
	propagationCheckTimer int      // Timer for auto-checking DNS propagation (counts down from 100)
	dnsPropagated         bool     // Whether DNS has been propagated
	actualNameservers     []string // Current nameservers returned by DNS query
	dnsCacheTTL           uint32   // TTL value from DNS response (indicates caching)
	isCheckingDNS         bool     // Whether we're currently checking DNS
	dnsTimerRunning       bool     // Whether a DNS propagation timer is already running
	accountIDInput        string   // AWS account ID input for production setup
	accountIDCursorPos    int      // Cursor position for account ID input
	dnsDebugLogs          []string // DNS debug logs for display
	isViewingExisting     bool     // Whether we're viewing existing config (not creating new)
	dnsDebugScrollOffset  int      // Scroll offset for DNS debug window
	showDNSDebugLog       bool     // Toggle to show/hide DNS debug log
}

type DNSEnvironment struct {
	Name      string
	Profile   string
	AccountID string
}

type dnsOperationMsg struct {
	Type    string
	Success bool
	Error   error
	Data    interface{}
}

type propagationStatusMsg struct {
	Status map[string]bool
}

type (
	animationTickMsg time.Time
	checkConfigMsg   struct{}
	dnsPropagationTickMsg time.Time
)

// animationTickCmd returns a command that ticks every 100ms for animations
func animationTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return animationTickMsg(t)
	})
}

// dnsPropagationTickCmd returns a command that ticks every second for DNS propagation countdown
func dnsPropagationTickCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return dnsPropagationTickMsg(t)
	})
}

func NewDNSSetupModel() DNSSetupModel {
	s := spinner.New()
	s.Spinner = spinner.Globe
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	p := progress.New(progress.WithDefaultGradient())
	
	envPermissions := make(map[string]bool)

	// Check if we're resuming from a saved state
	domain, accountID := loadTemporaryDNSState()
	
	model := DNSSetupModel{
		state:             StateCheckExisting,
		domainInput:       domain,
		cursorPos:         len(domain),
		spinner:           s,
		progress:          p,
		propagationStatus: make(map[string]bool),
		envPermissions:    envPermissions,
		currentStep:       "Initializing DNS setup wizard...",
	}
	
	// If we have saved state, skip directly to processing
	if domain != "" && accountID != "" {
		model.rootDomain = domain
		model.selectedAccountID = accountID
		model.accountIDInput = accountID
		// Jump directly to processing setup
		model.state = StateProcessingSetup
		model.setupSteps = []string{
			"Checking AWS profile",
			"Creating prod.yaml",
			"Creating DNS zone",
			"Setting up IAM role",
			"Saving configuration",
		}
		model.setupStepStatus = make([]bool, len(model.setupSteps))
		model.currentStepIndex = 0
	}
	
	return model
}

// saveTemporaryDNSState saves the DNS setup state for resumption
func saveTemporaryDNSState(domain, accountID string) {
	data := map[string]string{
		"domain":    domain,
		"accountID": accountID,
	}
	b, _ := yaml.Marshal(data)
	os.WriteFile(".dns_setup_temp.yaml", b, 0644)
}

// loadTemporaryDNSState loads saved DNS setup state
func loadTemporaryDNSState() (domain, accountID string) {
	data, err := os.ReadFile(".dns_setup_temp.yaml")
	if err != nil {
		return "", ""
	}
	
	var state map[string]string
	yaml.Unmarshal(data, &state)
	
	// Clean up temp file
	os.Remove(".dns_setup_temp.yaml")
	
	return state["domain"], state["accountID"]
}

func (m DNSSetupModel) Init() tea.Cmd {
	// If we're resuming from saved state, jump directly to profile check
	if m.state == StateProcessingSetup && m.selectedAccountID != "" {
		return tea.Batch(
			m.spinner.Tick,
			animationTickCmd(),
			m.findProductionProfile(),
		)
	}
	
	return tea.Batch(
		m.spinner.Tick,
		animationTickCmd(),
		// Delay the config check to allow animation to start
		tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
			return checkConfigMsg{}
		}),
	)
}

func (m DNSSetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = msg.Width - 4
		return m, nil

	case tea.KeyMsg:
		if m.state == StateInputDomain {
			return m.handleDomainInput(msg)
		}
		if m.state == StateInputAccountID {
			return m.handleAccountIDInput(msg)
		}
		switch msg.String() {
		case "ctrl+c", "q", "Q":
			return m, tea.Quit
		case "enter":
			return m.handleEnter()
		case "up", "k":
			return m.handleUp()
		case "down", "j":
			return m.handleDown()
		case "space":
			return m.handleSpace()
		case "b", "B":
			return m.handleBack()
		case "s", "S":
			return m.handleSkip()
		case " ":
			return m.handleSpace()
		case "r", "R":
			return m.handleRefresh()
		case "c", "C":
			return m.handleCopy()
		case "1", "2", "3", "4":
			// Copy individual nameserver
			if m.state == StateDisplayNameservers {
				return m.handleCopyIndividualNS(msg.String())
			}
		case "y", "Y":
			if m.state == StateCheckIAMPermissions && len(m.missingPermissions) > 0 {
				// Fix IAM permissions for missing environments
				m.currentStep = "Fixing IAM permissions..."
				m.errorMsg = "" // Clear any previous error
				// Start spinner for visual feedback
				cmd := m.spinner.Tick
				return m, tea.Batch(m.fixIAMPermissions(), cmd)
			}
			return m, nil
		case "d", "D":
			// Toggle DNS debug log
			if m.state == StateDisplayNameservers && len(m.dnsDebugLogs) > 0 {
				m.showDNSDebugLog = !m.showDNSDebugLog
				// Reset scroll position when opening - start from bottom
				if m.showDNSDebugLog {
					availableHeight := m.height - 5
					if availableHeight < 10 {
						availableHeight = 10
					}
					if len(m.dnsDebugLogs) > availableHeight {
						m.dnsDebugScrollOffset = len(m.dnsDebugLogs) - availableHeight
					} else {
						m.dnsDebugScrollOffset = 0
					}
				}
				// Force a full screen refresh by sending a clear screen command
				return m, tea.ClearScreen
			}
		case "esc":
			// Close debug log if open
			if m.state == StateDisplayNameservers && m.showDNSDebugLog {
				m.showDNSDebugLog = false
				// Force a full screen refresh
				return m, tea.ClearScreen
			}
		}

	case dnsOperationMsg:
		return m.handleDNSOperation(msg)

	case propagationStatusMsg:
		m.propagationStatus = msg.Status
		if m.state == StateWaitPropagation {
			allPropagated := true
			for _, status := range msg.Status {
				if !status {
					allPropagated = false
					break
				}
			}
			if allPropagated || m.propagationCheckCount > 30 {
				m.state = StateCheckIAMPermissions
				return m, tea.Batch(m.spinner.Tick, animationTickCmd(), m.checkIAMPermissions())
			}
			m.propagationCheckCount++
			return m, tea.Tick(time.Second*10, func(t time.Time) tea.Msg {
				return m.checkPropagation()
			})
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case dnsPropagationTickMsg:
		// Handle DNS propagation countdown timer (every second)
		if m.state == StateDisplayNameservers && !m.dnsPropagated {
			// Only decrement if not currently checking
			if !m.isCheckingDNS && m.propagationCheckTimer > 0 {
				m.propagationCheckTimer--
			}
			
			// Check when timer reaches 0 and not already checking
			if m.propagationCheckTimer == 0 && !m.isCheckingDNS {
				m.isCheckingDNS = true
				// Start the DNS check and continue the timer chain
				return m, tea.Batch(m.checkDNSPropagatedSimple(), dnsPropagationTickCmd())
			}
			
			// Continue the timer chain (only one chain running)
			return m, dnsPropagationTickCmd()
		}
		// Stop the timer if we've left the state or DNS has propagated
		m.dnsTimerRunning = false
		return m, nil
		
	case animationTickMsg:
		// Update animation frame
		m.animationFrame++

		// Track if we need one more render after clearing
		needsFinalRender := false
		var cmds []tea.Cmd

		// Decrement copy feedback timers
		if m.copyFeedbackTimer > 0 {
			m.copyFeedbackTimer--
			if m.copyFeedbackTimer == 0 {
				m.copiedToClipboard = false
				needsFinalRender = true
			}
		}
		if m.copiedNSTimer > 0 {
			m.copiedNSTimer--
			if m.copiedNSTimer == 0 {
				m.copiedNSIndex = 0
				needsFinalRender = true
			}
		}
		// No selection timer logic needed


		// Keep animation running for loading states or when showing copy feedback or selection animation
		// Also continue for one more tick if we just cleared something (needsFinalRender)
		// Or if we're checking DNS propagation
		// Or if we're checking IAM permissions (environments not loaded OR permissions incomplete OR fixing in progress)
		isCheckingIAM := len(m.environments) == 0 || len(m.envPermissions) < len(m.environments)-1 || m.currentStep != ""
		if m.state == StateCheckExisting || m.state == StateValidateConfig ||
			m.state == StateDisplayNameservers && (m.copiedNSTimer > 0 || m.copyFeedbackTimer > 0 || needsFinalRender || !m.dnsPropagated) ||
			m.state == StateCheckIAMPermissions && (needsFinalRender || isCheckingIAM) ||
			m.state == StateProcessingSetup {
			cmds = append(cmds, animationTickCmd())
		}

		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case checkConfigMsg:
		// Trigger the actual config check after animation has started
		if m.state == StateCheckExisting {
			return m, m.checkExistingConfig()
		}
		return m, nil
	}

	return m, nil
}

func (m DNSSetupModel) View() string {
	var content string

	switch m.state {
	case StateCheckExisting:
		content = m.viewCheckExisting()
	case StateInputDomain:
		content = m.viewInputDomain()
	case StateSelectRootAccount:
		content = m.viewSelectRootAccount()
	case StateCreateRootZone:
		content = m.viewCreateRootZone()
	case StateDisplayNameservers:
		content = m.viewDisplayNameservers()
	case StateWaitPropagation:
		content = m.viewWaitPropagation()
	case StateCheckIAMPermissions:
		content = m.viewCheckIAMPermissions()
	case StateDNSDetails:
		content = m.viewDNSDetails()
	case StateComplete:
		content = m.viewComplete()
	case StateError:
		content = m.viewError()
	case StateValidateConfig:
		content = m.viewValidateConfig()
	case StateSetupProduction:
		content = m.viewSetupProduction()
	case StateInputAccountID:
		content = m.viewInputAccountID()
	case StateProcessingSetup:
		content = m.viewProcessingSetup()
	case StateSetupAWSProfile:
		content = m.viewSetupAWSProfile()
	}

	// Use different titles based on state for clarity
	title := "DNS Custom Domain Setup"
	if m.state == StateCheckIAMPermissions {
		title = "IAM Permissions Check"
	} else if m.state == StateComplete {
		title = "DNS Setup Complete"
	}
	return m.renderBox(title, content)
}

func (m DNSSetupModel) renderBox(title, content string) string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("99")).
		Padding(1, 2).
		Width(m.width - 2)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("213")).
		Background(lipgloss.Color("56")).
		Padding(0, 1)

	return boxStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render(" 🌐 "+title+" "),
			"",
			content,
		),
	)
}

// Helper function to style keyboard shortcuts
func (m DNSSetupModel) renderKeyHelp(keys ...string) string {
	keyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#000")).
		Background(lipgloss.Color("226")).
		Padding(0, 1)
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("251"))
	dividerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	var result string
	for i := 0; i < len(keys); i += 2 {
		if i > 0 {
			result += dividerStyle.Render("  •  ")
		}
		if i+1 < len(keys) {
			result += keyStyle.Render(keys[i]) + textStyle.Render(" "+keys[i+1])
		}
	}
	return result
}

func (m DNSSetupModel) viewCheckExisting() string {
	welcomeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("87"))
	featureStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("156"))
	bulletStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214"))

	// Create animated loading box with gradient border
	borderColors := []string{"205", "206", "207", "171", "135", "99", "63"}
	borderColorIndex := (m.animationFrame / 2) % len(borderColors)
	loadingBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(borderColors[borderColorIndex])).
		Padding(1, 2).
		Width(50).
		Align(lipgloss.Center)

	// More compact DNS animation
	dnsFrames := []string{
		"[●                    ]",
		"[ ●                   ]",
		"[  ●                  ]",
		"[   ●                 ]",
		"[    ●                ]",
		"[     ●               ]",
		"[      ●              ]",
		"[       ●             ]",
		"[        ●            ]",
		"[         ●           ]",
		"[          ●          ]",
		"[           ●         ]",
		"[            ●        ]",
		"[             ●       ]",
		"[              ●      ]",
		"[               ●     ]",
		"[                ●    ]",
		"[                 ●   ]",
		"[                  ●  ]",
		"[                   ● ]",
		"[                    ●]",
	}

	// Fun rotating icons
	icons := []string{"🔍", "🔎", "🔬", "🔭", "🛰️", "📡", "🌍", "🌎", "🌏"}
	iconIndex := (m.animationFrame / 3) % len(icons)

	// Animated dots
	dots := []string{"   ", ".  ", ".. ", "..."}
	dotIndex := (m.animationFrame / 4) % len(dots)

	// Status messages that change
	statusMessages := []string{
		"Scanning DNS records",
		"Checking Route53 zones",
		"Validating configurations",
		"Analyzing DNS hierarchy",
		"Inspecting nameservers",
		"Querying AWS services",
	}
	statusIndex := (m.animationFrame / 10) % len(statusMessages)

	dnsFrameIndex := m.animationFrame % len(dnsFrames)

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Italic(true)

	var b strings.Builder
	b.WriteString(welcomeStyle.Render("🎆 Welcome to DNS Setup Wizard!") + "\n\n")
	b.WriteString("This wizard will help you:\n")
	b.WriteString(bulletStyle.Render("✓") + featureStyle.Render(" Set up a root domain") + "\n")
	b.WriteString(bulletStyle.Render("✓") + featureStyle.Render(" Configure subdomain delegations") + "\n")
	b.WriteString(bulletStyle.Render("✓") + featureStyle.Render(" Monitor DNS propagation") + "\n\n")

	// Create compact loading content
	progressBarStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("87"))

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("226")).
		Bold(true)

	// Simplified content that fits properly
	loadingContent := lipgloss.JoinVertical(
		lipgloss.Center,
		"",
		progressBarStyle.Render("🌐 "+dnsFrames[dnsFrameIndex]+" 🖥️ "),
		"",
		titleStyle.Render("Checking configuration"+dots[dotIndex]),
		"",
		statusStyle.Render(statusMessages[statusIndex]),
		"",
		icons[iconIndex],
		"",
	)

	b.WriteString(loadingBoxStyle.Render(loadingContent) + "\n\n")
	b.WriteString(m.renderKeyHelp("Q", "to quit"))
	return b.String()
}

func (m DNSSetupModel) viewInputDomain() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86"))
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))
	tipStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214"))
	bulletStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("99"))
	inputStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFF")).
		Background(lipgloss.Color("235")).
		Padding(0, 1)
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("87"))

	b.WriteString(headerStyle.Render("🌎 Enter your domain name:") + "\n\n")
	b.WriteString(descStyle.Render("This should be your root domain (e.g., example.com)") + "\n")
	b.WriteString(descStyle.Render("that you want to manage with AWS Route53.") + "\n\n")

	// Create input field with cursor
	inputLine := m.domainInput
	if m.cursorPos < len(inputLine) {
		inputLine = inputLine[:m.cursorPos] + "█" + inputLine[m.cursorPos:]
	} else {
		inputLine = inputLine + "█"
	}

	b.WriteString(labelStyle.Render("Domain: ") + inputStyle.Render(" "+inputLine+" ") + "\n\n")
	b.WriteString(tipStyle.Render("💡 Tips:") + "\n")
	b.WriteString(bulletStyle.Render("•") + descStyle.Render(" Enter domain without protocol (no https://)") + "\n")
	b.WriteString(bulletStyle.Render("•") + descStyle.Render(" Use your registered domain name") + "\n")
	b.WriteString(bulletStyle.Render("•") + descStyle.Render(" Subdomains will be created later") + "\n")
	b.WriteString(bulletStyle.Render("•") + descStyle.Render(" Paste with Cmd+V (Mac) or Ctrl+V (Linux/Windows)") + "\n\n")
	b.WriteString(m.renderKeyHelp("Enter", "to continue", "Esc/Q", "to quit"))

	return b.String()
}

func (m DNSSetupModel) viewSelectRootAccount() string {
	var b strings.Builder

	domainStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFF")).
		Background(lipgloss.Color("33")).
		Padding(0, 1)
	questionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("87"))
	selectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#000")).
		Background(lipgloss.Color("87")).
		Padding(0, 1)
	recommendedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("82")).
		Italic(true)
	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")).
		Bold(true)
	detailStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("246"))
	bulletStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("99"))

	b.WriteString("Domain: " + domainStyle.Render(" "+m.rootDomain+" ") + "\n\n")
	b.WriteString(questionStyle.Render("🔐 Configuring Production Account for Root DNS Zone") + "\n\n")

	if len(m.environments) > 0 {
		env := m.environments[0] // Should only be production
		if env.Name == "prod" {
			line := fmt.Sprintf("▸ %s (%s)", env.Name, env.AccountID)
			b.WriteString(selectedStyle.Render(line))
			b.WriteString(recommendedStyle.Render(" (Required)"))
			b.WriteString("\n")
		} else {
			// This shouldn't happen, but show error if it does
			errorStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("196")).
				Bold(true)
			b.WriteString(errorStyle.Render("⚠️  Error: Only production environment can host root zone"))
			b.WriteString("\n")
		}
	} else {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
		b.WriteString(errorStyle.Render("⚠️  Production environment not configured"))
		b.WriteString("\n")
	}

	b.WriteString("\n" + infoStyle.Render("ℹ️ The production account will:") + "\n")
	b.WriteString(bulletStyle.Render("▸") + detailStyle.Render(" Own the main domain zone") + "\n")
	b.WriteString(bulletStyle.Render("▸") + detailStyle.Render(" Manage all DNS delegations") + "\n")
	b.WriteString(bulletStyle.Render("▸") + detailStyle.Render(" Control subdomain NS records") + "\n\n")
	
	requiredStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("226")).
		Bold(true)
	b.WriteString(requiredStyle.Render("⚠️  Note: Root zone MUST be in production for security") + "\n\n")
	b.WriteString(m.renderKeyHelp("Enter", "to continue", "B", "to go back", "Q", "to quit"))

	return b.String()
}

func (m DNSSetupModel) viewCreateRootZone() string {
	var b strings.Builder
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("87")).
		Bold(true)
	domainStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("213")).
		Bold(true)
	accountStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214"))

	b.WriteString(headerStyle.Render("🚀 Creating DNS zone in AWS Route53:") + "\n\n")
	b.WriteString("Domain: " + domainStyle.Render(m.rootDomain) + "\n")

	accountInfo := m.selectedProfile
	if m.selectedAccountID != "" {
		accountInfo = fmt.Sprintf("%s (%s)", m.selectedProfile, m.selectedAccountID)
	}
	b.WriteString("Account: " + accountStyle.Render(accountInfo) + "\n\n")

	b.WriteString(headerStyle.Render("Progress:") + "\n")

	steps := []struct {
		name string
		done bool
	}{
		{"Creating hosted zone", m.zoneID != ""},
		{"Retrieving nameservers", len(m.nameservers) > 0},
		{"Creating delegation IAM role", m.delegationRoleArn != ""},
		{"Saving configuration", false},
	}

	for _, step := range steps {
		if step.done {
			b.WriteString(fmt.Sprintf("✓ %s\n", step.name))
		} else if m.currentStep == step.name {
			b.WriteString(fmt.Sprintf("%s %s\n", m.spinner.View(), step.name))
		} else {
			b.WriteString(fmt.Sprintf("○ %s\n", step.name))
		}
	}

	if len(m.nameservers) > 0 {
		b.WriteString("\nNameservers retrieved:\n")
		for _, ns := range m.nameservers {
			b.WriteString(fmt.Sprintf("• %s\n", ns))
		}
	}

	return b.String()
}

// viewDNSDebugFullscreen displays the DNS debug log in fullscreen mode
func (m DNSSetupModel) viewDNSDebugFullscreen() string {
	var b strings.Builder
	
	// Clear screen first by filling with spaces if needed
	// This ensures no leftover content from previous view
	
	// Styles for fullscreen debug view
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("87")).
		Background(lipgloss.Color("235")).
		Width(m.width).
		Padding(0, 2)
	
	contentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))
	
	lineNumberStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))
	
	highlightStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("226"))
	
	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196"))
	
	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("82"))
	
	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("33"))
	
	// Use a safe width that accounts for terminal size
	safeWidth := m.width
	if safeWidth > 120 {
		safeWidth = 120 // Cap at reasonable width
	}
	if safeWidth < 80 {
		safeWidth = 80 // Minimum width
	}
	
	// Header
	b.WriteString(headerStyle.Render("🔍 DNS Query Debug Log - Full View") + "\n")
	b.WriteString(strings.Repeat("─", safeWidth) + "\n")
	
	// Calculate how many lines we can display (leave room for header and footer)
	availableHeight := m.height - 5 // Reserve lines for header and footer
	if availableHeight < 10 {
		availableHeight = 10
	}
	
	// Determine the range of logs to show
	startIdx := m.dnsDebugScrollOffset
	endIdx := startIdx + availableHeight
	if endIdx > len(m.dnsDebugLogs) {
		endIdx = len(m.dnsDebugLogs)
		// Adjust start if we're at the end
		if endIdx-startIdx < availableHeight && len(m.dnsDebugLogs) > availableHeight {
			startIdx = len(m.dnsDebugLogs) - availableHeight
			if startIdx < 0 {
				startIdx = 0
			}
		}
	}
	
	// Display logs with syntax highlighting
	for i := startIdx; i < endIdx; i++ {
		lineNum := fmt.Sprintf("%4d │ ", i+1)
		b.WriteString(lineNumberStyle.Render(lineNum))
		
		line := m.dnsDebugLogs[i]
		
		// Apply syntax highlighting based on content
		switch {
		case strings.Contains(line, "Response code: NOERROR"):
			// NOERROR is a success response
			b.WriteString(successStyle.Render(line))
		case strings.Contains(line, "ERROR") && !strings.Contains(line, "NOERROR") || strings.Contains(line, "❌"):
			b.WriteString(errorStyle.Render(line))
		case strings.Contains(line, "✅") || strings.Contains(line, "Success") || strings.Contains(line, "✓"):
			b.WriteString(successStyle.Render(line))
		case strings.Contains(line, "---") || strings.Contains(line, "==="):
			b.WriteString(highlightStyle.Render(line))
		case strings.HasPrefix(line, "Expected") || strings.HasPrefix(line, "Actual"):
			b.WriteString(infoStyle.Render(line))
		case strings.Contains(line, "Query:") || strings.Contains(line, "Response:") && !strings.Contains(line, "NOERROR"):
			b.WriteString(highlightStyle.Render(line))
		case strings.Contains(line, "Found") && strings.Contains(line, "NS records"):
			b.WriteString(successStyle.Render(line))
		case strings.Contains(line, "TCP failed"):
			b.WriteString(errorStyle.Render(line))
		default:
			b.WriteString(contentStyle.Render(line))
		}
		b.WriteString("\n")
	}
	
	// Fill remaining space with empty lines
	for i := endIdx - startIdx; i < availableHeight; i++ {
		b.WriteString("\n")
	}
	
	// Footer with scroll info and controls
	b.WriteString(strings.Repeat("─", safeWidth) + "\n")
	
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))
	
	scrollInfo := fmt.Sprintf("Lines %d-%d of %d", startIdx+1, endIdx, len(m.dnsDebugLogs))
	scrollPercent := 0
	if len(m.dnsDebugLogs) > availableHeight {
		scrollPercent = (startIdx * 100) / (len(m.dnsDebugLogs) - availableHeight)
	}
	scrollBar := fmt.Sprintf(" [%d%%]", scrollPercent)
	
	controls := "↑↓: Scroll │ D: Close │ ESC: Back"
	
	// Build footer content that fits within width
	footerContent := fmt.Sprintf("%s%s │ %s", scrollInfo, scrollBar, controls)
	
	// Truncate if too long
	if len(footerContent) > safeWidth {
		footerContent = footerContent[:safeWidth-3] + "..."
	}
	
	b.WriteString(footerStyle.Render(footerContent))
	
	return b.String()
}

func (m DNSSetupModel) viewDisplayNameservers() string {
	// If showing debug log fullscreen, display only that
	if m.showDNSDebugLog && len(m.dnsDebugLogs) > 0 {
		return m.viewDNSDebugFullscreen()
	}
	
	var b strings.Builder

	warningStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("226")).
		Background(lipgloss.Color("52")).
		Padding(0, 1)
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("87")).
		Bold(true)
	nsBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("33")).
		Padding(1).
		Width(50)
	nsStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("123")).
		Bold(true)
	nsHighlightStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#000")).
		Background(lipgloss.Color("82")).
		Bold(true)
	numberStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")).
		Bold(true)
	copyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#000")).
		Background(lipgloss.Color("226")).
		Padding(0, 1)
	successStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("82")).
		Background(lipgloss.Color("22")).
		Padding(0, 1)
	registrarTitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")).
		Bold(true)
	registrarStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))
	bulletStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("99"))

	b.WriteString(warningStyle.Render("⚠️  Action Required!") + "\n\n")
	b.WriteString(headerStyle.Render("Update your domain registrar with these nameservers:") + "\n\n")

	// Create nameserver list with numbered formatting and highlight animation
	var nsList strings.Builder
	for i, ns := range m.nameservers {
		if i > 0 {
			nsList.WriteString("\n")
		}

		// Add number prefix
		number := fmt.Sprintf("%d. ", i+1)
		nsList.WriteString(numberStyle.Render(number))

		// Check if this NS is currently highlighted
		if m.copiedNSIndex == i+1 && m.copiedNSTimer > 0 {
			// Highlight this nameserver
			nsList.WriteString(nsHighlightStyle.Render(ns))
		} else {
			// Normal display
			nsList.WriteString(nsStyle.Render(ns))
		}
	}
	b.WriteString(nsBoxStyle.Render(nsList.String()) + "\n\n")

	// Show copy status
	if m.copiedNSIndex > 0 && m.copiedNSTimer > 0 {
		b.WriteString(successStyle.Render(fmt.Sprintf("✅ Copied nameserver %d to clipboard!", m.copiedNSIndex)) + "\n\n")
	} else if m.copiedToClipboard && m.copyFeedbackTimer > 0 {
		b.WriteString(successStyle.Render("✅ Successfully copied all nameservers to clipboard!") + "\n\n")
	} else {
		b.WriteString(copyStyle.Render("📋 Press C to copy all | Press 1-4 to copy individual") + "\n\n")
	}

	b.WriteString(registrarTitleStyle.Render("📍 Common Registrars:") + "\n")
	b.WriteString(bulletStyle.Render("▸") + registrarStyle.Render(" GoDaddy: DNS > Nameservers") + "\n")
	b.WriteString(bulletStyle.Render("▸") + registrarStyle.Render(" Namecheap: Domain > Nameservers") + "\n")
	b.WriteString(bulletStyle.Render("▸") + registrarStyle.Render(" Cloudflare: DNS > Nameservers") + "\n\n")

	// Show DNS propagation status
	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Italic(true)
	propagatedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("82")).
		Bold(true)
	notPropagatedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")).
		Bold(true)
	wrongNSStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196"))

	if m.dnsPropagated {
		b.WriteString(propagatedStyle.Render("✅ DNS has propagated! Proceeding...") + "\n\n")
	} else {
		timeInfo := ""
		if m.isCheckingDNS {
			timeInfo = " (checking now...)"
		} else if m.propagationCheckTimer > 0 {
			seconds := m.propagationCheckTimer
			timeInfo = fmt.Sprintf(" (next check in %ds)", seconds)
		}
		b.WriteString(notPropagatedStyle.Render("⏳ Checking DNS propagation...") + statusStyle.Render(timeInfo) + "\n")

		// Show current NS values
		if len(m.actualNameservers) > 0 {
			b.WriteString("\n" + statusStyle.Render("Current nameservers from DNS:") + "\n")
			
			// Show TTL warning if present (only for standard DNS)
			if m.dnsCacheTTL > 0 {
				ttlWarningStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("226")).
					Bold(true)
				hours := m.dnsCacheTTL / 3600
				minutes := (m.dnsCacheTTL % 3600) / 60
				
				if hours > 0 {
					b.WriteString(ttlWarningStyle.Render(fmt.Sprintf("⚠️ CACHED RESPONSE (TTL: %dh %dm remaining)", hours, minutes)) + "\n")
				} else {
					b.WriteString(ttlWarningStyle.Render(fmt.Sprintf("⚠️ CACHED RESPONSE (TTL: %d minutes remaining)", minutes)) + "\n")
				}
				b.WriteString(statusStyle.Render("DNS servers are returning cached values that won't update until TTL expires") + "\n\n")
			} else {
				// TTL is 0, meaning we got DoH results
				dohSuccessStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("82")).
					Italic(true)
				b.WriteString(dohSuccessStyle.Render("✓ Using DNS-over-HTTPS (real-time results)") + "\n\n")
			}
			
			for _, ns := range m.actualNameservers {
				// Check if this NS matches any expected
				isMatch := false
				for _, expected := range m.nameservers {
					if strings.EqualFold(strings.TrimSuffix(ns, "."), strings.TrimSuffix(expected, ".")) {
						isMatch = true
						break
					}
				}
				if isMatch {
					b.WriteString(propagatedStyle.Render("  ✓ "+ns) + "\n")
				} else {
					b.WriteString(wrongNSStyle.Render("  ✗ "+ns) + "\n")
				}
			}
		} else if !m.isCheckingDNS && m.propagationCheckTimer == 0 {
			// Only show "No nameservers" if we're not currently checking and the timer has reached 0
			// This means we've completed at least one check with no results
			b.WriteString("\n" + wrongNSStyle.Render("❌ No nameservers returned by DNS query") + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(m.renderKeyHelp("C", "copy all", "1-4", "copy NS", "R", "refresh", "D", "debug log", "Enter", "done"))

	// Show a small indicator that debug logs are available
	if !m.showDNSDebugLog && len(m.dnsDebugLogs) > 0 {
		b.WriteString("\n\n")
		infoStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true)
		b.WriteString(infoStyle.Render(fmt.Sprintf("💡 %d debug log entries available. Press D to view.", len(m.dnsDebugLogs))))
	}

	return b.String()
}

func (m DNSSetupModel) viewWaitPropagation() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Checking DNS propagation for %s\n\n", m.rootDomain))
	b.WriteString("DNS Servers Status:\n")

	servers := map[string]string{
		"Google DNS (8.8.8.8)":     "8.8.8.8",
		"Cloudflare (1.1.1.1)":     "1.1.1.1",
		"Quad9 (9.9.9.9)":          "9.9.9.9",
		"OpenDNS (208.67.222.222)": "208.67.222.222",
	}

	successCount := 0
	for name, ip := range servers {
		if status, ok := m.propagationStatus[ip]; ok {
			if status {
				b.WriteString(fmt.Sprintf("✓ %s\n", name))
				successCount++
			} else {
				b.WriteString(fmt.Sprintf("✗ %s\n", name))
			}
		} else {
			b.WriteString(fmt.Sprintf("%s %s\n", m.spinner.View(), name))
		}
	}

	b.WriteString(fmt.Sprintf("\n📊 Success Rate: %d/%d servers\n", successCount, len(servers)))
	b.WriteString(fmt.Sprintf("⏱ Check attempts: %d\n\n", m.propagationCheckCount))
	b.WriteString("💡 DNS propagation typically takes 5-30 minutes globally\n\n")
	b.WriteString("Press R to refresh • Press Enter to continue • Press B to go back")

	return b.String()
}

func (m DNSSetupModel) viewCheckIAMPermissions() string {
	var b strings.Builder

	// Styles
	successBadgeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#000")).
		Background(lipgloss.Color("82")).
		Padding(0, 1)
	warningBadgeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#000")).
		Background(lipgloss.Color("214")).
		Padding(0, 1)
	domainStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("226"))
	sectionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Bold(true)
	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("82"))
	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196"))
	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))
	fixStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("220")).
		Bold(true)

	// Show root zone status (more compact)
	b.WriteString(successBadgeStyle.Render("✓ ROOT ZONE CONFIGURED") + "\n\n")
	b.WriteString("Domain: " + domainStyle.Render(m.rootDomain) + " | Production Account: " + infoStyle.Render(m.selectedProfile) + "\n\n")

	// Check IAM permissions section (more compact title)
	b.WriteString(sectionStyle.Render("Checking IAM Permissions for Non-Production Environments") + "\n")
	
	if len(m.environments) <= 1 {
		// Only production environment
		b.WriteString("\n" + infoStyle.Render("No non-production environments. IAM permissions not needed.") + "\n\n")
		b.WriteString(m.renderKeyHelp("Enter", "continue", "Q", "quit"))
		return b.String()
	}

	// Show permission status for each environment (more compact)
	b.WriteString("\nEnvironment Access Status:\n")

	hasIssues := false
	for _, env := range m.environments {
		if env.Name == "prod" {
			continue // Skip production
		}

		envDisplay := fmt.Sprintf("%s.%s", env.Name, m.rootDomain)

		// Check if this environment is currently being checked
		if m.checkingEnvironment == env.Name {
			b.WriteString("  " + m.spinner.View() + " " + envDisplay + " - " +
				infoStyle.Render("Checking IAM permissions...") + "\n")
		} else if hasAccess, ok := m.envPermissions[env.Name]; ok {
			if hasAccess {
				b.WriteString(successStyle.Render("  ✓ ") + envDisplay + " - " +
					successStyle.Render("Has IAM access") + "\n")
			} else {
				b.WriteString(errorStyle.Render("  ✗ ") + envDisplay + " - " +
					errorStyle.Render("Missing IAM access") + "\n")
				hasIssues = true
			}
		} else {
			// Not yet checked - show waiting state
			b.WriteString(infoStyle.Render("  ⋯ ") + envDisplay + " - " +
				infoStyle.Render("Waiting...") + "\n")
		}
	}
	b.WriteString("\n")
	
	// Show fix option if there are issues
	if hasIssues && len(m.missingPermissions) > 0 {
		b.WriteString("\n" + warningBadgeStyle.Render("⚠️  IAM PERMISSIONS MISSING") + "\n\n")
		b.WriteString("The following environments cannot manage their subdomains:\n")
		for _, env := range m.missingPermissions {
			b.WriteString("  • " + errorStyle.Render(env) + "\n")
		}
		
		// Show current step if fixing
		if m.currentStep != "" && strings.Contains(m.currentStep, "Fixing") {
			b.WriteString(m.spinner.View() + " " + infoStyle.Render(m.currentStep) + "\n\n")
			b.WriteString(infoStyle.Render("Creating/updating dns-delegation-role...") + "\n")
			b.WriteString(infoStyle.Render("Adding trust policy for account: ") + errorStyle.Render(strings.Join(m.missingPermissions, ", ")) + "\n")
		} else if m.errorMsg != "" {
			// Show error if fix failed
			b.WriteString(errorStyle.Render("❌ Error: " + m.errorMsg) + "\n\n")
			b.WriteString(fixStyle.Render("Would you like to retry fixing permissions?") + "\n")
			b.WriteString(m.renderKeyHelp("Y", "retry fix", "S", "skip", "Q", "quit"))
		} else {
			b.WriteString(fixStyle.Render("Would you like to fix these permissions?") + "\n")
			b.WriteString(infoStyle.Render("This will update the delegation role trust policy.") + "\n\n")
			b.WriteString(m.renderKeyHelp("Y", "fix permissions", "S", "skip", "Q", "quit"))
		}
	} else if !hasIssues && len(m.envPermissions) > 0 {
		// All permissions OK
		b.WriteString("\n" + successBadgeStyle.Render("✅ ALL PERMISSIONS CONFIGURED") + "\n\n")
		if m.isViewingExisting {
			b.WriteString(m.renderKeyHelp("Enter", "view DNS details", "Q", "quit"))
		} else {
			b.WriteString(m.renderKeyHelp("Enter", "continue", "Q", "quit"))
		}
	} else {
		// Still checking - show loading indicator
		b.WriteString("\n" + m.spinner.View() + " " + infoStyle.Render("Checking IAM permissions...") + "\n\n")
		b.WriteString(m.renderKeyHelp("Q", "quit"))
	}
	
	return b.String()
}

func (m DNSSetupModel) viewDNSDetails() string {
	var b strings.Builder

	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#000")).
		Background(lipgloss.Color("39")).
		Padding(0, 1)
	sectionTitleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("213")).
		MarginTop(1)
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))
	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("226")).
		Bold(true)
	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("82"))
	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	// Header
	b.WriteString(titleStyle.Render("📋 DNS Configuration Details") + "\n\n")

	// Root Zone Configuration
	b.WriteString(sectionTitleStyle.Render("🌐 Root Zone") + "\n")
	b.WriteString(labelStyle.Render("Domain:         ") + valueStyle.Render(m.rootDomain) + "\n")
	b.WriteString(labelStyle.Render("Zone ID:        ") + valueStyle.Render(m.zoneID) + "\n")
	b.WriteString(labelStyle.Render("AWS Account:    ") + valueStyle.Render(m.selectedProfile) + "\n")

	// Nameservers
	b.WriteString("\n" + sectionTitleStyle.Render("📡 Nameservers") + "\n")
	if len(m.nameservers) > 0 {
		for i, ns := range m.nameservers {
			b.WriteString(fmt.Sprintf("%s NS%d: %s\n", successStyle.Render("✓"), i+1, valueStyle.Render(ns)))
		}
	} else {
		b.WriteString(infoStyle.Render("No nameservers configured\n"))
	}

	// IAM Delegation Role
	b.WriteString("\n" + sectionTitleStyle.Render("🔐 IAM Delegation") + "\n")
	if m.delegationRoleArn != "" {
		b.WriteString(successStyle.Render("✓ Configured\n"))
		b.WriteString(labelStyle.Render("Role ARN: ") + infoStyle.Render(m.delegationRoleArn) + "\n")
	} else {
		b.WriteString(infoStyle.Render("No delegation role configured\n"))
	}

	// Environments
	b.WriteString("\n" + sectionTitleStyle.Render("🏗️  Environments") + "\n")
	if len(m.environments) > 0 {
		for _, env := range m.environments {
			subdomain := env.Name
			if env.Name == "prod" {
				subdomain = m.rootDomain
			} else {
				subdomain = fmt.Sprintf("%s.%s", env.Name, m.rootDomain)
			}

			hasAccess := ""
			if access, ok := m.envPermissions[env.Name]; ok {
				if access {
					hasAccess = successStyle.Render(" ✓ IAM access")
				} else {
					hasAccess = infoStyle.Render(" ✗ No IAM access")
				}
			}

			b.WriteString(fmt.Sprintf("  %s %s%s\n",
				successStyle.Render("•"),
				valueStyle.Render(subdomain),
				hasAccess))
		}
	} else {
		b.WriteString(infoStyle.Render("No environments configured\n"))
	}

	// DNS Propagation Status
	if m.dnsPropagated {
		b.WriteString("\n" + successStyle.Render("✅ DNS fully propagated globally") + "\n")
	}

	b.WriteString("\n" + labelStyle.Render("Press ") + valueStyle.Render("Q") + labelStyle.Render(" to exit"))

	return b.String()
}

func (m DNSSetupModel) viewComplete() string {
	var b strings.Builder

	// Styles
	successBannerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#000")).
		Background(lipgloss.Color("82")).
		Padding(0, 2).
		Align(lipgloss.Center).
		Width(60)
	
	sectionTitleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("213")).
		Background(lipgloss.Color("57")).
		Padding(0, 1).
		MarginTop(1)
	
	domainStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("226"))
	
	zoneIDStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("87"))
	
	nsBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("82")).
		Padding(0, 1).
		Width(60)
	
	nsStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("159"))
	
	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("82"))
	
	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))
	
	stepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))
	
	commandStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("220")).
		Bold(true)
	
	acccessStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("99"))
	
	// Success Banner
	b.WriteString(successBannerStyle.Render("🎉 DNS SETUP COMPLETE! 🎉") + "\n\n")
	
	// Root Zone Information
	b.WriteString(sectionTitleStyle.Render("🌐 DNS Zone Configuration") + "\n\n")
	b.WriteString("Domain: " + domainStyle.Render(m.rootDomain) + "\n")
	b.WriteString("Zone ID: " + zoneIDStyle.Render(m.zoneID) + "\n")
	
	if m.selectedProfile != "" {
		b.WriteString("AWS Profile: " + infoStyle.Render(m.selectedProfile) + "\n")
	}
	if m.selectedAccountID != "" {
		b.WriteString("AWS Account: " + infoStyle.Render(m.selectedAccountID) + "\n")
	}
	b.WriteString("\n")
	
	// Nameservers
	b.WriteString(sectionTitleStyle.Render("📡 Active Nameservers") + "\n\n")
	if len(m.nameservers) > 0 {
		var nsList strings.Builder
		for i, ns := range m.nameservers {
			nsList.WriteString(successStyle.Render("✓ ") + nsStyle.Render(ns))
			if i < len(m.nameservers)-1 {
				nsList.WriteString("\n")
			}
		}
		b.WriteString(nsBoxStyle.Render(nsList.String()) + "\n\n")
		
		// DNS Propagation Status
		if m.dnsPropagated {
			b.WriteString(successStyle.Render("✅ DNS fully propagated globally") + "\n")
		} else {
			b.WriteString(infoStyle.Render("⏳ DNS propagation in progress...") + "\n")
		}
	} else {
		b.WriteString(infoStyle.Render("No nameservers configured") + "\n")
	}
	b.WriteString("\n")
	
	// IAM Configuration
	b.WriteString(sectionTitleStyle.Render("🔐 IAM Cross-Account Access") + "\n\n")
	
	if m.delegationRoleArn != "" {
		b.WriteString(successStyle.Render("✓") + " Delegation Role: " + acccessStyle.Render("Configured") + "\n")
		b.WriteString(infoStyle.Render("   Allows subdomain environments to create NS records") + "\n")
	} else {
		b.WriteString(infoStyle.Render("No cross-account IAM role configured") + "\n")
		b.WriteString(infoStyle.Render("Subdomain environments will manage zones independently") + "\n")
	}
	b.WriteString("\n")
	
	// Configuration Files
	b.WriteString(sectionTitleStyle.Render("📁 Configuration Status") + "\n\n")
	b.WriteString(successStyle.Render("✓") + " DNS config saved to: " + commandStyle.Render(DNSConfigFile) + "\n")
	
	// Check for environment files
	envFiles := []string{"prod.yaml", "dev.yaml", "staging.yaml"}
	for _, file := range envFiles {
		if _, err := os.Stat(file); err == nil {
			b.WriteString(successStyle.Render("✓") + " Environment file: " + commandStyle.Render(file) + "\n")
		}
	}
	b.WriteString("\n")
	
	// Next Steps
	b.WriteString(sectionTitleStyle.Render("🚀 Next Steps") + "\n\n")
	b.WriteString(stepStyle.Render("1.") + " Generate Terraform configuration:\n")
	b.WriteString("   " + commandStyle.Render("make infra-gen-prod") + "\n")
	b.WriteString("   " + commandStyle.Render("make infra-gen-dev") + " (if dev environment exists)\n\n")
	
	b.WriteString(stepStyle.Render("2.") + " Initialize and plan infrastructure:\n")
	b.WriteString("   " + commandStyle.Render("make infra-init env=prod") + "\n")
	b.WriteString("   " + commandStyle.Render("make infra-plan env=prod") + "\n\n")
	
	b.WriteString(stepStyle.Render("3.") + " Apply infrastructure changes:\n")
	b.WriteString("   " + commandStyle.Render("make infra-apply env=prod") + "\n\n")
	
	b.WriteString(stepStyle.Render("4.") + " Monitor DNS status:\n")
	b.WriteString("   " + commandStyle.Render("./meroku dns status") + "\n\n")
	
	// Footer
	b.WriteString(m.renderKeyHelp("Enter", "exit", "Q", "quit"))
	
	return b.String()
}

func (m DNSSetupModel) viewValidateConfig() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("87"))
	domainStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("213")).
		Bold(true)
	spinnerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205"))

	b.WriteString(headerStyle.Render("🔍 Validating existing DNS configuration...") + "\n\n")
	b.WriteString("Domain: " + domainStyle.Render(m.rootDomain) + "\n")
	b.WriteString("Zone ID: " + m.zoneID + "\n\n")
	b.WriteString(spinnerStyle.Render(m.spinner.View()) + " Checking if zone exists in AWS Route53...\n\n")
	b.WriteString("This may take a few seconds...")

	return b.String()
}

func (m DNSSetupModel) viewError() string {
	var b strings.Builder

	errorBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("196")).
		Padding(1, 2).
		Width(70)

	errorTitleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196")).
		MarginBottom(1)

	errorMsgStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		MarginTop(1)

	content := errorTitleStyle.Render("❌ Configuration Error") + "\n\n" +
		errorMsgStyle.Render(m.errorMsg) + "\n\n" +
		helpStyle.Render("Press R to retry • Press Q to exit")

	b.WriteString(errorBoxStyle.Render(content))

	return b.String()
}

func (m DNSSetupModel) viewSetupProduction() string {
	var b strings.Builder
	
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("214"))
	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("87"))
	explanationStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))
	exampleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("123")).
		Italic(true)
	importantStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("226"))
	bulletStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("99"))
	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true)
	domainStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFF")).
		Background(lipgloss.Color("33")).
		Padding(0, 1)
	
	b.WriteString(headerStyle.Render("🏗️  Production Environment Setup Required") + "\n\n")
	b.WriteString("Domain: " + domainStyle.Render(" "+m.rootDomain+" ") + "\n\n")
	
	b.WriteString(sectionStyle.Render("📖 DNS Architecture Overview:") + "\n")
	b.WriteString(explanationStyle.Render("Our DNS management follows a hub-and-spoke model:") + "\n\n")
	
	b.WriteString(bulletStyle.Render("•") + explanationStyle.Render(" Production account hosts the root DNS zone (") + exampleStyle.Render(m.rootDomain) + explanationStyle.Render(")") + "\n")
	b.WriteString(bulletStyle.Render("•") + explanationStyle.Render(" Other environments receive delegated subdomains") + "\n")
	b.WriteString(bulletStyle.Render("•") + explanationStyle.Render(" Cross-account IAM roles enable secure delegation") + "\n\n")
	
	b.WriteString(sectionStyle.Render("🌐 Domain Naming Convention:") + "\n\n")
	b.WriteString(explanationStyle.Render("Base domains:") + "\n")
	b.WriteString(bulletStyle.Render("  Production: ") + exampleStyle.Render(m.rootDomain) + "\n")
	b.WriteString(bulletStyle.Render("  Development: ") + exampleStyle.Render("dev."+m.rootDomain) + "\n")
	b.WriteString(bulletStyle.Render("  Staging: ") + exampleStyle.Render("staging."+m.rootDomain) + dimStyle.Render(" (managed separately)") + "\n\n")
	
	b.WriteString(explanationStyle.Render("Service endpoints (examples):") + "\n")
	b.WriteString(bulletStyle.Render("  APP: ") + exampleStyle.Render("app."+m.rootDomain) + explanationStyle.Render(" / ") + exampleStyle.Render("app.dev."+m.rootDomain) + "\n")
	b.WriteString(bulletStyle.Render("  API: ") + exampleStyle.Render("api."+m.rootDomain) + explanationStyle.Render(" / ") + exampleStyle.Render("api.dev."+m.rootDomain) + "\n")
	b.WriteString(bulletStyle.Render("  Landing: ") + exampleStyle.Render(m.rootDomain) + explanationStyle.Render(" / ") + exampleStyle.Render("dev."+m.rootDomain) + "\n\n")
	
	b.WriteString(importantStyle.Render("⚠️  Important:") + "\n")
	b.WriteString(explanationStyle.Render("• Root zone MUST be in production for security") + "\n")
	b.WriteString(explanationStyle.Render("• This will create DNS zone only (no deployment)") + "\n")
	b.WriteString(explanationStyle.Render("• You'll update your registrar's nameservers later") + "\n\n")
	
	b.WriteString(sectionStyle.Render("To continue, we need your production AWS account ID.") + "\n\n")
	b.WriteString(m.renderKeyHelp("Enter", "provide account ID", "Q", "to quit"))
	
	return b.String()
}

func (m DNSSetupModel) viewSetupAWSProfile() string {
	var b strings.Builder
	
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("214"))
	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("87"))
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))
	highlightStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("226"))
	exampleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("123")).
		Italic(true)
	bulletStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("99"))
	
	b.WriteString(headerStyle.Render("🔧 AWS Profile Setup Required") + "\n\n")
	b.WriteString(descStyle.Render("No AWS profile found for account ID: ") + highlightStyle.Render(m.selectedAccountID) + "\n\n")
	
	b.WriteString(sectionStyle.Render("What will happen next:") + "\n\n")
	
	b.WriteString(bulletStyle.Render("1.") + descStyle.Render(" AWS profile creation wizard will launch") + "\n")
	b.WriteString(bulletStyle.Render("2.") + descStyle.Render(" You'll set up SSO session or credentials") + "\n")
	b.WriteString(bulletStyle.Render("3.") + descStyle.Render(" Profile will be configured for account ") + highlightStyle.Render(m.selectedAccountID) + "\n")
	b.WriteString(bulletStyle.Render("4.") + descStyle.Render(" DNS setup will continue automatically") + "\n\n")
	
	b.WriteString(sectionStyle.Render("You'll be asked for:") + "\n")
	b.WriteString(bulletStyle.Render("•") + descStyle.Render(" SSO start URL (e.g., ") + exampleStyle.Render("https://myorg.awsapps.com/start") + descStyle.Render(")") + "\n")
	b.WriteString(bulletStyle.Render("•") + descStyle.Render(" SSO region (e.g., ") + exampleStyle.Render("us-east-1") + descStyle.Render(")") + "\n")
	b.WriteString(bulletStyle.Render("•") + descStyle.Render(" Profile name (suggested: ") + exampleStyle.Render(fmt.Sprintf("prod-%s", m.selectedAccountID[:4])) + descStyle.Render(")") + "\n")
	b.WriteString(bulletStyle.Render("•") + descStyle.Render(" IAM role (default: ") + exampleStyle.Render("AdministratorAccess") + descStyle.Render(")") + "\n\n")
	
	b.WriteString(m.renderKeyHelp("Enter", "launch AWS profile setup", "Q", "to quit"))
	
	return b.String()
}

func (m DNSSetupModel) viewInputAccountID() string {
	var b strings.Builder
	
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86"))
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))
	tipStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214"))
	bulletStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("99"))
	inputStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFF")).
		Background(lipgloss.Color("235")).
		Padding(0, 1)
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("87"))
	exampleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("123")).
		Italic(true)
	
	b.WriteString(headerStyle.Render("🔐 Enter Production AWS Account ID:") + "\n\n")
	b.WriteString(descStyle.Render("This is your 12-digit AWS account ID where the root") + "\n")
	b.WriteString(descStyle.Render("DNS zone will be created.") + "\n\n")
	
	// Create input field with cursor
	inputLine := m.accountIDInput
	if m.accountIDCursorPos < len(inputLine) {
		inputLine = inputLine[:m.accountIDCursorPos] + "█" + inputLine[m.accountIDCursorPos:]
	} else {
		inputLine = inputLine + "█"
	}
	
	b.WriteString(labelStyle.Render("Account ID: ") + inputStyle.Render(" "+inputLine+" ") + "\n\n")
	
	b.WriteString(tipStyle.Render("💡 How to find your AWS Account ID:") + "\n")
	b.WriteString(bulletStyle.Render("•") + descStyle.Render(" AWS Console: Top-right corner dropdown") + "\n")
	b.WriteString(bulletStyle.Render("•") + descStyle.Render(" CLI: ") + exampleStyle.Render("aws sts get-caller-identity") + "\n")
	b.WriteString(bulletStyle.Render("•") + descStyle.Render(" Format: ") + exampleStyle.Render("123456789012") + descStyle.Render(" (12 digits)") + "\n\n")
	
	b.WriteString(m.renderKeyHelp("Enter", "to continue", "Esc", "to go back", "Q", "to quit"))
	
	return b.String()
}

func (m DNSSetupModel) viewProcessingSetup() string {
	var b strings.Builder
	
	accountStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("214"))
	doneStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("82"))
	currentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("226")).
		Bold(true)
	pendingStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))
	
	// Clean, simple header without duplication
	if m.rootDomain != "" {
		b.WriteString("Domain: " + accountStyle.Render(m.rootDomain) + "\n")
	}
	b.WriteString("\n")
	
	// Show progress of each step
	for i, step := range m.setupSteps {
		var icon string
		var style lipgloss.Style

		// Check if status array is properly initialized
		if i < len(m.setupStepStatus) && m.setupStepStatus[i] {
			// Step completed
			icon = "✓"
			style = doneStyle
		} else if i == m.currentStepIndex {
			// Current step
			icon = m.spinner.View()
			style = currentStyle
		} else {
			// Pending step
			icon = "○"
			style = pendingStyle
		}
		
		b.WriteString(fmt.Sprintf("%s %s\n", icon, style.Render(step)))
	}
	
	
	b.WriteString("\n")
	
	return b.String()
}

func (m DNSSetupModel) handleEnter() (DNSSetupModel, tea.Cmd) {
	switch m.state {
	case StateCheckExisting:
		// Do nothing - this is an automatic check
		return m, nil
	case StateInputDomain:
		if m.domainInput == "" {
			m.errorMsg = "Domain name cannot be empty"
			return m, nil
		}
		// Basic domain validation
		if !isValidDomain(m.domainInput) {
			m.errorMsg = "Invalid domain format"
			return m, nil
		}
		m.rootDomain = m.domainInput
		m.state = StateSelectRootAccount
		return m, m.loadProductionEnvironment()
	case StateSelectRootAccount:
		// Production environment is required for root zone
		if len(m.environments) > 0 {
			selected := m.environments[0] // Should only be prod at this point
			
			// Double-check this is production
			if selected.Name != "prod" {
				m.state = StateError
				m.errorMsg = "Root DNS zone must be created in production environment.\nPlease configure a production environment first."
				return m, nil
			}
			
			m.selectedProfile = selected.Profile
			m.selectedAccountID = selected.AccountID

			// Validate the profile exists and works
			if m.selectedProfile == "" {
				m.state = StateError
				m.errorMsg = "No AWS profile configured for production environment.\nPlease configure AWS access for production account."
				return m, nil
			}

			// Try to get account ID to verify profile works
			accountID, err := getAWSAccountID(m.selectedProfile)
			if err != nil {
				m.state = StateError
				m.errorMsg = fmt.Sprintf("AWS profile '%s' for production is not working: %v\nPlease configure AWS access for production account", m.selectedProfile, err)
				return m, nil
			}

			// Update account ID with the actual one
			m.selectedAccountID = accountID
		} else {
			m.state = StateError
			m.errorMsg = "Production environment not found.\nPlease create prod.yaml first."
			return m, nil
		}
		m.state = StateCreateRootZone
		return m, m.createRootZone()
	case StateDisplayNameservers:
		m.state = StateWaitPropagation
		m.propagationCheckCount = 0
		return m, m.checkPropagation()
	case StateWaitPropagation:
		m.state = StateCheckIAMPermissions
		return m, tea.Batch(m.spinner.Tick, animationTickCmd(), m.checkIAMPermissions())
	case StateCheckIAMPermissions:
		if m.fixPermissions && len(m.missingPermissions) > 0 {
			m.fixPermissions = false
			return m, m.fixIAMPermissions()
		}
		// If viewing existing config, show DNS details. Otherwise save config and go through setup flow
		if m.isViewingExisting {
			m.state = StateDNSDetails
			return m, nil
		}
		m.state = StateComplete
		return m, m.saveConfiguration()
	case StateDNSDetails:
		// From DNS details, quit
		return m, tea.Quit
	case StateComplete, StateError:
		return m, tea.Quit
	case StateSetupProduction:
		// Move to input account ID
		m.state = StateInputAccountID
		m.accountIDInput = ""
		m.accountIDCursorPos = 0
		return m, nil
	case StateInputAccountID:
		// Validate account ID
		accountID := strings.TrimSpace(m.accountIDInput)
		if len(accountID) != 12 {
			m.errorMsg = "AWS Account ID must be exactly 12 digits"
			return m, nil
		}
		// Check if it's all digits
		for _, char := range accountID {
			if char < '0' || char > '9' {
				m.errorMsg = "AWS Account ID must contain only digits"
				return m, nil
			}
		}
		m.selectedAccountID = accountID
		
		// Initialize setup steps
		m.setupSteps = []string{
			"Checking AWS profile",
			"Creating prod.yaml",
			"Creating DNS zone",
			"Setting up IAM role",
			"Saving configuration",
		}
		m.setupStepStatus = make([]bool, len(m.setupSteps))
		m.currentStepIndex = 0
		m.state = StateProcessingSetup
		
		// Start the setup process
		return m, tea.Batch(
			m.spinner.Tick,
			animationTickCmd(),
			m.findProductionProfile(),
		)
	case StateSetupAWSProfile:
		// Exit DNS setup to create AWS profile
		// The main loop will handle profile creation and restart
		return m, tea.Quit
	case StateProcessingSetup:
		// Don't allow any input during processing
		return m, nil
	}
	return m, nil
}

func (m DNSSetupModel) handleUp() (DNSSetupModel, tea.Cmd) {
	switch m.state {
	case StateCheckIAMPermissions:
		// No scrolling needed in IAM check view
	case StateDisplayNameservers:
		// If debug log is shown, scroll it up
		if m.showDNSDebugLog {
			if m.dnsDebugScrollOffset > 0 {
				m.dnsDebugScrollOffset--
			}
		}
	}
	return m, nil
}

func (m DNSSetupModel) handleDown() (DNSSetupModel, tea.Cmd) {
	switch m.state {
	case StateCheckIAMPermissions:
		// No scrolling needed in IAM check view
	case StateDisplayNameservers:
		// If debug log is shown, scroll it down
		if m.showDNSDebugLog {
			// Calculate available height for fullscreen
			availableHeight := m.height - 5
			if availableHeight < 10 {
				availableHeight = 10
			}
			
			// Calculate maximum scroll offset
			maxOffset := len(m.dnsDebugLogs) - availableHeight
			if maxOffset < 0 {
				maxOffset = 0
			}
			
			// Scroll down if not at bottom
			if m.dnsDebugScrollOffset < maxOffset {
				m.dnsDebugScrollOffset++
			}
		}
	}
	return m, nil
}

func (m DNSSetupModel) handleSpace() (DNSSetupModel, tea.Cmd) {
	// Space key no longer used for manual subdomain selection
	// IAM permissions are checked automatically
	return m, nil
}

func (m DNSSetupModel) handleBack() (DNSSetupModel, tea.Cmd) {
	switch m.state {
	case StateInputDomain:
		m.state = StateCheckExisting
	case StateSelectRootAccount:
		m.state = StateInputDomain
	case StateCreateRootZone:
		m.state = StateSelectRootAccount
	case StateDisplayNameservers:
		m.state = StateCreateRootZone
	case StateWaitPropagation:
		m.state = StateDisplayNameservers
	case StateCheckIAMPermissions:
		m.state = StateDisplayNameservers
	}
	return m, nil
}

func (m DNSSetupModel) handleSkip() (DNSSetupModel, tea.Cmd) {
	switch m.state {
	case StateDisplayNameservers:
		// Skip nameserver update and go to propagation check
		m.state = StateWaitPropagation
		m.propagationCheckCount = 0
		return m, m.checkPropagation()
	case StateCheckIAMPermissions:
		// Skip IAM permission fixes and complete
		m.state = StateComplete
		return m, m.saveConfiguration()
	}
	return m, nil
}

func (m DNSSetupModel) handleRefresh() (DNSSetupModel, tea.Cmd) {
	switch m.state {
	case StateWaitPropagation:
		// Refresh propagation check
		return m, m.checkPropagation()
	case StateDisplayNameservers:
		// Manually check DNS propagation
		if !m.isCheckingDNS {
			m.propagationCheckTimer = 10 // Reset timer to 10 seconds
			m.isCheckingDNS = true
			return m, m.checkDNSPropagatedSimple()
		}
		return m, nil
	case StateError:
		// Retry from the beginning
		m.state = StateCheckExisting
		m.errorMsg = ""
		return m, m.checkExistingConfig()
	}
	return m, nil
}

func (m DNSSetupModel) handleCopy() (DNSSetupModel, tea.Cmd) {
	if m.state == StateDisplayNameservers && len(m.nameservers) > 0 {
		// Copy nameservers to clipboard
		nsText := strings.Join(m.nameservers, "\n")
		copyCmd := m.copyToClipboard(nsText)
		m.copiedToClipboard = true
		m.copyFeedbackTimer = 30 // Show feedback for 3 seconds (30 ticks at 100ms)
		m.copiedNSIndex = 0      // Reset individual copy indicator
		// Return both the copy command and start the animation ticker
		return m, tea.Batch(copyCmd, animationTickCmd())
	}
	return m, nil
}

func (m DNSSetupModel) handleCopyIndividualNS(key string) (DNSSetupModel, tea.Cmd) {
	if m.state == StateDisplayNameservers && len(m.nameservers) > 0 {
		// Parse the key to get the index (1-4)
		index := 0
		switch key {
		case "1":
			index = 1
		case "2":
			index = 2
		case "3":
			index = 3
		case "4":
			index = 4
		}

		// Check if the index is valid for our nameservers
		if index > 0 && index <= len(m.nameservers) {
			// Copy the specific nameserver
			nsText := m.nameservers[index-1]
			copyCmd := m.copyToClipboard(nsText)
			m.copiedNSIndex = index
			m.copiedNSTimer = 30        // Show highlight for 3 seconds (30 ticks at 100ms)
			m.copiedToClipboard = false // Reset the "all copied" indicator
			m.copyFeedbackTimer = 0
			// Return both the copy command and start the animation ticker
			return m, tea.Batch(copyCmd, animationTickCmd())
		}
	}
	return m, nil
}

func (m DNSSetupModel) handleDNSOperation(msg dnsOperationMsg) (DNSSetupModel, tea.Cmd) {
	if !msg.Success {
		m.state = StateError
		if msg.Error != nil {
			m.errorMsg = fmt.Sprintf("DNS Setup Error: %v", msg.Error)
		} else if msg.Type != "" {
			m.errorMsg = fmt.Sprintf("DNS operation failed: %s", msg.Type)
		} else {
			m.errorMsg = "An unknown error occurred during DNS setup"
		}
		return m, nil
	}

	switch msg.Type {
	case "check_existing":
		if msg.Data != nil {
			config, ok := msg.Data.(*DNSConfig)
			if ok && config != nil {
				m.dnsConfig = config
				// Populate model from existing config
				m.rootDomain = config.RootDomain
				m.selectedAccountID = config.RootAccount.AccountID
				m.zoneID = config.RootAccount.ZoneID
				m.delegationRoleArn = config.RootAccount.DelegationRoleArn

				// Don't do blocking operations here! Do them in a command
				return m, m.validateConfigAsync()
			}
		}
		// No existing config, proceed with setup
		m.state = StateInputDomain
		return m, nil

	case "validate_config_async":
		if msg.Success {
			data := msg.Data.(map[string]interface{})
			m.selectedProfile = data["profile"].(string)
			// Mark that we're viewing existing config
			m.isViewingExisting = true
			// Now validate the zone
			m.state = StateValidateConfig
			return m, m.validateExistingZone()
		} else {
			m.state = StateError
			m.errorMsg = msg.Error.Error()
			return m, nil
		}

	case "validate_zone":
		if msg.Success {
			// Zone validation successful, proceed to subdomain management
			data := msg.Data.(map[string]interface{})
			nameservers := data["nameservers"].([]string)
			// Sort nameservers to ensure consistent ordering
			sort.Strings(nameservers)
			m.nameservers = nameservers
			actualDomain := data["domain"].(string)
			isPropagated := data["propagated"].(bool)

			// Verify domain matches what's in config
			if actualDomain != m.rootDomain {
				m.state = StateError
				m.errorMsg = fmt.Sprintf("Zone domain mismatch!\nConfig says: %s\nAWS Zone has: %s\n\nPlease delete dns.yaml and reconfigure.",
					m.rootDomain, actualDomain)
				return m, nil
			}

			// Check if DNS has been properly propagated
			if !isPropagated {
				// DNS not propagated, show nameserver setup screen
				m.state = StateDisplayNameservers
				m.dnsPropagated = false
				m.propagationCheckTimer = 0 // Start with 0 for immediate check
				m.isCheckingDNS = true // Mark as checking immediately
				m.dnsTimerRunning = false // Timer will be started after first check
				// Start animation ticker and immediate check
				// The DNS timer will be started after the first check completes
				return m, tea.Batch(animationTickCmd(), m.checkDNSPropagatedSimple())
			}

			m.state = StateCheckIAMPermissions
			return m, tea.Batch(m.spinner.Tick, animationTickCmd(), m.checkIAMPermissions())
		} else {
			// Zone doesn't exist or is invalid
			m.state = StateError
			m.errorMsg = fmt.Sprintf("DNS zone validation failed: %v\n\nThe zone ID in dns.yaml may be invalid or deleted.\nPlease delete dns.yaml and reconfigure.", msg.Error)
			return m, nil
		}

	case "create_zone":
		data := msg.Data.(map[string]interface{})
		m.zoneID = data["zoneID"].(string)
		nameservers := data["nameservers"].([]string)
		// Sort nameservers to ensure consistent ordering
		sort.Strings(nameservers)
		m.nameservers = nameservers

		// Update progress - DNS zone created (only if array is initialized)
		if len(m.setupStepStatus) > 2 {
			m.setupStepStatus[2] = true
		}
		if len(m.setupStepStatus) > 0 {
			m.currentStepIndex = 3
		}

		return m, m.createDelegationRole()

	case "create_role":
		m.delegationRoleArn = msg.Data.(string)

		// Update progress - IAM role created (only if array is initialized)
		if len(m.setupStepStatus) > 3 {
			m.setupStepStatus[3] = true
		}
		if len(m.setupStepStatus) > 0 {
			m.currentStepIndex = 4
		}

		return m, m.saveConfiguration()

	case "save_config":
		// Update progress - all steps complete (only if array is initialized)
		if len(m.setupStepStatus) > 4 {
			m.setupStepStatus[4] = true
		}

		// Move to nameserver display
		m.state = StateDisplayNameservers
		m.dnsPropagated = false
		m.propagationCheckTimer = 0 // Start with 0 for immediate check
		m.isCheckingDNS = true // Mark as checking immediately
		m.dnsTimerRunning = false // Timer will be started after first check

		// Start animation ticker and immediate check
		// The DNS timer will be started after the first check completes
		return m, tea.Batch(animationTickCmd(), m.checkDNSPropagatedSimple())

	case "load_environments":
		if !msg.Success {
			m.state = StateError
			m.errorMsg = msg.Error.Error()
			return m, nil
		}
		data := msg.Data.(map[string]interface{})
		m.environments = data["environments"].([]DNSEnvironment)
		// Removed subdomain selection - now handled automatically in IAM check
		return m, nil
	
	case "need_production_setup":
		// Production environment doesn't exist, show setup screen
		m.state = StateSetupProduction
		return m, nil
	
	case "profile_found":
		// Profile was found for the account ID
		data := msg.Data.(map[string]interface{})
		m.selectedProfile = data["profile"].(string)

		// Update progress - profile check complete (only if array is initialized)
		if len(m.setupStepStatus) > 0 {
			m.setupStepStatus[0] = true
			m.currentStepIndex = 1
		}

		// Create prod.yaml with the provided account ID and profile
		if err := ensureProductionEnvironmentWithAccountID(m.rootDomain, m.selectedAccountID, m.selectedProfile); err != nil {
			m.state = StateError
			m.errorMsg = fmt.Sprintf("Failed to create prod.yaml: %v", err)
			return m, nil
		}

		// Update progress - prod.yaml created (only if array is initialized)
		if len(m.setupStepStatus) > 1 {
			m.setupStepStatus[1] = true
			m.currentStepIndex = 2
		}
		
		// Set up environment info
		m.environments = []DNSEnvironment{{
			Name:      "prod",
			Profile:   m.selectedProfile,
			AccountID: m.selectedAccountID,
		}}
		
		// Continue with zone creation
		return m, m.createRootZone()
	
	case "profile_not_found":
		// No AWS profile found - we need to create one
		// Save state and exit cleanly to create profile
		saveTemporaryDNSState(m.rootDomain, m.selectedAccountID)
		m.state = StateSetupAWSProfile
		return m, tea.Quit
	
	case "profile_created":
		// Profile was successfully created, continue with DNS setup
		data := msg.Data.(map[string]interface{})
		m.selectedProfile = data["profile"].(string)

		// Update progress - profile check complete (only if array is initialized)
		if len(m.setupStepStatus) > 0 {
			m.setupStepStatus[0] = true
			m.currentStepIndex = 1
		}

		// Create prod.yaml with the new profile
		if err := ensureProductionEnvironmentWithAccountID(m.rootDomain, m.selectedAccountID, m.selectedProfile); err != nil {
			m.state = StateError
			m.errorMsg = fmt.Sprintf("Failed to create prod.yaml: %v", err)
			return m, nil
		}

		// Update progress - prod.yaml created (only if array is initialized)
		if len(m.setupStepStatus) > 1 {
			m.setupStepStatus[1] = true
			m.currentStepIndex = 2
		}
		
		// Set up environments for DNS creation
		m.environments = []DNSEnvironment{{
			Name:      "prod",
			Profile:   m.selectedProfile,
			AccountID: m.selectedAccountID,
		}}
		
		// Continue with DNS zone creation
		return m, m.createRootZone()

	case "update_delegation_role":
		// IAM role updated with trusted accounts
		m.state = StateComplete
		return m, nil
	
	case "iam_check_init":
		// IAM check initialized - start checking environments
		if msg.Data != nil {
			data := msg.Data.(map[string]interface{})
			m.environments = data["environments"].([]DNSEnvironment)
			if roleArn, ok := data["delegationRoleArn"].(string); ok {
				m.delegationRoleArn = roleArn
			}

			// Initialize permissions map
			if m.envPermissions == nil {
				m.envPermissions = make(map[string]bool)
			}

			// Start checking first environment
			if len(m.environments) > 0 {
				// Set first non-prod environment as checking
				for i, env := range m.environments {
					if env.Name != "prod" {
						m.checkingEnvironment = env.Name
						return m, tea.Batch(
							m.spinner.Tick,
							m.checkSingleEnvironmentPermission(env, m.delegationRoleArn, i, len(m.environments)),
						)
					}
				}
			}
		}
		return m, m.spinner.Tick

	case "iam_check_progress":
		// One environment check completed
		if msg.Data != nil {
			data := msg.Data.(map[string]interface{})
			envName := data["envName"].(string)
			envIndex := data["envIndex"].(int)
			totalEnvs := data["totalEnvs"].(int)
			skip := data["skip"].(bool)

			if !skip {
				// Update permission status for this environment
				hasAccess := data["hasAccess"].(bool)
				m.envPermissions[envName] = hasAccess

				// Track missing permissions
				if !hasAccess {
					found := false
					for _, existing := range m.missingPermissions {
						if existing == envName {
							found = true
							break
						}
					}
					if !found {
						m.missingPermissions = append(m.missingPermissions, envName)
					}
				}
			}

			// Check if we're done
			isComplete := data["isComplete"].(bool)
			if isComplete {
				// All checks complete
				m.checkingEnvironment = ""
				return m, m.spinner.Tick
			}

			// Find and check next environment
			nextIndex := envIndex + 1
			if nextIndex < totalEnvs {
				nextEnv := m.environments[nextIndex]
				// Skip to next non-prod environment
				for nextIndex < totalEnvs && nextEnv.Name == "prod" {
					nextIndex++
					if nextIndex < totalEnvs {
						nextEnv = m.environments[nextIndex]
					}
				}

				if nextIndex < totalEnvs {
					m.checkingEnvironment = nextEnv.Name
					return m, tea.Batch(
						m.spinner.Tick,
						m.checkSingleEnvironmentPermission(nextEnv, m.delegationRoleArn, nextIndex, totalEnvs),
					)
				}
			}

			// No more environments to check
			m.checkingEnvironment = ""
		}
		return m, m.spinner.Tick
	
	case "iam_fix_complete":
		// IAM permissions fixed, re-check them
		if msg.Data != nil {
			data := msg.Data.(map[string]interface{})
			if roleArn, ok := data["roleArn"].(string); ok {
				m.delegationRoleArn = roleArn
			}
		}
		// Clear the fixing status
		m.currentStep = ""
		// Re-check permissions after fix
		return m, tea.Batch(m.spinner.Tick, animationTickCmd(), m.checkIAMPermissions())
	
	case "iam_fix_failed":
		// IAM fix failed, show error but stay on same screen
		// Clear the fixing status
		m.currentStep = ""
		if msg.Error != nil {
			m.errorMsg = msg.Error.Error()
		}
		// User can skip or try again
		return m, nil
	
	case "profile_creation_failed":
		// Profile creation failed, show the error
		m.state = StateError
		m.errorMsg = msg.Error.Error()
		return m, nil
	
	case "sso_sessions_loaded":
		// SSO sessions loaded, show selection
		// SSO session selection is now handled in separate TUI
		return m, nil
	
	case "sso_session_created":
		// SSO session creation is now handled in separate TUI
		return m, nil
	
	case "iam_roles_loaded":
		// IAM roles loaded, show selection
		// Role selection is now handled in separate TUI
		return m, nil

	case "clipboard_copy":
		// Clipboard copy result - just update the feedback
		if msg.Success {
			m.copiedToClipboard = true
			m.copyFeedbackTimer = 3
		}
		return m, nil

	case "dns_propagated":
		// DNS propagation check result
		m.isCheckingDNS = false
		if msg.Data != nil {
			data := msg.Data.(map[string]interface{})
			m.dnsPropagated = data["propagated"].(bool)
			actualNS := data["actualNS"].([]string)
			// Sort nameservers to ensure consistent ordering
			sort.Strings(actualNS)
			m.actualNameservers = actualNS
			
			// Store TTL if present
			if ttl, ok := data["ttl"].(uint32); ok {
				m.dnsCacheTTL = ttl
			}
			
			// Update debug logs if present
			if debugLogs, ok := data["debugLogs"].([]string); ok {
				// Append new debug logs
				m.dnsDebugLogs = append(m.dnsDebugLogs, debugLogs...)
				// Keep a reasonable limit to prevent memory issues (500 lines)
				if len(m.dnsDebugLogs) > 500 {
					m.dnsDebugLogs = m.dnsDebugLogs[len(m.dnsDebugLogs)-500:]
				}
				// Auto-scroll to bottom if in debug view
				if m.showDNSDebugLog {
					availableHeight := m.height - 5
					if availableHeight < 10 {
						availableHeight = 10
					}
					if len(m.dnsDebugLogs) > availableHeight {
						m.dnsDebugScrollOffset = len(m.dnsDebugLogs) - availableHeight
					}
				}
			}

			if m.dnsPropagated && m.state == StateDisplayNameservers {
				// DNS has propagated! Automatically proceed to next step
				m.dnsTimerRunning = false // Stop the timer
				m.state = StateCheckIAMPermissions
				return m, tea.Batch(m.spinner.Tick, animationTickCmd(), m.checkIAMPermissions())
			}
			
			// DNS not propagated yet, reset timer for next check
			if !m.dnsPropagated && m.state == StateDisplayNameservers {
				m.propagationCheckTimer = 10 // Reset to 10 seconds for next check
				// Start the timer ONLY if not already running
				if !m.dnsTimerRunning {
					m.dnsTimerRunning = true
					return m, dnsPropagationTickCmd()
				}
			}
		}
		return m, nil
	}

	return m, nil
}

// hasSelectedSubdomains removed - replaced by automatic IAM permission checking

// Command functions
func (m DNSSetupModel) checkExistingConfig() tea.Cmd {
	return func() tea.Msg {
		config, err := loadDNSConfig()
		if err != nil {
			return dnsOperationMsg{Type: "check_existing", Success: false, Error: err}
		}
		return dnsOperationMsg{Type: "check_existing", Success: true, Data: config}
	}
}

func (m DNSSetupModel) validateConfigAsync() tea.Cmd {
	return func() tea.Msg {
		// Do all blocking operations here in the goroutine
		if m.dnsConfig == nil || m.dnsConfig.RootAccount.AccountID == "" {
			return dnsOperationMsg{
				Type: "validate_config_async", Success: false,
				Error: fmt.Errorf("DNS configuration missing account ID. Please delete dns.yaml and reconfigure."),
			}
		}

		// This is the blocking call that was freezing the UI!
		matchingProfile, err := findAWSProfileByAccountID(m.dnsConfig.RootAccount.AccountID)
		if err != nil {
			return dnsOperationMsg{
				Type: "validate_config_async", Success: false,
				Error: fmt.Errorf("Cannot find AWS profile for account ID %s from dns.yaml\n\nPlease configure AWS access for account %s or update dns.yaml",
					m.dnsConfig.RootAccount.AccountID, m.dnsConfig.RootAccount.AccountID),
			}
		}

		data := map[string]interface{}{
			"profile": matchingProfile,
		}

		return dnsOperationMsg{Type: "validate_config_async", Success: true, Data: data}
	}
}

func (m DNSSetupModel) validateExistingZone() tea.Cmd {
	return func() tea.Msg {
		// Find the AWS profile for validation
		profile := m.selectedProfile
		if profile == "" {
			return dnsOperationMsg{
				Type: "validate_zone", Success: false,
				Error: fmt.Errorf("no AWS profile available"),
			}
		}

		// Validate the zone exists and get its details
		domain, nameservers, err := validateHostedZone(profile, m.zoneID)
		if err != nil {
			return dnsOperationMsg{Type: "validate_zone", Success: false, Error: err}
		}

		// Check if DNS has been propagated - query actual DNS servers
		propagated := false
		actualNS, err := queryNameservers(domain)
		if err == nil && len(actualNS) > 0 {
			// Check if at least one of the actual nameservers matches our AWS nameservers
			for _, actual := range actualNS {
				for _, expected := range nameservers {
					if strings.EqualFold(strings.TrimSuffix(actual, "."), strings.TrimSuffix(expected, ".")) {
						propagated = true
						break
					}
				}
				if propagated {
					break
				}
			}
		}

		data := map[string]interface{}{
			"domain":      domain,
			"nameservers": nameservers,
			"propagated":  propagated,
		}

		return dnsOperationMsg{Type: "validate_zone", Success: true, Data: data}
	}
}

// loadProductionEnvironment loads only the production environment for root zone creation
func (m DNSSetupModel) loadProductionEnvironment() tea.Cmd {
	return func() tea.Msg {
		// Try to load production environment
		env, err := loadEnv("prod")
		if err != nil {
			// Production environment doesn't exist - we'll need to create it
			// Go directly to production setup flow
			return dnsOperationMsg{
				Type: "need_production_setup", 
				Success: true,
				Data: nil,
			}
		}
		
		// Get AWS account ID for production
		accountID := env.AccountID
		profile := env.AWSProfile
		
		// If account ID is not in YAML, try to get it from the AWS profile
		if accountID == "" && profile != "" {
			if id, err := getAWSAccountID(profile); err == nil {
				accountID = id
			}
		}
		
		// If we still don't have profile/account, check if there's a "prod" AWS profile
		if profile == "" {
			// Try common production profile names
			prodProfiles := []string{"prod", "production", "prd"}
			for _, p := range prodProfiles {
				if id, err := getAWSAccountID(p); err == nil {
					profile = p
					accountID = id
					break
				}
			}
		}
		
		// Validate we have production environment configured
		if profile == "" && accountID == "" {
			return dnsOperationMsg{
				Type: "load_environments",
				Success: false,
				Error: fmt.Errorf("production AWS profile not configured. Please set up AWS credentials for production account"),
			}
		}
		
		// Create the production environment entry
		envs := []DNSEnvironment{{
			Name:      "prod",
			Profile:   profile,
			AccountID: accountID,
		}}
		
		data := map[string]interface{}{
			"environments": envs,
			"subdomains":   []string{}, // No subdomains at this stage
		}
		
		return dnsOperationMsg{Type: "load_environments", Success: true, Data: data}
	}
}

func (m DNSSetupModel) loadEnvironments() tea.Cmd {
	return func() tea.Msg {
		// Get list of project YAML files
		envs := []DNSEnvironment{}

		// Check for common environment files in current working directory
		// For DNS setup, we handle dev, staging, and prod environments
		envFiles := []string{"dev", "staging", "prod"}
		for _, envName := range envFiles {
			// Try to load the environment file from current directory
			env, err := loadEnv(envName)
			if err != nil {
				// File doesn't exist or can't be loaded, skip it
				continue
			}

			// Get AWS account ID for this environment
			accountID := env.AccountID
			profile := env.AWSProfile

			// If account ID is not in YAML, try to get it from the AWS profile
			if accountID == "" && profile != "" {
				if id, err := getAWSAccountID(profile); err == nil {
					accountID = id
				}
			}

			// If we still don't have an account ID, try the current selected profile
			if accountID == "" && selectedAWSProfile != "" {
				if id, err := getAWSAccountID(selectedAWSProfile); err == nil {
					accountID = id
					profile = selectedAWSProfile
				}
			}

			// Only add if we have some account information
			if accountID != "" || profile != "" {
				envs = append(envs, DNSEnvironment{
					Name:      envName,
					Profile:   profile,
					AccountID: accountID,
				})
			}
		}

		// If no environments found, use the current AWS profile
		if len(envs) == 0 && selectedAWSProfile != "" {
			accountID, _ := getAWSAccountID(selectedAWSProfile)
			envs = append(envs, DNSEnvironment{
				Name:      "default",
				Profile:   selectedAWSProfile,
				AccountID: accountID,
			})
		}

		// Generate subdomain suggestions (exclude production)
		subdomains := []string{}
		for _, env := range envs {
			// Only suggest subdomains for non-production environments
			if env.Name != "prod" && env.Name != m.selectedProfile {
				subdomains = append(subdomains, fmt.Sprintf("%s.%s", env.Name, m.rootDomain))
			}
		}

		data := map[string]interface{}{
			"environments": envs,
			"subdomains":   subdomains,
		}

		return dnsOperationMsg{Type: "load_environments", Success: true, Data: data}
	}
}

func (m DNSSetupModel) createRootZone() tea.Cmd {
	return func() tea.Msg {
		// Use the selected profile or find one by account ID
		profile := m.selectedProfile

		// If we have an account ID but no profile, find the matching profile
		if profile == "" && m.selectedAccountID != "" {
			matchingProfile, err := findAWSProfileByAccountID(m.selectedAccountID)
			if err != nil {
				return dnsOperationMsg{
					Type: "create_zone", Success: false,
					Error: fmt.Errorf("cannot find AWS profile for account ID %s: %v\nPlease configure AWS access for this account", m.selectedAccountID, err),
				}
			}
			profile = matchingProfile
		}

		// No fallback - if we don't have a profile at this point, it's an error
		if profile == "" {
			return dnsOperationMsg{
				Type: "create_zone", Success: false,
				Error: fmt.Errorf("no AWS profile configured. Please select an AWS account first"),
			}
		}

		// Actually create the hosted zone in AWS Route53
		zoneID, nameservers, err := createHostedZone(profile, m.rootDomain)
		if err != nil {
			return dnsOperationMsg{Type: "create_zone", Success: false, Error: err}
		}

		data := map[string]interface{}{
			"zoneID":      zoneID,
			"nameservers": nameservers,
		}

		return dnsOperationMsg{Type: "create_zone", Success: true, Data: data}
	}
}

func (m DNSSetupModel) createDelegationRole() tea.Cmd {
	return func() tea.Msg {
		// Use the selected profile or find one by account ID
		profile := m.selectedProfile

		// If we have an account ID but no profile, find the matching profile
		if profile == "" && m.selectedAccountID != "" {
			matchingProfile, err := findAWSProfileByAccountID(m.selectedAccountID)
			if err != nil {
				// For delegation role, we can continue without it - it's optional
				return dnsOperationMsg{
					Type: "create_role", Success: true,
					Data: fmt.Sprintf("arn:aws:iam::%s:role/dns-delegation", m.selectedAccountID),
				}
			}
			profile = matchingProfile
		}

		if profile == "" {
			// No profile available, skip role creation
			return dnsOperationMsg{
				Type: "create_role", Success: true,
				Data: fmt.Sprintf("arn:aws:iam::%s:role/dns-delegation", m.selectedAccountID),
			}
		}

		// Collect trusted accounts from environments
		var trustedAccounts []string
		for _, env := range m.environments {
			if env.AccountID != "" && env.AccountID != m.selectedAccountID {
				trustedAccounts = append(trustedAccounts, env.AccountID)
			}
		}

		// Create the delegation role
		roleArn, err := createDNSDelegationRole(profile, trustedAccounts)
		if err != nil {
			// If role creation fails, just use a placeholder
			// This is optional and won't block the setup
			roleArn = fmt.Sprintf("arn:aws:iam::%s:role/dns-delegation", m.selectedAccountID)
		}

		return dnsOperationMsg{Type: "create_role", Success: true, Data: roleArn}
	}
}

func (m DNSSetupModel) checkPropagation() tea.Cmd {
	return func() tea.Msg {
		// Actually check DNS propagation using the real function
		if len(m.nameservers) > 0 {
			status, err := checkDNSPropagation(m.rootDomain, m.nameservers)
			if err != nil {
				// On error, return empty status
				status = make(map[string]bool)
			}
			return propagationStatusMsg{Status: status}
		}

		// Fallback to simulation if no nameservers
		status := make(map[string]bool)
		status["8.8.8.8"] = true
		status["1.1.1.1"] = true
		status["9.9.9.9"] = m.propagationCheckCount > 1
		status["208.67.222.222"] = m.propagationCheckCount > 2

		return propagationStatusMsg{Status: status}
	}
}

func (m DNSSetupModel) checkDNSPropagatedSimple() tea.Cmd {
	return func() tea.Msg {
		result := map[string]interface{}{
			"propagated": false,
			"actualNS":   []string{},
			"debugLogs":  []string{},
			"ttl":        uint32(0),
		}

		debugLogs := []string{}
		debugLogs = append(debugLogs, "============================================")
		debugLogs = append(debugLogs, fmt.Sprintf("DNS CHECK: %s", time.Now().Format("15:04:05")))
		debugLogs = append(debugLogs, "============================================")
		debugLogs = append(debugLogs, "")
		
		// Debug: Show expected nameservers
		debugLogs = append(debugLogs, fmt.Sprintf("Expected AWS nameservers for %s:", m.rootDomain))
		for i, ns := range m.nameservers {
			debugLogs = append(debugLogs, fmt.Sprintf("  %d. %s", i+1, ns))
		}
		debugLogs = append(debugLogs, "")

		// Simple check if DNS has propagated
		if len(m.nameservers) > 0 && m.rootDomain != "" {
			// Use the debug version that returns logs and TTL
			actualNS, queryDebugLogs, ttl, err := queryNameserversWithDebug(m.rootDomain)
			debugLogs = append(debugLogs, queryDebugLogs...)
			result["ttl"] = ttl
			if err != nil {
				debugLogs = append(debugLogs, fmt.Sprintf("Error querying nameservers: %v", err))
			} else if len(actualNS) > 0 {
				// Sort the actual nameservers for consistent display
				sort.Strings(actualNS)
				debugLogs = append(debugLogs, "")
				if ttl == 0 {
					debugLogs = append(debugLogs, "✅ Current nameservers (via DoH - real-time):")
				} else {
					debugLogs = append(debugLogs, fmt.Sprintf("⚠️ Current nameservers (CACHED - TTL: %ds):", ttl))
				}
				for i, ns := range actualNS {
					debugLogs = append(debugLogs, fmt.Sprintf("  %d. %s", i+1, ns))
				}
				debugLogs = append(debugLogs, "")
				
				result["actualNS"] = actualNS
				// Check if at least one of the actual nameservers matches our AWS nameservers
				matchCount := 0
				for _, actual := range actualNS {
					for _, expected := range m.nameservers {
						actualClean := strings.TrimSuffix(strings.ToLower(actual), ".")
						expectedClean := strings.TrimSuffix(strings.ToLower(expected), ".")
						if actualClean == expectedClean {
							matchCount++
							debugLogs = append(debugLogs, fmt.Sprintf("Match found: %s == %s", actualClean, expectedClean))
							result["propagated"] = true
							break
						}
					}
				}
				
				if !result["propagated"].(bool) {
					debugLogs = append(debugLogs, "❌ No matches found! DNS not propagated yet.")
					debugLogs = append(debugLogs, "")
					if ttl > 0 {
						debugLogs = append(debugLogs, "Note: This could be due to DNS caching.")
						debugLogs = append(debugLogs, "The nameserver update may have already occurred,")
						debugLogs = append(debugLogs, "but cached values are being returned.")
					}
				} else {
					debugLogs = append(debugLogs, fmt.Sprintf("✅ Found %d matching nameservers!", matchCount))
					debugLogs = append(debugLogs, "✅ DNS HAS PROPAGATED SUCCESSFULLY!")
				}
			} else {
				debugLogs = append(debugLogs, "No nameservers returned")
			}
		}
		
		result["debugLogs"] = debugLogs
		return dnsOperationMsg{Type: "dns_propagated", Success: true, Data: result}
	}
}

// Initialize IAM permissions check by loading environments and delegation role
func (m DNSSetupModel) checkIAMPermissions() tea.Cmd {
	return func() tea.Msg {
		// Load environments to check
		envs := []DNSEnvironment{}
		envFiles := []string{"dev", "staging", "prod"}

		for _, envName := range envFiles {
			env, err := loadEnv(envName)
			if err != nil {
				continue // Skip if not found
			}

			accountID := env.AccountID
			profile := env.AWSProfile

			// Get account ID from profile if not in YAML
			if accountID == "" && profile != "" {
				if id, err := getAWSAccountID(profile); err == nil {
					accountID = id
				}
			}

			if accountID != "" || profile != "" {
				envs = append(envs, DNSEnvironment{
					Name:      envName,
					Profile:   profile,
					AccountID: accountID,
				})
			}
		}

		// Check if delegation role exists - use stored ARN or try to get it
		delegationRoleArn := m.delegationRoleArn
		if delegationRoleArn == "" && m.selectedProfile != "" {
			// Try to get the existing delegation role
			cfg, _ := config.LoadDefaultConfig(context.Background(),
				config.WithSharedConfigProfile(m.selectedProfile),
			)
			iamClient := iam.NewFromConfig(cfg)
			getRoleResp, err := iamClient.GetRole(context.Background(), &iam.GetRoleInput{
				RoleName: aws.String("dns-delegation-role"),
			})
			if err == nil && getRoleResp.Role != nil {
				delegationRoleArn = *getRoleResp.Role.Arn
			}
		}

		// Return initialization message with environments and delegation role info
		return dnsOperationMsg{
			Type:    "iam_check_init",
			Success: true,
			Data: map[string]interface{}{
				"environments":       envs,
				"delegationRoleArn":  delegationRoleArn,
			},
		}
	}
}

// Check IAM permissions for a single environment
func (m DNSSetupModel) checkSingleEnvironmentPermission(env DNSEnvironment, delegationRoleArn string, envIndex int, totalEnvs int) tea.Cmd {
	return func() tea.Msg {
		// Skip production environment
		if env.Name == "prod" {
			return dnsOperationMsg{
				Type:    "iam_check_progress",
				Success: true,
				Data: map[string]interface{}{
					"envName":   env.Name,
					"skip":      true,
					"envIndex":  envIndex,
					"totalEnvs": totalEnvs,
				},
			}
		}

		// Add a small delay for visual effect (100ms)
		time.Sleep(100 * time.Millisecond)

		var hasAccess bool

		if delegationRoleArn != "" && env.AccountID != "" {
			// Check if this account is in the trust policy
			trustedAccounts := getTrustedAccountsFromRole(m.selectedProfile, delegationRoleArn)

			for _, trusted := range trustedAccounts {
				if trusted == env.AccountID {
					hasAccess = true
					break
				}
			}
		} else {
			// No delegation role or no account ID = no access
			hasAccess = false
		}

		return dnsOperationMsg{
			Type:    "iam_check_progress",
			Success: true,
			Data: map[string]interface{}{
				"envName":    env.Name,
				"hasAccess":  hasAccess,
				"skip":       false,
				"envIndex":   envIndex,
				"totalEnvs":  totalEnvs,
				"isComplete": envIndex >= totalEnvs-1,
			},
		}
	}
}

func (m DNSSetupModel) fixIAMPermissions() tea.Cmd {
	return func() tea.Msg {
		// Collect account IDs that need access
		var trustedAccounts []string
		accountsToFix := make(map[string]string) // env name -> account ID
		
		for _, env := range m.environments {
			if env.Name != "prod" {
				// Check if this env is in missing permissions
				for _, missing := range m.missingPermissions {
					if missing == env.Name {
						// Get account ID if we don't have it yet
						accountID := env.AccountID
						if accountID == "" && env.Profile != "" {
							// Try to get account ID from profile
							if id, err := getAWSAccountID(env.Profile); err == nil {
								accountID = id
							}
						}
						
						if accountID != "" {
							trustedAccounts = append(trustedAccounts, accountID)
							accountsToFix[env.Name] = accountID
						}
						break
					}
				}
			}
		}
		
		// Update or create the delegation role
		if len(trustedAccounts) > 0 && m.selectedProfile != "" {
			roleArn, err := createDNSDelegationRole(m.selectedProfile, trustedAccounts)
			if err != nil {
				return dnsOperationMsg{
					Type:    "iam_fix_failed",
					Success: false,
					Error:   err,
				}
			}
			// Return success message to trigger re-check
			return dnsOperationMsg{
				Type:    "iam_fix_complete",
				Success: true,
				Data: map[string]interface{}{
					"roleArn": roleArn,
					"fixed":   accountsToFix,
				},
			}
		}
		
		// No accounts to fix (no account IDs found)
		return dnsOperationMsg{
			Type:    "iam_fix_failed",
			Success: false,
			Error:   fmt.Errorf("Could not determine account IDs for environments: %v", m.missingPermissions),
		}
	}
}

func getTrustedAccountsFromRole(profile, roleArn string) []string {
	ctx := context.Background()
	
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(profile),
	)
	if err != nil {
		return []string{}
	}
	
	iamClient := iam.NewFromConfig(cfg)
	
	// Get the role to check its trust policy
	getRoleResp, err := iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String("dns-delegation-role"),
	})
	if err != nil {
		return []string{}
	}
	
	// Parse the trust policy to get trusted accounts
	// The policy document might be URL encoded
	policyDoc := *getRoleResp.Role.AssumeRolePolicyDocument
	// URL decode if needed
	if strings.Contains(policyDoc, "%7B") || strings.Contains(policyDoc, "%22") {
		if decoded, err := url.QueryUnescape(policyDoc); err == nil {
			policyDoc = decoded
		}
	}
	
	var trustPolicy map[string]interface{}
	if err := json.Unmarshal([]byte(policyDoc), &trustPolicy); err != nil {
		return []string{}
	}
	
	var trustedAccounts []string
	if statements, ok := trustPolicy["Statement"].([]interface{}); ok {
		for _, stmt := range statements {
			if stmtMap, ok := stmt.(map[string]interface{}); ok {
				if principal, ok := stmtMap["Principal"].(map[string]interface{}); ok {
					if awsArns, ok := principal["AWS"].([]interface{}); ok {
						for _, arn := range awsArns {
							if arnStr, ok := arn.(string); ok {
								// Extract account ID from ARN
								// Format: arn:aws:iam::123456789012:root
								parts := strings.Split(arnStr, ":")
								if len(parts) >= 5 {
									trustedAccounts = append(trustedAccounts, parts[4])
								}
							}
						}
					} else if awsArn, ok := principal["AWS"].(string); ok {
						// Single ARN case
						parts := strings.Split(awsArn, ":")
						if len(parts) >= 5 {
							trustedAccounts = append(trustedAccounts, parts[4])
						}
					}
				}
			}
		}
	}
	
	return trustedAccounts
}

func (m DNSSetupModel) updateDelegationRole() tea.Cmd {
	return func() tea.Msg {
		// Get the root profile
		rootProfile := m.selectedProfile
		if rootProfile == "" && m.selectedAccountID != "" {
			matchingProfile, err := findAWSProfileByAccountID(m.selectedAccountID)
			if err != nil {
				// Role update is optional, can continue without it
				return dnsOperationMsg{
					Type: "update_delegation_role", Success: true,
				}
			}
			rootProfile = matchingProfile
		}

		if rootProfile == "" {
			// No profile available, skip role update
			return dnsOperationMsg{
				Type: "update_delegation_role", Success: true,
			}
		}

		// Collect account IDs that need IAM trust relationship
		var trustedAccounts []string
		for _, env := range m.environments {
			if env.Name != "prod" && env.AccountID != "" && env.AccountID != m.selectedAccountID {
				// Add all non-production accounts to trust relationship
				trustedAccounts = append(trustedAccounts, env.AccountID)
			}
		}

		// Update the delegation role with new trusted accounts
		if len(trustedAccounts) > 0 {
			_, err := createDNSDelegationRole(rootProfile, trustedAccounts)
			if err != nil {
				// Role update is optional, won't block completion
				return dnsOperationMsg{
					Type: "update_delegation_role", Success: true,
				}
			}
		}

		return dnsOperationMsg{Type: "update_delegation_role", Success: true}
	}
}

func (m DNSSetupModel) saveConfiguration() tea.Cmd {
	return func() tea.Msg {
		// Start with the account ID and profile we have
		accountID := m.selectedAccountID
		profile := m.selectedProfile

		// Both should be set by this point
		if accountID == "" || profile == "" {
			return dnsOperationMsg{
				Type: "save_config", Success: false,
				Error: fmt.Errorf("missing account ID or profile information"),
			}
		}

		// Verify the profile still works with the account ID
		actualAccountID, err := getAWSAccountID(profile)
		if err != nil {
			return dnsOperationMsg{
				Type: "save_config", Success: false,
				Error: fmt.Errorf("AWS profile %s is not working: %v", profile, err),
			}
		}

		if actualAccountID != accountID {
			return dnsOperationMsg{
				Type: "save_config", Success: false,
				Error: fmt.Errorf("profile %s has account ID %s but expected %s", profile, actualAccountID, accountID),
			}
		}

		config := &DNSConfig{
			RootDomain: m.rootDomain,
			RootAccount: DNSRootAccount{
				AccountID:         accountID,
				ZoneID:            m.zoneID,
				DelegationRoleArn: m.delegationRoleArn,
			},
			DelegatedZones: []DelegatedZone{},
		}

		// Delegated zones are now created by Terraform, not by the DNS setup tool
		// We only record which environments exist, not create zones for them

		if err = saveDNSConfig(config); err != nil {
			return dnsOperationMsg{Type: "save_config", Success: false, Error: err}
		}

		// Ensure production environment is configured
		if err = ensureProductionEnvironment(m.rootDomain, m.zoneID, accountID); err != nil {
			// Log the error but don't fail - this is optional
			fmt.Printf("Warning: Could not update prod.yaml: %v\n", err)
		}

		// Propagate DNS info to all environments
		if err = propagateRootZoneInfo(config); err != nil {
			// Log the error but don't fail - this is optional
			fmt.Printf("Warning: Could not propagate DNS info to environments: %v\n", err)
		}

		return dnsOperationMsg{Type: "save_config", Success: true}
	}
}

func (m DNSSetupModel) handleDomainInput(msg tea.KeyMsg) (DNSSetupModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit
	case "enter":
		return m.handleEnter()
	case "backspace":
		if m.cursorPos > 0 {
			m.domainInput = m.domainInput[:m.cursorPos-1] + m.domainInput[m.cursorPos:]
			m.cursorPos--
		}
	case "left":
		if m.cursorPos > 0 {
			m.cursorPos--
		}
	case "right":
		if m.cursorPos < len(m.domainInput) {
			m.cursorPos++
		}
	case "home":
		m.cursorPos = 0
	case "end":
		m.cursorPos = len(m.domainInput)
	case "ctrl+v", "cmd+v":
		// Handle paste - for now just accept any pasted text
		// In a real implementation, you'd read from clipboard
		// But Bubble Tea doesn't have direct clipboard access
		// So we'll handle multi-character input below
		return m, nil
	default:
		// Handle regular input including pasted text
		input := msg.String()

		// Check if it's a paste operation (multi-character input)
		// or regular single character input
		if len(input) > 0 {
			// Filter the input to only allow valid domain characters
			validInput := ""
			for _, char := range input {
				if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
					(char >= '0' && char <= '9') || char == '.' || char == '-' {
					validInput += string(char)
				}
			}

			if validInput != "" {
				// Insert the valid input at cursor position
				m.domainInput = m.domainInput[:m.cursorPos] + validInput + m.domainInput[m.cursorPos:]
				m.cursorPos += len(validInput)
			}
		}
	}
	return m, nil
}

func (m DNSSetupModel) handleAccountIDInput(msg tea.KeyMsg) (DNSSetupModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		// Go back to setup production screen
		m.state = StateSetupProduction
		return m, nil
	case "enter":
		return m.handleEnter()
	case "backspace":
		if m.accountIDCursorPos > 0 {
			m.accountIDInput = m.accountIDInput[:m.accountIDCursorPos-1] + m.accountIDInput[m.accountIDCursorPos:]
			m.accountIDCursorPos--
		}
	case "left":
		if m.accountIDCursorPos > 0 {
			m.accountIDCursorPos--
		}
	case "right":
		if m.accountIDCursorPos < len(m.accountIDInput) {
			m.accountIDCursorPos++
		}
	case "home":
		m.accountIDCursorPos = 0
	case "end":
		m.accountIDCursorPos = len(m.accountIDInput)
	default:
		// Handle regular input including pasted text
		input := msg.String()
		
		// Filter input to only allow digits
		validInput := ""
		for _, char := range input {
			if char >= '0' && char <= '9' {
				validInput += string(char)
			}
		}
		
		// Insert valid input at cursor position
		if validInput != "" {
			// Calculate how much we can add without exceeding 12 characters
			currentLen := len(m.accountIDInput)
			spaceLeft := 12 - currentLen
			
			if spaceLeft > 0 {
				// Truncate if pasted text would exceed 12 characters
				if len(validInput) > spaceLeft {
					validInput = validInput[:spaceLeft]
				}
				
				// Insert at cursor position
				m.accountIDInput = m.accountIDInput[:m.accountIDCursorPos] + validInput + m.accountIDInput[m.accountIDCursorPos:]
				m.accountIDCursorPos += len(validInput)
			}
		}
	}
	return m, nil
}

func (m DNSSetupModel) findProductionProfile() tea.Cmd {
	return func() tea.Msg {
		// Try to find an AWS profile that matches the account ID
		profile, err := findAWSProfileByAccountID(m.selectedAccountID)
		if err != nil {
			// No matching profile found, need to create one
			return dnsOperationMsg{
				Type: "profile_not_found",
				Success: true,  // Set to true so it reaches the handler
				Data: map[string]interface{}{
					"accountID": m.selectedAccountID,
				},
			}
		}
		
		data := map[string]interface{}{
			"profile": profile,
		}
		return dnsOperationMsg{Type: "profile_found", Success: true, Data: data}
	}
}

// AWS Profile Creation Views







// Commands for AWS profile creation




func isValidDomain(domain string) bool {
	// Basic domain validation
	if len(domain) < 3 || len(domain) > 253 {
		return false
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}
	if strings.Contains(domain, "..") {
		return false
	}
	// Check for at least one dot
	if !strings.Contains(domain, ".") {
		return false
	}
	// Check each label
	labels := strings.Split(domain, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
	}
	return true
}

// Add clipboard copy function
func (m DNSSetupModel) copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		// Use pbcopy on macOS, xclip on Linux
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("pbcopy")
		case "linux":
			cmd = exec.Command("xclip", "-selection", "clipboard")
		default:
			return nil
		}

		cmd.Stdin = strings.NewReader(text)
		err := cmd.Run()
		if err != nil {
			return dnsOperationMsg{Type: "clipboard_copy", Success: false, Error: err}
		}
		return dnsOperationMsg{Type: "clipboard_copy", Success: true}
	}
}

// ensureProductionEnvironmentWithAccountID creates prod.yaml with provided account ID
func ensureProductionEnvironmentWithAccountID(rootDomain, accountID, profile string) error {
	prodPath := "prod.yaml"
	
	// Get project name from existing files or use a default
	projectName := getProjectNameForDNS()
	
	// Try to get the region from the AWS profile
	region := "us-east-1" // Default region
	if profile != "" {
		// Try to get the region from AWS profile config
		if profileRegion, err := getAWSRegion(profile); err == nil && profileRegion != "" {
			region = profileRegion
		}
	}
	
	// Create new production environment
	env := createEnv(projectName, "prod")
	env.IsProd = true
	env.AccountID = accountID
	env.AWSProfile = profile
	env.Region = region
	
	// Set up domain configuration (zone will be created later)
	env.Domain.Enabled = true
	env.Domain.DomainName = rootDomain
	env.Domain.CreateDomainZone = true  // Will be created
	env.Domain.AddEnvDomainPrefix = false // No prefix for production
	
	// Save the configuration
	return saveEnvToFile(env, prodPath)
}

// getProjectNameForDNS gets the project name from existing environment files
func getProjectNameForDNS() string {
	// Try to load any existing environment file to get project name
	envFiles := []string{"dev", "staging", "prod"}
	for _, envName := range envFiles {
		env, err := loadEnv(envName)
		if err == nil && env.Project != "" {
			return env.Project
		}
	}
	// If no files exist, use a generic name
	return "myproject"
}

// ensureProductionEnvironment creates or updates prod.yaml with DNS configuration
func ensureProductionEnvironment(rootDomain, zoneID, accountID string) error {
	prodPath := "prod.yaml"
	
	// Try to load existing prod.yaml
	var env Env
	if data, err := os.ReadFile(prodPath); err == nil {
		// File exists, unmarshal it
		if err := yaml.Unmarshal(data, &env); err != nil {
			return fmt.Errorf("error parsing existing prod.yaml: %v", err)
		}
	} else {
		// File doesn't exist, create new environment
		env = createEnv("project", "prod")
		env.IsProd = true
	}
	
	// Update domain configuration
	env.Domain.Enabled = true
	env.Domain.DomainName = rootDomain
	env.Domain.ZoneID = zoneID
	env.Domain.CreateDomainZone = false  // We already have the zone
	env.Domain.AddEnvDomainPrefix = false // No prefix for production
	
	// Save the updated configuration
	return saveEnvToFile(env, prodPath)
}

// propagateRootZoneInfo updates all environment files with root zone information
func propagateRootZoneInfo(dnsConfig *DNSConfig) error {
	// Only update dev environment, prod already has the root zone
	envFiles := []string{"dev"}
	
	for _, envName := range envFiles {
		path := fmt.Sprintf("%s.yaml", envName)
		
		// Try to load existing environment
		var env Env
		if data, err := os.ReadFile(path); err == nil {
			// File exists, unmarshal it
			if err := yaml.Unmarshal(data, &env); err != nil {
				// Skip this file if we can't parse it
				continue
			}
		} else {
			// File doesn't exist, create new environment
			projectName := getProjectNameForDNS()
			env = createEnv(projectName, envName)
		}
		
		// CRITICAL: Keep domain_name as root domain!
		env.Domain.Enabled = true
		env.Domain.DomainName = dnsConfig.RootDomain
		env.Domain.CreateDomainZone = true  // Will create subdomain zone
		env.Domain.AddEnvDomainPrefix = true // This makes it dev.example.com
		
		// Add DNS info for delegation
		env.Domain.RootZoneID = dnsConfig.RootAccount.ZoneID
		env.Domain.RootAccountID = dnsConfig.RootAccount.AccountID
		
		// Save the updated configuration
		if err := saveEnvToFile(env, path); err != nil {
			// Log error but continue with other files
			fmt.Printf("Warning: Could not update %s: %v\n", path, err)
		}
	}
	
	return nil
}

func runDNSSetupWizard() {
	for {
		p := tea.NewProgram(NewDNSSetupModel(), tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			fmt.Printf("Error running DNS setup wizard: %v\n", err)
			return
		}
		
		// Check if we need to create an AWS profile
		if model, ok := finalModel.(DNSSetupModel); ok {
			if model.state == StateSetupAWSProfile {
				// Get project name for suggested profile name
				projectName := getProjectNameForDNS()
				suggestedName := fmt.Sprintf("%s-prod", projectName)
				
				// Run AWS profile creation in Bubble Tea
				profileModel := NewAWSProfileCreationModel(model.selectedAccountID, suggestedName)
				profileProgram := tea.NewProgram(profileModel, tea.WithAltScreen())
				profileFinalModel, err := profileProgram.Run()
				if err != nil {
					fmt.Printf("\nFailed to run AWS profile creation: %v\n", err)
					return
				}
				
				// Check if profile was created successfully
				if profileResult, ok := profileFinalModel.(AWSProfileCreationModel); ok {
					if profileResult.createdProfile != "" {
						// Create prod.yaml with the new profile
						if err := ensureProductionEnvironmentWithAccountID(model.rootDomain, model.selectedAccountID, profileResult.createdProfile); err != nil {
							fmt.Printf("Failed to create prod.yaml: %v\n", err)
							return
						}
						
						// Continue DNS setup - it will now find the profile and prod.yaml
						continue
					}
				}
				// User cancelled or error occurred
				return
			}
		}
		// Normal exit
		return
	}
}
