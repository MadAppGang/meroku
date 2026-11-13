package main

import (
	"strings"
	"testing"
)

// amplifyTestEnv is a minimal environment with one public service (api), one
// switched-off service (old) and one scheduled task (nightly), and the given
// amplify_apps.
func amplifyTestEnv(apps []interface{}) map[string]interface{} {
	return map[string]interface{}{
		"project": "myapp",
		"env":     "dev",
		"domain": map[string]interface{}{
			"enabled":           true,
			"domain_name":       "example.com",
			"api_domain_prefix": "api",
		},
		"services": []interface{}{
			map[string]interface{}{"name": "api"},
			map[string]interface{}{"name": "old", "enabled": false},
		},
		"scheduled_tasks": []interface{}{
			map[string]interface{}{"name": "nightly", "schedule": "rate(1 hours)"},
		},
		"amplify_apps": apps,
	}
}

func amplifyApp(rules ...interface{}) map[string]interface{} {
	return map[string]interface{}{
		"name":              "web",
		"github_repository": "example/web",
		"branches":          []interface{}{map[string]interface{}{"name": "main"}},
		"custom_rules":      rules,
	}
}

func amplifyRule(fields ...string) map[string]interface{} {
	rule := map[string]interface{}{}
	for i := 0; i+1 < len(fields); i += 2 {
		rule[fields[i]] = fields[i+1]
	}
	return rule
}

func renderedRules(t *testing.T, envMap map[string]interface{}) []map[string]interface{} {
	t.Helper()
	apps, _ := envMap["amplify_apps"].([]interface{})
	app, ok := apps[0].(map[string]interface{})
	if !ok {
		t.Fatalf("amplify_apps[0] is %T, want a string-keyed map", apps[0])
	}
	list, _ := app["custom_rules"].([]interface{})
	out := make([]map[string]interface{}, 0, len(list))
	for _, raw := range list {
		rule, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("custom_rules entry is %T, want a string-keyed map", raw)
		}
		out = append(out, rule)
	}
	return out
}

// A rule naming a switched-off service is left out with a warning, the way
// every other reference to a switched-off workload is; the rest of the app's
// rules are kept in order.
func TestAmplifyRules_SwitchedOffWorkloadIsDroppedWithWarning(t *testing.T) {
	envMap := amplifyTestEnv([]interface{}{amplifyApp(
		amplifyRule("source", "/api/<*>", "workload", "backend"),
		amplifyRule("source", "/old/<*>", "workload", "old"),
		amplifyRule("source", "/svc/<*>", "workload", "api"),
	)})

	if err := preprocessEnvMap(envMap); err != nil {
		t.Fatalf("preprocessEnvMap: %v", err)
	}

	rules := renderedRules(t, envMap)
	var workloads []string
	for _, r := range rules {
		workloads = append(workloads, r["workload"].(string))
	}
	if got := strings.Join(workloads, ","); got != "backend,api" {
		t.Errorf("kept rules name %s, want backend,api", got)
	}

	warnings := strings.Join(generateWarnings(envMap), "\n")
	if !strings.Contains(warnings, "custom_rules[1] names workload 'old', which is switched off") {
		t.Errorf("want a warning naming custom_rules[1] and 'old', got:\n%s", warnings)
	}
}

func TestAmplifyRules_UnknownAndNonServiceWorkloadsAreErrors(t *testing.T) {
	tests := []struct {
		name     string
		workload string
		want     string
	}{
		{"a name that is no workload", "ghost", "workload 'ghost' is not a workload"},
		{"a scheduled task", "nightly", "workload 'nightly' must be backend or a service"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			envMap := amplifyTestEnv([]interface{}{amplifyApp(
				amplifyRule("source", "/x/<*>", "workload", tc.workload),
			)})
			err := preprocessEnvMap(envMap)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
		})
	}
}

