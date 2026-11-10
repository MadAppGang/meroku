package main

import (
	"context"
	"os"
	"strings"
	"testing"
)

func stubStateLock(t *testing.T, lock *stateLock, pids []int) *int {
	t.Helper()
	prevRead, prevRemove, prevProcs := stateLockReader, stateLockRemover, terraformProcessesIn
	t.Cleanup(func() { stateLockReader, stateLockRemover, terraformProcessesIn = prevRead, prevRemove, prevProcs })
	removed := 0
	stateLockReader = func(context.Context, Env) (stateLock, bool, error) {
		if lock == nil || removed > 0 {
			return stateLock{}, false, nil
		}
		return *lock, true, nil
	}
	stateLockRemover = func(context.Context, Env) error { removed++; return nil }
	terraformProcessesIn = func(string) []int { return pids }
	return &removed
}

var lockEnv = Env{Env: "prod", StateBucket: "bucket", StateFile: "state.tfstate"}

func TestStaleLockFromThisMachineIsReleased(t *testing.T) {
	host, _ := os.Hostname()
	removed := stubStateLock(t, &stateLock{ID: "abc", Operation: "OperationTypeApply", Who: "jack@" + host}, nil)
	if err := clearStaleStateLock(context.Background(), lockEnv); err != nil {
		t.Fatal(err)
	}
	if *removed != 1 {
		t.Error("a lock whose run is gone must be released")
	}
}

func TestLockOfARunningTerraformIsKept(t *testing.T) {
	host, _ := os.Hostname()
	removed := stubStateLock(t, &stateLock{ID: "abc", Operation: "OperationTypeApply", Who: "jack@" + host}, []int{4242})
	err := clearStaleStateLock(context.Background(), lockEnv)
	if err == nil || !strings.Contains(err.Error(), "4242") || *removed != 0 {
		t.Errorf("err=%v removed=%d, want the deploy stopped and the lock kept", err, *removed)
	}
}

func TestLockFromAnotherMachineIsKept(t *testing.T) {
	removed := stubStateLock(t, &stateLock{ID: "abc", Who: "ci@build-runner-7"}, nil)
	err := clearStaleStateLock(context.Background(), lockEnv)
	if err == nil || !strings.Contains(err.Error(), "force-unlock abc") || *removed != 0 {
		t.Errorf("err=%v removed=%d, want the deploy stopped and the lock kept", err, *removed)
	}
}

func TestNoLockIsNothingToDo(t *testing.T) {
	removed := stubStateLock(t, nil, nil)
	if err := clearStaleStateLock(context.Background(), lockEnv); err != nil || *removed != 0 {
		t.Errorf("err=%v removed=%d", err, *removed)
	}
}
