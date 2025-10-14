package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type nukeStage int

const (
	selectEnvStage nukeStage = iota
	showDetailsStage
	firstConfirmStage
	projectNameStage
	finalPhraseStage
	destroyingStage
	completeStage
	cancelledStage
)

type nukeModel struct {
	stage           nukeStage
	selectedEnv     string
	projectName     string
	envData         Env
	input           string
	cursorPos       int
	width           int
	height          int
	error           string
	envs            []string
	selectedEnvIdx  int
	yesNoSelected   int // 0 for Yes, 1 for No
	animationFrame  int
	animationTicker *time.Ticker
}

type nukeCompleteMsg struct{}
type nukeErrorMsg struct{ err error }
type nukeAnimationTickMsg time.Time

func initNukeTUI(selectedEnv string) (*nukeModel, error) {
	m := &nukeModel{
		selectedEnv: selectedEnv,
		input:       "",
		cursorPos:   0,
	}

	// Get available environments
	envs, err := findFilesWithExts([]string{".yaml", ".yml"})
	if err != nil {
		return nil, err
	}

	// Filter out DNS config
	var filteredEnvs []string
	for _, env := range envs {
		if env != "dns.yaml" {
			filteredEnvs = append(filteredEnvs, env)
		}
	}
	m.envs = filteredEnvs

	if selectedEnv != "" {
		// Load environment data
		e, err := loadEnv(selectedEnv)
		if err != nil {
			return nil, fmt.Errorf("failed to load environment: %w", err)
		}
		m.envData = e
		m.projectName = e.Project
		m.stage = showDetailsStage
	} else {
		m.stage = selectEnvStage
	}

	return m, nil
}

func (m *nukeModel) Init() tea.Cmd {
	return nil
}

func (m *nukeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case nukeAnimationTickMsg:
		if m.stage == destroyingStage {
			m.animationFrame = (m.animationFrame + 1) % 24 // 24 frame animation
			return m, m.tickAnimation()
		}
		return m, nil

	case nukeErrorMsg:
		m.error = msg.err.Error()
		m.stage = cancelledStage
		if m.animationTicker != nil {
			m.animationTicker.Stop()
		}
		return m, nil

	case nukeCompleteMsg:
		m.stage = completeStage
		if m.animationTicker != nil {
			m.animationTicker.Stop()
		}
		return m, nil

	case tea.KeyMsg:
		switch m.stage {
		case selectEnvStage:
			return m.handleSelectEnvKeys(msg)
		case showDetailsStage:
			return m.handleShowDetailsKeys(msg)
		case firstConfirmStage:
			return m.handleFirstConfirmKeys(msg)
		case projectNameStage:
			return m.handleProjectNameKeys(msg)
		case finalPhraseStage:
			return m.handleFinalPhraseKeys(msg)
		case completeStage, cancelledStage:
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m *nukeModel) handleSelectEnvKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		m.stage = cancelledStage
		return m, tea.Quit
	case "up", "k":
		if m.selectedEnvIdx > 0 {
			m.selectedEnvIdx--
		}
	case "down", "j":
		if m.selectedEnvIdx < len(m.envs)-1 {
			m.selectedEnvIdx++
		}
	case "enter":
		m.selectedEnv = m.envs[m.selectedEnvIdx]
		// Load environment data
		e, err := loadEnv(m.selectedEnv)
		if err != nil {
			return m, func() tea.Msg { return nukeErrorMsg{err: err} }
		}
		m.envData = e
		m.projectName = e.Project
		m.stage = showDetailsStage
		return m, nil
	}
	return m, nil
}

func (m *nukeModel) handleShowDetailsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		m.stage = cancelledStage
		return m, tea.Quit
	case "enter", " ":
		m.stage = firstConfirmStage
		return m, nil
	}
	return m, nil
}

