package main

import (
	"os"
	"testing"

	"github.com/miekg/dns"
)

func TestDelegatedNameserversReadsTheReferral(t *testing.T) {
	resp := new(dns.Msg)
	for _, rr := range []string{
		"example.app. 172800 IN NS ns-1.awsdns-00.com.",
		"example.app. 172800 IN NS ns-2.awsdns-00.net.",
		"other.app. 172800 IN NS ns-9.awsdns-00.org.",
	} {
		r, err := dns.NewRR(rr)
		if err != nil {
			t.Fatal(err)
		}
		resp.Ns = append(resp.Ns, r)
	}
	got := delegatedNameservers(resp, "example.app")
	if len(got) != 2 || got[0] != "ns-1.awsdns-00.com" || got[1] != "ns-2.awsdns-00.net" {
		t.Errorf("delegatedNameservers = %v", got)
	}
}

// Resolver caches holding an older nameserver set must not keep a delegation
// the parent already publishes in "pending".
func TestPropagationTrustsTheParentOverStaleCaches(t *testing.T) {
	previous := delegationLookup
	t.Cleanup(func() { delegationLookup = previous })
	delegationLookup = func(string) ([]string, error) { return testZoneNS, nil }

	m := rootDomainModel()
	m.manualReason = "set the nameservers yourself"
	m.checking = true
	m.Update(dnsPropagationMsg{
		results:  map[string]dohVerdict{"Google": dohStale, "Cloudflare": dohResolved, "AdGuard": dohStale, "NextDNS": dohStale},
		ok:       true,
		atParent: true,
	})
	if m.checking {
		t.Error("the manual countdown stayed frozen on checking")
	}
	if m.firstAgreementAt.IsZero() {
		t.Error("a delegation published at the parent did not start the settle window")
	}
}

// MEROKU_LIVE_DNS=example.app runs the lookup against the real registry.
func TestDelegationAtParentLive(t *testing.T) {
	zone := os.Getenv("MEROKU_LIVE_DNS")
	if zone == "" {
		t.Skip("set MEROKU_LIVE_DNS to a delegated domain")
	}
	ns, err := delegationAtParent(zone)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s is delegated to %v", zone, ns)
	if len(ns) == 0 {
		t.Error("no delegation found")
	}
}
