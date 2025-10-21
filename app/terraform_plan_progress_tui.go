package main

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

type planPhase int

const (
	phaseInitializing planPhase = iota
	phaseValidating
	phaseRefreshing
	phasePlanning
	phaseComplete
	phaseError
	phaseRecovering
)

type planProgressModel struct {
	phase           planPhase
	currentResource string
	processedItems  []string
	totalItems      int
	progress        float64
	outputLines     []string
	outputMutex     sync.Mutex
	errorDetails    string
	errorOutput     []string
	isRecoverable   bool
	width           int
	height          int
	startTime       time.Time
	cmd             *exec.Cmd
	done            chan bool
	ticker          *time.Ticker
	spinnerFrame    int
}

type progressMsg struct {
	line string
}

type planCompleteMsg struct {
	output string
}

type planErrorMsg struct {
	err    error
	output []string
}

func newPlanProgressModel() *planProgressModel {
	return &planProgressModel{
		phase:          phaseInitializing,
		processedItems: []string{},
		outputLines:    []string{},
		startTime:      time.Now(),
		done:           make(chan bool),
	}
}

func (m *planProgressModel) Init() tea.Cmd {
	return tea.Batch(
		m.runTerraformPlan(),
		m.tickCmd(),
	)
}

// tickCmd returns a command that ticks every 100ms to update the display
func (m *planProgressModel) tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type tickMsg time.Time

func (m *planProgressModel) runTerraformPlan() tea.Cmd {
	return func() tea.Msg {
		// Create the terraform plan command with no color output for easier parsing
		cmd := exec.Command("terraform", "plan", "-no-color", "-out=tfplan")
		m.cmd = cmd
		
		// Create pipes for stdout and stderr
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return planErrorMsg{err: err, output: []string{err.Error()}}
		}
		
		stderr, err := cmd.StderrPipe()
		if err != nil {
			return planErrorMsg{err: err, output: []string{err.Error()}}
		}
		
		// Start the command
		if err := cmd.Start(); err != nil {
			return planErrorMsg{err: err, output: []string{err.Error()}}
		}
		
		// Process stdout in a goroutine
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				line := scanner.Text()
				m.outputMutex.Lock()
				m.outputLines = append(m.outputLines, line)
				// Keep only last 100 lines to prevent memory issues
				if len(m.outputLines) > 100 {
					m.outputLines = m.outputLines[len(m.outputLines)-100:]
				}
				m.outputMutex.Unlock()
			}
		}()
		
		// Process stderr in a goroutine
		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				line := scanner.Text()
				m.outputMutex.Lock()
				m.errorOutput = append(m.errorOutput, line)
				// Also add to output lines so we can see errors in the display
				m.outputLines = append(m.outputLines, "ERROR: " + line)
				if len(m.outputLines) > 100 {
					m.outputLines = m.outputLines[len(m.outputLines)-100:]
				}
				m.outputMutex.Unlock()
			}
		}()
		
		// Wait for command to complete
		err = cmd.Wait()
		
		if err != nil {
			m.outputMutex.Lock()
			errorLines := make([]string, len(m.errorOutput))
			copy(errorLines, m.errorOutput)
			m.outputMutex.Unlock()
			
			if len(errorLines) == 0 {
				m.outputMutex.Lock()
				errorLines = make([]string, len(m.outputLines))
				copy(errorLines, m.outputLines)
				m.outputMutex.Unlock()
			}
			return planErrorMsg{err: err, output: errorLines}
		}
		
		// Get final output for next stage
		m.outputMutex.Lock()
		finalLines := make([]string, len(m.outputLines))
		copy(finalLines, m.outputLines)
		m.outputMutex.Unlock()
		
		return planCompleteMsg{output: strings.Join(finalLines, "\n")}
	}
}