func (m *nukeModel) handleFirstConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.stage = cancelledStage
		return m, tea.Quit
	case "left", "h":
		m.yesNoSelected = 0
	case "right", "l":
		m.yesNoSelected = 1
	case "tab":
		m.yesNoSelected = (m.yesNoSelected + 1) % 2
	case "enter":
		if m.yesNoSelected == 1 {
			m.stage = cancelledStage
			return m, tea.Quit
		}
		m.stage = projectNameStage
		m.input = ""
		return m, nil
	}
	return m, nil
}

func (m *nukeModel) handleProjectNameKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.stage = cancelledStage
		return m, tea.Quit
	case "enter":
		if strings.TrimSpace(m.input) == m.projectName {
			m.stage = finalPhraseStage
			m.input = ""
			return m, nil
		}
		m.error = fmt.Sprintf("Project name does not match (expected: %s)", m.projectName)
		return m, nil
	case "backspace":
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	default:
		if len(msg.Runes) == 1 {
			m.input += string(msg.Runes[0])
		}
	}
	return m, nil
}

func (m *nukeModel) handleFinalPhraseKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.stage = cancelledStage
		return m, tea.Quit
	case "enter":
		if strings.ToLower(strings.TrimSpace(m.input)) == "destroy everything" {
			m.stage = destroyingStage
			return m, m.startDestroy()
		}
		m.error = "Confirmation phrase does not match (expected: destroy everything)"
		return m, nil
	case "backspace":
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	default:
		if len(msg.Runes) == 1 {
			m.input += string(msg.Runes[0])
		}
	}
	return m, nil
}

func (m *nukeModel) tickAnimation() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return nukeAnimationTickMsg(t)
	})
}

func (m *nukeModel) startDestroy() tea.Cmd {
	return tea.Batch(
		m.tickAnimation(),
		func() tea.Msg {
			if err := runCommandToNuke(m.selectedEnv); err != nil {
				return nukeErrorMsg{err: err}
			}
			return nukeCompleteMsg{}
		},
	)
}

func (m *nukeModel) View() string {
	switch m.stage {
	case selectEnvStage:
		return m.viewSelectEnv()
	case showDetailsStage:
		return m.viewShowDetails()
	case firstConfirmStage:
		return m.viewFirstConfirm()
	case projectNameStage:
		return m.viewProjectName()
	case finalPhraseStage:
		return m.viewFinalPhrase()
	case destroyingStage:
		return m.viewDestroying()
	case completeStage:
		return m.viewComplete()
	case cancelledStage:
		return m.viewCancelled()
	}
	return ""
}

func (m *nukeModel) viewSelectEnv() string {
	style := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Padding(2, 4)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("9")).
		MarginBottom(1)

	subtitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Italic(true).
		MarginBottom(2)

	itemStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		MarginLeft(2)

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("9")).
		Bold(true).
		MarginLeft(0).
		PaddingLeft(2).
		PaddingRight(2)

	var content strings.Builder
	content.WriteString(titleStyle.Render("💥 Nuke Environment - Select Target"))
	content.WriteString("\n")
	content.WriteString(subtitle.Render("⚠️  Warning: This will DESTROY all infrastructure resources!"))
	content.WriteString("\n\n")

	for i, env := range m.envs {
		if i == m.selectedEnvIdx {
			content.WriteString(selectedStyle.Render(fmt.Sprintf("▶ %s", env)))
		} else {
			content.WriteString(itemStyle.Render(fmt.Sprintf("  %s", env)))
		}
		content.WriteString("\n")
	}

	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("↑/↓: Navigate • Enter: Select • Esc/Q: Cancel"))

	return style.Render(content.String())
}

