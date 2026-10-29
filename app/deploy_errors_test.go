package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Generation, pre-flight, DNS and migration failures used to os.Exit(1) in the
// middle of a deploy, so the "Fix with AI (Claude Code)" handoff could never be
// offered for them. These tests pin the replacement: every one of those
// failures comes back as an error naming the step, the interactive deploy
// offers the handoff for it, and `meroku generate` still exits 1.
//
// All fixtures are synthetic. Nothing here reaches AWS, tmux or claude: the
// pre-flight is replaced before it could, and the handoff offer is a recorder.

// deployFixture lays out a minimal project at a temp root and moves into it.
// template is infrastructure/env/main.hbs; yamlBody, when non-empty, replaces
// the generated dev.yaml.
func deployFixture(t *testing.T, template, yamlBody string) string {
	t.Helper()
	root := t.TempDir()
	chdir(t, root)

	writeStaleConfig(t, root, "dev", CurrentSchemaVersion)
	if yamlBody != "" {
		if err := os.WriteFile(filepath.Join(root, "dev.yaml"), []byte(yamlBody), 0o644); err != nil {
			t.Fatalf("write yaml: %v", err)
		}
	}
	if template != "" {
		dir := filepath.Join(root, "infrastructure", "env")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir template dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.hbs"), []byte(template), 0o644); err != nil {
			t.Fatalf("write template: %v", err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "env", "dev"), 0o755); err != nil {
		t.Fatalf("mkdir env/dev: %v", err)
	}
	return root
}

// stubPreflight replaces the deploy's AWS pre-flight for one test.
func stubPreflight(t *testing.T, err error) *int {
	t.Helper()
	calls := 0
	orig := deployPreflight
	deployPreflight = func(context.Context, Env) error {
		calls++
		return err
	}
	t.Cleanup(func() { deployPreflight = orig })
	return &calls
}

func sameDir(t *testing.T, a, b string) bool {
	t.Helper()
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		t.Fatalf("resolve %s: %v", a, err)
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		t.Fatalf("resolve %s: %v", b, err)
	}
	return ra == rb
}

func TestApplyTemplateReturnsErrorWhenTemplateIsMissing(t *testing.T) {
	deployFixture(t, "", "")

	var err error
	captureOutput(t, func() { err = applyTemplate("dev") })

	if err == nil {
		t.Fatal("applyTemplate returned nil for a missing template")
	}
	if !strings.Contains(err.Error(), "main.hbs") {
		t.Errorf("error does not name the template: %v", err)
	}
}

func TestApplyTemplateReturnsErrorWhenTemplateDoesNotParse(t *testing.T) {
	deployFixture(t, "{{#if env}}\nunclosed\n", "")

	var err error
	captureOutput(t, func() { err = applyTemplate("dev") })

	if err == nil {
		t.Fatal("applyTemplate returned nil for a template that does not parse")
	}
}

// A broken YAML is the load-and-migrate step, not the render: the briefing is
// named after it, so the distinction reaches Claude.
func TestApplyTemplateTagsABrokenYAMLAsMigration(t *testing.T) {
	deployFixture(t, "# {{env}}\n", "env: [unclosed\n")

	var err error
	captureOutput(t, func() { err = applyTemplate("dev") })

	var se *deployStepError
	if !errors.As(err, &se) {
		t.Fatalf("want a *deployStepError, got %T: %v", err, err)
	}
	if se.Step != "migration" {
		t.Errorf("step = %q, want migration", se.Step)
	}
}

// generateEnvironmentFiles used to return nil unconditionally after calling
// applyTemplate; the recovery path then told the user env/<env>/ was restored.
func TestGenerateEnvironmentFilesPropagatesApplyTemplateError(t *testing.T) {
	root := deployFixture(t, "resource \"x\" \"y\" {\n", "")

	var err error
	captureOutput(t, func() { err = generateEnvironmentFiles("dev") })

	if err == nil {
		t.Fatal("generateEnvironmentFiles returned nil for terraform that does not parse")
	}
	if _, statErr := os.Stat(filepath.Join(root, "env", "dev", "main.tf")); statErr == nil {
		t.Error("an invalid render was written to main.tf")
	}
}

