package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// Decode the wire contract independently of vpcCIDRReport so incorrect JSON tags
// cannot be concealed by serializing and deserializing the same production type.
type cidrQAWireIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
	Fix     string `json:"fix"`
}
type cidrQAWireWarning struct {
	Message string `json:"message"`
	Hint    string `json:"hint"`
	Source  string `json:"source"`
}
type cidrQAWireCandidate struct {
	Value      string              `json:"value"`
	Valid      bool                `json:"valid"`
	Incomplete bool                `json:"incomplete"`
	Error      *cidrQAWireIssue    `json:"error"`
	Warnings   []cidrQAWireWarning `json:"warnings"`
}
type cidrQAWireReport struct {
	Env            string               `json:"env"`
	UseDefaultVPC  bool                 `json:"useDefaultVpc"`
	Override       bool                 `json:"override"`
	Configured     string               `json:"configured"`
	Effective      string               `json:"effective"`
	Error          *cidrQAWireIssue     `json:"error"`
	Warnings       []cidrQAWireWarning  `json:"warnings"`
	Suggestions    []string             `json:"suggestions"`
	AWSChecked     bool                 `json:"awsChecked"`
	AWSNote        string               `json:"awsNote"`
	Deployed       string               `json:"deployed"`
	DeployedReason string               `json:"deployedReason"`
	Candidate      *cidrQAWireCandidate `json:"candidate"`
}

// Serial tests only: both process cwd and the documented HTTP seams are global.
func cidrQASetupAPI(t *testing.T, fields map[string]interface{}, uses []vpcCIDRUse) string {
	t.Helper()
	dir := t.TempDir()
	chdir(t, dir)
	cidrQAWriteEnv(t, dir, "dev.yaml", "yaml-dev", "000000000000",
		"use_default_vpc: false\nvpc_cidr: 10.77.0.0/16\n")
	env := cidrQAEnv(t, fields)
	oldDeps, oldLoad := vpcCIDRAPIDeps, vpcCIDRAPILoadEnv
	vpcCIDRAPIDeps = func() vpcCIDRDeps { return cidrQADeps(uses) }
	// Deliberately accepts any argument: membership in the known file-stem
	// list is a handler requirement, not an assertion on this fake's calls.
	vpcCIDRAPILoadEnv = func(string) (Env, error) { return env, nil }
	t.Cleanup(func() {
		vpcCIDRAPIDeps, vpcCIDRAPILoadEnv = oldDeps, oldLoad
	})
	return dir
}

func cidrQAAPIRequest(method string, query url.Values) *httptest.ResponseRecorder {
	target := "/api/vpc-cidr"
	if len(query) != 0 {
		target += "?" + query.Encode()
	}
	w := httptest.NewRecorder()
	getVPCCIDRReport(w, httptest.NewRequest(method, target, nil))
	return w
}

func cidrQAReadAPIReport(t *testing.T, w *httptest.ResponseRecorder) (cidrQAWireReport, map[string]json.RawMessage) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var report cidrQAWireReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("response is not contract-shaped JSON: %v; body: %s", err, w.Body.String())
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil || raw == nil {
		t.Fatalf("response is not a JSON object: %v; body: %s", err, w.Body.String())
	}
	return report, raw
}

// TEST-30; REQ-28.
func TestGetVPCCIDRReport_RejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name, method string
		query        url.Values
		want         int
	}{
		{"missing env", http.MethodGet, url.Values{}, 400},
		{"empty env", http.MethodGet, url.Values{"env": {""}}, 400},
		{"unknown env", http.MethodGet, url.Values{"env": {"unknown"}}, 400},
		{"path is not a listed env", http.MethodGet, url.Values{"env": {"../dev"}}, 400},
		{"non-integer count", http.MethodGet, url.Values{"env": {"dev"}, "count": {"many"}}, 400},
		{"fractional count", http.MethodGet, url.Values{"env": {"dev"}, "count": {"2.5"}}, 400},
		{"POST", http.MethodPost, url.Values{"env": {"dev"}}, 405},
		{"PUT", http.MethodPut, url.Values{"env": {"dev"}}, 405},
		{"PATCH", http.MethodPatch, url.Values{"env": {"dev"}}, 405},
		{"DELETE", http.MethodDelete, url.Values{"env": {"dev"}}, 405},
		{"HEAD", http.MethodHead, url.Values{"env": {"dev"}}, 405},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := cidrQASetupAPI(t, nil, nil)
			before := cidrQASnapshot(t, dir)
			w := cidrQAAPIRequest(tt.method, tt.query)
			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d; body: %s", w.Code, tt.want, w.Body.String())
			}
			// The API specifies JSON error bodies for invalid GET input, not
			// the content type or body of a method-not-allowed response.
			if tt.want == 400 {
				var body struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error == "" {
					t.Errorf("400 body = %q, want a non-empty JSON error: %v", w.Body.String(), err)
				}
			}
			cidrQAAssertUnchanged(t, dir, before)
		})
	}
}

