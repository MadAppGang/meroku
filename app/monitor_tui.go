package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// View mode enum
// ---------------------------------------------------------------------------

type monitorViewMode int

const (
	monitorOverviewView   monitorViewMode = iota // View 1: System Status
	monitorDeploymentView                        // View 2: Deployments + CI/CD
	monitorLogsView                              // View 3: Service Logs
	monitorExecView                              // View 4: ECS Exec / SSH
)

// ---------------------------------------------------------------------------
// Tea message types
// ---------------------------------------------------------------------------

type monitorDataMsg struct {
	data DashboardState
	err  error
}

type monitorLogsMsg struct {
	page LogPage
}

type monitorWorkflowsMsg struct {
	runs []WorkflowRun
	err  error
}

type monitorTickMsg time.Time

type monitorExecDoneMsg struct {
	err error
}

// ---------------------------------------------------------------------------
// Key map
// ---------------------------------------------------------------------------

type monitorKeyMap struct {
	Tab      key.Binding
	ShiftTab key.Binding
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	Enter    key.Binding
	Refresh  key.Binding
	Filter   key.Binding
	Search   key.Binding
	Escape   key.Binding
	LoadMore key.Binding
	Help     key.Binding
	Quit     key.Binding
	View1    key.Binding
	View2    key.Binding
	View3    key.Binding
	View4    key.Binding
}

func defaultMonitorKeyMap() monitorKeyMap {
	return monitorKeyMap{
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("Tab", "next view"),
		),
		ShiftTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("Shift+Tab", "prev view"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "left panel"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "right panel"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("Enter", "select"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Filter: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "cycle filter"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("Esc", "cancel"),
		),
		LoadMore: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "load more"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		View1: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "overview"),
		),
		View2: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "deployments"),
		),
		View3: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "logs"),
		),
		View4: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", "exec"),
		),
	}
}

// ---------------------------------------------------------------------------
// Color palette and styles
// ---------------------------------------------------------------------------

var (
	monBgColor      = lipgloss.Color("#0a0a0a")
	monFgColor      = lipgloss.Color("#ffffff")
	monBorderColor  = lipgloss.Color("#333333")
	monPrimaryColor = lipgloss.Color("#7c3aed")
	monSuccessColor = lipgloss.Color("#10b981")
	monWarningColor = lipgloss.Color("#f59e0b")
	monDangerColor  = lipgloss.Color("#ef4444")
	monMutedColor   = lipgloss.Color("#6b7280")
	monAccentColor  = lipgloss.Color("#3b82f6")
	monDimColor     = lipgloss.Color("#9ca3af")

	// Monitor-specific
	monActivePanel = lipgloss.Color("#7c3aed")
	monHeaderBg    = lipgloss.Color("#1a1a1a")

	// Status colors
	monStatusOK    = monSuccessColor
	monStatusWarn  = monWarningColor
	monStatusError = monDangerColor
	monStatusMuted = monMutedColor

	// Shared styles
	monPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(monBorderColor).
			Padding(0, 1)

	monActivePanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(monActivePanel).
				Padding(0, 1)

	monHeaderStyle = lipgloss.NewStyle().
			Background(monHeaderBg).
			Foreground(monFgColor).
			Bold(true).
			Padding(0, 2)

	monStatusBarStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#111111")).
				Foreground(monMutedColor).
				Padding(0, 1)

	monLogErrorStyle = lipgloss.NewStyle().Foreground(monDangerColor)
	monLogWarnStyle  = lipgloss.NewStyle().Foreground(monWarningColor)
	monLogInfoStyle  = lipgloss.NewStyle().Foreground(monFgColor)
	monLogTimeStyle  = lipgloss.NewStyle().Foreground(monDimColor)
	monLogDebugStyle = lipgloss.NewStyle().Foreground(monMutedColor)

	monLabelStyle    = lipgloss.NewStyle().Foreground(monMutedColor)
	monSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#374151")).
				Foreground(lipgloss.Color("#ffffff")).
				Bold(true)
	monTitleStyle = lipgloss.NewStyle().
			Foreground(monPrimaryColor).
			Bold(true)
	monSuccessStyle = lipgloss.NewStyle().Foreground(monSuccessColor)
	monWarnStyle    = lipgloss.NewStyle().Foreground(monWarningColor)
	monDangerStyle  = lipgloss.NewStyle().Foreground(monDangerColor)
	monMutedStyle   = lipgloss.NewStyle().Foreground(monMutedColor)
	monAccentStyle  = lipgloss.NewStyle().Foreground(monAccentColor)
)

// monStatusStyle returns a styled status string based on the status value.
func monStatusStyle(status string) string {
	upper := strings.ToUpper(status)
	switch upper {
	case "ACTIVE", "RUNNING", "AVAILABLE", "ENABLED", "SUCCESS":
		return monSuccessStyle.Render(upper)
	case "PENDING", "IN_PROGRESS", "STARTING", "PROVISIONING":
		return monWarnStyle.Render(upper)
	case "DRAINING", "FAILED", "FAILURE", "ERROR", "STOPPED", "DISABLED":
		return monDangerStyle.Render(upper)
	default:
		return monMutedStyle.Render(upper)
	}
}

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

type monitorModel struct {
	// Config
	env     Env
	envName string

	// UI state
	width       int
	height      int
	currentView monitorViewMode
	loading     bool
	lastError   string
	showHelp    bool
	keys        monitorKeyMap

	// Data
	data         DashboardState
	workflowRuns []WorkflowRun

	// View 2: Deployments
	deploymentScroll int

	// View 3: Logs
	selectedService int
	logServiceList  []string
	logBuffer       []MonitorLogEntry
	logScrollOffset int
	logNextToken    string
	logLoading      bool
	logFilter       string // "ERROR", "WARN", "" (all)
	logSearchQuery  string
	logSearchMode   bool
	logFocusRight   bool // true when focus is on log panel

	// View 4: Exec
	execSelectedService int
	execSelectedTask    int
	execLaunching       bool
	execFocusRight      bool // true when focus is on tasks panel

	// Viewports for scrollable content
	overviewVP   viewport.Model
	deploymentVP viewport.Model
	logVP        viewport.Model
	execVP       viewport.Model
}

