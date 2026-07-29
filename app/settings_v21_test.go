package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestMigrateToV21NormalizesScheduledCommands(t *testing.T) {
	data := map[string]interface{}{
		"scheduled_tasks": []interface{}{
			map[interface{}]interface{}{
				"name":              "json-command",
				"container_command": `["bun","run","job.ts"]`,
			},
			map[interface{}]interface{}{
				"name":              "scalar-command",
				"container_command": "bin/report --daily",
			},
			map[interface{}]interface{}{
				"name":              "list-command",
				"container_command": []interface{}{"sh", "-c", "echo ready"},
			},
		},
	}

	if err := migrateToV21(data); err != nil {
		t.Fatalf("migrateToV21: %v", err)
	}

	tasks := data["scheduled_tasks"].([]interface{})
	jsonCommand := tasks[0].(map[interface{}]interface{})["container_command"]
	if !reflect.DeepEqual(
		jsonCommand,
		[]interface{}{"bun", "run", "job.ts"},
	) {
		t.Errorf("JSON command = %#v", jsonCommand)
	}

	scalarCommand := tasks[1].(map[interface{}]interface{})["container_command"]
	if !reflect.DeepEqual(scalarCommand, []interface{}{"bin/report --daily"}) {
		t.Errorf("scalar command = %#v", scalarCommand)
	}

	listCommand := tasks[2].(map[interface{}]interface{})["container_command"]
	if !reflect.DeepEqual(
		listCommand,
		[]interface{}{"sh", "-c", "echo ready"},
	) {
		t.Errorf("existing list command changed = %#v", listCommand)
	}
}

func TestSettingsV21RenderToTerraform(t *testing.T) {
	config := strings.Replace(
		albBaseConfig,
		"scheduled_tasks: []",
		`scheduled_tasks:
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
      REPORT_MODE: daily`,
		1,
	)
	config = strings.Replace(
		config,
		"event_processor_tasks: []",
		`event_processor_tasks:
  - name: events
    rule_name: events
    sources:
      - caremaster
    detail_types:
      - updated
    container_command:
      - bun
      - run
      - events.ts
    environment_variables:
      LOG_LEVEL: debug`,
		1,
	)
	config += `manage_dns_records: true
amplify_apps:
  - name: portal
    github_repository: https://github.com/example/portal
    branches:
      - name: main
        stage: PRODUCTION
`

	output := renderMainTemplate(t, config)
	for _, expected := range []string{
		`schedule_expression_timezone = "Australia/Sydney"`,
		"max_retry_attempts = 7",
		`dlq_arn = "arn:aws:sqs:ap-southeast-2:123456789012:report-dlq"`,
		`{ "name" : "REPORT_MODE", "value" : "daily" }`,
		`{ "name" : "LOG_LEVEL", "value" : "debug" }`,
		"manage_dns_records = true",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("rendered Terraform does not contain %q", expected)
		}
	}

	if strings.Contains(output, "[bun run report.ts]") {
		t.Error("scheduled command was rendered as a Go-style scalar")
	}
	if !strings.Contains(output, `"bun"`) ||
		!strings.Contains(output, `"report.ts"`) ||
		!strings.Contains(output, `"events.ts"`) {
		t.Error("container command arrays were not rendered as quoted Terraform values")
	}
}
