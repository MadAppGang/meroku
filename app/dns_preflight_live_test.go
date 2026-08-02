package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// TestCheckDNSPreflight_Live exercises the router against a real AWS account and
// real public DNS. It is skipped unless MEROKU_E2E_DNS=1 so ordinary `go test`
// runs stay hermetic.
//
//	MEROKU_E2E_DNS=1 \
//	MEROKU_E2E_PROFILE=circl-dev \
//	MEROKU_E2E_DOMAIN=coretechx.dev \
//	MEROKU_E2E_ENV=dev \
//	go test -run TestCheckDNSPreflight_Live -v ./app
//
// All calls are read-only: ListHostedZonesByName, GetHostedZone and NS resolution.
func TestCheckDNSPreflight_Live(t *testing.T) {
	if os.Getenv("MEROKU_E2E_DNS") != "1" {
		t.Skip("set MEROKU_E2E_DNS=1 to run the live DNS preflight check")
	}

	profile := os.Getenv("MEROKU_E2E_PROFILE")
	domain := os.Getenv("MEROKU_E2E_DOMAIN")
	envName := os.Getenv("MEROKU_E2E_ENV")
	if profile == "" || domain == "" || envName == "" {
		t.Fatal("MEROKU_E2E_PROFILE, MEROKU_E2E_DOMAIN and MEROKU_E2E_ENV are all required")
	}

	e := Env{
		Env:        envName,
		AWSProfile: profile,
		Domain: Domain{
			Enabled:            true,
			CreateDomainZone:   true,
			DomainName:         domain,
			AddEnvDomainPrefix: true,
		},
	}

	res, err := checkDNSPreflight(context.Background(), e)
	if err != nil {
		t.Fatalf("preflight failed: %v", err)
	}

	t.Logf("plan            = %s", res.Plan)
	t.Logf("zone name       = %s", res.ZoneName)
	t.Logf("parent domain   = %s", res.ParentDomain)
	t.Logf("zone id         = %s", res.ZoneID)
	t.Logf("zone NS         = %v", res.ZoneNameservers)
	t.Logf("public NS       = %v", res.PublicNameservers)
	t.Logf("reason          = %s", res.Reason)
	t.Logf("needs delegation= %v", res.NeedsDelegation())
	t.Logf("\n%s", describeDNSPreflight(res))
}

// TestScanProfilesForParentZone_Live checks that the profile scan finds the
// parent zone and correctly marks exactly the authoritative one. Read-only:
// ListHostedZonesByName, GetHostedZone and STS GetCallerIdentity per profile.
//
// Guarded by MEROKU_E2E_DNS=1, same as above.
func TestScanProfilesForParentZone_Live(t *testing.T) {
	if os.Getenv("MEROKU_E2E_DNS") != "1" {
		t.Skip("set MEROKU_E2E_DNS=1 to run the live profile scan")
	}

	parent := os.Getenv("MEROKU_E2E_PARENT")
	if parent == "" {
		t.Fatal("MEROKU_E2E_PARENT is required (e.g. coretechx.dev)")
	}

	publicNS, err := queryNameservers(parent)
	if err != nil {
		t.Fatalf("could not resolve public nameservers for %s: %v", parent, err)
	}
	t.Logf("public NS for %s = %v", parent, publicNS)

	if !looksLikeRoute53(publicNS) {
		t.Skipf("%s is not on Route53, nothing to scan for", parent)
	}

	profiles, err := getLocalAWSProfiles()
	if err != nil {
		t.Fatalf("could not list local AWS profiles: %v", err)
	}
	t.Logf("scanning %d profiles", len(profiles))

	candidates := scanProfilesForParentZone(context.Background(), profiles, parent, publicNS)

	authoritative := 0
	for _, c := range candidates {
		if c.ZoneID == "" && c.Err == nil {
			continue // no zone here; uninteresting
		}
		t.Logf("  %s", c.Label(parent))
		if c.Authoritative {
			authoritative++
		}
	}

	if authoritative != 1 {
		t.Errorf("expected exactly one authoritative candidate for %s, found %d", parent, authoritative)
	}
}

// TestDNSStatusEndpoint_Live exercises the HTTP handler against a real project
// directory and real AWS. Read-only.
//
//	MEROKU_E2E_DNS=1 MEROKU_E2E_PROJECT=/path/to/project MEROKU_E2E_ENV=dev \
//	go test -run TestDNSStatusEndpoint_Live -v ./app
func TestDNSStatusEndpoint_Live(t *testing.T) {
	if os.Getenv("MEROKU_E2E_DNS") != "1" {
		t.Skip("set MEROKU_E2E_DNS=1 to run the live DNS endpoint check")
	}
	projectDir := os.Getenv("MEROKU_E2E_PROJECT")
	envName := os.Getenv("MEROKU_E2E_ENV")
	if projectDir == "" || envName == "" {
		t.Fatal("MEROKU_E2E_PROJECT and MEROKU_E2E_ENV are required")
	}

	// The handler resolves the environment relative to the working directory.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir %s: %v", projectDir, err)
	}
	defer os.Chdir(wd)

	rec := httptest.NewRecorder()
	getDNSStatus(rec, httptest.NewRequest(http.MethodGet, "/api/dns/status?env="+envName, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp dnsStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}

	t.Logf("plan=%s zone=%s id=%s", resp.Plan, resp.ZoneName, resp.ZoneID)
	t.Logf("delegated=%v needs_delegation=%v parent_is_route53=%v can_auto_delegate=%v",
		resp.Delegated, resp.NeedsDelegation, resp.ParentIsRoute53, resp.CanAutoDelegate)
	t.Logf("zone NS   = %v", resp.ZoneNameservers)
	t.Logf("public NS = %v", resp.PublicNameservers)
	t.Logf("reason: %s", resp.Reason)

	if resp.ZoneName == "" {
		t.Error("expected a zone name in the response")
	}
}
