package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type destroyPhase int

const (
	destroyInitializing destroyPhase = iota
	destroyPlanning
	destroyDestroying
	destroyComplete
	destroyError
)

type destroyProgressModel struct {
	phase              destroyPhase
	currentResource    string
	destroyedItems     map[string]bool // Use map to track unique destroyed resources
	destroyedCount     int             // Actual count of destroyed resources
	outputLines        []string
	outputMutex        sync.Mutex
	errorDetails       string
	errorOutput        []string
	width              int
	height             int
	startTime          time.Time
	env                string
	spinnerFrame       int
	terraformCmd       *exec.Cmd // Store command reference for cleanup
	interrupted        bool      // Track if user interrupted
	errorViewport      viewport.Model
	showFullError      bool // Track if showing full error view
	copiedToClip       bool // Track if error was copied
	lastProcessedLines int  // Track how many lines we've processed
}

// wrapText wraps a long text string to fit within maxWidth
// Returns a slice of wrapped lines
func wrapText(text string, maxWidth int) []string {
	if len(text) <= maxWidth {
		return []string{text}
	}

	var lines []string
	for len(text) > maxWidth {
		// Find last space before maxWidth
		breakPoint := maxWidth
		for i := maxWidth - 1; i >= 0; i-- {
			if text[i] == ' ' {
				breakPoint = i
				break
			}
		}

		// If no space found, force break at maxWidth
		if breakPoint == maxWidth {
			for i := maxWidth - 1; i >= 0; i-- {
				if i < len(text) {
					breakPoint = i
					break
				}
			}
		}

		lines = append(lines, text[:breakPoint])
		text = strings.TrimSpace(text[breakPoint:])
	}

	// Add remaining text
	if len(text) > 0 {
		lines = append(lines, text)
	}

	return lines
}

// Simple emoji for destruction - no complex animation
func getDestructionEmoji(phase destroyPhase) string {
	switch phase {
	case destroyInitializing:
		return "🔧"
	case destroyPlanning:
		return "📋"
	case destroyDestroying:
		return "💥"
	case destroyComplete:
		return "✅"
	case destroyError:
		return "❌"
	default:
		return "💥"
	}
}

type destroyTickMsg time.Time
type destroyCompleteMsg struct{}
type destroyErrorMsg struct {
	err    error
	output []string
}

func newDestroyProgressModel(env string) *destroyProgressModel {
	return &destroyProgressModel{
		phase:              destroyInitializing,
		destroyedItems:     make(map[string]bool),
		destroyedCount:     0,
		outputLines:        []string{},
		startTime:          time.Now(),
		env:                env,
		lastProcessedLines: 0,
	}
}

func (m *destroyProgressModel) Init() tea.Cmd {
	return tea.Batch(
		m.runTerraformDestroy(),
		m.tickCmd(),
	)
}

func (m *destroyProgressModel) tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return destroyTickMsg(t)
	})
}

func (m *destroyProgressModel) runTerraformDestroy() tea.Cmd {
	return func() tea.Msg {
		// Change to environment directory
		wd, err := os.Getwd()
		if err != nil {
			return destroyErrorMsg{err: err, output: []string{err.Error()}}
		}
		defer os.Chdir(wd)

		// Ensure lambda bootstrap file exists before running terraform
		// This prevents errors when the archive_file data source tries to archive a missing file
		ensureLambdaBootstrapExists()

		envPath := filepath.Join("env", m.env)
		if err := os.Chdir(envPath); err != nil {
			return destroyErrorMsg{err: err, output: []string{err.Error()}}
		}

		// Initialize terraform first
		initCmd := exec.Command("terraform", "init", "-reconfigure")
		initOut, _ := initCmd.CombinedOutput()
		m.outputMutex.Lock()
		m.outputLines = append(m.outputLines, strings.Split(string(initOut), "\n")...)
		m.outputMutex.Unlock()

		// Run terraform destroy with no color output for easier parsing
		cmd := exec.Command("terraform", "destroy", "-auto-approve", "-no-color")

		// Store command reference for interrupt handling
		m.terraformCmd = cmd

		// Create pipes for stdout and stderr
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return destroyErrorMsg{err: err, output: []string{err.Error()}}
		}

		stderr, err := cmd.StderrPipe()
		if err != nil {
			return destroyErrorMsg{err: err, output: []string{err.Error()}}
		}

		// Start the command
		if err := cmd.Start(); err != nil {
			return destroyErrorMsg{err: err, output: []string{err.Error()}}
		}

		// Process stdout in a goroutine
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				line := scanner.Text()
				m.outputMutex.Lock()
				m.outputLines = append(m.outputLines, line)
				// Keep only last 100 lines
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
				m.outputLines = append(m.outputLines, "ERROR: "+line)
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

			// Check if it's actually successful (no resources to destroy)
			if len(errorLines) == 0 || strings.Contains(strings.Join(errorLines, " "), "No changes") {
				return destroyCompleteMsg{}
			}

			if len(errorLines) == 0 {
				m.outputMutex.Lock()
				errorLines = make([]string, len(m.outputLines))
				copy(errorLines, m.outputLines)
				m.outputMutex.Unlock()
			}
			return destroyErrorMsg{err: err, output: errorLines}
		}

		return destroyCompleteMsg{}
	}
}