func (m *planProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
		
	case tickMsg:
		// Increment spinner frame
		m.spinnerFrame++
		
		// Process any new output lines
		m.outputMutex.Lock()
		lines := make([]string, len(m.outputLines))
		copy(lines, m.outputLines)
		m.outputMutex.Unlock()
		
		// Parse the latest lines to determine phase and current resource
		for _, line := range lines {
			if strings.Contains(line, "Initializing") {
				m.phase = phaseInitializing
				m.currentResource = "Initializing Terraform providers..."
			} else if strings.Contains(line, "Terraform used the selected providers") {
				m.phase = phaseValidating
				m.currentResource = "Validating configuration..."
			} else if strings.Contains(line, "Refreshing state") || strings.Contains(line, "Reading...") {
				m.phase = phaseRefreshing
				// Extract resource name
				if strings.Contains(line, "[id=") {
					parts := strings.Split(line, "[id=")
					if len(parts) > 0 {
						resourceName := strings.TrimSpace(parts[0])
						resourceName = strings.TrimPrefix(resourceName, "Refreshing state... ")
						resourceName = strings.TrimPrefix(resourceName, "Reading... ")
						m.currentResource = resourceName
						// Add to processed items if not already there
						if len(m.processedItems) == 0 || m.processedItems[len(m.processedItems)-1] != resourceName {
							m.processedItems = append(m.processedItems, resourceName)
						}
					}
				} else {
					m.currentResource = strings.TrimSpace(line)
				}
			} else if strings.Contains(line, "Terraform will perform") {
				m.phase = phasePlanning
				m.currentResource = "Calculating changes..."
			} else if strings.Contains(line, "Plan:") && strings.Contains(line, "add") {
				m.phase = phaseComplete
				m.currentResource = line
			} else if strings.Contains(line, "No changes") {
				m.phase = phaseComplete
				m.currentResource = "No changes required"
			}
		}
		
		// Continue ticking if not complete
		if m.phase != phaseComplete && m.phase != phaseError {
			return m, m.tickCmd()
		}
		return m, nil
		
	case planCompleteMsg:
		m.phase = phaseComplete
		m.progress = 1.0
		return m, tea.Quit
		
	case planErrorMsg:
		m.errorDetails = msg.err.Error()
		m.errorOutput = msg.output

		// Check if error is recoverable before showing error screen
		errorText := strings.Join(msg.output, "\n")
		if errorText == "" {
			errorText = msg.err.Error()
		}

		_, recoverErr := terraformError(errorText)
		if recoverErr == nil {
			// Error is recoverable - don't show error screen, just quit
			// The caller (runTerraformApply) will handle recovery
			m.isRecoverable = true
			m.phase = phaseRecovering
			return m, tea.Quit
		}

		// Error is not recoverable - show error screen
		m.isRecoverable = false
		m.phase = phaseError
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			if m.cmd != nil {
				m.cmd.Process.Kill()
			}
			return m, tea.Quit
		case "enter":
			if m.phase == phaseError || m.phase == phaseComplete {
				return m, tea.Quit
			}
		}
	}
	
	return m, nil
}

