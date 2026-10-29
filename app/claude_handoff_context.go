package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// handoffTarget is the environment a run belongs to and the project it lives
// in. It is captured at the project root, before meroku changes into
// env/<env>, and threaded down to every TUI that can offer the Claude Code
// handoff — so the briefing never has to guess the env from the working
// directory or the profile from $AWS_PROFILE.
type handoffTarget struct {
	// ProjectRoot is the absolute project directory.
	ProjectRoot string
	// EnvName is the name the env was selected by; used when the YAML has no
	// env field.
	EnvName string
	// Env is the loaded env YAML. Zero when it could not be loaded.
	Env Env
	// EnvLoadError says why the env YAML could not be read, for the briefing.
	// Empty when it was read.
	EnvLoadError string
}

// context returns a HandoffContext describing where the run happened. The
// caller adds what failed.
func (t handoffTarget) context(operation, goal string) HandoffContext {
	name := t.Env.Env
	if name == "" {
		name = t.EnvName
	}
	ctx := HandoffContext{
		Operation:    operation,
		Goal:         goal,
		Env:          name,
		ProjectRoot:  t.ProjectRoot,
		AWSProfile:   t.Env.AWSProfile,
		AWSRegion:    t.Env.Region,
		AWSAccountID: t.Env.AccountID,
	}
	if t.EnvLoadError != "" {
		ctx.Diagnostics = []string{t.EnvLoadError}
	}
	return ctx
}

// handoffTargetFromProjectRoot builds a target for code that runs at the
// project root (menus, nuke). The env YAML is only read, never migrated or
// rewritten. A YAML that cannot be read still yields a target: the briefing
// says the profile and region were not recorded, and one diagnostic says why.
func handoffTargetFromProjectRoot(envName string) handoffTarget {
	t := handoffTarget{EnvName: envName}
	if root, err := os.Getwd(); err == nil {
		t.ProjectRoot = root
	}
	if envName != "" {
		e, err := peekEnvErr(envName)
		t.Env = e
		if err != nil {
			t.EnvLoadError = fmt.Sprintf("meroku: the env YAML for '%s' could not be read: %v", envName, err)
		}
	}
	return t
}

const (
	applyHandoffGoal = "Find out why `terraform apply` failed for this environment, explain the cause, " +
		"and propose a fix that follows the rules below."
	planHandoffGoal = "Find out why `terraform plan` failed for this environment, explain the cause, " +
		"and propose a fix that follows the rules below."
	destroyHandoffGoal = "Find out why `terraform destroy` failed for this environment, explain the cause, " +
		"and propose a fix that follows the rules below."
	menuHandoffGoal = "The user opened this from meroku's main menu. Ask the user what is wrong, " +
		"then investigate it in this project and explain what you find before proposing any change."
	ssoHandoffGoal = "Help fix and investigate the AWS SSO/profile setup for this project. " +
		"Explain what is missing or wrong in the AWS config before proposing any change."
)

// isCascadingCancellation reports whether a failed resource only failed
// because an earlier one did. Those are not causes and stay out of the
// briefing, as they stay out of the error count.
func isCascadingCancellation(res completedResource) bool {
	errText := strings.ToLower(res.Error)
	return res.Action == "cancelled" ||
		strings.Contains(errText, "cancelled due to previous errors") ||
		strings.Contains(errText, "deployment cancelled")
}