func (m *nukeModel) viewShowDetails() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("9")).
		Padding(1, 2).
		Width(60)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("9")).
		MarginBottom(1)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Bold(true)

	versionValueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true)

	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("11")).
		Bold(true).
		MarginTop(2).
		MarginBottom(2)

	var content strings.Builder
	content.WriteString(titleStyle.Render("🔥 Environment Destruction Details\n\n"))

	// Format with consistent spacing - labels on left, values on right
	content.WriteString(labelStyle.Render("Version:") + "          " + versionValueStyle.Render(GetVersion()) + "\n")
	content.WriteString(labelStyle.Render("Environment:") + "     " + valueStyle.Render(m.selectedEnv) + "\n")
	content.WriteString(labelStyle.Render("Project:") + "          " + valueStyle.Render(m.envData.Project) + "\n")
	content.WriteString(labelStyle.Render("Region:") + "           " + valueStyle.Render(m.envData.Region) + "\n")
	if m.envData.AccountID != "" {
		content.WriteString(labelStyle.Render("AWS Account:") + "    " + valueStyle.Render(m.envData.AccountID) + "\n")
	}

	content.WriteString("\n")
	content.WriteString(warningStyle.Render("⚠️  ALL RESOURCES WILL BE PERMANENTLY DELETED"))

	content.WriteString("\n\n")
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("Press Enter to continue • Esc to cancel"))

	centerStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		AlignHorizontal(lipgloss.Center).
		AlignVertical(lipgloss.Center)

	return centerStyle.Render(boxStyle.Render(content.String()))
}

func (m *nukeModel) viewFirstConfirm() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(lipgloss.Color("9")).
		Padding(2, 4).
		Width(70)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("9")).
		Align(lipgloss.Center).
		MarginBottom(2)

	questionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Align(lipgloss.Center).
		MarginBottom(3)

	buttonStyle := lipgloss.NewStyle().
		Padding(0, 3).
		Margin(0, 1)

	selectedButtonStyle := buttonStyle.Copy().
		Foreground(lipgloss.Color("0")).
		Background(lipgloss.Color("9")).
		Bold(true)

	unselectedButtonStyle := buttonStyle.Copy().
		Foreground(lipgloss.Color("250")).
		Background(lipgloss.Color("236"))

	var content strings.Builder
	content.WriteString(titleStyle.Render("⚠️  CONFIRMATION REQUIRED"))
	content.WriteString("\n\n")
	content.WriteString(questionStyle.Render(fmt.Sprintf("Are you ABSOLUTELY SURE you want to\ndestroy environment '%s'?", m.selectedEnv)))
	content.WriteString("\n")

	// Render buttons
	var buttons string
	if m.yesNoSelected == 0 {
		buttons = selectedButtonStyle.Render("[ YES ]") + unselectedButtonStyle.Render("  NO  ")
	} else {
		buttons = unselectedButtonStyle.Render("  YES  ") + selectedButtonStyle.Render("[ NO ]")
	}
	content.WriteString(lipgloss.NewStyle().Align(lipgloss.Center).Render(buttons))

	content.WriteString("\n\n")
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Align(lipgloss.Center).Render("← →: Navigate • Enter: Confirm • Esc: Cancel"))

	centerStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		AlignHorizontal(lipgloss.Center).
		AlignVertical(lipgloss.Center)

	return centerStyle.Render(boxStyle.Render(content.String()))
}

func (m *nukeModel) viewProjectName() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("11")).
		Padding(2, 3).
		Width(65)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("11")).
		Align(lipgloss.Center).
		MarginBottom(2)

	instructionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		Align(lipgloss.Center).
		MarginBottom(2)

	inputBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("11")).
		Padding(0, 1).
		Width(50).
		Align(lipgloss.Center)

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("9")).
		Align(lipgloss.Center).
		MarginTop(1)

	var content strings.Builder
	content.WriteString(titleStyle.Render("🔐 Security Verification - Step 2/3"))
	content.WriteString("\n\n")
	content.WriteString(instructionStyle.Render(fmt.Sprintf("Type the project name '%s' to confirm:", m.projectName)))
	content.WriteString("\n\n")

	inputDisplay := m.input + "█"
	content.WriteString(lipgloss.NewStyle().Align(lipgloss.Center).Render(inputBoxStyle.Render(inputDisplay)))

	if m.error != "" {
		content.WriteString("\n\n")
		content.WriteString(errorStyle.Render("❌ " + m.error))
		m.error = ""
	}

	content.WriteString("\n\n")
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Align(lipgloss.Center).Render("Enter: Confirm • Esc: Cancel"))

	centerStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		AlignHorizontal(lipgloss.Center).
		AlignVertical(lipgloss.Center)

	return centerStyle.Render(boxStyle.Render(content.String()))
}

