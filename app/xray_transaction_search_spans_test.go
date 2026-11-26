package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type spanFields = map[string]interface{}

type spanSearchVector struct {
	Name              string        `json:"name"`
	BackendEnabled    bool          `json:"backend_xray_enabled"`
	BackendSpans      bool          `json:"backend_xray_spans"`
	Services          []interface{} `json:"services"`
	TransactionSearch *bool         `json:"transaction_search"`
	Want              bool          `json:"want"`
	Refused           bool          `json:"refused"`
	GoOnly            bool          `json:"go_only"`
}

func spanSearchVectors(t *testing.T) []spanSearchVector {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "xray_transaction_search_vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Vectors []spanSearchVector `json:"vectors"`
	}
	if err := json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Vectors) == 0 {
		t.Fatal("no Transaction Search vectors")
	}
	return fixture.Vectors
}

// REQ-1, REQ-7
func TestEnvUsesTransactionSearchVectors(t *testing.T) {
	for _, v := range spanSearchVectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			env := spanFields{
				"workload": spanFields{"xray_enabled": v.BackendEnabled, "xray_spans": v.BackendSpans},
				"services": v.Services,
			}
			if v.TransactionSearch != nil {
				env["xray"] = spanFields{"transaction_search": *v.TransactionSearch}
			}
			if got := envUsesTransactionSearch(env); got != v.Want {
				t.Errorf("envUsesTransactionSearch = %v, want %v", got, v.Want)
			}
			if err := validateXRayMap(env); (err != nil) != v.Refused {
				t.Errorf("validation error = %v, want refused = %v", err, v.Refused)
			}
		})
	}
}

// REQ-8: this checks names only; each module run has its own inputs and oracle.
func TestXRayTransactionSearchVectorsMirroredInTFTest(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "modules", "workloads", "tests", "xray_transaction_search.tftest.hcl"))
	if err != nil {
		t.Fatal(err)
	}
	runs := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^[ \t]*run[ \t]+"([^"]+)"[ \t]*\{`).FindAllStringSubmatch(string(b), -1) {
		runs[m[1]] = true
	}
	for _, v := range spanSearchVectors(t) {
		if !v.GoOnly && !runs[v.Name] {
			t.Errorf("missing Terraform run %q", v.Name)
		}
	}
}

// REQ-1, REQ-7
func TestValidateXRaySpansAndTransactionSearch(t *testing.T) {
	service := func(fields spanFields) spanFields {
		fields["name"] = "orders"
		return spanFields{"services": []interface{}{spanFields{"name": "plain"}, fields}}
	}
	optOut := func(env spanFields) spanFields {
		env["xray"] = spanFields{"transaction_search": false}
		return env
	}
	for _, tc := range []struct {
		name          string
		env           spanFields
		wantFragments []string // nil means valid
	}{
		{"backend alone accepts true", spanFields{"workload": spanFields{"xray_spans": true}}, nil},
		{"explicit false switches are valid", optOut(service(spanFields{"xray_spans": false})), nil},
		{"disabled sidecar allows opt-out", optOut(service(spanFields{"enabled": false, "xray_enabled": true})), nil},
		{"backend sidecar refuses opt-out", optOut(spanFields{"workload": spanFields{"xray_enabled": true}}), []string{"workload.xray_enabled"}},
		{"named service sidecar refuses opt-out", optOut(service(spanFields{"xray_enabled": true})), []string{`services["orders"].xray_enabled`}},
		{"quoted transaction switch", spanFields{"xray": spanFields{"transaction_search": "true"}}, []string{"transaction_search"}},
		{"numeric transaction switch", spanFields{"xray": spanFields{"transaction_search": 0}}, []string{"transaction_search"}},
		{"quoted backend switch without xray block", spanFields{"workload": spanFields{"xray_spans": "true"}}, []string{"workload.xray_spans"}},
		{"quoted service switch without xray block", service(spanFields{"xray_spans": "true"}), []string{"orders", "xray_spans"}},
		{"numeric service switch without xray block", service(spanFields{"xray_spans": 1}), []string{"orders", "xray_spans"}},
		{"disabled service still validates its switch", service(spanFields{"enabled": false, "xray_spans": "false"}), []string{"orders", "xray_spans"}},
		{"unknown key names both alternatives", spanFields{"xray": spanFields{"transaction_serch": true}}, []string{"transaction_serch", "transaction_search", "indexing_percentage"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateXRayMap(tc.env)
			if tc.wantFragments == nil {
				if err != nil {
					t.Fatalf("valid configuration refused: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid configuration accepted")
			}
			for _, s := range tc.wantFragments {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("validation error %q does not name %q", err, s)
				}
			}
		})
	}
}

