package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type textInputModel struct {
	baseInputModel
	model textinput.Model
}

func newTextInputModel(i baseInputModel, value string) *textInputModel {
	model := textinput.New()
	model.SetValue(value)
	i.value = stringValue{value}
	return &textInputModel{
		baseInputModel: i,
		model:          model,
	}
}

func (i *textInputModel) Blur() {
	i.model.Blur()
}

func (i *textInputModel) Focus() tea.Cmd {
	return i.model.Focus()
}

func (i *textInputModel) value() inputValue {
	return i.baseInputModel.value
}

func (i *textInputModel) setValue(v inputValue) {
	i.baseInputModel.value = v
	i.model.SetValue(v.String())
}

func (i *textInputModel) View() string {
	var styles TextInputThemeOneStyle
	if i.model.Focused() {
		styles = baseTextInputTheme.Focused
	} else {
		styles = baseTextInputTheme.Blurred
	}

	// NB: since the method is on a pointer receiver these are being mutated.
	// Because this runs on every render this shouldn't matter in practice,
	// however.
	i.model.PlaceholderStyle = styles.Placeholder
	i.model.PromptStyle = styles.Prompt
	i.model.Cursor.Style = styles.Cursor
	i.model.Cursor.TextStyle = styles.CursorText
	i.model.TextStyle = styles.Text
	i.model.Prompt = styles.PromptText

	var sb strings.Builder
	if i.title != "" {
		sb.WriteString(styles.Title.Render(i.title))
		sb.WriteString("\n")
	}
	if i.description != "" {
		if styles.DescriptionMaxWidth > 0 {
			desc := strings.Join(splitStringByWidthStripLeftWS(i.description, styles.DescriptionMaxWidth), "\n")
			sb.WriteString(styles.Description.Render(desc))
		} else {
			sb.WriteString(styles.Description.Render(i.description))
		}
		sb.WriteString("\n")
	}
	sb.WriteString(i.model.View())
	sb.WriteString("\n")
	return styles.Base.Render(sb.String())
}

func (i *textInputModel) Update(msg tea.Msg) (inputModel, tea.Cmd) {
	var cmd tea.Cmd
	i.model, cmd = i.model.Update(msg)
	return i, cmd
}

func (i *textInputModel) Init() tea.Cmd {
	return nil
}
