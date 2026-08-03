package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func lipglossWidth(s string) int { return lipgloss.Width(s) }

// flatten collapses a rendered view to single-spaced text so an assertion on a
// sentence is not defeated by wherever wordWrap broke the line.
//
// Box borders have to go too: a wrapped sentence inside a panel comes back as
// "resolves until you │ │ update the nameservers", which contains neither the
// original sentence nor anything an assertion can reasonably target.
// flattenStacked renders a model at a width narrow enough that the layout
// stacks into one column, then flattens it.
//
// Flattening a two-column layout interleaves the columns — "cannot add an NS
// record for a" from the left lands next to "You asked for the custom domain"
// from the right — so any assertion on a phrase has to be made against the
// single-column form.
func flattenStacked(m *dnsSetupModel) string {
	orig := m.width
	m.width = 80
	defer func() { m.width = orig }()
	return flatten(m.View())
}

func flatten(view string) string {
	cleaned := strings.Map(func(r rune) rune {
		if strings.ContainsRune("│─╭╮╰╯┃━┏┓┗┛", r) {
			return ' '
		}
		return r
	}, view)
	return strings.Join(strings.Fields(cleaned), " ")
}

func adoptingModel(t *testing.T) *dnsSetupModel {
	t.Helper()
	m := testDNSModel(t)
	m.parent = "sploty.app"
	m.env.AWSProfile = "meroku2"
	m.nameservers = []string{"ns-839.awsdns-40.net", "ns-1058.awsdns-04.org"}
	updated, _ := m.Update(dnsCandidatesMsg{reason: "sploty.app is not hosted on Route53 (ns1.hover.com)"})
	return updated.(*dnsSetupModel)
}

// [t] is only offered where it is the answer: the manual fallback.
func TestAdopt_OfferedFromManualFallback(t *testing.T) {
	m := adoptingModel(t)
	flat := flattenStacked(m)

	if !strings.Contains(flat, "ROUTE53") {
		t.Errorf("the adoption route should be offered as an action:\n%s", m.View())
	}
	// The reason it exists at all — a provider that cannot delegate a subdomain —
	// has to be on screen, or the option looks arbitrary.
	if !strings.Contains(flat, "cannot add an NS record for a subdomain") {
		t.Errorf("the action should say who it is for:\n%s", flat)
	}
	// And it must reassure that existing records survive, since that is the
	// question anyone with live mail on the domain will ask first.
	if !strings.Contains(flat, "copies your existing records") {
		t.Error("the action should say existing records are carried over")
	}
	if !hintBound(m.footerHints(), "t") {
		t.Error("and be bound in the key legend")
	}
}

// Nothing about adoption should be reachable while a normal delegation is
// possible — the whole premise is that the current host cannot delegate.
func TestAdopt_NotOfferedWhenDelegationIsPossible(t *testing.T) {
	m := testDNSModel(t)
	m.candidates = []parentZoneCandidate{{Profile: "mag", ZoneID: "Z1", Authoritative: true}}
	m.choosing = true
	m.step = stepWriteRecord

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = updated.(*dnsSetupModel)

	if m.adopt.phase != adoptOff {
		t.Error("[t] must do nothing while a delegable parent zone is on offer")
	}
}

// The deploy account is preselected, because that is where everything else the
// environment owns already lives.
func TestAdopt_PreselectsTheDeployProfile(t *testing.T) {
	m := adoptingModel(t)
	m.adopt.profiles = []string{"alpha", "meroku2", "mag"}
	m.adopt.profileIdx = 0
	for i, p := range m.adopt.profiles {
		if p == m.env.AWSProfile {
			m.adopt.profileIdx = i
		}
	}
	m.adopt.phase = adoptPickProfile

	if m.adopt.profile() != "meroku2" {
		t.Errorf("expected the deploy profile preselected, got %q", m.adopt.profile())
	}
	if !strings.Contains(m.View(), "this environment deploys here") {
		t.Error("the deploy account should be labelled as such")
	}
}

