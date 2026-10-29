package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// All values here are synthetic. This repository is public.

func sampleHandoffContext(root string) HandoffContext {
	return HandoffContext{
		Operation:     "apply",
		Goal:          "fix the failed apply",
		Env:           "staging",
		ProjectRoot:   root,
		AWSProfile:    "example-profile",
		AWSRegion:     "eu-west-1",
		AWSAccountID:  "000000000000",
		MerokuVersion: "v0.0.0-test",
		Time:          time.Date(2026, 10, 29, 14, 5, 9, 0, time.UTC),
		Failures: []HandoffFailure{
			{
				Address:  "aws_lb_target_group.service_api",
				Action:   "create",
				Summary:  "name cannot be longer than 32 characters",
				Detail:   "expected length of name to be in the range (1 - 32)",
				Location: "../../infrastructure/modules/workloads/services.tf:42",
			},
			{
				Address: "aws_iam_role.backend_task",
				Action:  "update",
				Summary: "AccessDenied: not authorized to perform iam:UpdateRole",
			},
		},
		Diagnostics: []string{
			"Error: error configuring S3 Backend: no valid credential sources found",
			"Warning: provider registry.terraform.io/hashicorp/aws is deprecated-fake",
		},
		ErrorOutput: "terraform exited with status 1",
	}
}

// --- BuildBriefing -----------------------------------------------------------

func TestBuildBriefing_StatesBothProhibitions(t *testing.T) {
	b := BuildBriefing(sampleHandoffContext("/projects/fake-app"))

	for _, want := range []string{
		"Do NOT fix the problem directly in AWS",
		"Do NOT edit the generated Terraform in `env/staging/`",
	} {
		if !strings.Contains(b, want) {
			t.Errorf("briefing is missing prohibition %q", want)
		}
	}
}

func TestBuildBriefing_ListsTheThreeAllowedFixPathsInOrder(t *testing.T) {
	b := BuildBriefing(sampleHandoffContext("/projects/fake-app"))

	markers := []string{
		"(a) **The cause is a meroku bug**",
		"(b) **An edge case meroku cannot migrate by itself**",
		"(c) **A configuration problem**",
	}
	last := -1
	for _, m := range markers {
		i := strings.Index(b, m)
		if i < 0 {
			t.Fatalf("briefing is missing allowed path %q", m)
		}
		if i < last {
			t.Errorf("allowed path %q is out of order", m)
		}
		last = i
	}
	if !strings.Contains(b, "github.com/MadAppGang/meroku") {
		t.Error("path (a) must name the meroku repository")
	}
	if !strings.Contains(b, "ask before any write") {
		t.Error("briefing must say to ask before any write")
	}
}

func TestBuildBriefing_IncludesWhereItRan(t *testing.T) {
	ctx := sampleHandoffContext("/projects/fake-app")
	b := BuildBriefing(ctx)

	for _, want := range []string{
		"staging",
		"example-profile",
		"eu-west-1",
		"000000000000",
		"/projects/fake-app",
		"v0.0.0-test",
		"2026-10-29T14:05:09Z",
		"apply",
		"fix the failed apply",
	} {
		if !strings.Contains(b, want) {
			t.Errorf("briefing is missing %q", want)
		}
	}
}

func TestBuildBriefing_IncludesEveryFailureAndDiagnostic(t *testing.T) {
	ctx := sampleHandoffContext("/projects/fake-app")
	b := BuildBriefing(ctx)

	for _, f := range ctx.Failures {
		for _, want := range []string{f.Address, f.Action, f.Summary} {
			if !strings.Contains(b, want) {
				t.Errorf("briefing is missing failure field %q", want)
			}
		}
	}
	for _, want := range []string{ctx.Failures[0].Detail, ctx.Failures[0].Location} {
		if !strings.Contains(b, want) {
			t.Errorf("briefing is missing failure field %q", want)
		}
	}
	for _, d := range ctx.Diagnostics {
		if !strings.Contains(b, d) {
			t.Errorf("briefing is missing diagnostic %q", d)
		}
	}
	if !strings.Contains(b, ctx.ErrorOutput) {
		t.Error("briefing is missing the error output")
	}
}

