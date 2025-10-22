package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// AI Agent TUI Model
type aiAgentModel struct {
	agent          *AIAgent
	updateChan     chan AgentUpdate
	iterations     []AgentIteration
	currentStatus  string
	isThinking     bool
	isComplete     bool
	success        bool
	viewport       viewport.Model
	ready          bool
	selectedIndex  int
	width          int
	height         int
	err            error
	autoScroll     bool // Auto-scroll to bottom when new messages arrive
}

// Messages for TUI updates
type agentUpdateMsg struct {
	update AgentUpdate
}

type agentTickMsg struct{}

// NewAIAgentTUI creates a new AI Agent TUI
func NewAIAgentTUI(agentContext *AgentContext) (*aiAgentModel, error) {
	updateChan := make(chan AgentUpdate, 10)

	agent, err := NewAIAgent(agentContext, updateChan)
	if err != nil {
		return nil, err
	}

	return &aiAgentModel{
		agent:         agent,
		updateChan:    updateChan,
		iterations:    []AgentIteration{},
		currentStatus: "Initializing AI agent...",
		isThinking:    false,
		isComplete:    false,
		viewport:      viewport.New(80, 20),
		autoScroll:    true, // Start with auto-scroll enabled
	}, nil
}

// Init implements tea.Model
func (m aiAgentModel) Init() tea.Cmd {
	return tea.Batch(
		m.waitForUpdate(),
		m.tickCmd(),
		m.startAgent(),
	)
}

// Update implements tea.Model
func (m aiAgentModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.agent != nil {
				m.agent.Stop()
			}
			return m, tea.Quit

		case "up", "k":
			// Disable auto-scroll when user manually scrolls up
			m.autoScroll = false
			if m.selectedIndex > 0 {
				m.selectedIndex--
			}

		case "down", "j":
			if m.selectedIndex < len(m.iterations)-1 {
				m.selectedIndex++
			}
			// Re-enable auto-scroll if we reached the bottom
			if m.selectedIndex == len(m.iterations)-1 && m.viewport.AtBottom() {
				m.autoScroll = true
			}

		case "enter":
			// Show detail view for selected iteration
			if m.selectedIndex < len(m.iterations) {
				// Could expand to show full output
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-10)
			m.viewport.YPosition = 3
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 10
		}

		m.viewport.SetContent(m.renderIterations())

	case agentUpdateMsg:
		update := msg.update

		switch update.Type {
		case "thinking":
			m.isThinking = true
			m.currentStatus = update.Message

		case "action_start":
			m.isThinking = false
			if update.Iteration != nil {
				// Update or add iteration
				m.updateIteration(update.Iteration)
				m.currentStatus = update.Message
			}

		case "action_complete":
			if update.Iteration != nil {
				m.updateIteration(update.Iteration)
				m.currentStatus = update.Message
			}

		case "finished":
			m.isComplete = true
			m.success = update.Success
			m.currentStatus = update.Message

		case "error":
			m.isComplete = true
			m.success = false
			m.currentStatus = update.Message
			if update.Iteration != nil {
				m.updateIteration(update.Iteration)
			}
		}

		m.viewport.SetContent(m.renderIterations())

		// Auto-scroll to bottom if enabled and new content arrived
		if m.autoScroll {
			m.viewport.GotoBottom()
		}

		cmds = append(cmds, m.waitForUpdate())

	case agentTickMsg:
		// Periodic refresh for animations
		if !m.isComplete {
			cmds = append(cmds, m.tickCmd())
		}
	}

	// Update viewport
	var cmd tea.Cmd
	oldYOffset := m.viewport.YOffset
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	// Detect manual scrolling and update autoScroll state
	if m.viewport.YOffset != oldYOffset {
		// User manually scrolled - check if they're at the bottom
		m.autoScroll = m.viewport.AtBottom()
	}

	return m, tea.Batch(cmds...)
}

// View implements tea.Model
func (m aiAgentModel) View() string {
	if !m.ready {
		return "Initializing AI Agent..."
	}

	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		Background(lipgloss.Color("235")).
		Padding(0, 1)

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Italic(true)

	// Header
	header := titleStyle.Render("🤖 AI Agent - Autonomous Infrastructure Troubleshooter")
	header += "\n"

	// Status line
	statusLine := m.currentStatus
	if m.isThinking {
		statusLine = "💭 " + statusLine
	} else if m.isComplete {
		if m.success {
			statusLine = "✅ " + statusLine
		} else {
			statusLine = "❌ " + statusLine
		}
	} else {
		statusLine = "🔧 " + statusLine
	}
	header += statusStyle.Render(statusLine) + "\n"

	// Progress summary
	header += m.renderSummary() + "\n"

	// Main content (viewport with iterations)
	content := m.viewport.View()

	// Footer
	footer := m.renderFooter()

	return header + content + "\n" + footer
}