// The review step must show the incompleteness caveat above the record list.
// The operator is about to bet their mail on this list being right.
func TestAdopt_ReviewShowsIncompletenessWarning(t *testing.T) {
	m := adoptingModel(t)
	m.adopt.phase = adoptReview
	m.adopt.snapshot = zoneSnapshot{
		Domain:      "sploty.app",
		Method:      "probe sweep",
		NamesProbed: 74,
		Records: []dnsRecord{
			{Name: "sploty.app", Type: "A", Values: []string{"216.40.34.41"}},
			{Name: "sploty.app", Type: "MX", Values: []string{"10 mx.hover.com."}},
		},
		HasWildcard:  true,
		WildcardHint: "sploty.app answers for every name (a *.sploty.app wildcard, A).",
	}

	view := m.View()
	for _, want := range []string{"cannot be listed", "wildcard", "probe sweep"} {
		if !strings.Contains(view, want) {
			t.Errorf("review should surface %q:\n%s", want, view)
		}
	}
	if !strings.Contains(flatten(view), "nothing resolves differently yet") {
		t.Error("review should say the copy changes nothing yet")
	}
}

// A zone transfer is authoritative, so it must not carry the caveat.
func TestAdopt_ReviewOmitsWarningAfterZoneTransfer(t *testing.T) {
	m := adoptingModel(t)
	m.adopt.phase = adoptReview
	m.adopt.snapshot = zoneSnapshot{
		Domain:   "sploty.app",
		Method:   "zone transfer (AXFR)",
		Complete: true,
		Records:  []dnsRecord{{Name: "sploty.app", Type: "A", Values: []string{"192.0.2.1"}}},
	}
	if strings.Contains(m.View(), "cannot be listed") {
		t.Error("a complete zone transfer should not be hedged")
	}
}

// Mismatches found by the comparison must be shown before the nameserver step,
// because after it they are an outage rather than a warning.
func TestAdopt_VerifyShowsMismatchesBeforeTheSwitch(t *testing.T) {
	m := adoptingModel(t)
	m.adopt.phase = adoptVerify
	m.adopt.summary = adoptionSummary{
		Zone:    adoptedZone{ZoneID: "Z123", Nameservers: []string{"ns-1.awsdns.com"}},
		Written: []dnsRecord{{Name: "sploty.app", Type: "A"}},
	}
	m.adopt.diffs = []resolutionDiff{
		{Name: "sploty.app", Type: "A", Old: []string{"216.40.34.41"}, New: []string{"216.40.34.41"}, Match: true},
		{Name: "sploty.app", Type: "MX", Old: []string{"10 mx.hover.com."}, New: nil, Match: false},
	}

	view := m.View()
	if !strings.Contains(view, "1 record(s) differ") {
		t.Errorf("the mismatch count should be stated:\n%s", view)
	}
	if !strings.Contains(flatten(view), "deleting the new zone undoes all of this") {
		t.Error("the operator should be told this is still reversible")
	}
}

// Records Route53 refused must be listed, not swallowed — a partial copy plus a
// precise list beats all-or-nothing.
func TestAdopt_VerifyListsFailedRecords(t *testing.T) {
	m := adoptingModel(t)
	m.adopt.phase = adoptVerify
	m.adopt.summary = adoptionSummary{
		Zone: adoptedZone{ZoneID: "Z123"},
		Failures: []recordWriteFailure{{
			Record: dnsRecord{Name: "odd.sploty.app", Type: "DNSKEY"},
			Err:    errors.New("Route53 does not support DNSKEY records"),
		}},
	}
	m.adopt.diffs = []resolutionDiff{}

	view := m.View()
	if !strings.Contains(view, "add these by hand") || !strings.Contains(view, "odd.sploty.app") {
		t.Errorf("failed records must be named:\n%s", view)
	}
}

