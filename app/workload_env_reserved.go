package main

import (
	"fmt"
	"sort"
	"strings"
)

// Container variables meroku's capability modules set on a workload.
//
// They reach the container through extra_env (env/main.hbs local.workload_extra_env),
// after the workload's own variables. A YAML variable of the same name would put the
// name in `environment` twice, and the container would see whichever ECS keeps; so
// generate refuses it and names the YAML field. A name is reserved for a workload
// only when meroku sets it for that workload.
//
// AWS_S3_BUCKET is reserved for the backend, which has always had it from
// modules/workloads/env.tf (local.backend_env) rather than from extra_env; a YAML
// value would put it in the backend's `environment` twice. On services and tasks it
// is the one exception: they never had it, so a YAML AWS_S3_BUCKET there most likely
// names a bucket of the workload's own. Its value is kept, meroku does not add the
// backend bucket, and generate warns.
//
// This joins the ENV rule in app/env_reserved.go.

// workloadEnvContext is what a reserved-name rule needs to know about a workload.
type workloadEnvContext struct {
	envMap map[string]interface{}
	name   string // the workload name; "backend" for the backend
}

// reservedWorkloadEnv is one row of the table.
type reservedWorkloadEnv struct {
	// name is the variable name, or its prefix when prefix is true.
	name   string
	prefix bool
	// setBy names what sets it, for the error message.
	setBy string
	// appliesTo reports whether meroku sets the name for this workload.
	appliesTo func(ctx workloadEnvContext) bool
}

func everyWorkload(workloadEnvContext) bool { return true }

func isBackend(ctx workloadEnvContext) bool { return ctx.name == backendWorkloadName }

// reservedWorkloadEnvTable lists every name meroku sets through extra_env, and the
// backend's AWS_S3_BUCKET.
var reservedWorkloadEnvTable = []reservedWorkloadEnv{
	{name: backendBucketEnvName, setBy: "the backend bucket", appliesTo: isBackend},
	{name: "DATA_BUCKET_NAME", setBy: "the data bucket", appliesTo: everyWorkload},
	{name: "DATA_BUCKET_PATH", setBy: "the data bucket", appliesTo: everyWorkload},
	{name: "DATA_BUCKET_SHARED_", prefix: true, setBy: "the data bucket's shared folders", appliesTo: everyWorkload},
	{name: "EVENT_QUEUE_URL", setBy: "the workload's event queue", appliesTo: isEventConsumer},
	{name: "VALKEY_URL", setBy: "valkey.access", appliesTo: hasValkeyAccess},
	{name: "VALKEY_HOST", setBy: "valkey.access", appliesTo: hasValkeyAccess},
	{name: "VALKEY_PORT", setBy: "valkey.access", appliesTo: hasValkeyAccess},
}

// backendBucketEnvName is the variable that names the backend bucket.
const backendBucketEnvName = "AWS_S3_BUCKET"

// ownBackendBucketEnvKey is the envMap key listing the services and tasks whose
// YAML sets AWS_S3_BUCKET; modules/data_bucket adds none for them.
const ownBackendBucketEnvKey = "_data_bucket_own_backend_bucket_env"

func (r reservedWorkloadEnv) matches(variable string) bool {
	if r.prefix {
		return strings.HasPrefix(variable, r.name)
	}
	return variable == r.name
}

func (r reservedWorkloadEnv) display() string {
	if r.prefix {
		return r.name + "*"
	}
	return r.name
}

// isEventConsumer reports whether the workload is listed as an event consumer.
// The events normaliser writes _event_consumers; until a workload subscribes to
// something it is absent and no workload is a consumer.
func isEventConsumer(ctx workloadEnvContext) bool {
	switch consumers := ctx.envMap["_event_consumers"].(type) {
	case map[string]interface{}:
		_, ok := consumers[ctx.name]
		return ok
	case []interface{}:
		for _, raw := range consumers {
			if s, ok := raw.(string); ok && s == ctx.name {
				return true
			}
			if m, ok := computeNodeMap(raw); ok && stringFromMap(m, "name") == ctx.name {
				return true
			}
		}
	}
	return false
}

