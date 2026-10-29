package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// All values here are synthetic. This repository is public.

// sampleTarget is what runCommandToDeploy captures before it changes into
// env/<env>: the project root and the loaded Env.
func sampleTarget(root string) handoffTarget {
	return handoffTarget{
		ProjectRoot: root,
		EnvName:     "staging",
		Env: Env{
			Project:    "example",
			Env:        "staging",
			Region:     "eu-west-1",
			AccountID:  "000000000000",
			AWSProfile: "example-profile",
		},
	}
}

// The shell's AWS variables name whatever the user last exported, which need
// not be the profile meroku deployed with. The briefing must only ever carry
// the values from the env YAML.
func setMisleadingShellAWS(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_PROFILE", "shell-profile-should-not-appear")
	t.Setenv("AWS_REGION", "ap-southeast-9")
	t.Setenv("AWS_DEFAULT_REGION", "ap-southeast-9")
}

func assertWhereItRan(t *testing.T, ctx HandoffContext, root string) {
	t.Helper()
	if ctx.Env != "staging" {
		t.Errorf("Env = %q, want staging", ctx.Env)
	}
	if ctx.ProjectRoot != root {
		t.Errorf("ProjectRoot = %q, want %q", ctx.ProjectRoot, root)
	}
	if ctx.AWSProfile != "example-profile" {
		t.Errorf("AWSProfile = %q, want example-profile (from the env YAML, not $AWS_PROFILE)", ctx.AWSProfile)
	}
	if ctx.AWSRegion != "eu-west-1" {
		t.Errorf("AWSRegion = %q, want eu-west-1 (from the env YAML, not $AWS_REGION)", ctx.AWSRegion)
	}
	if ctx.AWSAccountID != "000000000000" {
		t.Errorf("AWSAccountID = %q, want 000000000000", ctx.AWSAccountID)
	}
	if strings.TrimSpace(ctx.Goal) == "" {
		t.Error("Goal must be set")
	}
	all := ctx.Env + ctx.AWSProfile + ctx.AWSRegion + ctx.ErrorOutput + strings.Join(ctx.Diagnostics, "\n")
	if strings.Contains(all, "shell-profile-should-not-appear") || strings.Contains(all, "ap-southeast-9") {
		t.Error("handoff context leaked the shell's AWS_PROFILE / AWS_REGION")
	}
}

func TestHandoffTarget_EnvNameFallsBackWhenYAMLHasNoEnvField(t *testing.T) {
	target := handoffTarget{ProjectRoot: "/tmp/fake-project", EnvName: "dev"}
	ctx := target.context("menu", "goal")
	if ctx.Env != "dev" {
		t.Errorf("Env = %q, want dev", ctx.Env)
	}
}

// --- apply ------------------------------------------------------------------

// failedApplyModel is an apply that finished with one real failure, one
// resource cancelled because of it, and an address-less backend diagnostic.
func failedApplyModel(root string) *modernPlanModel {
	m := newApplyViewModel(true)
	m.target = sampleTarget(root)
	m.applyState.hasErrors = true
	m.applyState.errorCount = 1
	m.applyState.diagnostics["aws_lb_target_group.service_api"] = &DiagnosticInfo{
		Severity: "error",
		Summary:  "expected length of name to be in the range (1 - 32)",
		Detail:   "EntityAlreadyExists: a target group with this name already exists",
		Address:  "aws_lb_target_group.service_api",
		Range:    &DiagnosticRange{Filename: "../../infrastructure/modules/workloads/services.tf"},
	}
	m.applyState.diagnostics["aws_lb_target_group.service_api"].Range.Start.Line = 42
	m.applyState.completed = []completedResource{
		{Address: "aws_ecs_cluster.main", Action: "create", Success: true},
		{
			Address: "aws_lb_target_group.service_api",
			Action:  "create",
			Success: false,
			Error:   "aws_lb_target_group.service_api: Creation errored after 1s",
			// Summary/Detail deliberately empty: the back-fill raced, so the
			// diagnostics map is the only place the text survived.
		},
		{
			Address: "aws_ecs_service.api",
			Action:  "cancelled",
			Success: false,
			Error:   "Deployment cancelled due to previous errors",
		},
	}
	m.applyState.logs = []logEntry{
		{Level: "info", Message: "Starting terraform apply with 3 resources"},
		{Level: "error", Message: "Error: error configuring S3 Backend: no valid credential sources found", IsDiagnostic: true},
		{Level: "error", Message: "Error: expected length of name", Resource: "aws_lb_target_group.service_api", IsDiagnostic: true},
	}
	return m
}

