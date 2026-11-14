package main

import (
	"fmt"
	"testing"
)

func TestQACapabilityValkeyValidatesAccessVersionAndStorage(t *testing.T) {
	for _, version := range []interface{}{"7", "8", "9", 7, 8, 9} {
		t.Run(fmt.Sprintf("version %v", version), func(t *testing.T) {
			m := qaCapabilityEnv()
			m["valkey"] = qaCapabilityMap{"enabled": true, "engine_version": version, "max_storage_gb": 1, "access": []interface{}{"backend", "api", "nightly", "mailer"}}
			qaCapabilityPrepareNames(t, m)
			qaCapabilityNoError(t, validateValkeyConfigMap(m))
		})
	}
	for _, tt := range []struct {
		name, field string
		value       interface{}
	}{
		{"old version", "engine_version", "6"}, {"future version", "engine_version", "10"},
		{"minor version", "engine_version", "8.0"}, {"fractional version", "engine_version", 8.5},
		{"boolean version", "engine_version", true},
		{"zero cap", "max_storage_gb", 0}, {"negative cap", "max_storage_gb", -1},
		{"fractional cap", "max_storage_gb", 1.5}, {"boolean cap", "max_storage_gb", true},
		{"unknown workload", "access", []interface{}{"missing"}},
		{"duplicate workload", "access", []interface{}{"api", "api"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := qaCapabilityEnv()
			cache := qaCapabilityMap{"enabled": true, "access": []interface{}{"api"}}
			cache[tt.field] = tt.value
			m["valkey"] = cache
			qaCapabilityPrepareNames(t, m)
			qaCapabilityRequireError(t, validateValkeyConfigMap(m), tt.field)
		})
	}
	t.Run("enabled without access warns", func(t *testing.T) {
		m := qaCapabilityEnv()
		m["valkey"] = qaCapabilityMap{"enabled": true}
		qaCapabilityPrepareNames(t, m)
		err, warnings := qaCapabilityDiagnostics(t, m, func() error { return validateValkeyConfigMap(m) })
		qaCapabilityNoError(t, err)
		qaCapabilityContains(t, warnings, "valkey", "access")
	})
	t.Run("defaults are valid", func(t *testing.T) {
		m := qaCapabilityEnv()
		m["valkey"] = qaCapabilityMap{"enabled": true, "access": []interface{}{"api"}}
		qaCapabilityPrepareNames(t, m)
		qaCapabilityNoError(t, validateValkeyConfigMap(m))
	})
}

func TestQACapabilityValkeyDropsDisabledAccess(t *testing.T) {
	for _, kind := range []string{"service", "scheduled_task", "event_task"} {
		t.Run(kind, func(t *testing.T) {
			m := qaCapabilityEnv()
			qaCapabilityOwner(m, kind)["enabled"] = false
			name := qaCapabilityOwnerName(kind)
			m["valkey"] = qaCapabilityMap{"enabled": true, "access": []interface{}{name, "backend"}}
			err, warnings := qaCapabilityDiagnostics(t, m, func() error { return preprocessEnvMap(m) })
			qaCapabilityNoError(t, err)
			qaCapabilityContains(t, warnings, name)
			body := qaCapabilityParse(t, renderMainTemplate(t, m))
			cache := qaCapabilityModule(t, body, "valkey")
			qaCapabilityWantJSON(t, qaCapabilityLiteral(t, cache, "access"), `["backend"]`)
		})
	}
}
