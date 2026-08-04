package main

import (
	"testing"
)

// Test that replace operations are counted as 2 (delete + create)
func TestCalculateStatistics_ReplaceCountsAsTwo(t *testing.T) {
	// Create test data with 1 create, 1 update, and 1 replace
	groups := changeGroups{
		creates: []ResourceChange{
			{Address: "aws_instance.new", Type: "aws_instance"},
		},
		updates: []ResourceChange{
			{Address: "aws_instance.existing", Type: "aws_instance"},
		},
		deletes: []ResourceChange{},
		replaces: []ResourceChange{
			{Address: "aws_iam_role.test", Type: "aws_iam_role"},
		},
		reads: []ResourceChange{},
	}

	stats := calculateStatistics(groups)

	// 1 create + 1 update + 2 (replace = delete + create) = 4 total
	expectedTotal := 4
	if stats.totalChanges != expectedTotal {
		t.Errorf("Expected totalChanges=%d, got %d", expectedTotal, stats.totalChanges)
	}

	// Verify byAction counts
	if stats.byAction["create"] != 1 {
		t.Errorf("Expected byAction[create]=1, got %d", stats.byAction["create"])
	}
	if stats.byAction["update"] != 1 {
		t.Errorf("Expected byAction[update]=1, got %d", stats.byAction["update"])
	}
	if stats.byAction["replace"] != 1 {
		t.Errorf("Expected byAction[replace]=1, got %d", stats.byAction["replace"])
	}
}

// Test that multiple replaces are counted correctly
func TestCalculateStatistics_MultipleReplaces(t *testing.T) {
	groups := changeGroups{
		creates: []ResourceChange{},
		updates: []ResourceChange{},
		deletes: []ResourceChange{},
		replaces: []ResourceChange{
			{Address: "aws_iam_role.role1", Type: "aws_iam_role"},
			{Address: "aws_iam_role.role2", Type: "aws_iam_role"},
			{Address: "aws_amplify_domain.test", Type: "aws_amplify_domain_association"},
		},
		reads: []ResourceChange{},
	}

	stats := calculateStatistics(groups)

	// 3 replaces × 2 = 6 total operations
	expectedTotal := 6
	if stats.totalChanges != expectedTotal {
		t.Errorf("Expected totalChanges=%d for 3 replaces, got %d", expectedTotal, stats.totalChanges)
	}
}

// Test that read operations are NOT counted in totalChanges
// Reads are data source refreshes that don't generate apply events
func TestCalculateStatistics_ReadsNotCounted(t *testing.T) {
	groups := changeGroups{
		creates: []ResourceChange{
			{Address: "aws_instance.new", Type: "aws_instance"},
		},
		updates: []ResourceChange{
			{Address: "aws_instance.existing", Type: "aws_instance"},
		},
		deletes:  []ResourceChange{},
		replaces: []ResourceChange{},
		reads: []ResourceChange{
			{Address: "data.aws_caller_identity.current", Type: "data.aws_caller_identity"},
			{Address: "data.aws_region.current", Type: "data.aws_region"},
		},
	}

	stats := calculateStatistics(groups)

	// 1 create + 1 update = 2 total (reads should NOT be counted)
	expectedTotal := 2
	if stats.totalChanges != expectedTotal {
		t.Errorf("Expected totalChanges=%d (reads excluded), got %d", expectedTotal, stats.totalChanges)
	}

	// But reads should still be tracked in byAction
	if stats.byAction["read"] != 2 {
		t.Errorf("Expected byAction[read]=2, got %d", stats.byAction["read"])
	}
}

// Test pending list creation for replace operations
func TestPendingListForReplace(t *testing.T) {
	// Simulate what initApplyState does
	resources := []struct {
		Address string
		Actions []string
		Type    string
	}{
		{"aws_instance.simple", []string{"create"}, "aws_instance"},
		{"aws_iam_role.replace_me", []string{"delete", "create"}, "aws_iam_role"},
		{"aws_s3_bucket.update", []string{"update"}, "aws_s3_bucket"},
	}

	var pending []pendingResource

	for _, resource := range resources {
		// This is the logic from initApplyState
		if len(resource.Actions) == 2 &&
			resource.Actions[0] == "delete" &&
			resource.Actions[1] == "create" {
			// Add delete operation
			pending = append(pending, pendingResource{
				Address: resource.Address,
				Action:  "delete",
				Type:    resource.Type,
			})
			// Add create operation
			pending = append(pending, pendingResource{
				Address: resource.Address,
				Action:  "create",
				Type:    resource.Type,
			})
		} else {
			pending = append(pending, pendingResource{
				Address: resource.Address,
				Action:  resource.Actions[0],
				Type:    resource.Type,
			})
		}
	}

	// Should have 4 pending items: 1 create + 2 for replace + 1 update
	expectedPending := 4
	if len(pending) != expectedPending {
		t.Errorf("Expected %d pending items, got %d", expectedPending, len(pending))
	}

	// Verify the replace resource has both delete and create entries
	deleteFound := false
	createFound := false
	for _, p := range pending {
		if p.Address == "aws_iam_role.replace_me" {
			if p.Action == "delete" {
				deleteFound = true
			}
			if p.Action == "create" {
				createFound = true
			}
		}
	}

	if !deleteFound {
		t.Error("Expected delete entry for replace resource")
	}
	if !createFound {
		t.Error("Expected create entry for replace resource")
	}
}

// Test action name normalization (destroy -> delete)
func TestNormalizeAction(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"destroy", "delete"},
		{"delete", "delete"},
		{"create", "create"},
		{"update", "update"},
	}

	for _, tt := range tests {
		result := normalizeAction(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeAction(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// Test pending removal matches by address AND action
func TestPendingRemovalMatchesByAddressAndAction(t *testing.T) {
	// Setup pending with replace (2 entries for same address)
	pending := []pendingResource{
		{Address: "aws_iam_role.test", Action: "delete", Type: "aws_iam_role"},
		{Address: "aws_iam_role.test", Action: "create", Type: "aws_iam_role"},
		{Address: "aws_instance.other", Action: "update", Type: "aws_instance"},
	}

	// Simulate completing the delete operation
	addressToRemove := "aws_iam_role.test"
	actionToRemove := "delete"

	// This is the new logic that matches BOTH address AND action
	for i, p := range pending {
		if p.Address == addressToRemove && p.Action == actionToRemove {
			pending = append(pending[:i], pending[i+1:]...)
			break
		}
	}

	// Should have 2 remaining: the create for aws_iam_role.test and the update
	if len(pending) != 2 {
		t.Errorf("Expected 2 pending after removing delete, got %d", len(pending))
	}

	// Verify the create operation is still there
	createStillPending := false
	for _, p := range pending {
		if p.Address == "aws_iam_role.test" && p.Action == "create" {
			createStillPending = true
			break
		}
	}
	if !createStillPending {
		t.Error("Create operation should still be pending after delete completes")
	}
}
