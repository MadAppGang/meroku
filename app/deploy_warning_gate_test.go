package main

import (
	"bytes"
	"strings"
	"testing"
)

func gateTestWarning() ConfigWarning {
	return ConfigWarning{
		Code:   "service_no_health_check",
		Item:   WarningItem{Kind: WarningKindService, Name: "magento-bridge"},
		Title:  "service needs a health check",
		Detail: "Add a health_check_command.",
	}
}

// REQ-FR-7, AC-2: Cancel is a true gate; the deploy callback never runs.
func TestDeployWithGateCancelDoesNotRunDeploy(t *testing.T) {
	var out bytes.Buffer
	runCalls := 0
	askCalls := 0
	gate := deployWarningGate{
		collect: func(env string) ConfigWarningReport {
			return ConfigWarningReport{Env: env, Warnings: []ConfigWarning{gateTestWarning()}}
		},
		interactive: func() bool { return true },
		ask: func(env string, warnings []ConfigWarning) (warningChoice, error) {
			askCalls++
			if env != "dev" || len(warnings) != 1 {
				t.Errorf("ask received env %q and warnings %#v", env, warnings)
			}
			return warningChoiceCancel, nil
		},
		out: &out,
	}

	deployWithGate("dev", gate, func(env string) { runCalls++ })
	if runCalls != 0 {
		t.Fatalf("deploy ran %d times after Cancel, want 0", runCalls)
	}
	if askCalls != 1 {
		t.Fatalf("warning screen shown %d times, want 1", askCalls)
	}
	if out.String() != "Deploy cancelled. Nothing ran.\n" {
		t.Fatalf("cancel output = %q, want the contracted line", out.String())
	}
}

// REQ-FR-7, AC-2: Proceed crosses the gate exactly once.
func TestDeployWithGateProceedRunsDeployOnce(t *testing.T) {
	var out bytes.Buffer
	runCalls := 0
	runEnv := ""
	gate := deployWarningGate{
		collect: func(env string) ConfigWarningReport {
			return ConfigWarningReport{Env: env, Warnings: []ConfigWarning{gateTestWarning()}}
		},
		interactive: func() bool { return true },
		ask: func(env string, warnings []ConfigWarning) (warningChoice, error) {
			return warningChoiceProceed, nil
		},
		out: &out,
	}

	deployWithGate("dev", gate, func(env string) {
		runCalls++
		runEnv = env
	})
	if runCalls != 1 || runEnv != "dev" {
		t.Fatalf("deploy calls = %d for env %q, want one dev deploy", runCalls, runEnv)
	}
	if strings.Contains(out.String(), "Deploy cancelled") {
		t.Fatalf("Proceed printed cancellation: %q", out.String())
	}
}

// REQ-FR-3, REQ-FR-8, AC-3: no terminal means continue without asking; generate prints later.
func TestDeployWithGateNonInteractiveRunsWithoutGateOutput(t *testing.T) {
	var out bytes.Buffer
	runCalls := 0
	gate := deployWarningGate{
		collect: func(env string) ConfigWarningReport {
			return ConfigWarningReport{Env: env, Warnings: []ConfigWarning{gateTestWarning()}}
		},
		interactive: func() bool { return false },
		ask: func(env string, warnings []ConfigWarning) (warningChoice, error) {
			t.Fatal("non-interactive deploy must not open the warning screen")
			return warningChoiceCancel, nil
		},
		out: &out,
	}

	deployWithGate("dev", gate, func(env string) { runCalls++ })
	if runCalls != 1 {
		t.Fatalf("non-interactive deploy ran %d times, want 1", runCalls)
	}
	if out.Len() != 0 {
		t.Fatalf("non-interactive gate produced output before generate: %q", out.String())
	}
}

// AC-4: clean YAML never creates a deploy dialog.
func TestDeployWithGateNoWarningsRunsWithoutScreen(t *testing.T) {
	var out bytes.Buffer
	runCalls := 0
	gate := deployWarningGate{
		collect: func(env string) ConfigWarningReport {
			return ConfigWarningReport{Env: env, Warnings: []ConfigWarning{}}
		},
		interactive: func() bool { return true },
		ask: func(env string, warnings []ConfigWarning) (warningChoice, error) {
			t.Fatal("clean deploy must not open the warning screen")
			return warningChoiceCancel, nil
		},
		out: &out,
	}

	deployWithGate("dev", gate, func(env string) { runCalls++ })
	if runCalls != 1 {
		t.Fatalf("clean deploy ran %d times, want 1", runCalls)
	}
	if out.Len() != 0 {
		t.Fatalf("clean deploy produced warning-gate output: %q", out.String())
	}
}

// Contract 8.6: generate reports a configError later; the warning screen stays out of it.
func TestDeployWithGateConfigErrorRunsWithoutWarningScreen(t *testing.T) {
	var out bytes.Buffer
	runCalls := 0
	gate := deployWarningGate{
		collect: func(env string) ConfigWarningReport {
			return ConfigWarningReport{
				Env: env, Warnings: []ConfigWarning{gateTestWarning()}, ConfigError: "invalid configuration",
			}
		},
		interactive: func() bool { return true },
		ask: func(env string, warnings []ConfigWarning) (warningChoice, error) {
			t.Fatal("configError must not open the warning screen")
			return warningChoiceCancel, nil
		},
		out: &out,
	}

	deployWithGate("dev", gate, func(env string) { runCalls++ })
	if runCalls != 1 {
		t.Fatalf("configError deploy ran %d times, want 1", runCalls)
	}
	if out.Len() != 0 {
		t.Fatalf("configError gate produced output before generate: %q", out.String())
	}
}
