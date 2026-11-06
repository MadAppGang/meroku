package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestSuggestSESSPF(t *testing.T) {
	cases := []struct{ name, live, want string }{
		{"no SPF", "", "v=spf1 include:amazonses.com ~all"},
		{"soft fail", "v=spf1 include:_spf.google.com ~all", "v=spf1 include:_spf.google.com include:amazonses.com ~all"},
		{"hard fail", "v=spf1 mx -all", "v=spf1 mx include:amazonses.com -all"},
		{"neutral", "v=spf1 a ?all", "v=spf1 a include:amazonses.com ?all"},
		{"bare all", "v=spf1 a all", "v=spf1 a include:amazonses.com all"},
		{"redirect", "v=spf1 redirect=_spf.example.com", "v=spf1 redirect=_spf.example.com include:amazonses.com"},
		{"already present", "v=spf1 include:amazonses.com -all", "v=spf1 include:amazonses.com -all"},
	}
	for _, c := range cases {
		if got := suggestSESSPF(c.live); got != c.want {
			t.Errorf("%s: suggestSESSPF(%q) = %q, want %q", c.name, c.live, got, c.want)
		}
	}
}

func sesDNSTestEnvMap() map[string]interface{} {
	return map[string]interface{}{
		"project":     "myapp",
		"env":         "dev",
		"region":      "us-east-1",
		"aws_profile": "fake-profile",
		"domain": map[string]interface{}{
			"enabled":     true,
			"domain_name": "example.net",
			"additional_domains": []interface{}{
				map[string]interface{}{"domain": "example.org", "create_zone": true},
			},
		},
		"ses": map[string]interface{}{
			"enabled": true,
			"domains": []interface{}{
				map[string]interface{}{"domain": "example.com", "zone_id": "Z000000000000A"},
				map[string]interface{}{"domain": "mail.example.org"},
			},
		},
	}
}

// sesDNSAPITest runs the handler from a temporary project root holding
// dev.yaml, with the env loader and the live reader replaced.
func sesDNSAPITest(t *testing.T, query url.Values, read sesDNSLiveReader) *httptest.ResponseRecorder {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dev.yaml"), []byte("project: myapp\nenv: dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	oldLoad, oldRead := sesDNSAPILoadEnvMap, sesDNSAPIReader
	sesDNSAPILoadEnvMap = func(string) (map[string]interface{}, error) { return sesDNSTestEnvMap(), nil }
	sesDNSAPIReader = read
	t.Cleanup(func() { sesDNSAPILoadEnvMap, sesDNSAPIReader = oldLoad, oldRead })

	w := httptest.NewRecorder()
	getSESDNSReport(w, httptest.NewRequest(http.MethodGet, "/api/ses-dns?"+query.Encode(), nil))
	return w
}

func decodeSESDNSReport(t *testing.T, w *httptest.ResponseRecorder) sesDNSReport {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var rep sesDNSReport
	if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestGetSESDNSReport_RejectsCallerMistakes(t *testing.T) {
	never := func(context.Context, string, string, string, string) (*int64, []string, error) {
		t.Fatal("the live reader ran for a refused request")
		return nil, nil, nil
	}
	for _, q := range []url.Values{
		{"env": {"dev"}},
		{"domain": {"example.com"}},
		{"env": {"prod"}, "domain": {"example.com"}},
		{"env": {"dev"}, "domain": {"unknown.example.net"}},
	} {
		if w := sesDNSAPITest(t, q, never); w.Code != http.StatusBadRequest {
			t.Errorf("%v: status %d, want 400", q, w.Code)
		}
	}
}

func TestGetSESDNSReport_ExistingZoneLiveApex(t *testing.T) {
	ttl := int64(3600)
	var gotZone, gotProfile string
	read := func(_ context.Context, profile, _, zoneID, name string) (*int64, []string, error) {
		gotZone, gotProfile = zoneID, profile
		return &ttl, []string{`"v=spf1 include:_spf.google.com ~all"`, `"google-site-verification=fake-token"`}, nil
	}
	rep := decodeSESDNSReport(t, sesDNSAPITest(t, url.Values{"env": {"dev"}, "domain": {"example.com"}}, read))

	if rep.ZoneOrigin != sesZoneExisting || rep.ZoneID != "Z000000000000A" {
		t.Errorf("zone = %q %q", rep.ZoneOrigin, rep.ZoneID)
	}
	if gotZone != "Z000000000000A" || gotProfile != "fake-profile" {
		t.Errorf("reader got zone %q profile %q", gotZone, gotProfile)
	}
	if rep.Live == nil || rep.Live.ApexTTL == nil || *rep.Live.ApexTTL != 3600 {
		t.Fatalf("live = %+v", rep.Live)
	}
	if len(rep.Live.SPF) != 1 || len(rep.Live.ApexOther) != 1 || rep.Live.ApexOther[0] != "google-site-verification=fake-token" {
		t.Errorf("live = %+v", rep.Live)
	}
	if rep.SuggestedSPF != "v=spf1 include:_spf.google.com include:amazonses.com ~all" {
		t.Errorf("suggested_spf = %q", rep.SuggestedSPF)
	}
}

func TestGetSESDNSReport_AWSFailureIsA200(t *testing.T) {
	read := func(context.Context, string, string, string, string) (*int64, []string, error) {
		return nil, nil, errors.New("no credentials")
	}
	rep := decodeSESDNSReport(t, sesDNSAPITest(t, url.Values{"env": {"dev"}, "domain": {"mail.example.org"}}, read))
	if rep.ZoneOrigin != sesZoneCreated {
		t.Errorf("zone_origin = %q", rep.ZoneOrigin)
	}
	if rep.Live == nil || rep.Live.ReadError != "no credentials" {
		t.Errorf("live = %+v", rep.Live)
	}
	if rep.SuggestedSPF != sesDefaultSPFRecord {
		t.Errorf("suggested_spf = %q", rep.SuggestedSPF)
	}
}

func TestInspectSESDNS_LegacySingleDomain(t *testing.T) {
	envMap := map[string]interface{}{
		"project": "myapp",
		"env":     "prod",
		"domain":  map[string]interface{}{"enabled": true, "domain_name": "example.com"},
		"ses":     map[string]interface{}{"enabled": true, "domain_name": "mail.example.com"},
	}
	rep, err := inspectSESDNS(context.Background(), envMap, "mail.example.com", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Domain != "mail.example.com" || rep.ZoneOrigin != sesZoneExisting {
		t.Errorf("report = %+v", rep)
	}
	if _, err := inspectSESDNS(context.Background(), envMap, "other.example.com", nil, nil); err == nil {
		t.Error("an unknown domain was accepted")
	}
}

func TestGetSESDNSReport_ChecksCandidateWithoutAWS(t *testing.T) {
	q := url.Values{"env": {"dev"}, "domain": {"example.com"}, "candidate": {"include:amazonses.com"}, "live": {"0"}}
	rep := decodeSESDNSReport(t, sesDNSAPITest(t, q, nil))
	if rep.Live != nil {
		t.Errorf("live read ran with live=0: %+v", rep.Live)
	}
	if len(rep.Issues) != 1 || rep.Issues[0].Code != "not_spf" {
		t.Errorf("issues = %+v", rep.Issues)
	}
}
