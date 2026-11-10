package main

import (
	"context"
	"strings"
	"time"
)

// While Terraform waits on aws_acm_certificate_validation it prints nothing but
// elapsed time, for up to its full timeout. This watch reads ACM's own state for
// the environment's certificates and puts it on that resource's status line, so
// the screen says whether ACM has validated, and if not, why.

// certWatchInterval paces the ACM reads. ACM re-checks a pending validation on
// its own schedule, measured in minutes, so more often only repaints the same
// answer.
const certWatchInterval = 30 * time.Second

func isCertValidationAddress(addr string) bool {
	return strings.Contains(addr, "aws_acm_certificate_validation")
}

// startCertWatch begins the watch the first time a validation resource starts.
func (m *modernPlanModel) startCertWatch() {
	if m.applyState == nil {
		return
	}
	m.applyState.mu.Lock()
	if m.applyState.certWatchStarted {
		m.applyState.mu.Unlock()
		return
	}
	m.applyState.certWatchStarted = true
	m.applyState.mu.Unlock()

	env := m.target.Env
	if expectedZoneName(env) == "" {
		return
	}
	go m.runCertWatch(env)
}

func (m *modernPlanModel) runCertWatch(env Env) {
	ticker := time.NewTicker(certWatchInterval)
	defer ticker.Stop()
	last := ""
	for range ticker.C {
		if !m.certValidationRunning() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		checks, err := checkCertificateValidation(ctx, env, nil)
		cancel()
		if err != nil || len(checks) == 0 {
			continue
		}
		summary := summarizeCertChecks(checks)
		m.setCertNote(summary)
		if summary != last {
			level := "info"
			if verdictSeverity(checks[0].Verdict) >= verdictSeverity(certOutsideZone) {
				level = "warning"
			}
			m.sendLogMessage(level, summary, "")
			last = summary
		}
	}
}

// certValidationRunning reports whether a validation resource is still being
// applied.
func (m *modernPlanModel) certValidationRunning() bool {
	m.applyState.mu.Lock()
	defer m.applyState.mu.Unlock()
	if m.applyState.applyComplete {
		return false
	}
	for addr := range m.applyState.currentOps {
		if isCertValidationAddress(addr) {
			return true
		}
	}
	return false
}

// setCertNote attaches the ACM state to every validation resource in flight.
func (m *modernPlanModel) setCertNote(note string) {
	m.applyState.mu.Lock()
	defer m.applyState.mu.Unlock()
	for addr, op := range m.applyState.currentOps {
		if isCertValidationAddress(addr) {
			op.Note = note
			op.Status = withNote(op.Status, note)
		}
	}
}

// withNote appends note to a status line, replacing any note already there.
func withNote(status, note string) string {
	if i := strings.Index(status, " · ACM:"); i >= 0 {
		status = status[:i]
	}
	if note == "" {
		return status
	}
	return status + " · " + note
}
