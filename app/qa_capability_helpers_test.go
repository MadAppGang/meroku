package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

type qaCapabilityMap = map[string]interface{}

func qaCapabilityCloneValue(value interface{}) interface{} {
	switch value := value.(type) {
	case map[string]interface{}:
		copy := qaCapabilityMap{}
		for key, entry := range value {
			copy[key] = qaCapabilityCloneValue(entry)
		}
		return copy
	case []interface{}:
		copy := make([]interface{}, len(value))
		for i, entry := range value {
			copy[i] = qaCapabilityCloneValue(entry)
		}
		return copy
	default:
		return value
	}
}

func qaCapabilityList(items ...qaCapabilityMap) []interface{} {
	result := make([]interface{}, len(items))
	for i, item := range items {
		result[i] = item
	}
	return result
}

// Each call owns fresh input data. The overlay replaces the shipped sample's
// names, domain, account and optional capabilities with synthetic values.
func qaCapabilityEnv() qaCapabilityMap {
	m := minimalEnvMap("public_ip")
	overrides := qaCapabilityMap{
		"project": "myapp", "env": "dev", "region": "us-east-1",
		"account_id": "000000000000", "aws_profile": "myapp-dev",
		"state_bucket": "myapp-state", "state_file": "dev.tfstate",
		"state_lock_table": "myapp-locks", "is_prod": false,
		"ecr_strategy": "local", "ecr_account_id": "", "ecr_account_region": "",
		"ecr_trusted_accounts": []interface{}{},
		"workload": qaCapabilityMap{
			"backend_runtime": "fargate", "backend_image_port": 8080,
			"backend_cpu": "256", "backend_memory": "512",
			// Nested overlays replace the sample's workload map, so retain its
			// explicit booleans even when these optional features are disabled.
			"bucket_public": false, "setup_fcnsns": false,
			"enable_github_oidc": false, "xray_enabled": false,
			"install_pg_admin":      false,
			"backend_env_variables": qaCapabilityMap{},
		},
		"domain": qaCapabilityMap{
			"enabled": true, "domain_name": "example.com", "create_domain_zone": false,
			"api_domain_prefix": "api", "add_env_domain_prefix": true,
		},
		"alb":      qaCapabilityMap{"enabled": false},
		"postgres": qaCapabilityMap{"enabled": false},
		"cognito":  qaCapabilityMap{"enabled": false},
		"ses":      qaCapabilityMap{"enabled": false},
		"sqs":      qaCapabilityMap{"enabled": false, "name": "jobs"},
		"services": qaCapabilityList(qaCapabilityMap{
			"name": "api", "runtime": "fargate", "container_port": 3000,
			"host_port": 3000, "cpu": 256, "memory": 512,
			"network_access": "public", "env_vars": qaCapabilityMap{},
			"ecr_config": qaCapabilityMap{"mode": "create_ecr"},
		}),
		"scheduled_tasks": qaCapabilityList(qaCapabilityMap{
			"name": "nightly", "schedule": "rate(1 day)",
			"environment_variables": qaCapabilityMap{},
			"ecr_config":            qaCapabilityMap{"mode": "create_ecr"},
		}),
		"event_processor_tasks": qaCapabilityList(qaCapabilityMap{
			"name": "mailer",
			"rules": qaCapabilityList(qaCapabilityMap{
				"name": "ingest", "sources": []interface{}{"myapp.api"},
				"detail_types": []interface{}{"order.created"},
			}),
			"environment_variables": qaCapabilityMap{},
			"ecr_config":            qaCapabilityMap{"mode": "create_ecr"},
		}),
		"amplify_apps": []interface{}{}, "cloudfront_distributions": []interface{}{},
		"efs": []interface{}{}, "buckets": []interface{}{},
		"extensions": qaCapabilityMap{}, "data_bucket": nil,
		"vaults": []interface{}{}, "event_bus": qaCapabilityMap{"enabled": false},
		"valkey":         qaCapabilityMap{"enabled": false},
		"github_runners": nil,
	}
	for key, value := range overrides {
		m[key] = value
	}
	return m
}

func qaCapabilityKinds() []string {
	return []string{"backend", "service", "scheduled_task", "event_task"}
}

// This reads only test-owned input, never implementation state.
func qaCapabilityOwner(m qaCapabilityMap, kind string) qaCapabilityMap {
	if kind == "backend" {
		return m["workload"].(qaCapabilityMap)
	}
	key := map[string]string{
		"service": "services", "scheduled_task": "scheduled_tasks",
		"event_task": "event_processor_tasks",
	}[kind]
	return m[key].([]interface{})[0].(qaCapabilityMap)
}

func qaCapabilityOwnerName(kind string) string {
	return map[string]string{
		"backend": "backend", "service": "api",
		"scheduled_task": "nightly", "event_task": "mailer",
	}[kind]
}

func qaCapabilityEnvField(kind string) string {
	if kind == "backend" {
		return "backend_env_variables"
	}
	if kind == "service" {
		return "env_vars"
	}
	return "environment_variables"
}

func qaCapabilityPrepareNames(t *testing.T, m qaCapabilityMap) {
	t.Helper()
	collectDisabledWorkloadNames(m)
	for _, key := range []string{"services", "scheduled_tasks", "event_processor_tasks"} {
		filterDisabledItems(m, key)
	}
	if err := validateWorkloadNamesConfigMap(m); err != nil {
		t.Fatalf("valid workload namespace refused: %v", err)
	}
}

func qaCapabilityRequireError(t *testing.T, err error, fragments ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("invalid configuration was accepted")
	}
	qaCapabilityContains(t, err.Error(), fragments...)
}