func (m *planProgressModel) View() string {
	if m.phase == phaseError {
		return m.renderError()
	}

	if m.phase == phaseRecovering {
		return m.renderRecovering()
	}

	// Get output copy
	m.outputMutex.Lock()
	outputCopy := make([]string, len(m.outputLines))
	copy(outputCopy, m.outputLines)
	m.outputMutex.Unlock()

	// Calculate content width - leave margins on sides
	contentWidth := min(m.width-8, 120)

	var content strings.Builder

	// ═══════════════════════════════════════
	// TITLE SECTION
	// ═══════════════════════════════════════
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("220")).
		Align(lipgloss.Center).
		Width(contentWidth)

	content.WriteString(titleStyle.Render("🔄 Terraform Plan Progress"))
	content.WriteString("\n\n")

	// ═══════════════════════════════════════
	// STATUS SECTION
	// ═══════════════════════════════════════
	phaseText := m.getPhaseText()

	statusStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		Align(lipgloss.Center).
		Width(contentWidth)

	content.WriteString(statusStyle.Render(phaseText))
	content.WriteString("\n")

	// Current resource being processed
	if m.currentResource != "" {
		resourceStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Align(lipgloss.Center).
			Width(contentWidth)

		content.WriteString(resourceStyle.Render("Processing: " + m.currentResource))
	}
	content.WriteString("\n")

	// ═══════════════════════════════════════
	// OUTPUT BOX (ALWAYS SHOWN)
	// ═══════════════════════════════════════
	labelStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		Width(contentWidth)

	content.WriteString(labelStyle.Render("Terraform Output:"))
	content.WriteString("\n")

	// Calculate dynamic height for output box
	outputHeight := 15
	if m.height > 30 {
		outputHeight = m.height - 15
	} else if m.height > 20 {
		outputHeight = m.height - 10
	}

	// Create bordered output box
	outputBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Width(contentWidth).
		Height(outputHeight)

	var outputContent strings.Builder

	if len(outputCopy) == 0 {
		// Show waiting message when no output yet
		waitingStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true).
			Align(lipgloss.Center).
			Width(contentWidth - 6)

		outputContent.WriteString(waitingStyle.Render("Waiting for Terraform output..."))
	} else {
		// Show actual output
		linesToShow := outputHeight - 4 // Account for padding and borders
		start := 0
		if len(outputCopy) > linesToShow {
			start = len(outputCopy) - linesToShow
		}

		maxLineWidth := contentWidth - 6 // Account for padding
		for i := start; i < len(outputCopy); i++ {
			line := outputCopy[i]

			// Truncate long lines
			if len(line) > maxLineWidth {
				line = line[:maxLineWidth-3] + "..."
			}

			// Color-code important lines
			if strings.Contains(line, "Plan:") {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true).Render(line)
			} else if strings.Contains(line, "No changes") {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render(line)
			} else if strings.Contains(line, "ERROR") || strings.Contains(line, "Error") {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(line)
			}

			outputContent.WriteString(line + "\n")
		}
	}

	content.WriteString(outputBoxStyle.Render(outputContent.String()))
	content.WriteString("\n")

	// ═══════════════════════════════════════
	// FOOTER SECTION (Stats & Instructions)
	// ═══════════════════════════════════════
	var footerParts []string

	// Progress stats
	if len(m.processedItems) > 0 {
		footerParts = append(footerParts,
			fmt.Sprintf("Processed: %d resources", len(m.processedItems)))
	}

	// Elapsed time
	elapsed := time.Since(m.startTime).Round(time.Second)
	footerParts = append(footerParts, fmt.Sprintf("Elapsed: %s", elapsed))

	// Instructions
	if m.phase == phaseComplete {
		footerParts = append(footerParts, "✅ Press Enter to continue")
	} else {
		footerParts = append(footerParts, "Press q or Ctrl+C to cancel")
	}

	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Align(lipgloss.Center).
		Width(contentWidth)

	content.WriteString(footerStyle.Render(strings.Join(footerParts, " • ")))

	// ═══════════════════════════════════════
	// CENTER EVERYTHING HORIZONTALLY
	// ═══════════════════════════════════════
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Top,
		content.String(),
		lipgloss.WithWhitespaceChars(" "),
	)
}

func (m *planProgressModel) renderRecovering() string {
	recoveringStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("214")). // Orange color
		Padding(1, 2)

	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	frame := spinner[m.spinnerFrame%len(spinner)]

	content := fmt.Sprintf("%s Attempting auto-recovery...\n\n"+
		"Detected a recoverable error. Running automatic fix.\n"+
		"Please wait...", frame)

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		recoveringStyle.Render(content),
	)
}

func (m *planProgressModel) renderError() string {
	// Error styles
	errorTitleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196")).
		Background(lipgloss.Color("52")).
		Padding(1, 2).
		MarginBottom(1)
	
	errorBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(lipgloss.Color("196")).
		Padding(1).
		Width(100)
	
	// Create error table
	errorTable := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("196"))).
		Width(96).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == 0 {
				return lipgloss.NewStyle().
					Bold(true).
					Foreground(lipgloss.Color("196")).
					Align(lipgloss.Center)
			}
			return lipgloss.NewStyle().
				Foreground(lipgloss.Color("251")).
				PaddingLeft(1).
				PaddingRight(1)
		}).
		Headers("Error Details")
	
	// Add error message
	errorTable.Row(m.errorDetails)
	
	// Add error output lines if available
	if len(m.errorOutput) > 0 {
		for _, line := range m.errorOutput {
			if strings.TrimSpace(line) != "" {
				errorTable.Row(line)
			}
		}
	}
	
	// Build the error view
	var content strings.Builder
	
	content.WriteString(errorTitleStyle.Render("❌ TERRAFORM PLAN FAILED"))
	content.WriteString("\n\n")
	content.WriteString(errorTable.String())
	content.WriteString("\n\n")
	
	// Add suggestions based on error type
	suggestions := m.getErrorSuggestions()
	if suggestions != "" {
		suggestionStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Bold(true)
		
		content.WriteString(suggestionStyle.Render("💡 Suggestions:"))
		content.WriteString("\n")
		content.WriteString(suggestions)
		content.WriteString("\n\n")
	}
	
	content.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Render("Press Enter to return to menu or q to quit"))
	
	// Wrap in error box
	errorContent := errorBoxStyle.Render(content.String())
	
	// Use full height, center horizontally only
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Top,
		errorContent,
	)
}