func TestRunCommandToDeployReturnsMigrationErrorForBrokenYAML(t *testing.T) {
	root := deployFixture(t, "# {{env}}\n", "env: [unclosed\n")
	calls := stubPreflight(t, nil)

	var err error
	captureOutput(t, func() { err = runCommandToDeploy("dev") })

	var se *deployStepError
	if !errors.As(err, &se) {
		t.Fatalf("want a *deployStepError, got %T: %v", err, err)
	}
	if se.Step != "migration" {
		t.Errorf("step = %q, want migration", se.Step)
	}
	if *calls != 0 {
		t.Errorf("pre-flight ran %d times after the config failed to load", *calls)
	}
	wd, _ := os.Getwd()
	if !sameDir(t, wd, root) {
		t.Errorf("cwd = %s, want the project root %s restored", wd, root)
	}
}

func TestRunCommandToDeployReturnsPreflightError(t *testing.T) {
	root := deployFixture(t, "# {{env}}\n", "")
	cause := errors.New("fake pre-flight failure: credentials expired")
	stubPreflight(t, cause)

	var err error
	captureOutput(t, func() { err = runCommandToDeploy("dev") })

	var se *deployStepError
	if !errors.As(err, &se) {
		t.Fatalf("want a *deployStepError, got %T: %v", err, err)
	}
	if se.Step != "preflight" {
		t.Errorf("step = %q, want preflight", se.Step)
	}
	if !errors.Is(err, cause) {
		t.Errorf("the pre-flight cause was not wrapped: %v", err)
	}
	wd, _ := os.Getwd()
	if !sameDir(t, wd, root) {
		t.Errorf("cwd = %s, want the project root %s restored", wd, root)
	}
}

func TestReportDeployFailureOffersHandoffOnA(t *testing.T) {
	target := handoffTarget{
		ProjectRoot: "/fake/project",
		EnvName:     "dev",
		Env:         Env{Env: "dev", AWSProfile: "fake-profile", Region: "eu-west-1", AccountID: "000000000000"},
	}
	err := &deployStepError{Step: "dns", Err: errors.New("fake zone not delegated")}

	var got []HandoffContext
	out := captureOutput(t, func() {
		reportDeployFailure(err, target, strings.NewReader("a\n"), func(c HandoffContext) { got = append(got, c) })
	})

	if len(got) != 1 {
		t.Fatalf("offer called %d times, want 1", len(got))
	}
	c := got[0]
	if c.Operation != "dns" || c.ProjectRoot != "/fake/project" || c.Env != "dev" ||
		c.AWSProfile != "fake-profile" || c.AWSRegion != "eu-west-1" || c.AWSAccountID != "000000000000" {
		t.Errorf("context = %+v", c)
	}
	if !strings.Contains(c.ErrorOutput, "fake zone not delegated") {
		t.Errorf("ErrorOutput lacks the cause: %q", c.ErrorOutput)
	}
	if c.Goal == "" {
		t.Error("Goal is empty")
	}
	for _, want := range []string{"fake zone not delegated", "[a] Fix with AI (Claude Code)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestReportDeployFailureEnterContinuesWithoutHandoff(t *testing.T) {
	err := &deployStepError{Step: "generate", Err: errors.New("fake render failure")}
	called := false
	captureOutput(t, func() {
		reportDeployFailure(err, handoffTarget{ProjectRoot: "/fake"}, strings.NewReader("\n"), func(HandoffContext) { called = true })
	})
	if called {
		t.Error("Enter launched the handoff")
	}
}

// Plan and apply failures already offered the handoff on their own error
// screen; asking a second time after the TUI closes would be noise.
func TestReportDeployFailureDoesNotPromptForTerraformErrors(t *testing.T) {
	called := false
	out := captureOutput(t, func() {
		reportDeployFailure(errors.New("terraform plan failed: fake"), handoffTarget{ProjectRoot: "/fake"},
			strings.NewReader("a\n"), func(HandoffContext) { called = true })
	})
	if called {
		t.Error("handoff offered for a terraform error the TUI already handled")
	}
	if !strings.Contains(out, "terraform plan failed: fake") {
		t.Errorf("error not printed:\n%s", out)
	}
}

// End to end through the interactive entry: the project root in the handoff is
// the directory the deploy started in, and the env details come from the YAML.
func TestDeployInteractiveHandsPreflightFailureToHandoff(t *testing.T) {
	root := deployFixture(t, "# {{env}}\n", "")
	stubPreflight(t, errors.New("fake pre-flight failure"))

	var got []HandoffContext
	captureOutput(t, func() {
		deployInteractive("dev", strings.NewReader("a\n"), func(c HandoffContext) { got = append(got, c) })
	})

	if len(got) != 1 {
		t.Fatalf("offer called %d times, want 1", len(got))
	}
	c := got[0]
	if c.Operation != "preflight" {
		t.Errorf("Operation = %q, want preflight", c.Operation)
	}
	if !sameDir(t, c.ProjectRoot, root) {
		t.Errorf("ProjectRoot = %q, want %q", c.ProjectRoot, root)
	}
	if c.AWSAccountID != "000000000000" || c.AWSRegion != "ap-southeast-2" || c.AWSProfile != "none" {
		t.Errorf("env details not taken from the YAML: %+v", c)
	}
	wd, _ := os.Getwd()
	if !sameDir(t, wd, root) {
		t.Errorf("cwd = %s, want %s", wd, root)
	}
}

func TestHandleGenerateCommandFailsWithExitCodeOne(t *testing.T) {
	deployFixture(t, "# {{env}}\n", "env: [unclosed\n")

	var err error
	out := captureOutput(t, func() {
		err = handleGenerateCommand([]string{"dev"})
	})
	if err == nil {
		t.Fatalf("handleGenerateCommand returned nil for a broken YAML:\n%s", out)
	}

	var code int
	out = captureOutput(t, func() { code = cliExitCode(err) })
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "Error:") {
		t.Errorf("the error was not printed: %q", out)
	}
}

