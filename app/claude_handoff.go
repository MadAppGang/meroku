package main

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

// The Claude Code handoff replaces meroku's own AI features with one action:
// write a briefing about what failed and start Claude Code beside meroku to
// read it. The briefing text is embedded, so it always describes the binary
// that is running rather than whatever version a doc on disk was written for.

//go:embed claude_briefing/*.md
var claudeBriefingFS embed.FS

// HandoffFailure is one resource Terraform reported as failed.
type HandoffFailure struct {
	Address  string
	Action   string
	Summary  string
	Detail   string
	Location string
}

// HandoffContext is everything the briefing says about where meroku ran and
// what went wrong. Callers fill it from the loaded Env, never from the shell
// ($AWS_PROFILE may name a different profile than the one meroku used).
type HandoffContext struct {
	// Operation is what was running: apply, plan, destroy, generate,
	// preflight, dns, migration, sso or menu. It also names the briefing file.
	Operation string
	// Goal is the one-line task for Claude, e.g. "fix the failed apply".
	Goal string

	Env string
	// ProjectRoot is the absolute project directory, captured before meroku
	// changes into env/<env>. Required: the handoff never falls back to cwd.
	ProjectRoot   string
	AWSProfile    string
	AWSRegion     string
	AWSAccountID  string
	MerokuVersion string
	Time          time.Time

	Failures []HandoffFailure
	// Diagnostics are errors and warnings Terraform did not tie to a resource
	// address: provider, backend and configuration problems.
	Diagnostics []string
	// ErrorOutput is raw stderr / the error chain. Only its tail is kept.
	ErrorOutput string
}

// HandoffResult says what LaunchClaudeHandoff did, for the caller to print.
type HandoffResult struct {
	// BriefingPath is the absolute path of the written briefing.
	BriefingPath string
	// PaneOpened is true when Claude Code was started in a tmux split.
	PaneOpened bool
	// ManualCommand starts Claude Code on the briefing from any shell.
	ManualCommand string
	// PaneError is why opening the pane failed, when it was attempted.
	PaneError string
}

// maxBriefingErrorOutput caps the raw error output in the briefing. Terraform
// can print megabytes; the cause is almost always at the end.
const maxBriefingErrorOutput = 16 * 1024

// handoffDeps is the part of the outside world the handoff touches. Tests
// replace handoffSystem so they never spawn tmux or claude.
type handoffDeps struct {
	getenv   func(string) string
	lookPath func(string) (string, error)
	run      func(name string, args ...string) error
}

var handoffSystem = handoffDeps{
	getenv:   os.Getenv,
	lookPath: exec.LookPath,
	run:      runHandoffCommand,
}

