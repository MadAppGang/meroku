package main

import (
	"testing"

	"gopkg.in/yaml.v2"
)

func TestFilterDisabledItems(t *testing.T) {
	yamlContent := `
scheduled_tasks:
- name: task1
  enabled: true
  schedule: "rate(1 hours)"
- name: task2
  enabled: false
  schedule: "rate(2 hours)"
- name: task3
  schedule: "rate(3 hours)"
`
	var e map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &e); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	converted := convertToJSONCompatible(e)
	cm := converted.(map[string]interface{})

	filterDisabledItems(cm, "scheduled_tasks")

	remaining := cm["scheduled_tasks"].([]interface{})
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining tasks, got %d", len(remaining))
	}

	names := make([]string, 0, len(remaining))
	for _, task := range remaining {
		tm := task.(map[string]interface{})
		names = append(names, tm["name"].(string))
	}

	if names[0] != "task1" || names[1] != "task3" {
		t.Errorf("expected [task1, task3], got %v", names)
	}
}

func TestFilterDisabledServices(t *testing.T) {
	yamlContent := `
services:
- name: svc1
  enabled: true
  container_port: 3000
- name: svc2
  enabled: false
  container_port: 8080
- name: svc3
  container_port: 9090
`
	var e map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &e); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	converted := convertToJSONCompatible(e)
	cm := converted.(map[string]interface{})

	filterDisabledItems(cm, "services")

	remaining := cm["services"].([]interface{})
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining services, got %d", len(remaining))
	}

	names := make([]string, 0, len(remaining))
	for _, svc := range remaining {
		sm := svc.(map[string]interface{})
		names = append(names, sm["name"].(string))
	}

	if names[0] != "svc1" || names[1] != "svc3" {
		t.Errorf("expected [svc1, svc3], got %v", names)
	}
}

func TestFilterDisabledEventProcessorTasks(t *testing.T) {
	yamlContent := `
event_processor_tasks:
- name: evt1
  enabled: false
- name: evt2
  enabled: true
`
	var e map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &e); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	converted := convertToJSONCompatible(e)
	cm := converted.(map[string]interface{})

	filterDisabledItems(cm, "event_processor_tasks")

	remaining := cm["event_processor_tasks"].([]interface{})
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining event task, got %d", len(remaining))
	}

	tm := remaining[0].(map[string]interface{})
	if tm["name"] != "evt2" {
		t.Errorf("expected evt2, got %v", tm["name"])
	}
}
