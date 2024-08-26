package main

import (
	"fmt"
	"log/slog"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type detailView interface {
	base() detailViewModel
	focused() bool
	setFocused(bool)
	setSize(int, int)

	Update(msg tea.Msg) (tea.Model, tea.Cmd)
	View() string
	helpMessage() string
}

type detailViewModel struct {
	inputs      []inputModel
	selectedIdx int
	title       string
	description string
	width       int
	height      int
	isFocused   bool
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
		slog.Warn("detailViewModel.Update", "updateFieldMsg", slog.Int("index", msg.index), "value", msg.value)
		m.inputs[msg.index].setValue(msg.value)
	}

	cmd := m.updateInputs(msg)
	return m, cmd
}

func (m *detailViewModel) updateInputs(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, len(m.inputs))

	for i := range m.inputs {
		m.inputs[i], cmds[i] = m.inputs[i].Update(msg)
	}

	return tea.Batch(cmds...)
}

func (m *detailViewModel) View() string {
	var b strings.Builder

	for i := range m.inputs {
		var style lipgloss.Style
		if i == m.selectedIdx {
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("205")).
				Bold(true)
		} else {
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))
		}

		inputView := m.inputs[i].View()
		b.WriteString(style.Render(inputView))
		if i < len(m.inputs)-1 {
			b.WriteRune('\n')
		}
	}

	return lipgloss.NewStyle().
		Padding(1, 2).
		Render(b.String())
}

func (m *detailViewModel) helpMessage() string {
	return fmt.Sprintf("Field %d  •  \"%s\" • Enter: Edit", m.selectedIdx+1, m.inputs[m.selectedIdx].base().title)
}

type detailLeaveFocusMsg struct{}
