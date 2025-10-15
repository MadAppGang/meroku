package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func nukeMenu() {
	// Initialize the confirmation wizard TUI
	model, err := initNukeTUI(selectedEnvironment)
	if err != nil {
		// Fall back to simple error message if TUI can't initialize
		println("Error initializing nuke interface:", err.Error())
		return
	}

	// Create and run the confirmation wizard
	p := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		println("Error running nuke interface:", err.Error())
		return
	}

	// Check if user confirmed the destruction
	if nukeFinal, ok := finalModel.(*nukeModel); ok {
		if nukeFinal.stage == confirmedStage {
			// User confirmed - launch the dedicated destroy progress TUI
			println(fmt.Sprintf("\n🔥 Proceeding with destruction of environment '%s'...\n", nukeFinal.selectedEnv))

			// Run the same destroy progress TUI used by the regular destroy command
			err := runTerraformDestroyWithProgress(nukeFinal.selectedEnv)
			if err != nil {
				println(fmt.Sprintf("\n❌ Destruction failed: %v\n", err))
				return
			}

			println(fmt.Sprintf("\n✅ Environment '%s' has been destroyed successfully.", nukeFinal.selectedEnv))
			println(fmt.Sprintf("💡 Configuration file '%s.yaml' has been preserved.", nukeFinal.selectedEnv))
			println("   You can redeploy later if needed.\n")
		}
	}

	// User either completed destruction or cancelled - return to main menu
}