func (m *planProgressModel) getPhaseText() string {
	switch m.phase {
	case phaseInitializing:
		return "🔧 Initializing Terraform..."
	case phaseValidating:
		return "✓ Validating configuration..."
	case phaseRefreshing:
		return "🔄 Refreshing state..."
	case phasePlanning:
		return "📋 Planning changes..."
	case phaseComplete:
		return "✅ Complete!"
	default:
		return "⏳ Processing..."
	}
}

func (m *planProgressModel) renderProgressBar() string {
	// Spinner frames for infinite loading
	spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	frame := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
	
	spinnerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("82")).
		Bold(true)
	
	countStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("220"))
	
	// Show spinner with processed items count
	processedCount := len(m.processedItems)
	
	var status string
	switch m.phase {
	case phaseRefreshing:
		status = fmt.Sprintf("Refreshing state... %d resources processed", processedCount)
	case phasePlanning:
		status = fmt.Sprintf("Planning changes... %d resources analyzed", processedCount)
	default:
		status = "Processing..."
	}
	
	return spinnerStyle.Render(frame) + " " + countStyle.Render(status)
}

func (m *planProgressModel) getErrorSuggestions() string {
	errorStr := strings.ToLower(m.errorDetails + strings.Join(m.errorOutput, " "))
	
	var suggestions []string
	
	if strings.Contains(errorStr, "backend") || strings.Contains(errorStr, "init") {
		suggestions = append(suggestions, "• Run 'terraform init' or 'terraform init -reconfigure'")
	}
	
	if strings.Contains(errorStr, "credentials") || strings.Contains(errorStr, "authentication") {
		suggestions = append(suggestions, "• Check your AWS credentials and profile configuration")
		suggestions = append(suggestions, "• Ensure AWS_PROFILE is set correctly")
	}
	
	if strings.Contains(errorStr, "module") {
		suggestions = append(suggestions, "• Run 'terraform init' to download required modules")
	}
	
	if strings.Contains(errorStr, "state") {
		suggestions = append(suggestions, "• Check if the state file is locked by another process")
		suggestions = append(suggestions, "• Verify remote state configuration")
	}
	
	if strings.Contains(errorStr, "syntax") || strings.Contains(errorStr, "parse") {
		suggestions = append(suggestions, "• Check your Terraform configuration files for syntax errors")
		suggestions = append(suggestions, "• Run 'terraform validate' to check configuration")
	}
	
	if len(suggestions) == 0 {
		suggestions = append(suggestions, "• Check the error details above for more information")
		suggestions = append(suggestions, "• Ensure all required resources and permissions are available")
	}
	
	return strings.Join(suggestions, "\n")
}

// Helper function to run the TUI
func runTerraformPlanWithProgress() error {
	p := tea.NewProgram(
		newPlanProgressModel(),
		tea.WithAltScreen(),
	)
	
	model, err := p.Run()
	if err != nil {
		return fmt.Errorf("error running plan progress TUI: %w", err)
	}
	
	// Check if there was an error in the plan
	if finalModel, ok := model.(*planProgressModel); ok {
		if finalModel.phase == phaseError || finalModel.phase == phaseRecovering {
			// Include both the error details and the full output for error recovery
			errorMsg := finalModel.errorDetails
			if len(finalModel.errorOutput) > 0 {
				errorMsg = strings.Join(finalModel.errorOutput, "\n")
			}
			// Return error so runTerraformApply can attempt recovery
			return fmt.Errorf("terraform plan failed: %s", errorMsg)
		}
	}

	return nil
}