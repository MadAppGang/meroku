package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// The read grant on the enhanced bus is written out in each of the three
// modules whose task roles get it. A module call shared by all three would
// need `terraform init` in every deployed environment before its next plan,
// so the copies stay, and this test keeps them identical.
func TestEventBridgeV2ReadStatementsAreIdentical(t *testing.T) {
	files := []string{
		"../modules/workloads/eventbridge.tf",
		"../modules/ecs_task/eventbridge.tf",
		"../modules/event_bridge_task/eventbridge.tf",
	}
	block := regexp.MustCompile(`(?ms)^  eventbridge_v2_read_statements = .*?^  \]\)\)$`)
	var want string
	for i, f := range files {
		data, err := os.ReadFile(filepath.FromSlash(f))
		if err != nil {
			t.Fatal(err)
		}
		got := block.FindString(string(data))
		if got == "" {
			t.Fatalf("%s has no eventbridge_v2_read_statements local", f)
		}
		if i == 0 {
			want = got
			continue
		}
		if got != want {
			t.Errorf("%s's eventbridge_v2_read_statements differs from %s's:\n%s", f, files[0], eventsClassicFirstDiff(want, got))
		}
	}
}
