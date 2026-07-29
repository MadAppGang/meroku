package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aymerick/raymond"
	"gopkg.in/yaml.v2"
)

// renderMainTemplate renders env/main.hbs against a YAML config, exactly as the real
// generate pipeline does (registerCustomHelpers + convertToJSONCompatible + raymond).
func renderMainTemplate(t *testing.T, config string) string {
	t.Helper()
	registerCustomHelpers()

	var m map[string]interface{}
	if err := yaml.Unmarshal([]byte(config), &m); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	envMap, ok := convertToJSONCompatible(m).(map[string]interface{})
	if !ok {
		t.Fatal("convertToJSONCompatible did not return a map")
	}
	envMap["modules"] = "../modules"

	tpl, err := os.ReadFile(filepath.Join("..", "env", "main.hbs"))
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	parsed, err := raymond.Parse(string(tpl))
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}
	out, err := parsed.Exec(envMap)
	if err != nil {
		t.Fatalf("exec template: %v", err)
	}
	return out
}

const albBaseConfig = `
schema_version: 20
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
buckets: []
efs: []
services: []
domain:
  enabled: true
  domain_name: example.com
  api_domain_prefix: api
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
scheduled_tasks: []
event_processor_tasks: []
`

// The ALB must be enablable on its own. Previously the template only set enable_alb when
// workload.backend_alb_domain_name was ALSO set, which made "switch to the ALB" impossible
// without simultaneously changing the public hostname.
func TestALB_EnabledWithoutExtraHostname(t *testing.T) {
	out := renderMainTemplate(t, albBaseConfig+"alb:\n  enabled: true\n")

	if !strings.Contains(out, "enable_alb = true") {
		t.Error("alb.enabled did not produce enable_alb = true")
	}
	if !strings.Contains(out, "alb_arn = module.alb.alb_arn") {
		t.Error("alb_arn not wired to the alb module")
	}
	// The backend security group allows ingress from the ALB's security group, which is
	// created in modules/alb — so it must be passed across. Without this the ALB path
	// fails to plan: backend.tf used to reference a non-existent aws_security_group.alb.
	if !strings.Contains(out, "alb_security_group_id = module.alb.alb_security_group_id") {
		t.Error("alb_security_group_id not passed to the workloads module")
	}
	if strings.Contains(out, "backend_alb_domain_name =") {
		t.Error("backend_alb_domain_name must stay absent unless explicitly configured")
	}
	// The workloads module needs the env-resolved domain so it never re-derives the
	// "<env>." prefix itself (that rule lives in modules/domain).
	if !strings.Contains(out, "env_domain = module.domain.domain_name") {
		t.Error("env_domain not passed to the workloads module")
	}
}

// idle_timeout is the whole SSE story: an ALB drops a streaming connection after this many
// idle seconds. Absent, it must fall back to the AWS default of 60.
func TestALB_IdleTimeout(t *testing.T) {
	out := renderMainTemplate(t, albBaseConfig+"alb:\n  enabled: true\n  idle_timeout: 300\n")
	if !strings.Contains(out, "idle_timeout = 300") {
		t.Error("alb.idle_timeout not passed to the alb module")
	}

	def := renderMainTemplate(t, albBaseConfig+"alb:\n  enabled: true\n")
	if !strings.Contains(def, "idle_timeout = 60") {
		t.Error("idle_timeout should default to 60 when unset")
	}
}

// backend_alb_domain_name is now an optional EXTRA hostname, not the trigger for the ALB.
func TestALB_OptionalExtraHostname(t *testing.T) {
	cfg := strings.Replace(albBaseConfig,
		"  backend_health_endpoint: /health",
		"  backend_health_endpoint: /health\n  backend_alb_domain_name: backend", 1)
	out := renderMainTemplate(t, cfg+"alb:\n  enabled: true\n")

	if !strings.Contains(out, "enable_alb = true") {
		t.Error("enable_alb missing")
	}
	if !strings.Contains(out, `backend_alb_domain_name = "backend"`) {
		t.Error("backend_alb_domain_name not passed through when set")
	}
}

// The default AWS provider must pin the region from the YAML. Without it, the region came
// only from the shell/profile default, so the S3 backend used the YAML region while the
// resources were created in a completely different one.
func TestProviderPinsRegionFromYaml(t *testing.T) {
	out := renderMainTemplate(t, albBaseConfig+"alb:\n  enabled: false\n")

	if !strings.Contains(out, "provider \"aws\" {\n  region = \"us-east-1\"\n}") {
		t.Error("default aws provider does not pin region from the YAML config")
	}
}

// With the ALB off, nothing ALB-related may leak into the generated Terraform.
func TestALB_DisabledEmitsNothing(t *testing.T) {
	out := renderMainTemplate(t, albBaseConfig+"alb:\n  enabled: false\n")

	if strings.Contains(out, "enable_alb = true") {
		t.Error("enable_alb must not be set when alb.enabled is false")
	}
	if strings.Contains(out, `module "alb"`) {
		t.Error("alb module must not be rendered when alb.enabled is false")
	}
}
