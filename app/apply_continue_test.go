package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// newApplyViewModel builds the apply view as it looks once terraform has
// started; complete reports whether terraform has exited.
func newApplyViewModel(complete bool) *modernPlanModel {
	return &modernPlanModel{
		keys:        modernKeys,
		currentView: applyView,
		width:       160,
		height:      50,
		applyState: &applyState{
			isApplying:    !complete,
			applyComplete: complete,
			diagnostics:   map[string]*DiagnosticInfo{},
		},
	}
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// The footer advertises "[Enter] Continue" once apply finishes, but the Enter
// handler only knew the dashboard and diff views, so the key was dropped and
// the operator was left on the Apply Complete screen with no way back to the
// main menu. Continue means leaving the viewer: showModernTerraformPlanTUI
// returns to the menu when the program quits.
func TestApplyComplete_EnterReturnsToMenu(t *testing.T) {
	for _, hasErrors := range []bool{false, true} {
		m := newApplyViewModel(true)
		m.applyState.hasErrors = hasErrors

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

		if !isQuit(cmd) {
			t.Errorf("hasErrors=%v: Enter on Apply Complete must quit the viewer", hasErrors)
		}
	}
}

// Enter is not advertised while terraform is still running, and must not end
// the viewer under a live apply.
func TestApplyInProgress_EnterDoesNotQuit(t *testing.T) {
	m := newApplyViewModel(false)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if isQuit(cmd) {
		t.Error("Enter during a running apply must not quit the viewer")
	}
	if m.currentView != applyView {
		t.Errorf("Enter during a running apply must stay on the apply view, got view %v", m.currentView)
	}
}