// TEST-31; REQ-28.
func TestGetVPCCIDRReport_ClampsSuggestionCount(t *testing.T) {
	tests := []struct {
		name, count string
		present     bool
		want        int
	}{
		{"default five", "", false, 5},
		{"negative becomes one", "-5", true, 1},
		{"zero becomes one", "0", true, 1},
		{"lower endpoint", "1", true, 1},
		{"middle", "4", true, 4},
		{"upper endpoint", "10", true, 10},
		{"just above upper", "11", true, 10},
		{"large count", "100", true, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cidrQASetupAPI(t, map[string]interface{}{"vpc_cidr": "10.77.0.0/16"}, nil)
			query := url.Values{"env": {"dev"}}
			if tt.present {
				query.Set("count", tt.count)
			}
			report, _ := cidrQAReadAPIReport(t, cidrQAAPIRequest(http.MethodGet, query))
			if len(report.Suggestions) != tt.want {
				t.Errorf("count %q produced %d suggestions, want %d", tt.count, len(report.Suggestions), tt.want)
			}
			cidrQAAssertSuggestions(t, report.Suggestions, nil)
		})
	}
	t.Run("short pool returns every remaining choice once", func(t *testing.T) {
		uses := cidrQAPoolUsesExcept(42, 77)
		cidrQASetupAPI(t, nil, uses)
		report, _ := cidrQAReadAPIReport(t, cidrQAAPIRequest(http.MethodGet, url.Values{"env": {"dev"}, "count": {"5"}}))
		if len(report.Suggestions) != 2 {
			t.Errorf("short pool count = %d, want 2", len(report.Suggestions))
		}
		cidrQAAssertSuggestions(t, report.Suggestions, uses)
	})
}

// TEST-32; REQ-18/M4, REQ-28, REQ-32.
func TestGetVPCCIDRReport_EmitsContractJSON(t *testing.T) {
	tests := []struct {
		name                        string
		fields                      map[string]interface{}
		configured, effective, code string
		override, defaultVPC        bool
	}{
		{"unset remains legacy effective value", nil, "", "10.0.0.0/16", "", false, false},
		{"explicit free", map[string]interface{}{"vpc_cidr": "10.77.0.0/16"}, "10.77.0.0/16", "10.77.0.0/16", "", false, false},
		{"configured issue is serialized", map[string]interface{}{"vpc_cidr": "10.77.0.0/20"}, "10.77.0.0/20", "10.77.0.0/20", "size", false, false},
		{"override is a boolean and suppresses configured issue", map[string]interface{}{"vpc_cidr": "172.20.0.0/16", "allow_vpc_cidr_override": true}, "172.20.0.0/16", "172.20.0.0/16", "", true, false},
		{"default VPC effective value is empty", map[string]interface{}{"use_default_vpc": true, "vpc_cidr": "10.42.0.0/16"}, "10.42.0.0/16", "", "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cidrQASetupAPI(t, tt.fields, nil)
			report, raw := cidrQAReadAPIReport(t, cidrQAAPIRequest(http.MethodGet, url.Values{"env": {"dev"}}))
			for _, key := range []string{"env", "useDefaultVpc", "override", "configured", "effective", "warnings", "suggestions", "awsChecked", "deployed", "deployedReason"} {
				if value, exists := raw[key]; !exists || string(value) == "null" {
					t.Errorf("required JSON key %q is missing or null: %v", key, raw)
				}
			}
			for _, key := range []string{"useDefaultVpc", "override", "awsChecked"} {
				if s := string(raw[key]); s != "true" && s != "false" {
					t.Errorf("JSON %s = %s, want boolean", key, s)
				}
			}
			for _, key := range []string{"warnings", "suggestions"} {
				if s := strings.TrimSpace(string(raw[key])); !strings.HasPrefix(s, "[") {
					t.Errorf("JSON %s = %s, want an array (including an empty array)", key, s)
				}
			}
			if report.Env != "dev" || report.Configured != tt.configured || report.Effective != tt.effective || report.Override != tt.override || report.UseDefaultVPC != tt.defaultVPC {
				t.Errorf("wire report has wrong identity/ranges/flags: %+v", report)
			}
			if report.AWSChecked || report.Deployed != "no" || report.DeployedReason == "" {
				t.Errorf("offline no-target deployment wire state = %+v", report)
			}
			if tt.code == "" {
				if report.Error != nil {
					t.Errorf("unexpected configured error: %+v", report.Error)
				}
			} else if report.Error == nil || report.Error.Code != tt.code || report.Error.Message == "" {
				t.Errorf("wire issue = %+v, want code %q and message", report.Error, tt.code)
			} else {
				cidrQAAssertPlainHint(t, report.Error.Hint)
			}
			if _, exists := raw["candidate"]; exists {
				t.Errorf("candidate key was returned without a candidate query: %s", raw["candidate"])
			}
			for key := range raw {
				if strings.EqualFold(key, "AWSError") || strings.Contains(key, "_") {
					t.Errorf("internal or snake-case report field leaked: %q", key)
				}
			}
		})
	}
}

