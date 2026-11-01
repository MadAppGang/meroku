package main

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

// The two YAML forms, read through the Env struct the web API handlers use.
func TestHealthCheckCommand_ReadsBothForms(t *testing.T) {
	for name, tc := range map[string]struct {
		yaml string
		want []string
	}{
		"string is a shell command": {
			yaml: `health_check_command: "curl -f http://localhost:3000/health/live || exit 1"`,
			want: []string{"CMD-SHELL", "curl -f http://localhost:3000/health/live || exit 1"},
		},
		"list is the raw ECS form": {
			yaml: `health_check_command: ["CMD", "/app/healthcheck"]`,
			want: []string{"CMD", "/app/healthcheck"},
		},
		"absent is no check": {
			yaml: `name: x`,
			want: nil,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var s Service
			if err := yaml.Unmarshal([]byte(tc.yaml), &s); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !reflect.DeepEqual(s.HealthCheckCommand.Command, tc.want) {
				t.Errorf("Command = %q, want %q", s.HealthCheckCommand.Command, tc.want)
			}
		})
	}
}

// meroku loads and saves configs through the struct; a user's string must not
// come back as a list they never wrote, and an unset field must not appear.
func TestHealthCheckCommand_RoundTripsInTheFormItWasWritten(t *testing.T) {
	for _, in := range []string{
		`health_check_command: curl -f http://localhost:3000/ || exit 1`,
		`health_check_command: [CMD, /app/healthcheck]`,
	} {
		var s struct {
			HC HealthCheckCommand `yaml:"health_check_command,omitempty"`
		}
		if err := yaml.Unmarshal([]byte(in), &s); err != nil {
			t.Fatalf("unmarshal %q: %v", in, err)
		}
		out, err := yaml.Marshal(s)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var again struct {
			HC HealthCheckCommand `yaml:"health_check_command,omitempty"`
		}
		if err := yaml.Unmarshal(out, &again); err != nil {
			t.Fatalf("re-unmarshal %q: %v", out, err)
		}
		if again.HC.fromString != s.HC.fromString || !reflect.DeepEqual(again.HC.Command, s.HC.Command) {
			t.Errorf("round trip changed %q into %q", in, out)
		}
	}

	var empty struct {
		HC HealthCheckCommand `yaml:"health_check_command,omitempty"`
	}
	out, _ := yaml.Marshal(empty)
	if strings.Contains(string(out), "health_check_command") {
		t.Errorf("an unset command must be omitted, got %q", out)
	}
}

func TestValidateHealthCheckCommand(t *testing.T) {
	ok := [][]interface{}{
		{"CMD-SHELL", "curl -f http://localhost/ || exit 1"},
		{"CMD", "/app/healthcheck"},
		{"CMD", "/app/healthcheck", "--port", "8080"},
	}
	for _, cmd := range ok {
		if p := validateHealthCheckCommand("svc", cmd); p != "" {
			t.Errorf("%q refused: %s", cmd, p)
		}
	}

	bad := map[string][]interface{}{
		"no CMD prefix":           {"curl", "-f", "http://localhost/"},
		"CMD-SHELL with two args": {"CMD-SHELL", "curl", "-f"},
		"CMD with no program":     {"CMD"},
	}
	for name, cmd := range bad {
		if p := validateHealthCheckCommand("svc", cmd); p == "" || !strings.Contains(p, "svc") {
			t.Errorf("%s: %q must be refused naming the service, got %q", name, cmd, p)
		}
	}
}

// The string form must reach the module call as a list, or Terraform's
// list(string) conversion fails on a plain string.
func TestHealthCheckStringReachesTheModuleAsAList(t *testing.T) {
	overlay := map[string]interface{}{
		"services": []interface{}{
			map[interface{}]interface{}{
				"name": "bridge", "container_port": 8080, "host_port": 8080,
				"network_access":       "internal",
				"health_check_command": "wget -qO- http://localhost:8080/health/live || exit 1",
			},
		},
	}
	block := workloadsModuleBlock(t, renderMainHBS(t, overlay))

	servicesLine := regexp.MustCompile(`(?m)^\s*services = (.*)$`).FindStringSubmatch(block)
	if servicesLine == nil {
		t.Fatalf("no services argument in the module call:\n%s", block)
	}
	want := `"health_check_command":["CMD-SHELL","wget -qO- http://localhost:8080/health/live || exit 1"]`
	if !strings.Contains(servicesLine[1], want) {
		t.Errorf("services argument is missing %s:\n%s", want, servicesLine[1])
	}
}
