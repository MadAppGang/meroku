package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The configuration warnings screen: shown before a TUI deploy when the
// environment has warnings, with "Cancel and fix" (the default) and "Proceed
// anyway". Drawn inline, not on the alternate screen. Colours come only from
// theme.

const (
	// warningsScreenChrome is the lines around the body: title, rule, blank,
	// the bordered buttons (3), help and a spare line for the cursor.
	warningsScreenChrome = 8
	// warningsScreenMinBody is the fewest body lines shown, however small the
	// window.
	warningsScreenMinBody = 3
	// warningsScreenMaxRule caps the rule under the title on wide windows.
	warningsScreenMaxRule = 100
	// warningsScreenDetailIndent indents detail and docs under the title.
	warningsScreenDetailIndent = "  "
)

var (
	warningsTitleStyle  = lipgloss.NewStyle().Foreground(theme.Warning).Bold(true)
	warningsRuleStyle   = lipgloss.NewStyle().Foreground(theme.Border)
	warningsItemStyle   = lipgloss.NewStyle().Foreground(theme.Muted)
	warningsHeadStyle   = lipgloss.NewStyle().Foreground(theme.TextStrong).Bold(true)
	warningsDetailStyle = lipgloss.NewStyle().Foreground(theme.Text)
	warningsDocsStyle   = lipgloss.NewStyle().Foreground(theme.Info)
	warningsHelpStyle   = lipgloss.NewStyle().Foreground(theme.Faint)

	warningsButtonFocused = lipgloss.NewStyle().
				Foreground(theme.Warning).
				Bold(true).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(theme.Warning).
				Padding(0, 1)
	warningsButtonBlurred = lipgloss.NewStyle().
				Foreground(theme.Muted).
				Border(lipgloss.HiddenBorder()).
				Padding(0, 1)
)

// warningsButtons are the two choices, left to right.
var warningsButtons = []struct {
	label  string
	choice warningChoice
}{
	{"Cancel and fix", warningChoiceCancel},
	{"Proceed anyway", warningChoiceProceed},
}

// configWarningsModel is the Bubble Tea model of the warnings screen.
//
// width and height are 0 until the terminal reports its size; 0 means no
// wrapping and no scrolling.
type configWarningsModel struct {
	env      string
	warnings []ConfigWarning
	focus    int // index into warningsButtons
	choice   warningChoice
	done     bool
	width    int
	height   int
	lines    []string // the body, wrapped to width and styled
	offset   int      // first body line shown
}

func newConfigWarningsModel(env string, ws []ConfigWarning) configWarningsModel {
	m := configWarningsModel{env: env, warnings: ws, choice: warningChoiceCancel}
	m.lines = m.renderBody()
	return m
}

// bodyHeight is how many body lines fit. The buttons always keep their lines.
func (m configWarningsModel) bodyHeight() int {
	if m.height <= 0 {
		return len(m.lines)
	}
	return min(len(m.lines), max(m.height-warningsScreenChrome, warningsScreenMinBody))
}

// maxOffset is the furthest the body can scroll.
func (m configWarningsModel) maxOffset() int {
	return max(len(m.lines)-m.bodyHeight(), 0)
}

func (m *configWarningsModel) scroll(by int) {
	m.offset = min(max(m.offset+by, 0), m.maxOffset())
}

// renderBody is every warning: its item, title, detail and docs link, one
// styled line per screen row.
func (m configWarningsModel) renderBody() []string {
	var out []string
	for i, w := range m.warnings {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, m.wrapStyled(warningsItemStyle, "", warningItemLabel(w.Item))...)
		out = append(out, m.wrapStyled(warningsHeadStyle, "", w.Title)...)
		for _, line := range strings.Split(w.Detail, "\n") {
			out = append(out, m.wrapStyled(warningsDetailStyle, warningsScreenDetailIndent, line)...)
		}
		if w.Docs != "" {
			out = append(out, m.wrapStyled(warningsDocsStyle, warningsScreenDetailIndent, "Docs: "+w.Docs)...)
		}
	}
	return out
}

// wrapStyled wraps plain text to the window, indents it and colours each
// resulting line. Text is wrapped before it is coloured so line widths are
// measured on what is printed.
func (m configWarningsModel) wrapStyled(style lipgloss.Style, indent, text string) []string {
	if strings.TrimSpace(text) == "" {
		return []string{""}
	}
	var lines []string
	if avail := m.width - 1 - len(indent); m.width > 0 && avail > 10 {
		wrapped := lipgloss.NewStyle().Width(avail).Render(text)
		for _, l := range strings.Split(wrapped, "\n") {
			lines = append(lines, strings.TrimRight(l, " "))
		}
	} else {
		lines = []string{text}
	}
	for i, l := range lines {
		lines[i] = style.Render(indent + l)
	}
	return lines
}

