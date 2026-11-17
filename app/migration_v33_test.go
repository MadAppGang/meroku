package main

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v2"
)

// Schema v33 moves workload.backend_policies, which no template line read, onto
// workload.policy, the one key the backend's statements render from.

func v33Statement(action, resource string) map[interface{}]interface{} {
	return map[interface{}]interface{}{
		"actions":   []interface{}{action},
		"resources": []interface{}{resource},
	}
}

// v33Workload builds either workload map type a migration receives: yaml.v2
// decodes map[interface{}]interface{}, and a config built in memory or passed
// through convertToJSONCompatible is map[string]interface{}.
type v33Workload struct {
	name string
	make func(map[string]interface{}) interface{}
}

var v33WorkloadKinds = []v33Workload{
	{
		name: "yaml mapping",
		make: func(fields map[string]interface{}) interface{} {
			w := map[interface{}]interface{}{}
			for k, v := range fields {
				w[k] = v
			}
			return w
		},
	},
	{
		name: "string-keyed map",
		make: func(fields map[string]interface{}) interface{} {
			w := map[string]interface{}{}
			for k, v := range fields {
				w[k] = v
			}
			return w
		},
	},
}

func v33Get(t *testing.T, doc map[string]interface{}, key string) (interface{}, bool) {
	t.Helper()
	workload, ok := asYAMLMapping(doc["workload"])
	if !ok {
		t.Fatalf("workload is not a mapping after migration: %#v", doc["workload"])
	}
	return workload.lookup(key)
}

func TestMigrateToV33MovesBackendPoliciesOntoPolicy(t *testing.T) {
	existing := map[interface{}]interface{}{
		"effect":    "Deny",
		"actions":   []interface{}{"s3:DeleteObject"},
		"resources": []interface{}{"*"},
	}
	movedA := v33Statement("dynamodb:GetItem", "arn:aws:dynamodb:us-east-1:000000000000:table/example")
	movedB := v33Statement("sqs:SendMessage", "arn:aws:sqs:us-east-1:000000000000:example-queue")

	tests := []struct {
		name       string
		fields     map[string]interface{}
		wantPolicy []interface{}
	}{
		{
			name:       "backend_policies alone becomes policy",
			fields:     map[string]interface{}{"backend_policies": []interface{}{movedA, movedB}},
			wantPolicy: []interface{}{movedA, movedB},
		},
		{
			// policy is what was deployed; its statements keep their place
			// and the moved ones follow.
			name: "existing policy first, moved statements after",
			fields: map[string]interface{}{
				"policy":           []interface{}{existing},
				"backend_policies": []interface{}{movedA},
			},
			wantPolicy: []interface{}{existing, movedA},
		},
		{
			name: "an empty policy list takes the moved statements",
			fields: map[string]interface{}{
				"policy":           []interface{}{},
				"backend_policies": []interface{}{movedA},
			},
			wantPolicy: []interface{}{movedA},
		},
	}

	for _, kind := range v33WorkloadKinds {
		for _, test := range tests {
			t.Run(kind.name+"/"+test.name, func(t *testing.T) {
				doc := map[string]interface{}{"workload": kind.make(test.fields)}
				if err := migrateToV33(doc, io.Discard); err != nil {
					t.Fatalf("migrateToV33: %v", err)
				}
				if v, present := v33Get(t, doc, "backend_policies"); present {
					t.Errorf("workload.backend_policies is still present: %#v", v)
				}
				got, _ := v33Get(t, doc, "policy")
				if !reflect.DeepEqual(got, test.wantPolicy) {
					t.Errorf("policy = %#v\nwant %#v", got, test.wantPolicy)
				}
			})
		}
	}
}

// An empty or valueless backend_policies grants nothing; the key goes and
// policy is not created or touched.
func TestMigrateToV33DropsAnEmptyBackendPolicies(t *testing.T) {
	for _, kind := range v33WorkloadKinds {
		for name, value := range map[string]interface{}{"empty list": []interface{}{}, "no value": nil} {
			t.Run(kind.name+"/"+name, func(t *testing.T) {
				doc := map[string]interface{}{"workload": kind.make(map[string]interface{}{"backend_policies": value})}
				if err := migrateToV33(doc, io.Discard); err != nil {
					t.Fatalf("migrateToV33: %v", err)
				}
				if v, present := v33Get(t, doc, "backend_policies"); present {
					t.Errorf("an empty backend_policies must be removed, got %#v", v)
				}
				if v, present := v33Get(t, doc, "policy"); present {
					t.Errorf("an empty backend_policies must not create policy, got %#v", v)
				}
			})
		}
	}
}

