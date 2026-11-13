package main

import (
	"strings"
	"testing"
)

func TestQACapabilityStorageKeepsCustomBackendBucketEnvironment(t *testing.T) {
	for _, kind := range []string{"service", "scheduled_task", "event_task"} {
		t.Run(kind, func(t *testing.T) {
			m := qaCapabilityEnv()
			field := qaCapabilityEnvField(kind)
			qaCapabilityOwner(m, kind)[field] = qaCapabilityMap{"AWS_S3_BUCKET": "myapp-custom"}
			err, warnings := qaCapabilityDiagnostics(t, m, func() error { return preprocessEnvMap(m) })
			qaCapabilityNoError(t, err)
			qaCapabilityContains(t, warnings, "AWS_S3_BUCKET", field, qaCapabilityOwnerName(kind))
			rendered := renderMainTemplate(t, m)
			body := qaCapabilityParse(t, rendered)
			bucket := qaCapabilityModule(t, body, "data_bucket")
			qaCapabilityWantJSON(t, qaCapabilityLiteral(t, bucket, "own_backend_bucket_env"), `["`+qaCapabilityOwnerName(kind)+`"]`)
			if kind == "service" {
				services := qaCapabilityLiteral(t, qaCapabilityModule(t, body, "workloads"), "services")
				qaCapabilityWantJSON(t, qaCapabilityNamed(t, services, "api")["env_vars"], `{"AWS_S3_BUCKET":"myapp-custom"}`)
			} else {
				module := "schedule_task_nightly"
				if kind == "event_task" {
					module = "event_bus_task_mailer"
				}
				vars := qaCapabilityLiteral(t, qaCapabilityModule(t, body, module), "custom_env_vars")
				qaCapabilityEqual(t, qaCapabilityNamed(t, vars, "AWS_S3_BUCKET")["value"], "myapp-custom")
			}
		})
	}
}

func TestQACapabilityStorageRendersSharedFoldersAndBucketOverrides(t *testing.T) {
	m := qaCapabilityEnv()
	m["data_bucket"] = qaCapabilityMap{"shared": qaCapabilityList(qaCapabilityMap{
		"name": "uploads", "read_write": []interface{}{"backend", "api"},
		"read": []interface{}{"nightly", "mailer"},
	})}
	rendered := renderMainHBS(t, m)
	body := qaCapabilityParse(t, rendered)
	bucket := qaCapabilityModule(t, body, "data_bucket")
	shared := qaCapabilityLiteral(t, bucket, "shared")
	qaCapabilityEqual(t, len(qaCapabilityArray(t, shared)), 1)
	folder := qaCapabilityNamed(t, shared, "uploads")
	qaCapabilityStringSet(t, folder["read_write"], "backend", "api")
	qaCapabilityStringSet(t, folder["read"], "nightly", "mailer")
	for _, input := range []string{"workloads", "backend_bucket_name", "backend_bucket_arn"} {
		qaCapabilityAttr(t, bucket, input)
	}
	for _, moduleName := range []string{"workloads", "schedule_task_nightly", "event_bus_task_mailer"} {
		module := qaCapabilityModule(t, body, moduleName)
		extra := qaCapabilityExprText(qaCapabilityAttr(t, module, "extra_env").Expr, rendered)
		if !strings.Contains(extra, "local.workload_extra_env") {
			t.Errorf("%s does not receive the workload environment extras: %s", moduleName, extra)
		}
	}
	t.Run("bucket is mandatory without data_bucket YAML", func(t *testing.T) {
		m := qaCapabilityEnv()
		delete(m, "data_bucket")
		body := qaCapabilityParse(t, renderMainHBS(t, m))
		qaCapabilityModule(t, body, "data_bucket")
	})
}
