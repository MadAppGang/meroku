package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v2"
)

// Existing structs may add legacy default keys. Every input key and leaf must
// nevertheless survive; equal-length arrays also protect order and duplicates.
func qaCapabilityPreserveYAML(t *testing.T, want, got interface{}, path string) {
	t.Helper()
	switch expected := want.(type) {
	case map[interface{}]interface{}:
		actual, ok := got.(map[interface{}]interface{})
		if !ok {
			t.Errorf("%s: expected mapping, got %#v", path, got)
			return
		}
		for key, value := range expected {
			actualValue, exists := actual[key]
			if !exists {
				t.Errorf("%s.%v: input field was lost", path, key)
				continue
			}
			qaCapabilityPreserveYAML(t, value, actualValue, fmt.Sprintf("%s.%v", path, key))
		}
	case []interface{}:
		actual, ok := got.([]interface{})
		if !ok || len(actual) != len(expected) {
			t.Errorf("%s: got %#v, want %#v", path, got, want)
			return
		}
		for i := range expected {
			qaCapabilityPreserveYAML(t, expected[i], actual[i], fmt.Sprintf("%s[%d]", path, i))
		}
	default:
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %#v, want %#v", path, got, want)
		}
	}
}

func qaCapabilitySaveYAML(t *testing.T, raw []byte) interface{} {
	t.Helper()
	var env Env
	qaCapabilityNoError(t, yaml.Unmarshal(raw, &env))
	filename := filepath.Join(t.TempDir(), "dev.yaml")
	qaCapabilityNoError(t, saveEnvToFile(env, filename))
	saved, err := os.ReadFile(filename)
	qaCapabilityNoError(t, err)
	var result interface{}
	qaCapabilityNoError(t, yaml.Unmarshal(saved, &result))
	return result
}

func TestQACapabilityRoundTripPreservesFeatureFields(t *testing.T) {
	for _, filename := range []string{"env_roundtrip.yaml", "qa_capabilities_roundtrip.yaml"} {
		t.Run(filename, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", filename))
			qaCapabilityNoError(t, err)
			var input interface{}
			qaCapabilityNoError(t, yaml.Unmarshal(raw, &input))
			qaCapabilityPreserveYAML(t, input, qaCapabilitySaveYAML(t, raw), "env")
		})
	}
}

func TestQACapabilityRoundTripKeepsAbsentFeaturesAbsent(t *testing.T) {
	raw := []byte("project: myapp\nenv: dev\nregion: us-east-1\naccount_id: \"000000000000\"\nworkload: {}\nservices:\n  - name: api\nscheduled_tasks:\n  - name: nightly\nevent_processor_tasks:\n  - name: mailer\n")
	got := qaCapabilitySaveYAML(t, raw).(map[interface{}]interface{})
	for _, key := range []string{"data_bucket", "vaults", "event_bus", "valkey", "amplify_apps"} {
		if _, exists := got[key]; exists {
			t.Errorf("save invented optional root %q", key)
		}
	}
	backend := got["workload"].(map[interface{}]interface{})
	for _, key := range []string{"policy", "vaults", "events"} {
		if _, exists := backend[key]; exists {
			t.Errorf("save invented workload.%s", key)
		}
	}
	for _, kind := range []string{"services", "scheduled_tasks", "event_processor_tasks"} {
		owner := got[kind].([]interface{})[0].(map[interface{}]interface{})
		for _, key := range []string{"policy", "vaults", "events"} {
			if _, exists := owner[key]; exists {
				t.Errorf("save invented %s[0].%s", kind, key)
			}
		}
	}
}