func TestApplyHandoffContext_UsesThreadedEnvNotShell(t *testing.T) {
	setMisleadingShellAWS(t)
	root := t.TempDir()

	ctx := failedApplyModel(root).handoffContext()

	if ctx.Operation != "apply" {
		t.Errorf("Operation = %q, want apply", ctx.Operation)
	}
	assertWhereItRan(t, ctx, root)
}

func TestApplyHandoffContext_IncludesFailuresFromDiagnosticsMap(t *testing.T) {
	ctx := failedApplyModel(t.TempDir()).handoffContext()

	if len(ctx.Failures) != 1 {
		t.Fatalf("want exactly 1 failure (cancelled cascade excluded), got %d: %+v", len(ctx.Failures), ctx.Failures)
	}
	f := ctx.Failures[0]
	if f.Address != "aws_lb_target_group.service_api" || f.Action != "create" {
		t.Errorf("failure = %+v", f)
	}
	if !strings.Contains(f.Summary, "range (1 - 32)") {
		t.Errorf("Summary must come from the diagnostics map, got %q", f.Summary)
	}
	if !strings.Contains(f.Detail, "EntityAlreadyExists") {
		t.Errorf("Detail must come from the diagnostics map, got %q", f.Detail)
	}
	if !strings.Contains(f.Detail, "already exists in the AWS account") {
		t.Errorf("Detail must carry meroku's collision hint, got %q", f.Detail)
	}
	if f.Location != "../../infrastructure/modules/workloads/services.tf:42" {
		t.Errorf("Location = %q", f.Location)
	}
}

func TestApplyHandoffContext_ExcludesCancelledCascade(t *testing.T) {
	ctx := failedApplyModel(t.TempDir()).handoffContext()
	for _, f := range ctx.Failures {
		if f.Address == "aws_ecs_service.api" {
			t.Errorf("a resource cancelled due to previous errors is not a failure: %+v", f)
		}
	}
}

func TestApplyHandoffContext_IncludesAddresslessDiagnostics(t *testing.T) {
	ctx := failedApplyModel(t.TempDir()).handoffContext()

	joined := strings.Join(ctx.Diagnostics, "\n")
	if !strings.Contains(joined, "error configuring S3 Backend") {
		t.Errorf("address-less diagnostic missing from Diagnostics: %q", ctx.Diagnostics)
	}
	if strings.Contains(joined, "Starting terraform apply") {
		t.Errorf("ordinary log lines are not diagnostics: %q", ctx.Diagnostics)
	}
}

// Addendum 3: the apply path used to drop terraform's stderr and the exit
// error carried by applyErrorMsg. Both must reach the briefing.
func TestApplyHandoffContext_RetainsStderrAndExitCause(t *testing.T) {
	m := failedApplyModel(t.TempDir())
	m.applyState.applyComplete = false
	m.applyState.isApplying = true

	m.Update(applyErrorMsg{
		err:    errors.New("exit status 1"),
		stderr: "Error: Failed to load plugin schemas (fake)\n",
	})

	ctx := m.handoffContext()
	if !strings.Contains(ctx.ErrorOutput, "exit status 1") {
		t.Errorf("ErrorOutput must carry the applyErrorMsg cause, got %q", ctx.ErrorOutput)
	}
	if !strings.Contains(ctx.ErrorOutput, "Failed to load plugin schemas (fake)") {
		t.Errorf("ErrorOutput must carry terraform stderr, got %q", ctx.ErrorOutput)
	}
}

