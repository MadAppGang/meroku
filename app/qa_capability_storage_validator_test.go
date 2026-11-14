package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestQACapabilityNamesRejectDuplicatesAndReservedNames(t *testing.T) {
	keys := []string{"services", "scheduled_tasks", "event_processor_tasks"}
	for i, first := range keys {
		for _, second := range keys[i:] {
			t.Run(first+"/"+second, func(t *testing.T) {
				m := qaCapabilityEnv()
				m[first] = qaCapabilityList(qaCapabilityMap{"name": "duplicate"})
				if first == second {
					m[first] = append(m[first].([]interface{}), qaCapabilityMap{"name": "duplicate"})
				} else {
					m[second] = qaCapabilityList(qaCapabilityMap{"name": "duplicate"})
				}
				qaCapabilityRequireError(t, validateWorkloadNamesConfigMap(m), "duplicate")
			})
		}
		for _, name := range []string{"backend", "vault", "task", "shared"} {
			t.Run(first+"/"+name, func(t *testing.T) {
				m := qaCapabilityEnv()
				m[first] = qaCapabilityList(qaCapabilityMap{"name": name})
				qaCapabilityRequireError(t, validateWorkloadNamesConfigMap(m), name)
			})
		}
	}
}

func TestQACapabilityNamesAcceptDistinctUnrestrictedNames(t *testing.T) {
	m := qaCapabilityEnv()
	qaCapabilityOwner(m, "service")["name"] = "_ecs_instance"
	qaCapabilityOwner(m, "scheduled_task")["name"] = "Nightly.job"
	qaCapabilityOwner(m, "event_task")["name"] = "mail-worker"
	qaCapabilityNoError(t, validateWorkloadNamesConfigMap(m))
}

func TestQACapabilityStorageDropsDisabledReferencesAndRefusesUnknown(t *testing.T) {
	for _, access := range []string{"read", "read_write"} {
		for _, kind := range []string{"service", "scheduled_task", "event_task"} {
			t.Run(kind+" disabled/"+access, func(t *testing.T) {
				m := qaCapabilityEnv()
				qaCapabilityOwner(m, kind)["enabled"] = false
				m["data_bucket"] = qaCapabilityMap{"shared": qaCapabilityList(qaCapabilityMap{
					"name": "uploads", access: []interface{}{qaCapabilityOwnerName(kind), "backend"},
				})}
				err, warnings := qaCapabilityDiagnostics(t, m, func() error { return preprocessEnvMap(m) })
				qaCapabilityNoError(t, err)
				qaCapabilityContains(t, warnings, qaCapabilityOwnerName(kind))
				body := qaCapabilityParse(t, renderMainTemplate(t, m))
				shared := qaCapabilityLiteral(t, qaCapabilityModule(t, body, "data_bucket"), "shared")
				folder := qaCapabilityNamed(t, shared, "uploads")
				qaCapabilityWantJSON(t, folder[access], `["backend"]`)
			})
		}
		t.Run("unknown is an error/"+access, func(t *testing.T) {
			m := qaCapabilityEnv()
			m["data_bucket"] = qaCapabilityMap{"shared": qaCapabilityList(qaCapabilityMap{"name": "uploads", access: []interface{}{"missing"}})}
			qaCapabilityPrepareNames(t, m)
			qaCapabilityRequireError(t, validateDataBucketConfigMap(m), "missing", "data_bucket")
		})
	}
}

func TestQACapabilityStorageValidatesSharedFolders(t *testing.T) {
	for _, name := range []string{"a", "0", "a-b", strings.Repeat("a", 32)} {
		t.Run("accepts "+name, func(t *testing.T) {
			m := qaCapabilityEnv()
			m["data_bucket"] = qaCapabilityMap{"shared": qaCapabilityList(qaCapabilityMap{"name": name, "read": []interface{}{"backend"}})}
			qaCapabilityPrepareNames(t, m)
			qaCapabilityNoError(t, validateDataBucketConfigMap(m))
		})
	}
	for _, name := range []string{"", "-a", "a-", "UPPER", "a_b", "a/b", strings.Repeat("a", 33)} {
		t.Run("refuses "+name, func(t *testing.T) {
			m := qaCapabilityEnv()
			m["data_bucket"] = qaCapabilityMap{"shared": qaCapabilityList(qaCapabilityMap{"name": name})}
			qaCapabilityPrepareNames(t, m)
			qaCapabilityRequireError(t, validateDataBucketConfigMap(m), "name")
		})
	}
	for _, tt := range []struct {
		name   string
		shared []interface{}
	}{
		{"folder repeated", qaCapabilityList(qaCapabilityMap{"name": "uploads"}, qaCapabilityMap{"name": "uploads"})},
		{"reader repeated", qaCapabilityList(qaCapabilityMap{"name": "uploads", "read": []interface{}{"api", "api"}})},
		{"writer repeated", qaCapabilityList(qaCapabilityMap{"name": "uploads", "read_write": []interface{}{"api", "api"}})},
		{"reader also writer", qaCapabilityList(qaCapabilityMap{"name": "uploads", "read": []interface{}{"api"}, "read_write": []interface{}{"api"}})},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := qaCapabilityEnv()
			m["data_bucket"] = qaCapabilityMap{"shared": tt.shared}
			qaCapabilityPrepareNames(t, m)
			qaCapabilityRequireError(t, validateDataBucketConfigMap(m), "uploads")
		})
	}
}