// handoffContext describes a failed apply: every failed resource, every
// diagnostic Terraform did not tie to one of them, and the exit cause and
// stderr the apply would otherwise drop.
func (m *modernPlanModel) handoffContext() HandoffContext {
	ctx := m.target.context("apply", applyHandoffGoal)
	if m.applyState == nil {
		return ctx
	}

	failed := map[string]bool{}
	for _, res := range m.applyState.completed {
		if res.Success || isCascadingCancellation(res) {
			continue
		}
		failed[res.Address] = true

		summary := m.errorSummaryFor(res)
		detail := m.errorDetailFor(res)
		if summary == "" && detail == "" {
			summary = res.Error
		}
		if hint := collisionHint(summary + " " + detail + " " + res.Error); hint != "" {
			if detail != "" {
				detail += "\n\n"
			}
			detail += "meroku's hint: " + hint
		}

		ctx.Failures = append(ctx.Failures, HandoffFailure{
			Address:  res.Address,
			Action:   res.Action,
			Summary:  summary,
			Detail:   detail,
			Location: m.diagnosticLocation(res.Address),
		})
	}

	for _, log := range m.applyState.logs {
		if !log.IsDiagnostic || failed[log.Resource] {
			continue
		}
		line := log.Message
		if log.Resource != "" {
			line = log.Resource + ": " + line
		}
		ctx.Diagnostics = append(ctx.Diagnostics, line)
	}

	var out []string
	if m.applyState.failureCause != "" {
		out = append(out, "terraform apply exited: "+m.applyState.failureCause)
	}
	if s := strings.TrimSpace(m.applyState.stderrOutput); s != "" {
		out = append(out, "terraform stderr:\n"+s)
	}
	ctx.ErrorOutput = strings.Join(out, "\n\n")
	return ctx
}

// diagnosticLocation is "<file>:<line>" for the diagnostic Terraform reported
// against address, or "" when it gave no source range.
func (m *modernPlanModel) diagnosticLocation(address string) string {
	m.applyState.mu.Lock()
	d := m.applyState.diagnostics[address]
	m.applyState.mu.Unlock()
	if d == nil || d.Range == nil || d.Range.Filename == "" {
		return ""
	}
	if d.Range.Start.Line > 0 {
		return fmt.Sprintf("%s:%d", d.Range.Filename, d.Range.Start.Line)
	}
	return d.Range.Filename
}

// handoffContext describes a failed `terraform plan`.
func (m *planProgressModel) handoffContext() HandoffContext {
	ctx := m.target.context("plan", planHandoffGoal)
	m.outputMutex.Lock()
	lines := append([]string(nil), m.errorOutput...)
	m.outputMutex.Unlock()
	ctx.Diagnostics = append(ctx.Diagnostics, terraformTextDiagnostics(lines)...)
	ctx.ErrorOutput = joinErrorOutput(m.errorDetails, lines)
	return ctx
}

// handoffContext describes a failed `terraform destroy`.
func (m *destroyProgressModel) handoffContext() HandoffContext {
	ctx := m.target.context("destroy", destroyHandoffGoal)
	m.outputMutex.Lock()
	lines := append([]string(nil), m.errorOutput...)
	m.outputMutex.Unlock()
	ctx.Diagnostics = append(ctx.Diagnostics, terraformTextDiagnostics(lines)...)
	ctx.ErrorOutput = joinErrorOutput(m.errorDetails, lines)
	return ctx
}

func joinErrorOutput(cause string, lines []string) string {
	var parts []string
	if cause = strings.TrimSpace(cause); cause != "" {
		parts = append(parts, cause)
	}
	if body := strings.TrimSpace(strings.Join(lines, "\n")); body != "" {
		parts = append(parts, body)
	}
	return strings.Join(parts, "\n\n")
}

// terraformTextDiagnostics picks the "Error: ..." / "Warning: ..." headline
// lines out of Terraform's human-readable output, stripping the box drawing
// it puts around each diagnostic.
func terraformTextDiagnostics(lines []string) []string {
	var out []string
	for _, l := range lines {
		l = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "│╷╵|"))
		l = strings.TrimPrefix(l, "ERROR: ")
		if strings.HasPrefix(l, "Error:") || strings.HasPrefix(l, "Warning:") {
			out = append(out, l)
		}
	}
	return out
}

// menuHandoffContext is the handoff started from the main menu, with nothing
// failed yet: Claude is told to ask the user.
func menuHandoffContext(t handoffTarget) HandoffContext {
	return t.context("menu", menuHandoffGoal)
}

