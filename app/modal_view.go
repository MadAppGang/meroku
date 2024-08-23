package main

import (
	"fmt"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type modalModel struct {
	input        baseInputModel
	model        tea.Model
	width        int
	height       int
	screenWidth  int
	screenHeight int
	onConfirm    func(inputValue) tea.Cmd
}

type openModalMsg struct {
	input     baseInputModel
	onConfirm func(inputValue) tea.Cmd
}

type updateFieldMsg struct {
	index int
	value inputValue
}

type closeModalMsg struct{}

func newModalModel(input baseInputModel, width, height, screenWidth, screenHeight int, onConfirm func(inputValue) tea.Cmd) *modalModel {
	slog.Info("newModalModel", "input", &input, "input type", input.value.Type())
	var model tea.Model
	switch input.value.Type() {
	case InputValueTypeString, InputValueTypeInt, InputValueTypeSlice, InputValueTypeBool:
		styles := baseTextInputTheme.Focused
		textinput := NewTextInputFullModel()
		textinput.SetValue(input.value.String())
		textinput.Focus()
		textinput.Placeholder = input.placeholder
		textinput.Width = width - 4 // Account for padding
		textinput.PlaceholderStyle = styles.Placeholder
		textinput.PromptStyle = styles.Prompt
		textinput.Cursor.Style = styles.Cursor
		textinput.Cursor.TextStyle = styles.CursorText
		textinput.TextStyle = styles.Text
		textinput.Prompt = styles.PromptText
		model = textinput
		slog.Info("newModalModel >>", "model", model)
	}

	return &modalModel{
		input:        input,
		model:        model,
		width:        width,
		height:       height,
		onConfirm:    onConfirm,
		screenWidth:  screenWidth,
		screenHeight: screenHeight,
	}
}

func (m modalModel) Init() tea.Cmd {
	return m.model.Init()
}

func (m modalModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			// Perform validation using regex for string values
			if m.input.IsValid() {
				return m, tea.Batch(
					m.onConfirm(m.input.value),
					func() tea.Msg { return closeModalMsg{} },
				)
			}
		case "esc":
			return m, func() tea.Msg {
				return closeModalMsg{}
			}
		}
	}

	var cmd tea.Cmd
	m.model, cmd = m.model.Update(msg)

	switch m.input.value.Type() {
	case InputValueTypeString:
		mm, _ := m.model.(TextInputFullModel)
		m.input.value = stringValue{mm.Model.Value()}
	}

	return m, cmd
}

func (m modalModel) View() string {
	valid := m.input.IsValid()
	styles := baseTextInputTheme.Focused

	var validationStyle lipgloss.Style
	var validationResult string
	var helpText string

	if valid {
		validationStyle = validStyle
		validationResult = fmt.Sprintf("✓ %s", m.input.validationMessage)
		helpText = fmt.Sprintf("Press Enter to confirm, Esc to cancel")
	} else {
		validationStyle = invalidStyle
		validationResult = fmt.Sprintf("✗ %s", m.input.validationMessage)
		helpText = fmt.Sprintf("Fix the input or press Esc to cancel")
	}

	switch m.input.value.Type() {
	case InputValueTypeString:
		mm, _ := m.model.(TextInputFullModel)
		mm.PlaceholderStyle = styles.Placeholder
		mm.PromptStyle = styles.Prompt
		mm.Cursor.Style = styles.Cursor
		mm.Cursor.TextStyle = styles.CursorText
		mm.TextStyle = styles.Text
		mm.Prompt = styles.PromptText
	}

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 0).
		Width(m.width).
		Height(m.height)

	vr := wrapText(validationResult, m.width-4)
	desc := wrapText(m.input.description, m.width-4)

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		titleStyle.MarginBottom(0).Render(m.input.title),
		styles.Description.Padding(2, 2).Render(desc),
		lipgloss.NewStyle().PaddingLeft(2).PaddingRight(2).PaddingBottom(2).Render(m.model.View()),
		validationStyle.Padding(0, 2, 0, 0).Render(vr),
		helpTextStyle.Render(helpText),
	)

	return lipgloss.Place(
		m.screenWidth,
		m.screenHeight,
		lipgloss.Center,
		lipgloss.Center,
		modalStyle.Render(content),
		lipgloss.WithWhitespaceChars("░"),
		lipgloss.WithWhitespaceForeground(subtle),
	)
}
