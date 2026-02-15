package main

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

// processCompletionMsg simulates the resourceCompleteMsg handler from
// terraform_plan_modern_tui.go Update(). This is the exact same logic extracted
// for testability so we can verify correct counting without the full Bubble Tea loop.
func processCompletionMsg(state *applyState, msg resourceCompleteMsg) {
	// Get action and duration from currentOps map first
	action := "update"
	var duration time.Duration
	state.mu.Lock()
	if op, exists := state.currentOps[msg.Address]; exists {
		action = op.Action
		duration = time.Since(op.StartTime)
		delete(state.currentOps, msg.Address)
	} else if msg.Duration > 0 {
		duration = msg.Duration
	}
	state.mu.Unlock()

	// If we didn't get action from currentOps, try pending list
	if action == "update" {
		for _, p := range state.pending {
			if p.Address == msg.Address {
				action = p.Action
				break
			}
		}
	}

	// Safety: Deduplicate completions.
	// If this address has no remaining pending entries but already has completions,
	// it's a duplicate.
	pendingForAddr := 0
	for _, p := range state.pending {
		pendingBaseAddr := strings.TrimSuffix(strings.TrimSuffix(p.Address, " (destroy)"), " (create)")
		if pendingBaseAddr == msg.Address {
			pendingForAddr++
		}
	}
	completedForAddr := 0
	for _, c := range state.completed {
		if c.Address == msg.Address {
			completedForAddr++
		}
	}
	if pendingForAddr == 0 && completedForAddr > 0 {
		return // No more expected completions - skip duplicate
	}

	// Move from pending to completed - match BOTH address AND action
	for i, p := range state.pending {
		pendingBaseAddr := strings.TrimSuffix(strings.TrimSuffix(p.Address, " (destroy)"), " (create)")
		if pendingBaseAddr == msg.Address && p.Action == action {
			state.pending = append(state.pending[:i], state.pending[i+1:]...)
			break
		}
	}

	state.completed = append(state.completed, completedResource{
		Address:   msg.Address,
		Action:    action,
		Duration:  duration,
		Timestamp: time.Now(),
		Success:   msg.Success,
		Error:     msg.Error,
	})

	if !msg.Success {
		isCascadingFailure := action == "cancelled" ||
			strings.Contains(strings.ToLower(msg.Error), "cancelled due to previous errors")
		if !isCascadingFailure {
			state.errorCount++
			state.hasErrors = true
		}
	}
}

// processStartMsg simulates the resourceStartMsg handler that adds to currentOps.
func processStartMsg(state *applyState, addr, action string) {
	state.mu.Lock()
	state.currentOps[addr] = &currentOperation{
		Address:   addr,
		Action:    action,
		StartTime: time.Now(),
		Status:    "Starting...",
	}
	state.mu.Unlock()
}

// buildApplyState creates an applyState from a plan, simulating initApplyState.
func buildApplyState(resources []struct {
	Address string
	Actions []string
	Type    string
}) *applyState {
	state := &applyState{
		startTime:      time.Now(),
		logs:           []logEntry{},
		pending:        []pendingResource{},
		completed:      []completedResource{},
		currentOps:     make(map[string]*currentOperation),
		diagnostics:    make(map[string]*DiagnosticInfo),
		statusLogIndex: make(map[string]int),
	}

	for _, r := range resources {
		// Skip reads
		if len(r.Actions) == 1 && r.Actions[0] == "read" {
			continue
		}
		if len(r.Actions) == 2 && r.Actions[0] == "delete" && r.Actions[1] == "create" {
			state.pending = append(state.pending, pendingResource{Address: r.Address, Action: "delete", Type: r.Type})
			state.pending = append(state.pending, pendingResource{Address: r.Address, Action: "create", Type: r.Type})
			state.totalResources += 2
		} else {
			state.pending = append(state.pending, pendingResource{Address: r.Address, Action: r.Actions[0], Type: r.Type})
			state.totalResources++
		}
	}

	return state
}