func TestQACapabilityReservedEnvironmentIsConditional(t *testing.T) {
	for _, kind := range qaCapabilityKinds() {
		for _, variable := range []string{"DATA_BUCKET_NAME", "DATA_BUCKET_PATH", "DATA_BUCKET_SHARED_UPLOADS"} {
			t.Run(kind+"/"+variable, func(t *testing.T) {
				m := qaCapabilityEnv()
				field := qaCapabilityEnvField(kind)
				qaCapabilityOwner(m, kind)[field] = qaCapabilityMap{variable: "custom"}
				qaCapabilityRequireError(t, preprocessEnvMap(m), variable, field)
			})
		}
		for _, consuming := range []bool{false, true} {
			if consuming && (kind == "scheduled_task" || kind == "event_task") {
				continue
			}
			t.Run(fmt.Sprintf("%s/queue consumer %t", kind, consuming), func(t *testing.T) {
				m := qaCapabilityEnv()
				owner := qaCapabilityOwner(m, kind)
				field := qaCapabilityEnvField(kind)
				owner[field] = qaCapabilityMap{"EVENT_QUEUE_URL": "https://sqs.us-east-1.amazonaws.com/000000000000/custom"}
				if consuming {
					owner["events"] = qaCapabilityMap{"subscribes": qaCapabilityList(qaCapabilityMap{"detail_type": "order.created"})}
				}
				err := preprocessEnvMap(m)
				if consuming {
					qaCapabilityRequireError(t, err, "EVENT_QUEUE_URL", field)
				} else {
					qaCapabilityNoError(t, err)
				}
			})
		}
		for _, variable := range []string{"VALKEY_URL", "VALKEY_HOST", "VALKEY_PORT"} {
			for _, mode := range []string{"disabled", "unlisted", "listed"} {
				t.Run(kind+"/"+variable+"/"+mode, func(t *testing.T) {
					m := qaCapabilityEnv()
					field := qaCapabilityEnvField(kind)
					name := qaCapabilityOwnerName(kind)
					access := name
					if mode == "unlisted" {
						access = "backend"
						if kind == "backend" {
							access = "api"
						}
					}
					m["valkey"] = qaCapabilityMap{"enabled": mode != "disabled", "access": []interface{}{access}}
					qaCapabilityOwner(m, kind)[field] = qaCapabilityMap{variable: "custom"}
					err := preprocessEnvMap(m)
					if mode == "listed" {
						qaCapabilityRequireError(t, err, variable, field)
					} else {
						qaCapabilityNoError(t, err)
					}
				})
			}
		}
	}
	m := qaCapabilityEnv()
	qaCapabilityOwner(m, "backend")["backend_env_variables"] = qaCapabilityMap{"AWS_S3_BUCKET": "myapp-private"}
	qaCapabilityRequireError(t, preprocessEnvMap(m), "AWS_S3_BUCKET", "backend_env_variables")
}

func TestQACapabilityCustomTerraformRejectsReservedRootNames(t *testing.T) {
	tests := []struct{ declaration, name string }{
		{`module "data_bucket" { source = "./unused" }`, "data_bucket"},
		{`module "vaults" { source = "./unused" }`, "vaults"},
		{`module "events" { source = "./unused" }`, "events"},
		{`module "valkey" { source = "./unused" }`, "valkey"},
		{`locals { workload_roles = {} }`, "workload_roles"},
		{`locals { workload_extra_env = {} }`, "workload_extra_env"},
		{`locals { workload_extra_secrets = {} }`, "workload_extra_secrets"},
	}
	for _, scope := range []string{"_shared", "dev"} {
		for _, tt := range tests {
			t.Run(scope+"/"+tt.name, func(t *testing.T) {
				t.Chdir(t.TempDir())
				filename := filepath.Join("custom", "terraform", scope, "settings.tf")
				source := "# synthetic custom settings\n" + tt.declaration + "\n"
				uses := reservedRootNameUses(filename, []byte(source))
				qaCapabilityContains(t, strings.Join(uses, "\n"), filename+":2", tt.name)
				qaCapabilityWrite(t, filename, source)
				qaCapabilityRequireError(t, validateCustomTerraformRootNames("dev"), filename, tt.name, ":2")
			})
		}
	}
	t.Run("unrelated roots and nested attributes are allowed", func(t *testing.T) {
		t.Chdir(t.TempDir())
		filename := filepath.Join("custom", "terraform", "dev", "settings.tf")
		source := "module \"my_cache\" {\n source = \"./cache\"\n settings = { workload_roles = {} }\n}\nlocals { my_events = {} }\n"
		qaCapabilityEqual(t, len(reservedRootNameUses(filename, []byte(source))), 0)
		qaCapabilityWrite(t, filename, source)
		qaCapabilityNoError(t, validateCustomTerraformRootNames("dev"))
	})
}