// --- apply keys -------------------------------------------------------------

func TestApplyErrorScreen_AKeyRequestsClaudeAndQuits(t *testing.T) {
	m := failedApplyModel(t.TempDir())

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})

	if !m.launchClaude {
		t.Error("a on a failed apply must set launchClaude")
	}
	if !isQuit(cmd) {
		t.Error("a on a failed apply must quit the viewer so the caller can hand off")
	}
}

func TestApplySuccessScreen_AKeyDoesNothing(t *testing.T) {
	m := newApplyViewModel(true)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})

	if m.launchClaude {
		t.Error("a after a successful apply must not hand off")
	}
	if isQuit(cmd) {
		t.Error("a after a successful apply must not quit")
	}
	if m.currentView != applyView {
		t.Errorf("a after a successful apply must stay on the apply view, got %v", m.currentView)
	}
}

func TestDashboard_AKeyStillStartsApply(t *testing.T) {
	m := &modernPlanModel{
		keys:        modernKeys,
		currentView: dashboardView,
		width:       160,
		height:      50,
	}

	// The returned command would run terraform; it is deliberately not run.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})

	if m.launchClaude {
		t.Error("a on the plan dashboard is Apply, not the Claude handoff")
	}
	if m.currentView != applyView || m.applyState == nil {
		t.Error("a on the plan dashboard must switch to the apply view")
	}
	if cmd == nil {
		t.Error("a on the plan dashboard must return the apply command")
	}
}

func TestApplyErrorScreen_SAndEDoNothingAI(t *testing.T) {
	for _, k := range []string{"s", "e"} {
		m := failedApplyModel(t.TempDir())

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})

		if m.launchClaude {
			t.Errorf("%s must not hand off to Claude", k)
		}
		if isQuit(cmd) {
			t.Errorf("%s on a failed apply must not quit", k)
		}
		if m.currentView != applyView {
			t.Errorf("%s must leave the apply view alone, got %v", k, m.currentView)
		}
	}
}

func TestApplyFooter_AdvertisesClaudeOnlyOnFailure(t *testing.T) {
	failed := failedApplyModel(t.TempDir()).renderApplyFooter()
	if !strings.Contains(failed, "[a] Fix with AI (Claude Code)") {
		t.Errorf("failed-apply footer missing the handoff key: %q", failed)
	}
	if !strings.Contains(failed, "[Enter] Continue") {
		t.Errorf("failed-apply footer must keep [Enter] Continue: %q", failed)
	}
	for _, gone := range []string{"Solve with AI", "AI Help"} {
		if strings.Contains(failed, gone) {
			t.Errorf("footer still advertises %q: %q", gone, failed)
		}
	}

	ok := newApplyViewModel(true).renderApplyFooter()
	if strings.Contains(ok, "[a]") {
		t.Errorf("successful-apply footer must not offer the handoff: %q", ok)
	}
}

// --- plan progress ----------------------------------------------------------

func failedPlanModel(root string) *planProgressModel {
	m := newPlanProgressModel(sampleTarget(root))
	m.width, m.height = 160, 50
	m.phase = phaseError
	m.errorDetails = "exit status 1"
	m.errorOutput = []string{
		"╷",
		"│ Error: Unsupported argument",
		"│ ",
		"│   on main.tf line 12, in module \"workloads\":",
		"╵",
	}
	return m
}

func TestPlanHandoffContext_UsesThreadedEnvAndKeepsOutput(t *testing.T) {
	setMisleadingShellAWS(t)
	root := t.TempDir()

	ctx := failedPlanModel(root).handoffContext()

	if ctx.Operation != "plan" {
		t.Errorf("Operation = %q, want plan", ctx.Operation)
	}
	assertWhereItRan(t, ctx, root)
	if !strings.Contains(ctx.ErrorOutput, "exit status 1") || !strings.Contains(ctx.ErrorOutput, "on main.tf line 12") {
		t.Errorf("ErrorOutput must hold the error and stderr, got %q", ctx.ErrorOutput)
	}
	if len(ctx.Diagnostics) != 1 || ctx.Diagnostics[0] != "Error: Unsupported argument" {
		t.Errorf("Diagnostics = %q, want the one Error: line", ctx.Diagnostics)
	}
}