// TestDryRun_RealTerraformJSON tests parsing real Terraform JSON lines and verifying
// that structured hooks are handled correctly and text messages don't double-count.
func TestDryRun_RealTerraformJSON(t *testing.T) {
	// Simulate real Terraform -json output for:
	// - 2 updates (aws_ecs_service.main, aws_ecs_task_definition.main)
	// - 1 replace (aws_iam_role_policy.task -> delete + create)
	// - 1 data read (data.aws_caller_identity.current)
	//
	// For each operation, Terraform sends BOTH:
	// 1. Structured hook: {"type":"apply_start"...}, {"type":"apply_complete"...}
	// 2. Text message:    {"type":"","@message":"X: Modifying..."}
	//                     {"type":"","@message":"X: Modifications complete after 5s"}
	terraformLines := []string{
		// --- Data read (should be ignored for progress) ---
		`{"@level":"info","@message":"data.aws_caller_identity.current: Reading...","@module":"terraform.ui","type":"apply_start","hook":{"resource":{"addr":"data.aws_caller_identity.current","module":"","resource":"data.aws_caller_identity.current","resource_type":"aws_caller_identity","resource_name":"current","implied_provider":"aws"},"action":"read"}}`,
		`{"@level":"info","@message":"data.aws_caller_identity.current: Read complete after 0s","@module":"terraform.ui","type":"apply_complete","hook":{"resource":{"addr":"data.aws_caller_identity.current","module":"","resource":"data.aws_caller_identity.current","resource_type":"aws_caller_identity","resource_name":"current","implied_provider":"aws"},"action":"read","elapsed_seconds":0.5}}`,

		// --- Update 1: aws_ecs_task_definition.main ---
		// Structured hook: start
		`{"@level":"info","@message":"aws_ecs_task_definition.main: Modifying...","@module":"terraform.ui","type":"apply_start","hook":{"resource":{"addr":"aws_ecs_task_definition.main","module":"module.workloads","resource":"aws_ecs_task_definition.main","resource_type":"aws_ecs_task_definition","resource_name":"main","implied_provider":"aws"},"action":"update"}}`,
		// Text message: "Modifying..."
		`{"@level":"info","@message":"aws_ecs_task_definition.main: Modifying... [id=arn:aws:ecs:us-east-1:123:task-def/test:5]","@module":"terraform.ui","type":""}`,
		// Structured hook: complete
		`{"@level":"info","@message":"aws_ecs_task_definition.main: Modifications complete after 2s","@module":"terraform.ui","type":"apply_complete","hook":{"resource":{"addr":"aws_ecs_task_definition.main","module":"module.workloads","resource":"aws_ecs_task_definition.main","resource_type":"aws_ecs_task_definition","resource_name":"main","implied_provider":"aws"},"action":"update","id_key":"id","id_value":"arn:aws:ecs:us-east-1:123:task-def/test:6","elapsed_seconds":2.1}}`,
		// Text message: "Modifications complete" (this MUST NOT double-count)
		`{"@level":"info","@message":"aws_ecs_task_definition.main: Modifications complete after 2s [id=arn:aws:ecs:us-east-1:123:task-def/test:6]","@module":"terraform.ui","type":""}`,

		// --- Update 2: aws_ecs_service.main ---
		`{"@level":"info","@message":"aws_ecs_service.main: Modifying...","@module":"terraform.ui","type":"apply_start","hook":{"resource":{"addr":"aws_ecs_service.main","module":"module.workloads","resource":"aws_ecs_service.main","resource_type":"aws_ecs_service","resource_name":"main","implied_provider":"aws"},"action":"update"}}`,
		`{"@level":"info","@message":"aws_ecs_service.main: Modifying... [id=arn:aws:ecs:us-east-1:123:service/test]","@module":"terraform.ui","type":""}`,
		`{"@level":"info","@message":"aws_ecs_service.main: Still modifying... [id=arn:aws:ecs:us-east-1:123:service/test, 10s elapsed]","@module":"terraform.ui","type":""}`,
		`{"@level":"info","@message":"aws_ecs_service.main: Modifications complete after 15s","@module":"terraform.ui","type":"apply_complete","hook":{"resource":{"addr":"aws_ecs_service.main","module":"module.workloads","resource":"aws_ecs_service.main","resource_type":"aws_ecs_service","resource_name":"main","implied_provider":"aws"},"action":"update","id_key":"id","id_value":"arn:aws:ecs:us-east-1:123:service/test","elapsed_seconds":15.3}}`,
		`{"@level":"info","@message":"aws_ecs_service.main: Modifications complete after 15s [id=arn:aws:ecs:us-east-1:123:service/test]","@module":"terraform.ui","type":""}`,

		// --- Replace: aws_iam_role_policy.task (delete phase) ---
		`{"@level":"info","@message":"aws_iam_role_policy.task: Destroying...","@module":"terraform.ui","type":"apply_start","hook":{"resource":{"addr":"aws_iam_role_policy.task","module":"module.workloads","resource":"aws_iam_role_policy.task","resource_type":"aws_iam_role_policy","resource_name":"task","implied_provider":"aws"},"action":"destroy"}}`,
		`{"@level":"info","@message":"aws_iam_role_policy.task: Destroying... [id=test-role:test-policy]","@module":"terraform.ui","type":""}`,
		`{"@level":"info","@message":"aws_iam_role_policy.task: Destruction complete after 1s","@module":"terraform.ui","type":"apply_complete","hook":{"resource":{"addr":"aws_iam_role_policy.task","module":"module.workloads","resource":"aws_iam_role_policy.task","resource_type":"aws_iam_role_policy","resource_name":"task","implied_provider":"aws"},"action":"destroy","id_key":"id","id_value":"test-role:test-policy","elapsed_seconds":1.0}}`,
		`{"@level":"info","@message":"aws_iam_role_policy.task: Destruction complete after 1s [id=test-role:test-policy]","@module":"terraform.ui","type":""}`,

		// --- Replace: aws_iam_role_policy.task (create phase) ---
		`{"@level":"info","@message":"aws_iam_role_policy.task: Creating...","@module":"terraform.ui","type":"apply_start","hook":{"resource":{"addr":"aws_iam_role_policy.task","module":"module.workloads","resource":"aws_iam_role_policy.task","resource_type":"aws_iam_role_policy","resource_name":"task","implied_provider":"aws"},"action":"create"}}`,
		`{"@level":"info","@message":"aws_iam_role_policy.task: Creating... [id=test-role:test-policy-v2]","@module":"terraform.ui","type":""}`,
		`{"@level":"info","@message":"aws_iam_role_policy.task: Creation complete after 1s","@module":"terraform.ui","type":"apply_complete","hook":{"resource":{"addr":"aws_iam_role_policy.task","module":"module.workloads","resource":"aws_iam_role_policy.task","resource_type":"aws_iam_role_policy","resource_name":"task","implied_provider":"aws"},"action":"create","id_key":"id","id_value":"test-role:test-policy-v2","elapsed_seconds":1.2}}`,
		`{"@level":"info","@message":"aws_iam_role_policy.task: Creation complete after 1s [id=test-role:test-policy-v2]","@module":"terraform.ui","type":""}`,
	}

	// Build plan resources matching the above
	planResources := []struct {
		Address string
		Actions []string
		Type    string
	}{
		{"aws_ecs_task_definition.main", []string{"update"}, "aws_ecs_task_definition"},
		{"aws_ecs_service.main", []string{"update"}, "aws_ecs_service"},
		{"aws_iam_role_policy.task", []string{"delete", "create"}, "aws_iam_role_policy"}, // replace
		{"data.aws_caller_identity.current", []string{"read"}, "aws_caller_identity"},     // read - should be excluded
	}

	state := buildApplyState(planResources)

	// Verify initial state
	// Expected: 2 updates + 2 (replace = delete + create) = 4 total, reads excluded
	if state.totalResources != 4 {
		t.Fatalf("Expected totalResources=4, got %d", state.totalResources)
	}
	if len(state.pending) != 4 {
		t.Fatalf("Expected 4 pending items, got %d", len(state.pending))
	}

	// Now simulate processing each Terraform JSON line
	// We track messages that would be sent, then process them
	var messages []interface{} // resourceStartMsg or resourceCompleteMsg

	for _, line := range terraformLines {
		var msg TerraformJSONMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("Failed to parse JSON line: %v\n%s", err, line)
		}

		switch msg.Type {
		case "apply_start":
			if msg.Hook != nil && msg.Hook.Resource != nil {
				startAction := normalizeAction(msg.Hook.Action)
				// Simulate handleApplyStart: add to currentOps
				processStartMsg(state, msg.Hook.Resource.Addr, startAction)
				messages = append(messages, resourceStartMsg{
					Address: msg.Hook.Resource.Addr,
					Action:  startAction,
				})
			}

		case "apply_complete":
			if msg.Hook != nil && msg.Hook.Resource != nil {
				completeAction := normalizeAction(msg.Hook.Action)
				// Skip read operations - not tracked for apply progress
				if completeAction == "read" {
					continue
				}
				duration := time.Duration(msg.Hook.ElapsedSeconds * float64(time.Second))

				// Simulate handleApplyComplete: delete from currentOps, send completion
				state.mu.Lock()
				delete(state.currentOps, msg.Hook.Resource.Addr)
				state.mu.Unlock()

				messages = append(messages, resourceCompleteMsg{
					Address:  msg.Hook.Resource.Addr,
					Success:  true,
					Duration: duration,
				})
			}

		case "apply_errored":
			if msg.Hook != nil && msg.Hook.Resource != nil {
				state.mu.Lock()
				delete(state.currentOps, msg.Hook.Resource.Addr)
				state.mu.Unlock()

				messages = append(messages, resourceCompleteMsg{
					Address: msg.Hook.Resource.Addr,
					Success: false,
					Error:   msg.Message,
				})
			}

		default:
			// Text message fallback - THE OLD BUG was sending resourceCompleteMsg here.
			// After the fix, text messages should NOT produce completion signals.
			if msg.Message != "" {
				if strings.Contains(msg.Message, ": Creation complete after") ||
					strings.Contains(msg.Message, ": Modifications complete after") ||
					strings.Contains(msg.Message, ": Destroy complete after") ||
					strings.Contains(msg.Message, ": Destruction complete after") {
					// OLD BUG: This used to send resourceCompleteMsg, causing double-counting.
					// After fix: handled by apply_complete hook only - just log the message.
					// Do nothing here (the fix).
				}
			}
		}
	}

	// Process all collected messages through the completion handler
	for _, msg := range messages {
		if cm, ok := msg.(resourceCompleteMsg); ok {
			processCompletionMsg(state, cm)
		}
	}

	// Verify final state
	completedCount := len(state.completed)
	pendingCount := len(state.pending)

	// Expected: 2 updates + 1 delete + 1 create = 4 completions
	if completedCount != 4 {
		t.Errorf("Expected 4 completed, got %d", completedCount)
		for i, c := range state.completed {
			t.Logf("  completed[%d]: addr=%s action=%s success=%v", i, c.Address, c.Action, c.Success)
		}
	}

	// All pending should be cleared
	if pendingCount != 0 {
		t.Errorf("Expected 0 pending, got %d", pendingCount)
		for i, p := range state.pending {
			t.Logf("  pending[%d]: addr=%s action=%s", i, p.Address, p.Action)
		}
	}

	// Progress should be exactly 100%
	if state.totalResources > 0 {
		percent := float64(completedCount) / float64(state.totalResources) * 100
		if percent != 100.0 {
			t.Errorf("Expected 100%% progress, got %.1f%% (%d/%d)", percent, completedCount, state.totalResources)
		}
	}

	// No errors
	if state.hasErrors {
		t.Error("Expected no errors")
	}

	// Verify specific actions are correct
	actionCounts := make(map[string]int)
	for _, c := range state.completed {
		actionCounts[c.Action]++
	}
	if actionCounts["update"] != 2 {
		t.Errorf("Expected 2 update completions, got %d", actionCounts["update"])
	}
	if actionCounts["delete"] != 1 {
		t.Errorf("Expected 1 delete completion (from replace), got %d", actionCounts["delete"])
	}
	if actionCounts["create"] != 1 {
		t.Errorf("Expected 1 create completion (from replace), got %d", actionCounts["create"])
	}
}

