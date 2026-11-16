package main

import (
	"fmt"
	"io"
	"os"
)

// warningChoice is what the user chose on the configuration warnings screen.
type warningChoice int

const (
	warningChoiceCancel warningChoice = iota
	warningChoiceProceed
)

// deployCancelledMessage is written when the user chooses to fix the
// configuration first. Nothing has been generated, migrated or planned yet.
const deployCancelledMessage = "Deploy cancelled. Nothing ran."

// deployWarningGate decides whether a TUI deploy may start, given the
// environment's configuration warnings. Every field is a plain function so
// tests can drive each rule without a terminal.
type deployWarningGate struct {
	// collect computes the warnings for env.
	collect func(env string) ConfigWarningReport
	// interactive reports whether there is a human at a terminal to ask.
	interactive func() bool
	// ask shows the warnings and returns the user's choice.
	ask func(env string, ws []ConfigWarning) (warningChoice, error)
	// out receives the cancel line.
	out io.Writer
}

// defaultDeployWarningGate is the gate the main-menu deploy uses: warnings
// computed in this process, so region_drift matches what generate will print.
func defaultDeployWarningGate() deployWarningGate {
	return deployWarningGate{
		collect: func(env string) ConfigWarningReport {
			return collectEnvConfigWarnings(env, defaultWarningInputs(env))
		},
		interactive: interactiveTerminal,
		ask:         runConfigWarningsScreen,
		out:         os.Stdout,
	}
}

// allow reports whether the deploy may start. Rules, in order:
//
//   - a config error: allowed, so generate reports the error with the Claude
//     Code handoff as it always has;
//   - no warnings: allowed, nothing is shown;
//   - no terminal (CI, a pipe): allowed without asking; generate prints the
//     warnings;
//   - the user chose Proceed anyway: allowed;
//   - otherwise not allowed, and the cancel line is written.
//
// An ask that fails counts as Cancel, so a screen that cannot start never
// deploys.
func (g deployWarningGate) allow(env string) bool {
	report := g.collect(env)
	if report.ConfigError != "" {
		return true
	}
	if len(report.Warnings) == 0 {
		return true
	}
	if !g.interactive() {
		return true
	}

	choice, err := g.ask(env, report.Warnings)
	if err == nil && choice == warningChoiceProceed {
		return true
	}
	if err != nil {
		fmt.Fprintf(g.out, "Could not show the configuration warnings: %v\n", err)
	}
	fmt.Fprintln(g.out, deployCancelledMessage)
	return false
}

// deployWithGate starts run(env) only if the gate allows it. It is the one
// place that puts the warnings before the deploy: the gate runs before run
// generates, migrates or plans anything.
func deployWithGate(env string, gate deployWarningGate, run func(env string)) {
	if gate.allow(env) {
		run(env)
	}
}