func TestBuildBriefing_UsesRealCommandsOnly(t *testing.T) {
	b := BuildBriefing(sampleHandoffContext("/projects/fake-app"))

	for _, banned := range []string{"--generate", "merokudemo1"} {
		if strings.Contains(b, banned) {
			t.Errorf("briefing contains %q, which is not a real meroku command or path", banned)
		}
	}
	for _, want := range []string{
		"meroku generate staging",
		"infrastructure/env/main.hbs",
		"infrastructure/modules/",
		"project/staging.yaml",
	} {
		if !strings.Contains(b, want) {
			t.Errorf("briefing is missing real command/path %q", want)
		}
	}
}

func TestBuildBriefing_TruncatesLongErrorOutputKeepingTheTail(t *testing.T) {
	ctx := sampleHandoffContext("/projects/fake-app")
	ctx.ErrorOutput = "HEAD-MARKER\n" + strings.Repeat("filler line of terraform noise\n", 10000) + "TAIL-MARKER"

	b := BuildBriefing(ctx)

	if !strings.Contains(b, "TAIL-MARKER") {
		t.Error("truncation must keep the tail of the error output")
	}
	if strings.Contains(b, "HEAD-MARKER") {
		t.Error("truncation must drop the head of very long error output")
	}
	if !strings.Contains(b, "earlier bytes omitted") {
		t.Error("truncation must say that output was omitted")
	}
	if len(b) > maxBriefingErrorOutput+20000 {
		t.Errorf("briefing is %d bytes; error output was not capped", len(b))
	}
}

func TestBuildBriefing_HandlesEmptyContext(t *testing.T) {
	b := BuildBriefing(HandoffContext{})

	if !strings.Contains(b, "Do NOT fix the problem directly in AWS") {
		t.Error("rules must be present even with no context")
	}
	if strings.Contains(b, "<no value>") {
		t.Error("empty fields must not render as <no value>")
	}
}

// --- LaunchClaudeHandoff -----------------------------------------------------

type runCall struct {
	name string
	args []string
}

// fakeHandoffSystem swaps handoffSystem for the duration of a test. Nothing
// here ever spawns tmux or claude.
func fakeHandoffSystem(t *testing.T, env map[string]string, claudePath string, runErr error) *[]runCall {
	t.Helper()
	calls := &[]runCall{}
	orig := handoffSystem
	t.Cleanup(func() { handoffSystem = orig })
	handoffSystem = handoffDeps{
		getenv: func(k string) string { return env[k] },
		lookPath: func(name string) (string, error) {
			if name == "claude" && claudePath != "" {
				return claudePath, nil
			}
			return "", exec.ErrNotFound
		},
		run: func(name string, args ...string) error {
			*calls = append(*calls, runCall{name, append([]string(nil), args...)})
			return runErr
		},
	}
	return calls
}

func TestLaunchClaudeHandoff_InTmuxOpensOneSplitPane(t *testing.T) {
	root := t.TempDir()
	calls := fakeHandoffSystem(t, map[string]string{"TMUX": "/tmp/tmux-501/default,1,0"}, "/usr/local/bin/claude", nil)
	ctx := sampleHandoffContext(root)

	res, err := LaunchClaudeHandoff(ctx)
	if err != nil {
		t.Fatalf("LaunchClaudeHandoff: %v", err)
	}

	if !res.PaneOpened {
		t.Error("PaneOpened = false, want true")
	}
	if len(*calls) != 1 {
		t.Fatalf("got %d runner calls, want exactly 1: %+v", len(*calls), *calls)
	}
	c := (*calls)[0]
	if c.name != "tmux" {
		t.Fatalf("runner called %q, want tmux", c.name)
	}
	if len(c.args) != 5 || c.args[0] != "split-window" || c.args[1] != "-h" || c.args[2] != "-c" || c.args[3] != root {
		t.Fatalf("tmux args = %q, want [split-window -h -c %s <command>]", c.args, root)
	}
	command := c.args[4]
	if !strings.Contains(command, "claude") {
		t.Errorf("pane command %q does not start claude", command)
	}
	if !strings.Contains(command, res.BriefingPath) {
		t.Errorf("pane command %q does not reference the briefing %q", command, res.BriefingPath)
	}
	if strings.Contains(command, "Do NOT fix the problem directly in AWS") || len(command) > 1000 {
		t.Errorf("pane command inlines the briefing body (%d bytes)", len(command))
	}
}

