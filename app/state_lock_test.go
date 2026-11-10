package main

import (
	"os"
	"strings"
	"testing"
)

// Copied from a real failure, with the bucket and IDs replaced.
const lockFailureOutput = `ERROR: Error: Error acquiring the state lock
ERROR:
ERROR: Error message: operation error S3: PutObject, https response error
ERROR: StatusCode: 412, RequestID: 000000000000, HostID: fake
ERROR: api error PreconditionFailed: At least one of the pre-conditions you
ERROR: specified did not hold
ERROR: Lock Info:
ERROR:   ID:        00000000-0000-0000-0000-000000000000
ERROR:   Path:      state-bucket-example/state.tfstate
ERROR:   Operation: OperationTypeApply
ERROR:   Who:       someone@HOSTNAME
ERROR:   Version:   1.17.1
ERROR:   Created:   2026-11-10 04:43:07.436086 +0000 UTC
ERROR:   Info:
ERROR:
ERROR: Terraform acquires a state lock to protect the state from being written`

func lockOutputFor(host string) string {
	return strings.Replace(lockFailureOutput, "HOSTNAME", host, 1)
}

func TestParseStateLock(t *testing.T) {
	l, ok := parseStateLock(lockOutputFor("m5"))
	if !ok {
		t.Fatal("lock not recognised")
	}
	if l.ID != "00000000-0000-0000-0000-000000000000" || l.Operation != "OperationTypeApply" ||
		l.Who != "someone@m5" || l.Path != "state-bucket-example/state.tfstate" {
		t.Errorf("parsed %+v", l)
	}
	if _, ok := parseStateLock("Error: something else"); ok {
		t.Error("an unrelated error parsed as a lock")
	}
}

func withProcesses(t *testing.T, pids []int) {
	t.Helper()
	prev := terraformProcessesIn
	t.Cleanup(func() { terraformProcessesIn = prev })
	terraformProcessesIn = func(string) []int { return pids }
}

func TestFindLockHolder(t *testing.T) {
	host, _ := os.Hostname()
	short, _, _ := strings.Cut(host, ".")
	l, _ := parseStateLock(lockOutputFor(short))

	withProcesses(t, []int{4242})
	if h := findLockHolder(l, "/env/prod"); !h.ThisMachine || h.PID != 4242 {
		t.Errorf("running holder: %+v", h)
	}
	withProcesses(t, nil)
	if h := findLockHolder(l, "/env/prod"); !h.ThisMachine || h.PID != 0 {
		t.Errorf("stale holder: %+v", h)
	}
	other, _ := parseStateLock(lockOutputFor("some-other-machine"))
	withProcesses(t, []int{4242})
	if h := findLockHolder(other, "/env/prod"); h.ThisMachine || h.PID != 0 {
		t.Errorf("another machine's lock must not be tied to a local process: %+v", h)
	}
}

func TestDestroyScreenExplainsALiveLock(t *testing.T) {
	host, _ := os.Hostname()
	short, _, _ := strings.Cut(host, ".")
	withProcesses(t, []int{4242})

	m := &destroyProgressModel{env: "prod", width: 140, height: 50}
	m.Update(destroyErrorMsg{err: os.ErrExist, output: strings.Split(lockOutputFor(short), "\n")})
	if m.lock == nil || m.holder.PID != 4242 {
		t.Fatalf("lock=%v holder=%+v", m.lock, m.holder)
	}
	view := m.View()
	for _, want := range []string{"still running on this machine", "PID 4242", "[s] Stop the running apply"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q", want)
		}
	}
}
