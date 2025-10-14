package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

func nukeMenu() {
	// Initialize the TUI model
	model, err := initNukeTUI(selectedEnvironment)
	if err != nil {
		// Fall back to simple error message if TUI can't initialize
		println("Error initializing nuke interface:", err.Error())
		return
	}

	// Run the TUI
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		println("Error running nuke interface:", err.Error())
	}
}
