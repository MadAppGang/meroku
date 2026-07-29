package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"gopkg.in/yaml.v2"
)

func TestEnvironmentConfigAPISettingsRoundTrip(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("change to temporary directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	}()

	initialYAML := `schema_version: 21
project: api-round-trip
env: dev
manage_dns_records: false
scheduled_tasks:
  - name: daily-report
    schedule: cron(0 8 * * ? *)
    timezone: UTC
    container_command:
      - bun
      - run
      - report.ts
    max_retry_attempts: 3
event_processor_tasks:
  - name: events
    environment_variables:
      LOG_LEVEL: info
`
	if err := os.WriteFile("dev.yaml", []byte(initialYAML), 0o600); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/api/environment?name=dev", nil)
	getResponse := httptest.NewRecorder()
	getEnvironmentConfig(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", getResponse.Code, http.StatusOK)
	}

	var loaded ConfigResponse
	if err := json.NewDecoder(getResponse.Body).Decode(&loaded); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if loaded.Content != initialYAML {
		t.Fatalf("GET content changed:\n%s", loaded.Content)
	}

	updatedYAML := `schema_version: 21
project: api-round-trip
env: dev
manage_dns_records: true
scheduled_tasks:
  - name: daily-report
    schedule: cron(0 8 * * ? *)
    timezone: Australia/Sydney
    container_command:
      - bun
      - run
      - report.ts
    max_retry_attempts: 7
    dlq_arn: arn:aws:sqs:ap-southeast-2:123456789012:report-dlq
    environment_variables:
      REPORT_MODE: daily
event_processor_tasks:
  - name: events
    container_command:
      - bun
      - run
      - events.ts
    environment_variables:
      LOG_LEVEL: debug
`
	requestBody, err := json.Marshal(map[string]string{"content": updatedYAML})
	if err != nil {
		t.Fatalf("encode update request: %v", err)
	}
	updateRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/environment/update?name=dev",
		bytes.NewReader(requestBody),
	)
	updateResponse := httptest.NewRecorder()
	updateEnvironmentConfig(updateResponse, updateRequest)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf(
			"POST status = %d, want %d: %s",
			updateResponse.Code,
			http.StatusOK,
			updateResponse.Body.String(),
		)
	}

	saved, err := os.ReadFile("dev.yaml")
	if err != nil {
		t.Fatalf("read updated config: %v", err)
	}
	if string(saved) != updatedYAML {
		t.Fatalf("POST content changed:\n%s", saved)
	}

	var decoded Env
	if err := yaml.Unmarshal(saved, &decoded); err != nil {
		t.Fatalf("decode updated config through the application model: %v", err)
	}
	if decoded.ManageDNSRecords == nil || !*decoded.ManageDNSRecords {
		t.Error("manage_dns_records was not decoded")
	}
	if len(decoded.ScheduledTasks) != 1 {
		t.Fatalf("scheduled task count = %d, want 1", len(decoded.ScheduledTasks))
	}
	scheduledTask := decoded.ScheduledTasks[0]
	if scheduledTask.Timezone != "Australia/Sydney" {
		t.Errorf("timezone = %q, want Australia/Sydney", scheduledTask.Timezone)
	}
	if scheduledTask.MaxRetryAttempts == nil || *scheduledTask.MaxRetryAttempts != 7 {
		t.Errorf("max_retry_attempts = %v, want 7", scheduledTask.MaxRetryAttempts)
	}
	if scheduledTask.DLQArn == "" || len(scheduledTask.ContainerCommand) != 3 {
		t.Errorf("scheduled task settings were not decoded: %+v", scheduledTask)
	}
	if scheduledTask.EnvVariables["REPORT_MODE"] != "daily" {
		t.Errorf("scheduled environment variables = %v", scheduledTask.EnvVariables)
	}
	if decoded.EventProcessorTasks[0].EnvVariables["LOG_LEVEL"] != "debug" {
		t.Errorf(
			"event environment variables = %v",
			decoded.EventProcessorTasks[0].EnvVariables,
		)
	}

	readbackRequest := httptest.NewRequest(
		http.MethodGet,
		"/api/environment?name=dev",
		nil,
	)
	readbackResponse := httptest.NewRecorder()
	getEnvironmentConfig(readbackResponse, readbackRequest)

	var readback ConfigResponse
	if err := json.NewDecoder(readbackResponse.Body).Decode(&readback); err != nil {
		t.Fatalf("decode readback response: %v", err)
	}
	if readback.Content != updatedYAML {
		t.Fatalf("readback content changed:\n%s", readback.Content)
	}
}