func (m *nukeModel) viewFinalPhrase() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("196")).
		Padding(2, 3).
		Width(70)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196")).
		Align(lipgloss.Center).
		MarginBottom(2)

	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Bold(true).
		Align(lipgloss.Center).
		MarginBottom(2)

	instructionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		Align(lipgloss.Center).
		MarginBottom(2)

	inputBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("196")).
		Padding(0, 1).
		Width(55).
		Align(lipgloss.Center)

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("9")).
		Align(lipgloss.Center).
		MarginTop(1)

	var content strings.Builder
	content.WriteString(titleStyle.Render("🚨 FINAL WARNING - Step 3/3"))
	content.WriteString("\n\n")
	content.WriteString(warningStyle.Render("THIS IS YOUR LAST CHANCE TO CANCEL!"))
	content.WriteString("\n\n")
	content.WriteString(instructionStyle.Render("Type 'destroy everything' to proceed:"))
	content.WriteString("\n\n")

	inputDisplay := m.input + "█"
	content.WriteString(lipgloss.NewStyle().Align(lipgloss.Center).Render(inputBoxStyle.Render(inputDisplay)))

	if m.error != "" {
		content.WriteString("\n\n")
		content.WriteString(errorStyle.Render("❌ " + m.error))
		m.error = ""
	}

	content.WriteString("\n\n")
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Align(lipgloss.Center).Render("Enter: DESTROY • Esc: Cancel"))

	centerStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		AlignHorizontal(lipgloss.Center).
		AlignVertical(lipgloss.Center)

	return centerStyle.Render(boxStyle.Render(content.String()))
}

func (m *nukeModel) viewDestroying() string {
	// Nuclear explosion ASCII art animation frames
	explosionFrames := []string{
		// Frame 0-3: Initial blast
		`
                 .
                .:.
               .:::.
              .:::::.
             .:::::::.
        `,
		`
                ...
              .......
             .........
            ...........
           .............
          ...............
         .................
        `,
		`
              *****
            *********
          *************
         ***************
        *****************
       *******************
        *****************
         ***************
          *************
        `,
		`
           ╔═══════╗
          ║░░░░░░░░║
         ║░░█████░░░║
        ║░░███████░░░║
       ║░░█████████░░░║
      ║░░███████████░░░║
       ║░░█████████░░░║
        ║░░███████░░░║
         ║░░█████░░░║
          ║░░░░░░░░║
           ╚═══════╝
        `,
		// Frame 4-7: Mushroom forming
		`
              ▄▄▄▄
           ▄▄████▄▄
          ██████████
         ████████████
        ▄██████████▄
         ▀████████▀
           ▀████▀
             ██
             ██
             ██
        `,
		`
            ▄████▄
          ▄████████▄
         ████████████
        ██████████████
       ▄██████████████▄
        ▀████████████▀
          ▀████████▀
            ▀████▀
              ██
              ██
              ██
        `,
		`
           ╔══════════╗
          ║░▄████████▄░║
         ║░████████████░║
        ║░██████████████░║
       ║░████████████████░║
        ║░▀████████████▀░║
         ║░░▀████████▀░░║
          ║░░░▀████▀░░░║
           ╚════██════╝
                ██
                ██
        `,
		`
           ⚡ BOOM! ⚡
          ╔══════════╗
         ║▓▓▓▓▓▓▓▓▓▓▓║
        ║▓░████████░▓▓║
       ║▓░██████████░▓▓║
      ║▓░████████████░▓▓║
       ║▓░██████████░▓▓║
        ║▓░████████░▓▓║
         ║▓▓▓▓▓▓▓▓▓▓▓║
          ╚════██════╝
               ██
               ██
        `,
	}

	// Colors for different stages
	colors := []string{
		"11",  // Yellow
		"220", // Bright yellow
		"214", // Orange
		"208", // Dark orange
		"202", // Red-orange
		"196", // Red
		"160", // Dark red
		"124", // Deep red
	}

	// Determine which frame and color to use
	frameIndex := m.animationFrame / 3 // Slow down animation
	if frameIndex >= len(explosionFrames) {
		frameIndex = len(explosionFrames) - 1
	}

	colorIndex := m.animationFrame / 3
	if colorIndex >= len(colors) {
		colorIndex = len(colors) - 1
	}

	explosionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colors[colorIndex])).
		Bold(true).
		Align(lipgloss.Center)

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true).
		Align(lipgloss.Center).
		MarginBottom(2)

	messageStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		Align(lipgloss.Center).
		MarginTop(2)

	progressDots := strings.Repeat(".", (m.animationFrame%4)+1)

	var content strings.Builder
	content.WriteString(titleStyle.Render("💥 INFRASTRUCTURE DESTRUCTION IN PROGRESS 💥"))
	content.WriteString("\n\n")
	content.WriteString(explosionStyle.Render(explosionFrames[frameIndex]))
	content.WriteString("\n\n")
	content.WriteString(messageStyle.Render(fmt.Sprintf("🔥 Destroying %s%s", m.selectedEnv, progressDots)))
	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Align(lipgloss.Center).Render("This may take several minutes..."))

	centerStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		AlignHorizontal(lipgloss.Center).
		AlignVertical(lipgloss.Center)

	return centerStyle.Render(content.String())
}