// REQ-1: the backend alone can opt into Transaction Search without a sidecar.
func TestTransactionSearchBackendSpansAlone(t *testing.T) {
	env := spanFields{"workload": spanFields{"xray_spans": true}}
	if !envUsesTransactionSearch(env) {
		t.Fatal("backend xray_spans alone must enable Transaction Search")
	}
}

// REQ-3, REQ-5
func TestXRayUnusedIndexingInEnvWarnings(t *testing.T) {
	for _, tc := range []struct {
		name           string
		xray, workload spanFields
		want           bool
	}{
		{"no keys", nil, nil, false},
		{"empty block", spanFields{}, nil, false},
		{"null percentage", spanFields{"indexing_percentage": nil}, nil, false},
		{"unused zero", spanFields{"indexing_percentage": 0}, nil, true},
		{"backend spans consumes percentage", spanFields{"indexing_percentage": 10}, spanFields{"xray_spans": true}, false},
		{"sidecar consumes percentage", spanFields{"indexing_percentage": 10}, spanFields{"xray_enabled": true}, false},
		{"explicit owner consumes percentage", spanFields{"indexing_percentage": 10, "transaction_search": true}, nil, false},
		{"non-owner with spans ignores percentage", spanFields{"indexing_percentage": 10, "transaction_search": false}, spanFields{"xray_spans": true}, true},
		{"non-owner without percentage", spanFields{"transaction_search": false}, spanFields{"xray_spans": true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := spanFields{"env": "dev", "account_id": xrayTSAccount, "region": xrayTSRegion}
			if tc.xray != nil {
				env["xray"] = tc.xray
			}
			if tc.workload != nil {
				env["workload"] = tc.workload
			}
			warnings := envLevelWarnings(env, warningInputs{EnvName: "dev"})
			count := 0
			for _, w := range warnings {
				if w.Code == "xray_indexing_unused" {
					count++
					if w.Item.Kind != "env" {
						t.Errorf("unused indexing warning item = %#v, want env", w.Item)
					}
				}
			}
			if (count == 1) != tc.want || count > 1 {
				t.Errorf("unused warnings = %d, want present = %v: %#v", count, tc.want, warnings)
			}
		})
	}
}

// REQ-4, REQ-5: exercise sibling selection and the self gate together.
func TestTransactionSearchOwnershipInConflictWarnings(t *testing.T) {
	const spansSibling = "services:\n  - name: api\n    xray_spans: true\nxray:\n  indexing_percentage: 5\n"
	const explicitSibling = "xray:\n  transaction_search: true\n  indexing_percentage: 5\n"
	for _, tc := range []struct {
		name     string
		self     spanFields
		sibling  string
		conflict bool
	}{
		{"spans owners disagree", spanFields{"workload": spanFields{"xray_spans": true}}, spansSibling, true},
		{"explicit owner sees explicit sibling", spanFields{"xray": spanFields{"transaction_search": true}}, explicitSibling, true},
		{"opted-out sibling is ignored", spanFields{"workload": spanFields{"xray_spans": true}}, spansSibling + "  transaction_search: false\n", false},
		{"opted-out self ignores owner", spanFields{"xray": spanFields{"transaction_search": false}, "workload": spanFields{"xray_spans": true}}, spansSibling, false},
		{"equal percentages are not a conflict", spanFields{"xray": spanFields{"transaction_search": true, "indexing_percentage": 5}}, spansSibling, false},
		{"plain env has no new warnings", spanFields{}, spansSibling, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := tc.self
			env["project"], env["env"], env["account_id"], env["region"] = "myapp", "dev", xrayTSAccount, xrayTSRegion
			dir := t.TempDir()
			xrayTSWriteEnv(t, dir, "prod", "project: myapp\nenv: prod\naccount_id: \"000000000000\"\nregion: us-east-1\n"+tc.sibling)
			warnings := envLevelWarnings(env, warningInputs{EnvName: "dev", EnvFile: filepath.Join(dir, "dev.yaml"), XRaySiblings: siblingXRayIndexingUses})
			if got := hasConfigWarningCode(warnings, "xray_indexing_conflict"); got != tc.conflict {
				t.Errorf("conflict present = %v, want %v: %#v", got, tc.conflict, warnings)
			}
			for _, w := range warnings {
				if w.Code == "xray_indexing_conflict" && !strings.Contains(w.Detail, "prod") {
					t.Errorf("conflict does not identify prod: %s", w.Detail)
				}
				if strings.HasPrefix(w.Code, "xray_") && w.Code != "xray_indexing_conflict" {
					t.Errorf("unexpected X-Ray warning: %#v", w)
				}
			}
		})
	}
}