func TestPlanErrorScreen_AKeyRequestsClaudeAndQuits(t *testing.T) {
	m := failedPlanModel(t.TempDir())

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})

	if !m.launchClaude || !isQuit(cmd) {
		t.Errorf("a in the plan error phase: launchClaude=%v quit=%v, want both", m.launchClaude, isQuit(cmd))
	}
}

func TestPlanRunning_AKeyDoesNothing(t *testing.T) {
	m := failedPlanModel(t.TempDir())
	m.phase = phasePlanning

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})

	if m.launchClaude || isQuit(cmd) {
		t.Error("a while the plan is running must do nothing")
	}
}

func TestPlanErrorFooter_AdvertisesClaude(t *testing.T) {
	view := failedPlanModel(t.TempDir()).renderError()
	if !strings.Contains(view, "[a] Fix with AI (Claude Code)") {
		t.Errorf("plan error screen missing the handoff key:\n%s", view)
	}
}

func TestPlanRunResult_HandedOffErrorIsMarked(t *testing.T) {
	m := failedPlanModel(t.TempDir())
	m.launchClaude = true
	offered := 0

	err := planRunResult(m, func(HandoffContext) { offered++ })

	if offered != 1 {
		t.Errorf("handoff offered %d times, want 1", offered)
	}
	if !errors.Is(err, errHandedOff) {
		t.Errorf("err = %v, want it to wrap errHandedOff", err)
	}
}

func TestPlanRunResult_PlainFailureIsNotMarked(t *testing.T) {
	m := failedPlanModel(t.TempDir())

	err := planRunResult(m, func(HandoffContext) { t.Error("offered without [a]") })

	if err == nil || errors.Is(err, errHandedOff) {
		t.Errorf("err = %v, want a plan error that is not errHandedOff", err)
	}
}

// A plan the user handed to Claude must not be followed by meroku's own
// init/-reconfigure recovery, even when its output names `terraform init`.
func TestRunTerraformApply_HandedOffPlanSkipsRecovery(t *testing.T) {
	origPlan, origRecovery := applyPlanStep, applyPlanRecovery
	t.Cleanup(func() { applyPlanStep, applyPlanRecovery = origPlan, origRecovery })
	applyPlanStep = func(handoffTarget) error {
		return fmt.Errorf("terraform plan failed: please run \"terraform init\": %w", errHandedOff)
	}
	recoveries := 0
	applyPlanRecovery = func(string) ([]string, error) {
		recoveries++
		return nil, errors.New("fake: no recovery")
	}

	err := runTerraformApply(sampleTarget(t.TempDir()))

	if recoveries != 0 {
		t.Errorf("recovery consulted %d times after a handoff, want 0", recoveries)
	}
	if !errors.Is(err, errHandedOff) {
		t.Errorf("err = %v, want it to wrap errHandedOff", err)
	}
}

func TestRunTerraformApply_PlainPlanFailureStillTriesRecovery(t *testing.T) {
	origPlan, origRecovery := applyPlanStep, applyPlanRecovery
	t.Cleanup(func() { applyPlanStep, applyPlanRecovery = origPlan, origRecovery })
	applyPlanStep = func(handoffTarget) error { return errors.New("terraform plan failed: boom") }
	recoveries := 0
	applyPlanRecovery = func(string) ([]string, error) {
		recoveries++
		return nil, errors.New("fake: no recovery")
	}

	if err := runTerraformApply(sampleTarget(t.TempDir())); err == nil {
		t.Fatal("want an error")
	}
	if recoveries != 1 {
		t.Errorf("recovery consulted %d times, want 1", recoveries)
	}
}

