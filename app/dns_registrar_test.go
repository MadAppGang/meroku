package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

var testZoneNS = []string{"ns-833.awsdns-40.net", "ns-1056.awsdns-04.org", "ns-63.awsdns-07.com", "ns-1907.awsdns-46.co.uk"}

func TestDecideRegistrarAction(t *testing.T) {
	other := []string{"ns-1627.awsdns-11.co.uk", "ns-1005.awsdns-61.net"}
	cases := map[string]struct {
		reg  domainRegistration
		want registrarAction
	}{
		"already set":        {domainRegistration{Nameservers: testZoneNS, LiveRecords: 5}, registrarAlreadySet},
		"empty zone behind":  {domainRegistration{Nameservers: other, LiveRecords: 0}, registrarAutoSwitch},
		"records behind":     {domainRegistration{Nameservers: other, LiveRecords: 3}, registrarConfirmSwitch},
		"cannot see records": {domainRegistration{Nameservers: other, LiveRecords: -1}, registrarConfirmSwitch},
	}
	for name, tc := range cases {
		if got := decideRegistrarAction(tc.reg, testZoneNS); got != tc.want {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
		}
	}
}

func TestFindDomainRegistrationScansEveryProfile(t *testing.T) {
	previous := registrationLookup
	t.Cleanup(func() { registrationLookup = previous })
	registrationLookup = func(_ context.Context, profile, domain string) (*domainRegistration, error) {
		switch profile {
		case "owner":
			return &domainRegistration{Profile: profile, Domain: domain}, nil
		case "expired":
			return nil, errors.New("sso token expired")
		}
		return nil, nil
	}

	scan := findDomainRegistration(context.Background(), []string{"a", "owner", "expired", "b"}, "example.work")
	if scan.Registration == nil || scan.Registration.Profile != "owner" {
		t.Fatalf("registration = %+v, want the owner profile", scan.Registration)
	}
	if len(scan.Unreachable) != 1 || scan.Unreachable[0] != "expired" {
		t.Errorf("unreachable = %v, want [expired]", scan.Unreachable)
	}
}

func rootDomainModel() *dnsSetupModel {
	return newDNSSetupModel(Env{Env: "dev"}, dnsPreflightResult{
		ZoneName: "example.work", ZoneID: "Z1", ZoneNameservers: testZoneNS,
	})
}

// A root domain whose registration sits in a reachable account is delegated
// with no keypress when the nameservers it replaces serve nothing.
func TestRootDomainSwitchesTheRegistrarByItself(t *testing.T) {
	previous := registrarNameserverWriter
	t.Cleanup(func() { registrarNameserverWriter = previous })
	var wrote []string
	registrarNameserverWriter = func(_ context.Context, profile, domain string, ns []string) error {
		wrote = append([]string{profile, domain}, ns...)
		return nil
	}

	m := rootDomainModel()
	_, cmd := m.Update(dnsRegistrationMsg{scan: registrationScan{Registration: &domainRegistration{
		Profile: "owner", Domain: "example.work",
		Nameservers: []string{"ns-1.awsdns-00.com"}, LiveRecords: 0, OldZoneID: "/hostedzone/ZOLD",
	}}})
	if cmd == nil || m.confirmRegistrar || m.manualReason != "" {
		t.Fatalf("expected an automatic write; confirm=%v manual=%q", m.confirmRegistrar, m.manualReason)
	}
	msg := cmd()
	if len(wrote) != 6 || wrote[0] != "owner" || wrote[1] != "example.work" {
		t.Fatalf("writer called with %v", wrote)
	}

	m.Update(msg)
	if m.states[stepWriteRecord] != stepOK || m.step != stepPropagate {
		t.Errorf("after the write: step=%v state=%v, want propagate", m.step, m.states[stepWriteRecord])
	}
}

func TestRootDomainAsksBeforeReplacingLiveNameservers(t *testing.T) {
	m := rootDomainModel()
	_, cmd := m.Update(dnsRegistrationMsg{scan: registrationScan{Registration: &domainRegistration{
		Profile: "owner", Domain: "example.work",
		Nameservers: []string{"ns-1.awsdns-00.com"}, LiveRecords: 4,
	}}})
	if cmd != nil || !m.confirmRegistrar {
		t.Fatal("nameservers serving records must not be replaced without a confirmation")
	}
	if !strings.Contains(m.View(), "4 records") {
		t.Error("the confirmation must say how many records stop resolving")
	}
}

func TestRootDomainWithoutAReachableRegistrationFallsBackToManual(t *testing.T) {
	m := rootDomainModel()
	m.Update(dnsRegistrationMsg{scan: registrationScan{Unreachable: []string{"alpha"}}})
	if m.manualReason == "" || !strings.Contains(m.manualReason, "alpha") {
		t.Fatalf("manual reason = %q", m.manualReason)
	}
	view := m.View()
	for _, bad := range []string{"into  ", "once  ", "MOVE  TO"} {
		if strings.Contains(view, bad) {
			t.Errorf("view has an empty parent-domain gap %q", bad)
		}
	}
	if strings.Contains(view, "✗ Find") {
		t.Error("not finding a registration is the manual path, not a failure")
	}
}

// With the canary written, the propagate check counts resolvers that return
// its value, not resolvers whose nameservers match.
func TestPropagationUsesTheCanary(t *testing.T) {
	prevCheck, prevDeleg := canaryCheck, delegationLookup
	t.Cleanup(func() { canaryCheck, delegationLookup = prevCheck, prevDeleg })
	delegationLookup = func(string) ([]string, error) { return testZoneNS, nil }
	asked := 0
	canaryCheck = func(c dnsCanary) map[string]dohVerdict {
		asked++
		if c.Token != "meroku-test" {
			t.Errorf("token = %q", c.Token)
		}
		return map[string]dohVerdict{"Google": dohResolved, "Cloudflare": dohResolved, "AdGuard": dohStale, "NextDNS": dohNotYet}
	}

	m := rootDomainModel()
	m.Update(dnsCanaryReadyMsg{canary: dnsCanary{Zone: "example.work", Token: "meroku-test"}})
	msg := m.pollPropagationCmd()().(dnsPropagationMsg)
	if asked != 1 || !msg.ok {
		t.Errorf("asked=%d ok=%v, want the canary to decide", asked, msg.ok)
	}
}

func TestCanaryNamesAreFreshUnderTheWildcard(t *testing.T) {
	c := dnsCanary{Zone: "example.work."}
	if c.wildcard() != "*._meroku-check.example.work" {
		t.Errorf("wildcard = %q", c.wildcard())
	}
	a, b := c.freshName(), c.freshName()
	if a == b || !strings.HasSuffix(a, "._meroku-check.example.work") {
		t.Errorf("fresh names %q %q", a, b)
	}
}

// [s] on the propagate screen goes on with the deploy; it never cancels it.
func TestStopWaitingContinuesTheDeploy(t *testing.T) {
	m := rootDomainModel()
	m.step = stepPropagate
	m.firstAgreementAt = time.Now()
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if !m.Delegated || m.ContinueAnyway {
		t.Errorf("after agreement: delegated=%v continueAnyway=%v", m.Delegated, m.ContinueAnyway)
	}

	m = rootDomainModel()
	m.step = stepPropagate
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if m.Delegated || !m.ContinueAnyway {
		t.Errorf("before agreement: delegated=%v continueAnyway=%v", m.Delegated, m.ContinueAnyway)
	}
}
