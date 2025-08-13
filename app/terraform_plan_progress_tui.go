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
	width           int
	height          int
	startTime       time.Time
	cmd             *exec.Cmd
	done            chan bool
	ticker          *time.Ticker
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
		m.phase = phaseError
		m.errorDetails = msg.err.Error()
		m.errorOutput = msg.output
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
	
	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("220")).
		MarginBottom(1)
	
	phaseStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86"))
	
	resourceStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39"))
	
	timeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))
	
	// Calculate elapsed time
	elapsed := time.Since(m.startTime).Round(time.Second)
	
	// Build the view
	var content strings.Builder
	
	content.WriteString(titleStyle.Render("🔄 Terraform Plan Progress"))
	content.WriteString("\n\n")
	
	// Current phase
	phaseText := m.getPhaseText()
	content.WriteString(phaseStyle.Render("Phase: ") + phaseText)
	content.WriteString("\n\n")
	
	// Current resource being processed
	if m.currentResource != "" {
		content.WriteString(resourceStyle.Render("Processing: ") + m.currentResource)
		content.WriteString("\n\n")
	}
	
	// Progress bar
	if m.phase == phaseRefreshing || m.phase == phasePlanning {
		progressBar := m.renderProgressBar()
		content.WriteString(progressBar)
		content.WriteString("\n\n")
	}
	
	// Show recent output lines (last 10)
	m.outputMutex.Lock()
	outputCopy := make([]string, len(m.outputLines))
	copy(outputCopy, m.outputLines)
	m.outputMutex.Unlock()
	
	if len(outputCopy) > 0 {
		content.WriteString(phaseStyle.Render("Live Output:"))
		content.WriteString("\n")
		
		// Create a box for output
		outputBox := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1).
			Width(80).
			Height(10)
		
		// Get last 8 lines
		start := 0
		if len(outputCopy) > 8 {
			start = len(outputCopy) - 8
		}
		
		var outputContent strings.Builder
		for i := start; i < len(outputCopy); i++ {
			line := outputCopy[i]
			// Truncate long lines
			if len(line) > 75 {
				line = line[:75] + "..."
			}
			outputContent.WriteString(line + "\n")
		}
		
		content.WriteString(outputBox.Render(outputContent.String()))
		content.WriteString("\n")
	}
	
	// Show processed resources count
	if len(m.processedItems) > 0 {
		content.WriteString(fmt.Sprintf("%s %d resources processed\n", 
			phaseStyle.Render("Progress:"), 
			len(m.processedItems)))
		content.WriteString("\n")
	}
	
	// Elapsed time
	content.WriteString(timeStyle.Render(fmt.Sprintf("Elapsed: %s", elapsed)))
	content.WriteString("\n\n")
	
	// Instructions
	if m.phase == phaseComplete {
		content.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("82")).
			Bold(true).
			Render("✅ Plan completed! Press Enter to continue..."))
	} else {
		content.WriteString(timeStyle.Render("Press q or Ctrl+C to cancel"))
	}
	
	// Center the content
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		content.String(),
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
	
	// Center the error display
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
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
	width := 50
	filled := int(m.progress * float64(width))
	
	progressStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	
	bar := progressStyle.Render(strings.Repeat("█", filled))
	bar += emptyStyle.Render(strings.Repeat("░", width-filled))
	
	percentage := fmt.Sprintf(" %.0f%%", m.progress*100)
	
	return bar + percentage
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
		if finalModel.phase == phaseError {
			return fmt.Errorf("terraform plan failed: %s", finalModel.errorDetails)
		}
	}
	
	return nil
}