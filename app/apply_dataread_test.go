package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// applyModelForOps builds the minimum needed to drive the apply hook handlers
// directly. sendMsg is a no-op while program is nil, so no Bubble Tea program
// has to be running.
func applyModelForOps() *modernPlanModel {
	return &modernPlanModel{
		applyState: &applyState{
			currentOps:  map[string]*currentOperation{},
			diagnostics: map[string]*DiagnosticInfo{},
		},
	}
}

func hookMsg(msgType, addr, action string, elapsed float64) *TerraformJSONMessage {
	return &TerraformJSONMessage{
		Type: msgType,
		Hook: &HookInfo{
			Resource:       &ResourceInfo{Addr: addr},
			Action:         action,
			ElapsedSeconds: elapsed,
		},
	}
}

// A data source read must leave the Currently Updating panel when it finishes.
//
// Regression test for a real defect: handleApplyStart adds every address it is
// given, but handleApplyComplete returned early for action=="read" *before* the
// delete, so reads were added and never removed. After a successful apply the
// panel still showed entries like
//
//	Reading module.workloads.data.aws_vpc.selected [148s elapsed]
//
// with a live timer, below a header reading "Apply Complete" and 100% 64/64.
func TestApplyComplete_ClearsDataSourceReads(t *testing.T) {
	m := applyModelForOps()

	reads := []string{
		"module.workloads.data.aws_ssm_parameters_by_path.backend",
		"module.workloads.data.aws_vpc.selected",
	}
	for _, addr := range reads {
		m.handleApplyStart(hookMsg("apply_start", addr, "read", 0))
	}
	if got := len(m.applyState.currentOps); got != len(reads) {
		t.Fatalf("expected %d in-flight reads, got %d", len(reads), got)
	}

	for _, addr := range reads {
		m.handleApplyComplete(hookMsg("apply_complete", addr, "read", 1.5))
	}

	if got := len(m.applyState.currentOps); got != 0 {
		for addr := range m.applyState.currentOps {
			t.Errorf("read left pinned in Currently Updating: %s", addr)
		}
		t.Fatalf("expected no in-flight operations, got %d", got)
	}
}

// Managed resources must still be cleared — the fix reorders the delete and the
// read guard, so this pins the ordinary path too.
func TestApplyComplete_ClearsManagedResources(t *testing.T) {
	m := applyModelForOps()

	m.handleApplyStart(hookMsg("apply_start", "module.workloads.aws_ecs_service.backend", "create", 0))
	m.handleApplyComplete(hookMsg("apply_complete", "module.workloads.aws_ecs_service.backend", "create", 3))

	if got := len(m.applyState.currentOps); got != 0 {
		t.Errorf("expected the created resource to be cleared, got %d in flight", got)
	}
}

// A failed resource must not stay in flight either.
func TestApplyError_ClearsOperation(t *testing.T) {
	m := applyModelForOps()

	m.handleApplyStart(hookMsg("apply_start", "module.workloads.aws_iam_role.backend_task", "create", 0))
	m.handleApplyError(hookMsg("apply_errored", "module.workloads.aws_iam_role.backend_task", "create", 1))

	if got := len(m.applyState.currentOps); got != 0 {
		t.Errorf("expected the failed resource to be cleared, got %d in flight", got)
	}
}

// Reads and managed resources interleave in a real apply; finishing one must not
// disturb the other.
func TestApplyComplete_ReadsAndResourcesInterleave(t *testing.T) {
	m := applyModelForOps()

	m.handleApplyStart(hookMsg("apply_start", "module.workloads.data.aws_vpc.selected", "read", 0))
	m.handleApplyStart(hookMsg("apply_start", "module.workloads.aws_ecs_service.backend", "create", 0))

	// The read finishes first, as it does in practice.
	m.handleApplyComplete(hookMsg("apply_complete", "module.workloads.data.aws_vpc.selected", "read", 1))

	if _, stillThere := m.applyState.currentOps["module.workloads.data.aws_vpc.selected"]; stillThere {
		t.Error("the finished read should be gone")
	}
	if _, running := m.applyState.currentOps["module.workloads.aws_ecs_service.backend"]; !running {
		t.Error("the in-flight resource should be untouched")
	}

	m.handleApplyComplete(hookMsg("apply_complete", "module.workloads.aws_ecs_service.backend", "create", 2))
	if got := len(m.applyState.currentOps); got != 0 {
		t.Errorf("expected nothing in flight, got %d", got)
	}
}

// Belt and braces: whatever leaks, the finished screen must never contradict
// itself by showing in-flight work under an "Apply Complete" header.
func TestApplyComplete_ClearsInFlightOnFinish(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  tea.Msg
	}{
		{"success", applyCompleteMsg{}},
		{"failure", applyErrorMsg{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &modernPlanModel{applyState: &applyState{
				currentOps:  map[string]*currentOperation{"module.workloads.data.aws_vpc.selected": {Address: "module.workloads.data.aws_vpc.selected"}},
				diagnostics: map[string]*DiagnosticInfo{},
			}}

			m.Update(tc.msg)

			if !m.applyState.applyComplete {
				t.Error("expected the apply to be marked complete")
			}
			if got := len(m.applyState.currentOps); got != 0 {
				t.Errorf("finished apply still shows %d operations in flight", got)
			}
		})
	}
}