// renderSummary shows overall progress
func (m aiAgentModel) renderSummary() string {
	totalIterations := len(m.iterations)
	successCount := 0
	failedCount := 0

	for _, iter := range m.iterations {
		if iter.Status == "success" {
			successCount++
		} else if iter.Status == "failed" {
			failedCount++
		}
	}

	summaryStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Padding(0, 1)

	return summaryStyle.Render(fmt.Sprintf(
		"Iterations: %d | Success: %d | Failed: %d",
		totalIterations, successCount, failedCount,
	))
}

// renderIterations renders all iterations in a scrollable list
func (m aiAgentModel) renderIterations() string {
	if len(m.iterations) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Italic(true)
		return emptyStyle.Render("Waiting for agent to start...")
	}

	var content strings.Builder

	for i, iter := range m.iterations {
		isSelected := i == m.selectedIndex
		content.WriteString(m.renderIteration(iter, isSelected))
		content.WriteString("\n")
	}

	// Add completion summary at the end if agent is done
	if m.isComplete {
		content.WriteString("\n" + m.renderCompletionSummary())
	}

	return content.String()
}

// renderIteration renders a single iteration
func (m aiAgentModel) renderIteration(iter AgentIteration, selected bool) string {
	// Status icon
	var statusIcon string
	var statusColor lipgloss.Color
	switch iter.Status {
	case "running":
		statusIcon = "→"
		statusColor = lipgloss.Color("226")
	case "success":
		statusIcon = "✓"
		statusColor = lipgloss.Color("82")
	case "failed":
		statusIcon = "✗"
		statusColor = lipgloss.Color("196")
	default:
		statusIcon = "•"
		statusColor = lipgloss.Color("243")
	}

	// Base style
	baseStyle := lipgloss.NewStyle()
	if selected {
		baseStyle = baseStyle.Background(lipgloss.Color("237"))
	}

	// Render iteration header
	headerStyle := baseStyle.Copy().
		Foreground(statusColor).
		Bold(true)

	header := headerStyle.Render(fmt.Sprintf(
		"%s %d. %s",
		statusIcon,
		iter.Number,
		truncateString(iter.Thought, 80),
	))

	// Render tool call block based on action type
	var toolBlock string
	switch iter.Action {
	case "aws_cli", "shell":
		toolBlock = "\n" + m.renderCommandBlock(iter, selected)
	case "file_edit":
		toolBlock = "\n" + m.renderFileEditBlock(iter, selected)
	case "terraform_plan", "terraform_apply":
		toolBlock = "\n" + m.renderTerraformBlock(iter, selected)
	case "web_search":
		toolBlock = "\n" + m.renderWebSearchBlock(iter, selected)
	case "complete":
		toolBlock = "\n" + m.renderCompleteBlock(iter, selected)
	default:
		// Fallback to simple rendering
		toolBlock = "\n" + m.renderSimpleBlock(iter, selected)
	}

	return header + toolBlock
}

// renderCommandBlock renders a bordered block for shell/AWS CLI commands
func (m aiAgentModel) renderCommandBlock(iter AgentIteration, selected bool) string {
	// Determine border color based on action type
	var borderColor lipgloss.Color
	var title string
	if iter.Action == "aws_cli" {
		borderColor = lipgloss.Color("214") // Orange for AWS
		title = "AWS CLI"
	} else {
		borderColor = lipgloss.Color("51") // Cyan for shell
		title = "Shell"
	}

	// Status color
	statusColor := lipgloss.Color("82") // Green
	if iter.Status == "running" {
		statusColor = lipgloss.Color("226") // Yellow
	} else if iter.Status == "failed" {
		statusColor = lipgloss.Color("196") // Red
	}

	// Create bordered box style
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		MarginLeft(4).
		Width(100)

	// Build content
	var content strings.Builder
	content.WriteString(lipgloss.NewStyle().Foreground(borderColor).Bold(true).Render(title + " details") + "\n\n")

	// Status
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("Status: "))
	content.WriteString(lipgloss.NewStyle().Foreground(statusColor).Render(iter.Status) + "\n")

	// Runtime/Duration
	if iter.Duration > 0 {
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("Runtime: "))
		content.WriteString(fmt.Sprintf("%v\n", iter.Duration.Round(time.Millisecond)))
	}

	// Command
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("Command: "))
	content.WriteString(iter.Command + "\n")

	// Output
	if iter.Output != "" {
		content.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("Stdout:") + "\n\n")

		// Show output in a code block style
		outputLines := strings.Split(iter.Output, "\n")
		displayLines := outputLines
		if len(outputLines) > 10 {
			displayLines = outputLines[:10]
		}

		outputStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("235")).
			Padding(0, 1)

		for _, line := range displayLines {
			content.WriteString(outputStyle.Render(line) + "\n")
		}

		if len(outputLines) > 10 {
			content.WriteString("\n" + lipgloss.NewStyle().
				Foreground(lipgloss.Color("243")).
				Italic(true).
				Render(fmt.Sprintf("Showing 10 lines")))
		}
	}

	// Error
	if iter.Status == "failed" && iter.ErrorDetail != "" {
		content.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render("Error:") + "\n")
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(iter.ErrorDetail))
	}

	return boxStyle.Render(content.String())
}

