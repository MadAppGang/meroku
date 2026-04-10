package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// runMonitorDashboard is the single entry point for the monitor TUI.
// It loads the environment config and launches the tea.Program.
func runMonitorDashboard(envName string) error {
	if envName == "" {
		envName = selectedEnvironment
	}
	if envName == "" {
		return fmt.Errorf("no environment selected; use --env flag or select an environment first")
	}

	env, err := loadEnv(envName)
	if err != nil {
		return fmt.Errorf("failed to load environment %s: %w", envName, err)
	}

	model := initMonitorModel(env, envName)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
}

// initMonitorModel creates the initial monitorModel with sane defaults.
func initMonitorModel(env Env, envName string) monitorModel {
	return monitorModel{
		env:         env,
		envName:     envName,
		currentView: monitorOverviewView,
		loading:     true,
		keys:        defaultMonitorKeyMap(),
	}
}