// TEST-33; REQ-6/R1.6, REQ-20/M6, REQ-28.
func TestGetVPCCIDRReport_HandlesAWSAndProtectsNetworkIdentity(t *testing.T) {
	for _, flag := range []string{"1", "true"} {
		t.Run("successful AWS flag "+flag, func(t *testing.T) {
			fields := map[string]interface{}{"account_id": "000000000000", "aws_profile": "fixture-profile", "vpc_cidr": "10.77.0.0/16"}
			cidrQASetupAPI(t, fields, nil)
			vpcCIDRAPIDeps = func() vpcCIDRDeps {
				deps := cidrQADeps(nil)
				deps.aws = cidrQAFakeAWS([]awsVPC{{
					ID:    "vpc-synthetic-private-id",
					Tags:  map[string]string{"Name": "synthetic-private-vpc-name"},
					CIDRs: []netip.Prefix{netip.MustParsePrefix("10.40.0.0/16"), netip.MustParsePrefix("10.77.0.0/16")},
				}}, nil)
				return deps
			}
			w := cidrQAAPIRequest(http.MethodGet, url.Values{"env": {"dev"}, "aws": {flag}, "count": {"10"}})
			report, raw := cidrQAReadAPIReport(t, w)
			if !report.AWSChecked || len(report.Warnings) != 1 || report.Warnings[0].Source != "another network in this AWS account" {
				t.Errorf("wire AWS result did not include secondary association warning: %+v", report)
			}
			used := []vpcCIDRUse{{Prefix: netip.MustParsePrefix("10.40.0.0/16")}, {Prefix: netip.MustParsePrefix("10.77.0.0/16")}}
			cidrQAAssertSuggestions(t, report.Suggestions, used)
			for _, private := range []string{"vpc-synthetic-private-id", "synthetic-private-vpc-name"} {
				if strings.Contains(w.Body.String(), private) {
					t.Errorf("API JSON leaked synthetic AWS identity %q: %s", private, w.Body.String())
				}
			}
			for key := range raw {
				if strings.EqualFold(key, "AWSError") {
					t.Errorf("internal AWSError exposed: %s", w.Body.String())
				}
			}
		})
	}
	for _, stage := range []string{"factory", "listing"} {
		t.Run("AWS "+stage+" failure is 200", func(t *testing.T) {
			cidrQASetupAPI(t, map[string]interface{}{"account_id": "000000000000", "aws_profile": "fixture-profile"}, nil)
			const diagnostic = "synthetic-private-diagnostic vpc-synthetic-error-id"
			vpcCIDRAPIDeps = func() vpcCIDRDeps {
				deps := cidrQADeps(nil)
				if stage == "factory" {
					deps.aws = func(context.Context, Env) (vpcCIDRAWS, error) { return vpcCIDRAWS{}, errors.New(diagnostic) }
				} else {
					deps.aws = cidrQAFakeAWS(nil, errors.New(diagnostic))
				}
				return deps
			}
			w := cidrQAAPIRequest(http.MethodGet, url.Values{"env": {"dev"}, "aws": {"1"}})
			report, _ := cidrQAReadAPIReport(t, w)
			if report.AWSChecked || report.AWSNote == "" || report.Deployed != "unknown" || report.Error != nil || len(report.Suggestions) != 5 {
				t.Errorf("AWS failure did not return a usable offline report: %+v", report)
			}
			if strings.Contains(w.Body.String(), diagnostic) || strings.Contains(w.Body.String(), "vpc-synthetic-error-id") {
				t.Errorf("raw AWS diagnostic escaped the hidden AWSError field: %s", w.Body.String())
			}
		})
	}
}

