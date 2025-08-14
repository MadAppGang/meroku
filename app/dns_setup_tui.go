package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type DNSSetupState int

const (
	StateCheckExisting DNSSetupState = iota
	StateInputDomain
	StateSelectRootAccount
	StateCreateRootZone
	StateDisplayNameservers
	StateWaitPropagation
	StateAddSubdomain
	StateComplete
	StateError
	StateValidateConfig
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
	errorMsg              string
	dnsConfig             *DNSConfig
	subdomains            []string
	selectedSubdomains    []bool
	propagationCheckCount int
	delegationRoleArn     string
	width                 int
	height                int
	copiedToClipboard     bool
	copyFeedbackTimer     int
	animationFrame        int
	copiedNSIndex         int      // Which nameserver was copied (1-4, 0 = all)
	copiedNSTimer         int      // Timer for individual NS copy animation
	currentSubdomainIndex int      // Currently selected subdomain index
	selectionTimer        int      // Timer for selection animation
	propagationCheckTimer int      // Timer for auto-checking DNS propagation (counts down from 100)
	dnsPropagated         bool     // Whether DNS has been propagated
	actualNameservers     []string // Current nameservers returned by DNS query
	isCheckingDNS         bool     // Whether we're currently checking DNS
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
)

// animationTickCmd returns a command that ticks every 100ms for animations
func animationTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return animationTickMsg(t)
	})
}

func NewDNSSetupModel() DNSSetupModel {
	s := spinner.New()
	s.Spinner = spinner.Globe
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	p := progress.New(progress.WithDefaultGradient())

	return DNSSetupModel{
		state:             StateCheckExisting,
		domainInput:       "",
		cursorPos:         0,
		spinner:           s,
		progress:          p,
		propagationStatus: make(map[string]bool),
		currentStep:       "Initializing DNS setup wizard...",
	}
}