func TestLaunchClaudeHandoff_WritesBriefingAndSelfIgnoringDir(t *testing.T) {
	root := t.TempDir()
	fakeHandoffSystem(t, nil, "", nil)
	ctx := sampleHandoffContext(root)

	res, err := LaunchClaudeHandoff(ctx)
	if err != nil {
		t.Fatalf("LaunchClaudeHandoff: %v", err)
	}

	want := filepath.Join(root, ".meroku", "claude", "20261029-140509-apply.md")
	if res.BriefingPath != want {
		t.Errorf("BriefingPath = %q, want %q", res.BriefingPath, want)
	}
	data, err := os.ReadFile(res.BriefingPath)
	if err != nil {
		t.Fatalf("briefing file not written: %v", err)
	}
	if string(data) != BuildBriefing(ctx) {
		t.Error("briefing file content differs from BuildBriefing(ctx)")
	}
	ignore, err := os.ReadFile(filepath.Join(root, ".meroku", ".gitignore"))
	if err != nil {
		t.Fatalf(".meroku/.gitignore not written: %v", err)
	}
	if strings.TrimSpace(string(ignore)) != "*" {
		t.Errorf(".meroku/.gitignore = %q, want \"*\"", ignore)
	}
}

func TestLaunchClaudeHandoff_KeepsAnExistingGitignore(t *testing.T) {
	root := t.TempDir()
	fakeHandoffSystem(t, nil, "", nil)
	if err := os.MkdirAll(filepath.Join(root, ".meroku"), 0o755); err != nil {
		t.Fatal(err)
	}
	custom := "*\n!keep.txt\n"
	if err := os.WriteFile(filepath.Join(root, ".meroku", ".gitignore"), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LaunchClaudeHandoff(sampleHandoffContext(root)); err != nil {
		t.Fatalf("LaunchClaudeHandoff: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(root, ".meroku", ".gitignore"))
	if string(got) != custom {
		t.Errorf(".meroku/.gitignore was overwritten: %q", got)
	}
}

func TestLaunchClaudeHandoff_OutsideTmuxReturnsManualCommand(t *testing.T) {
	root := t.TempDir()
	calls := fakeHandoffSystem(t, map[string]string{}, "/usr/local/bin/claude", nil)

	res, err := LaunchClaudeHandoff(sampleHandoffContext(root))
	if err != nil {
		t.Fatalf("LaunchClaudeHandoff: %v", err)
	}

	if len(*calls) != 0 {
		t.Errorf("runner called outside tmux: %+v", *calls)
	}
	if res.PaneOpened {
		t.Error("PaneOpened = true outside tmux")
	}
	if !filepath.IsAbs(res.BriefingPath) {
		t.Errorf("BriefingPath %q is not absolute", res.BriefingPath)
	}
	if !strings.HasPrefix(res.ManualCommand, "cd ") || !strings.Contains(res.ManualCommand, "&& claude ") {
		t.Errorf("ManualCommand = %q, want `cd <root> && claude <prompt>`", res.ManualCommand)
	}
	if !strings.Contains(res.ManualCommand, res.BriefingPath) {
		t.Errorf("ManualCommand %q does not reference the briefing", res.ManualCommand)
	}
}

func TestLaunchClaudeHandoff_ClaudeNotOnPathDoesNotExec(t *testing.T) {
	root := t.TempDir()
	calls := fakeHandoffSystem(t, map[string]string{"TMUX": "/tmp/tmux-501/default,1,0"}, "", nil)

	res, err := LaunchClaudeHandoff(sampleHandoffContext(root))
	if err != nil {
		t.Fatalf("LaunchClaudeHandoff: %v", err)
	}

	if len(*calls) != 0 {
		t.Errorf("runner called without claude on PATH: %+v", *calls)
	}
	if res.PaneOpened {
		t.Error("PaneOpened = true without claude on PATH")
	}
	if res.ManualCommand == "" {
		t.Error("ManualCommand is empty")
	}
}

func TestLaunchClaudeHandoff_TmuxFailureFallsBackToManual(t *testing.T) {
	root := t.TempDir()
	fakeHandoffSystem(t, map[string]string{"TMUX": "x"}, "/usr/local/bin/claude", errors.New("no space for new pane"))

	res, err := LaunchClaudeHandoff(sampleHandoffContext(root))
	if err != nil {
		t.Fatalf("a tmux failure must not fail the handoff: %v", err)
	}
	if res.PaneOpened {
		t.Error("PaneOpened = true after tmux failed")
	}
	if !strings.Contains(res.PaneError, "no space for new pane") {
		t.Errorf("PaneError = %q, want the tmux error", res.PaneError)
	}
	if res.ManualCommand == "" {
		t.Error("ManualCommand is empty after tmux failed")
	}
}

// hostileRoot builds a project root whose path contains everything a shell
// would otherwise interpret: a space, a single quote, $ and a backtick.
func hostileRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "it's my $HOME `id` proj")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// onlyQuotedWords matches space-separated words in shellQuote's form: single
// quoted runs joined by '\”. Nothing outside the quotes, so no operators.
var onlyQuotedWords = regexp.MustCompile(`^'[^']*'(\\''[^']*')*( '[^']*'(\\''[^']*')*)*$`)

// shellWords runs words through a real POSIX shell and returns what it parsed,
// one per line. This is the parse tmux's `sh -c` would do.
//
// It must only ever see plain words: a shell operator such as && would end the
// printf and run whatever follows. PATH is emptied as a second guard, so even a
// quoting bug cannot find claude or tmux to execute.
func shellWords(t *testing.T, words string) []string {
	t.Helper()
	if !onlyQuotedWords.MatchString(words) {
		t.Fatalf("shellWords given %q, which is not only single-quoted words; refusing to evaluate it", words)
	}
	cmd := exec.Command("/bin/sh", "-c", "printf '%s\\n' "+words)
	cmd.Env = []string{"PATH="}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("sh could not parse %q: %v", words, err)
	}
	return strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
}

func TestLaunchClaudeHandoff_PaneCommandQuotesHostilePaths(t *testing.T) {
	root := hostileRoot(t)
	claudePath := "/opt/it's $x/claude"
	calls := fakeHandoffSystem(t, map[string]string{"TMUX": "x"}, claudePath, nil)

	res, err := LaunchClaudeHandoff(sampleHandoffContext(root))
	if err != nil {
		t.Fatalf("LaunchClaudeHandoff: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("got %d runner calls, want 1", len(*calls))
	}
	if got := (*calls)[0].args[3]; got != root {
		t.Errorf("tmux -c got %q, want %q (passed as an argv element, unquoted)", got, root)
	}

	words := shellWords(t, (*calls)[0].args[4])
	if len(words) != 2 {
		t.Fatalf("pane command parsed into %d words, want 2: %q", len(words), words)
	}
	if words[0] != claudePath {
		t.Errorf("word 0 = %q, want %q", words[0], claudePath)
	}
	if words[1] != handoffPrompt(res.BriefingPath) {
		t.Errorf("prompt did not round-trip:\n got %q\nwant %q", words[1], handoffPrompt(res.BriefingPath))
	}
}

func TestLaunchClaudeHandoff_ManualCommandQuotesHostilePaths(t *testing.T) {
	root := hostileRoot(t)
	fakeHandoffSystem(t, nil, "", nil)

	res, err := LaunchClaudeHandoff(sampleHandoffContext(root))
	if err != nil {
		t.Fatalf("LaunchClaudeHandoff: %v", err)
	}

	// Never evaluate the manual command itself: its && would run claude. Check
	// its exact shape, then round-trip each quoted word on its own.
	quotedRoot := shellQuote(root)
	quotedPrompt := shellQuote(handoffPrompt(res.BriefingPath))
	if want := "cd " + quotedRoot + " && claude " + quotedPrompt; res.ManualCommand != want {
		t.Fatalf("ManualCommand =\n %q\nwant\n %q", res.ManualCommand, want)
	}
	if got := shellWords(t, quotedRoot); len(got) != 1 || got[0] != root {
		t.Errorf("root did not round-trip: %q", got)
	}
	if got := shellWords(t, quotedPrompt); len(got) != 1 || got[0] != handoffPrompt(res.BriefingPath) {
		t.Errorf("prompt did not round-trip: %q", got)
	}
}

func TestBuildBriefing_ProjectRootWithBacktickStaysOneCodeSpan(t *testing.T) {
	root := "/projects/it's `id` | x"
	b := BuildBriefing(sampleHandoffContext(root))

	if !strings.Contains(b, "| Project root | `` /projects/it's `id` \\| x `` |") {
		t.Error("table cell must wrap the root in a longer backtick run and escape |")
	}
	if !strings.Contains(b, "(`` /projects/it's `id` | x ``)") {
		t.Error("prose must wrap the root in a longer backtick run")
	}
}

func TestShellQuote_ExactForm(t *testing.T) {
	cases := map[string]string{
		"plain":     "'plain'",
		"it's":      `'it'\''s'`,
		"a $b `c`":  "'a $b `c`'",
		"":          "''",
		"/x y/z.md": "'/x y/z.md'",
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLaunchClaudeHandoff_SanitisesOperationInFileName(t *testing.T) {
	root := t.TempDir()
	fakeHandoffSystem(t, nil, "", nil)
	ctx := sampleHandoffContext(root)
	ctx.Operation = "../DNS bootstrap"

	res, err := LaunchClaudeHandoff(ctx)
	if err != nil {
		t.Fatalf("LaunchClaudeHandoff: %v", err)
	}
	if filepath.Dir(res.BriefingPath) != filepath.Join(root, ".meroku", "claude") {
		t.Errorf("briefing escaped .meroku/claude: %q", res.BriefingPath)
	}
	if got := filepath.Base(res.BriefingPath); got != "20261029-140509-dns-bootstrap.md" {
		t.Errorf("file name = %q", got)
	}
}

// Two handoffs in the same second for the same operation (the SSO fix-up
// loops over broken profiles) must not share a file: each pane has to read
// its own briefing.
func TestLaunchClaudeHandoff_SameSecondSameOperationGetDistinctFiles(t *testing.T) {
	root := t.TempDir()
	calls := fakeHandoffSystem(t, map[string]string{"TMUX": "x"}, "/usr/local/bin/claude", nil)

	first := sampleHandoffContext(root)
	first.ErrorOutput = "FIRST-HANDOFF-MARKER"
	second := sampleHandoffContext(root)
	second.ErrorOutput = "SECOND-HANDOFF-MARKER"

	r1, err := LaunchClaudeHandoff(first)
	if err != nil {
		t.Fatalf("first LaunchClaudeHandoff: %v", err)
	}
	r2, err := LaunchClaudeHandoff(second)
	if err != nil {
		t.Fatalf("second LaunchClaudeHandoff: %v", err)
	}

	if r1.BriefingPath == r2.BriefingPath {
		t.Fatalf("both handoffs wrote %q", r1.BriefingPath)
	}
	for _, c := range []struct {
		res  HandoffResult
		ctx  HandoffContext
		mark string
	}{{r1, first, "FIRST-HANDOFF-MARKER"}, {r2, second, "SECOND-HANDOFF-MARKER"}} {
		data, err := os.ReadFile(c.res.BriefingPath)
		if err != nil {
			t.Fatalf("reading %s: %v", c.res.BriefingPath, err)
		}
		if string(data) != BuildBriefing(c.ctx) || !strings.Contains(string(data), c.mark) {
			t.Errorf("%s does not hold its own briefing", c.res.BriefingPath)
		}
	}
	if len(*calls) != 2 {
		t.Fatalf("got %d runner calls, want 2", len(*calls))
	}
	if cmd := (*calls)[0].args[4]; !strings.Contains(cmd, r1.BriefingPath+" ") || strings.Contains(cmd, r2.BriefingPath) {
		t.Errorf("first pane command %q does not reference only %q", cmd, r1.BriefingPath)
	}
	if cmd := (*calls)[1].args[4]; !strings.Contains(cmd, r2.BriefingPath+" ") {
		t.Errorf("second pane command %q does not reference %q", cmd, r2.BriefingPath)
	}
	if got := filepath.Base(r2.BriefingPath); got != "20261029-140509-apply-2.md" {
		t.Errorf("second file name = %q, want 20261029-140509-apply-2.md", got)
	}
	if info, err := os.Stat(r2.BriefingPath); err == nil && info.Mode().Perm() != 0o600 {
		t.Errorf("second briefing mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestLaunchClaudeHandoff_SSOFileNameCarriesTheProfile(t *testing.T) {
	root := t.TempDir()
	fakeHandoffSystem(t, nil, "", nil)
	ctx := sampleHandoffContext(root)
	ctx.Operation = "sso"
	ctx.AWSProfile = "Example_Profile/../x"

	res, err := LaunchClaudeHandoff(ctx)
	if err != nil {
		t.Fatalf("LaunchClaudeHandoff: %v", err)
	}
	if got := filepath.Base(res.BriefingPath); got != "20261029-140509-sso-example-profile-x.md" {
		t.Errorf("file name = %q, want 20261029-140509-sso-example-profile-x.md", got)
	}
	if filepath.Dir(res.BriefingPath) != filepath.Join(root, ".meroku", "claude") {
		t.Errorf("briefing escaped .meroku/claude: %q", res.BriefingPath)
	}
}

func TestLaunchClaudeHandoff_RequiresProjectRoot(t *testing.T) {
	fakeHandoffSystem(t, nil, "", nil)
	ctx := sampleHandoffContext("")

	if _, err := LaunchClaudeHandoff(ctx); err == nil {
		t.Error("an empty ProjectRoot must be an error, never a fallback to cwd")
	}
}

// --- FormatHandoffResult -----------------------------------------------------

func TestFormatHandoffResult_PaneOpened(t *testing.T) {
	out := FormatHandoffResult(HandoffResult{
		BriefingPath: "/projects/fake-app/.meroku/claude/x.md",
		PaneOpened:   true,
	})

	if !strings.Contains(out, "right-hand pane") || !strings.Contains(out, "/projects/fake-app/.meroku/claude/x.md") {
		t.Errorf("message = %q", out)
	}
}

func TestFormatHandoffResult_PaneOpenedWarnsAboutRawErrorOutput(t *testing.T) {
	out := FormatHandoffResult(HandoffResult{
		BriefingPath: "/projects/fake-app/.meroku/claude/x.md",
		PaneOpened:   true,
	})

	if !strings.Contains(out, "may contain raw error") {
		t.Errorf("pane-opened message must warn that the briefing may contain raw error output:\n%s", out)
	}
}

func TestFormatHandoffResult_NotOpened(t *testing.T) {
	out := FormatHandoffResult(HandoffResult{
		BriefingPath:  "/projects/fake-app/.meroku/claude/x.md",
		ManualCommand: "cd '/projects/fake-app' && claude 'Read ...'",
	})

	for _, want := range []string{
		"/projects/fake-app/.meroku/claude/x.md",
		"cd '/projects/fake-app' && claude 'Read ...'",
		"may contain raw error",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("message is missing %q:\n%s", want, out)
		}
	}
}

// --- version -----------------------------------------------------------------

func TestLaunchClaudeHandoff_FillsVersionFromProjectRoot(t *testing.T) {
	root := t.TempDir()
	fakeHandoffSystem(t, nil, "", nil)
	if err := os.MkdirAll(filepath.Join(root, "infrastructure"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "infrastructure", "version.txt"), []byte("v9.9.9-fake\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := sampleHandoffContext(root)
	ctx.MerokuVersion = ""

	res, err := LaunchClaudeHandoff(ctx)
	if err != nil {
		t.Fatalf("LaunchClaudeHandoff: %v", err)
	}
	data, _ := os.ReadFile(res.BriefingPath)
	if !strings.Contains(string(data), "v9.9.9-fake") {
		t.Error("briefing does not carry the version read from <root>/infrastructure/version.txt")
	}
}
