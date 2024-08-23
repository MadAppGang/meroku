package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type textFieldModel struct {
	baseInputModel
	focused bool
	invalid bool
}

func newTextFieldModel(i baseInputModel, v inputValue) *textFieldModel {
	i.value = v
	invalid := i.validator != nil && !i.validator.MatchString(v.String())
	return &textFieldModel{
		focused:        false,
		baseInputModel: i,
		invalid:        invalid,
	}
}

func (i *textFieldModel) Blur() {
	i.focused = false
}

func (i *textFieldModel) base() baseInputModel {
	return i.baseInputModel
}

func (i *textFieldModel) Focus() tea.Cmd {
	i.focused = true
	return nil
}

func (i *textFieldModel) value() inputValue {
	return i.baseInputModel.value
}

func (i *textFieldModel) setValue(v inputValue) {
	i.baseInputModel.value = v
	i.invalid = !i.IsValid()
}

func (i *textFieldModel) View() string {
	var styles TextInputThemeOneStyle
	if i.focused {
		styles = baseTextInputTheme.Focused
	} else {
		styles = baseTextInputTheme.Blurred
	}

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
	sb.WriteString(styles.Prompt.Render(styles.PromptText))
	if len(i.value().String()) == 0 {
		sb.WriteString(styles.Placeholder.Render(i.placeholder))
	} else {
		sb.WriteString(styles.Text.Render(i.value().String()))
	}
	if i.invalid {
		sb.WriteString("\n")
		sb.WriteString(styles.ValidationErrorMessage.Render(i.validationMessage))
	}
	sb.WriteString("\n")
	return styles.Base.Render(sb.String())
}

func (i *textFieldModel) Update(msg tea.Msg) (inputModel, tea.Cmd) {
	return i, nil
}

func (i *textFieldModel) Init() tea.Cmd {
	return nil
}