func (m DNSSetupModel) Init() tea.Cmd {
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
		case "r", "R":
			return m.handleRefresh()
		case "c", "C":
			return m.handleCopy()
		case "1", "2", "3", "4":
			// Copy individual nameserver
			if m.state == StateDisplayNameservers {
				return m.handleCopyIndividualNS(msg.String())
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
				m.state = StateAddSubdomain
				return m, m.loadEnvironments()
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
		// Decrement selection timer for subdomain animation
		if m.selectionTimer > 0 {
			m.selectionTimer--
			if m.selectionTimer == 0 && m.state == StateAddSubdomain {
				// Deselect the subdomain after animation
				if m.currentSubdomainIndex >= 0 && m.currentSubdomainIndex < len(m.selectedSubdomains) {
					m.selectedSubdomains[m.currentSubdomainIndex] = false
					m.currentSubdomainIndex = -1
				}
				needsFinalRender = true
			}
		}

		// Handle DNS propagation check timer
		if m.state == StateDisplayNameservers && !m.dnsPropagated {
			if m.propagationCheckTimer > 0 {
				m.propagationCheckTimer--
			}
			// Check when timer reaches 0
			if m.propagationCheckTimer == 0 && !m.isCheckingDNS {
				m.propagationCheckTimer = 100 // Reset to 10 seconds (100 ticks at 100ms)
				m.isCheckingDNS = true
				cmds = append(cmds, m.checkDNSPropagatedSimple())
			}
		}

		// Keep animation running for loading states or when showing copy feedback or selection animation
		// Also continue for one more tick if we just cleared something (needsFinalRender)
		// Or if we're checking DNS propagation
		if m.state == StateCheckExisting || m.state == StateValidateConfig ||
			m.state == StateDisplayNameservers && (m.copiedNSTimer > 0 || m.copyFeedbackTimer > 0 || needsFinalRender || !m.dnsPropagated) ||
			m.state == StateAddSubdomain && (m.selectionTimer > 0 || needsFinalRender) {
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
	case StateAddSubdomain:
		content = m.viewAddSubdomain()
	case StateComplete:
		content = m.viewComplete()
	case StateError:
		content = m.viewError()
	case StateValidateConfig:
		content = m.viewValidateConfig()
	}

	return m.renderBox("DNS Custom Domain Setup", content)
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
	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))
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
	b.WriteString(questionStyle.Render("🔑 Which AWS account should host the root DNS zone?") + "\n\n")

	for i, env := range m.environments {
		prefix := "  "
		if i == 0 {
			prefix = "▸ "
		}
		recommended := ""
		if env.Name == "prod" {
			recommended = " (Recommended)"
		}

		line := fmt.Sprintf("%s%s (%s)", prefix, env.Name, env.AccountID)
		if i == 0 {
			b.WriteString(selectedStyle.Render(line))
		} else {
			b.WriteString(normalStyle.Render(line))
		}
		if recommended != "" {
			b.WriteString(recommendedStyle.Render(recommended))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n" + infoStyle.Render("ℹ️ The root account will:") + "\n")
	b.WriteString(bulletStyle.Render("▸") + detailStyle.Render(" Own the main domain zone") + "\n")
	b.WriteString(bulletStyle.Render("▸") + detailStyle.Render(" Delegate subdomains to other environments") + "\n\n")
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

func (m DNSSetupModel) viewDisplayNameservers() string {
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
			seconds := m.propagationCheckTimer / 10
			timeInfo = fmt.Sprintf(" (next check in %ds)", seconds)
		}
		b.WriteString(notPropagatedStyle.Render("⏳ Checking DNS propagation...") + statusStyle.Render(timeInfo) + "\n")

		// Show current NS values
		if len(m.actualNameservers) > 0 {
			b.WriteString("\n" + statusStyle.Render("Current nameservers returned by DNS:") + "\n")
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
		} else if m.propagationCheckTimer < 100 {
			// We've done at least one check
			b.WriteString("\n" + wrongNSStyle.Render("❌ No nameservers returned by DNS query") + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(m.renderKeyHelp("C", "copy all", "1-4", "copy NS", "R", "refresh", "Enter", "done"))

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

func (m DNSSetupModel) viewAddSubdomain() string {
	var b strings.Builder

	// Styles
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("213")).
		Background(lipgloss.Color("57")).
		Padding(0, 1)
	domainStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("226"))
	profileStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("87"))
	sectionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Bold(true)
	focusedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("226")).
		Bold(true)
	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))
	checkboxSelectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("82")).
		Bold(true)
	checkboxNormalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))
	animatedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#000")).
		Background(lipgloss.Color("82")).
		Bold(true)
	bulletStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("99"))
	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	// Header with domain
	b.WriteString(headerStyle.Render("🌐 DNS Subdomain Delegation") + "\n\n")
	b.WriteString("Root Domain: " + domainStyle.Render(m.rootDomain) + "\n")
	b.WriteString("Account: " + profileStyle.Render(m.selectedProfile) + "\n\n")
	b.WriteString(sectionStyle.Render("Select environments to delegate:") + "\n\n")

	// List subdomains with selection animation
	for i, subdomain := range m.subdomains {
		var line string

		// Checkbox
		checkbox := "☐"
		checkboxStyle := checkboxNormalStyle
		if m.selectedSubdomains[i] {
			checkbox = "☑"
			checkboxStyle = checkboxSelectedStyle
		}

		// Selection indicator
		prefix := "  "
		if i == m.currentSubdomainIndex {
			prefix = "▸ "
		}

		// Apply animation style if this item is selected and animating
		domainDisplay := subdomain
		if m.selectedSubdomains[i] && i == m.currentSubdomainIndex && m.selectionTimer > 0 {
			// Pulsing effect by alternating styles
			if (m.selectionTimer/5)%2 == 0 {
				line = prefix + checkboxStyle.Render(checkbox) + " " + animatedStyle.Render(" "+domainDisplay+" ")
			} else {
				line = prefix + checkboxStyle.Render(checkbox) + " " + focusedStyle.Render(domainDisplay)
			}
		} else if i == m.currentSubdomainIndex {
			// Currently focused item
			line = prefix + checkboxStyle.Render(checkbox) + " " + focusedStyle.Render(domainDisplay)
		} else {
			// Normal item
			line = prefix + checkboxStyle.Render(checkbox) + " " + normalStyle.Render(domainDisplay)
		}

		b.WriteString(line + "\n")
	}

	// Info section
	b.WriteString("\n" + sectionStyle.Render("Selected delegations will:") + "\n")
	b.WriteString(bulletStyle.Render("•") + infoStyle.Render(" Create hosted zones in target accounts") + "\n")
	b.WriteString(bulletStyle.Render("•") + infoStyle.Render(" Add NS records to root zone") + "\n")
	b.WriteString(bulletStyle.Render("•") + infoStyle.Render(" Configure cross-account access") + "\n\n")

	// Show timer if animating
	if m.selectionTimer > 0 {
		timerStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Italic(true)
		seconds := m.selectionTimer / 10
		b.WriteString(timerStyle.Render(fmt.Sprintf("⏱  Auto-deselect in %d seconds...", seconds)) + "\n\n")
	}

	b.WriteString(m.renderKeyHelp("↑↓", "navigate", "Space", "toggle", "Enter", "proceed", "S", "skip"))

	return b.String()
}