// A YAML loaded with yaml.v2 has map[interface{}]interface{} for every
// mapping. The rules must be validated and normalised all the same, and the
// normalised fields must land in maps the template reads.
func TestAmplifyRules_YAMLv2MapsAreValidatedAndNormalised(t *testing.T) {
	v2Rule := func(fields ...string) map[interface{}]interface{} {
		rule := map[interface{}]interface{}{}
		for i := 0; i+1 < len(fields); i += 2 {
			rule[fields[i]] = fields[i+1]
		}
		return rule
	}
	v2App := func(rules ...interface{}) map[interface{}]interface{} {
		return map[interface{}]interface{}{
			"name":         "web",
			"branches":     []interface{}{map[interface{}]interface{}{"name": "main"}},
			"custom_rules": rules,
		}
	}

	t.Run("an invalid rule is refused", func(t *testing.T) {
		envMap := amplifyTestEnv([]interface{}{v2App(v2Rule("source", "/a/<*>/b"))})
		err := preprocessEnvMap(envMap)
		if err == nil || !strings.Contains(err.Error(), "only once, at the end") {
			t.Fatalf("want the wildcard error, got %v", err)
		}
	})

	t.Run("a valid rule is normalised in place", func(t *testing.T) {
		envMap := amplifyTestEnv([]interface{}{v2App(v2Rule("source", "/api/<*>", "workload", "backend"))})
		if err := preprocessEnvMap(envMap); err != nil {
			t.Fatalf("preprocessEnvMap: %v", err)
		}
		rules := renderedRules(t, envMap)
		if len(rules) != 1 || rules[0][amplifyWorkloadRuleKey] != true || rules[0][amplifyTargetPathKey] != "/api/<*>" {
			t.Errorf("rule not normalised: %#v", rules)
		}
	})
}

func TestAmplifyRules_MalformedValuesAreErrors(t *testing.T) {
	tests := []struct {
		name string
		apps []interface{}
		want string
	}{
		{
			name: "a rule that is not a mapping",
			apps: []interface{}{amplifyApp("/api/<*>")},
			want: "custom_rules[0] must be a mapping",
		},
		{
			name: "custom_rules that is not a list",
			apps: []interface{}{map[string]interface{}{"name": "web", "custom_rules": "nope"}},
			want: "custom_rules must be a list",
		},
		{
			name: "an app that is not a mapping",
			apps: []interface{}{"web"},
			want: "amplify_apps[0] must be a mapping",
		},
		{
			name: "a wildcard in the middle of path",
			apps: []interface{}{amplifyApp(amplifyRule("source", "/r/<*>", "workload", "api", "path", "/<*>/x"))},
			want: `path "/<*>/x" may use <*> only once, at the end`,
		},
		{
			name: "two wildcards in target",
			apps: []interface{}{amplifyApp(amplifyRule("source", "/r/<*>", "target", "/<*>/<*>"))},
			want: `target "/<*>/<*>" may use <*> only once, at the end`,
		},
		{
			name: "a wildcard in target with none in source",
			apps: []interface{}{amplifyApp(amplifyRule("source", "/r", "target", "/new/<*>"))},
			want: `target "/new/<*>" uses <*>, but source "/r" has none`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := preprocessEnvMap(amplifyTestEnv(tc.apps))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got %v", tc.want, err)
			}
		})
	}
}

// The app host follows module "domain": under a delegated subdomain
// (root_zone_id) it is <prefix>.<env>.<domain> even with
// add_env_domain_prefix: false, so the self-loop check compares real hosts.
func TestAmplifyAppHost_DelegatedSubdomainAlwaysHasEnv(t *testing.T) {
	envMap := map[string]interface{}{
		"env": "dev",
		"domain": map[string]interface{}{
			"domain_name":           "example.com",
			"root_zone_id":          "Z000000000000",
			"add_env_domain_prefix": false,
		},
	}
	app := map[string]interface{}{"subdomain_prefix": "app"}
	if got := amplifyAppHost(envMap, app); got != "app.dev.example.com" {
		t.Errorf("amplifyAppHost = %q, want app.dev.example.com", got)
	}

	envMap["domain"].(map[string]interface{})["root_zone_id"] = ""
	if got := amplifyAppHost(envMap, app); got != "app.example.com" {
		t.Errorf("without root_zone_id, amplifyAppHost = %q, want app.example.com", got)
	}
}

func TestAmplifyRules_SelfLoopDetectedUnderDelegatedSubdomain(t *testing.T) {
	envMap := amplifyTestEnv([]interface{}{map[string]interface{}{
		"name":             "web",
		"subdomain_prefix": "api",
		"custom_rules":     []interface{}{amplifyRule("source", "/api/<*>", "workload", "backend")},
	}})
	domain := envMap["domain"].(map[string]interface{})
	domain["root_zone_id"] = "Z000000000000"
	domain["add_env_domain_prefix"] = false

	err := preprocessEnvMap(envMap)
	if err == nil || !strings.Contains(err.Error(), "api.dev.example.com is also the host") {
		t.Fatalf("want the self-loop error for api.dev.example.com, got %v", err)
	}
}