// applyDNSOutcome sits on the deploy's DNS path. It used to os.Exit(1) when
// it could not disable the domain and os.Exit(0) on a cancel, ending the
// whole interactive session either way.
func TestApplyDNSOutcomeReturnsErrorWhenDomainCannotBeDisabled(t *testing.T) {
	chdir(t, t.TempDir()) // no dev.yaml: there is nothing to flip

	var e Env
	var err error
	captureOutput(t, func() { _, err = applyDNSOutcome("dev", &e, dnsSetupOutcome{SkipDomain: true}) })

	if err == nil {
		t.Fatal("applyDNSOutcome returned nil when the domain could not be disabled")
	}
	if errors.Is(err, errDeployStopped) {
		t.Error("a failure was reported as a deliberate stop")
	}
}

func TestApplyDNSOutcomeCancelStopsWithoutFailing(t *testing.T) {
	chdir(t, t.TempDir())

	var e Env
	var err error
	captureOutput(t, func() { _, err = applyDNSOutcome("dev", &e, dnsSetupOutcome{}) })

	if !errors.Is(err, errDeployStopped) {
		t.Fatalf("want errDeployStopped for a cancel, got %v", err)
	}
}

// A stop the user chose is not a failure: no error, no handoff prompt.
func TestDeployStoppedIsNotReportedAsAFailure(t *testing.T) {
	called := false
	out := captureOutput(t, func() {
		reportDeployFailure(stepErr("dns", errDeployStopped), handoffTarget{ProjectRoot: "/fake"},
			strings.NewReader("a\n"), func(HandoffContext) { called = true })
	})
	if called || strings.Contains(out, "❌") {
		t.Errorf("a deliberate stop was reported as a failure (offered=%v):\n%s", called, out)
	}
}

func TestCLIExitCodeIsZeroOnSuccess(t *testing.T) {
	if code := cliExitCode(nil); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

// A failed Getwd used to be dropped, so the only later symptom was "project
// root is not set". Its cause is printed when it happens.
func TestDeployProjectRoot_PrintsTheGetwdCause(t *testing.T) {
	var out strings.Builder
	root := deployProjectRoot(func() (string, error) { return "", errors.New("fake: getwd failed") }, &out)

	if root != "" {
		t.Errorf("root = %q, want empty", root)
	}
	if !strings.Contains(out.String(), "fake: getwd failed") {
		t.Errorf("output = %q, want the Getwd cause", out.String())
	}
}

func TestDeployProjectRoot_ReturnsTheDirectorySilently(t *testing.T) {
	var out strings.Builder
	root := deployProjectRoot(func() (string, error) { return "/projects/fake-app", nil }, &out)

	if root != "/projects/fake-app" || out.Len() != 0 {
		t.Errorf("root = %q, output = %q; want the directory and no output", root, out.String())
	}
}
