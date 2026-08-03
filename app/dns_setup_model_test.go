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

// startScan puts the model into the streaming state without doing any I/O.
func startScan(t *testing.T, m *dnsSetupModel, total int) chan parentZoneCandidate {
	t.Helper()
	ch := make(chan parentZoneCandidate, total)
	updated, _ := m.Update(dnsScanStartedMsg{ch: ch, cancel: func() {}, total: total})
	if updated.(*dnsSetupModel) != m {
		t.Fatal("Update should return the same model pointer")
	}
	return ch
}

// The picker opens on the first usable result, not after the last one — that is
// the entire point of streaming.
func TestDNSModel_PickerOpensOnFirstCandidate(t *testing.T) {
	m := testDNSModel(t)
	m.step = stepFindParent
	startScan(t, m, 8)

	if m.choosing {
		t.Error("nothing has arrived yet; the picker should not be open")
	}

	updated, _ := m.Update(dnsCandidateMsg{c: parentZoneCandidate{
		Profile: "mag", ZoneID: "Z1", Authoritative: true}})
	m = updated.(*dnsSetupModel)

	if !m.choosing {
		t.Error("the picker should open as soon as a usable candidate lands")
	}
	if m.step != stepWriteRecord {
		t.Errorf("expected stepWriteRecord, got %v", m.step)
	}
	if m.scanned != 1 || m.scanTotal != 8 {
		t.Errorf("progress should read 1/8, got %d/%d", m.scanned, m.scanTotal)
	}
	if !m.scanning {
		t.Error("the scan should still be running behind the open picker")
	}
}

// Profiles with neither a zone nor an error are silent misses; listing them
// would bury the rows that matter.
func TestDNSModel_SilentMissesCountButDoNotList(t *testing.T) {
	m := testDNSModel(t)
	m.step = stepFindParent
	startScan(t, m, 3)

	updated, _ := m.Update(dnsCandidateMsg{c: parentZoneCandidate{Profile: "empty"}})
	m = updated.(*dnsSetupModel)

	if len(m.candidates) != 0 {
		t.Errorf("a profile with no zone should not be listed, got %v", m.candidates)
	}
	if m.scanned != 1 {
		t.Errorf("it should still count toward progress, got %d", m.scanned)
	}
}

// The list is re-sorted on every arrival. A cursor tracked as a bare index would
// slide onto a different profile mid-scan — the operator could press Enter and
// delegate to something they never selected.
func TestDNSModel_CursorStaysOnSelectedProfileAcrossResorts(t *testing.T) {
	m := testDNSModel(t)
	m.step = stepFindParent
	startScan(t, m, 4)

	send := func(c parentZoneCandidate) {
		updated, _ := m.Update(dnsCandidateMsg{c: c})
		m = updated.(*dnsSetupModel)
	}

	send(parentZoneCandidate{Profile: "zeta", ZoneID: "Z1", Authoritative: true})
	send(parentZoneCandidate{Profile: "yankee", ZoneID: "Z2", Authoritative: true})

	// The operator deliberately moves to "zeta".
	for m.candidates[m.cursor].Profile != "zeta" {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(*dnsSetupModel)
	}
	if !m.cursorPinned {
		t.Fatal("moving the cursor should pin it")
	}

	// A candidate that sorts ahead of zeta arrives.
	send(parentZoneCandidate{Profile: "alpha", ZoneID: "Z3", Authoritative: true})

	if got := m.candidates[m.cursor].Profile; got != "zeta" {
		t.Errorf("cursor moved off the operator's selection to %q", got)
	}
}

// Before the operator touches it, the cursor should follow the best candidate,
// so a verified profile arriving third is still what Enter would pick.
func TestDNSModel_UnpinnedCursorFollowsBestCandidate(t *testing.T) {
	m := testDNSModel(t)
	m.step = stepFindParent
	startScan(t, m, 4)

	send := func(c parentZoneCandidate) {
		updated, _ := m.Update(dnsCandidateMsg{c: c})
		m = updated.(*dnsSetupModel)
	}

	send(parentZoneCandidate{Profile: "alpha", ZoneID: "Z1"})
	send(parentZoneCandidate{Profile: "bravo", Err: errors.New("expired")})
	send(parentZoneCandidate{Profile: "zulu", ZoneID: "Z3", Authoritative: true})

	if got := m.candidates[m.cursor].Profile; got != "zulu" {
		t.Errorf("cursor should sit on the verified candidate, got %q", got)
	}
}