func TestAmplifyRewriteWarnings_KeepTheIndexWrittenInYAML(t *testing.T) {
	envMap := amplifyTestEnv([]interface{}{amplifyApp(
		amplifyRule("source", "/old/<*>", "workload", "old"),
		amplifyRule("source", "/<*>", "target", "/index.html"),
		amplifyRule("source", "/late", "target", "/x"),
	)})
	if err := preprocessEnvMap(envMap); err != nil {
		t.Fatalf("preprocessEnvMap: %v", err)
	}
	warnings := strings.Join(amplifyRewriteWarnings(envMap), "\n")
	if !strings.Contains(warnings, "custom_rules[2] comes after the catch-all custom_rules[1]") {
		t.Errorf("want the YAML indexes 2 and 1, got:\n%s", warnings)
	}
}

// Rendered through the real template: app-level values with quotes and angle
// brackets come out as valid HCL strings, never HTML entities, and a branch
// without environment_variables does not inherit the app's.
func TestAmplifyTemplate_EnvVarsAndRulesRenderAsHCL(t *testing.T) {
	rendered := renderMainHBS(t, map[string]interface{}{
		"amplify_apps": []interface{}{map[string]interface{}{
			"name":              "web",
			"github_repository": "example/web",
			"branches": []interface{}{
				map[string]interface{}{"name": "main"},
				map[string]interface{}{"name": "dev", "environment_variables": map[string]interface{}{"STAGE": `a "b"`}},
			},
			"environment_variables": map[string]interface{}{"QUOTED": `say "hi" <b>&</b>`},
			"custom_rules": []interface{}{
				amplifyRule("source", "/api/<*>", "workload", "backend"),
				amplifyRule("source", "/old", "target", "/new", "status", "301"),
			},
		}},
	})
	block := amplifyModuleBlock(t, rendered)

	for _, entity := range []string{"&quot;", "&lt;", "&gt;", "&amp;", "&#"} {
		if strings.Contains(block, entity) {
			t.Errorf("the amplify block contains the HTML entity %s:\n%s", entity, block)
		}
	}
	for _, want := range []string{
		`"QUOTED" = "say \"hi\" \u003cb\u003e\u0026\u003c/b\u003e"`,
		`STAGE = "a \"b\""`,
		`source = "/api/\u003c*\u003e"`,
		`workload    = "backend"`,
		`status = "301"`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("want %s in the amplify block, got:\n%s", want, block)
		}
	}
	if n := strings.Count(block, "QUOTED"); n != 1 {
		t.Errorf("QUOTED appears %d times; a branch without environment_variables must not inherit the app's:\n%s", n, block)
	}
}

func TestAmplifyTemplate_DelegatedSubdomainPassesEnvPrefix(t *testing.T) {
	app := []interface{}{map[string]interface{}{
		"name": "web", "github_repository": "example/web", "subdomain_prefix": "app",
		"branches": []interface{}{map[string]interface{}{"name": "main"}},
	}}
	tests := []struct {
		name   string
		domain map[string]interface{}
		want   string // "" means the argument must be absent
	}{
		{
			name:   "root_zone_id overrides add_env_domain_prefix false",
			domain: map[string]interface{}{"enabled": true, "domain_name": "example.com", "root_zone_id": "Z000000000000", "root_account_id": "000000000000", "add_env_domain_prefix": false},
			want:   "add_env_domain_prefix = true",
		},
		{
			name:   "without root_zone_id the YAML value is passed",
			domain: map[string]interface{}{"enabled": true, "domain_name": "example.com", "create_domain_zone": true, "add_env_domain_prefix": false},
			want:   "add_env_domain_prefix = false",
		},
		{
			name:   "absent stays absent",
			domain: map[string]interface{}{"enabled": true, "domain_name": "example.com", "create_domain_zone": true},
			want:   "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			block := amplifyModuleBlock(t, renderMainHBS(t, map[string]interface{}{"domain": tc.domain, "amplify_apps": app}))
			if tc.want == "" {
				if strings.Contains(block, "add_env_domain_prefix") {
					t.Errorf("add_env_domain_prefix should be absent, got:\n%s", block)
				}
				return
			}
			if !strings.Contains(block, tc.want) {
				t.Errorf("want %q in the amplify block, got:\n%s", tc.want, block)
			}
		})
	}
}