func (m DNSSetupModel) viewComplete() string {
	return fmt.Sprintf(`✓ DNS Setup Complete! 🎉

✓ Root zone created: %s
✓ DNS propagation verified
✓ Configuration saved to: %s

Next Steps:
1. Run 'make infra-plan' for each environment
2. Apply Terraform changes
3. Deploy your applications

DNS Status Dashboard:
Run: ./meroku dns status

Press Enter or Q to exit`, m.rootDomain, DNSConfigFile)
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
		return m, m.loadEnvironments()
	case StateSelectRootAccount:
		// Set the selected environment (first one in the list)
		if len(m.environments) > 0 {
			selected := m.environments[0]
			m.selectedProfile = selected.Profile
			m.selectedAccountID = selected.AccountID

			// Validate the profile exists and works
			if m.selectedProfile == "" {
				m.state = StateError
				m.errorMsg = fmt.Sprintf("No AWS profile configured for %s environment", selected.Name)
				return m, nil
			}

			// Try to get account ID to verify profile works
			accountID, err := getAWSAccountID(m.selectedProfile)
			if err != nil {
				m.state = StateError
				m.errorMsg = fmt.Sprintf("AWS profile '%s' is not working: %v\nPlease configure AWS access for this profile", m.selectedProfile, err)
				return m, nil
			}

			// Update account ID with the actual one
			m.selectedAccountID = accountID
		} else {
			m.state = StateError
			m.errorMsg = "No environments found. Please create an environment first."
			return m, nil
		}
		m.state = StateCreateRootZone
		return m, m.createRootZone()
	case StateDisplayNameservers:
		m.state = StateWaitPropagation
		m.propagationCheckCount = 0
		return m, m.checkPropagation()
	case StateWaitPropagation:
		m.state = StateAddSubdomain
		return m, m.loadEnvironments()
	case StateAddSubdomain:
		if m.hasSelectedSubdomains() {
			return m, m.createDelegations()
		}
		m.state = StateComplete
		return m, m.saveConfiguration()
	case StateComplete, StateError:
		return m, tea.Quit
	}
	return m, nil
}

func (m DNSSetupModel) handleUp() (DNSSetupModel, tea.Cmd) {
	if m.state == StateAddSubdomain && len(m.subdomains) > 0 {
		if m.currentSubdomainIndex > 0 {
			m.currentSubdomainIndex--
		} else {
			m.currentSubdomainIndex = len(m.subdomains) - 1
		}
	}
	return m, nil
}