// Values that cannot be merged safely stay where they are, so generate can
// refuse them by name instead of the migration discarding them.
func TestMigrateToV33LeavesUnmergeableValuesInPlace(t *testing.T) {
	moved := v33Statement("dynamodb:GetItem", "arn:aws:dynamodb:us-east-1:000000000000:table/example")

	tests := map[string]map[string]interface{}{
		"backend_policies is not a list": {"backend_policies": "dynamodb:GetItem"},
		"policy is not a list": {
			"policy":           "s3:GetObject",
			"backend_policies": []interface{}{moved},
		},
	}

	for _, kind := range v33WorkloadKinds {
		for name, fields := range tests {
			t.Run(kind.name+"/"+name, func(t *testing.T) {
				doc := map[string]interface{}{"workload": kind.make(fields)}
				want := map[string]interface{}{"workload": kind.make(fields)}
				if err := migrateToV33(doc, io.Discard); err != nil {
					t.Fatalf("migrateToV33: %v", err)
				}
				if !reflect.DeepEqual(doc, want) {
					t.Errorf("the workload changed:\nbefore %#v\nafter  %#v", want, doc)
				}
			})
		}
	}
}

// A second run changes nothing, and a config without workload, or without
// backend_policies, is left exactly as it was.
func TestMigrateToV33IsIdempotent(t *testing.T) {
	existing := v33Statement("s3:GetObject", "arn:aws:s3:::example-bucket/*")
	moved := v33Statement("dynamodb:GetItem", "arn:aws:dynamodb:us-east-1:000000000000:table/example")

	for _, kind := range v33WorkloadKinds {
		t.Run(kind.name+"/second run changes nothing", func(t *testing.T) {
			doc := map[string]interface{}{"workload": kind.make(map[string]interface{}{
				"policy":           []interface{}{existing},
				"backend_policies": []interface{}{moved},
			})}
			if err := migrateToV33(doc, io.Discard); err != nil {
				t.Fatalf("first run: %v", err)
			}
			first, _ := v33Get(t, doc, "policy")
			if err := migrateToV33(doc, io.Discard); err != nil {
				t.Fatalf("second run: %v", err)
			}
			second, _ := v33Get(t, doc, "policy")
			if !reflect.DeepEqual(first, second) {
				t.Errorf("second run changed policy:\nfirst  %#v\nsecond %#v", first, second)
			}
		})

		t.Run(kind.name+"/no backend_policies is a no-op", func(t *testing.T) {
			fields := map[string]interface{}{"policy": []interface{}{existing}, "policies": []interface{}{}}
			doc := map[string]interface{}{"workload": kind.make(fields)}
			before := map[string]interface{}{"workload": kind.make(fields)}
			if err := migrateToV33(doc, io.Discard); err != nil {
				t.Fatalf("migrateToV33: %v", err)
			}
			if !reflect.DeepEqual(doc, before) {
				t.Errorf("a config without backend_policies changed:\nbefore %#v\nafter  %#v", before, doc)
			}
		})
	}

	t.Run("no workload section", func(t *testing.T) {
		doc := map[string]interface{}{"project": "acme"}
		if err := migrateToV33(doc, io.Discard); err != nil {
			t.Fatalf("migrateToV33: %v", err)
		}
		if !reflect.DeepEqual(doc, map[string]interface{}{"project": "acme"}) {
			t.Errorf("a config without workload changed: %#v", doc)
		}
	})
}

// The template loader migrates before rendering, so a v32 file with
// backend_policies reaches the template with those statements under policy.
func TestLoadEnvToMapMovesBackendPoliciesOntoPolicy(t *testing.T) {
	dir := t.TempDir()
	cfg := map[string]interface{}{
		"schema_version": 32,
		"env":            "dev",
		"project":        "regression",
		"region":         "us-east-1",
		"account_id":     "000000000000",
		"aws_profile":    "none",
		"workload": map[string]interface{}{
			"backend_image_port": 8080,
			"backend_policies": []interface{}{
				map[string]interface{}{
					"actions":   []interface{}{"s3:GetObject"},
					"resources": []interface{}{"arn:aws:s3:::example-bucket/*"},
				},
			},
		},
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dev.yaml"), data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	chdir(t, dir)

	m, err := loadEnvToMap("dev.yaml")
	if err != nil {
		t.Fatalf("loadEnvToMap: %v", err)
	}
	workload, ok := m["workload"].(map[string]interface{})
	if !ok {
		t.Fatalf("workload is not a mapping: %#v", m["workload"])
	}
	if v, present := workload["backend_policies"]; present {
		t.Errorf("workload.backend_policies reached the template: %#v", v)
	}
	want := []interface{}{
		map[string]interface{}{
			"actions":   []interface{}{"s3:GetObject"},
			"resources": []interface{}{"arn:aws:s3:::example-bucket/*"},
		},
	}
	if !reflect.DeepEqual(workload["policy"], want) {
		t.Errorf("workload.policy = %#v\nwant %#v", workload["policy"], want)
	}
}
