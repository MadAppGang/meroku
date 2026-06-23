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
	if t.Failed() {
		t.Logf("---- RENDER OUTPUT (scheduled+amplify excerpt) ----\n%s", out)
	}
}