// warningItemLabel names the item a warning is about, as the YAML does.
func warningItemLabel(item WarningItem) string {
	switch item.Kind {
	case WarningKindEnv:
		return "environment"
	case WarningKindBackend:
		return "backend"
	case WarningKindValkey:
		return "valkey"
	case WarningKindEventBus:
		return "event bus"
	}
	kind := strings.ReplaceAll(item.Kind, "_", " ")
	if item.Name == "" {
		return kind
	}
	return kind + " " + item.Name
}

func (m configWarningsModel) Init() tea.Cmd { return nil }

func (m configWarningsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.lines = m.renderBody()
		m.scroll(0)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h", "shift+tab":
			m.focus = (m.focus + len(warningsButtons) - 1) % len(warningsButtons)
		case "right", "l", "tab":
			m.focus = (m.focus + 1) % len(warningsButtons)
		case "enter":
			return m.finish(warningsButtons[m.focus].choice)
		case "p":
			return m.finish(warningChoiceProceed)
		case "esc", "q", "ctrl+c":
			return m.finish(warningChoiceCancel)
		case "up", "k":
			m.scroll(-1)
		case "down", "j":
			m.scroll(1)
		case "pgup":
			m.scroll(-m.bodyHeight())
		case "pgdown":
			m.scroll(m.bodyHeight())
		}
	}
	return m, nil
}

func (m configWarningsModel) finish(choice warningChoice) (tea.Model, tea.Cmd) {
	m.choice = choice
	m.done = true
	return m, tea.Quit
}

func (m configWarningsModel) View() string {
	if m.done {
		// The screen clears itself; runConfigWarningsScreen prints the list as
		// plain text, so the scrollback keeps every link whole.
		return ""
	}

	buttons := make([]string, 0, len(warningsButtons))
	for i, btn := range warningsButtons {
		style := warningsButtonBlurred
		if i == m.focus {
			style = warningsButtonFocused
		}
		buttons = append(buttons, style.Render(btn.label))
	}

	height := m.bodyHeight()
	help := "←/→ choose · enter confirm · esc cancel · p proceed"
	if len(m.lines) > height {
		help += fmt.Sprintf(" · ↑/↓ scroll (%d-%d of %d)", m.offset+1, m.offset+height, len(m.lines))
	}

	parts := []string{m.header()}
	parts = append(parts, m.lines[m.offset:m.offset+height]...)
	parts = append(parts,
		"",
		lipgloss.JoinHorizontal(lipgloss.Top, buttons...),
		warningsHelpStyle.Render(help),
	)
	return strings.Join(parts, "\n") + "\n"
}

// header is the title line and the rule under it.
func (m configWarningsModel) header() string {
	noun := "warnings"
	if len(m.warnings) == 1 {
		noun = "warning"
	}
	title := warningsTitleStyle.Render(fmt.Sprintf("⚠  %d configuration %s in %s", len(m.warnings), noun, m.env))
	ruleWidth := warningsScreenMaxRule
	if m.width > 0 {
		ruleWidth = min(m.width-1, warningsScreenMaxRule)
	}
	rule := warningsRuleStyle.Render(strings.Repeat("─", max(ruleWidth, 10)))
	return title + "\n" + rule
}

// runConfigWarningsScreen shows the warnings and waits for a choice. When the
// screen closes it leaves a plain-text record in the scrollback: a line for a
// Proceed, or the warnings themselves for a Cancel, so the user has what to
// fix with every link whole; the gate then prints the Cancel line.
func runConfigWarningsScreen(env string, ws []ConfigWarning) (warningChoice, error) {
	final, err := tea.NewProgram(newConfigWarningsModel(env, ws)).Run()
	if err != nil {
		return warningChoiceCancel, err
	}
	m, ok := final.(configWarningsModel)
	if !ok {
		return warningChoiceCancel, fmt.Errorf("unexpected warnings screen model %T", final)
	}
	if m.choice == warningChoiceProceed {
		// Generate prints the same list as it runs.
		fmt.Printf("Proceeding with %d configuration warning(s) in %s.\n", len(ws), env)
	} else {
		printConfigWarnings(os.Stdout, ws)
	}
	return m.choice, nil
}