// Once the apex resolves to the new zone, the subdomain delegation is an
// ordinary write and the flow rejoins the normal path.
func TestAdopt_ApexGoingLiveTriggersDelegation(t *testing.T) {
	m := adoptingModel(t)
	m.adopt.phase = adoptWaitNS
	m.adopt.profiles = []string{"meroku2"}
	m.adopt.summary = adoptionSummary{Zone: adoptedZone{ZoneID: "ZNEW", Nameservers: []string{"ns-1.awsdns.com"}}}

	updated, cmd := m.Update(adoptNSLiveMsg{live: true})
	m = updated.(*dnsSetupModel)

	if m.adopt.phase != adoptOff {
		t.Error("adoption should be finished once the apex is live")
	}
	if m.manualReason != "" {
		t.Error("the manual fallback no longer applies once we own the parent zone")
	}
	if cmd == nil {
		t.Error("expected the subdomain delegation to start")
	}
}

// A registrar change that has not landed yet must re-arm rather than give up.
func TestAdopt_ApexNotLiveRearmsCountdown(t *testing.T) {
	m := adoptingModel(t)
	m.adopt.phase = adoptWaitNS
	m.adopt.checking = true
	m.adopt.nsCheckIn = 0

	updated, _ := m.Update(adoptNSLiveMsg{live: false})
	m = updated.(*dnsSetupModel)

	if m.adopt.checking {
		t.Error("the check should have finished")
	}
	if m.adopt.nsCheckIn != secondsBetweenDNSChecks {
		t.Errorf("countdown should be re-armed, got %d", m.adopt.nsCheckIn)
	}
	if m.adopt.phase != adoptWaitNS {
		t.Error("it should keep waiting")
	}
}

