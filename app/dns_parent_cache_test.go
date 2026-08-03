package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// inTempDir runs fn with the working directory pointed at a fresh temp dir, so
// tests that touch dns.yaml never see or clobber a real one.
func inTempDir(t *testing.T, fn func()) {
	t.Helper()

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(orig); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	fn()
}

// The cache is keyed by parent domain, which is the whole point: it is what
// makes the *next* environment cheap. Keying by subdomain (as DelegatedZones
// does) could never help, because staging.example.com is a different key from
// dev.example.com.
func TestParentZoneCacheIsKeyedByParentDomain(t *testing.T) {
	inTempDir(t, func() {
		err := recordDelegation(delegationRecord{
			Subdomain:       "dev.example.com",
			AccountID:       "111122223333",
			ZoneID:          "Zdev",
			Nameservers:     []string{"ns-1.example.net"},
			ParentDomain:    "example.com",
			ParentProfile:   "mag",
			ParentZoneID:    "Zparent",
			ParentAccountID: "999988887777",
		})
		if err != nil {
			t.Fatalf("recordDelegation: %v", err)
		}

		// A different environment of the same project must find the answer.
		profile, ok := cachedParentProfile("example.com")
		if !ok {
			t.Fatal("expected the parent profile to be remembered")
		}
		if profile != "mag" {
			t.Errorf("got profile %q, want mag", profile)
		}

		cfg, err := loadDNSConfig()
		if err != nil {
			t.Fatalf("loadDNSConfig: %v", err)
		}
		if findDelegatedZone(cfg, "dev.example.com") == nil {
			t.Error("the delegated zone should still be recorded for dns status")
		}
	})
}

func TestParentZoneCacheMissesAreQuiet(t *testing.T) {
	inTempDir(t, func() {
		// No dns.yaml at all.
		if _, ok := cachedParentProfile("example.com"); ok {
			t.Error("expected no cached profile when dns.yaml does not exist")
		}

		if err := recordDelegation(delegationRecord{
			Subdomain: "dev.example.com", ParentDomain: "example.com", ParentProfile: "mag",
		}); err != nil {
			t.Fatalf("recordDelegation: %v", err)
		}
		if _, ok := cachedParentProfile("other.com"); ok {
			t.Error("a different domain must not match")
		}
	})
}

// Route53 reports zone names with a trailing dot; operators type them without
// one. A cache that distinguished the two would silently never hit.
func TestParentZoneLookupNormalizesDomains(t *testing.T) {
	cfg := &DNSConfig{}
	addOrUpdateParentZone(cfg, ParentZoneRef{Domain: "Example.COM.", Profile: "mag"})

	for _, q := range []string{"example.com", "example.com.", "EXAMPLE.com", " example.com "} {
		if ref := findParentZone(cfg, q); ref == nil {
			t.Errorf("lookup %q did not match the stored zone", q)
		}
	}
}

func TestAddOrUpdateParentZoneReplacesInPlace(t *testing.T) {
	cfg := &DNSConfig{}
	addOrUpdateParentZone(cfg, ParentZoneRef{Domain: "example.com", Profile: "old", ZoneID: "Z1"})
	addOrUpdateParentZone(cfg, ParentZoneRef{Domain: "example.com.", Profile: "new", ZoneID: "Z2"})

	if len(cfg.ParentZones) != 1 {
		t.Fatalf("expected one entry, got %d", len(cfg.ParentZones))
	}
	if cfg.ParentZones[0].Profile != "new" || cfg.ParentZones[0].ZoneID != "Z2" {
		t.Errorf("entry was not replaced: %+v", cfg.ParentZones[0])
	}
}

// An entry with no profile is not a usable hint, and writing one would make
// cachedParentProfile return an empty profile name.
func TestAddOrUpdateParentZoneRejectsIncomplete(t *testing.T) {
	cfg := &DNSConfig{}
	addOrUpdateParentZone(cfg, ParentZoneRef{Domain: "example.com"})
	addOrUpdateParentZone(cfg, ParentZoneRef{Profile: "mag"})

	if len(cfg.ParentZones) != 0 {
		t.Errorf("expected incomplete refs to be dropped, got %+v", cfg.ParentZones)
	}
}

