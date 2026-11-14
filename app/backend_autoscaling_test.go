package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

// workload.backend_autoscaling_target_cpu and _target_memory: the YAML ->
// main.tf hop, and what a save through the Workload struct does to a config
// that sets them and to one that never did. Both reach Terraform as
// target_value on the two target-tracking policies in
// modules/workloads/backend_autoscaling.tf, where 0 is not a usable target.

// autoscalingWorkload returns project/dev.yaml's workload section with both
// target keys removed, then the given keys set.
//
// The shipped config omits them, so the two deletes are what keeps "an
// environment that never tuned its targets" the case it describes if dev.yaml
// ever gains the keys.
func autoscalingWorkload(t *testing.T, set map[string]interface{}) map[interface{}]interface{} {
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
	delete(workload, "backend_autoscaling_target_cpu")
	delete(workload, "backend_autoscaling_target_memory")
	for k, v := range set {
		workload[k] = v
	}
	return workload
}

// savedAutoscalingWorkload is a workload section as saveEnvToFile writes it:
// unmarshalled into Workload and marshalled straight back out. Rendering that
// rather than the original is what makes the assertion about a save and not
// only about the template.
func savedAutoscalingWorkload(t *testing.T, workload map[interface{}]interface{}) map[string]interface{} {
	t.Helper()

	src, err := yaml.Marshal(workload)
	if err != nil {
		t.Fatalf("marshalling the workload section: %v", err)
	}
	return roundTripWorkload(t, string(src))
}

// assertAutoscalingTargets checks the two arguments in the rendered
// `module "workloads"` call, and that the render as a whole parses.
func assertAutoscalingTargets(t *testing.T, rendered, cpu, memory string) {
	t.Helper()

	if err := validateGeneratedHCL("env/dev/main.tf", rendered); err != nil {
		t.Fatalf("the rendered Terraform does not parse: %v", err)
	}
	block := workloadsModuleBlock(t, rendered)
	for _, want := range []struct{ name, value string }{
		{"backend_autoscaling_target_cpu", cpu},
		{"backend_autoscaling_target_memory", memory},
	} {
		got := strings.TrimSpace(argumentValue(t, block, want.name))
		if got != want.value {
			t.Errorf("%s = %s, want %s", want.name, got, want.value)
		}
	}
}

// The requirement both fields exist to meet: a config that omits them renders
// the template's defaults before a save and still renders them after one, and a
// config that sets them keeps its own values through the same round trip.
//
// The two halves fail differently. Dropping a set value silently resets a
// project's scaling targets to 70 and 80 at the next generate. Writing 0 for an
// unset one is worse: the `default` helper returns 0 for 0 rather than falling
// back, so the policy renders target_value = 0. The template passes all five
// arguments unconditionally while both policies are
// count = var.backend_autoscaling_enabled ? 1 : 0, so that 0 reaches AWS only in
// a config that turned autoscaling on — and there the apply fails on it.
func TestBackendAutoscalingTargetsRenderTheSameAfterASave(t *testing.T) {
	cases := []struct {
		name        string
		set         map[string]interface{}
		cpu, memory string
	}{
		{name: "absent keys take the template defaults", cpu: "70", memory: "80"},
		{
			name: "set keys keep their values",
			set: map[string]interface{}{
				"backend_autoscaling_target_cpu":    55,
				"backend_autoscaling_target_memory": 65,
			},
			cpu:    "55",
			memory: "65",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workload := autoscalingWorkload(t, tc.set)
			assertAutoscalingTargets(t, renderMainHBS(t, map[string]interface{}{"workload": workload}), tc.cpu, tc.memory)

			saved := savedAutoscalingWorkload(t, workload)
			assertAutoscalingTargets(t, renderMainHBS(t, map[string]interface{}{"workload": saved}), tc.cpu, tc.memory)
		})
	}
}

func TestBackendAutoscalingTargetsSurviveASave(t *testing.T) {
	m := roundTripWorkload(t, `
backend_autoscaling_enabled: true
backend_autoscaling_min_capacity: 2
backend_autoscaling_max_capacity: 8
backend_autoscaling_target_cpu: 55
backend_autoscaling_target_memory: 65
`)

	for name, want := range map[string]interface{}{
		"backend_autoscaling_target_cpu":    55,
		"backend_autoscaling_target_memory": 65,
	} {
		if got, present := m[name]; !present || got != want {
			t.Errorf("%s must survive a save as %v, got %#v (present=%v)", name, want, got, present)
		}
	}
}

// The reason the two fields are *int32 rather than int32 with omitempty, pinned.
//
// Nothing else here can tell the two shapes apart: neither one drops a value the
// cases above set, and neither one writes a key into a config that never set it.
// An explicit 0 is the only value that separates them, because omitempty on an
// int32 treats it as absent while a non-nil pointer keeps it. Only the round trip
// is asserted — what it renders is target_value = 0, which is not a
// configuration anybody should apply.
func TestBackendAutoscalingTargetZeroSurvivesASave(t *testing.T) {
	m := roundTripWorkload(t, `
backend_autoscaling_enabled: true
backend_autoscaling_target_cpu: 0
`)

	got, present := m["backend_autoscaling_target_cpu"]
	if !present {
		t.Fatal("an explicit backend_autoscaling_target_cpu: 0 did not survive a save: omitempty on a plain int32 drops it, which is why the field is *int32")
	}
	if got != 0 {
		t.Errorf("backend_autoscaling_target_cpu: 0 saved as %#v, want 0", got)
	}
}

// An unset key stays unset through a save. This says nothing about the fields
// still existing — deleting either one leaves it green. Deletion is caught by
// TestEnvRoundTrip_FixtureCoversTheGuardedFields, which names both fields and so
// fails to compile without them, and by
// TestBackendAutoscalingTargetsSurviveASave, which fails when a key it set comes
// back absent.
func TestBackendAutoscalingTargetsStayAbsentWhenUnset(t *testing.T) {
	m := roundTripWorkload(t, "backend_autoscaling_enabled: true\n")

	for _, name := range []string{"backend_autoscaling_target_cpu", "backend_autoscaling_target_memory"} {
		if got, present := m[name]; present {
			t.Errorf("a YAML without %s must save without it, got %#v: a 0 here renders target_value = 0, because the default helper returns 0 for 0 instead of falling back", name, got)
		}
	}
}
