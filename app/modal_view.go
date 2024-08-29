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

func newModalModel(input baseInputModel, screenWidth, screenHeight int, onConfirm func(inputValue) tea.Cmd) *modalModel {
	slog.Info("newModalModel", "input", &input, "input type", input.value.Type())
	var model tea.Model
	width := 60
	height := 7
	styles := baseTextInputTheme.Focused
	switch input.value.Type() {
	case InputValueTypeString, InputValueTypeInt:
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
	case InputValueTypeSingleSelect:
		height = 30
		list := NewInputListSelectModel(input.value, width-4, height-7)
		model = list
	case InputValueTypeSlice:
		height = 30
		list := NewInputListSelectModel(input.value, width-4, height-7)
		model = list
	case InputValueTypeBool:
		boolInput := newBoolInputModel(input, width-8)
		boolInput.Focus()
		model = boolInput
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
			if m.input.IsValid() && m.input.value.Type() != InputValueTypeSlice {
				value := m.input.value
				if m.input.value.Type() == InputValueTypeSingleSelect {
					l := m.model.(InputListSelectModel)
					v := m.input.value.(sliceSelectValue)
					value = sliceSelectValue{index: l.Index(), value: v.value}
				}
				return m, tea.Batch(
					m.onConfirm(value),
					func() tea.Msg { return closeModalMsg{} },
				)
			}
		case "shift+enter", "tab":
			if m.input.value.Type() == InputValueTypeSlice {
				s := m.model.(InputListSelectModel)
				// if we can not escape it means we can not commit edit as well? Right? Debatable.
				if s.CanEscape(msg) {
					value := sliceValue{s.ListItems()}
					return m, tea.Batch(
						m.onConfirm(value),
						func() tea.Msg { return closeModalMsg{} },
					)
				}
			}
		case "esc":
			if m.input.value.Type() == InputValueTypeSlice {
				m := m.model.(InputListSelectModel)
				// we can escape only if we are not editing, otherwise we need pass message to the list to cancel edit
				if m.CanEscape(msg) {
					return m, func() tea.Msg {
						return closeModalMsg{}
					}
				}
			} else {
				return m, func() tea.Msg {
					return closeModalMsg{}
				}
			}
		}
	}

	var cmd tea.Cmd
	m.model, cmd = m.model.Update(msg)

	switch m.input.value.Type() {
	case InputValueTypeString, InputValueTypeInt:
		mm, _ := m.model.(TextInputFullModel)
		m.input.value = stringValue{mm.Model.Value()}
	case InputValueTypeSlice:
		// do nothing for list
		sliceModel, _ := m.model.(InputListSelectModel)
		m.input.value = sliceValue{sliceModel.ListItems()}
	case InputValueTypeSingleSelect:
		// do nothing fo single select
	case InputValueTypeBool:
		boolInput, _ := m.model.(boolInputModel)
		m.input.value = boolValue{boolInput.Value().Bool()}
	}

	return m, cmd
}

func (m modalModel) View() string {
	valid := m.input.IsValid()
	styles := baseTextInputTheme.Focused

	var validationStyle lipgloss.Style
	var validationResult string
	var helpText string

	vr := ""
	if m.input.base().validator != nil {
		if valid {
			validationStyle = validStyle
			validationResult = fmt.Sprintf("✓ %s", m.input.validationMessage)
			helpText = "Press Enter to confirm, Esc to cancel"
		} else {
			validationStyle = invalidStyle
			validationResult = fmt.Sprintf("✗ %s", m.input.validationMessage)
			helpText = "Fix the input or press Esc to cancel"
		}
		vr = validationStyle.Padding(0, 2, 0, 0).Render(wrapText(validationResult, m.width-4))
	}

	var modelView string
	switch m.input.value.Type() {
	case InputValueTypeString, InputValueTypeInt:
		mm, _ := m.model.(TextInputFullModel)
		mm.PlaceholderStyle = styles.Placeholder
		mm.PromptStyle = styles.Prompt
		mm.Cursor.Style = styles.Cursor
		mm.Cursor.TextStyle = styles.CursorText
		mm.TextStyle = styles.Text
		mm.Prompt = styles.PromptText
		modelView = mm.View()
	case InputValueTypeSingleSelect:
		lm, _ := m.model.(InputListSelectModel)
		modelView = lm.View()
		helpText = "Select one and press Enter, or press Esc to cancel"
	case InputValueTypeSlice:
		lm, _ := m.model.(InputListSelectModel)
		modelView = lm.View()
		helpText = "Tab: commit the list, Enter: start and commit edit, Esc: cancel edit or exit, A/a: append new, d/D: delete selected"
	case InputValueTypeBool:
		boolInput, _ := m.model.(boolInputModel)
		modelView = boolInput.View()
		helpText = "Space: toggle value, Enter: confirm, Esc: cancel"
	}

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 0).
		Width(m.width).
		Height(m.height)

	desc := wrapText(m.input.description, m.width-4)

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		titleStyle.MarginBottom(0).Render(m.input.title),
		styles.Description.Padding(2, 2).Render(desc),
		lipgloss.NewStyle().PaddingLeft(2).PaddingRight(2).PaddingBottom(2).Width(m.width-4).Render(modelView),
		vr,
		lipgloss.NewStyle().Align(lipgloss.Center).Render(helpTextStyle.Render(wrapText(helpText, m.width-4))),
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
