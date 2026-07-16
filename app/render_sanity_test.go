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
services:
  - name: api
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
  - name: reuse
    schedule: "rate(3 days)"
    ecr_config:
      mode: use_existing
      source_service_name: api
      source_service_type: services
  - name: pinned
    schedule: "rate(4 days)"
    docker_image: "pinned:1.2.3"
    ecr_config:
      mode: use_existing
      source_service_name: api
      source_service_type: services
event_processor_tasks:
  - name: notifier
    docker_image: "img:latest"
    container_command: ["bun", "run", "jobs/notify.ts"]
    rules:
      - name: invoice-paid
        sources: ["sample.billing"]
        detail_types: ["InvoicePaid"]
    environment_variables:
      FOO: "bar"
      LOG_LEVEL: "info"
  - name: silent
    docker_image: "img:latest"
    rules:
      - name: anything
        sources: ["sample.x"]
        detail_types: ["Anything"]
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
		`realtime_parent_domain = module.domain.domain_name`,
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

	// ECR use_existing (ecr_config mode "use_existing"): the "reuse" task shares the
	// "api" service's repository — the module must skip its per-task ECR repo and the
	// image must resolve from the workloads service ECR map (same precedence as
	// services.tf: an explicit docker_image wins over the resolved repository).
	reuseBlock := scheduledTaskModuleBlock(t, out, "reuse")
	if !strings.Contains(reuseBlock, "create_ecr_repo = false") {
		t.Errorf("reuse task missing create_ecr_repo = false; got:\n%s", reuseBlock)
	}
	if !strings.Contains(reuseBlock, `docker_image = "${module.workloads.service_ecr_url_map["api"]}:latest"`) {
		t.Errorf("reuse task docker_image not resolved from the source service ECR repo; got:\n%s", reuseBlock)
	}
	pinnedBlock := scheduledTaskModuleBlock(t, out, "pinned")
	if !strings.Contains(pinnedBlock, `docker_image = "pinned:1.2.3"`) {
		t.Errorf("pinned task must keep its explicit docker_image; got:\n%s", pinnedBlock)
	}
	if !strings.Contains(pinnedBlock, "create_ecr_repo = false") {
		t.Errorf("pinned task missing create_ecr_repo = false; got:\n%s", pinnedBlock)
	}
	if strings.Contains(pinnedBlock, "service_ecr_url_map") {
		t.Errorf("pinned task must NOT resolve docker_image from the ECR map (explicit image wins); got:\n%s", pinnedBlock)
	}
	// Feature-off: tasks without ecr_config use_existing render unchanged.
	if strings.Contains(noenvBlock, "create_ecr_repo") {
		t.Errorf("noenv task must NOT render create_ecr_repo; got:\n%s", noenvBlock)
	}

	// Event-processor-task environment_variables -> custom_env_vars (same mechanism,
	// rendered into the modules/event_bridge_task module which accepts custom_env_vars).
	// The "notifier" task declares env vars and MUST render a custom_env_vars block; the
	// "silent" task declares none and MUST NOT render one.
	notifierBlock := eventProcessorTaskModuleBlock(t, out, "notifier")
	if !strings.Contains(notifierBlock, "custom_env_vars = [") {
		t.Errorf("notifier event task missing custom_env_vars block; got:\n%s", notifierBlock)
	}
	for _, kv := range []string{
		`{ "name" : "FOO", "value" : "bar" }`,
		`{ "name" : "LOG_LEVEL", "value" : "info" }`,
	} {
		if !strings.Contains(notifierBlock, kv) {
			t.Errorf("notifier event task custom_env_vars missing entry %q; got:\n%s", kv, notifierBlock)
		}
	}
	// container_command on an event task must render as a valid HCL list (regression:
	// it previously rendered the raw []string via {{{container_command}}}, now uses {{{array}}}).
	if !strings.Contains(notifierBlock, `container_command = ["bun","run","jobs/notify.ts"]`) {
		t.Errorf("notifier event task container_command not rendered as HCL list; got:\n%s", notifierBlock)
	}

	silentBlock := eventProcessorTaskModuleBlock(t, out, "silent")
	if strings.Contains(silentBlock, "custom_env_vars") {
		t.Errorf("silent event task must NOT render custom_env_vars; got:\n%s", silentBlock)
	}
	// Feature-off: an event task without container_command renders no command argument
	// (the event_bridge_task module defaults it to [] and omits the container command).
	if strings.Contains(silentBlock, "container_command") {
		t.Errorf("silent event task must NOT render container_command; got:\n%s", silentBlock)
	}

	// Backward compat: a config WITHOUT enable_realtime_alb renders none of the
	// realtime wiring (feature-off negative — old configs stay unchanged).
	// This mutates envMap, so it runs after every assertion on `out`.
	workloadMap, ok := envMap["workload"].(map[string]interface{})
	if !ok {
		t.Fatal("workload map missing from envMap")
	}
	delete(workloadMap, "enable_realtime_alb")
	outOff, err := tmpl.Exec(envMap)
	if err != nil {
		t.Fatalf("template exec (realtime off) failed: %v", err)
	}
	for _, absent := range []string{"enable_realtime_alb", "realtime_parent_domain"} {
		if strings.Contains(outOff, absent) {
			t.Errorf("realtime-off render must not contain %q", absent)
		}
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

// eventProcessorTaskModuleBlock extracts the rendered `module "event_bus_task_<name>" { ... }`
// block from the template output so assertions can be scoped to one event-processor task.
// The template renders one module per event-processor task as `module "event_bus_task_{{name}}"`.
func eventProcessorTaskModuleBlock(t *testing.T, out, name string) string {
	t.Helper()
	header := `module "event_bus_task_` + name + `"`
	start := strings.Index(out, header)
	if start < 0 {
		t.Fatalf("event processor task module block %q not found in render output", header)
	}
	rest := out[start:]
	// Block ends at the next top-level `module "` declaration or EOF.
	if next := strings.Index(rest[len(header):], "\nmodule \""); next >= 0 {
		return rest[:len(header)+next]
	}
	return rest
}