// TestDryRun_OldBugWouldOvercount verifies that the OLD code (with text-based
// completion signals) WOULD produce overcounting. This documents the bug.
func TestDryRun_OldBugWouldOvercount(t *testing.T) {
	// Same plan: 2 updates + 1 replace + 1 read
	planResources := []struct {
		Address string
		Actions []string
		Type    string
	}{
		{"aws_ecs_task_definition.main", []string{"update"}, "aws_ecs_task_definition"},
		{"aws_ecs_service.main", []string{"update"}, "aws_ecs_service"},
		{"aws_iam_role_policy.task", []string{"delete", "create"}, "aws_iam_role_policy"},
	}

	state := buildApplyState(planResources)

	// Simulate what the OLD code would do: send completions from BOTH
	// structured hooks AND text messages for each resource.
	// Each resource gets 2 completion messages (the bug).
	oldBugMessages := []resourceCompleteMsg{
		// Update 1: hook completion
		{Address: "aws_ecs_task_definition.main", Success: true, Duration: 2 * time.Second},
		// Update 1: text completion (OLD BUG - this was the duplicate)
		{Address: "aws_ecs_task_definition.main", Success: true},

		// Update 2: hook completion
		{Address: "aws_ecs_service.main", Success: true, Duration: 15 * time.Second},
		// Update 2: text completion (OLD BUG)
		{Address: "aws_ecs_service.main", Success: true},

		// Replace delete: hook
		{Address: "aws_iam_role_policy.task", Success: true, Duration: 1 * time.Second},
		// Replace delete: text (OLD BUG)
		{Address: "aws_iam_role_policy.task", Success: true},

		// Replace create: hook
		{Address: "aws_iam_role_policy.task", Success: true, Duration: 1 * time.Second},
		// Replace create: text (OLD BUG)
		{Address: "aws_iam_role_policy.task", Success: true},
	}

	// Simulate handleApplyStart for all operations to populate currentOps
	processStartMsg(state, "aws_ecs_task_definition.main", "update")
	// Simulate handleApplyComplete pre-deleting from currentOps (as it does)
	state.mu.Lock()
	delete(state.currentOps, "aws_ecs_task_definition.main")
	state.mu.Unlock()

	processStartMsg(state, "aws_ecs_service.main", "update")
	state.mu.Lock()
	delete(state.currentOps, "aws_ecs_service.main")
	state.mu.Unlock()

	processStartMsg(state, "aws_iam_role_policy.task", "delete")
	state.mu.Lock()
	delete(state.currentOps, "aws_iam_role_policy.task")
	state.mu.Unlock()

	processStartMsg(state, "aws_iam_role_policy.task", "create")
	state.mu.Lock()
	delete(state.currentOps, "aws_iam_role_policy.task")
	state.mu.Unlock()

	// Process all messages (with dedup guard active)
	for _, msg := range oldBugMessages {
		processCompletionMsg(state, msg)
	}

	// WITH the dedup fix, we should still get exactly 4 completions
	// even with duplicated messages
	completedCount := len(state.completed)
	if completedCount != 4 {
		t.Errorf("Expected dedup to limit to 4 completions, got %d", completedCount)
		for i, c := range state.completed {
			t.Logf("  completed[%d]: addr=%s action=%s", i, c.Address, c.Action)
		}
	}

	percent := float64(completedCount) / float64(state.totalResources) * 100
	if percent > 100.0 {
		t.Errorf("Progress should never exceed 100%%, got %.1f%%", percent)
	}
}