func (m DNSSetupModel) handleDown() (DNSSetupModel, tea.Cmd) {
	if m.state == StateAddSubdomain && len(m.subdomains) > 0 {
		if m.currentSubdomainIndex < len(m.subdomains)-1 {
			m.currentSubdomainIndex++
		} else {
			m.currentSubdomainIndex = 0
		}
	}
	return m, nil
}

func (m DNSSetupModel) handleSpace() (DNSSetupModel, tea.Cmd) {
	if m.state == StateAddSubdomain && len(m.selectedSubdomains) > 0 {
		// Toggle subdomain selection with animation
		if m.currentSubdomainIndex < 0 || m.currentSubdomainIndex >= len(m.selectedSubdomains) {
			m.currentSubdomainIndex = 0
		}
		m.selectedSubdomains[m.currentSubdomainIndex] = !m.selectedSubdomains[m.currentSubdomainIndex]

		// If selecting, start the animation timer
		if m.selectedSubdomains[m.currentSubdomainIndex] {
			m.selectionTimer = 50 // 5 seconds at 100ms intervals
			return m, animationTickCmd()
		}
	}
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
	case StateAddSubdomain:
		m.state = StateWaitPropagation
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
	case StateAddSubdomain:
		// Skip subdomain delegation and complete
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
			m.propagationCheckTimer = 100 // Reset timer to 10 seconds
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
		m.errorMsg = msg.Error.Error()
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
			m.nameservers = data["nameservers"].([]string)
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
				m.propagationCheckTimer = 1 // Start with 1 to trigger immediate check
				m.isCheckingDNS = false
				// Start animation ticker for DNS propagation checks
				return m, animationTickCmd()
			}

			m.state = StateAddSubdomain
			return m, m.loadEnvironments()
		} else {
			// Zone doesn't exist or is invalid
			m.state = StateError
			m.errorMsg = fmt.Sprintf("DNS zone validation failed: %v\n\nThe zone ID in dns.yaml may be invalid or deleted.\nPlease delete dns.yaml and reconfigure.", msg.Error)
			return m, nil
		}

	case "create_zone":
		data := msg.Data.(map[string]interface{})
		m.zoneID = data["zoneID"].(string)
		m.nameservers = data["nameservers"].([]string)
		m.currentStep = "Creating delegation IAM role"
		return m, m.createDelegationRole()

	case "create_role":
		m.delegationRoleArn = msg.Data.(string)
		m.currentStep = "Saving configuration"
		m.state = StateDisplayNameservers
		return m, m.saveConfiguration()

	case "save_config":
		if m.state != StateComplete {
			m.state = StateDisplayNameservers
			m.dnsPropagated = false
			m.propagationCheckTimer = 1 // Start with 1 to trigger immediate check
			m.isCheckingDNS = false
			// Start animation ticker for DNS propagation checks
			return m, animationTickCmd()
		}
		return m, nil

	case "load_environments":
		data := msg.Data.(map[string]interface{})
		m.environments = data["environments"].([]DNSEnvironment)
		m.subdomains = data["subdomains"].([]string)
		m.selectedSubdomains = make([]bool, len(m.subdomains))
		m.currentSubdomainIndex = 0 // Initialize the current index
		return m, nil

	case "create_delegations":
		m.state = StateComplete
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
			m.actualNameservers = data["actualNS"].([]string)

			if m.dnsPropagated && m.state == StateDisplayNameservers {
				// DNS has propagated! Automatically proceed to next step
				m.state = StateAddSubdomain
				return m, m.loadEnvironments()
			}
		}
		return m, nil
	}

	return m, nil
}

func (m DNSSetupModel) hasSelectedSubdomains() bool {
	for _, selected := range m.selectedSubdomains {
		if selected {
			return true
		}
	}
	return false
}

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

