package main

import (
	"fmt"
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

// change builds a ResourceChange with the given plan actions.
func change(address string, actions ...string) ResourceChange {
	c := ResourceChange{Address: address, Type: "aws_iam_role"}
	c.Change.Actions = actions
	return c
}

// The counting tests above hand-build changeGroups, so they never exercised the
// classifier that fills those groups -- and the classifier switched on
// Actions[0] looking for a literal "replace" action terraform never emits. Every
// replacement was filed as a delete (or a create, under create_before_destroy)
// and counted once, while apply reported a completion for each half. Start from
// raw plan actions so the two halves of the fix stay connected.
func TestGroupResourceChanges_ClassifiesReplacementsFromActionPairs(t *testing.T) {
	groups := groupResourceChanges([]ResourceChange{
		change("aws_iam_role.destroy_then_create", "delete", "create"),
		change("aws_iam_role.create_before_destroy", "create", "delete"),
		change("aws_instance.new", "create"),
		change("aws_instance.gone", "delete"),
		change("aws_s3_bucket.tweaked", "update"),
		change("data.aws_region.current", "read"),
		change("aws_instance.untouched", "no-op"),
	})

	if len(groups.replaces) != 2 {
		t.Errorf("expected both replacement forms grouped as replaces, got %d", len(groups.replaces))
	}
	if len(groups.creates) != 1 {
		t.Errorf("expected 1 create, got %d", len(groups.creates))
	}
	if len(groups.deletes) != 1 {
		t.Errorf("expected 1 delete, got %d", len(groups.deletes))
	}
	if len(groups.updates) != 1 {
		t.Errorf("expected 1 update, got %d", len(groups.updates))
	}
}

// The apply progress bar counts one completion per operation, so the denominator
// has to be operations too. This is the "102/68" case: a plan whose resource
// count is smaller than the number of hooks the apply will emit.
func TestPlanTotalsCountReplacementsAsTwoOperations(t *testing.T) {
	var changes []ResourceChange
	for i := 0; i < 34; i++ {
		changes = append(changes, change(fmt.Sprintf("aws_iam_role.replaced_%d", i), "delete", "create"))
	}
	for i := 0; i < 18; i++ {
		changes = append(changes, change(fmt.Sprintf("aws_instance.created_%d", i), "create"))
	}
	for i := 0; i < 13; i++ {
		changes = append(changes, change(fmt.Sprintf("aws_s3_bucket.updated_%d", i), "update"))
	}
	for i := 0; i < 3; i++ {
		changes = append(changes, change(fmt.Sprintf("aws_instance.destroyed_%d", i), "delete"))
	}

	stats := calculateStatistics(groupResourceChanges(changes))

	if stats.totalChanges != 102 {
		t.Errorf("expected 102 operations for 68 planned resources, got %d", stats.totalChanges)
	}

	// Matches terraform's own "Apply complete! Resources: 52 added, 13 changed,
	// 37 destroyed." for the same plan.
	adds, updates, destroys := stats.plannedOperations()
	if adds != 52 || updates != 13 || destroys != 37 {
		t.Errorf("expected 52/13/37 add/change/destroy, got %d/%d/%d", adds, updates, destroys)
	}
	if adds+updates+destroys != stats.totalChanges {
		t.Errorf("summary columns (%d) must sum to totalChanges (%d)", adds+updates+destroys, stats.totalChanges)
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
