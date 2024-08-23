package main

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

const (
	listWidth              = 30 // Fixed width for the list panel
	ADD_NEW_SCHEDULED_TASK = "ADD SCHEDULED TASK"
	ADD_NEW_EVENT_TASK     = "ADD EVENT TASK"
	ADD_NEW_PREFIX         = "ADD "
)

var (
	subtle      = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	highlight   = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	brightGreen = lipgloss.Color("#4CAF50")

	titleStyle = lipgloss.NewStyle().
			MarginLeft(2).
			MarginRight(2).
			Padding(0, 1).
			Foreground(lipgloss.Color("#FFF7DB")).
			Background(highlight).
			Bold(true)

	focusedBorderColor   = lipgloss.Color("69")
	unfocusedBorderColor = lipgloss.Color("240")

	listStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(unfocusedBorderColor).
			Padding(0, 0, 1, 2)

	focusedListStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(focusedBorderColor).
				Padding(0, 0, 1, 2)

	detailStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(unfocusedBorderColor).
			Padding(0, 2, 1, 2)

	focusedDetailStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(focusedBorderColor).
				Padding(0, 2, 1, 2)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#FF5F87")).
			Padding(0, 1)

	itemStyle         = lipgloss.NewStyle().PaddingLeft(0)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(0).Foreground(highlight)

	validStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#399918"))

	invalidStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EB3678"))

	helpTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262"))

	dialogListItemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	dialogListSelectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
	dialogListTitleStyle        = lipgloss.NewStyle().MarginLeft(2)
	dialogListPaginationStyle   = list.DefaultStyles().PaginationStyle.PaddingLeft(4)
)
