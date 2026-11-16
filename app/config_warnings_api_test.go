package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const configWarningsTestYAML = `schema_version: 32
project: myapp
env: dev
is_prod: false
region: us-east-1
account_id: "000000000000"
aws_profile: myapp-dev
state_bucket: myapp-terraform-state-dev
state_file: state.tfstate
state_lock_table: myapp-terraform-locks-dev
use_default_vpc: false
vpc_cidr: 10.20.0.0/16
egress_strategy: public_ip
ecr_strategy: local
domain:
  enabled: true
  create_domain_zone: true
  domain_name: example.com
workload:
  backend_image_port: 8080
  backend_cpu: "256"
  backend_memory: "512"
  backend_desired_count: 1
  backend_env_variables:
    LOG_LEVEL: info
services: []
scheduled_tasks: []
event_processor_tasks: []
`

func getConfigWarningsForYAML(t *testing.T, yamlText, target string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	dir := t.TempDir()
	envFile := filepath.Join(dir, "dev.yaml")
	if err := os.WriteFile(envFile, []byte(yamlText), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	response := httptest.NewRecorder()
	getConfigWarnings(response, req)
	return response, envFile
}

func decodeConfigWarningReport(t *testing.T, response *httptest.ResponseRecorder) ConfigWarningReport {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", response.Code, response.Body.String())
	}
	var report ConfigWarningReport
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON report: %v; body: %s", err, response.Body.String())
	}
	return report
}

// REQ-FR-1, REQ-FR-4, AC-1: the complete acceptance report is observable over HTTP.
func TestGetConfigWarningsReturnsInternalServiceAcceptanceWarning(t *testing.T) {
	yamlText := strings.Replace(configWarningsTestYAML, "services: []", `services:
  - name: magento-bridge
    network_access: internal
    container_port: 8080
    cpu: 256
    memory: 512
    desired_count: 1`, 1)
	response, _ := getConfigWarningsForYAML(t, yamlText, "/api/config-warnings?env=dev")
	report := decodeConfigWarningReport(t, response)

	if report.Env != "dev" || report.ConfigError != "" {
		t.Fatalf("report metadata = env %q, configError %q", report.Env, report.ConfigError)
	}
	if len(report.Warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %#v", len(report.Warnings), report.Warnings)
	}
	warning := report.Warnings[0]
	if warning.Code != "service_no_health_check" {
		t.Errorf("code = %q, want service_no_health_check", warning.Code)
	}
	if warning.Item != (WarningItem{Kind: "service", Name: "magento-bridge"}) {
		t.Errorf("item = %#v, want service/magento-bridge", warning.Item)
	}
	wantTitle := "service 'magento-bridge' is network_access: internal and has no health_check_command"
	if warning.Title != wantTitle {
		t.Errorf("title = %q, want %q", warning.Title, wantTitle)
	}
	if !strings.Contains(warning.Detail, "Add a health_check_command") || !strings.Contains(warning.Detail, "distroless") {
		t.Errorf("detail lacks the acceptance guidance: %q", warning.Detail)
	}
	wantDocs := "https://github.com/MadAppGang/meroku/blob/main/ai_docs/SERVICE_NETWORK_ACCESS.md#check-that-the-tool-is-in-the-image"
	if warning.Docs != wantDocs {
		t.Errorf("docs = %q, want %q", warning.Docs, wantDocs)
	}
}

// AC-4 and contract 8.2: clean YAML encodes [] and omits configError.
func TestGetConfigWarningsCleanYAMLReturnsEmptyArray(t *testing.T) {
	response, _ := getConfigWarningsForYAML(t, configWarningsTestYAML, "/api/config-warnings?env=dev")
	report := decodeConfigWarningReport(t, response)
	if report.Env != "dev" || report.Warnings == nil || len(report.Warnings) != 0 {
		t.Fatalf("clean report = %#v, want env dev and non-nil []", report)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, present := raw["configError"]; present {
		t.Fatalf("clean report unexpectedly contains configError: %s", response.Body.String())
	}
	if value, ok := raw["warnings"].([]interface{}); !ok || len(value) != 0 {
		t.Fatalf("JSON warnings = %#v, want []", raw["warnings"])
	}
}

// Contract 8.2: request errors are precise and do not become warning reports.
func TestGetConfigWarningsValidatesEnvironmentParameter(t *testing.T) {
	tests := []struct {
		name, target, wantError string
	}{
		{name: "missing", target: "/api/config-warnings", wantError: "env parameter is required"},
		{name: "empty", target: "/api/config-warnings?env=", wantError: "env parameter is required"},
		{name: "unknown", target: "/api/config-warnings?env=prod", wantError: "unknown environment: prod"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, _ := getConfigWarningsForYAML(t, configWarningsTestYAML, tt.target)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", response.Code, response.Body.String())
			}
			var body map[string]string
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("error body is not JSON: %v; body: %s", err, response.Body.String())
			}
			if body["error"] != tt.wantError {
				t.Fatalf("error = %q, want %q", body["error"], tt.wantError)
			}
		})
	}
}

