package main

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Key bindings
type keyMap struct {
	Open key.Binding
	Quit key.Binding
}

var keys = keyMap{
	Open: key.NewBinding(
		key.WithKeys("o"),
		key.WithHelp("o", "open web page"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit to main menu"),
	),
}

// Styles
var (
	webTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			MarginTop(1).
			MarginBottom(1)

	webUrlStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Bold(true)

	webInfoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262")).
			MarginBottom(1)

	webSuccessStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575"))

	webErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6B6B"))

	webHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262")).
			MarginTop(1)

	webBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#874BFD")).
			Padding(1, 2).
			MarginTop(1).
			MarginBottom(1)
)

type serverStatus int

const (
	statusRunning serverStatus = iota
	statusStarting
	statusError
)

type webServerModel struct {
	serverURL    string
	status       serverStatus
	spinner      spinner.Model
	message      string
	messageTimer time.Time
}

func initialWebServerModel(url string) webServerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	
	return webServerModel{
		serverURL: url,
		status:    statusStarting,
		spinner:   s,
	}
}

func (m webServerModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		tea.Tick(time.Second, func(t time.Time) tea.Msg { return serverStartedMsg{} }),
	)
}

type serverStartedMsg struct{}
type openBrowserMsg struct{ success bool }
type clearMessageMsg struct{}

func (m webServerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Open):
			go func() {
				err := openBrowser(m.serverURL)
				if err == nil {
					m.message = "Browser opened successfully!"
				} else {
					m.message = fmt.Sprintf("Failed to open browser: %v", err)
				}
			}()
			m.message = "Opening browser..."
			m.messageTimer = time.Now()
			return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg { return clearMessageMsg{} })
		}
		
	case serverStartedMsg:
		m.status = statusRunning
		return m, nil
		
	case clearMessageMsg:
		if time.Since(m.messageTimer) >= 3*time.Second {
			m.message = ""
		}
		return m, nil
		
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	
	return m, nil
}

func (m webServerModel) View() string {
	var content string
	
	// Title
	title := webTitleStyle.Render("🌐 Web Application Server")
	
	// Server status
	var statusLine string
	switch m.status {
	case statusStarting:
		statusLine = m.spinner.View() + " Starting server..."
	case statusRunning:
		statusLine = webSuccessStyle.Render("✓ Server running at ") + webUrlStyle.Render(m.serverURL)
	case statusError:
		statusLine = webErrorStyle.Render("✗ Server error")
	}
	
	// Info box
	infoContent := lipgloss.JoinVertical(
		lipgloss.Left,
		statusLine,
		"",
		webInfoStyle.Render("The web application is now accessible in your browser."),
		webInfoStyle.Render("API endpoints are available at "+m.serverURL+"/api/*"),
	)
	
	infoBox := webBoxStyle.Render(infoContent)
	
	// Message (if any)
	var messageView string
	if m.message != "" {
		messageView = "\n" + webSuccessStyle.Render(m.message) + "\n"
	}
	
	// Help
	help := webHelpStyle.Render(
		"Press 'o' to open web page • Press 'q' to return to main menu",
	)
	
	// Combine all elements
	content = lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		infoBox,
		messageView,
		help,
	)
	
	return content
}

func runWebServerTUI(serverURL string) error {
	p := tea.NewProgram(initialWebServerModel(serverURL))
	_, err := p.Run()
	return err
}