func (m *destroyProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case destroyTickMsg:
		m.spinnerFrame++

		// Process only NEW output lines to determine phase and current resource
		m.outputMutex.Lock()
		allLines := m.outputLines
		totalLines := len(allLines)

		// Get only the new lines since last tick
		var newLines []string
		if totalLines > m.lastProcessedLines {
			newLines = allLines[m.lastProcessedLines:]
			m.lastProcessedLines = totalLines
		}
		m.outputMutex.Unlock()

		// If we have output and still initializing, move to planning
		if totalLines > 0 && m.phase == destroyInitializing {
			m.phase = destroyPlanning
		}

		// Process only new lines
		for _, line := range newLines {
			if strings.Contains(line, "Initializing") {
				if m.phase != destroyPlanning && m.phase != destroyDestroying {
					m.phase = destroyInitializing
					m.currentResource = "Initializing Terraform..."
				}
			} else if strings.Contains(line, "Destroy complete!") {
				m.phase = destroyComplete
			} else if strings.Contains(line, "Destroying...") {
				m.phase = destroyDestroying
				// Extract resource name from "Destroying... [resource_name]"
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					resourceName := parts[1]
					m.currentResource = resourceName
					// Track as in-progress (not yet destroyed)
				}
			} else if strings.Contains(line, "Destruction complete after") || strings.Contains(line, "Destroyed:") {
				// This is the actual completion message - count it
				parts := strings.Fields(line)
				if len(parts) > 0 {
					// Extract resource name (usually second field)
					var resourceName string
					if strings.Contains(line, "Destruction complete") {
						// Format: "resource_type.resource_name: Destruction complete after 1s"
						resourceName = strings.Split(parts[0], ":")[0]
					} else if strings.Contains(line, "Destroyed:") {
						// Format: "Destroyed: resource_type.resource_name"
						if len(parts) >= 2 {
							resourceName = parts[1]
						}
					}

					// Add to destroyed items map (automatically deduplicates)
					if resourceName != "" && !m.destroyedItems[resourceName] {
						m.destroyedItems[resourceName] = true
						m.destroyedCount++
					}
				}
			} else if strings.Contains(line, "Refreshing state") || strings.Contains(line, "Reading...") {
				m.phase = destroyPlanning
				m.currentResource = "Planning destruction..."
			} else if strings.Contains(line, "No changes") || strings.Contains(line, "0 destroyed") {
				m.phase = destroyComplete
				m.currentResource = "No resources to destroy"
			} else if strings.Contains(line, "Terraform will perform") || strings.Contains(line, "Plan:") {
				// Detected plan output - about to start destroying
				m.phase = destroyDestroying
			}
		}

		// Continue ticking if not complete
		if m.phase != destroyComplete && m.phase != destroyError {
			return m, m.tickCmd()
		}
		return m, nil

	case destroyCompleteMsg:
		m.phase = destroyComplete
		return m, nil

	case destroyErrorMsg:
		m.phase = destroyError
		m.errorDetails = msg.err.Error()
		if len(msg.output) > 0 {
			m.errorOutput = msg.output
		}
		// Stop the animation and wait for user to press a key
		return m, nil

	case tea.KeyMsg:
		// Handle Ctrl+C during ANY phase
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			// Kill terraform process if running
			if m.terraformCmd != nil && m.terraformCmd.Process != nil {
				m.terraformCmd.Process.Kill()
			}
			m.interrupted = true
			m.phase = destroyError
			m.errorDetails = "Operation interrupted by user"
			return m, tea.Quit
		}

		// Special handling when in error or complete state
		if m.phase == destroyComplete || m.phase == destroyError {
			// Toggle full error view with 'e' or 'f' key (only in error state)
			if m.phase == destroyError && (msg.String() == "e" || msg.String() == "f") {
				m.showFullError = !m.showFullError
				if m.showFullError {
					// Initialize viewport with full error content
					m.errorViewport = viewport.New(m.width-8, m.height-10)
					fullError := strings.Join(m.errorOutput, "\n")
					if fullError == "" {
						fullError = m.errorDetails
					}
					m.errorViewport.SetContent(fullError)
				}
				return m, nil
			}

			// Copy error to clipboard with 'c' key (only in error state)
			if m.phase == destroyError && msg.String() == "c" {
				fullError := strings.Join(m.errorOutput, "\n")
				if fullError == "" {
					fullError = m.errorDetails
				}
				err := clipboard.WriteAll(fullError)
				if err == nil {
					m.copiedToClip = true
					// Reset the copied flag after 2 seconds
					return m, tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
						m.copiedToClip = false
						return nil
					})
				}
				return m, nil
			}

			// Handle viewport scrolling when showing full error
			if m.showFullError {
				switch msg.String() {
				case "up", "k":
					m.errorViewport.LineUp(1)
					return m, nil
				case "down", "j":
					m.errorViewport.LineDown(1)
					return m, nil
				case "pgup", "b":
					m.errorViewport.ViewUp()
					return m, nil
				case "pgdown", "f":
					m.errorViewport.ViewDown()
					return m, nil
				case "home", "g":
					m.errorViewport.GotoTop()
					return m, nil
				case "end", "G":
					m.errorViewport.GotoBottom()
					return m, nil
				case "esc":
					// Exit full error view
					m.showFullError = false
					return m, nil
				}
			}

			// Allow quitting with any key when not in special modes
			if !m.showFullError {
				return m, tea.Quit
			}
		}
	}

	return m, nil
}

