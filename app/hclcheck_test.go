package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

func TestValidateGeneratedHCLAcceptsValidTerraform(t *testing.T) {
	const source = `
module "workloads" {
  source            = "../../infrastructure/modules/workloads"
  setup_FCM_SNS     = false
  container_command = ["npm", "run", "cron"]
  filter_policy     = jsonencode({"event": ["created"]})
}
`
	if err := validateGeneratedHCL("env/dev/main.tf", source); err != nil {
		t.Fatalf("valid terraform was rejected: %v", err)
	}
}

// The shape a missing YAML key renders into. This is the defect the gate exists
// for, reduced to one line.
func TestValidateGeneratedHCLRejectsMissingExpression(t *testing.T) {
	const source = `module "workloads" {
  backend_remote_access = true
  setup_FCM_SNS =
  backend_image_port = 8080
}
`
	err := validateGeneratedHCL("env/dev/main.tf", source)
	if err == nil {
		t.Fatal("a bare `=` with no expression was accepted as valid terraform")
	}

	message := err.Error()
	for _, want := range []string{
		"env/dev/main.tf", // which file
		"line 3",          // where
		"setup_FCM_SNS =", // the offending source, quoted back
		"Nothing was written.",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("error message is missing %q:\n%s", want, message)
		}
	}
}

// A cascade must not bury the first diagnostic, which is nearly always the cause.
func TestValidateGeneratedHCLReportsFirstErrorFirst(t *testing.T) {
	const source = `module "a" {
  first =
  second = "ok"
}
`
	err := validateGeneratedHCL("main.tf", source)
	if err == nil {
		t.Fatal("expected an error")
	}
	message := err.Error()

	firstAt := strings.Index(message, "line 2")
	if firstAt < 0 {
		t.Fatalf("the first bad line is not reported at all:\n%s", message)
	}
	for _, later := range []string{"line 3", "line 4"} {
		if at := strings.Index(message, later); at >= 0 && at < firstAt {
			t.Errorf("%s is reported before line 2:\n%s", later, message)
		}
	}
}

// The end-to-end proof: the real env/main.hbs over the real project/dev.yaml has
// to produce Terraform that parses. This is the regression net for all ~70 of
// the template's bare `key = {{value}}` assignment sites at once — any one of
// them losing its value shows up here rather than at terraform init.
func TestShippedConfigGeneratesValidHCL(t *testing.T) {
	rendered := renderMainHBS(t, nil)

	if err := validateGeneratedHCL("env/dev/main.tf", rendered); err != nil {
		t.Fatalf("env/main.hbs over project/dev.yaml does not generate valid terraform:\n%v", err)
	}
}

// Dropping one scalar key is all it takes, and the template gives no sign.
// postgres.iam_database_authentication_enabled renders bare, with no default:
// the YAML migrator always writes it, so only a hand-written file can leave it
// out. applyTemplate reads the YAML into a plain map rather than the Env
// struct, so a missing key arrives as nothing at all, not as false, and the
// assignment is left with no value.
func TestOmittedScalarIsCaughtBeforeWriting(t *testing.T) {
	postgres := fixtureMapping(t, "postgres")
	if postgres["enabled"] != true {
		t.Fatal("project/dev.yaml no longer enables postgres, so the postgres module is not rendered; pick another scalar key for this test")
	}
	if _, present := postgres["iam_database_authentication_enabled"]; !present {
		t.Fatal("project/dev.yaml no longer sets postgres.iam_database_authentication_enabled; pick another scalar key for this test")
	}
	delete(postgres, "iam_database_authentication_enabled")

	rendered := renderMainHBS(t, map[string]interface{}{"postgres": postgres})

	// The template renders it happily, which is the whole problem. The trailing
	// space is the one in `iam_database_authentication_enabled = {{...}}`, left
	// behind once the stache resolves to nothing.
	if !strings.Contains(rendered, "iam_database_authentication_enabled = \n") {
		t.Fatalf("expected the omitted key to render a bare `=`, got:\n%s", excerptAround(rendered, "iam_database_authentication_enabled"))
	}

	if err := validateGeneratedHCL("env/dev/main.tf", rendered); err == nil {
		t.Fatal("generation would have written terraform that cannot be parsed")
	}
}

// fixtureMapping returns one top-level mapping from the shipped
// project/dev.yaml that renderMainHBS uses as its base.
func fixtureMapping(t *testing.T, key string) map[interface{}]interface{} {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "project", "dev.yaml"))
	if err != nil {
		t.Fatalf("reading project/dev.yaml: %v", err)
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing project/dev.yaml: %v", err)
	}
	section, ok := doc[key].(map[interface{}]interface{})
	if !ok {
		t.Fatalf("project/dev.yaml %s is not a mapping: %#v", key, doc[key])
	}
	return section
}

// excerptAround returns a few lines of rendered output around needle, for
// failure messages that would otherwise dump a thousand lines of Terraform.
func excerptAround(rendered, needle string) string {
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		if strings.Contains(line, needle) {
			return strings.Join(lines[max(i-2, 0):min(i+3, len(lines))], "\n")
		}
	}
	return "(" + needle + " not found in the rendered output)"
}