func qaCapabilityContains(t *testing.T, got string, fragments ...string) {
	t.Helper()
	for _, want := range fragments {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func qaCapabilityNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("valid configuration refused: %v", err)
	}
}

// Diagnostics are observable output. The file avoids pipe buffer limits and is
// restored even when the callback fails. Tests using process streams stay serial.
func qaCapabilityDiagnostics(t *testing.T, m qaCapabilityMap, f func() error) (error, string) {
	t.Helper()
	capture, err := os.CreateTemp(t.TempDir(), "diagnostics")
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	stdout, stderr := os.Stdout, os.Stderr
	var callErr error
	func() {
		os.Stdout, os.Stderr = capture, capture
		defer func() { os.Stdout, os.Stderr = stdout, stderr }()
		callErr = f()
	}()
	raw, err := os.ReadFile(capture.Name())
	if err != nil {
		t.Fatal(err)
	}
	return callErr, string(raw) + "\n" + strings.Join(generateWarnings(m), "\n")
}

func qaCapabilityParse(t *testing.T, rendered string) *hclsyntax.Body {
	t.Helper()
	file, diagnostics := hclsyntax.ParseConfig([]byte(rendered), "generated.tf", hcl.Pos{Line: 1, Column: 1})
	if diagnostics.HasErrors() {
		t.Fatalf("generated HCL does not parse: %s", diagnostics.Error())
	}
	return file.Body.(*hclsyntax.Body)
}

func qaCapabilityFindModule(body *hclsyntax.Body, name string) *hclsyntax.Body {
	for _, block := range body.Blocks {
		if block.Type == "module" && len(block.Labels) == 1 && block.Labels[0] == name {
			return block.Body
		}
	}
	return nil
}

func qaCapabilityModule(t *testing.T, body *hclsyntax.Body, name string) *hclsyntax.Body {
	t.Helper()
	block := qaCapabilityFindModule(body, name)
	if block == nil {
		t.Fatalf("generated HCL has no module %q", name)
	}
	return block
}

func qaCapabilityAttr(t *testing.T, body *hclsyntax.Body, name string) *hclsyntax.Attribute {
	t.Helper()
	attr := body.Attributes[name]
	if attr == nil {
		t.Fatalf("generated module has no %q input", name)
	}
	return attr
}

func qaCapabilityJSON(t *testing.T, expr hclsyntax.Expression, ctx *hcl.EvalContext) interface{} {
	t.Helper()
	value, diagnostics := expr.Value(ctx)
	if diagnostics.HasErrors() {
		t.Fatalf("cannot evaluate generated expression: %s", diagnostics.Error())
	}
	raw, err := ctyjson.Marshal(value, value.Type())
	if err != nil {
		t.Fatal(err)
	}
	var result interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func qaCapabilityLiteral(t *testing.T, body *hclsyntax.Body, name string) interface{} {
	t.Helper()
	return qaCapabilityJSON(t, qaCapabilityAttr(t, body, name).Expr, nil)
}

func qaCapabilityEqual(t *testing.T, got, want interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v\nwant %#v", got, want)
	}
}

func qaCapabilityWantJSON(t *testing.T, got interface{}, want string) {
	t.Helper()
	var decoded interface{}
	if err := json.Unmarshal([]byte(want), &decoded); err != nil {
		t.Fatalf("invalid test expectation: %v", err)
	}
	qaCapabilityEqual(t, got, decoded)
}

func qaCapabilityObject(t *testing.T, value interface{}) qaCapabilityMap {
	t.Helper()
	object, ok := value.(map[string]interface{})
	if !ok {
		t.Fatalf("expected object, got %T (%#v)", value, value)
	}
	return object
}

func qaCapabilityArray(t *testing.T, value interface{}) []interface{} {
	t.Helper()
	array, ok := value.([]interface{})
	if !ok {
		t.Fatalf("expected array, got %T (%#v)", value, value)
	}
	return array
}

func qaCapabilityNamed(t *testing.T, value interface{}, name string) qaCapabilityMap {
	t.Helper()
	for _, entry := range qaCapabilityArray(t, value) {
		object := qaCapabilityObject(t, entry)
		if object["name"] == name {
			return object
		}
	}
	t.Fatalf("missing entry named %q in %#v", name, value)
	return nil
}

func qaCapabilityExprText(expr hclsyntax.Expression, rendered string) string {
	return string(expr.Range().SliceBytes([]byte(rendered)))
}

func qaCapabilityStringSet(t *testing.T, value interface{}, want ...string) {
	t.Helper()
	var got []string
	for _, entry := range qaCapabilityArray(t, value) {
		name, ok := entry.(string)
		if !ok {
			t.Fatalf("expected string member, got %#v", entry)
		}
		got = append(got, name)
	}
	sort.Strings(got)
	expected := append([]string(nil), want...)
	sort.Strings(expected)
	qaCapabilityEqual(t, got, expected)
}

func qaCapabilityWrite(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

// HCL is evaluated with synthetic public module outputs, never live AWS data.
func qaCapabilityURLContext(apiHost string) *hcl.EvalContext {
	return &hcl.EvalContext{Variables: map[string]cty.Value{
		"module": cty.ObjectVal(map[string]cty.Value{
			"domain": cty.ObjectVal(map[string]cty.Value{
				"api_domain_name": cty.StringVal(apiHost),
			}),
			"workloads": cty.ObjectVal(map[string]cty.Value{
				"api_gateway_endpoint": cty.StringVal("https://example.execute-api.us-east-1.amazonaws.com/dev"),
			}),
		}),
	}}
}