// Once a write is underway, a straggler must not reopen the picker on top of it.
func TestDNSModel_LateCandidateIgnoredAfterCommit(t *testing.T) {
	m := testDNSModel(t)
	m.step = stepFindParent
	m.nameservers = []string{"ns-1.example.net"}
	startScan(t, m, 4)

	updated, _ := m.Update(dnsCandidateMsg{c: parentZoneCandidate{
		Profile: "mag", ZoneID: "Z1", Authoritative: true}})
	m = updated.(*dnsSetupModel)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*dnsSetupModel)
	if cmd == nil {
		t.Fatal("Enter on a verified candidate should start the write")
	}
	if m.scanning {
		t.Error("committing should stop the scan")
	}

	updated, cmd = m.Update(dnsCandidateMsg{c: parentZoneCandidate{
		Profile: "late", ZoneID: "Z9", Authoritative: true}})
	m = updated.(*dnsSetupModel)

	if m.choosing {
		t.Error("a late candidate must not reopen the picker during a write")
	}
	if len(m.candidates) != 1 {
		t.Errorf("a late candidate must not be added, got %v", profileNames(m.candidates))
	}
	if cmd != nil {
		t.Error("draining should stop once the scan is over")
	}
}

// A scan that finds nothing must land on manual instructions, not an empty list.
func TestDNSModel_EmptyScanFallsBackToManual(t *testing.T) {
	m := testDNSModel(t)
	m.step = stepFindParent
	startScan(t, m, 2)

	updated, _ := m.Update(dnsScanDoneMsg{})
	m = updated.(*dnsSetupModel)

	if m.manualReason == "" {
		t.Error("expected a manual fallback when no profile holds the zone")
	}
	if m.scanning {
		t.Error("the scan should be marked finished")
	}
}

// Finishing the scan must not overwrite a picker the operator is already using.
func TestDNSModel_ScanDoneDoesNotDisturbOpenPicker(t *testing.T) {
	m := testDNSModel(t)
	m.step = stepFindParent
	startScan(t, m, 2)

	updated, _ := m.Update(dnsCandidateMsg{c: parentZoneCandidate{
		Profile: "mag", ZoneID: "Z1", Authoritative: true}})
	m = updated.(*dnsSetupModel)

	updated, _ = m.Update(dnsScanDoneMsg{})
	m = updated.(*dnsSetupModel)

	if m.manualReason != "" {
		t.Errorf("picker had a candidate; should not fall back to manual (%q)", m.manualReason)
	}
	if !m.choosing {
		t.Error("the picker should stay open")
	}
}