// ---------------------------------------------------------------------------
// tea.Model interface
// ---------------------------------------------------------------------------

func (m monitorModel) Init() tea.Cmd {
	return tea.Batch(
		fetchDashboardDataCmd(m.env, m.envName),
		tickCmd(),
	)
}

func (m monitorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateViewports()
		return m, nil

	case monitorTickMsg:
		// Auto-refresh every 30 seconds
		return m, tea.Batch(
			fetchDashboardDataCmd(m.env, m.envName),
			tickCmd(),
		)

	case monitorDataMsg:
		m.loading = false
		if msg.err != nil {
			m.lastError = msg.err.Error()
		} else {
			m.data = msg.data
			m.lastError = ""
			// Rebuild log service list inline (value receiver can't mutate m)
			var list []string
			for _, svc := range m.data.Services {
				list = append(list, svc.Name)
			}
			for _, s := range m.data.Schedules {
				list = append(list, s.Name)
			}
			m.logServiceList = list
			if m.selectedService >= len(list) {
				m.selectedService = 0
			}
		}
		// Also fetch GitHub workflows (fire-and-forget)
		return m, fetchWorkflowsCmd(m.env)

	case monitorWorkflowsMsg:
		if msg.err == nil && msg.runs != nil {
			m.workflowRuns = msg.runs
		}
		return m, nil

	case monitorLogsMsg:
		m.logLoading = false
		if msg.page.Error == nil {
			m.logBuffer = append(m.logBuffer, msg.page.Entries...)
			m.logNextToken = msg.page.NextToken
		} else {
			m.lastError = fmt.Sprintf("Logs: %v", msg.page.Error)
		}
		return m, nil

	case monitorExecDoneMsg:
		m.execLaunching = false
		if msg.err != nil {
			m.lastError = fmt.Sprintf("ECS Exec failed: %v", msg.err)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}

	return m, nil
}

func (m monitorModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Search mode captures all input
	if m.logSearchMode {
		switch {
		case key.Matches(msg, m.keys.Escape):
			m.logSearchMode = false
			m.logSearchQuery = ""
		case msg.Type == tea.KeyBackspace:
			if len(m.logSearchQuery) > 0 {
				m.logSearchQuery = m.logSearchQuery[:len(m.logSearchQuery)-1]
			}
		case msg.Type == tea.KeyEnter:
			m.logSearchMode = false
		default:
			if msg.Type == tea.KeyRunes {
				m.logSearchQuery += string(msg.Runes)
			}
		}
		return m, nil
	}

	// Global bindings
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, m.keys.Help):
		m.showHelp = !m.showHelp
		return m, nil

	case key.Matches(msg, m.keys.Refresh):
		m.loading = true
		return m, fetchDashboardDataCmd(m.env, m.envName)

	case key.Matches(msg, m.keys.View1):
		m.currentView = monitorOverviewView
		return m, nil

	case key.Matches(msg, m.keys.View2):
		m.currentView = monitorDeploymentView
		return m, nil

	case key.Matches(msg, m.keys.View3):
		m.currentView = monitorLogsView
		return m, nil

	case key.Matches(msg, m.keys.View4):
		m.currentView = monitorExecView
		return m, nil

	case key.Matches(msg, m.keys.Tab):
		m.currentView = (m.currentView + 1) % 4
		return m, nil

	case key.Matches(msg, m.keys.ShiftTab):
		m.currentView = (m.currentView + 3) % 4
		return m, nil
	}

	// Per-view bindings
	switch m.currentView {
	case monitorLogsView:
		return m.handleLogsKey(msg)
	case monitorExecView:
		return m.handleExecKey(msg)
	}

	return m, nil
}

func (m monitorModel) handleLogsKey(msg tea.KeyMsg) (monitorModel, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Search):
		m.logSearchMode = true
		return m, nil

	case key.Matches(msg, m.keys.Filter):
		switch m.logFilter {
		case "":
			m.logFilter = "ERROR"
		case "ERROR":
			m.logFilter = "WARN"
		case "WARN":
			m.logFilter = ""
		}
		return m, nil

	case key.Matches(msg, m.keys.LoadMore):
		if m.logNextToken != "" && !m.logLoading && len(m.logServiceList) > 0 {
			m.logLoading = true
			svcName := ""
			if m.selectedService < len(m.logServiceList) {
				svcName = m.logServiceList[m.selectedService]
			}
			return m, fetchLogsCmd(m.env, svcName, m.logNextToken)
		}
		return m, nil

	case key.Matches(msg, m.keys.Left):
		m.logFocusRight = false
		return m, nil

	case key.Matches(msg, m.keys.Right):
		m.logFocusRight = true
		return m, nil

	case key.Matches(msg, m.keys.Up):
		if !m.logFocusRight {
			if m.selectedService > 0 {
				m.selectedService--
			}
		} else {
			if m.logScrollOffset > 0 {
				m.logScrollOffset--
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.Down):
		if !m.logFocusRight {
			if m.selectedService < len(m.logServiceList)-1 {
				m.selectedService++
			}
		} else {
			filtered := m.filteredLogs()
			maxScroll := len(filtered) - 1
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.logScrollOffset < maxScroll {
				m.logScrollOffset++
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.Enter):
		// Load logs for selected service
		if len(m.logServiceList) > 0 && m.selectedService < len(m.logServiceList) {
			m.logBuffer = nil
			m.logScrollOffset = 0
			m.logNextToken = ""
			m.logLoading = true
			svcName := m.logServiceList[m.selectedService]
			return m, fetchLogsCmd(m.env, svcName, "")
		}
		return m, nil

	case key.Matches(msg, m.keys.Escape):
		m.logSearchQuery = ""
		m.logSearchMode = false
		return m, nil
	}
	return m, nil
}

