package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/aymerick/raymond"
	"gopkg.in/yaml.v2"
)

// The YAML -> HCL link for auto_deploy.
//
// Everything downstream of `module "workloads"` is covered: tf_identifiers has
// its own tests, lambda.tf has the boundary guards, and the Lambda has the
// handler tests. The one hop nothing watched is this one — whether the value a
// user sets in YAML reaches the module call at all. It is also the hop with the
// most ways to fail silently: a service field travels inside `{{{array
// services}}}` and is dropped without a word by Terraform's object conversion if
// variables.tf does not declare it, while a scheduled task's value needs a map
// built by hand in the template.

// renderMainHBS renders the real env/main.hbs over the shipped project/dev.yaml
// with an overlay applied, through the same helpers and the same pre-processing
// applyTemplate uses.
//
// The base is a real committed config rather than a minimal hand-written one:
// main.hbs reads several dozen keys, so a hand-rolled fixture would mostly be a
// transcription of that file's requirements, kept up to date by nobody.
func renderMainHBS(t *testing.T, overlay map[string]interface{}) string {
	t.Helper()

	registerCustomHelpers()

	raw, err := os.ReadFile(filepath.Join("..", "env", "main.hbs"))
	if err != nil {
		t.Fatalf("reading env/main.hbs: %v", err)
	}

	base, err := os.ReadFile(filepath.Join("..", "project", "dev.yaml"))
	if err != nil {
		t.Fatalf("reading project/dev.yaml: %v", err)
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal(base, &doc); err != nil {
		t.Fatalf("parsing project/dev.yaml: %v", err)
	}
	for k, v := range overlay {
		doc[k] = v
	}
	envMap, ok := convertToJSONCompatible(doc).(map[string]interface{})
	if !ok {
		t.Fatal("fixture yaml is not a mapping")
	}

	// The real pre-processing, not a copy of it: filters, then the zero-config
	// compute pool, then the compute refusals. Calling the same function
	// applyTemplate calls is what keeps every render test below on the pipeline
	// that actually ships — a hand-replicated block went stale silently the
	// first time applyTemplate gained a step.
	if err := preprocessEnvMap(envMap); err != nil {
		t.Fatalf("pre-processing the fixture config: %v", err)
	}
	envMap["modules"] = "../../infrastructure/modules"
	envMap["custom_modules"] = "../../custom"
	envMap["has_custom_pre"] = false
	envMap["has_custom_post"] = false

	tmpl, err := raymond.Parse(string(raw))
	if err != nil {
		t.Fatalf("parsing env/main.hbs: %v", err)
	}
	out, err := tmpl.Exec(envMap)
	if err != nil {
		t.Fatalf("rendering env/main.hbs: %v", err)
	}
	return out
}

// workloadsModuleBlock returns the body of `module "workloads" { ... }`.
func workloadsModuleBlock(t *testing.T, rendered string) string {
	t.Helper()

	start := strings.Index(rendered, `module "workloads" {`)
	if start < 0 {
		t.Fatal(`module "workloads" not found in the rendered template`)
	}
	rest := rendered[start:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatal(`module "workloads" block is unterminated`)
	}
	return rest[:end]
}

// autoDeployOverlay is the part of the config under test: one service on, one
// off, one scheduled task on, one off, and the backend off.
//
// withAutoDeploy=false drops every auto_deploy key, which is what a
// not-yet-migrated or hand-written YAML looks like.
func autoDeployOverlay(t *testing.T, withAutoDeploy bool) map[string]interface{} {
	t.Helper()

	base, err := os.ReadFile(filepath.Join("..", "project", "dev.yaml"))
	if err != nil {
		t.Fatalf("reading project/dev.yaml: %v", err)
	}
	var doc map[interface{}]interface{}
	if err := yaml.Unmarshal(base, &doc); err != nil {
		t.Fatalf("parsing project/dev.yaml: %v", err)
	}
	workload, ok := doc["workload"].(map[interface{}]interface{})
	if !ok {
		t.Fatal("project/dev.yaml has no workload section")
	}
	if withAutoDeploy {
		workload["backend_auto_deploy"] = false
	} else {
		delete(workload, "backend_auto_deploy")
	}

	service := func(name string, autoDeploy bool) map[interface{}]interface{} {
		s := map[interface{}]interface{}{
			"name": name, "container_port": 3000, "host_port": 3000,
		}
		if withAutoDeploy {
			s["auto_deploy"] = autoDeploy
		}
		return s
	}
	task := func(name, schedule string, autoDeploy bool) map[interface{}]interface{} {
		s := map[interface{}]interface{}{"name": name, "schedule": schedule}
		if withAutoDeploy {
			s["auto_deploy"] = autoDeploy
		}
		return s
	}

	return map[string]interface{}{
		"env":      "prod",
		"is_prod":  true,
		"workload": workload,
		"services": []interface{}{
			service("api", true),
			service("reporting", false),
		},
		"scheduled_tasks": []interface{}{
			task("cleanup", "rate(1 day)", true),
			task("archive", "rate(7 days)", false),
		},
	}
}

func TestAutoDeployReachesTheWorkloadsModule(t *testing.T) {
	block := workloadsModuleBlock(t, renderMainHBS(t, autoDeployOverlay(t, true)))

	// Backend: its own variable.
	if !regexp.MustCompile(`backend_auto_deploy\s*=\s*false`).MatchString(block) {
		t.Errorf("backend_auto_deploy=false did not reach the module call:\n%s", block)
	}

	// Services: carried inside the services array. Terraform drops an
	// undeclared attribute during object conversion without a word, so this
	// only proves the template half — modules/workloads/variables.tf declaring
	// auto_deploy is the other half, and terraform validate covers that.
	servicesLine := regexp.MustCompile(`(?m)^\s*services = (.*)$`).FindStringSubmatch(block)
	if servicesLine == nil {
		t.Fatalf("no services argument in the module call:\n%s", block)
	}
	for _, want := range []string{
		`"name":"api"`, `"auto_deploy":true`,
		`"name":"reporting"`, `"auto_deploy":false`,
	} {
		if !strings.Contains(servicesLine[1], want) {
			t.Errorf("services argument is missing %s:\n%s", want, servicesLine[1])
		}
	}

	// Scheduled tasks: a hand-built map, because the module takes names only.
	for _, want := range []string{`"cleanup" = true`, `"archive" = false`} {
		if !strings.Contains(block, want) {
			t.Errorf("scheduled_task_auto_deploy is missing %s:\n%s", want, block)
		}
	}
}

// An absent auto_deploy must render as true, not as false and not as an error.
// That is the upgrade path: a hand-written or not-yet-migrated YAML has to keep
// deploying exactly as it does today, in every environment, including prod.
func TestAbsentAutoDeployRendersAsTrue(t *testing.T) {
	block := workloadsModuleBlock(t, renderMainHBS(t, autoDeployOverlay(t, false)))

	if !regexp.MustCompile(`backend_auto_deploy\s*=\s*true`).MatchString(block) {
		t.Errorf("an absent workload.backend_auto_deploy must render as true:\n%s", block)
	}
	for _, want := range []string{`"cleanup" = true`, `"archive" = true`} {
		if !strings.Contains(block, want) {
			t.Errorf("an absent task auto_deploy must render as true, missing %s:\n%s", want, block)
		}
	}
	// Services carry no key at all, and modules/workloads/variables.tf defaults
	// the optional attribute to true.
	if strings.Contains(block, "auto_deploy\":") {
		t.Errorf("services should carry no auto_deploy key when the YAML has none:\n%s", block)
	}
}

// `false` must survive the `default` helper. The helper used to treat any falsy
// value as "not set", which would have turned every disabled target back on at
// render time — the exact failure this setting exists to make visible.
func TestAutoDeployFalseSurvivesTheDefaultHelper(t *testing.T) {
	registerCustomHelpers()

	out, err := raymond.Render(`{{default auto_deploy true}}`, map[string]interface{}{
		"auto_deploy": false,
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if out != "false" {
		t.Errorf("default helper turned an explicit false into %q", out)
	}
}