// hasValkeyAccess reports whether the workload is in valkey.access of an enabled
// valkey block.
func hasValkeyAccess(ctx workloadEnvContext) bool {
	valkey, ok := yamlSubMap(ctx.envMap, "valkey")
	if !ok {
		return false
	}
	if enabled, _ := valkey["enabled"].(bool); !enabled {
		return false
	}
	access, _ := valkey["access"].([]interface{})
	for _, raw := range access {
		if s, ok := raw.(string); ok && s == ctx.name {
			return true
		}
	}
	return false
}

// workloadEnvSource is one workload's YAML env-var map and where it lives.
type workloadEnvSource struct {
	name  string // workload name
	field string // YAML path of the map, for messages
	kind  string // "backend", "service" or "task"
	vars  []string
}

// workloadEnvSources lists every workload's YAML env-var names.
func workloadEnvSources(envMap map[string]interface{}) []workloadEnvSource {
	var out []workloadEnvSource
	if workload, ok := yamlSubMap(envMap, "workload"); ok {
		out = append(out, workloadEnvSource{
			name: backendWorkloadName, field: "workload.backend_env_variables", kind: "backend",
			vars: envVarNames(workload["backend_env_variables"]),
		})
	} else {
		out = append(out, workloadEnvSource{name: backendWorkloadName, field: "workload.backend_env_variables", kind: "backend"})
	}

	lists := []struct{ key, field, kind string }{
		{"services", "env_vars", "service"},
		{"scheduled_tasks", "environment_variables", "task"},
		{"event_processor_tasks", "environment_variables", "task"},
	}
	for _, list := range lists {
		items, _ := envMap[list.key].([]interface{})
		for i, raw := range items {
			item, ok := computeNodeMap(raw)
			if !ok {
				continue
			}
			name := stringFromMap(item, "name")
			if name == "" {
				continue
			}
			out = append(out, workloadEnvSource{
				name:  name,
				field: fmt.Sprintf("%s[%d].%s ('%s')", list.key, i, list.field, name),
				kind:  list.kind,
				vars:  envVarNames(item[list.field]),
			})
		}
	}
	return out
}

// envVarNames returns the keys of a YAML env-var map, sorted.
func envVarNames(raw interface{}) []string {
	var names []string
	switch vars := raw.(type) {
	case map[string]interface{}:
		for k := range vars {
			names = append(names, k)
		}
	case map[string]string:
		for k := range vars {
			names = append(names, k)
		}
	case map[interface{}]interface{}:
		for k := range vars {
			names = append(names, fmt.Sprintf("%v", k))
		}
	}
	sort.Strings(names)
	return names
}

// validateWorkloadReservedEnvConfigMap refuses a YAML env var that meroku now sets
// for the same workload, and records the services and tasks that keep their own
// AWS_S3_BUCKET under ownBackendBucketEnvKey, with a warning for each.
func validateWorkloadReservedEnvConfigMap(envMap map[string]interface{}) error {
	var problems []string
	var ownBucket []interface{}

	for _, src := range workloadEnvSources(envMap) {
		ctx := workloadEnvContext{envMap: envMap, name: src.name}
		for _, v := range src.vars {
			if v == backendBucketEnvName && src.kind != "backend" {
				ownBucket = append(ownBucket, src.name)
				addGenerateWarning(envMap, fmt.Sprintf(
					"%s sets %s. Its value is kept and meroku does not add its own; for every other workload %s now names the backend bucket, which this workload can also read and write.",
					src.field, backendBucketEnvName, backendBucketEnvName))
				continue
			}
			for _, row := range reservedWorkloadEnvTable {
				if row.matches(v) && row.appliesTo(ctx) {
					problems = append(problems, fmt.Sprintf(
						"%s sets %s, which meroku sets for this workload (%s: %s); remove it from the YAML and read meroku's value",
						src.field, v, row.display(), row.setBy))
					break
				}
			}
		}
	}

	envMap[ownBackendBucketEnvKey] = ownBucket

	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("environment variables reserved by meroku:\n  %s", strings.Join(problems, "\n  "))
}