// Results now arrive in completion order, so the sort must impose an order the
// operator can predict rather than one that depends on credential latency.
func TestSortCandidatesIsDeterministic(t *testing.T) {
	build := func() []parentZoneCandidate {
		return []parentZoneCandidate{
			{Profile: "zeta", Err: errors.New("expired")},
			{Profile: "beta", ZoneID: "Z2"},
			{Profile: "alpha"},
			{Profile: "delta", ZoneID: "Z4", Authoritative: true},
			{Profile: "charlie", ZoneID: "Z3", Authoritative: true},
		}
	}

	want := []string{"charlie", "delta", "beta", "alpha", "zeta"}
	for run := 0; run < 3; run++ {
		got := build()
		sortCandidates(got)
		for i, w := range want {
			if got[i].Profile != w {
				t.Fatalf("run %d position %d: got %q, want %q (full: %v)",
					run, i, got[i].Profile, w, profileNames(got))
			}
		}
	}
}

func profileNames(cs []parentZoneCandidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Profile
	}
	return out
}

// The stream must always close, even when every profile fails, or the TUI would
// wait forever for a dnsScanDoneMsg.
func TestScanStreamAlwaysCloses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Profiles that do not exist locally: each probe errors fast.
	profiles := []string{"meroku-test-missing-1", "meroku-test-missing-2"}

	count := 0
	for range scanProfilesForParentZoneStream(ctx, profiles, "example.invalid", []string{"ns-1.example.net"}) {
		count++
	}
	if count != len(profiles) {
		t.Errorf("expected %d results, got %d", len(profiles), count)
	}
}

// Cancelling must not deadlock on a channel nobody is draining — this is what
// happens every time the operator picks a profile before the scan finishes.
func TestScanStreamStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := scanProfilesForParentZoneStream(ctx,
		[]string{"a", "b", "c", "d", "e", "f", "g", "h"},
		"example.invalid", []string{"ns-1.example.net"})

	// Take one result, then walk away.
	<-ch
	cancel()

	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("stream did not close after cancel")
	}
}

// applyDelegation must authenticate as the profile the operator chose.
//
// Regression test for a real failure: createNSRecordDelegation took an unused
// second `childProfile` parameter, the call passed the chosen profile there,
// and the credential-bearing first parameter got "". The SDK then fell back to
// the ambient AWS_PROFILE — the account being deployed into — and tried to
// write to a hosted zone in a different account. The resulting AccessDenied
// looked like a permissions problem on the operator's side; it was not.
func TestApplyDelegationUsesTheChosenProfile(t *testing.T) {
	var gotProfile, gotZone, gotSubdomain string
	var gotNS []string

	orig := delegationWriter
	origVerify := delegationVerifier
	defer func() { delegationWriter = orig; delegationVerifier = origVerify }()

	delegationWriter = func(profile, zoneID, subdomain string, ns []string) error {
		gotProfile, gotZone, gotSubdomain, gotNS = profile, zoneID, subdomain, ns
		return nil
	}
	// Stub the read-back too; this test is about what applyDelegation forwards,
	// and it must not depend on a live zone.
	delegationVerifier = func(_ context.Context, _, _, _ string, _ []string) (delegationCheck, error) {
		return delegationCheck{Present: true, Matches: true}, nil
	}

	err := applyDelegation(delegationRequest{
		ParentProfile: "mag",
		ParentZoneID:  "Z03970472IHTTTNCP0DZD",
		Subdomain:     "dev.coretechx.dev",
		Nameservers:   []string{"ns-644.awsdns-16.net", "ns-80.awsdns-10.com"},
	})
	if err != nil {
		t.Fatalf("applyDelegation: %v", err)
	}

	if gotProfile != "mag" {
		t.Errorf("wrote as profile %q, want mag — the operator's choice was dropped", gotProfile)
	}
	if gotZone != "Z03970472IHTTTNCP0DZD" {
		t.Errorf("zone = %q", gotZone)
	}
	if gotSubdomain != "dev.coretechx.dev" {
		t.Errorf("subdomain = %q", gotSubdomain)
	}
	if len(gotNS) != 2 {
		t.Errorf("nameservers = %v", gotNS)
	}
}

// An empty profile must fail loudly rather than silently authenticating as
// whatever AWS_PROFILE is set to.
func TestCreateNSRecordDelegationRejectsEmptyProfile(t *testing.T) {
	err := createNSRecordDelegation("", "Z1", "dev.example.com", []string{"ns-1.example.net"})
	if err == nil {
		t.Fatal("expected an error for an empty parent profile")
	}
	if !strings.Contains(err.Error(), "no AWS profile") {
		t.Errorf("error should name the cause, got: %v", err)
	}
}
