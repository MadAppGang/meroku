package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aymerick/raymond"
)

// policyCorsFixture is a minimal but fully valid env YAML exercising the two
// regression surfaces of this test file:
//   - workload.policy -> backend_policy rendering (IAM statements)
//   - buckets[].cors_rules -> modules/s3 pass-through
const policyCorsFixture = `
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
  policy:
  - actions:
    - s3:GetObject
    - s3:PutObject
    - s3:DeleteObject
    resources:
    - arn:aws:s3:::sample-uploads-dev
    - arn:aws:s3:::sample-uploads-dev/*
buckets:
- name: uploads
  public: true
  versioning: false
  cors_rules:
  - allowed_headers:
    - '*'
    allowed_methods:
    - DELETE
    - GET
    - HEAD
    - POST
    - PUT
    allowed_origins:
    - '*'
    expose_headers:
    - ETag
    max_age_seconds: 3600
- name: plain
  public: false
  versioning: true
services: []
efs: []
domain:
  enabled: false
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
scheduled_tasks: []
event_processor_tasks: []
`

// renderMainHbs renders the real env/main.hbs with the same pipeline as
// deploy.go applyTemplate: loadEnvToMap -> filterDisabledItems -> raymond.
func renderMainHbs(t *testing.T, yamlPath string) string {
	t.Helper()

	// raymond keeps helper registration in global state; another test in this
	// binary may have registered the helpers already and re-registration panics.
	func() {
		defer func() { _ = recover() }()
		registerCustomHelpers()
	}()

	envMap, err := loadEnvToMap(yamlPath)
	if err != nil {
		t.Fatalf("loadEnvToMap: %v", err)
	}
	filterDisabledItems(envMap, "services")
	filterDisabledItems(envMap, "scheduled_tasks")
	filterDisabledItems(envMap, "event_processor_tasks")
	envMap["modules"] = "../../infrastructure/modules"
	envMap["custom_modules"] = "../../custom"
	envMap["has_custom_pre"] = false
	envMap["has_custom_post"] = false

	tplBytes, err := os.ReadFile(filepath.Join("..", "env", "main.hbs"))
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	tmpl, err := raymond.Parse(string(tplBytes))
	if err != nil {
		t.Fatalf("template parse failed: %v", err)
	}
	out, err := tmpl.Exec(envMap)
	if err != nil {
		t.Fatalf("template exec failed: %v", err)
	}
	return out
}

// assertPolicyRendered asserts the backend_policy block contains the fixture's
// IAM actions and resources — the exact surface that regressed when
// workload.policy was dropped and terraform plan proposed deleting the live
// backend IAM policy.
func assertPolicyRendered(t *testing.T, out, context string) {
	t.Helper()
	for _, want := range []string{
		`actions = ["s3:GetObject","s3:PutObject","s3:DeleteObject"]`,
		`resources = ["arn:aws:s3:::sample-uploads-dev","arn:aws:s3:::sample-uploads-dev/*"]`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("%s: rendered output missing %q", context, want)
		}
	}
	if strings.Contains(out, "backend_policy = [\n  ]") {
		t.Errorf("%s: backend_policy rendered EMPTY", context)
	}
}

// TestRenderWorkloadPolicy renders the fixture through the real pipeline and
// asserts workload.policy materializes as a populated backend_policy block.
// []interface{} (raw YAML shape) must flow through the array helper intact.
func TestRenderWorkloadPolicy(t *testing.T) {
	dir := t.TempDir()
	yfile := filepath.Join(dir, "dev.yaml")
	if err := os.WriteFile(yfile, []byte(policyCorsFixture), 0o644); err != nil {
		t.Fatal(err)
	}

	out := renderMainHbs(t, yfile)
	assertPolicyRendered(t, out, "raw yaml render")
}

// TestRenderBucketCorsRules asserts buckets[].cors_rules pass through the
// template into the modules/s3 buckets JSON, and that a bucket WITHOUT
// cors_rules does not gain an explicit empty list (an explicit `"cors_rules":[]`
// defeats the module's optional default and deletes the live CORS config).
func TestRenderBucketCorsRules(t *testing.T) {
	dir := t.TempDir()
	yfile := filepath.Join(dir, "dev.yaml")
	if err := os.WriteFile(yfile, []byte(policyCorsFixture), 0o644); err != nil {
		t.Fatal(err)
	}

	out := renderMainHbs(t, yfile)

	bucketsLine := extractLineContaining(t, out, "buckets = [")
	for _, want := range []string{
		`"allowed_headers":["*"]`,
		`"allowed_methods":["DELETE","GET","HEAD","POST","PUT"]`,
		`"allowed_origins":["*"]`,
		`"expose_headers":["ETag"]`,
		`"max_age_seconds":3600`,
	} {
		if !strings.Contains(bucketsLine, want) {
			t.Errorf("buckets JSON missing cors rule fragment %q; got:\n%s", want, bucketsLine)
		}
	}
	if strings.Contains(bucketsLine, `"cors_rules":[]`) {
		t.Errorf("bucket without cors_rules must not render an explicit empty list; got:\n%s", bucketsLine)
	}
}

// TestTypedRoundTripPreservesPolicyAndCors is the root-cause regression: a
// typed Env load->save round-trip (schema migration, web UI save, profile
// update all take this path) must NOT drop workload.policy from the YAML and
// must NOT inject `cors_rules: []` into buckets. Before the Workload.Policy
// field and the BucketConfig omitempty fix, this exact round-trip produced a
// main.tf with an empty backend_policy and explicit empty cors_rules — and
// terraform plan proposed deleting the live IAM policy and CORS configs.
func TestTypedRoundTripPreservesPolicyAndCors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dev.yaml"), []byte(policyCorsFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	e, err := loadEnv("dev")
	if err != nil {
		t.Fatalf("typed loadEnv: %v", err)
	}
	if err := saveEnv(e); err != nil {
		t.Fatalf("saveEnv: %v", err)
	}

	data, err := os.ReadFile("dev.yaml")
	if err != nil {
		t.Fatal(err)
	}
	roundTripped := string(data)
	if !strings.Contains(roundTripped, "policy:") {
		t.Errorf("typed round-trip DROPPED workload.policy from the YAML:\n%s", roundTripped)
	}
	if strings.Contains(roundTripped, "cors_rules: []") {
		t.Errorf("typed round-trip injected explicit empty cors_rules:\n%s", roundTripped)
	}

	// chdir back so renderMainHbs can resolve ../env/main.hbs.
	if err := os.Chdir(wd); err != nil {
		t.Fatal(err)
	}
	out := renderMainHbs(t, filepath.Join(dir, "dev.yaml"))
	assertPolicyRendered(t, out, "post round-trip render")

	bucketsLine := extractLineContaining(t, out, "buckets = [")
	if !strings.Contains(bucketsLine, `"max_age_seconds":3600`) {
		t.Errorf("round-trip lost explicit bucket cors_rules; got:\n%s", bucketsLine)
	}
	if strings.Contains(bucketsLine, `"cors_rules":[]`) {
		t.Errorf("round-trip render contains explicit empty cors_rules; got:\n%s", bucketsLine)
	}
}

// extractLineContaining returns the first output line containing the marker.
func extractLineContaining(t *testing.T, out, marker string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, marker) {
			return line
		}
	}
	t.Fatalf("no line containing %q in render output", marker)
	return ""
}
