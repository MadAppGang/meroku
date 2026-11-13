package main

import (
	"context"
	"errors"
	"testing"
)

// A local backend before the first apply has no state file, and `terraform
// state list` exits 1 saying so. That is an empty state, not a broken one.
const noStateFileStderr = "terraform state: exit status 1\n\x1b[31mNo state file was found!\n\nState management commands require a state file. Run this command\nin a directory where Terraform has been run or use the -state flag\nto point the command to a specific state location.\x1b[0m"

func TestReadNeonStateTreatsMissingStateFileAsEmpty(t *testing.T) {
	calls := 0
	tf := func(ctx context.Context, purpose tfPurpose, dir string, args ...string) ([]byte, error) {
		calls++
		return nil, errors.New(noStateFileStderr)
	}
	view, err := readNeonState(context.Background(), tf, t.TempDir())
	if err != nil {
		t.Fatalf("readNeonState: %v, want an empty view", err)
	}
	if len(view.Managed) != 0 || len(view.SSMServices) != 0 {
		t.Fatalf("view = %+v, want empty", view)
	}
	if calls != 1 {
		t.Fatalf("terraform ran %d times, want 1 (no show -json on an empty state)", calls)
	}
}

func TestReadNeonStateStillFailsOnOtherStateErrors(t *testing.T) {
	tf := func(ctx context.Context, purpose tfPurpose, dir string, args ...string) ([]byte, error) {
		return nil, errors.New("terraform state: exit status 1\nError: Error acquiring the state lock")
	}
	if _, err := readNeonState(context.Background(), tf, t.TempDir()); err == nil {
		t.Fatal("readNeonState succeeded on a state lock error, want the error")
	}
}