func (m *nukeModel) viewComplete() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("10")).
		Padding(2, 4).
		Width(60)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("10")).
		Align(lipgloss.Center).
		MarginBottom(2)

	messageStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		Align(lipgloss.Center)

	noteStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Italic(true).
		Align(lipgloss.Center).
		MarginTop(2)

	var content strings.Builder
	content.WriteString(titleStyle.Render("✅ DESTRUCTION COMPLETE"))
	content.WriteString("\n\n")
	content.WriteString(messageStyle.Render(fmt.Sprintf("Environment '%s' has been destroyed.", m.selectedEnv)))
	content.WriteString("\n\n")
	content.WriteString(noteStyle.Render(fmt.Sprintf("💡 Configuration file '%s.yaml' preserved.\nYou can redeploy later if needed.", m.selectedEnv)))
	content.WriteString("\n\n")
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Align(lipgloss.Center).Render("Press any key to exit"))

	centerStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		AlignHorizontal(lipgloss.Center).
		AlignVertical(lipgloss.Center)

	return centerStyle.Render(boxStyle.Render(content.String()))
}

func (m *nukeModel) viewCancelled() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("10")).
		Padding(2, 4).
		Width(50)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("10")).
		Align(lipgloss.Center).
		MarginBottom(2)

	messageStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		Align(lipgloss.Center)

	var content strings.Builder
	content.WriteString(titleStyle.Render("✅ CANCELLED"))
	content.WriteString("\n\n")
	content.WriteString(messageStyle.Render("No resources were destroyed.\nYour infrastructure is safe."))
	content.WriteString("\n\n")
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Align(lipgloss.Center).Render("Press any key to exit"))

	centerStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		AlignHorizontal(lipgloss.Center).
		AlignVertical(lipgloss.Center)

	return centerStyle.Render(boxStyle.Render(content.String()))
}

func runCommandToNuke(env string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("error getting current working directory: %w", err)
	}
	defer os.Chdir(wd)

	// Check if env directory exists
	envPath := filepath.Join("env", env)
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		return fmt.Errorf("environment directory not found: %s (has this environment been deployed?)", envPath)
	}

	// Change to environment directory
	err = os.Chdir(envPath)
	if err != nil {
		return fmt.Errorf("error changing directory to env folder: %w", err)
	}

	// Ensure terraform is initialized
	terraformInitIfNeeded()

	// Run terraform destroy
	return runTerraformDestroy()
}
