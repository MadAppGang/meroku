package main

import (
	"fmt"
	"sort"
	"strings"
)

// ReservedEnvName is the container variable meroku feeds from SSM itself.
//
// modules/workloads/env_services.tf and env.tf, modules/ecs_task/env.tf and
// modules/event_bridge_task/env.tf create one SecureString per workload,
// /<env>/<project>/<service>/env (.../backend/env, .../task/<task>/env), with
// ignore_changes on its value. Every parameter under the workload's path becomes
// a secret named after its upper-cased last segment, so that parameter always
// reaches the container as ENV. Its value is meant to be set in SSM and picked
// up by the next task start, without a new task-definition revision.
//
// Declaring ENV in YAML as well puts the name in both `environment` and
// `secrets`, which ECS refuses at RegisterTaskDefinition. Deleting the parameter
// does not help: Terraform creates it again on the next apply.
const ReservedEnvName = "ENV"

// validateReservedEnvNamesConfigMap refuses ENV in services[].env_vars,
// workload.backend_env_variables and the environment_variables of
// scheduled_tasks and event_processor_tasks at generate time, naming the
// workload and the SSM parameter that carries the value instead.
//
// The plan-time precondition in modules/workloads/env_secret_check.tf would
// catch the same collision, but only after generation and init, and only for a
// workload whose SSM path already exists.
func validateReservedEnvNamesConfigMap(envMap map[string]interface{}) error {
	project, _ := envMap["project"].(string)
	env, _ := envMap["env"].(string)
	ssmPath := func(workload string) string {
		return fmt.Sprintf("/%s/%s/%s/env", env, project, workload)
	}

	var problems []string

	if workload, ok := envMap["workload"].(map[string]interface{}); ok {
		if declaresReservedEnv(workload["backend_env_variables"]) {
			problems = append(problems, fmt.Sprintf(
				"workload.backend_env_variables sets %s; remove it and put the value in the SSM parameter %s",
				ReservedEnvName, ssmPath("backend")))
		}
	}

	lists := []struct {
		key, field, pathPrefix string
	}{
		{"services", "env_vars", ""},
		{"scheduled_tasks", "environment_variables", "task/"},
		{"event_processor_tasks", "environment_variables", "task/"},
	}
	for _, list := range lists {
		items, ok := envMap[list.key].([]interface{})
		if !ok {
			continue
		}
		for i, raw := range items {
			item, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := item["name"].(string)
			if name == "" {
				name = fmt.Sprintf("#%d", i+1)
			}
			if declaresReservedEnv(item[list.field]) {
				problems = append(problems, fmt.Sprintf(
					"%s '%s': %s sets %s; remove it and put the value in the SSM parameter %s",
					list.key, name, list.field, ReservedEnvName, ssmPath(list.pathPrefix+name)))
			}
		}
	}

	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf(
		"%s is reserved: meroku creates an SSM parameter named env for every workload and passes it to the container as the secret %s, so ECS would refuse a task definition that also sets it as a plain variable.\n"+
			"Set the value with `aws ssm put-parameter --name <parameter> --value <value> --overwrite`; running tasks pick it up when they restart, with no new task definition.\n  %s",
		ReservedEnvName, ReservedEnvName, strings.Join(problems, "\n  "))
}

// declaresReservedEnv reports whether an env-var map from the YAML has the key
// ENV. Names are compared exactly, as ECS compares them: `env` is a different
// variable from `ENV` inside the container.
func declaresReservedEnv(raw interface{}) bool {
	switch vars := raw.(type) {
	case map[string]interface{}:
		_, ok := vars[ReservedEnvName]
		return ok
	case map[string]string:
		_, ok := vars[ReservedEnvName]
		return ok
	}
	return false
}