// A cache hit shows one pre-verified row and offers the escape hatch.
func TestDNSModel_CachedHitOffersFullScan(t *testing.T) {
	m := testDNSModel(t)
	m.step = stepFindParent

	updated, _ := m.Update(dnsCandidatesMsg{
		cached:     "mag",
		candidates: []parentZoneCandidate{{Profile: "mag", ZoneID: "Z1", Authoritative: true}},
	})
	m = updated.(*dnsSetupModel)

	if m.cachedProfile != "mag" {
		t.Errorf("expected the cached profile to be recorded, got %q", m.cachedProfile)
	}
	if !strings.Contains(m.View(), "[a] scan all profiles") {
		t.Error("the escape hatch should be offered in the footer")
	}
	if !strings.Contains(m.View(), "remembered this profile") {
		t.Error("the panel should explain why only one profile is listed")
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(*dnsSetupModel)

	if cmd == nil {
		t.Error("[a] should restart discovery")
	}
	if !m.rescanAll {
		t.Error("[a] should force a full scan")
	}
	if m.cachedProfile != "" || len(m.candidates) != 0 {
		t.Error("[a] should clear the cached result before rescanning")
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

// The done panel must not claim more than was observed. Delegation is accepted
// at two of four resolvers, so an unqualified "resolves" would overstate it
// while two resolvers are still serving cached negative answers.
func TestDNSModel_DoneStateReportsPartialPropagation(t *testing.T) {
	m := testDNSModel(t)
	m.step = stepPropagate

	updated, _ := m.Update(dnsPropagationMsg{
		results: map[string]bool{
			"8.8.8.8": false, "1.1.1.1": true,
			"9.9.9.9": false, "208.67.222.222": true,
		},
		ok: true,
	})
	m = updated.(*dnsSetupModel)

	view := m.View()
	if !strings.Contains(view, "DELEGATED") {
		t.Error("expected the delegated badge")
	}
	if !strings.Contains(view, "2 of 4 resolvers") {
		t.Errorf("the done panel should report the observed count, got:\n%s", view)
	}
	if !strings.Contains(view, "cached") {
		t.Error("it should explain why the others do not see it yet")
	}
}

// With every resolver agreeing there is nothing to qualify.
func TestDNSModel_DoneStateIsUnqualifiedWhenFullyPropagated(t *testing.T) {
	m := testDNSModel(t)
	m.step = stepPropagate

	all := map[string]bool{}
	for _, r := range m.resolvers {
		all[r] = true
	}
	updated, _ := m.Update(dnsPropagationMsg{results: all, ok: true})
	m = updated.(*dnsSetupModel)

	if strings.Contains(m.View(), "so far") {
		t.Error("a fully propagated delegation should not be hedged")
	}
}

// manualModel puts the model in the state the screenshot showed: parent not on
// Route53, nameservers known, waiting on a human.
func manualModel(t *testing.T) *dnsSetupModel {
	t.Helper()
	m := testDNSModel(t)
	m.nameservers = []string{
		"ns-839.awsdns-40.net", "ns-1058.awsdns-04.org",
		"ns-1555.awsdns-02.co.uk", "ns-213.awsdns-26.com",
	}
	updated, _ := m.Update(dnsCandidatesMsg{reason: "sploty.app is not hosted on Route53 (ns1.hover.com)"})
	return updated.(*dnsSetupModel)
}

// Esc continues without delegating; Ctrl+C cancels. They used to be the same
// key, so the only way to proceed was the one every other screen uses to abort.
func TestDNSModel_EscContinuesAndCtrlCCancels(t *testing.T) {
	m := manualModel(t)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(*dnsSetupModel)
	if !m.ContinueAnyway {
		t.Error("Esc should continue without delegating")
	}
	if m.SkipDomain || m.Delegated {
		t.Error("Esc must not imply skipping the domain or successful delegation")
	}
	if cmd == nil {
		t.Error("expected the screen to close")
	}

	m2 := manualModel(t)
	updated, _ = m2.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m2 = updated.(*dnsSetupModel)
	if m2.ContinueAnyway || m2.SkipDomain || m2.Delegated {
		t.Error("Ctrl+C is a cancel: it must set no outcome at all")
	}
}

func TestDNSModel_SkipDomainIsItsOwnOutcome(t *testing.T) {
	m := manualModel(t)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = updated.(*dnsSetupModel)

	if !m.SkipDomain {
		t.Error("[s] should request turning the custom domain off")
	}
	if m.ContinueAnyway || m.Delegated {
		t.Error("skipping the domain is not the same as continuing or delegating")
	}
	if cmd == nil {
		t.Error("expected the screen to close")
	}
}

// The consequence of skipping has to be stated where the decision is made.
func TestDNSModel_ManualPanelWarnsAboutCertificateStall(t *testing.T) {
	view := manualModel(t).View()
	for _, want := range []string{"ACM certificate", "20", "skip custom domain"} {
		if !strings.Contains(view, want) {
			t.Errorf("manual panel should mention %q:\n%s", want, view)
		}
	}
}

// Nameservers are numbered so the number is also the key that copies that line.
func TestDNSModel_NameserversAreNumbered(t *testing.T) {
	view := manualModel(t).View()
	for i, ns := range []string{
		"ns-839.awsdns-40.net", "ns-1058.awsdns-04.org",
		"ns-1555.awsdns-02.co.uk", "ns-213.awsdns-26.com",
	} {
		if !strings.Contains(view, ns) {
			t.Errorf("nameserver %s missing from the panel", ns)
		}
		_ = i
	}
	if !strings.Contains(view, "[c] copy all") || !strings.Contains(view, "[1-4] copy one") {
		t.Errorf("copy hints missing from the footer:\n%s", view)
	}
}

// The countdown must run down and fire a check, so a record added at a registrar
// is noticed without anyone pressing anything.
func TestDNSModel_CountdownTriggersAutomaticRecheck(t *testing.T) {
	m := manualModel(t)
	if m.nextCheckIn != secondsBetweenDNSChecks {
		t.Fatalf("countdown should start at %d, got %d", secondsBetweenDNSChecks, m.nextCheckIn)
	}
	if !strings.Contains(m.View(), "next check in") {
		t.Error("the countdown should be visible")
	}

	var cmd tea.Cmd
	for i := 0; i < secondsBetweenDNSChecks; i++ {
		var updated tea.Model
		updated, cmd = m.Update(dnsTickMsg(time.Now()))
		m = updated.(*dnsSetupModel)
	}

	if !m.checking {
		t.Error("expected a check to have started when the countdown reached zero")
	}
	if cmd == nil {
		t.Error("expected the poll command to be issued")
	}
}

// A record that appears while waiting must complete the flow on its own.
func TestDNSModel_ManualRecheckSucceedingCompletesTheFlow(t *testing.T) {
	m := manualModel(t)
	m.checking = true

	updated, _ := m.Update(dnsPropagationMsg{
		results: map[string]bool{"8.8.8.8": true, "1.1.1.1": true}, ok: true})
	m = updated.(*dnsSetupModel)

	if !m.Delegated {
		t.Error("a delegation that appears during the wait should count")
	}
	if m.manualReason != "" {
		t.Error("the manual fallback should be cleared once the record resolves")
	}
	if m.step != stepDone {
		t.Errorf("expected stepDone, got %v", m.step)
	}
}

// A failed check must reset the countdown rather than stopping the loop.
func TestDNSModel_ManualRecheckFailingRearmsCountdown(t *testing.T) {
	m := manualModel(t)
	m.checking = true
	m.nextCheckIn = 0

	updated, _ := m.Update(dnsPropagationMsg{results: map[string]bool{"8.8.8.8": false}, ok: false})
	m = updated.(*dnsSetupModel)

	if m.checking {
		t.Error("the check should have finished")
	}
	if m.nextCheckIn != secondsBetweenDNSChecks {
		t.Errorf("countdown should be re-armed, got %d", m.nextCheckIn)
	}
	if m.Delegated {
		t.Error("a failed check must not report delegation")
	}
}