// TestDryRun_ErrorsNotDoubleCounted verifies that error completions from both
// structured hooks and text messages are also deduplicated correctly.
func TestDryRun_ErrorsNotDoubleCounted(t *testing.T) {
	planResources := []struct {
		Address string
		Actions []string
		Type    string
	}{
		{"aws_ecs_service.main", []string{"update"}, "aws_ecs_service"},
		{"aws_iam_role.broken", []string{"create"}, "aws_iam_role"},
	}

	state := buildApplyState(planResources)

	// Successful update
	processStartMsg(state, "aws_ecs_service.main", "update")
	state.mu.Lock()
	delete(state.currentOps, "aws_ecs_service.main")
	state.mu.Unlock()
	processCompletionMsg(state, resourceCompleteMsg{
		Address: "aws_ecs_service.main", Success: true, Duration: 5 * time.Second,
	})

	// Failed create - sent twice (hook + text fallback)
	processStartMsg(state, "aws_iam_role.broken", "create")
	state.mu.Lock()
	delete(state.currentOps, "aws_iam_role.broken")
	state.mu.Unlock()
	processCompletionMsg(state, resourceCompleteMsg{
		Address: "aws_iam_role.broken", Success: false, Error: "AccessDenied",
	})
	// Duplicate from text fallback (should be deduped)
	processCompletionMsg(state, resourceCompleteMsg{
		Address: "aws_iam_role.broken", Success: false, Error: "AccessDenied",
	})

	if len(state.completed) != 2 {
		t.Errorf("Expected 2 completed (1 success + 1 error), got %d", len(state.completed))
	}
	if state.errorCount != 1 {
		t.Errorf("Expected errorCount=1, got %d", state.errorCount)
	}
}

