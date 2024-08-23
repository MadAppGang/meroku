package main

import (
	"log/slog"
	"regexp"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type inputModel interface {
	base() baseInputModel
	value() inputValue
	setValue(inputValue)
	Focus() tea.Cmd
	Blur()
	IsValid() bool

	// tea model
	View() string
	Update(msg tea.Msg) (inputModel, tea.Cmd)
	Init() tea.Cmd
}

type baseInputModel struct {
	title             string
	placeholder       string
	description       string
	value             inputValue
	validator         *regexp.Regexp
	validationMessage string
}

func (s *baseInputModel) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("title", s.title),
		slog.String("placeholder", s.placeholder),
		slog.String("description", s.description),
		slog.Any("value", s.value),
	)
}

func (i *baseInputModel) base() baseInputModel {
	return *i
}

func (i *baseInputModel) Focus() tea.Cmd {
	return textinput.Blink
}

func (i *baseInputModel) Blur() {}
func (i *baseInputModel) View() string {
	if i.value == nil {
		return ""
	}
	return i.value.String()
}

func (i *baseInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return i, nil
}

func (i *baseInputModel) Init() tea.Cmd {
	return nil
}

func (i *baseInputModel) IsValid() bool {
	if i.validator == nil {
		return true
	}
	return i.validator.MatchString(i.value.String())
}

type TextInputFullModel struct {
	textinput.Model
}

func NewTextInputFullModel() TextInputFullModel {
	tip := textinput.New()
	return TextInputFullModel{tip}
}

func (TextInputFullModel) Init() tea.Cmd {
	return nil
}

func (t TextInputFullModel) Update(m tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	t.Model, cmd = t.Model.Update(m)
	return t, cmd
}