// ssoHandoffContext is the handoff for a broken AWS profile. It carries the
// profile name, what the inspector found missing and where the AWS config
// lives. ProfileInfo holds no credentials, and none are read here.
func ssoHandoffContext(t handoffTarget, profile string, info *ProfileInfo, configPath string, problems []string) HandoffContext {
	ctx := t.context("sso", ssoHandoffGoal)
	if profile != "" {
		ctx.AWSProfile = profile
	}

	var diags []string
	switch {
	case info == nil:
		diags = append(diags, fmt.Sprintf("Profile '%s' could not be inspected", profile))
	case !info.Exists:
		diags = append(diags, fmt.Sprintf("Profile '%s' does not exist in the AWS config", profile))
	case len(info.MissingFields) > 0:
		diags = append(diags, fmt.Sprintf("Profile '%s' is missing required fields: %s",
			profile, strings.Join(info.MissingFields, ", ")))
	}
	if info != nil && info.SSOSessionInfo != nil && len(info.SSOSessionInfo.MissingFields) > 0 {
		diags = append(diags, fmt.Sprintf("sso-session '%s' is missing required fields: %s",
			info.SSOSessionInfo.Name, strings.Join(info.SSOSessionInfo.MissingFields, ", ")))
	}
	diags = append(diags, problems...)
	ctx.Diagnostics = append(ctx.Diagnostics, diags...)

	var out strings.Builder
	fmt.Fprintf(&out, "AWS profile: %s\n", profile)
	fmt.Fprintf(&out, "AWS config file: %s\n", configPath)
	if info != nil {
		fmt.Fprintf(&out, "Profile exists: %v\n", info.Exists)
		if info.Type != "" {
			fmt.Fprintf(&out, "Profile type: %s\n", info.Type)
		}
		if len(info.MissingFields) > 0 {
			fmt.Fprintf(&out, "Missing fields: %s\n", strings.Join(info.MissingFields, ", "))
		}
	}
	ctx.ErrorOutput = out.String()
	return ctx
}

// offerMenuHandoff is the main menu's "Fix with AI (Claude Code)". The
// profile and region are the ones this session uses: the --profile flag and
// environment selection set them, and they fall back to the env YAML.
func offerMenuHandoff() {
	t := handoffTargetFromProjectRoot(selectedEnvironment)
	if selectedAWSProfile != "" {
		t.Env.AWSProfile = selectedAWSProfile
	}
	if selectedAWSRegion != "" {
		t.Env.Region = selectedAWSRegion
	}
	offerClaudeHandoff(menuHandoffContext(t))
}

// offerSSOHandoff hands a broken AWS profile to Claude Code. It runs at the
// project root, re-inspects the profile so the briefing describes the config
// as it is now, and passes on the problems the caller already found.
func offerSSOHandoff(envName string, yamlEnv Env, profile string, problems []string) {
	t := handoffTargetFromProjectRoot("")
	t.EnvName = envName
	t.Env = yamlEnv

	var info *ProfileInfo
	if inspector, err := NewProfileInspector(); err != nil {
		problems = append(problems, fmt.Sprintf("meroku could not read the AWS config: %v", err))
	} else if pi, err := inspector.InspectProfile(profile); err != nil {
		problems = append(problems, fmt.Sprintf("meroku could not inspect profile '%s': %v", profile, err))
	} else {
		info = pi
	}

	offerClaudeHandoff(ssoHandoffContext(t, profile, info, getAWSConfigPath(), problems))
}

// offerClaudeHandoff writes the briefing and starts Claude Code beside meroku
// (or prints how to). When no pane opened, it waits for Enter so the path and
// command stay on screen until the user has copied them.
func offerClaudeHandoff(ctx HandoffContext) {
	res, err := LaunchClaudeHandoff(ctx)
	if err != nil {
		fmt.Printf("\n❌ Could not hand off to Claude Code: %v\n", err)
		return
	}
	fmt.Println()
	fmt.Print(FormatHandoffResult(res))
	if !res.PaneOpened {
		fmt.Print("\nPress Enter to continue...")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
}