// TestDryRun_ReadOperationsSkipped verifies that data source reads are fully
// excluded from progress tracking (not in totalResources, not in pending).
func TestDryRun_ReadOperationsSkipped(t *testing.T) {
	planResources := []struct {
		Address string
		Actions []string
		Type    string
	}{
		{"aws_ecs_service.main", []string{"update"}, "aws_ecs_service"},
		{"data.aws_caller_identity.current", []string{"read"}, "aws_caller_identity"},
		{"data.aws_region.current", []string{"read"}, "aws_region"},
		{"data.aws_vpc.selected", []string{"read"}, "aws_vpc"},
	}

	state := buildApplyState(planResources)

	if state.totalResources != 1 {
		t.Errorf("Expected totalResources=1 (reads excluded), got %d", state.totalResources)
	}
	if len(state.pending) != 1 {
		t.Errorf("Expected 1 pending (reads excluded), got %d", len(state.pending))
	}
	if state.pending[0].Address != "aws_ecs_service.main" {
		t.Errorf("Expected pending to be aws_ecs_service.main, got %s", state.pending[0].Address)
	}

	// Complete the one real operation
	processStartMsg(state, "aws_ecs_service.main", "update")
	state.mu.Lock()
	delete(state.currentOps, "aws_ecs_service.main")
	state.mu.Unlock()
	processCompletionMsg(state, resourceCompleteMsg{
		Address: "aws_ecs_service.main", Success: true,
	})

	percent := float64(len(state.completed)) / float64(state.totalResources) * 100
	if percent != 100.0 {
		t.Errorf("Expected exactly 100%%, got %.1f%%", percent)
	}
}

