package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

// API Gateway and the ALB are two ingress paths for the same hostname, and the
// switch between them has to be a single flag.
//
// It was not. The template only emitted `enable_alb = true` inside a nested
// `{{#if alb.enabled}}{{#if workload.backend_alb_domain_name}}`, so setting
// alb.enabled on its own rendered nothing at all: the ALB module was created,
// the workloads module never heard about it, and traffic kept going to API
// Gateway. backend_alb_domain_name is an optional extra hostname, not the
// switch that turns the ALB on.

func albModuleBlock(t *testing.T, rendered string) string {
	t.Helper()

	start := strings.Index(rendered, `module "alb" {`)
	if start < 0 {
		t.Fatal(`module "alb" not found in the rendered template`)
	}
	rest := rendered[start:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatal(`module "alb" block is unterminated`)
	}
	return rest[:end]
}

// fixtureWorkload returns the workload mapping from the shipped project/dev.yaml
// that renderMainHBS uses as its base.
func fixtureWorkload(t *testing.T) map[interface{}]interface{} {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "project", "dev.yaml"))
	if err != nil {
		t.Fatalf("reading project/dev.yaml: %v", err)
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing project/dev.yaml: %v", err)
	}
	workload, ok := doc["workload"].(map[interface{}]interface{})
	if !ok {
		t.Fatalf("project/dev.yaml workload is not a mapping: %#v", doc["workload"])
	}
	return workload
}

// albOverlay builds an overlay for renderMainHBS.
//
// The workload overrides are MERGED into the fixture's workload rather than
// replacing it: main.hbs reads a few dozen workload keys, so a wholesale
// replacement would break the render. The base project/dev.yaml sets
// backend_alb_domain_name, so a test that needs it absent has to clear it here
// rather than simply omit it.
func albOverlay(t *testing.T, alb map[string]interface{}, workload map[string]interface{}) map[string]interface{} {
	t.Helper()

	base := fixtureWorkload(t)
	for k, v := range workload {
		base[k] = v
	}
	return map[string]interface{}{
		"alb":      alb,
		"workload": base,
	}
}

// noExtraHostname clears the fixture's backend_alb_domain_name.
var noExtraHostname = map[string]interface{}{"backend_alb_domain_name": ""}

// The headline fix: alb.enabled alone must be enough to switch ingress.
func TestALBEnabledAloneTurnsOnTheALB(t *testing.T) {
	overlay := albOverlay(t, map[string]interface{}{"enabled": true}, noExtraHostname)
	block := workloadsModuleBlock(t, renderMainHBS(t, overlay))

	for _, want := range []string{
		"enable_alb = true",
		"alb_arn = module.alb.alb_arn",
		"alb_security_group_id = module.alb.alb_security_group_id",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("want %q with alb.enabled and no extra hostname:\n%s", want, block)
		}
	}

	// The old coupling, from the other side: the extra hostname is not required
	// to enable the ALB, and must not be assigned when it was not asked for.
	//
	// Matched as an assignment rather than a bare name: the rendered block
	// carries a comment explaining what backend_alb_domain_name is, so a
	// substring search for the name alone matches the prose and never fails.
	if strings.Contains(block, "backend_alb_domain_name =") {
		t.Errorf("backend_alb_domain_name should not be assigned when unset:\n%s", block)
	}
}

// It stays an optional extra when it is set.
func TestBackendALBDomainNameIsAnOptionalExtra(t *testing.T) {
	overlay := albOverlay(t,
		map[string]interface{}{"enabled": true},
		map[string]interface{}{"backend_alb_domain_name": "stream.example.com"},
	)
	block := workloadsModuleBlock(t, renderMainHBS(t, overlay))

	if !strings.Contains(block, "enable_alb = true") {
		t.Errorf("enable_alb missing:\n%s", block)
	}
	if !strings.Contains(block, `backend_alb_domain_name = "stream.example.com"`) {
		t.Errorf("the extra hostname did not render:\n%s", block)
	}
}

// Disabling the ALB must switch it off, not delete it from the graph.
//
// The module used to be wrapped in {{#if alb.enabled}} and its outputs passed to
// workloads only when on. Disabling therefore removed module.workloads' reference
// to the ALB security group — and that reference was the only thing telling
// Terraform that the backend's ingress rule depends on that group. With no edge,
// Terraform destroyed the group while the rule still pointed at it, AWS refused
// with DependencyViolation, and the apply retried for ~14 minutes before failing.
//
// So the assertions below are deliberately the opposite of what "disabled" would
// suggest: the wiring must still be emitted, carrying false and "".
func TestALBDisabledKeepsTheWiringSoTeardownOrders(t *testing.T) {
	overlay := albOverlay(t, map[string]interface{}{"enabled": false}, noExtraHostname)
	rendered := renderMainHBS(t, overlay)
	block := workloadsModuleBlock(t, rendered)

	// The references must survive, or the dependency edge dies with them.
	for _, want := range []string{
		"enable_alb = false",
		"alb_arn = module.alb.alb_arn",
		"alb_security_group_id = module.alb.alb_security_group_id",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("want %q — dropping it removes the edge that orders teardown:\n%s", want, block)
		}
	}

	// The extra hostname is genuinely conditional and must not appear.
	if strings.Contains(block, "backend_alb_domain_name =") {
		t.Errorf("backend_alb_domain_name should not be assigned when the ALB is off:\n%s", block)
	}

	// And the module itself has to still be emitted, switched off internally.
	albBlock := albModuleBlock(t, rendered)
	if !strings.Contains(albBlock, "enabled = false") {
		t.Errorf("module \"alb\" should be emitted with enabled = false, got:\n%s", albBlock)
	}
}

// The same wiring with the ALB on.
func TestALBEnabledPassesEnabledTrue(t *testing.T) {
	overlay := albOverlay(t, map[string]interface{}{"enabled": true}, noExtraHostname)
	albBlock := albModuleBlock(t, renderMainHBS(t, overlay))

	if !strings.Contains(albBlock, "enabled = true") {
		t.Errorf("module \"alb\" should be emitted with enabled = true, got:\n%s", albBlock)
	}
}

// idle_timeout is the reason to pick the ALB over API Gateway, so it has to
// reach the module. It always renders — the default helper supplies 60 — rather
// than being omitted when unset, so the value in effect is visible in the HCL.
func TestALBIdleTimeout(t *testing.T) {
	t.Run("defaults to 60", func(t *testing.T) {
		overlay := albOverlay(t, map[string]interface{}{"enabled": true}, nil)
		block := albModuleBlock(t, renderMainHBS(t, overlay))
		if !strings.Contains(block, "idle_timeout = 60") {
			t.Errorf("want the default idle_timeout = 60:\n%s", block)
		}
	})

	t.Run("a configured value wins", func(t *testing.T) {
		overlay := albOverlay(t, map[string]interface{}{"enabled": true, "idle_timeout": 300}, nil)
		block := albModuleBlock(t, renderMainHBS(t, overlay))
		if !strings.Contains(block, "idle_timeout = 300") {
			t.Errorf("want idle_timeout = 300:\n%s", block)
		}
	})
}

// Hostnames inside the environment's zone must come from the domain module.
// Re-deriving "${env}.${domain}" ignores add_env_domain_prefix and puts records
// outside the zone and the wildcard certificate.
func TestEnvDomainComesFromTheDomainModule(t *testing.T) {
	block := workloadsModuleBlock(t, renderMainHBS(t, map[string]interface{}{}))

	if !strings.Contains(block, "env_domain = module.domain.domain_name") {
		t.Errorf("env_domain is not wired from the domain module:\n%s", block)
	}
}