func runHandoffCommand(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

var briefingTemplate = template.Must(
	template.New("briefing.md").
		Funcs(template.FuncMap{
			"inc":    func(i int) int { return i + 1 },
			"fence":  fenceBlock,
			"indent": indentBlock,
			"code":   inlineCode,
			"cell":   cellCode,
		}).
		ParseFS(claudeBriefingFS, "claude_briefing/briefing.md"),
)

// briefingView is HandoffContext with every blank replaced by something a
// reader can act on, so the template never renders an empty cell.
type briefingView struct {
	HandoffContext
	EnvName            string
	EnvDisplay         string
	ProjectRootDisplay string
	ProfileDisplay     string
	ProfileArg         string
	RegionDisplay      string
	AccountDisplay     string
	VersionDisplay     string
	TimeDisplay        string
	Truncated          bool
	HasFailureData     bool
}

// BuildBriefing renders the briefing for ctx. Pure: no clock, no environment,
// no files beyond the embedded template.
func BuildBriefing(ctx HandoffContext) string {
	const unknown = "(not recorded)"
	or := func(s, fallback string) string {
		if strings.TrimSpace(s) == "" {
			return fallback
		}
		return s
	}

	v := briefingView{HandoffContext: ctx}
	v.Operation = or(ctx.Operation, "handoff")
	v.Goal = or(ctx.Goal, "Ask the user what is wrong, then investigate it.")
	v.EnvName = or(ctx.Env, "<env>")
	v.EnvDisplay = or(ctx.Env, unknown)
	v.ProjectRootDisplay = or(ctx.ProjectRoot, unknown)
	v.ProfileDisplay = or(ctx.AWSProfile, "(none in the env YAML: the AWS default credential chain applies)")
	v.ProfileArg = or(ctx.AWSProfile, "<profile>")
	v.RegionDisplay = or(ctx.AWSRegion, unknown)
	v.AccountDisplay = or(ctx.AWSAccountID, unknown)
	v.VersionDisplay = or(ctx.MerokuVersion, unknown)
	if ctx.Time.IsZero() {
		v.TimeDisplay = unknown
	} else {
		v.TimeDisplay = ctx.Time.Format(time.RFC3339)
	}
	v.ErrorOutput, v.Truncated = tailOf(strings.TrimRight(ctx.ErrorOutput, "\n"), maxBriefingErrorOutput)
	v.HasFailureData = len(ctx.Failures) > 0 || len(ctx.Diagnostics) > 0 || v.ErrorOutput != ""

	var buf bytes.Buffer
	if err := briefingTemplate.Execute(&buf, v); err != nil {
		// Only reachable through a broken embedded template, which the tests
		// catch. Still hand Claude the rules rather than nothing.
		return fmt.Sprintf("meroku could not render its briefing (%v).\n\n"+
			"Do NOT fix the problem directly in AWS and do NOT edit the generated Terraform in env/%s/. "+
			"Investigate and explain first; ask before any write.\n", err, v.EnvName)
	}
	return buf.String()
}

// tailOf keeps the last max bytes of s, starting at a line boundary when one
// is close, and says how much was dropped.
func tailOf(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	cut := len(s) - max
	if nl := strings.IndexByte(s[cut:], '\n'); nl >= 0 && nl < 512 {
		cut += nl + 1
	}
	return fmt.Sprintf("[... %d earlier bytes omitted ...]\n%s", cut, s[cut:]), true
}

// fenceBlock wraps s in a code fence longer than any backtick run inside it,
// so error text containing ``` cannot close the block early.
func fenceBlock(s string) string {
	fence := strings.Repeat("`", max(3, longestBacktickRun(s)+1))
	return fence + "\n" + strings.TrimRight(s, "\n") + "\n" + fence
}

// inlineCode wraps s in a backtick run longer than any inside it. A project
// root may contain backticks, which would close a plain `...` span early.
// When the fence is longer than one backtick, s is padded with a space on each
// side; CommonMark strips exactly that padding.
func inlineCode(s string) string {
	n := longestBacktickRun(s) + 1
	fence := strings.Repeat("`", n)
	if n == 1 {
		return fence + s + fence
	}
	return fence + " " + s + " " + fence
}

// cellCode is inlineCode for a table cell, where an unescaped | ends the cell.
func cellCode(s string) string {
	return inlineCode(strings.ReplaceAll(s, "|", `\|`))
}

func longestBacktickRun(s string) int {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return longest
}

// indentBlock fences s and indents it to sit under a list item.
func indentBlock(s string) string {
	lines := strings.Split(fenceBlock(s), "\n")
	for i, l := range lines {
		lines[i] = "     " + l
	}
	return strings.Join(lines, "\n")
}

// LaunchClaudeHandoff writes the briefing to
// <ProjectRoot>/.meroku/claude/<YYYYMMDD-HHMMSS>-<operation>.md and, inside
// tmux with claude on PATH, opens Claude Code on it in a split to the right.
// Otherwise nothing is executed and the result carries the command to run.
//
// A failure to open the pane is not an error: the briefing exists, and the
// manual command still works. Only a failure to write the briefing is.
func LaunchClaudeHandoff(ctx HandoffContext) (HandoffResult, error) {
	if strings.TrimSpace(ctx.ProjectRoot) == "" {
		return HandoffResult{}, errors.New("claude handoff: project root is not set")
	}
	root, err := filepath.Abs(ctx.ProjectRoot)
	if err != nil {
		return HandoffResult{}, fmt.Errorf("claude handoff: resolving project root: %w", err)
	}
	ctx.ProjectRoot = root
	if ctx.Time.IsZero() {
		ctx.Time = time.Now()
	}
	if ctx.MerokuVersion == "" {
		ctx.MerokuVersion = handoffMerokuVersion(root)
	}

	path, err := writeHandoffBriefing(ctx)
	if err != nil {
		return HandoffResult{}, err
	}

	prompt := handoffPrompt(path)
	res := HandoffResult{
		BriefingPath:  path,
		ManualCommand: "cd " + shellQuote(root) + " && claude " + shellQuote(prompt),
	}

	if handoffSystem.getenv("TMUX") == "" {
		return res, nil
	}
	claudePath, err := handoffSystem.lookPath("claude")
	if err != nil {
		return res, nil
	}

	// tmux hands a single command argument to its default shell with -c, so
	// this is the one string that needs quoting. The resolved path is used
	// because the tmux server's PATH can differ from meroku's.
	paneCommand := shellQuote(claudePath) + " " + shellQuote(prompt)
	if err := handoffSystem.run("tmux", "split-window", "-h", "-c", root, paneCommand); err != nil {
		res.PaneError = err.Error()
		return res, nil
	}
	res.PaneOpened = true
	return res, nil
}

// writeHandoffBriefing creates .meroku/claude/ under the project root, makes
// .meroku ignore itself, and writes the briefing there.
func writeHandoffBriefing(ctx HandoffContext) (string, error) {
	meroku := filepath.Join(ctx.ProjectRoot, ".meroku")
	dir := filepath.Join(meroku, "claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("claude handoff: creating %s: %w", dir, err)
	}

	// The briefing can hold raw error output, so .meroku/ is kept out of git
	// by its own .gitignore. An existing one is the user's and is left alone.
	ignore := filepath.Join(meroku, ".gitignore")
	if _, err := os.Stat(ignore); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(ignore, []byte("*\n"), 0o644); err != nil {
			return "", fmt.Errorf("claude handoff: writing %s: %w", ignore, err)
		}
	}

	// The SSO fix-up hands off one profile after another, so its briefings are
	// told apart by profile as well as by time.
	slugSource := ctx.Operation
	if ctx.Operation == "sso" && ctx.AWSProfile != "" {
		slugSource += " " + ctx.AWSProfile
	}
	base := ctx.Time.Format("20060102-150405") + "-" + handoffFileSlug(slugSource)

	// O_EXCL, never truncate: two handoffs in the same second must not share a
	// file, or one pane reads the other's briefing.
	body := []byte(BuildBriefing(ctx))
	for n := 1; n <= maxBriefingNameAttempts; n++ {
		name := base + ".md"
		if n > 1 {
			name = fmt.Sprintf("%s-%d.md", base, n)
		}
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("claude handoff: writing %s: %w", path, err)
		}
		_, werr := f.Write(body)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return "", fmt.Errorf("claude handoff: writing %s: %w", path, werr)
		}
		return path, nil
	}
	return "", fmt.Errorf("claude handoff: %d briefings named %s already exist in %s", maxBriefingNameAttempts, base, dir)
}