// --- destroy ----------------------------------------------------------------

func failedDestroyModel(root string) *destroyProgressModel {
	m := newDestroyProgressModel(sampleTarget(root))
	m.width, m.height = 160, 50
	m.phase = destroyError
	m.errorDetails = "exit status 1"
	m.errorOutput = []string{"Error: deleting S3 Bucket (example-bucket-fake): BucketNotEmpty"}
	return m
}

func TestDestroyHandoffContext_UsesThreadedEnvAndKeepsOutput(t *testing.T) {
	setMisleadingShellAWS(t)
	root := t.TempDir()

	ctx := failedDestroyModel(root).handoffContext()

	if ctx.Operation != "destroy" {
		t.Errorf("Operation = %q, want destroy", ctx.Operation)
	}
	assertWhereItRan(t, ctx, root)
	if !strings.Contains(ctx.ErrorOutput, "BucketNotEmpty") {
		t.Errorf("ErrorOutput = %q", ctx.ErrorOutput)
	}
	if len(ctx.Diagnostics) != 1 || !strings.Contains(ctx.Diagnostics[0], "BucketNotEmpty") {
		t.Errorf("Diagnostics = %q", ctx.Diagnostics)
	}
}

func TestDestroyErrorScreen_AKeyRequestsClaudeAndQuits(t *testing.T) {
	m := failedDestroyModel(t.TempDir())

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})

	if !m.launchClaude || !isQuit(cmd) {
		t.Errorf("a on the destroy error screen: launchClaude=%v quit=%v, want both", m.launchClaude, isQuit(cmd))
	}
}

func TestDestroyErrorScreen_OtherKeysDoNotHandOff(t *testing.T) {
	m := failedDestroyModel(t.TempDir())

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})

	if m.launchClaude {
		t.Error("only a hands off to Claude")
	}
}

func TestDestroyErrorFooter_AdvertisesClaude(t *testing.T) {
	view := failedDestroyModel(t.TempDir()).View()
	if !strings.Contains(view, "[a] Fix with AI (Claude Code)") {
		t.Errorf("destroy error screen missing the handoff key:\n%s", view)
	}
}

func TestDestroyRunResult_HandedOffErrorIsMarked(t *testing.T) {
	m := failedDestroyModel(t.TempDir())
	m.launchClaude = true
	offered := 0

	err := destroyRunResult(m, func(HandoffContext) { offered++ })

	if offered != 1 {
		t.Errorf("handoff offered %d times, want 1", offered)
	}
	if !errors.Is(err, errHandedOff) {
		t.Errorf("err = %v, want it to wrap errHandedOff", err)
	}
}

func TestDestroyRunResult_PlainFailureIsNotMarked(t *testing.T) {
	err := destroyRunResult(failedDestroyModel(t.TempDir()), func(HandoffContext) { t.Error("offered without [a]") })

	if err == nil || errors.Is(err, errHandedOff) {
		t.Errorf("err = %v, want a destroy error that is not errHandedOff", err)
	}
}

// --- handoffTargetFromProjectRoot -------------------------------------------

// The menu and nuke handoffs read the env YAML for profile and region. That
// must be a read: a migrating loader would rewrite the file (and leave a
// backup) on the way to a briefing.
func TestHandoffTargetFromProjectRoot_DoesNotMigrateTheYAML(t *testing.T) {
	root := t.TempDir()
	stale := "project: example\nenv: staging\nregion: eu-west-1\naccount_id: \"000000000000\"\naws_profile: example-profile\n"
	yamlPath := filepath.Join(root, "staging.yaml")
	if err := os.WriteFile(yamlPath, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, root)

	target := handoffTargetFromProjectRoot("staging")

	got, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != stale {
		t.Errorf("env YAML was rewritten:\n%s", got)
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if strings.Contains(e.Name(), "backup") {
			t.Errorf("a backup %q was written, so the YAML was migrated", e.Name())
		}
	}
	if target.Env.AWSProfile != "example-profile" || target.Env.Region != "eu-west-1" {
		t.Errorf("target.Env = %+v, want the profile and region from the YAML", target.Env)
	}
	if d := target.context("menu", menuHandoffGoal).Diagnostics; len(d) != 0 {
		t.Errorf("a readable YAML must add no diagnostics, got %q", d)
	}
}

