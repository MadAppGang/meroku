package main

import (
	"fmt"
	"strings"
)

// validateNetworkAccessConfigMap checks services[].network_access at generate
// time, on the JSON-compatible map applyTemplate renders from.
//
// Terraform validates the same values (modules/workloads/variables.tf), but
// only at plan, after generation, and its message points at a variable rather
// than at the service. Refusing here names the service and the file.
//
// It also warns — never refuses — when an internal or none service has no
// health_check_command. Those services have no ALB target group, and the ALB's
// health check was the only one meroku ever configured, so ECS would only
// replace such a task when its process exits. meroku does not invent a check:
// ECS runs it inside the container, and a curl check on an image without curl
// fails on every probe and restarts the task in a loop.
func validateNetworkAccessConfigMap(envMap map[string]interface{}) error {
	services, ok := envMap["services"].([]interface{})
	if !ok {
		return nil
	}

	var problems []string
	for i, raw := range services {
		service, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := service["name"].(string)
		if name == "" {
			name = fmt.Sprintf("#%d", i+1)
		}

		// Runs after normalizeHealthCheckCommands, so a string is already a list.
		if problem := validateHealthCheckCommand(name, service["health_check_command"]); problem != "" {
			problems = append(problems, problem)
		}

		accessRaw, present := service["network_access"]
		if !present || accessRaw == nil {
			continue // absent renders as public
		}
		access, isString := accessRaw.(string)
		if !isString || !isNetworkAccessValue(access) {
			problems = append(problems, fmt.Sprintf(
				"service '%s': network_access is %v; it must be one of: %s",
				name, accessRaw, strings.Join(NetworkAccessValues, ", ")))
			continue
		}

		if access == NetworkAccessInternal || access == NetworkAccessNone {
			if cmd, _ := service["health_check_command"].([]interface{}); len(cmd) == 0 {
				fmt.Printf("⚠️  service '%s' is network_access: %s and has no health_check_command.\n", name, access)
				fmt.Println("   It is not behind the ALB, so nothing health-checks it: ECS replaces a task only")
				fmt.Println("   when its process exits. Add a command that works in the image, e.g.")
				fmt.Println(`   health_check_command: "wget -qO- http://localhost:<port>/health/live || exit 1"`)
			}
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid service network settings:\n  %s", strings.Join(problems, "\n  "))
	}
	return nil
}

func isNetworkAccessValue(v string) bool {
	for _, allowed := range NetworkAccessValues {
		if v == allowed {
			return true
		}
	}
	return false
}