func (m *destroyProgressModel) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	// Show full error view if enabled
	if m.showFullError {
		return m.renderFullErrorView()
	}

	// Spinner frames
	spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinner := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]

	// Get output copy
	m.outputMutex.Lock()
	outputCopy := make([]string, len(m.outputLines))
	copy(outputCopy, m.outputLines)
	m.outputMutex.Unlock()

	// Calculate content width - leave margins on sides
	contentWidth := min(m.width-8, 120)

	var content strings.Builder

	// ═══════════════════════════════════════
	// TITLE & ICON SECTION
	// ═══════════════════════════════════════
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196")).
		Align(lipgloss.Center).
		Width(contentWidth)

	titleText := "INFRASTRUCTURE DESTRUCTION IN PROGRESS"

	content.WriteString(titleStyle.Render(titleText))
	content.WriteString("\n\n")

	// Simple emoji icon (no complex animation)
	emoji := getDestructionEmoji(m.phase)
	emojiStyle := lipgloss.NewStyle().
		Align(lipgloss.Center).
		Width(contentWidth)

	content.WriteString(emojiStyle.Render(emoji))
	content.WriteString("\n\n")

	// ═══════════════════════════════════════
	// STATUS SECTION (Fixed-Height Container)
	// ═══════════════════════════════════════
	statusHeight := 3 // Reserve fixed space for status to prevent layout shifts

	var statusText string
	var statusColor string

	switch m.phase {
	case destroyInitializing:
		statusText = fmt.Sprintf("%s Initializing Terraform", spinner)
		statusColor = "220"
	case destroyPlanning:
		statusText = fmt.Sprintf("%s Planning destruction", spinner)
		statusColor = "214"
	case destroyDestroying:
		resourceName := m.currentResource
		if resourceName == "" {
			resourceName = m.env
		}
		// Truncate long resource names to prevent layout shifts
		maxResourceNameLen := contentWidth - 15 // Reserve space for spinner and "Destroying: "
		if len(resourceName) > maxResourceNameLen {
			resourceName = resourceName[:maxResourceNameLen-3] + "..."
		}
		statusText = fmt.Sprintf("%s Destroying: %s", spinner, resourceName)
		statusColor = "196"
	case destroyComplete:
		statusText = "✅ Destruction complete!"
		statusColor = "82"
	case destroyError:
		if m.interrupted {
			statusText = "⚠️  Operation cancelled by user"
			statusColor = "214"
		} else {
			statusText = "❌ Destruction failed!"
			statusColor = "196"
		}
	}

	// Use fixed-height container for status with centered text
	statusContainer := lipgloss.NewStyle().
		Height(statusHeight).
		Width(contentWidth).
		Align(lipgloss.Center, lipgloss.Top).
		Bold(true).
		Foreground(lipgloss.Color(statusColor))

	content.WriteString(statusContainer.Render(statusText))
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

	// Calculate dynamic height for output box based on available space
	// Account for all fixed-height elements:
	// - Title section: 1 line + 2 newlines = 3 lines
	// - Emoji: 1 line + 2 newlines = 3 lines
	// - Status: 3 lines + 1 newline = 4 lines
	// - Output label: 1 line + 1 newline = 2 lines
	// - Output box newline: 1 line
	// - Footer: 1 line
	// Total fixed: 14 lines
	fixedContentHeight := 14
	outputHeight := max(10, m.height-fixedContentHeight-4) // Minimum 10 lines for output, -4 for padding/borders

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
		// Note: outputHeight is the CONTENT height in lipgloss (padding/borders are added on top)
		// But we need to account for the internal padding space
		linesToShow := outputHeight - 2 // Account for top/bottom padding only
		start := 0
		if len(outputCopy) > linesToShow {
			start = len(outputCopy) - linesToShow
		}

		maxLineWidth := contentWidth - 6 // Account for left/right padding
		linesRendered := 0               // Track actual lines rendered (including wrapped)

		for i := start; i < len(outputCopy) && linesRendered < linesToShow; i++ {
			line := outputCopy[i]

			// Check if this is an error line before truncating
			isError := strings.Contains(line, "ERROR") || strings.Contains(line, "Error")

			// Word-wrap long lines instead of truncating (especially errors)
			if len(line) > maxLineWidth {
				if isError {
					// For errors, wrap to multiple lines to show full message
					wrapped := wrapText(line, maxLineWidth)
					// Only show wrapped lines if we have space
					for _, wrappedLine := range wrapped {
						if linesRendered >= linesToShow {
							break
						}
						styledLine := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(wrappedLine)
						outputContent.WriteString(styledLine + "\n")
						linesRendered++
					}
					continue
				} else {
					// For non-errors, truncate as before
					line = line[:maxLineWidth-3] + "..."
				}
			}

			// Color-code important lines
			if strings.Contains(line, "Destroying...") || strings.Contains(line, "Destruction complete") {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render(line)
			} else if isError {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(line)
			} else if strings.Contains(line, "Success") || strings.Contains(line, "complete!") {
				line = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render(line)
			}

			outputContent.WriteString(line + "\n")
			linesRendered++
		}
	}

	content.WriteString(outputBoxStyle.Render(outputContent.String()))
	content.WriteString("\n")

	// ═══════════════════════════════════════
	// FOOTER SECTION (Stats & Instructions)
	// ═══════════════════════════════════════
	var footerParts []string

	// Progress stats
	if m.destroyedCount > 0 {
		footerParts = append(footerParts,
			fmt.Sprintf("Destroyed: %d resources", m.destroyedCount))
	}

	// Elapsed time
	elapsed := time.Since(m.startTime).Seconds()
	footerParts = append(footerParts, fmt.Sprintf("Elapsed: %.1fs", elapsed))

	// Instructions
	if m.phase == destroyComplete {
		footerParts = append(footerParts, "Press any key to continue")
	} else if m.phase == destroyError {
		// Show error-specific options
		if m.copiedToClip {
			footerParts = append(footerParts, "✓ Copied!")
		} else {
			footerParts = append(footerParts, "[E] Full Error • [C] Copy • Press any key to exit")
		}
	} else {
		// Show cancel instruction during active phases
		footerParts = append(footerParts, "Press Ctrl+C or Q to cancel")
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

func (m *destroyProgressModel) renderFullErrorView() string {
	var content strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196")).
		Align(lipgloss.Center).
		Width(m.width)

	content.WriteString(titleStyle.Render("❌ FULL ERROR OUTPUT"))
	content.WriteString("\n\n")

	// Viewport with error content
	viewportStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Width(m.width - 4).
		Height(m.height - 8)

	content.WriteString(viewportStyle.Render(m.errorViewport.View()))
	content.WriteString("\n")

	// Footer with instructions
	footerParts := []string{}

	// Show scroll position
	scrollInfo := fmt.Sprintf("%3.f%%", m.errorViewport.ScrollPercent()*100)
	footerParts = append(footerParts, scrollInfo)

	if m.copiedToClip {
		footerParts = append(footerParts, "✓ Copied to clipboard!")
	} else {
		footerParts = append(footerParts, "[↑↓] Scroll • [PgUp/PgDn] Page • [C] Copy • [ESC] Back")
	}

	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Align(lipgloss.Center).
		Width(m.width)

	content.WriteString(footerStyle.Render(strings.Join(footerParts, " • ")))

	return content.String()
}

func runTerraformDestroyWithProgress(env string) error {
	p := tea.NewProgram(
		newDestroyProgressModel(env),
		tea.WithAltScreen(),
	)

	model, err := p.Run()
	if err != nil {
		return fmt.Errorf("error running destroy progress TUI: %w", err)
	}

	// Check if there was an error
	if finalModel, ok := model.(*destroyProgressModel); ok {
		if finalModel.phase == destroyError {
			errorMsg := finalModel.errorDetails
			if len(finalModel.errorOutput) > 0 {
				errorMsg = strings.Join(finalModel.errorOutput, "\n")
			}
			return fmt.Errorf("terraform destroy failed: %s", errorMsg)
		}
	}

	return nil
}