// TEST-34; REQ-13/H2, REQ-18/M4, REQ-23/L2, REQ-26/L5, REQ-28.
func TestGetVPCCIDRReport_ValidatesCandidateLocally(t *testing.T) {
	tests := []struct {
		value             string
		valid, incomplete bool
		code, fix         string
		warnings          int
	}{
		{"", false, true, "", "", 0},
		{"10.77", false, true, "", "", 0},
		{"10.77.0.0/16", true, false, "", "", 0},
		{"10.42.0.0/16", true, false, "", "", 1},
		{"10.77.0.0/20", false, false, "size", "", 0},
		{"10.5.33.0/19", false, false, "not_network_address", "10.5.32.0/19", 0},
	}
	for i, tt := range tests {
		t.Run(strconv.Itoa(i)+"-"+tt.value, func(t *testing.T) {
			siblings := []vpcCIDRUse{{Source: `env "staging"`, Name: "staging", File: "staging.yaml", Prefix: netip.MustParsePrefix("10.42.0.0/16")}}
			dir := cidrQASetupAPI(t, map[string]interface{}{
				"account_id": "000000000000", "aws_profile": "fixture-profile",
				"vpc_cidr": "10.77.0.0/20", "allow_vpc_cidr_override": true,
			}, siblings)
			before := cidrQASnapshot(t, dir)
			vpcCIDRAPIDeps = func() vpcCIDRDeps {
				deps := cidrQADeps(siblings)
				deps.aws = cidrQAFakeAWS([]awsVPC{{ID: "vpc-synthetic-candidate", CIDRs: []netip.Prefix{netip.MustParsePrefix("10.77.0.0/16")}}}, nil)
				return deps
			}
			report, raw := cidrQAReadAPIReport(t, cidrQAAPIRequest(http.MethodGet,
				url.Values{"env": {"dev"}, "aws": {"1"}, "candidate": {tt.value}}))
			if !report.Override || report.Error != nil || report.AWSChecked || report.Deployed != "unchecked" {
				t.Errorf("candidate request did not retain offline/override semantics: %+v", report)
			}
			c := report.Candidate
			if c == nil || len(raw["candidate"]) == 0 {
				t.Fatal("candidate query presence, including empty string, was lost")
			}
			if c.Value != tt.value || c.Valid != tt.valid || c.Incomplete != tt.incomplete {
				t.Errorf("wire candidate = %+v, want typed value %q valid %v incomplete %v", c, tt.value, tt.valid, tt.incomplete)
			}
			if c.Warnings == nil || len(c.Warnings) != tt.warnings {
				t.Errorf("candidate warnings = %#v, want only %d sibling warnings", c.Warnings, tt.warnings)
			}
			if tt.code == "" {
				if c.Error != nil {
					t.Errorf("incomplete/valid candidate got error: %+v", c.Error)
				}
			} else if c.Error == nil || c.Error.Code != tt.code || c.Error.Fix != tt.fix {
				t.Errorf("candidate issue = %+v, want %s fix %s despite configured override", c.Error, tt.code, tt.fix)
			} else {
				cidrQAAssertPlainHint(t, c.Error.Hint)
			}
			cidrQAAssertUnchanged(t, dir, before)
		})
	}
}
