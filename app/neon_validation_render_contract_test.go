package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

// REQ-7 / TEST-6: expected rule sets come from the public shared vectors, not current output.
func TestNeonContractValidationVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "neon_validation_vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Vectors []struct {
			Name   string
			Config map[string]interface{}
			Want   []string
		}
	}
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Vectors) == 0 {
		t.Fatal("validation vectors must not be empty")
	}
	for _, v := range doc.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			err := validateNeonConfigMap(v.Config)
			got := append([]string{}, neonRules(err)...)
			want := append([]string{}, v.Want...)
			sort.Strings(got)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("rules = %v, want %v; error=%v", got, want, err)
			}
			if len(want) > 0 && err == nil {
				t.Fatal("invalid config was accepted")
			}
			if len(want) == 0 && err != nil {
				t.Fatalf("valid config refused: %v", err)
			}
		})
	}
}

// REQ-13 / TEST-13.
func TestNeonContractRoleNameVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "neon_role_name_vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Vectors []struct{ Name, Project, Env, Service, Database, Want string }
	}
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Vectors) == 0 {
		t.Fatal("role vectors must not be empty")
	}
	valid := regexp.MustCompile("^[a-z_][a-z0-9_]*$")
	for _, v := range doc.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			got := neonRoleName(v.Project, v.Env, v.Service, v.Database)
			if got != v.Want {
				t.Fatalf("role=%q want %q", got, v.Want)
			}
			if len(got) > 63 || !valid.MatchString(got) {
				t.Fatalf("not a valid Postgres role: %q", got)
			}
		})
	}
}

func qaNeonOverlay(t *testing.T, name string) map[string]interface{} {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	// yaml.v2 decodes nested interface maps; JSON roundtrip is deliberately avoided.
	var decoded map[string]interface{}
	if err = yaml.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	var convert func(interface{}) interface{}
	convert = func(v interface{}) interface{} {
		switch x := v.(type) {
		case map[interface{}]interface{}:
			m := map[string]interface{}{}
			for k, v := range x {
				m[k.(string)] = convert(v)
			}
			return m
		case map[string]interface{}:
			m := map[string]interface{}{}
			for k, v := range x {
				m[k] = convert(v)
			}
			return m
		case []interface{}:
			r := make([]interface{}, len(x))
			for i, v := range x {
				r[i] = convert(v)
			}
			return r
		default:
			return v
		}
	}
	m := convert(decoded).(map[string]interface{})
	m["project"] = "myapp"
	m["env"] = "dev"
	return m
}

// REQ-11, REQ-17 / TEST-11: inspect the documented JSON-shaped render contract.
func TestNeonContractRenderRetainsDisabledServicesAndCreatedUnusedDatabases(t *testing.T) {
	for _, tc := range []struct {
		file          string
		dbs, services []string
	}{
		{"neon_overlay_create.yaml", []string{"app", "archive", "neondb"}, []string{"backend", "service1"}},
		{"neon_overlay_reuse.yaml", []string{"analytics", "app"}, []string{"backend", "reports", "service1"}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			m := qaNeonOverlay(t, tc.file)
			first, err := buildNeonRender(m)
			if err != nil {
				t.Fatal(err)
			}
			second, err := buildNeonRender(m)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatal("render input is not deterministic")
			}
			var render struct {
				Mode        string
				Owner       string   `json:"owner_role"`
				PG          int      `json:"pg_version"`
				Services    []string `json:"ssm_services"`
				Connections []struct {
					Database, Alias string
					Revoke          bool `json:"revoke_public"`
					Services        map[string]struct {
						Schemas []string
						Access  string
						Role    string `json:"role_name"`
						Version int    `json:"password_version"`
					}
				}
			}
			if err = json.Unmarshal([]byte(qaNeonJSON(t, first)), &render); err != nil {
				t.Fatal(err)
			}
			if render.Owner != "neondb_owner" || render.PG != 17 {
				t.Fatalf("defaults=%s", qaNeonJSON(t, first))
			}
			if !reflect.DeepEqual(render.Services, tc.services) {
				t.Fatalf("ssm services=%v want %v", render.Services, tc.services)
			}
			var dbs []string
			for _, c := range render.Connections {
				dbs = append(dbs, c.Database)
				if !regexp.MustCompile("^neon_[a-z0-9_]+_[a-f0-9]{6}$").MatchString(c.Alias) {
					t.Errorf("invalid connection alias %q", c.Alias)
				}
				wantRevoke := c.Database == "app" || c.Database == "archive"
				if c.Revoke != wantRevoke {
					t.Errorf("%s revoke_public=%v want %v", c.Database, c.Revoke, wantRevoke)
				}
				for svc, a := range c.Services {
					if a.Role != neonRoleName("myapp", "dev", svc, c.Database) || a.Version != 1 {
						t.Errorf("wrong role identity/default version: %+v", a)
					}
					if len(a.Schemas) == 0 {
						t.Errorf("%s missing schema list", svc)
					}
					if svc == "reports" && !reflect.DeepEqual(a.Schemas, []string{"reporting"}) {
						t.Errorf("disabled reports schemas=%v", a.Schemas)
					}
					if svc == "backend" && tc.file == "neon_overlay_create.yaml" && !reflect.DeepEqual(a.Schemas, []string{"public"}) {
						t.Errorf("default schemas=%v", a.Schemas)
					}
				}
				if c.Database == "archive" && len(c.Services) != 0 {
					t.Error("unused created database gained a service")
				}
			}
			if !reflect.DeepEqual(dbs, tc.dbs) {
				t.Errorf("connections=%v want %v", dbs, tc.dbs)
			}
			if strings.Contains(qaNeonJSON(t, first), "search_path") {
				t.Error("render input must use schemas, not search_path")
			}
			if err = applyNeonPreprocess(m); err != nil {
				t.Fatal(err)
			}
			if _, ok := m[neonRenderKey]; !ok {
				t.Fatal("neon_render not injected")
			}
			if _, ok := m["workload"].(map[string]interface{})["backend_neon"]; ok {
				t.Error("backend access leaked into workload template input")
			}
			for _, raw := range m["services"].([]interface{}) {
				if _, ok := raw.(map[string]interface{})["neon"]; ok {
					t.Error("service access not stripped")
				}
			}
		})
	}
}

// REQ-17 / TEST-11. This proves preprocessing identity, not unavailable historical HCL bytes.
func TestNeonContractAWSOnlyPreprocessIsUnchanged(t *testing.T) {
	m := qaNeonMap(t, `{"project":"myapp","env":"dev","workload":{"backend_cpu":256},"services":[{"name":"api","enabled":false}]}`)
	before := qaNeonJSON(t, m)
	r, err := buildNeonRender(m)
	if err != nil || r != nil {
		t.Fatalf("AWS-only render=%v error=%v", r, err)
	}
	if err = applyNeonPreprocess(m); err != nil {
		t.Fatal(err)
	}
	if qaNeonJSON(t, m) != before {
		t.Fatal("AWS-only map changed")
	}
}

// REQ-7 / TEST-6: validation precedes disabled-service filtering.
func TestNeonContractDisabledServiceStillRequiresDeclaredNeon(t *testing.T) {
	m := qaNeonMap(t, `{"project":"myapp","env":"dev","services":[{"name":"reports","enabled":false,"neon":{"database":"app","access":"readonly"}}]}`)
	if got := neonRules(applyNeonPreprocess(m)); !reflect.DeepEqual(got, []string{"V-N13"}) {
		t.Fatalf("rules=%v want V-N13", got)
	}
}
