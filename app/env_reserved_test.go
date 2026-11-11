package main

import (
	"strings"
	"testing"
)

func reservedEnvMap(backendVars map[string]interface{}, services ...map[string]interface{}) map[string]interface{} {
	list := make([]interface{}, len(services))
	for i, s := range services {
		list[i] = s
	}
	m := map[string]interface{}{"project": "myapp", "env": "dev", "services": list}
	if backendVars != nil {
		m["workload"] = map[string]interface{}{"backend_env_variables": backendVars}
	}
	return m
}

func TestReservedEnv_AcceptsOtherNames(t *testing.T) {
	err := validateReservedEnvNamesConfigMap(reservedEnvMap(
		map[string]interface{}{"APP_ENV": "dev"},
		map[string]interface{}{"name": "bridge", "env_vars": map[string]interface{}{"EVENT_BUS": "local", "env": "x"}},
		map[string]interface{}{"name": "worker"},
	))
	if err != nil {
		t.Fatalf("config without ENV refused: %v", err)
	}
}

func TestReservedEnv_RefusesServiceEnvNamingServiceAndParameter(t *testing.T) {
	err := validateReservedEnvNamesConfigMap(reservedEnvMap(nil,
		map[string]interface{}{"name": "magento-bridge", "env_vars": map[string]interface{}{"ENV": "dev"}},
	))
	if err == nil {
		t.Fatal("ENV in a service's env_vars must be refused")
	}
	for _, want := range []string{"services 'magento-bridge'", "env_vars", "/dev/myapp/magento-bridge/env", "put-parameter"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestReservedEnv_RefusesBackendEnv(t *testing.T) {
	err := validateReservedEnvNamesConfigMap(reservedEnvMap(map[string]interface{}{"ENV": "dev"}))
	if err == nil {
		t.Fatal("ENV in backend_env_variables must be refused")
	}
	for _, want := range []string{"backend_env_variables", "/dev/myapp/backend/env"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestReservedEnv_RunsInPreprocess(t *testing.T) {
	err := preprocessEnvMap(reservedEnvMap(nil,
		map[string]interface{}{"name": "bridge", "env_vars": map[string]interface{}{"ENV": "dev"}},
	))
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("preprocessEnvMap must refuse ENV, got %v", err)
	}
}

func TestReservedEnv_RefusesTaskEnv(t *testing.T) {
	m := reservedEnvMap(nil)
	m["scheduled_tasks"] = []interface{}{map[string]interface{}{"name": "cleanup", "environment_variables": map[string]interface{}{"ENV": "dev"}}}
	m["event_processor_tasks"] = []interface{}{map[string]interface{}{"name": "sync", "environment_variables": map[string]interface{}{"ENV": "dev"}}}
	err := validateReservedEnvNamesConfigMap(m)
	if err == nil {
		t.Fatal("ENV in a task's environment_variables must be refused")
	}
	for _, want := range []string{"scheduled_tasks 'cleanup'", "/dev/myapp/task/cleanup/env", "event_processor_tasks 'sync'", "/dev/myapp/task/sync/env"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}
