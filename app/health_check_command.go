package main

import (
	"fmt"
	"math"
	"strings"
)

// HealthCheckCommand is services[].health_check_command, in either of two forms:
//
//	health_check_command: "curl -f http://localhost:3000/health/live || exit 1"
//	health_check_command: ["CMD", "/app/healthcheck"]
//
// A string is a shell command and becomes ["CMD-SHELL", "<string>"], the ECS
// form that runs it through /bin/sh -c. A list is the raw ECS form and is passed
// through, which is the only way to health-check an image with no shell
// (distroless, scratch): ["CMD", "<binary>", "<args>"...].
//
// The form the user wrote is remembered so a config that meroku loads and saves
// again (the web API handlers round-trip the Env struct) keeps the user's form.
type HealthCheckCommand struct {
	Command    []string // ECS form, always a list
	fromString bool
}

// UnmarshalYAML accepts a string or a list of strings (yaml.v2 interface).
func (h *HealthCheckCommand) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		*h = HealthCheckCommand{Command: shellHealthCheck(s), fromString: true}
		return nil
	}
	var list []string
	if err := unmarshal(&list); err != nil {
		return fmt.Errorf("health_check_command must be a string or a list of strings")
	}
	*h = HealthCheckCommand{Command: list}
	return nil
}

// MarshalYAML writes back the form that was read.
func (h HealthCheckCommand) MarshalYAML() (interface{}, error) {
	if h.fromString && len(h.Command) == 2 {
		return h.Command[1], nil
	}
	return h.Command, nil
}

// IsZero lets `omitempty` drop an unset command (yaml.v2 IsZeroer).
func (h HealthCheckCommand) IsZero() bool { return len(h.Command) == 0 }

// shellHealthCheck turns a shell command string into the ECS list form. An
// empty string is no health check.
func shellHealthCheck(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return []string{"CMD-SHELL", s}
}

// normalizeHealthCheckCommands rewrites each service's health_check_command to
// the ECS list form in the map applyTemplate renders from, so the Terraform
// module only ever receives list(string). It writes to the in-memory map only,
// never to the user's YAML.
//
// It does the same for workload.backend_health_check.command. An empty string
// there is left in place rather than deleted, so validateBackendHealthConfigMap
// can refuse a backend_health_check that has no command.
func normalizeHealthCheckCommands(envMap map[string]interface{}) {
	if workload, ok := envMap["workload"].(map[string]interface{}); ok {
		if check, ok := workload["backend_health_check"].(map[string]interface{}); ok {
			if s, isString := check["command"].(string); isString {
				if cmd := shellHealthCheck(s); cmd != nil {
					check["command"] = []interface{}{cmd[0], cmd[1]}
				}
			}
		}
	}

	services, ok := envMap["services"].([]interface{})
	if !ok {
		return
	}
	for _, raw := range services {
		service, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if s, isString := service["health_check_command"].(string); isString {
			if cmd := shellHealthCheck(s); cmd != nil {
				service["health_check_command"] = []interface{}{cmd[0], cmd[1]}
			} else {
				delete(service, "health_check_command")
			}
		}
	}
}

// validateHealthCheckCommand checks a service's health_check_command in the
// list form. ECS only accepts a command whose first element is CMD or
// CMD-SHELL, and rejects anything else when the task definition is registered
// — partway through an apply, naming no service.
func validateHealthCheckCommand(name string, raw interface{}) string {
	return healthCheckCommandProblem(fmt.Sprintf("service '%s'", name), "health_check_command", raw)
}