// renderFileEditBlock renders a block for file edit operations
func (m aiAgentModel) renderFileEditBlock(iter AgentIteration, selected bool) string {
	borderColor := lipgloss.Color("141") // Purple for file edits

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		MarginLeft(4).
		Width(100)

	var content strings.Builder
	content.WriteString(lipgloss.NewStyle().Foreground(borderColor).Bold(true).Render("File Edit") + "\n\n")

	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("Status: "))
	statusColor := lipgloss.Color("82")
	if iter.Status == "failed" {
		statusColor = lipgloss.Color("196")
	}
	content.WriteString(lipgloss.NewStyle().Foreground(statusColor).Render(iter.Status) + "\n")

	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("Operation: "))
	content.WriteString(iter.Command + "\n")

	if iter.Output != "" {
		content.WriteString("\n" + iter.Output)
	}

	if iter.Status == "failed" && iter.ErrorDetail != "" {
		content.WriteString("\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("Error: " + iter.ErrorDetail))
	}

	return boxStyle.Render(content.String())
}

// renderTerraformBlock renders a block for terraform operations
func (m aiAgentModel) renderTerraformBlock(iter AgentIteration, selected bool) string {
	borderColor := lipgloss.Color("105") // Purple/blue for terraform

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		MarginLeft(4).
		Width(100)

	title := "Terraform Plan"
	if iter.Action == "terraform_apply" {
		title = "Terraform Apply"
	}

	var content strings.Builder
	content.WriteString(lipgloss.NewStyle().Foreground(borderColor).Bold(true).Render(title) + "\n\n")

	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("Status: "))
	statusColor := lipgloss.Color("82")
	if iter.Status == "running" {
		statusColor = lipgloss.Color("226")
	} else if iter.Status == "failed" {
		statusColor = lipgloss.Color("196")
	}
	content.WriteString(lipgloss.NewStyle().Foreground(statusColor).Render(iter.Status) + "\n")

	if iter.Duration > 0 {
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("Duration: "))
		content.WriteString(fmt.Sprintf("%v\n", iter.Duration.Round(time.Millisecond)))
	}

	if iter.Output != "" {
		content.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("Output:") + "\n\n")
		outputPreview := truncateString(iter.Output, 200)
		content.WriteString(outputPreview)
	}

	return boxStyle.Render(content.String())
}

// renderWebSearchBlock renders a block for web search operations
func (m aiAgentModel) renderWebSearchBlock(iter AgentIteration, selected bool) string {
	borderColor := lipgloss.Color("39") // Blue for web search

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		MarginLeft(4).
		Width(100)

	var content strings.Builder
	content.WriteString(lipgloss.NewStyle().Foreground(borderColor).Bold(true).Render("Web Search") + "\n\n")

	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("Query: "))
	content.WriteString(iter.Command + "\n")

	if iter.Output != "" {
		content.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("Results:") + "\n\n")
		outputPreview := truncateString(iter.Output, 300)
		content.WriteString(outputPreview)
	}

	return boxStyle.Render(content.String())
}

// renderCompleteBlock renders a block for completion
func (m aiAgentModel) renderCompleteBlock(iter AgentIteration, selected bool) string {
	borderColor := lipgloss.Color("82") // Green for completion

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		MarginLeft(4).
		Width(100)

	var content strings.Builder
	content.WriteString(lipgloss.NewStyle().Foreground(borderColor).Bold(true).Render("✓ Problem Solved") + "\n\n")
	content.WriteString(iter.Command)

	return boxStyle.Render(content.String())
}

