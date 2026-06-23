package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aymerick/raymond"
	"gopkg.in/yaml.v2"
)

func TestUpstreamRenderSanity(t *testing.T) {
	sample := `
schema_version: 21
project: sample
env: dev
region: us-east-1
account_id: "000000000000"
state_bucket: b
state_file: state.tfstate
workload:
  backend_image_port: 8080
  backend_health_endpoint: /health
  github_oidc_subjects: []
  env_files_s3: []
  policy: []
  enable_realtime_alb: true
  realtime_subdomain_prefix: realtime
  realtime_alb_idle_timeout: 600
manage_dns_records: true
buckets: []
services: []
efs: []
domain:
  enabled: true
  domain_name: example.com
postgres:
  enabled: false
cognito:
  enabled: false
  dashboard_callback_urls: []
  auto_verified_attributes: []
ses:
  enabled: false
  test_emails: []
sqs:
  enabled: false
alb:
  enabled: false
scheduled_tasks:
  - name: cleanup
    schedule: "rate(1 days)"
    timezone: "Australia/Sydney"
    container_command: ["bun", "run", "jobs/cleanup.ts"]
    max_retry_attempts: 5
    dlq_arn: "arn:aws:sqs:us-east-1:000000000000:sample-dlq"
    docker_image: "img:latest"
    environment_variables:
      FOO: "bar"
      LOG_LEVEL: "info"
  - name: noenv
    schedule: "rate(2 days)"
    docker_image: "img:latest"
amplify_apps:
  - name: portal
    github_repository: "https://github.com/x/y"
    branches:
      - name: main
`
	dir := t.TempDir()
	yfile := filepath.Join(dir, "dev.yaml")
	if err := os.WriteFile(yfile, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}

	// Register the custom handlebars helpers (array/default/exists/...) exactly as
	// the real render pipeline does (deploy.go applyTemplate / main.go). raymond keeps
	// helper registration in global state, so another test in this binary may have
	// already registered them — registerCustomHelpers panics on a duplicate. Recover
	// so this test is order-independent and the helpers are guaranteed present.
	func() {
		defer func() { _ = recover() }()
		registerCustomHelpers()
	}()

	var m map[string]interface{}
	if err := yaml.Unmarshal([]byte(sample), &m); err != nil {
		t.Fatal(err)
	}
	envMap, ok := convertToJSONCompatible(m).(map[string]interface{})
	if !ok {
		t.Fatal("convertToJSONCompatible did not return a map")
	}
	// inject modules path placeholder used by template
	envMap["modules"] = "../modules"

	tplBytes, err := os.ReadFile(filepath.Join("..", "env", "main.hbs"))
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := raymond.Parse(string(tplBytes))
	if err != nil {
		t.Fatalf("template parse failed: %v", err)
	}
	out, err := tmpl.Exec(envMap)
	if err != nil {
		t.Fatalf("template exec failed: %v", err)
	}

	checks := []string{
		`container_command = ["bun","run","jobs/cleanup.ts"]`,
		`schedule_expression_timezone = "Australia/Sydney"`,
		`max_retry_attempts = 5`,
		`dlq_arn = "arn:aws:sqs:us-east-1:000000000000:sample-dlq"`,
		`enable_realtime_alb = true`,
		`realtime_subdomain_prefix = "realtime"`,
		`realtime_alb_idle_timeout = 600`,
		`manage_dns_records = true`,
	}
	for _, c := range checks {
		if !strings.Contains(out, c) {
			t.Errorf("rendered output missing expected fragment: %q", c)
		}
	}

	// Scheduled-task environment_variables -> custom_env_vars (same mechanism as
	// backend_env_variables). The "cleanup" task declares env vars and MUST render a
	// custom_env_vars block; the "noenv" task declares none and MUST NOT render one.
	cleanupBlock := scheduledTaskModuleBlock(t, out, "cleanup")
	if !strings.Contains(cleanupBlock, "custom_env_vars = [") {
		t.Errorf("cleanup task missing custom_env_vars block; got:\n%s", cleanupBlock)
	}
	for _, kv := range []string{
		`{ "name" : "FOO", "value" : "bar" }`,
		`{ "name" : "LOG_LEVEL", "value" : "info" }`,
	} {
		if !strings.Contains(cleanupBlock, kv) {
			t.Errorf("cleanup task custom_env_vars missing entry %q; got:\n%s", kv, cleanupBlock)
		}
	}

	noenvBlock := scheduledTaskModuleBlock(t, out, "noenv")
	if strings.Contains(noenvBlock, "custom_env_vars") {
		t.Errorf("noenv task must NOT render custom_env_vars; got:\n%s", noenvBlock)
	}

	if t.Failed() {
		t.Logf("---- RENDER OUTPUT (scheduled+amplify excerpt) ----\n%s", out)
	}
}

// scheduledTaskModuleBlock extracts the rendered `module "schedule_task_<name>" { ... }`
// block from the template output so assertions can be scoped to one task. The template
// renders one module per scheduled task as `module "schedule_task_{{name}}"`.
func scheduledTaskModuleBlock(t *testing.T, out, name string) string {
	t.Helper()
	header := `module "schedule_task_` + name + `"`
	start := strings.Index(out, header)
	if start < 0 {
		t.Fatalf("scheduled task module block %q not found in render output", header)
	}
	rest := out[start:]
	// Block ends at the next top-level `module "` declaration or EOF.
	if next := strings.Index(rest[len(header):], "\nmodule \""); next >= 0 {
		return rest[:len(header)+next]
	}
	return rest
}