func (m monitorModel) handleExecKey(msg tea.KeyMsg) (monitorModel, tea.Cmd) {
	services := m.execServiceList()

	switch {
	case key.Matches(msg, m.keys.Left):
		m.execFocusRight = false
		return m, nil

	case key.Matches(msg, m.keys.Right):
		if len(services) > 0 {
			m.execFocusRight = true
		}
		return m, nil

	case key.Matches(msg, m.keys.Up):
		if !m.execFocusRight {
			if m.execSelectedService > 0 {
				m.execSelectedService--
				m.execSelectedTask = 0
			}
		} else {
			if m.execSelectedTask > 0 {
				m.execSelectedTask--
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.Down):
		if !m.execFocusRight {
			if m.execSelectedService < len(services)-1 {
				m.execSelectedService++
				m.execSelectedTask = 0
			}
		} else {
			if m.execSelectedService < len(services) {
				svc := services[m.execSelectedService]
				if m.execSelectedTask < len(svc.Tasks)-1 {
					m.execSelectedTask++
				}
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.Enter):
		if m.execFocusRight && m.execSelectedService < len(services) {
			svc := services[m.execSelectedService]
			if m.execSelectedTask < len(svc.Tasks) {
				task := svc.Tasks[m.execSelectedTask]
				return m.execConnect(task)
			}
		} else if !m.execFocusRight {
			// Move focus to right
			m.execFocusRight = true
		}
		return m, nil
	}
	return m, nil
}

// execConnect launches ECS Exec for the given task.
func (m monitorModel) execConnect(task MonitorTask) (monitorModel, tea.Cmd) {
	clusterName := fmt.Sprintf("%s_cluster_%s", m.env.Project, m.env.Env)
	region := m.env.Region
	profile := selectedAWSProfile

	args := []string{
		"ecs", "execute-command",
		"--cluster", clusterName,
		"--task", task.TaskArn,
		"--container", task.ContainerName,
		"--command", "/bin/sh",
		"--interactive",
	}
	if region != "" {
		args = append(args, "--region", region)
	}
	if profile != "" {
		args = append(args, "--profile", profile)
	}

	cmd := exec.Command("aws", args...)
	m.execLaunching = true
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return monitorExecDoneMsg{err: err}
	})
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (m monitorModel) View() string {
	if m.width == 0 {
		return "Loading monitor dashboard..."
	}

	header := m.renderHeader()
	statusBar := m.renderStatusBar()

	// Available height for body
	headerH := lipgloss.Height(header)
	statusH := lipgloss.Height(statusBar)
	bodyH := m.height - headerH - statusH
	if bodyH < 0 {
		bodyH = 0
	}

	var body string
	switch m.currentView {
	case monitorOverviewView:
		body = m.viewOverview(bodyH)
	case monitorDeploymentView:
		body = m.viewDeployments(bodyH)
	case monitorLogsView:
		body = m.viewLogs(bodyH)
	case monitorExecView:
		body = m.viewExec(bodyH)
	}

	if m.showHelp {
		body = m.renderHelpOverlay(body)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		body,
		statusBar,
	)
}

// ---------------------------------------------------------------------------
// Header
// ---------------------------------------------------------------------------

func (m monitorModel) renderHeader() string {
	project := m.env.Project
	if project == "" {
		project = m.envName
	}
	envStr := m.env.Env
	if envStr == "" {
		envStr = m.envName
	}
	region := m.env.Region
	if region == "" {
		region = "unknown"
	}
	account := m.env.AccountID
	if account == "" {
		account = "unknown"
	}

	left := fmt.Sprintf("meroku monitor  %s / %s  %s  %s", project, envStr, region, account)
	if m.loading {
		left += "  [Refreshing...]"
	}

	tabs := m.renderViewTabs()

	// Pad left to fill width minus tabs
	tabsW := lipgloss.Width(tabs)
	leftW := m.width - tabsW - 4
	if leftW < 0 {
		leftW = 0
	}
	leftStyled := monHeaderStyle.Width(leftW).Render(left)
	tabsStyled := monHeaderStyle.Width(tabsW).Render(tabs)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftStyled, tabsStyled)
}

func (m monitorModel) renderViewTabs() string {
	views := []struct {
		mode  monitorViewMode
		label string
	}{
		{monitorOverviewView, "[1 Overview]"},
		{monitorDeploymentView, "[2 Deploy]"},
		{monitorLogsView, "[3 Logs]"},
		{monitorExecView, "[4 Exec]"},
	}

	var parts []string
	for _, v := range views {
		if m.currentView == v.mode {
			parts = append(parts, lipgloss.NewStyle().Foreground(monPrimaryColor).Bold(true).Render(v.label))
		} else {
			parts = append(parts, monMutedStyle.Render(v.label))
		}
	}
	return strings.Join(parts, " ")
}

// ---------------------------------------------------------------------------
// Status bar
// ---------------------------------------------------------------------------