// Esc steps back through the flow rather than out of it: everything before the
// nameserver switch is reversible, so a second look should be cheap.
func TestAdopt_EscStepsBack(t *testing.T) {
	m := adoptingModel(t)
	m.adopt.phase = adoptReview

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(*dnsSetupModel)
	if m.adopt.phase != adoptPickProfile {
		t.Errorf("Esc from review should go back to the account picker, got %v", m.adopt.phase)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(*dnsSetupModel)
	if m.adopt.phase != adoptOff {
		t.Error("Esc from the picker should leave adoption entirely")
	}
}

// No adoption screen may overflow its frame.
func TestAdopt_NoLineExceedsWidth(t *testing.T) {
	phases := []adoptPhase{adoptPickProfile, adoptDiscovering, adoptReview, adoptCopying, adoptVerify, adoptWaitNS}
	for _, size := range []int{80, 100, 120, 200} {
		for _, phase := range phases {
			m := adoptingModel(t)
			m.width, m.height = size, 40
			m.adopt.phase = phase
			m.adopt.profiles = []string{"alpha", "meroku2", "a-very-long-profile-name-here"}
			m.adopt.snapshot = zoneSnapshot{
				Domain: "sploty.app", Method: "probe sweep", NamesProbed: 74,
				HasWildcard:  true,
				WildcardHint: "sploty.app answers for every name (a *.sploty.app wildcard, A). Names covered by it are not listed separately.",
				Records: []dnsRecord{
					{Name: "averyveryverylongsubdomain.sploty.app", Type: "TXT",
						Values: []string{`"v=spf1 include:_spf.google.com include:mailgun.org ~all"`}},
				},
			}
			m.adopt.summary = adoptionSummary{
				Zone: adoptedZone{ZoneID: "Z0580793YTBKHE7ID6NJ", Nameservers: []string{
					"ns-1930.awsdns-49.co.uk", "ns-1050.awsdns-03.org",
					"ns-678.awsdns-20.net", "ns-247.awsdns-30.com"}},
				Failures: []recordWriteFailure{{
					Record: dnsRecord{Name: "odd.sploty.app", Type: "DNSKEY"},
					Err:    errors.New("Route53 does not support DNSKEY records at all, sorry"),
				}},
			}
			m.adopt.diffs = []resolutionDiff{{
				Name: "averyveryverylongsubdomain.sploty.app", Type: "TXT",
				Old: []string{"something long here"}, New: nil, Match: false}}

			for n, line := range strings.Split(m.View(), "\n") {
				if w := lipglossWidth(line); w > size {
					t.Errorf("phase %v at %d cols: line %d is %d wide\n%q", phase, size, n, w, line)
				}
			}
		}
	}
}

// A finished flow must never keep rendering a sub-screen.
//
// Regression test for a run that showed every step green — including
// Propagate — while the body still said "SET THESE AT YOUR REGISTRAR ·
// rechecking in 2s". Two pollers were running at once: the adoption flow
// watching the apex, and the manual auto-recheck still armed from the screen
// adoption was launched from. The manual one saw the delegation resolve and
// completed the flow, but renderBody tested adopt.phase before the terminal
// state, so the finished run stayed hidden behind a screen describing work
// that had already happened.
func TestAdopt_CompletedFlowIsNotMaskedByAdoptScreen(t *testing.T) {
	m := adoptingModel(t)
	m.adopt.phase = adoptWaitNS
	m.adopt.summary = adoptionSummary{Zone: adoptedZone{ZoneID: "ZNEW",
		Nameservers: []string{"ns-423.awsdns-52.com"}}}

	// The delegation resolves by some other route while adoption waits.
	updated, _ := m.Update(dnsPropagationMsg{
		results: map[string]bool{"8.8.8.8": true, "1.1.1.1": true}, ok: true})
	m = updated.(*dnsSetupModel)

	if !m.Delegated || m.step != stepDone {
		t.Fatalf("expected the flow to complete, got step %v delegated=%v", m.step, m.Delegated)
	}
	if m.adopt.phase != adoptOff {
		t.Error("the adoption sub-flow should end once the delegation is live")
	}

	view := flattenStacked(m)
	if strings.Contains(view, "SET THESE AT YOUR REGISTRAR") {
		t.Errorf("a completed run must not still show the registrar screen:\n%s", view)
	}
	if strings.Contains(view, "rechecking in") {
		t.Error("a completed run must not still be counting down")
	}
	if !strings.Contains(view, "DELEGATED") {
		t.Errorf("the done state should be on screen:\n%s", view)
	}
}

// Only one countdown may run at a time. Two pollers on one screen is what let
// the flow finish by a route the visible sub-screen knew nothing about.
func TestAdopt_ManualRecheckPausesWhileAdopting(t *testing.T) {
	m := adoptingModel(t)
	m.adopt.phase = adoptWaitNS
	// The adoption countdown must be mid-flight, or its own branch fires on this
	// tick and returns before the main handler is reached — which would make this
	// test pass without ever exercising the thing it is checking.
	m.adopt.nsCheckIn = 10
	m.nextCheckIn = 1

	updated, _ := m.Update(dnsTickMsg(time.Now()))
	m = updated.(*dnsSetupModel)

	if m.checking {
		t.Error("the manual recheck must not fire while the adoption flow owns the screen")
	}
	if m.adopt.nsCheckIn != 9 {
		t.Errorf("the adoption countdown should be the one advancing, got %d", m.adopt.nsCheckIn)
	}
}

// The apex watcher must recognise the thing it is ultimately waiting for, not
// only the step it happens to be on.
func TestAdopt_SubdomainGoingLiveFinishesEverything(t *testing.T) {
	m := adoptingModel(t)
	m.adopt.phase = adoptWaitNS
	m.states[stepFindParent] = stepFailed

	updated, _ := m.Update(adoptNSLiveMsg{live: true, subdomainLive: true})
	m = updated.(*dnsSetupModel)

	if !m.Delegated || m.step != stepDone {
		t.Error("a subdomain that already resolves should finish the flow")
	}
	if m.adopt.phase != adoptOff {
		t.Error("adoption should be over")
	}
	if m.states[stepFindParent] != stepOK {
		t.Error("a step left failed from an earlier attempt should not stay red once it succeeded another way")
	}
}
