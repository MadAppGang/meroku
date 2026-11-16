package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func configWarningsModelForTest() configWarningsModel {
	return newConfigWarningsModel("dev", []ConfigWarning{{
		Code:   "service_no_health_check",
		Item:   WarningItem{Kind: WarningKindService, Name: "magento-bridge"},
		Title:  "Service needs a health check",
		Detail: "Add a health_check_command that exists in the image.",
		Docs:   "https://example.com/health-checks",
	}})
}

func requireConfigWarningsModel(t *testing.T, model tea.Model) configWarningsModel {
	t.Helper()
	got, ok := model.(configWarningsModel)
	if !ok {
		t.Fatalf("Update returned %T, want configWarningsModel", model)
	}
	return got
}

// REQ-FR-7, AC-2: the safe choice is selected before any key is pressed.
func TestConfigWarningsModelDefaultsToCancelAndRendersWarning(t *testing.T) {
	model := configWarningsModelForTest()
	if model.focus != 0 || model.choice != warningChoiceCancel || model.done {
		t.Fatalf("initial model = focus %d, choice %d, done %v; want unfinished Cancel", model.focus, model.choice, model.done)
	}
	view := model.View()
	for _, want := range []string{
		"Cancel and fix", "Proceed anyway", "service", "magento-bridge",
		"Service needs a health check", "Add a health_check_command", "https://example.com/health-checks",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("warning screen does not contain %q:\n%s", want, view)
		}
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := requireConfigWarningsModel(t, updated)
	if !got.done || got.choice != warningChoiceCancel {
		t.Fatalf("Enter on default = done %v, choice %d; want finished Cancel", got.done, got.choice)
	}
}

// REQ-FR-7, AC-2: keyboard navigation reaches the explicit unsafe choice.
func TestConfigWarningsModelRightThenEnterProceeds(t *testing.T) {
	model := configWarningsModelForTest()
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = requireConfigWarningsModel(t, updated)
	if model.focus != 1 {
		t.Fatalf("Right moved focus to %d, want Proceed at index 1", model.focus)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = requireConfigWarningsModel(t, updated)
	if !model.done || model.choice != warningChoiceProceed {
		t.Fatalf("Right then Enter = done %v, choice %d; want finished Proceed", model.done, model.choice)
	}
}

// REQ-FR-7: Escape is the keyboard equivalent of Cancel and fix.
func TestConfigWarningsModelEscapeCancels(t *testing.T) {
	model := configWarningsModelForTest()
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = requireConfigWarningsModel(t, updated)
	if !model.done || model.choice != warningChoiceCancel {
		t.Fatalf("Escape = done %v, choice %d; want finished Cancel", model.done, model.choice)
	}
}