func (m monitorModel) renderStatusBar() string {
	lastUpdate := "Never"
	if !m.data.LoadedAt.IsZero() {
		lastUpdate = m.data.LoadedAt.Format("15:04:05")
	}

	left := fmt.Sprintf("Last updated: %s  Auto-refresh: 30s", lastUpdate)
	if m.lastError != "" {
		errShort := m.lastError
		if len(errShort) > 60 {
			errShort = errShort[:60] + "..."
		}
		left += "  ERR: " + errShort
	}

	right := "[Tab] view  [r] refresh  [?] help  [q] quit"

	// Pad to fill width
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	separator := strings.Repeat(" ", gap)

	return monStatusBarStyle.Width(m.width).Render(left + separator + right)
}

// ---------------------------------------------------------------------------
// View 1: System Status Overview
// ---------------------------------------------------------------------------

func (m monitorModel) viewOverview(availH int) string {
	leftW := m.width * 6 / 10
	rightW := m.width - leftW - 1
	if rightW < 20 {
		rightW = 20
	}

	// Left column
	clusterPanel := m.renderClusterPanel(leftW)
	servicesPanel := m.renderServicesPanel(leftW)
	tasksPanel := m.renderTasksPanel(leftW)
	leftCol := lipgloss.JoinVertical(lipgloss.Left, clusterPanel, servicesPanel, tasksPanel)

	// Right column
	dbPanel := m.renderDatabasePanel(rightW)
	schedPanel := m.renderSchedulesPanel(rightW)
	rightCol := lipgloss.JoinVertical(lipgloss.Left, dbPanel, schedPanel)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, " ", rightCol)
}

func (m monitorModel) renderClusterPanel(w int) string {
	c := m.data.Cluster
	name := c.Name
	if name == "" {
		name = fmt.Sprintf("%s_cluster_%s", m.env.Project, m.env.Env)
	}

	title := monTitleStyle.Render("ECS Cluster: " + name)
	statusLine := fmt.Sprintf("Status: %s   Running Tasks: %d   Active Services: %d",
		monStatusStyle(c.Status), c.RunningTasks, c.ActiveServices)

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		statusLine,
	)
	innerW := w - 4
	if innerW < 1 {
		innerW = 1
	}
	return monPanelStyle.Width(innerW).Render(content)
}