// healthCheckCommandProblem checks a health check command in the list form.
// subject names the owner in the message ("service 'api'", "workload") and key
// is the YAML key the command was written under.
func healthCheckCommandProblem(subject, key string, raw interface{}) string {
	if raw == nil {
		return ""
	}
	list, ok := raw.([]interface{})
	if !ok {
		return fmt.Sprintf("%s: %s must be a string or a list of strings", subject, key)
	}
	if len(list) == 0 {
		return ""
	}
	first, _ := list[0].(string)
	switch {
	case first == "CMD-SHELL" && len(list) != 2:
		return fmt.Sprintf(`%s: %s ["CMD-SHELL", ...] takes exactly one command string; write it as a plain string instead`, subject, key)
	case first == "CMD" && len(list) < 2:
		return fmt.Sprintf(`%s: %s ["CMD"] needs the program to run, e.g. ["CMD", "/app/healthcheck"]`, subject, key)
	case first != "CMD" && first != "CMD-SHELL":
		return fmt.Sprintf(`%s: a %s list must start with "CMD" (run a program, no shell) or "CMD-SHELL"; for a shell command, write it as a plain string`, subject, key)
	}
	return ""
}

// backendHealthCheckRanges are the ECS limits for a container health check,
// in seconds except retries. ECS refuses a value outside them when the task
// definition is registered, partway through an apply.
var backendHealthCheckRanges = []struct {
	key      string
	min, max int
}{
	{"interval", 5, 300},
	{"timeout", 2, 120},
	{"retries", 1, 10},
	{"start_period", 0, 300},
}

// validateBackendHealthConfigMap checks workload.backend_health_check and
// workload.backend_circuit_breaker at generate time, on the map applyTemplate
// renders from. It runs after normalizeHealthCheckCommands, so a string
// command is already a list.
//
// A backend_health_check with no command is refused rather than ignored:
// writing the block says the operator wants a check, and rendering none would
// hide that.
func validateBackendHealthConfigMap(envMap map[string]interface{}) error {
	workload, ok := envMap["workload"].(map[string]interface{})
	if !ok {
		return nil
	}

	var problems []string

	if raw, present := workload["backend_health_check"]; present && raw != nil {
		check, ok := raw.(map[string]interface{})
		if !ok {
			problems = append(problems, "workload: backend_health_check must be a map with a command")
		} else {
			problems = append(problems, backendHealthCheckProblems(check)...)
		}
	}

	if raw, present := workload["backend_circuit_breaker"]; present && raw != nil {
		breaker, ok := raw.(map[string]interface{})
		if !ok {
			problems = append(problems, "workload: backend_circuit_breaker must be a map with enable and rollback")
		} else {
			for _, key := range []string{"enable", "rollback"} {
				v, present := breaker[key]
				if !present || v == nil {
					continue
				}
				if _, isBool := v.(bool); !isBool {
					problems = append(problems, fmt.Sprintf(
						"workload: backend_circuit_breaker.%s is %v; it must be true or false", key, v))
				}
			}
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid backend health settings:\n  %s", strings.Join(problems, "\n  "))
	}
	return nil
}

func backendHealthCheckProblems(check map[string]interface{}) []string {
	var problems []string

	cmd := check["command"]
	list, isList := cmd.([]interface{})
	blank, isString := cmd.(string)
	switch {
	case cmd == nil, isList && len(list) == 0, isString && strings.TrimSpace(blank) == "":
		problems = append(problems, `workload: backend_health_check needs a command, e.g. command: "curl -fsS http://localhost:8080/health/live || exit 1"; remove backend_health_check for no container health check`)
	default:
		if problem := healthCheckCommandProblem("workload", "backend_health_check.command", cmd); problem != "" {
			problems = append(problems, problem)
		}
	}

	for _, r := range backendHealthCheckRanges {
		v, present := check[r.key]
		if !present || v == nil {
			continue
		}
		n, isWhole := wholeNumber(v)
		if !isWhole || n < r.min || n > r.max {
			problems = append(problems, fmt.Sprintf(
				"workload: backend_health_check.%s is %v; it must be a whole number from %d to %d",
				r.key, v, r.min, r.max))
		}
	}
	return problems
}

// wholeNumber reads an integer from the JSON-compatible map, where a YAML
// number arrives as int, int64, uint64 or float64 depending on the decoder.
func wholeNumber(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case uint64:
		return int(n), true
	case float64:
		if math.Floor(n) == n {
			return int(n), true
		}
	}
	return 0, false
}