// renderSimpleBlock renders a simple block for unknown action types
func (m aiAgentModel) renderSimpleBlock(iter AgentIteration, selected bool) string {
	detailStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		PaddingLeft(4)

	var content strings.Builder
	content.WriteString(detailStyle.Render(fmt.Sprintf("Action: %s | Command: %s", iter.Action, truncateString(iter.Command, 60))))

	if iter.Output != "" {
		content.WriteString("\n" + detailStyle.Render(fmt.Sprintf("Output: %s", truncateString(iter.Output, 100))))
	}

	if iter.Status == "failed" && iter.ErrorDetail != "" {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			PaddingLeft(4)
		content.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Error: %s", truncateString(iter.ErrorDetail, 100))))
	}

	return content.String()
}

// renderCompletionSummary shows a summary when agent finishes
func (m aiAgentModel) renderCompletionSummary() string {
	if !m.isComplete {
		return ""
	}

	totalIterations := len(m.iterations)
	successCount := 0
	failedCount := 0
	totalDuration := time.Duration(0)

	for _, iter := range m.iterations {
		if iter.Status == "success" {
			successCount++
		} else if iter.Status == "failed" {
			failedCount++
		}
		totalDuration += iter.Duration
	}

	// Determine outcome
	var outcomeStyle lipgloss.Style
	var outcomeText string
	if m.success {
		outcomeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("82")).
			Bold(true)
		outcomeText = "✓ Problem Solved Successfully"
	} else {
		outcomeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
		outcomeText = "✗ Agent Unable to Resolve Issue"
	}

	// Create summary box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("39")).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1)

	var summary strings.Builder
	summary.WriteString(outcomeStyle.Render(outcomeText) + "\n\n")
	summary.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("Summary:") + "\n")
	summary.WriteString(fmt.Sprintf("  • Total Iterations: %d\n", totalIterations))
	summary.WriteString(fmt.Sprintf("  • Successful Actions: %d\n", successCount))
	summary.WriteString(fmt.Sprintf("  • Failed Actions: %d\n", failedCount))
	summary.WriteString(fmt.Sprintf("  • Total Duration: %v\n", totalDuration.Round(time.Second)))

	if m.currentStatus != "" {
		summary.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(m.currentStatus))
	}

	return boxStyle.Render(summary.String())
}

// renderFooter shows help text
func (m aiAgentModel) renderFooter() string {
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Background(lipgloss.Color("235")).
		Padding(0, 1)

	var scrollIndicator string
	if !m.autoScroll {
		scrollIndicator = " [Auto-scroll: OFF]"
	}

	if m.isComplete {
		return footerStyle.Render("[q] Quit and return to menu" + scrollIndicator)
	}

	return footerStyle.Render("[↑/↓] Navigate | [Enter] Details | [q] Stop agent" + scrollIndicator)
}

// Helper methods

func (m *aiAgentModel) updateIteration(iter *AgentIteration) {
	// Find existing iteration by number or add new one
	found := false
	for i := range m.iterations {
		if m.iterations[i].Number == iter.Number {
			m.iterations[i] = *iter
			found = true
			break
		}
	}

	if !found {
		m.iterations = append(m.iterations, *iter)
		m.selectedIndex = len(m.iterations) - 1 // Auto-select newest
	}
}

func (m aiAgentModel) waitForUpdate() tea.Cmd {
	return func() tea.Msg {
		update := <-m.updateChan
		return agentUpdateMsg{update: update}
	}
}

func (m aiAgentModel) tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*500, func(t time.Time) tea.Msg {
		return agentTickMsg{}
	})
}

func (m aiAgentModel) startAgent() tea.Cmd {
	return func() tea.Msg {
		// Run agent in background
		go func() {
			err := m.agent.Start()

			// Prepare update message
			update := AgentUpdate{
				Type: "complete",
			}
			if err != nil {
				update.Type = "error"
				update.Message = fmt.Sprintf("Agent failed: %v", err)
			}

			// SECURITY: Safely send update with timeout and context check to prevent goroutine leak
			select {
			case <-m.agent.ctx.Done():
				// TUI quit, don't send
				return
			case m.updateChan <- update:
				// Sent successfully
			case <-time.After(100 * time.Millisecond):
				// Channel blocked, abandon send to prevent goroutine leak
				return
			}
		}()
		return nil
	}
}

// Utility functions

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// RunAIAgentTUI is the entry point to run the AI agent TUI
func RunAIAgentTUI(agentContext *AgentContext) error {
	model, err := NewAIAgentTUI(agentContext)
	if err != nil {
		return fmt.Errorf("failed to create AI agent TUI: %w", err)
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}
