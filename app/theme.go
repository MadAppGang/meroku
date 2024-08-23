package main

import "github.com/charmbracelet/lipgloss"

type TextInputThemeOneStyle struct {
	Base                   lipgloss.Style
	Placeholder            lipgloss.Style
	Description            lipgloss.Style
	DescriptionMaxWidth    int
	Help                   lipgloss.Style
	ValidationMessage      lipgloss.Style
	ValidationErrorMessage lipgloss.Style
	Prompt                 lipgloss.Style
	PromptText             string
	Cursor                 lipgloss.Style
	CursorText             lipgloss.Style
	Text                   lipgloss.Style
	Title                  lipgloss.Style
}

type TextInputTheme struct {
	Blurred TextInputThemeOneStyle
	Focused TextInputThemeOneStyle
}

var (
	normalFg   = lipgloss.AdaptiveColor{Light: "235", Dark: "252"}
	indigo     = lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7571F9"}
	indigoDark = lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#4A47A3"}
	cream      = lipgloss.AdaptiveColor{Light: "#FFFDF5", Dark: "#FFFDF5"}
	fuchsia    = lipgloss.Color("#F780E2")
	green      = lipgloss.AdaptiveColor{Light: "#02BA84", Dark: "#02BF87"}
	red        = lipgloss.AdaptiveColor{Light: "#FF4672", Dark: "#ED567A"}
	darkRed    = lipgloss.AdaptiveColor{Light: "#FF4672", Dark: "88"}
	lightGray  = lipgloss.AdaptiveColor{Light: "", Dark: "237"}
)

var baseTextInputTheme = TextInputTheme{
	Blurred: TextInputThemeOneStyle{
		Base:                   lipgloss.NewStyle().PaddingLeft(1).BorderStyle(lipgloss.ThickBorder()).BorderLeft(true).BorderForeground(lipgloss.Color("238")),
		Placeholder:            lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		Prompt:                 lipgloss.NewStyle().Foreground(lightGray),
		Description:            lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "", Dark: "237"}),
		DescriptionMaxWidth:    40,
		Help:                   lipgloss.NewStyle().Foreground(cream),
		ValidationMessage:      lipgloss.NewStyle().Foreground(green),
		ValidationErrorMessage: lipgloss.NewStyle().Foreground(darkRed),
		Title:                  lipgloss.NewStyle().Foreground(indigoDark).Bold(false),
		PromptText:             "> ",
	},
	Focused: TextInputThemeOneStyle{
		Base:                   lipgloss.NewStyle().PaddingLeft(1).BorderStyle(lipgloss.ThickBorder()).BorderLeft(true).BorderForeground(green),
		Placeholder:            lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		Prompt:                 lipgloss.NewStyle().Foreground(green),
		Cursor:                 lipgloss.NewStyle().Foreground(green),
		Description:            lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "", Dark: "243"}),
		DescriptionMaxWidth:    40,
		Help:                   lipgloss.NewStyle().Foreground(cream),
		ValidationMessage:      lipgloss.NewStyle().Foreground(green),
		ValidationErrorMessage: lipgloss.NewStyle().Foreground(red),
		Title:                  lipgloss.NewStyle().Foreground(indigo).Bold(true),
		PromptText:             "👉 ",
		Text:                   lipgloss.NewStyle().Bold(true),
	},
}