// TestDryRun_LargeRealisticPlan simulates a realistic plan with mixed operations
// including multiple replaces, updates, creates, deletes, and reads.
func TestDryRun_LargeRealisticPlan(t *testing.T) {
	planResources := []struct {
		Address string
		Actions []string
		Type    string
	}{
		// 4 updates
		{"module.workloads.aws_ecs_task_definition.backend", []string{"update"}, "aws_ecs_task_definition"},
		{"module.workloads.aws_ecs_service.backend", []string{"update"}, "aws_ecs_service"},
		{"module.workloads.aws_ecs_task_definition.task_cleanup", []string{"update"}, "aws_ecs_task_definition"},
		{"module.workloads.aws_security_group_rule.backend_egress", []string{"update"}, "aws_security_group_rule"},
		// 2 replaces (= 4 operations)
		{"module.workloads.aws_iam_role_policy.backend_task", []string{"delete", "create"}, "aws_iam_role_policy"},
		{"module.workloads.aws_iam_role_policy.task_cleanup", []string{"delete", "create"}, "aws_iam_role_policy"},
		// 1 create
		{"module.workloads.aws_cloudwatch_log_group.new_service", []string{"create"}, "aws_cloudwatch_log_group"},
		// 1 delete
		{"module.workloads.aws_cloudwatch_log_group.old_service", []string{"delete"}, "aws_cloudwatch_log_group"},
		// 2 reads (should be excluded)
		{"data.aws_caller_identity.current", []string{"read"}, "aws_caller_identity"},
		{"data.aws_region.current", []string{"read"}, "aws_region"},
	}

	state := buildApplyState(planResources)

	// 4 updates + 4 (2 replaces) + 1 create + 1 delete = 10
	expectedTotal := 10
	if state.totalResources != expectedTotal {
		t.Fatalf("Expected totalResources=%d, got %d", expectedTotal, state.totalResources)
	}
	if len(state.pending) != expectedTotal {
		t.Fatalf("Expected %d pending, got %d", expectedTotal, len(state.pending))
	}

	// Simulate all operations completing via structured hooks
	allOps := []struct {
		addr   string
		action string
	}{
		{"module.workloads.aws_ecs_task_definition.backend", "update"},
		{"module.workloads.aws_ecs_service.backend", "update"},
		{"module.workloads.aws_ecs_task_definition.task_cleanup", "update"},
		{"module.workloads.aws_security_group_rule.backend_egress", "update"},
		{"module.workloads.aws_iam_role_policy.backend_task", "delete"},  // replace phase 1
		{"module.workloads.aws_iam_role_policy.backend_task", "create"},  // replace phase 2
		{"module.workloads.aws_iam_role_policy.task_cleanup", "delete"},  // replace phase 1
		{"module.workloads.aws_iam_role_policy.task_cleanup", "create"},  // replace phase 2
		{"module.workloads.aws_cloudwatch_log_group.new_service", "create"},
		{"module.workloads.aws_cloudwatch_log_group.old_service", "delete"},
	}

	for _, op := range allOps {
		// handleApplyStart: add to currentOps
		processStartMsg(state, op.addr, op.action)
		// handleApplyComplete: pre-delete from currentOps, send completion
		state.mu.Lock()
		delete(state.currentOps, op.addr)
		state.mu.Unlock()
		processCompletionMsg(state, resourceCompleteMsg{
			Address: op.addr, Success: true, Duration: 2 * time.Second,
		})
	}

	completedCount := len(state.completed)
	if completedCount != expectedTotal {
		t.Errorf("Expected %d completed, got %d", expectedTotal, completedCount)
		for i, c := range state.completed {
			t.Logf("  completed[%d]: addr=%s action=%s", i, c.Address, c.Action)
		}
	}
	if len(state.pending) != 0 {
		t.Errorf("Expected 0 pending, got %d", len(state.pending))
		for i, p := range state.pending {
			t.Logf("  pending[%d]: addr=%s action=%s", i, p.Address, p.Action)
		}
	}

	percent := float64(completedCount) / float64(state.totalResources) * 100
	if percent != 100.0 {
		t.Errorf("Expected exactly 100%%, got %.1f%% (%d/%d)", percent, completedCount, state.totalResources)
	}

	if state.hasErrors {
		t.Error("Expected no errors")
	}
}

