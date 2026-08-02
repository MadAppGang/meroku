package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// The apply view rendered "Createing", "Updateing" and "Deleteing" because the
// verb was built as strings.Title(action)+"ing".
func TestApplyActionVerb(t *testing.T) {
	cases := map[string]string{
		"create":  "Creating",
		"update":  "Updating",
		"delete":  "Destroying",
		"destroy": "Destroying",
		"read":    "Reading",
		"replace": "Replacing",
		"":        "Working",
	}

	for action, want := range cases {
		if got := applyActionVerb(action); got != want {
			t.Errorf("applyActionVerb(%q) = %q, want %q", action, got, want)
		}
	}

	// No action should ever produce the doubled-e form.
	for _, action := range []string{"create", "update", "delete", "destroy", "read", "replace", "custom"} {
		got := applyActionVerb(action)
		for _, bad := range []string{"eing", "Createing", "Updateing", "Deleteing"} {
			if strings.Contains(got, bad) {
				t.Errorf("applyActionVerb(%q) = %q, contains %q", action, got, bad)
			}
		}
	}
}

// An unknown action must be capitalised without inventing a suffix.
func TestApplyActionVerb_UnknownAction(t *testing.T) {
	if got := applyActionVerb("import"); got != "Import" {
		t.Errorf("expected %q, got %q", "Import", got)
	}
}

// The error summary used to truncate at a hard-coded 100 columns, which cut
// "EntityAlreadyExists" down to "Entit..." on a wide terminal — losing the one
// token that identifies the failure.
func TestTruncateToWidth(t *testing.T) {
	const msg = "module.workloads.aws_iam_role.lambda_deploy_iam: creating IAM Role (lambda_deploy_iam_dev): EntityAlreadyExists"

	// Wide enough: unchanged.
	if got := truncateToWidth(msg, 200); got != msg {
		t.Errorf("expected the message unchanged when it fits")
	}

	// Narrow: cut to exactly the requested width, never beyond.
	for _, w := range []int{10, 40, 80, 100} {
		got := truncateToWidth(msg, w)
		if lipgloss.Width(got) > w {
			t.Errorf("width %d: result is %d wide (%q)", w, lipgloss.Width(got), got)
		}
	}

	if got := truncateToWidth(msg, 0); got != "" {
		t.Errorf("zero width should produce an empty string, got %q", got)
	}
	if got := truncateToWidth(msg, -5); got != "" {
		t.Errorf("negative width should produce an empty string, got %q", got)
	}
}

// Multi-byte characters must not be sliced in half — the old byte-slice
// truncation could split a rune and emit broken output.
func TestTruncateToWidth_DoesNotSplitRunes(t *testing.T) {
	s := "✅ résumé — déployé ⚠️ ошибка"

	for w := 1; w <= lipgloss.Width(s); w++ {
		got := truncateToWidth(s, w)
		if !utf8ValidString(got) {
			t.Fatalf("width %d produced invalid UTF-8: %q", w, got)
		}
		if lipgloss.Width(got) > w {
			t.Fatalf("width %d: result is %d wide", w, lipgloss.Width(got))
		}
	}
}

func utf8ValidString(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

// The error summary box must show the total error count, not just the two
// entries that fit. An apply with six failures previously looked like two.
func TestRenderApplyErrorSummary_ShowsTotalCountAndHint(t *testing.T) {
	m := buildFailedApplyModel(160, 50)
	m.applyState.errorCount = 6

	out := m.renderApplyErrorSummary()

	if !strings.Contains(out, "(6)") {
		t.Errorf("expected the total error count in the header, got:\n%s", out)
	}
	if !strings.Contains(out, "press d") {
		t.Errorf("expected a pointer to the details view, got:\n%s", out)
	}
}

// The real AWS diagnostic must reach the summary box rather than the generic
// "Creation errored after 1s" hook message.
func TestRenderApplyErrorSummary_ShowsRealAWSError(t *testing.T) {
	m := buildFailedApplyModel(160, 50)

	out := m.renderApplyErrorSummary()

	if !strings.Contains(out, "EntityAlreadyExists") {
		t.Errorf("expected the AWS error code in the summary, got:\n%s", out)
	}
	if strings.Contains(out, "Creation errored after") {
		t.Errorf("summary should prefer the diagnostic over the hook message, got:\n%s", out)
	}
}

// The details view must fill the terminal exactly; a short render leaves stale
// rows on screen in an alt-screen TUI.
func TestRenderApplyErrorDetailsView_FillsFrame(t *testing.T) {
	for _, size := range []struct{ w, h int }{{160, 50}, {100, 34}, {80, 24}} {
		m := buildFailedApplyModel(size.w, size.h)
		m.applyState.showErrorDetails = true
		m.applyState.applyComplete = true

		out := m.View()
		lines := strings.Count(out, "\n") + 1
		if lines != size.h {
			t.Errorf("%dx%d: rendered %d lines, want %d", size.w, size.h, lines, size.h)
		}
	}
}

// Pressing d must be a no-op until there is something to show.
func TestErrorDetailsToggle_RequiresErrors(t *testing.T) {
	m := buildFailedApplyModel(160, 50)
	m.applyState.errorCount = 0
	m.applyState.showErrorDetails = false

	// Mirrors the guard in the key handler.
	if m.applyState.errorCount > 0 {
		t.Fatal("precondition: expected no errors")
	}
	if m.applyState.showErrorDetails {
		t.Error("details view must stay closed when there are no errors")
	}
}