// maxBriefingNameAttempts bounds the -2, -3, … suffixes tried for one second.
const maxBriefingNameAttempts = 100

// handoffFileSlug reduces an operation name to [a-z0-9-] so it cannot add a
// path separator or anything a shell would trip on.
func handoffFileSlug(op string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(op) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if b.Len() > 0 && !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	if s := strings.TrimRight(b.String(), "-"); s != "" {
		return s
	}
	return "handoff"
}

// handoffMerokuVersion is GetVersion's lookup anchored at the project root
// instead of the working directory, which is env/<env> during a deploy.
func handoffMerokuVersion(root string) string {
	if version != "dev" {
		return version
	}
	if data, err := os.ReadFile(filepath.Join(root, "infrastructure", "version.txt")); err == nil {
		if v := strings.TrimSpace(string(data)); v != "" {
			return v
		}
	}
	return version
}

// handoffPrompt is the whole of what goes on Claude's command line. The
// briefing stays in the file: it is long, and it may contain error text.
func handoffPrompt(briefingPath string) string {
	return "Read the meroku handoff briefing at " + briefingPath +
		" and follow it. Investigate and explain first; ask me before any write."
}

// shellQuote quotes s as one POSIX shell word. Inside single quotes nothing is
// special, so only the quote itself needs handling.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// FormatHandoffResult is the message callers print after a handoff.
func FormatHandoffResult(r HandoffResult) string {
	if r.PaneOpened {
		return fmt.Sprintf("🤖 Claude Code opened in the right-hand pane. Briefing: %s\n", r.BriefingPath) +
			briefingPrivacyNote
	}
	var b strings.Builder
	if r.PaneError != "" {
		fmt.Fprintf(&b, "⚠️  Could not open a tmux pane for Claude Code: %s\n\n", r.PaneError)
	}
	fmt.Fprintf(&b, "🤖 Briefing for Claude Code written to:\n   %s\n\n", r.BriefingPath)
	fmt.Fprintf(&b, "Start Claude Code on it with:\n   %s\n\n", r.ManualCommand)
	b.WriteString(briefingPrivacyNote)
	return b.String()
}

const briefingPrivacyNote = "Note: the briefing may contain raw error output from Terraform or AWS. " +
	"It lives under .meroku/, which ignores itself in git; review it before sharing.\n"
