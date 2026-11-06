package main

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

// workload.backend_health_check and workload.backend_circuit_breaker: the
// YAML -> main.tf hop, the refusals before rendering, and the typed config
// round trip. The module half lives in
// modules/workloads/tests/backend_health.tftest.hcl.

const backendHealthTestCommand = "curl -f http://localhost:8080/health/live || exit 1"

// backendHealthWorkload returns project/dev.yaml's workload section with both
// keys removed, then the given keys set.
func backendHealthWorkload(t *testing.T, set map[interface{}]interface{}) map[interface{}]interface{} {
	t.Helper()

	base, err := os.ReadFile(filepath.Join("..", "project", "dev.yaml"))
	if err != nil {
		t.Fatalf("reading project/dev.yaml: %v", err)
	}
	var doc map[interface{}]interface{}
	if err := yaml.Unmarshal(base, &doc); err != nil {
		t.Fatalf("parsing project/dev.yaml: %v", err)
	}
	workload, ok := doc["workload"].(map[interface{}]interface{})
	if !ok {
		t.Fatal("project/dev.yaml has no workload section")
	}
	delete(workload, "backend_health_check")
	delete(workload, "backend_circuit_breaker")
	for k, v := range set {
		workload[k] = v
	}
	return workload
}

// backendHealthPreprocess runs the real pre-processing over project/dev.yaml
// with the given workload keys and returns its error.
func backendHealthPreprocess(t *testing.T, set map[interface{}]interface{}) error {
	t.Helper()

	base, err := os.ReadFile(filepath.Join("..", "project", "dev.yaml"))
	if err != nil {
		t.Fatalf("reading project/dev.yaml: %v", err)
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(base, &doc); err != nil {
		t.Fatalf("parsing project/dev.yaml: %v", err)
	}
	doc["workload"] = backendHealthWorkload(t, set)
	envMap, ok := convertToJSONCompatible(doc).(map[string]interface{})
	if !ok {
		t.Fatal("fixture yaml is not a mapping")
	}
	return preprocessEnvMap(envMap)
}

func backendHealthRender(t *testing.T, set map[interface{}]interface{}) string {
	t.Helper()
	return renderMainHBS(t, map[string]interface{}{"workload": backendHealthWorkload(t, set)})
}

// argumentValue returns the value text of `<name> = ...` in rendered HCL: from
// the `=` to the end of the balanced {...} or [...] that follows, or to the end
// of the line for a scalar. Braces inside quoted strings are ignored.
func argumentValue(t *testing.T, rendered, name string) string {
	t.Helper()

	loc := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s*=\s*`).FindStringIndex(rendered)
	if loc == nil {
		t.Fatalf("no %s argument in the rendered main.tf", name)
	}
	rest := rendered[loc[1]:]
	if rest == "" || (rest[0] != '{' && rest[0] != '[') {
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			return rest[:nl]
		}
		return rest
	}
	depth := 0
	inString := false
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if inString {
			if c == '\\' {
				i++
			} else if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return rest[:i+1]
			}
		}
	}
	t.Fatalf("%s argument is unterminated:\n%s", name, rest)
	return ""
}

// field matches `name = value` (HCL) or `"name": value` (JSON).
func field(name, value string) *regexp.Regexp {
	return regexp.MustCompile(`"?\b` + regexp.QuoteMeta(name) + `\b"?\s*[=:]\s*` + value + `\b`)
}

// R1
func TestBackendHealthAbsentKeysRenderNothing(t *testing.T) {
	out := backendHealthRender(t, nil)

	for _, name := range []string{"backend_health_check", "backend_circuit_breaker"} {
		if strings.Contains(out, name) {
			t.Errorf("a YAML without workload.%s must render no %s in main.tf", name, name)
		}
	}
}

// R2: a string command becomes ["CMD-SHELL", "<string>"].
func TestBackendHealthStringCommandBecomesCmdShell(t *testing.T) {
	out := backendHealthRender(t, map[interface{}]interface{}{
		"backend_health_check": map[interface{}]interface{}{"command": backendHealthTestCommand},
	})
	value := argumentValue(t, out, "backend_health_check")

	want := regexp.MustCompile(`\[\s*"CMD-SHELL"\s*,\s*"` + regexp.QuoteMeta(backendHealthTestCommand) + `"\s*\]`)
	if !want.MatchString(value) {
		t.Errorf("a string command must render as [\"CMD-SHELL\", %q], got:\n%s", backendHealthTestCommand, value)
	}
}

// R2: a list command is passed through unchanged.
func TestBackendHealthListCommandPassesThrough(t *testing.T) {
	cases := []struct {
		name    string
		command []interface{}
		want    *regexp.Regexp
	}{
		{
			name:    "CMD form",
			command: []interface{}{"CMD", "/app/healthcheck"},
			want:    regexp.MustCompile(`\[\s*"CMD"\s*,\s*"/app/healthcheck"\s*\]`),
		},
		{
			name:    "CMD with arguments",
			command: []interface{}{"CMD", "/app/healthcheck", "--live"},
			want:    regexp.MustCompile(`\[\s*"CMD"\s*,\s*"/app/healthcheck"\s*,\s*"--live"\s*\]`),
		},
		{
			name:    "CMD-SHELL form",
			command: []interface{}{"CMD-SHELL", backendHealthTestCommand},
			want:    regexp.MustCompile(`\[\s*"CMD-SHELL"\s*,\s*"` + regexp.QuoteMeta(backendHealthTestCommand) + `"\s*\]`),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := backendHealthRender(t, map[interface{}]interface{}{
				"backend_health_check": map[interface{}]interface{}{"command": tc.command},
			})
			value := argumentValue(t, out, "backend_health_check")
			if !tc.want.MatchString(value) {
				t.Errorf("list command %v must pass through unchanged, got:\n%s", tc.command, value)
			}
			if tc.command[0] == "CMD" && strings.Contains(value, "CMD-SHELL") {
				t.Errorf("a CMD list must not be wrapped in CMD-SHELL, got:\n%s", value)
			}
		})
	}
}

// R2: fields present are rendered with their values, start_period 0 included.
func TestBackendHealthPresentFieldsAreRendered(t *testing.T) {
	out := backendHealthRender(t, map[interface{}]interface{}{
		"backend_health_check": map[interface{}]interface{}{
			"command":      backendHealthTestCommand,
			"interval":     15,
			"timeout":      7,
			"retries":      4,
			"start_period": 0,
		},
	})
	value := argumentValue(t, out, "backend_health_check")

	for name, v := range map[string]string{"interval": "15", "timeout": "7", "retries": "4", "start_period": "0"} {
		if !field(name, v).MatchString(value) {
			t.Errorf("%s = %s must be rendered, got:\n%s", name, v, value)
		}
	}
}

// R2: absent fields are not rendered, so the module defaults apply.
func TestBackendHealthAbsentFieldsAreNotRendered(t *testing.T) {
	out := backendHealthRender(t, map[interface{}]interface{}{
		"backend_health_check": map[interface{}]interface{}{
			"command": backendHealthTestCommand,
			"retries": 2,
		},
	})
	value := argumentValue(t, out, "backend_health_check")

	if !field("retries", "2").MatchString(value) {
		t.Errorf("retries = 2 must be rendered, got:\n%s", value)
	}
	for _, name := range []string{"interval", "timeout", "start_period"} {
		if regexp.MustCompile(`\b` + name + `\b`).MatchString(value) {
			t.Errorf("%s is absent from the YAML and must not be rendered, got:\n%s", name, value)
		}
	}
	if strings.Contains(out, "backend_circuit_breaker") {
		t.Errorf("setting only backend_health_check must not render backend_circuit_breaker")
	}
}

// R2: boundary values are accepted by pre-processing.
func TestBackendHealthAcceptsBoundaryValues(t *testing.T) {
	cases := map[string]map[interface{}]interface{}{
		"lower bounds": {"command": backendHealthTestCommand, "interval": 5, "timeout": 2, "retries": 1, "start_period": 0},
		"upper bounds": {"command": backendHealthTestCommand, "interval": 300, "timeout": 120, "retries": 10, "start_period": 300},
		"command only": {"command": []interface{}{"CMD", "/app/healthcheck"}},
	}
	for name, hc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := backendHealthPreprocess(t, map[interface{}]interface{}{"backend_health_check": hc}); err != nil {
				t.Errorf("valid backend_health_check %v was refused: %v", hc, err)
			}
		})
	}
}

// R2: refusals before rendering.
func TestBackendHealthRefusesInvalidConfig(t *testing.T) {
	cmd := backendHealthTestCommand
	cases := map[string]map[interface{}]interface{}{
		"missing command":              {"interval": 30},
		"empty string command":         {"command": ""},
		"empty list command":           {"command": []interface{}{}},
		"list not starting with CMD":   {"command": []interface{}{"NONE", "/app/healthcheck"}},
		"list starting with lowercase": {"command": []interface{}{"cmd", "/app/healthcheck"}},
		"CMD-SHELL alone":              {"command": []interface{}{"CMD-SHELL"}},
		"CMD-SHELL with two strings":   {"command": []interface{}{"CMD-SHELL", "curl -f http://localhost:8080/", "exit 1"}},
		"CMD alone":                    {"command": []interface{}{"CMD"}},
		"interval below 5":             {"command": cmd, "interval": 4},
		"interval above 300":           {"command": cmd, "interval": 301},
		"timeout below 2":              {"command": cmd, "timeout": 1},
		"timeout above 120":            {"command": cmd, "timeout": 121},
		"retries below 1":              {"command": cmd, "retries": 0},
		"retries above 10":             {"command": cmd, "retries": 11},
		"start_period below 0":         {"command": cmd, "start_period": -1},
		"start_period above 300":       {"command": cmd, "start_period": 301},
	}
	for name, hc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := backendHealthPreprocess(t, map[interface{}]interface{}{"backend_health_check": hc}); err == nil {
				t.Errorf("backend_health_check %v must be refused before rendering, got no error", hc)
			}
		})
	}
}

// R3: the circuit breaker reaches main.tf with the given values.
func TestBackendCircuitBreakerRendersGivenValues(t *testing.T) {
	cases := []struct {
		name             string
		cb               map[interface{}]interface{}
		enable, rollback string
	}{
		{"enable false", map[interface{}]interface{}{"enable": false, "rollback": true}, "false", "true"},
		{"rollback false", map[interface{}]interface{}{"enable": true, "rollback": false}, "true", "false"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := backendHealthRender(t, map[interface{}]interface{}{"backend_circuit_breaker": tc.cb})
			value := argumentValue(t, out, "backend_circuit_breaker")
			if !field("enable", tc.enable).MatchString(value) {
				t.Errorf("enable = %s must be rendered, got:\n%s", tc.enable, value)
			}
			if !field("rollback", tc.rollback).MatchString(value) {
				t.Errorf("rollback = %s must be rendered, got:\n%s", tc.rollback, value)
			}
			if strings.Contains(out, "backend_health_check") {
				t.Errorf("setting only backend_circuit_breaker must not render backend_health_check")
			}
		})
	}
}

// R3: `backend_circuit_breaker: {}` is valid and reaches the module call; it
// must not render false for either field (the defaults are true/true).
func TestBackendCircuitBreakerEmptyMapIsValid(t *testing.T) {
	set := map[interface{}]interface{}{"backend_circuit_breaker": map[interface{}]interface{}{}}
	if err := backendHealthPreprocess(t, set); err != nil {
		t.Fatalf("backend_circuit_breaker: {} must be valid, got: %v", err)
	}
	out := backendHealthRender(t, set)
	value := argumentValue(t, out, "backend_circuit_breaker")
	if strings.Contains(value, "false") {
		t.Errorf("backend_circuit_breaker: {} must get enable/rollback true, got:\n%s", value)
	}
}

// R3: non-boolean enable / rollback is refused before rendering.
func TestBackendCircuitBreakerRefusesNonBoolean(t *testing.T) {
	cases := map[string]map[interface{}]interface{}{
		"enable string":   {"enable": "yes"},
		"enable number":   {"enable": 1},
		"rollback string": {"rollback": "true"},
		"rollback number": {"rollback": 0},
	}
	for name, cb := range cases {
		t.Run(name, func(t *testing.T) {
			if err := backendHealthPreprocess(t, map[interface{}]interface{}{"backend_circuit_breaker": cb}); err == nil {
				t.Errorf("backend_circuit_breaker %v must be refused before rendering, got no error", cb)
			}
		})
	}
}

// toStringKeyed turns yaml.v2's map[interface{}]interface{} into string keys.
func toStringKeyed(v interface{}) interface{} {
	switch m := v.(type) {
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(m))
		for k, val := range m {
			out[k.(string)] = toStringKeyed(val)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(m))
		for i, val := range m {
			out[i] = toStringKeyed(val)
		}
		return out
	}
	return v
}

func roundTripWorkload(t *testing.T, src string) map[string]interface{} {
	t.Helper()
	var w Workload
	if err := yaml.Unmarshal([]byte(src), &w); err != nil {
		t.Fatalf("unmarshalling into Workload: %v", err)
	}
	out, err := yaml.Marshal(w)
	if err != nil {
		t.Fatalf("marshalling Workload: %v", err)
	}
	var m map[interface{}]interface{}
	if err := yaml.Unmarshal(out, &m); err != nil {
		t.Fatalf("re-reading marshalled Workload: %v\n%s", err, out)
	}
	return toStringKeyed(m).(map[string]interface{})
}

// R4
func TestBackendHealthRoundTripKeepsStringCommandAndZeroStartPeriod(t *testing.T) {
	m := roundTripWorkload(t, `
backend_health_check:
  command: "`+backendHealthTestCommand+`"
  interval: 15
  start_period: 0
backend_circuit_breaker:
  enable: false
  rollback: true
`)

	hc, ok := m["backend_health_check"].(map[string]interface{})
	if !ok {
		t.Fatalf("backend_health_check lost in the round trip: %v", m)
	}
	if hc["command"] != backendHealthTestCommand {
		t.Errorf("a string command must stay the same string, got %#v", hc["command"])
	}
	if hc["interval"] != 15 {
		t.Errorf("interval must stay 15, got %#v", hc["interval"])
	}
	if v, present := hc["start_period"]; !present || v != 0 {
		t.Errorf("start_period: 0 must stay 0, got %#v (present=%v)", v, present)
	}

	cb, ok := m["backend_circuit_breaker"].(map[string]interface{})
	if !ok {
		t.Fatalf("backend_circuit_breaker lost in the round trip: %v", m)
	}
	if v, present := cb["enable"]; !present || v != false {
		t.Errorf("enable: false must stay false, got %#v (present=%v)", v, present)
	}
	if cb["rollback"] != true {
		t.Errorf("rollback must stay true, got %#v", cb["rollback"])
	}
}

// R4: the list form survives too.
func TestBackendHealthRoundTripKeepsListCommand(t *testing.T) {
	m := roundTripWorkload(t, `
backend_health_check:
  command: ["CMD", "/app/healthcheck"]
`)
	hc, ok := m["backend_health_check"].(map[string]interface{})
	if !ok {
		t.Fatalf("backend_health_check lost in the round trip: %v", m)
	}
	want := []interface{}{"CMD", "/app/healthcheck"}
	if !reflect.DeepEqual(hc["command"], want) {
		t.Errorf("a list command must stay %v, got %#v", want, hc["command"])
	}
}

// R4: absent keys stay absent.
func TestBackendHealthRoundTripOmitsAbsentKeys(t *testing.T) {
	m := roundTripWorkload(t, "backend_auto_deploy: true\n")
	for _, k := range []string{"backend_health_check", "backend_circuit_breaker"} {
		if _, present := m[k]; present {
			t.Errorf("a YAML without %s must marshal without it, got %#v", k, m[k])
		}
	}
}
