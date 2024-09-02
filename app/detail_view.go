package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type detailView interface {
	base() detailViewModel
	focused() bool
	setFocused(bool)
	setSize(int, int)
	// env(e Env) Env

	Update(msg tea.Msg) (tea.Model, tea.Cmd)
	View() string
	helpMessage() string
}

type envModifierView interface {
	env(e Env) Env
}

type detailViewModel struct {
	inputs      []inputModel
	selectedIdx int
	title       string
	description string
	width       int
	height      int
	isFocused   bool
	viewport    viewport.Model
}

func (i *detailViewModel) base() detailViewModel {
	return *i
}

func (i *detailViewModel) focused() bool {
	return i.isFocused
}

func (i *detailViewModel) setSize(width, height int) {
	i.width = width
	i.height = height
	i.viewport.Width = width
	i.viewport.Height = height
	i.updateViewportContent()
}

func (m *detailViewModel) updateViewportContent() {
	m.viewport.SetContent(m.renderContent())
}

func (i *detailViewModel) setFocused(focused bool) {
	i.isFocused = focused
}

func (m *detailViewModel) Init() tea.Cmd {
	return m.inputs[0].Focus()
}

func (m *detailViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !m.isFocused {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "left":
			return m, func() tea.Msg { return detailLeaveFocusMsg{} }
		case "tab", "shift+tab", "up", "down":
			s := msg.String()

			if s == "up" || s == "shift+tab" {
				m.selectedIdx--
			} else {
				m.selectedIdx++
			}

			if m.selectedIdx > len(m.inputs)-1 {
				m.selectedIdx = 0
			} else if m.selectedIdx < 0 {
				m.selectedIdx = len(m.inputs) - 1
			}

			cmds := make([]tea.Cmd, len(m.inputs))
			for i := 0; i < len(m.inputs); i++ {
				if i == m.selectedIdx {
					cmds[i] = m.inputs[i].Focus()
					continue
				}
				m.inputs[i].Blur()
			}
			m.ensureSelectedItemVisible()

			return m, tea.Batch(cmds...)
		case "enter":
			// Open modal for editing the current field
			return m, func() tea.Msg {
				return openModalMsg{
					input: m.inputs[m.selectedIdx].base(),
					onConfirm: func(newValue inputValue) tea.Cmd {
						return func() tea.Msg {
							return updateFieldMsg{index: m.selectedIdx, value: newValue}
						}
					},
				}
			}
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	case updateFieldMsg:
		m.inputs[msg.index].setValue(msg.value)
		m.updateViewportContent()
	}

	cmds := []tea.Cmd{}
	cmd := m.updateInputs(msg)
	cmds = append(cmds, cmd)
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m *detailViewModel) ensureSelectedItemVisible() {
	contentHeight := 0
	selectedTop := 0
	selectedBottom := 0

	for i, input := range m.inputs {
		inputHeight := lipgloss.Height(input.View())
		if i < m.selectedIdx {
			selectedTop += inputHeight + 1 // Add 1 for the newline between inputs
		}
		if i == m.selectedIdx {
			selectedBottom = selectedTop + inputHeight
		}
		contentHeight += inputHeight + 1
	}

	if selectedTop < m.viewport.YOffset {
		m.viewport.YOffset = selectedTop
	} else if selectedBottom > m.viewport.YOffset+m.viewport.Height {
		m.viewport.YOffset = selectedBottom - m.viewport.Height
	}

	m.updateViewportContent()
}

func (m *detailViewModel) updateInputs(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, len(m.inputs))

	for i := range m.inputs {
		m.inputs[i], cmds[i] = m.inputs[i].Update(msg)
	}

	return tea.Batch(cmds...)
}

func (m *detailViewModel) renderContent() string {
	var b strings.Builder

	for i, input := range m.inputs {
		var style lipgloss.Style
		if i == m.selectedIdx {
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("205")).
				Bold(true)
		} else {
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))
		}

		inputView := input.View()
		b.WriteString(style.Render(inputView))
		if i < len(m.inputs)-1 {
			b.WriteRune('\n')
		}
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(b.String())
}

func (m *detailViewModel) View() string {
	return fmt.Sprintf("%s\n\n%s",
		// lipgloss.NewStyle().Bold(true).Render(m.title),
		m.viewport.View(),
		helpTextStyle.Render(m.helpMessage()),
	)
}

func (m *detailViewModel) helpMessage() string {
	return fmt.Sprintf("Field %d  •  \"%s\" • Enter: Edit", m.selectedIdx+1, m.inputs[m.selectedIdx].base().title)
}

type detailLeaveFocusMsg struct{}
