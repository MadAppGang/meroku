package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func testDNSModel(t *testing.T) *dnsSetupModel {
	t.Helper()
	m := newDNSSetupModel(
		Env{Env: "dev", AccountID: "285253872242"},
		dnsPreflightResult{ZoneName: "dev.example.com", ParentDomain: "example.com"},
	)
	m.width, m.height = 120, 32
	return m
}

// A missing zone starts at phase 1; an existing-but-undelegated zone skips it.
func TestDNSModel_StartStepDependsOnZone(t *testing.T) {
	missing := newDNSSetupModel(Env{}, dnsPreflightResult{ZoneName: "dev.example.com", ParentDomain: "example.com"})
	if missing.step != stepCreateZone {
		t.Errorf("expected stepCreateZone when the zone is absent, got %v", missing.step)
	}

	existing := newDNSSetupModel(Env{}, dnsPreflightResult{
		ZoneName: "dev.example.com", ParentDomain: "example.com",
		ZoneID: "Z1", ZoneNameservers: []string{"ns-1.example.net"},
	})
	if existing.step != stepFindParent {
		t.Errorf("expected stepFindParent when the zone already exists, got %v", existing.step)
	}
	if existing.states[stepCreateZone] != stepSkipped {
		t.Error("an existing zone should mark the create step skipped, not pending")
	}
}

func TestDNSModel_ZoneCreatedAdvances(t *testing.T) {
	m := testDNSModel(t)
	updated, _ := m.Update(dnsZoneCreatedMsg{zoneID: "Z9", nameservers: []string{"ns-1.example.net"}})
	m = updated.(*dnsSetupModel)

	if m.step != stepFindParent {
		t.Errorf("expected to advance to stepFindParent, got %v", m.step)
	}
	if m.states[stepCreateZone] != stepOK {
		t.Error("create step should be marked OK")
	}
	if m.zoneID != "Z9" {
		t.Errorf("zone id not recorded, got %q", m.zoneID)
	}
}

// A failed phase 1 must stop rather than continue into a deploy that cannot work.
func TestDNSModel_ZoneCreateFailureStops(t *testing.T) {
	m := testDNSModel(t)
	updated, cmd := m.Update(dnsZoneCreatedMsg{err: errors.New("boom")})
	m = updated.(*dnsSetupModel)

	if m.err == nil {
		t.Error("expected the error to be recorded")
	}
	if m.states[stepCreateZone] != stepFailed {
		t.Error("create step should be marked failed")
	}
	if cmd == nil {
		t.Error("expected a quit command")
	}
	if m.Delegated {
		t.Error("a failed run must never report delegation as done")
	}
}

// No usable candidate falls back to manual instructions, not a dead end.
func TestDNSModel_NoCandidatesFallsBackToManual(t *testing.T) {
	m := testDNSModel(t)
	updated, _ := m.Update(dnsCandidatesMsg{reason: "example.com is not hosted on Route53"})
	m = updated.(*dnsSetupModel)

	if m.manualReason == "" {
		t.Error("expected a manual fallback reason")
	}
	if m.states[stepFindParent] != stepFailed {
		t.Error("find-parent should be marked failed")
	}
	if !strings.Contains(m.View(), "MANUAL") {
		t.Error("the manual badge should be visible in the view")
	}
}

// The safety property: a zone that does not match public DNS cannot be chosen,
// because writing into it would silently do nothing.
func TestDNSModel_CannotDelegateToNonAuthoritative(t *testing.T) {
	m := testDNSModel(t)
	m.candidates = []parentZoneCandidate{{Profile: "wrong", ZoneID: "Z1", Authoritative: false}}
	m.choosing = true
	m.step = stepWriteRecord
	m.cursor = 0

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("Enter on a non-authoritative candidate must not start a write")
	}
	if !m.choosing {
		t.Error("the picker should stay open after an invalid selection")
	}
}

func TestDNSModel_DelegatesToAuthoritative(t *testing.T) {
	m := testDNSModel(t)
	m.candidates = []parentZoneCandidate{
		{Profile: "bad", ZoneID: "Z1", Authoritative: false},
		{Profile: "good", ZoneID: "Z2", Authoritative: true},
	}
	m.nameservers = []string{"ns-1.example.net"}
	m.choosing = true
	m.step = stepWriteRecord
	m.cursor = 1

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Error("expected a delegation command for a verified candidate")
	}
	if m.choosing {
		t.Error("the picker should close once a write starts")
	}
}

