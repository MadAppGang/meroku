package main

import (
	"context"
	"slices"
	"testing"
)

var (
	zoneA = hostedZone{ID: "ZAAAA", Nameservers: []string{"ns-1.awsdns-01.net", "ns-2.awsdns-02.org"}}
	zoneB = hostedZone{ID: "ZBBBB", Nameservers: []string{"ns-3.awsdns-03.com", "ns-4.awsdns-04.co.uk"}}
)

func TestChooseZone(t *testing.T) {
	cases := []struct {
		name      string
		state     stateZone
		zones     []hostedZone
		delegated []string
		want      string
		adopt     bool
		replaces  string
		strays    []string
		blocked   bool
	}{
		{name: "no zone anywhere", state: stateZone{Known: true}},
		{name: "state owns the only zone", state: stateZone{Known: true, ZoneID: "ZAAAA"},
			zones: []hostedZone{zoneA}, delegated: zoneA.Nameservers, want: "ZAAAA"},
		{name: "state owns it, delegation elsewhere outside AWS", state: stateZone{Known: true, ZoneID: "ZAAAA"},
			zones: []hostedZone{zoneA}, delegated: []string{"ns1.registrar.example"}, want: "ZAAAA"},
		{name: "zone left behind, state empty", state: stateZone{Known: true},
			zones: []hostedZone{zoneA}, want: "ZAAAA", adopt: true},
		{name: "state owns one, delegation points at the other", state: stateZone{Known: true, ZoneID: "ZBBBB"},
			zones: []hostedZone{zoneA, zoneB}, delegated: zoneA.Nameservers,
			want: "ZAAAA", adopt: true, replaces: "ZBBBB", strays: []string{"ZBBBB"}},
		{name: "state owns the delegated one of two", state: stateZone{Known: true, ZoneID: "ZAAAA"},
			zones: []hostedZone{zoneA, zoneB}, delegated: zoneA.Nameservers, want: "ZAAAA", strays: []string{"ZBBBB"}},
		{name: "two zones, state empty, one delegated", state: stateZone{Known: true},
			zones: []hostedZone{zoneB, zoneA}, delegated: zoneA.Nameservers,
			want: "ZAAAA", adopt: true, strays: []string{"ZBBBB"}},
		{name: "two zones, none owned, none delegated", state: stateZone{Known: true},
			zones: []hostedZone{zoneA, zoneB}, blocked: true},
		{name: "state unreadable picks the delegated zone, imports nothing", state: stateZone{},
			zones: []hostedZone{zoneB, zoneA}, delegated: zoneA.Nameservers, want: "ZAAAA", strays: []string{"ZBBBB"}},
	}
	for _, tc := range cases {
		c := chooseZone(tc.state, tc.zones, tc.delegated)
		if (c.Blocked != "") != tc.blocked {
			t.Errorf("%s: blocked = %q", tc.name, c.Blocked)
			continue
		}
		if c.Zone.ID != tc.want || c.Adopt != tc.adopt || c.Replaces != tc.replaces || !slices.Equal(c.Strays, tc.strays) {
			t.Errorf("%s: got zone=%s adopt=%v replaces=%q strays=%v", tc.name, c.Zone.ID, c.Adopt, c.Replaces, c.Strays)
		}
	}
}

// Two zones share the name; state holds the one nobody delegates to. The
// pre-flight must not approve the delegated zone while the apply writes to
// the other one.
func TestPreflightAdoptsTheDelegatedZoneOverTheOneInState(t *testing.T) {
	prevZones, prevState, prevDeleg := hostedZonesLookup, stateZoneLookup, delegationLookup
	t.Cleanup(func() { hostedZonesLookup, stateZoneLookup, delegationLookup = prevZones, prevState, prevDeleg })
	hostedZonesLookup = func(context.Context, string, string) ([]hostedZone, error) {
		return []hostedZone{zoneA, zoneB}, nil
	}
	stateZoneLookup = func(Env) stateZone { return stateZone{Known: true, ZoneID: "ZBBBB"} }
	delegationLookup = func(string) ([]string, error) { return zoneA.Nameservers, nil }

	e := Env{Env: "prod", Domain: Domain{Enabled: true, CreateDomainZone: true, DomainName: "example.app"}}
	res, err := checkDNSPreflight(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	if res.Plan != dnsPlanAdopt || res.ZoneID != "ZAAAA" || res.AdoptReplaces != "ZBBBB" {
		t.Fatalf("plan=%v zone=%s replaces=%s, want adopt ZAAAA in place of ZBBBB", res.Plan, res.ZoneID, res.AdoptReplaces)
	}
}

func TestZoneImportBlock(t *testing.T) {
	want := "import {\n  to = module.domain.aws_route53_zone.domain[0]\n  id = \"ZAAAA\"\n}\n"
	if got := zoneImportBlock("ZAAAA"); got != want {
		t.Errorf("import block:\n%s", got)
	}
}

func TestStateHasAddress(t *testing.T) {
	list := "module.domain.aws_acm_certificate.api_domain\nmodule.domain.aws_route53_zone.domain[0]\n"
	if !stateHasAddress(list, zoneTargetAddress) {
		t.Error("address present in the list")
	}
	if stateHasAddress("module.domain.aws_route53_record.apex_ns[0]\n", zoneTargetAddress) {
		t.Error("address absent from the list")
	}
	m := zoneIDInState.FindStringSubmatch("    name    = \"example.app\"\n    zone_id = \"Z07327561KIEG48N8VG1I\"\n")
	if m == nil || m[1] != "Z07327561KIEG48N8VG1I" {
		t.Errorf("zone_id parse = %v", m)
	}
}