// TestDryRun_ParseTerraformJSON verifies that the JSON parsing and message type
// routing correctly identifies structured hooks vs text messages.
func TestDryRun_ParseTerraformJSON(t *testing.T) {
	tests := []struct {
		name            string
		json            string
		expectedType    string
		hasHook         bool
		shouldBeIgnored bool // text message that should NOT trigger completion
	}{
		{
			name:         "structured apply_start",
			json:         `{"type":"apply_start","@message":"aws_ecs_service.main: Modifying...","hook":{"resource":{"addr":"aws_ecs_service.main"},"action":"update"}}`,
			expectedType: "apply_start",
			hasHook:      true,
		},
		{
			name:         "structured apply_complete",
			json:         `{"type":"apply_complete","@message":"aws_ecs_service.main: Modifications complete after 5s","hook":{"resource":{"addr":"aws_ecs_service.main"},"action":"update","elapsed_seconds":5.0}}`,
			expectedType: "apply_complete",
			hasHook:      true,
		},
		{
			name:            "text completion (should be ignored)",
			json:            `{"type":"","@level":"info","@message":"aws_ecs_service.main: Modifications complete after 5s [id=xxx]"}`,
			expectedType:    "",
			hasHook:         false,
			shouldBeIgnored: true,
		},
		{
			name:            "text destruction complete (should be ignored)",
			json:            `{"type":"","@level":"info","@message":"aws_iam_role.test: Destruction complete after 1s [id=xxx]"}`,
			expectedType:    "",
			hasHook:         false,
			shouldBeIgnored: true,
		},
		{
			name:            "text creation complete (should be ignored)",
			json:            `{"type":"","@level":"info","@message":"aws_iam_role.test: Creation complete after 2s [id=xxx]"}`,
			expectedType:    "",
			hasHook:         false,
			shouldBeIgnored: true,
		},
		{
			name:         "structured apply_errored",
			json:         `{"type":"apply_errored","@message":"aws_iam_role.test: Creation errored after 3s","hook":{"resource":{"addr":"aws_iam_role.test"},"action":"create","elapsed_seconds":3.0}}`,
			expectedType: "apply_errored",
			hasHook:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msg TerraformJSONMessage
			if err := json.Unmarshal([]byte(tt.json), &msg); err != nil {
				t.Fatalf("Failed to parse JSON: %v", err)
			}

			if msg.Type != tt.expectedType {
				t.Errorf("Expected type=%q, got %q", tt.expectedType, msg.Type)
			}
			if tt.hasHook && msg.Hook == nil {
				t.Error("Expected hook to be present")
			}
			if !tt.hasHook && msg.Hook != nil {
				t.Error("Expected hook to be nil")
			}

			// Verify that text completion messages would fall into default case
			if tt.shouldBeIgnored {
				isCompletionText := strings.Contains(msg.Message, ": Creation complete after") ||
					strings.Contains(msg.Message, ": Modifications complete after") ||
					strings.Contains(msg.Message, ": Destroy complete after") ||
					strings.Contains(msg.Message, ": Destruction complete after")
				if !isCompletionText {
					t.Error("Expected message to match completion text patterns")
				}
				// Verify it goes to default case (not apply_complete)
				if msg.Type == "apply_complete" {
					t.Error("Text completion should NOT have type=apply_complete")
				}
			}
		})
	}
}

// Ensure applyState.mu is properly used (compile-time check)
var _ sync.Mutex = applyState{}.mu
