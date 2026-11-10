package main

import (
	"context"
	"errors"
	"strings"
	"testing"
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
	previous, previousDelete := registrarNameserverWriter, emptyZoneDeleter
	t.Cleanup(func() { registrarNameserverWriter, emptyZoneDeleter = previous, previousDelete })
	var wrote []string
	registrarNameserverWriter = func(_ context.Context, profile, domain string, ns []string) error {
		wrote = append([]string{profile, domain}, ns...)
		return nil
	}
	deleted := ""
	emptyZoneDeleter = func(_ context.Context, profile, zoneID string) error {
		deleted = profile + "/" + zoneID
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
	if deleted != "owner//hostedzone/ZOLD" {
		t.Errorf("the old empty zone was not deleted (got %q)", deleted)
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
