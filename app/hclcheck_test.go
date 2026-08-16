package main

import (
	"strings"
	"testing"
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

// Dropping one optional key is all it takes, and the template gives no sign.
// setup_fcnsns is a bool in the Env struct, so it would always have a value if
// generation went through the struct — but applyTemplate reads the YAML into a
// plain map, which is why the zero value never arrives.
func TestOmittedScalarIsCaughtBeforeWriting(t *testing.T) {
	workload := fixtureWorkload(t)
	if _, present := workload["setup_fcnsns"]; !present {
		t.Fatal("project/dev.yaml no longer sets workload.setup_fcnsns; pick another scalar key for this test")
	}
	delete(workload, "setup_fcnsns")

	rendered := renderMainHBS(t, map[string]interface{}{"workload": workload})

	// The template renders it happily, which is the whole problem. The trailing
	// space is the one in `setup_FCM_SNS = {{workload.setup_fcnsns}}`, left
	// behind once the stache resolves to nothing.
	if !strings.Contains(rendered, "setup_FCM_SNS = \n") {
		t.Fatalf("expected the omitted key to render a bare `=`, got:\n%s", excerptAround(rendered, "setup_FCM_SNS"))
	}

	if err := validateGeneratedHCL("env/dev/main.tf", rendered); err == nil {
		t.Fatal("generation would have written terraform that cannot be parsed")
	}
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
