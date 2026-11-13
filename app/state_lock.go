package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// A Terraform run holds a lock on the state for as long as it runs. A second
// run against the same state fails with "Error acquiring the state lock" and a
// block of lock details. Those details answer the question the operator has —
// is someone still using it — only with a look at the processes on this
// machine, so meroku takes that look.

// stateLock is the lock a failed run reported.
type stateLock struct {
	ID        string
	Path      string
	Operation string
	Who       string // user@host
	Created   string
}

var lockField = regexp.MustCompile(`(?m)^\s*(?:ERROR:\s*)?(ID|Path|Operation|Who|Created):\s+(.+?)\s*$`)

// parseStateLock reads the lock details out of Terraform's output.
func parseStateLock(output string) (stateLock, bool) {
	if !strings.Contains(output, "Error acquiring the state lock") {
		return stateLock{}, false
	}
	var l stateLock
	for _, m := range lockField.FindAllStringSubmatch(output, -1) {
		switch m[1] {
		case "ID":
			l.ID = m[2]
		case "Path":
			l.Path = m[2]
		case "Operation":
			l.Operation = m[2]
		case "Who":
			l.Who = m[2]
		case "Created":
			l.Created = m[2]
		}
	}
	return l, l.ID != ""
}

// lockHolder is what meroku found about the run holding a lock.
type lockHolder struct {
	// ThisMachine is true when the lock was taken from this host.
	ThisMachine bool
	// PID is the terraform process still running in the environment's
	// directory, or 0 when there is none.
	PID int
}

// terraformProcessesIn returns the PIDs of terraform processes whose working
// directory is dir. A variable so tests can replace it.
var terraformProcessesIn = func(dir string) []int {
	out, err := exec.Command("pgrep", "-x", "terraform").Output()
	if err != nil {
		return nil
	}
	want, _ := filepath.EvalSymlinks(dir)
	var pids []int
	for _, field := range strings.Fields(string(out)) {
		pid, err := strconv.Atoi(field)
		if err != nil {
			continue
		}
		cwd, err := exec.Command("lsof", "-a", "-p", field, "-d", "cwd", "-Fn").Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(cwd), "\n") {
			if p, ok := strings.CutPrefix(line, "n"); ok {
				if real, _ := filepath.EvalSymlinks(p); real == want {
					pids = append(pids, pid)
				}
			}
		}
	}
	return pids
}

// findLockHolder looks for the run behind lock on this machine.
func findLockHolder(lock stateLock, envDir string) lockHolder {
	h := lockHolder{}
	host, _ := os.Hostname()
	if _, who, ok := strings.Cut(lock.Who, "@"); ok {
		short := func(s string) string { s, _, _ = strings.Cut(strings.ToLower(s), "."); return s }
		h.ThisMachine = short(who) == short(host)
	}
	if h.ThisMachine {
		if pids := terraformProcessesIn(envDir); len(pids) > 0 {
			h.PID = pids[0]
		}
	}
	return h
}

// describeStateLock explains the lock in the operator's terms and names the
// way out.
func describeStateLock(env string, lock stateLock, h lockHolder) []string {
	what := strings.TrimPrefix(strings.ToLower(lock.Operation), "operationtype")
	if what == "" {
		what = "run"
	}
	since := lock.Created
	if t, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", lock.Created); err == nil {
		since = fmt.Sprintf("%s (%s ago)", t.Local().Format("15:04"), time.Since(t).Round(time.Minute))
	}
	lines := []string{fmt.Sprintf("🔒 %s's state is locked by a terraform %s started at %s by %s.", env, what, since, lock.Who)}
	switch {
	case h.PID != 0:
		lines = append(lines,
			fmt.Sprintf("That %s is still running on this machine (terraform PID %d), probably in another meroku window.", what, h.PID),
			"Nothing is wrong with the state. Let that run finish, or press [k] to stop it: terraform",
			"cancels cleanly, releases the lock, and this destroy starts again.")
	case h.ThisMachine:
		lines = append(lines,
			fmt.Sprintf("No terraform is running in env/%s on this machine any more: that run ended without", env),
			"releasing the lock (a closed terminal or a crash). Press [u] to release it; this destroy then starts again.")
	default:
		lines = append(lines,
			"It was taken on another machine. If that run has finished or died, release it with:",
			fmt.Sprintf("  cd env/%s && terraform force-unlock %s", env, lock.ID))
	}
	return lines
}

// stopLockHolder interrupts the terraform holding the lock and waits for it to
// exit. Terraform treats the first interrupt as "cancel cleanly": it stops
// starting new operations, waits for those in flight, writes the state and
// releases the lock.
func stopLockHolder(pid int, timeout time.Duration) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("finding terraform (PID %d): %w", pid, err)
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("stopping terraform (PID %d): %w", pid, err)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("terraform (PID %d) is still finishing after %s; try again shortly", pid, timeout)
}

// releaseStaleLock force-unlocks a lock whose run is gone.
func releaseStaleLock(envDir, id string) error {
	cmd, err := newTerraformCmd(context.Background(), tfState, terraformDirOrCwd(envDir), "force-unlock", "-force", id)
	if err != nil {
		return fmt.Errorf("terraform force-unlock: %w", err)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("terraform force-unlock: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