func TestHandoffTargetFromProjectRoot_UnreadableYAMLIsSaidInTheBriefing(t *testing.T) {
	root := t.TempDir()
	chdir(t, root)

	ctx := menuHandoffContext(handoffTargetFromProjectRoot("staging"))

	if len(ctx.Diagnostics) != 1 {
		t.Fatalf("Diagnostics = %q, want one line about the env YAML", ctx.Diagnostics)
	}
	d := ctx.Diagnostics[0]
	if !strings.Contains(d, "staging") || !strings.Contains(d, "could not be read") || !strings.Contains(d, "not found") {
		t.Errorf("diagnostic = %q, want it to name the env, say it could not be read, and why", d)
	}
}

// The destroy screen's own diagnostics must not overwrite the YAML note.
func TestDestroyHandoffContext_KeepsTheEnvYAMLNote(t *testing.T) {
	m := failedDestroyModel(t.TempDir())
	m.target.EnvLoadError = "env YAML for 'staging' could not be read: fake reason"

	ctx := m.handoffContext()

	joined := strings.Join(ctx.Diagnostics, "\n")
	if !strings.Contains(joined, "fake reason") || !strings.Contains(joined, "BucketNotEmpty") {
		t.Errorf("Diagnostics = %q, want both the YAML note and Terraform's error", ctx.Diagnostics)
	}
}

// --- menu and SSO -----------------------------------------------------------

func TestMenuHandoffContext_AsksTheUser(t *testing.T) {
	setMisleadingShellAWS(t)
	root := t.TempDir()

	ctx := menuHandoffContext(sampleTarget(root))

	if ctx.Operation != "menu" {
		t.Errorf("Operation = %q, want menu", ctx.Operation)
	}
	assertWhereItRan(t, ctx, root)
	if !strings.Contains(strings.ToLower(ctx.Goal), "ask the user") {
		t.Errorf("menu Goal must tell Claude to ask the user what is wrong, got %q", ctx.Goal)
	}
}

func TestSSOHandoffContext_DescribesTheProfileProblem(t *testing.T) {
	setMisleadingShellAWS(t)
	root := t.TempDir()
	info := &ProfileInfo{
		Name:          "example-profile",
		Exists:        true,
		Type:          ProfileTypeModernSSO,
		MissingFields: []string{"sso_account_id", "sso_role_name"},
	}

	ctx := ssoHandoffContext(sampleTarget(root), "example-profile", info,
		"/home/example/.aws/config", []string{"Profile 'example-profile' is incomplete"})

	if ctx.Operation != "sso" {
		t.Errorf("Operation = %q, want sso", ctx.Operation)
	}
	assertWhereItRan(t, ctx, root)
	if !strings.Contains(ctx.Goal, "AWS SSO") {
		t.Errorf("Goal = %q", ctx.Goal)
	}
	text := ctx.ErrorOutput + "\n" + strings.Join(ctx.Diagnostics, "\n")
	for _, want := range []string{"example-profile", "sso_account_id", "sso_role_name",
		"/home/example/.aws/config", "is incomplete"} {
		if !strings.Contains(text, want) {
			t.Errorf("SSO context missing %q:\n%s", want, text)
		}
	}
}

func TestSSOHandoffContext_MissingProfile(t *testing.T) {
	ctx := ssoHandoffContext(sampleTarget(t.TempDir()), "example-profile",
		&ProfileInfo{Name: "example-profile"}, "/home/example/.aws/config", nil)

	if !strings.Contains(strings.Join(ctx.Diagnostics, "\n"), "does not exist") {
		t.Errorf("a missing profile must be stated, got %q", ctx.Diagnostics)
	}
}
