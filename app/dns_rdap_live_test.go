package main

import (
	"os"
	"testing"
)

// MEROKU_LIVE_RDAP=example.app asks that domain's registry over RDAP.
func TestRDAPNameserversLive(t *testing.T) {
	domain := os.Getenv("MEROKU_LIVE_RDAP")
	if domain == "" {
		t.Skip("set MEROKU_LIVE_RDAP to a registered domain")
	}
	ns, err := rdapNameservers(domain)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: %v", domain, ns)
	if len(ns) == 0 {
		t.Error("no nameservers")
	}
	via, err := delegationLookup(domain)
	t.Logf("delegationLookup: %v err=%v", via, err)
}
