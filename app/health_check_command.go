package main

import (
	"fmt"
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
func normalizeHealthCheckCommands(envMap map[string]interface{}) {
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

// validateHealthCheckCommand checks the list form. ECS only accepts a command
// whose first element is CMD or CMD-SHELL, and rejects anything else when the
// task definition is registered — partway through an apply, naming no service.
func validateHealthCheckCommand(name string, raw interface{}) string {
	if raw == nil {
		return ""
	}
	list, ok := raw.([]interface{})
	if !ok {
		return fmt.Sprintf("service '%s': health_check_command must be a string or a list of strings", name)
	}
	if len(list) == 0 {
		return ""
	}
	first, _ := list[0].(string)
	switch {
	case first == "CMD-SHELL" && len(list) != 2:
		return fmt.Sprintf(`service '%s': health_check_command ["CMD-SHELL", ...] takes exactly one command string; write it as a plain string instead`, name)
	case first == "CMD" && len(list) < 2:
		return fmt.Sprintf(`service '%s': health_check_command ["CMD"] needs the program to run, e.g. ["CMD", "/app/healthcheck"]`, name)
	case first != "CMD" && first != "CMD-SHELL":
		return fmt.Sprintf(`service '%s': a health_check_command list must start with "CMD" (run a program, no shell) or "CMD-SHELL"; for a shell command, write it as a plain string`, name)
	}
	return ""
}
