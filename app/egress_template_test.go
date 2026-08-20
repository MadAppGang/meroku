package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aymerick/raymond"
)

// These tests render env/main.hbs the way applyTemplate does and assert on the
// Terraform it produces. Generation is where egress_strategy actually takes
// effect, so a unit test of the Go model alone would not catch a template that
// forgets to pass the value through.

// renderMainTemplate renders env/main.hbs against a config map, mirroring the
// setup applyTemplate performs before Exec.
func renderMainTemplate(t *testing.T, envMap map[string]interface{}) string {
	t.Helper()

	// The template uses custom helpers ({{default}}, {{compare}}, ...). They are
	// registered behind a sync.Once, so this is safe to call from every test and
	// matches what the binary does before rendering.
	registerCustomHelpers()

	raw, err := os.ReadFile(filepath.Join("..", "env", "main.hbs"))
	if err != nil {
		t.Fatalf("reading env/main.hbs: %v", err)
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
		t.Fatalf("executing env/main.hbs: %v", err)
	}
	return out
}

// minimalEnvMap is the smallest config that renders. Every test starts here and
// changes only the field under test.
func minimalEnvMap(strategy string) map[string]interface{} {
	m := map[string]interface{}{
		"project":         "testproj",
		"env":             "dev",
		"region":          "us-east-1",
		"state_bucket":    "testproj-tfstate",
		"state_file":      "dev.tfstate",
		"use_default_vpc": false,
		"workload":        map[string]interface{}{},
		"postgres":        map[string]interface{}{},
		"domain":          map[string]interface{}{},
		"cognito":         map[string]interface{}{},
		"ses":             map[string]interface{}{},
		"sqs":             map[string]interface{}{},
		"alb":             map[string]interface{}{},
		"pubsub_appsync":  map[string]interface{}{},
	}
	if strategy != "" {
		m["egress_strategy"] = strategy
	}
	return m
}

func TestTemplate_PassesEgressStrategyToVPCModule(t *testing.T) {
	for _, strategy := range []string{"public_ip", "nat_gateway", "nat_gateway_ha"} {
		t.Run(strategy, func(t *testing.T) {
			out := renderMainTemplate(t, minimalEnvMap(strategy))

			want := `egress_strategy = "` + strategy + `"`
			if !strings.Contains(out, want) {
				t.Errorf("generated main.tf does not pass the strategy to the vpc module.\nwant a line containing: %s", want)
			}
		})
	}
}

// An environment written before schema v27 has no egress_strategy key at all.
// It must still render, and must render as public_ip so the next plan on an
// existing stack shows no changes.
func TestTemplate_MissingStrategyDefaultsToPublicIP(t *testing.T) {
	out := renderMainTemplate(t, minimalEnvMap(""))

	if !strings.Contains(out, `egress_strategy = "public_ip"`) {
		t.Error("a config with no egress_strategy must render as public_ip, or upgrading meroku would change existing infrastructure")
	}
}

// ECS tasks must follow the strategy, while internet-facing resources keep the
// public subnets. Getting this backwards puts an internet-facing ALB in private
// subnets, or leaves tasks with public IPs a NAT was meant to replace.
func TestTemplate_TasksUseTaskSubnetsAndALBUsesPublic(t *testing.T) {
	out := renderMainTemplate(t, minimalEnvMap("nat_gateway"))

	if !strings.Contains(out, "task_subnet_ids  = module.vpc.task_subnet_ids") {
		t.Error("locals must resolve task_subnet_ids from the vpc module")
	}
	if !strings.Contains(out, "assign_public_ip = module.vpc.assign_public_ip") {
		t.Error("locals must resolve assign_public_ip from the vpc module")
	}

	// The workloads module carries the ECS services; it must receive the task
	// subnets, not the public ones.
	if !strings.Contains(out, "subnet_ids       = local.task_subnet_ids") {
		t.Error("the workloads module must receive local.task_subnet_ids")
	}
	if !strings.Contains(out, "assign_public_ip = local.assign_public_ip") {
		t.Error("the workloads module must receive local.assign_public_ip")
	}
}

// The default VPC has only public subnets, so the locals must pin to public
// addressing there no matter what the YAML says.
func TestTemplate_DefaultVPCPinsToPublicAddressing(t *testing.T) {
	m := minimalEnvMap("public_ip")
	m["use_default_vpc"] = true

	out := renderMainTemplate(t, m)

	if !strings.Contains(out, "assign_public_ip = true") {
		t.Error("on the default VPC, assign_public_ip must be a literal true — there are no private subnets to fall back to")
	}
	if strings.Contains(out, "module.vpc.task_subnet_ids") {
		t.Error("the default VPC path must not reference the vpc module, which is not created")
	}
}

// Generation is the last gate before terraform runs, so the validator that
// rejects an impossible combination has to be reachable from the same map the
// template renders from.
func TestValidateEgressStrategyMap_BlocksNATOnDefaultVPC(t *testing.T) {
	m := map[string]interface{}{
		"egress_strategy": "nat_gateway",
		"use_default_vpc": true,
	}

	err := validateEgressStrategyMap(m)
	if err == nil {
		t.Fatal("expected an error: a NAT strategy has no private subnets on the default VPC")
	}
	if !strings.Contains(err.Error(), "use_default_vpc") {
		t.Errorf("the error should name the field to change, got: %v", err)
	}
}

func TestValidateEgressStrategyMap_RejectsUnknownValue(t *testing.T) {
	err := validateEgressStrategyMap(map[string]interface{}{"egress_strategy": "nat"})
	if err == nil {
		t.Fatal("expected an error for an unrecognised strategy")
	}
	// The message should list what IS accepted, so the fix is obvious.
	for _, want := range []string{"public_ip", "nat_gateway", "nat_gateway_ha"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message should list %q as a valid option, got: %v", want, err)
		}
	}
}

func TestValidateEgressStrategyMap_AllowsAbsentAndEmpty(t *testing.T) {
	if err := validateEgressStrategyMap(map[string]interface{}{}); err != nil {
		t.Errorf("absent egress_strategy must be valid (pre-v27 files): %v", err)
	}
	if err := validateEgressStrategyMap(map[string]interface{}{"egress_strategy": ""}); err != nil {
		t.Errorf("empty egress_strategy must be valid: %v", err)
	}
}