func TestGetConfigWarningsRejectsNonGETMethod(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dev.yaml"), []byte(configWarningsTestYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	response := httptest.NewRecorder()
	getConfigWarnings(response, httptest.NewRequest(http.MethodPost, "/api/config-warnings?env=dev", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405; body: %s", response.Code, response.Body.String())
	}
}

// Contract 8.2: generate refusals are data in a 200 response, not transport errors.
func TestGetConfigWarningsReportsConfigurationErrors(t *testing.T) {
	futureSchema := strings.Replace(configWarningsTestYAML, "schema_version: 32", "schema_version: 999999", 1)
	defaultVPCNAT := strings.Replace(configWarningsTestYAML, "use_default_vpc: false", "use_default_vpc: true", 1)
	defaultVPCNAT = strings.Replace(defaultVPCNAT, "egress_strategy: public_ip", "egress_strategy: nat_gateway", 1)
	forbiddenCIDR := strings.Replace(configWarningsTestYAML, "vpc_cidr: 10.20.0.0/16", "vpc_cidr: 172.16.0.0/16", 1)

	tests := []struct {
		name, yamlText string
	}{
		{name: "malformed YAML", yamlText: "project: [\n"},
		{name: "schema newer than binary", yamlText: futureSchema},
		{name: "NAT egress on default VPC", yamlText: defaultVPCNAT},
		{name: "forbidden VPC CIDR", yamlText: forbiddenCIDR},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, _ := getConfigWarningsForYAML(t, tt.yamlText, "/api/config-warnings?env=dev")
			report := decodeConfigWarningReport(t, response)
			if report.ConfigError == "" {
				t.Fatalf("configError is empty for refused YAML: %s", response.Body.String())
			}
			if report.Warnings == nil || len(report.Warnings) != 0 {
				t.Fatalf("pre-refusal warnings = %#v, want non-nil []", report.Warnings)
			}
		})
	}
}

// REQ-FR-4: a read-only GET must not migrate or otherwise rewrite YAML.
func TestGetConfigWarningsDoesNotModifyYAML(t *testing.T) {
	older := strings.Replace(configWarningsTestYAML, "schema_version: 32", "schema_version: 1", 1)
	response, envFile := getConfigWarningsForYAML(t, older, "/api/config-warnings?env=dev")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", response.Code, response.Body.String())
	}
	after, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != older {
		t.Fatalf("GET modified the YAML:\n--- before ---\n%s\n--- after ---\n%s", older, after)
	}
}

// Contract 8.1: declaration order must not leak into the API response.
func TestGetConfigWarningsOrdersWarningsByItem(t *testing.T) {
	yamlText := strings.Replace(configWarningsTestYAML, "services: []", `services:
  - name: zeta
    network_access: internal
  - name: alpha
    network_access: none`, 1)
	response, _ := getConfigWarningsForYAML(t, yamlText, "/api/config-warnings?env=dev")
	report := decodeConfigWarningReport(t, response)
	if len(report.Warnings) != 2 {
		t.Fatalf("got %d warnings, want 2: %#v", len(report.Warnings), report.Warnings)
	}
	if report.Warnings[0].Item.Name != "alpha" || report.Warnings[1].Item.Name != "zeta" {
		t.Fatalf("warning order = %q, %q; want alpha, zeta", report.Warnings[0].Item.Name, report.Warnings[1].Item.Name)
	}
}

// Contract 8.2/8.4: shell region drift is never part of this API report.
func TestGetConfigWarningsNeverIncludesRegionDrift(t *testing.T) {
	t.Setenv("AWS_REGION", "eu-west-1")
	response, _ := getConfigWarningsForYAML(t, configWarningsTestYAML, "/api/config-warnings?env=dev")
	report := decodeConfigWarningReport(t, response)
	for _, warning := range report.Warnings {
		if warning.Code == warnCodeRegionDrift {
			t.Fatalf("API report contains shell-specific region drift: %#v", report.Warnings)
		}
	}
}