// The cursor should land on a usable candidate, not on the first row blindly.
func TestDNSModel_CursorStartsOnAuthoritative(t *testing.T) {
	m := testDNSModel(t)
	updated, _ := m.Update(dnsCandidatesMsg{candidates: []parentZoneCandidate{
		{Profile: "bad", ZoneID: "Z1", Authoritative: false},
		{Profile: "good", ZoneID: "Z2", Authoritative: true},
	}})
	m = updated.(*dnsSetupModel)

	if m.cursor != 1 {
		t.Errorf("expected the cursor on the verified candidate (index 1), got %d", m.cursor)
	}
}

// Delegated must only become true once propagation is actually observed.
func TestDNSModel_DelegatedOnlyAfterPropagation(t *testing.T) {
	m := testDNSModel(t)
	m.step = stepPropagate

	updated, _ := m.Update(dnsPropagationMsg{results: map[string]bool{"8.8.8.8": false}, ok: false})
	m = updated.(*dnsSetupModel)
	if m.Delegated {
		t.Error("must not report delegated while resolvers still disagree")
	}

	updated, _ = m.Update(dnsPropagationMsg{
		results: map[string]bool{"8.8.8.8": true, "1.1.1.1": true}, ok: true})
	m = updated.(*dnsSetupModel)
	if !m.Delegated {
		t.Error("expected delegated once propagation is confirmed")
	}
	if m.step != stepDone {
		t.Errorf("expected stepDone, got %v", m.step)
	}
}

// Layout lock: no rendered line may exceed the terminal width, at any size or
// state. Overflow pushes content outside the panel border and corrupts the frame.
func TestDNSModel_NoLineExceedsWidth(t *testing.T) {
	sizes := []struct{ w, h int }{{80, 24}, {100, 30}, {120, 32}, {200, 50}}

	build := func(w, h int) []*dnsSetupModel {
		mk := func(f func(*dnsSetupModel)) *dnsSetupModel {
			m := newDNSSetupModel(Env{}, dnsPreflightResult{
				ZoneName: "dev.coretechx.dev", ParentDomain: "coretechx.dev",
			})
			m.width, m.height = w, h
			m.nameservers = []string{
				"ns-1930.awsdns-49.co.uk", "ns-1050.awsdns-03.org",
				"ns-678.awsdns-20.net", "ns-247.awsdns-30.com",
			}
			m.zoneID = "Z0580793YTBKHE7ID6NJ"
			f(m)
			return m
		}
		return []*dnsSetupModel{
			mk(func(m *dnsSetupModel) { m.step = stepCreateZone }),
			mk(func(m *dnsSetupModel) { m.step = stepFindParent }),
			mk(func(m *dnsSetupModel) {
				m.step = stepWriteRecord
				m.choosing = true
				m.candidates = []parentZoneCandidate{
					{Profile: "mag", AccountID: "891880437329", ZoneID: "Z0", Authoritative: true},
					{Profile: "averyverylongprofilename", AccountID: "111122223333", ZoneID: "Z1"},
					{Profile: "expired", Err: errors.New("ExpiredToken: the security token included in the request is expired")},
				}
			}),
			mk(func(m *dnsSetupModel) {
				m.step = stepPropagate
				m.resolverResults = map[string]bool{"8.8.8.8": true, "1.1.1.1": false}
			}),
			mk(func(m *dnsSetupModel) { m.step = stepDone }),
			mk(func(m *dnsSetupModel) {
				m.manualReason = "coretechx.dev is not hosted on Route53 (ada.ns.cloudflare.com), so meroku cannot write the record for you"
			}),
		}
	}

	for _, size := range sizes {
		for i, m := range build(size.w, size.h) {
			for n, line := range strings.Split(m.View(), "\n") {
				if got := lipgloss.Width(line); got > size.w {
					t.Errorf("%dx%d state %d line %d: width %d exceeds %d\n%q",
						size.w, size.h, i, n, got, size.w, line)
				}
			}
		}
	}
}

// pulse must stay inside [0,1) and never claim completion for work of unknown
// duration — a meter that reaches 100% and keeps waiting reads as a hang.
func TestPulse_StaysBelowComplete(t *testing.T) {
	for _, secs := range []int{0, 1, 10, 60, 600, 36000} {
		got := pulse(durationSeconds(secs))
		if got < 0 || got >= 1 {
			t.Errorf("pulse(%ds) = %v, want [0,1)", secs, got)
		}
	}
	if pulse(durationSeconds(0)) != 0 {
		t.Error("pulse should start at zero")
	}
	if pulse(durationSeconds(60)) <= pulse(durationSeconds(10)) {
		t.Error("pulse should increase with elapsed time")
	}
}

// durationSeconds is a readability helper for the pulse table.
func durationSeconds(n int) time.Duration { return time.Duration(n) * time.Second }
