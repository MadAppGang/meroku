package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/exp/slog"
)

type boolFieldModel struct {
	baseInputModel
	focused bool
}

func newBoolFieldModel(i baseInputModel, v inputValue) *boolFieldModel {
	i.value = v
	return &boolFieldModel{
		focused:        false,
		baseInputModel: i,
	}
}

func (i *boolFieldModel) Blur() {
	i.focused = false
}

func (i *boolFieldModel) base() baseInputModel {
	return i.baseInputModel
}

func (i *boolFieldModel) Focus() tea.Cmd {
	i.focused = true
	return nil
}

func (i *boolFieldModel) value() inputValue {
	return i.baseInputModel.value
}

func (i *boolFieldModel) setValue(v inputValue) {
	i.baseInputModel.value = v
}

func (i *boolFieldModel) View() string {
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
	if i.value().Bool() {
		sb.WriteString(styles.BoolTrue.Render(styles.BoolTrueText))
	} else {
		sb.WriteString(styles.BoolFalse.Render(styles.BoolFalseText))
	}
	sb.WriteString("\n")

	return styles.Base.Render(sb.String())
}

func (i *boolFieldModel) Update(msg tea.Msg) (inputModel, tea.Cmd) {
	return i, nil
}

func (i *boolFieldModel) Init() tea.Cmd {
	return nil
}

type boolInputModel struct {
	boolFieldModel
	width int
}

func newBoolInputModel(v baseInputModel, width int) boolInputModel {
	return boolInputModel{
		boolFieldModel: boolFieldModel{baseInputModel: v, focused: false},
		width:          width,
	}
}

func (i boolInputModel) View() string {
	var styles TextInputThemeOneStyle
	if i.focused {
		styles = baseTextInputTheme.Focused
	} else {
		styles = baseTextInputTheme.Blurred
	}

	var sb strings.Builder
	if i.value().Bool() {
		slog.Warn("boolInputModel.View [TRUE]")
		sb.WriteString(styles.BoolTrue.Render(styles.BoolTrueText))
	} else {
		slog.Warn("boolInputModel.View [FALSE]")
		sb.WriteString(styles.BoolFalse.Render(styles.BoolFalseText))
	}
	sb.WriteString("\n")

	return lipgloss.NewStyle().Width(i.width).AlignHorizontal(lipgloss.Center).Render(sb.String())
}

func (i boolInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	slog.Warn("boolInputModel.Update", "msg", fmt.Sprintf("%+v", msg))
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case " ":
			newVal := !i.value().Bool()
			slog.Warn("boolInputModel.Update >> SPACE", slog.Bool("newVal", newVal))
			i.baseInputModel.value = boolValue{newVal}
			return i, nil
		}
	}
	return i, nil
}

func (i boolInputModel) Init() tea.Cmd {
	return nil
}

func (i boolInputModel) Value() inputValue {
	return i.baseInputModel.value
}

func (i boolInputModel) SetValue(v inputValue) {
	i.baseInputModel.value = v
}