func (m monitorModel) renderServicesPanel(w int) string {
	innerW := w - 4
	if innerW < 1 {
		innerW = 1
	}

	title := monTitleStyle.Render("ECS Services")

	if len(m.data.Services) == 0 {
		content := lipgloss.JoinVertical(lipgloss.Left, title, monMutedStyle.Render("  No services found"))
		return monPanelStyle.Width(innerW).Render(content)
	}

	// Header row
	header := lipgloss.NewStyle().Foreground(monAccentColor).Render(
		fmt.Sprintf("%-20s %-8s %8s %8s %8s", "SERVICE", "STATUS", "RUN/DES", "CPU", "MEM"),
	)

	var rows []string
	rows = append(rows, title, header)

	for _, svc := range m.data.Services {
		cpuStr := "N/A"
		memStr := "N/A"
		if svc.CPUPercent >= 0 {
			cpuStr = fmt.Sprintf("%.1f%%", svc.CPUPercent)
		}
		if svc.MemPercent >= 0 {
			memStr = fmt.Sprintf("%.1f%%", svc.MemPercent)
		}
		counts := fmt.Sprintf("%d/%d", svc.RunningCount, svc.DesiredCount)
		row := fmt.Sprintf("%-20s %-8s %8s %8s %8s",
			truncate(svc.Name, 20),
			truncate(string(svc.Status), 8),
			counts,
			cpuStr,
			memStr,
		)
		// Color the status portion
		statusColored := monStatusStyle(string(svc.Status))
		row = fmt.Sprintf("%-20s %s %8s %8s %8s",
			truncate(svc.Name, 20),
			padRight(statusColored, 8),
			counts,
			cpuStr,
			memStr,
		)
		rows = append(rows, row)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return monPanelStyle.Width(innerW).Render(content)
}

func (m monitorModel) renderTasksPanel(w int) string {
	innerW := w - 4
	if innerW < 1 {
		innerW = 1
	}

	title := monTitleStyle.Render("Running Tasks")

	var allTasks []struct {
		task    MonitorTask
		svcName string
	}
	for _, svc := range m.data.Services {
		for _, t := range svc.Tasks {
			allTasks = append(allTasks, struct {
				task    MonitorTask
				svcName string
			}{t, svc.Name})
		}
	}

	if len(allTasks) == 0 {
		content := lipgloss.JoinVertical(lipgloss.Left, title, monMutedStyle.Render("  No running tasks"))
		return monPanelStyle.Width(innerW).Render(content)
	}

	header := lipgloss.NewStyle().Foreground(monAccentColor).Render(
		fmt.Sprintf("%-14s %-12s %-14s %s", "TASK ID", "SERVICE", "AZ", "UPTIME"),
	)

	var rows []string
	rows = append(rows, title, header)
	for _, item := range allTasks {
		t := item.task
		az := t.AZ
		if len(az) > 14 {
			az = az[len(az)-14:]
		}
		row := fmt.Sprintf("%-14s %-12s %-14s %s",
			truncate(t.TaskID, 14),
			truncate(item.svcName, 12),
			truncate(az, 14),
			formatUptime(t.StartedAt),
		)
		rows = append(rows, row)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return monPanelStyle.Width(innerW).Render(content)
}

func (m monitorModel) renderDatabasePanel(w int) string {
	innerW := w - 4
	if innerW < 1 {
		innerW = 1
	}

	title := monTitleStyle.Render("Database")

	if m.data.Database == nil {
		msg := "No database configured"
		if m.env.Postgres.Enabled {
			msg = "Database not found in AWS"
		}
		content := lipgloss.JoinVertical(lipgloss.Left, title, monMutedStyle.Render("  "+msg))
		return monPanelStyle.Width(innerW).Render(content)
	}

	db := m.data.Database
	engineLabel := db.Engine
	if db.EngineVersion != "" {
		engineLabel = db.Engine + " " + db.EngineVersion
	}

	statusLine := fmt.Sprintf("Status: %s    Engine: %s", monStatusStyle(db.Status), engineLabel)
	endpointLine := fmt.Sprintf("Endpoint: %s", truncate(db.Endpoint, innerW-10))

	lines := []string{title, statusLine, endpointLine}

	if db.CPUPercent != nil {
		lines = append(lines, fmt.Sprintf("CPU: %.1f%%    Connections: %.0f", *db.CPUPercent,
			func() float64 {
				if db.Connections != nil {
					return *db.Connections
				}
				return 0
			}()))
	}
	if db.FreeStorageBytes != nil {
		gb := *db.FreeStorageBytes / (1024 * 1024 * 1024)
		lines = append(lines, fmt.Sprintf("Storage Free: %.1f GB", gb))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return monPanelStyle.Width(innerW).Render(content)
}

func (m monitorModel) renderSchedulesPanel(w int) string {
	innerW := w - 4
	if innerW < 1 {
		innerW = 1
	}

	title := monTitleStyle.Render("Scheduled Tasks")

	if len(m.data.Schedules) == 0 {
		msg := "No scheduled tasks configured"
		content := lipgloss.JoinVertical(lipgloss.Left, title, monMutedStyle.Render("  "+msg))
		return monPanelStyle.Width(innerW).Render(content)
	}

	header := lipgloss.NewStyle().Foreground(monAccentColor).Render(
		fmt.Sprintf("%-12s %-20s %-10s", "NAME", "SCHEDULE", "STATE"),
	)

	var rows []string
	rows = append(rows, title, header)
	for _, s := range m.data.Schedules {
		row := fmt.Sprintf("%-12s %-20s %s",
			truncate(s.Name, 12),
			truncate(s.Schedule, 20),
			monStatusStyle(s.State),
		)
		rows = append(rows, row)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return monPanelStyle.Width(innerW).Render(content)
}

// ---------------------------------------------------------------------------
// View 2: Deployment Status + CI/CD
// ---------------------------------------------------------------------------

func (m monitorModel) viewDeployments(availH int) string {
	pipelinePanel := m.renderPipelinePanel(m.width - 2)
	ghPanel := m.renderGithubPanel((m.width - 2) / 2)
	ecsEventsPanel := m.renderECSEventsPanel(m.width - lipgloss.Width(ghPanel) - 2)

	bottom := lipgloss.JoinHorizontal(lipgloss.Top, ghPanel, " ", ecsEventsPanel)
	return lipgloss.JoinVertical(lipgloss.Left, pipelinePanel, bottom)
}

func (m monitorModel) renderPipelinePanel(w int) string {
	innerW := w - 4
	if innerW < 1 {
		innerW = 1
	}
	title := monTitleStyle.Render("Latest Deployment Pipeline")

	// Determine pipeline stage statuses from ECS events
	stages := []struct {
		label  string
		status string
		detail string
	}{
		{"Git Push", "done", ""},
		{"GH Build", "pending", ""},
		{"ECR Push", "pending", ""},
		{"ECS Deploy", "pending", ""},
		{"Healthy", "pending", ""},
	}

	// Update stages based on available data
	if len(m.workflowRuns) > 0 {
		run := m.workflowRuns[0]
		switch run.Status {
		case "completed":
			if run.Conclusion == "success" {
				stages[1].status = "done"
				stages[2].status = "done"
			} else {
				stages[1].status = "failed"
			}
		case "in_progress":
			stages[1].status = "running"
			if run.StartedAt != nil {
				stages[1].detail = formatUptime(run.StartedAt)
			}
		}
	}

	// Check ECS events for deployment progress
	for _, evt := range m.data.Deployments {
		msg := strings.ToLower(evt.Message)
		if strings.Contains(msg, "deploy") || strings.Contains(msg, "started") {
			if stages[3].status == "pending" {
				stages[3].status = "running"
			}
		}
		if strings.Contains(msg, "healthy") || strings.Contains(msg, "steady") {
			stages[3].status = "done"
			stages[4].status = "done"
		}
		if strings.Contains(msg, "fail") || strings.Contains(msg, "error") {
			stages[3].status = "failed"
		}
	}

	// Render pipeline boxes
	var boxes []string
	for _, stage := range stages {
		icon := "-"
		style := monMutedStyle
		switch stage.status {
		case "done":
			icon = "+"
			style = monSuccessStyle
		case "running":
			icon = "~"
			style = monWarnStyle
		case "failed":
			icon = "x"
			style = monDangerStyle
		}
		detail := stage.detail
		if detail == "" {
			detail = stage.status
		}
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(monBorderColor).
			Padding(0, 1).
			Width(12).
			Render(lipgloss.JoinVertical(lipgloss.Center,
				stage.label,
				style.Render(icon),
				monMutedStyle.Render(truncate(detail, 10)),
			))
		boxes = append(boxes, box)
	}

	var pipelineParts []string
	for i, box := range boxes {
		pipelineParts = append(pipelineParts, box)
		if i < len(boxes)-1 {
			pipelineParts = append(pipelineParts, " ---> ")
		}
	}

	pipeline := lipgloss.JoinHorizontal(lipgloss.Center, pipelineParts...)
	content := lipgloss.JoinVertical(lipgloss.Left, title, "", pipeline, "")
	return monPanelStyle.Width(innerW).Render(content)
}

func (m monitorModel) renderGithubPanel(w int) string {
	innerW := w - 4
	if innerW < 1 {
		innerW = 1
	}
	title := monTitleStyle.Render("GitHub Actions Runs")

	if len(m.workflowRuns) == 0 {
		content := lipgloss.JoinVertical(lipgloss.Left, title,
			monMutedStyle.Render("  No workflow runs (gh CLI not configured)"))
		return monPanelStyle.Width(innerW).Render(content)
	}

	header := lipgloss.NewStyle().Foreground(monAccentColor).Render(
		fmt.Sprintf("%-18s %-10s %-8s", "WORKFLOW", "STATUS", "BRANCH"),
	)

	var rows []string
	rows = append(rows, title, header)
	for i, run := range m.workflowRuns {
		if i >= 6 {
			break
		}
		status := run.Status
		if run.Conclusion != "" {
			status = run.Conclusion
		}
		branch := run.HeadBranch
		row := fmt.Sprintf("%-18s %s %-8s",
			truncate(run.Name, 18),
			padRight(monStatusStyle(status), 10),
			truncate(branch, 8),
		)
		rows = append(rows, row)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return monPanelStyle.Width(innerW).Render(content)
}

func (m monitorModel) renderECSEventsPanel(w int) string {
	innerW := w - 4
	if innerW < 1 {
		innerW = 1
	}
	title := monTitleStyle.Render("ECS Service Events")

	if len(m.data.Deployments) == 0 {
		content := lipgloss.JoinVertical(lipgloss.Left, title, monMutedStyle.Render("  No recent events"))
		return monPanelStyle.Width(innerW).Render(content)
	}

	header := lipgloss.NewStyle().Foreground(monAccentColor).Render(
		fmt.Sprintf("%-8s %-10s %s", "TIME", "SERVICE", "EVENT"),
	)

	var rows []string
	rows = append(rows, title, header)
	for i, evt := range m.data.Deployments {
		if i >= 8 {
			break
		}
		timeStr := evt.Timestamp.Format("15:04:05")
		msg := evt.Message
		maxMsgW := innerW - 8 - 10 - 2
		if maxMsgW < 10 {
			maxMsgW = 10
		}
		row := fmt.Sprintf("%-8s %-10s %s",
			timeStr,
			truncate(evt.ServiceName, 10),
			truncate(msg, maxMsgW),
		)
		rows = append(rows, row)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return monPanelStyle.Width(innerW).Render(content)
}

// ---------------------------------------------------------------------------
// View 3: Service Logs
// ---------------------------------------------------------------------------

func (m monitorModel) viewLogs(availH int) string {
	sideW := m.width / 4
	mainW := m.width - sideW - 2

	serviceList := m.renderLogServiceList(sideW, availH)
	logPanel := m.renderLogPanel(mainW, availH)

	return lipgloss.JoinHorizontal(lipgloss.Top, serviceList, " ", logPanel)
}

func (m monitorModel) renderLogServiceList(w, h int) string {
	innerW := w - 4
	if innerW < 1 {
		innerW = 1
	}

	panelStyle := monPanelStyle
	if !m.logFocusRight {
		panelStyle = monActivePanelStyle
	}

	title := monTitleStyle.Render("Services")

	var rows []string
	rows = append(rows, title)
	for i, name := range m.logServiceList {
		prefix := "  "
		if i == m.selectedService {
			prefix = "> "
		}
		line := prefix + truncate(name, innerW-2)
		if i == m.selectedService {
			line = monSelectedStyle.Width(innerW).Render(line)
		}
		rows = append(rows, line)
	}
	if len(m.logServiceList) == 0 {
		rows = append(rows, monMutedStyle.Render("  No services"))
	}

	rows = append(rows, "")
	rows = append(rows, monMutedStyle.Render("[↑↓] select"))
	rows = append(rows, monMutedStyle.Render("[Enter] load"))
	rows = append(rows, monMutedStyle.Render("[f] filter"))
	rows = append(rows, monMutedStyle.Render("[/] search"))
	rows = append(rows, monMutedStyle.Render("[n] more"))

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return panelStyle.Width(innerW).Render(content)
}

func (m monitorModel) renderLogPanel(w, h int) string {
	innerW := w - 4
	if innerW < 1 {
		innerW = 1
	}

	panelStyle := monPanelStyle
	if m.logFocusRight {
		panelStyle = monActivePanelStyle
	}

	// Determine title
	svcName := "no service selected"
	if m.selectedService < len(m.logServiceList) {
		svcName = m.logServiceList[m.selectedService]
	}
	title := monTitleStyle.Render("Logs: " + svcName)

	// Filter bar
	filterLabel := monMutedStyle.Render("Filter: ")
	filterAll := monMutedStyle.Render("[all]")
	filterErr := monMutedStyle.Render("[ERROR]")
	filterWarn := monMutedStyle.Render("[WARN]")
	switch m.logFilter {
	case "ERROR":
		filterErr = lipgloss.NewStyle().Foreground(monDangerColor).Bold(true).Render("[ERROR]")
	case "WARN":
		filterWarn = lipgloss.NewStyle().Foreground(monWarningColor).Bold(true).Render("[WARN]")
	default:
		filterAll = lipgloss.NewStyle().Foreground(monSuccessColor).Bold(true).Render("[all]")
	}
	searchStr := ""
	if m.logSearchMode {
		searchStr = "  Search: /" + m.logSearchQuery + "_"
	} else if m.logSearchQuery != "" {
		searchStr = "  Search: " + m.logSearchQuery
	}
	filterBar := filterLabel + filterAll + " " + filterErr + " " + filterWarn + searchStr

	// Filter and search log entries
	filtered := m.filteredLogs()

	// Render log lines
	var logLines []string
	startIdx := m.logScrollOffset
	if startIdx >= len(filtered) {
		if len(filtered) > 0 {
			startIdx = len(filtered) - 1
		} else {
			startIdx = 0
		}
	}
	maxLines := h - 6
	if maxLines < 1 {
		maxLines = 1
	}
	for i := startIdx; i < len(filtered) && i-startIdx < maxLines; i++ {
		entry := filtered[i]
		ts := monLogTimeStyle.Render(entry.Timestamp.Format("15:04:05"))
		var levelStr string
		var msgStr string
		switch entry.Level {
		case "ERROR":
			levelStr = monLogErrorStyle.Render("ERROR")
			msgStr = monLogErrorStyle.Render(truncate(entry.Message, innerW-20))
		case "WARN":
			levelStr = monLogWarnStyle.Render("WARN ")
			msgStr = monLogWarnStyle.Render(truncate(entry.Message, innerW-20))
		case "DEBUG":
			levelStr = monLogDebugStyle.Render("DEBUG")
			msgStr = monLogDebugStyle.Render(truncate(entry.Message, innerW-20))
		default:
			levelStr = monLogInfoStyle.Render("INFO ")
			msgStr = monLogInfoStyle.Render(truncate(entry.Message, innerW-20))
		}
		line := ts + " " + levelStr + " " + msgStr
		logLines = append(logLines, line)
	}

	if len(logLines) == 0 {
		if m.logLoading {
			logLines = append(logLines, monMutedStyle.Render("Loading logs..."))
		} else if len(m.logBuffer) == 0 {
			logLines = append(logLines, monMutedStyle.Render("Press [Enter] to load logs for selected service"))
		} else {
			logLines = append(logLines, monMutedStyle.Render("No log entries match current filter"))
		}
	}

	bottomBar := monMutedStyle.Render("[n] load more  [f] filter  [/] search  [Esc] clear  [←] services")
	if m.logNextToken == "" && len(m.logBuffer) > 0 {
		bottomBar = monMutedStyle.Render("End of logs  [f] filter  [/] search  [←] services")
	}

	var rows []string
	rows = append(rows, title, filterBar, "")
	rows = append(rows, logLines...)
	rows = append(rows, "", bottomBar)

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return panelStyle.Width(innerW).Render(content)
}

func (m monitorModel) filteredLogs() []MonitorLogEntry {
	var result []MonitorLogEntry
	for _, entry := range m.logBuffer {
		if m.logFilter != "" && entry.Level != m.logFilter {
			continue
		}
		if m.logSearchQuery != "" && !strings.Contains(strings.ToLower(entry.Message), strings.ToLower(m.logSearchQuery)) {
			continue
		}
		result = append(result, entry)
	}
	return result
}

// ---------------------------------------------------------------------------
// View 4: ECS Exec / SSH
// ---------------------------------------------------------------------------

func (m monitorModel) execServiceList() []MonitorService {
	var services []MonitorService
	for _, svc := range m.data.Services {
		if len(svc.Tasks) > 0 {
			services = append(services, svc)
		}
	}
	return services
}

func (m monitorModel) viewExec(availH int) string {
	sideW := m.width / 4
	mainW := m.width - sideW - 2

	svcPanel := m.renderExecServiceList(sideW, availH)
	taskPanel := m.renderExecTaskPanel(mainW, availH)

	return lipgloss.JoinHorizontal(lipgloss.Top, svcPanel, " ", taskPanel)
}

func (m monitorModel) renderExecServiceList(w, h int) string {
	innerW := w - 4
	if innerW < 1 {
		innerW = 1
	}

	panelStyle := monPanelStyle
	if !m.execFocusRight {
		panelStyle = monActivePanelStyle
	}

	title := monTitleStyle.Render("Services")
	services := m.execServiceList()

	var rows []string
	rows = append(rows, title)
	for i, svc := range services {
		prefix := "  "
		if i == m.execSelectedService {
			prefix = "> "
		}
		taskCount := fmt.Sprintf("%d tasks", len(svc.Tasks))
		if len(svc.Tasks) == 1 {
			taskCount = "1 task"
		}
		line := fmt.Sprintf("%s%-12s %s", prefix, truncate(svc.Name, 12), monMutedStyle.Render(taskCount))
		if i == m.execSelectedService {
			line = monSelectedStyle.Width(innerW).Render(prefix + truncate(svc.Name, 12) + " " + taskCount)
		}
		rows = append(rows, line)
	}
	if len(services) == 0 {
		rows = append(rows, monMutedStyle.Render("  No services with tasks"))
	}
	rows = append(rows, "")
	rows = append(rows, monMutedStyle.Render("[↑↓] select"))
	rows = append(rows, monMutedStyle.Render("[→] view tasks"))
	rows = append(rows, monMutedStyle.Render("[Enter] connect"))

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return panelStyle.Width(innerW).Render(content)
}

func (m monitorModel) renderExecTaskPanel(w, h int) string {
	innerW := w - 4
	if innerW < 1 {
		innerW = 1
	}

	panelStyle := monPanelStyle
	if m.execFocusRight {
		panelStyle = monActivePanelStyle
	}

	services := m.execServiceList()
	svcName := "no service selected"
	var tasks []MonitorTask
	if m.execSelectedService < len(services) {
		svcName = services[m.execSelectedService].Name
		tasks = services[m.execSelectedService].Tasks
	}

	title := monTitleStyle.Render("Tasks: " + svcName)

	var rows []string
	rows = append(rows, title, "")

	if len(tasks) == 0 {
		rows = append(rows, monMutedStyle.Render("  No running tasks for this service"))
	} else {
		header := lipgloss.NewStyle().Foreground(monAccentColor).Render(
			fmt.Sprintf("%-14s %-20s %-8s %-14s %s", "TASK ID", "CONTAINER", "STATUS", "AZ", "UPTIME"),
		)
		rows = append(rows, header)

		for i, t := range tasks {
			prefix := "  "
			if i == m.execSelectedTask {
				prefix = "> "
			}
			az := t.AZ
			if len(az) > 14 {
				az = az[len(az)-14:]
			}
			line := fmt.Sprintf("%s%-14s %-20s %-8s %-14s %s",
				prefix,
				truncate(t.TaskID, 14),
				truncate(t.ContainerName, 20),
				truncate(t.Status, 8),
				truncate(az, 14),
				formatUptime(t.StartedAt),
			)
			if i == m.execSelectedTask && m.execFocusRight {
				line = monSelectedStyle.Width(innerW).Render(line)
			}
			rows = append(rows, line)
		}
	}

	rows = append(rows, "")
	rows = append(rows, monMutedStyle.Render("Press [Enter] to open interactive shell"))
	rows = append(rows, "")
	rows = append(rows, monMutedStyle.Render("This uses ECS Exec (aws ecs execute-command)"))
	rows = append(rows, monMutedStyle.Render("The TUI will pause while the shell is active."))
	rows = append(rows, monMutedStyle.Render("Type 'exit' to return to the dashboard."))
	rows = append(rows, "")
	rows = append(rows, monMutedStyle.Render("Prerequisites: ECS Exec must be enabled on the service,"))
	rows = append(rows, monMutedStyle.Render("task role must have SSM permissions, SSM Plugin installed."))

	if m.execLaunching {
		rows = append(rows, "", monWarnStyle.Render("Launching ECS Exec..."))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return panelStyle.Width(innerW).Render(content)
}

// ---------------------------------------------------------------------------
// Help overlay
// ---------------------------------------------------------------------------

func (m monitorModel) renderHelpOverlay(body string) string {
	helpContent := lipgloss.JoinVertical(lipgloss.Left,
		monTitleStyle.Render("Keyboard Shortcuts"),
		"",
		monAccentStyle.Render("Navigation:"),
		"  Tab / Shift+Tab    Cycle views",
		"  1 / 2 / 3 / 4     Jump to view",
		"  ↑/k  ↓/j           Move up/down",
		"  ←/h  →/l           Focus left/right panel",
		"",
		monAccentStyle.Render("Actions:"),
		"  Enter              Select / connect",
		"  r                  Manual refresh",
		"  q / Ctrl+C         Quit",
		"",
		monAccentStyle.Render("Logs (View 3):"),
		"  f                  Cycle filter (all/ERROR/WARN)",
		"  /                  Enter search mode",
		"  Esc                Clear search",
		"  n                  Load more logs",
		"",
		monAccentStyle.Render("Exec (View 4):"),
		"  Enter on task      Open ECS Exec shell",
		"",
		monMutedStyle.Render("Press ? to close help"),
	)

	helpBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(monPrimaryColor).
		Padding(1, 2).
		Render(helpContent)

	// Overlay help box centered over body
	bodyLines := strings.Split(body, "\n")
	bodyH := len(bodyLines)
	helpLines := strings.Split(helpBox, "\n")
	helpH := len(helpLines)
	helpW := lipgloss.Width(helpBox)

	startRow := (bodyH - helpH) / 2
	startCol := (m.width - helpW) / 2
	if startRow < 0 {
		startRow = 0
	}
	if startCol < 0 {
		startCol = 0
	}

	// Simple approach: return help box alone with padding
	topPad := strings.Repeat("\n", startRow)
	leftPad := strings.Repeat(" ", startCol)
	var centeredLines []string
	for _, l := range helpLines {
		centeredLines = append(centeredLines, leftPad+l)
	}

	return topPad + strings.Join(centeredLines, "\n")
}

// ---------------------------------------------------------------------------
// Viewport management
// ---------------------------------------------------------------------------

func (m *monitorModel) updateViewports() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	vpH := m.height - 4
	if vpH < 1 {
		vpH = 1
	}
	m.overviewVP = viewport.New(m.width, vpH)
	m.deploymentVP = viewport.New(m.width, vpH)
	m.logVP = viewport.New(m.width, vpH)
	m.execVP = viewport.New(m.width, vpH)
}

// ---------------------------------------------------------------------------
// Tea commands
// ---------------------------------------------------------------------------

func fetchDashboardDataCmd(env Env, envName string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		data, err := fetchDashboardData(ctx, env, selectedAWSProfile)
		return monitorDataMsg{data: data, err: err}
	}
}

func fetchWorkflowsCmd(env Env) tea.Cmd {
	return func() tea.Msg {
		runs, err := fetchGitHubWorkflows(env)
		return monitorWorkflowsMsg{runs: runs, err: err}
	}
}

func fetchLogsCmd(env Env, serviceName, nextToken string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		cfg, err := buildAWSConfig(ctx, selectedAWSProfile, env.Region)
		if err != nil {
			return monitorLogsMsg{page: LogPage{
				ServiceName: serviceName,
				Error:       err,
			}}
		}
		cwlClient := cloudwatchlogs.NewFromConfig(cfg)
		page, err := fetchLogsPage(ctx, cwlClient, env, serviceName, nextToken, 100)
		if err != nil {
			return monitorLogsMsg{page: LogPage{
				ServiceName: serviceName,
				Error:       err,
			}}
		}
		return monitorLogsMsg{page: page}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
		return monitorTickMsg(t)
	})
}

// ---------------------------------------------------------------------------
// Utility helpers
// ---------------------------------------------------------------------------

// truncate shortens a string to maxLen, appending "..." if needed.
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// padRight pads a string (which may include ANSI codes) to the desired visible width.
func padRight(s string, width int) string {
	visible := lipgloss.Width(s)
	if visible >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visible)
}