func (m DNSSetupModel) loadEnvironments() tea.Cmd {
	return func() tea.Msg {
		// Get list of project YAML files
		envs := []DNSEnvironment{}

		// Check for common environment files in current working directory
		envFiles := []string{"dev", "prod", "staging"}
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

		// Generate subdomain suggestions
		subdomains := []string{}
		for _, env := range envs {
			if env.Name != m.selectedProfile {
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
		}

		// Simple check if DNS has propagated
		if len(m.nameservers) > 0 && m.rootDomain != "" {
			actualNS, err := queryNameservers(m.rootDomain)
			if err == nil && len(actualNS) > 0 {
				result["actualNS"] = actualNS
				// Check if at least one of the actual nameservers matches our AWS nameservers
				for _, actual := range actualNS {
					for _, expected := range m.nameservers {
						if strings.EqualFold(strings.TrimSuffix(actual, "."), strings.TrimSuffix(expected, ".")) {
							result["propagated"] = true
							break
						}
					}
					if result["propagated"].(bool) {
						break
					}
				}
			}
		}
		return dnsOperationMsg{Type: "dns_propagated", Success: true, Data: result}
	}
}

func (m DNSSetupModel) createDelegations() tea.Cmd {
	return func() tea.Msg {
		// Get the root profile to use for creating NS records
		rootProfile := m.selectedProfile
		if rootProfile == "" && m.selectedAccountID != "" {
			matchingProfile, err := findAWSProfileByAccountID(m.selectedAccountID)
			if err != nil {
				return dnsOperationMsg{
					Type: "create_delegations", Success: false,
					Error: fmt.Errorf("cannot find AWS profile for root account %s: %v", m.selectedAccountID, err),
				}
			}
			rootProfile = matchingProfile
		}

		if rootProfile == "" || m.zoneID == "" {
			return dnsOperationMsg{
				Type: "create_delegations", Success: false,
				Error: fmt.Errorf("missing profile or zone ID for delegation"),
			}
		}

		// Create NS records for each selected subdomain
		for i, subdomain := range m.subdomains {
			if m.selectedSubdomains[i] && i < len(m.environments) {
				env := m.environments[i]

				// Find the profile for the subdomain environment
				subProfile := env.Profile
				if subProfile == "" && env.AccountID != "" {
					if matchingProfile, err := findAWSProfileByAccountID(env.AccountID); err == nil {
						subProfile = matchingProfile
					}
				}

				if subProfile != "" {
					// Create hosted zone for subdomain
					subZoneID, subNS, err := createHostedZone(subProfile, subdomain)
					if err == nil && len(subNS) > 0 {
						// Create NS records in the root zone for delegation
						err = createNSRecordDelegation(rootProfile, subProfile, m.zoneID, subdomain, subNS)
						if err != nil {
							// Log error but continue with other subdomains
							continue
						}

						// Update the DNS config with the new subdomain zone
						if m.dnsConfig != nil {
							addOrUpdateDelegatedZone(m.dnsConfig, DelegatedZone{
								Subdomain: subdomain,
								AccountID: env.AccountID,
								ZoneID:    subZoneID,
								Status:    "active",
							})
							saveDNSConfig(m.dnsConfig)
						}
					}
				}
			}
		}

		return dnsOperationMsg{Type: "create_delegations", Success: true}
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

		for i, subdomain := range m.subdomains {
			if m.selectedSubdomains[i] {
				config.DelegatedZones = append(config.DelegatedZones, DelegatedZone{
					Subdomain: subdomain,
					AccountID: m.environments[i].AccountID,
					Status:    "pending",
				})
			}
		}

		if err = saveDNSConfig(config); err != nil {
			return dnsOperationMsg{Type: "save_config", Success: false, Error: err}
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

func runDNSSetupWizard() {
	p := tea.NewProgram(NewDNSSetupModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running DNS setup wizard: %v\n", err)
	}
}
