# Claude Code Handoff ("Fix with AI")

meroku does not call any AI API. When something fails, it writes a briefing file and
starts Claude Code beside meroku to read it. This replaced the built-in ReAct agent,
the one-shot "AI Help", the plan "explain with AI" HTML export and the AWS SSO AI
agent, all of which called the Anthropic API with `ANTHROPIC_API_KEY`.

## Entry points

| Where | How | Operation | Goal given to Claude |
|---|---|---|---|
| Failed apply screen | key `a` | `apply` | find why `terraform apply` failed, explain, propose a fix |
| Failed plan screen | key `a` | `plan` | same, for `terraform plan` |
| Failed destroy screen | key `a` | `destroy` | same, for `terraform destroy` |
| Main menu | "🤖 Fix with AI (Claude Code)" | `menu` | ask the user what is wrong, then investigate |
| AWS SSO Tools menu | "🤖 Fix with AI (Claude Code)" | `sso` | help fix and investigate the AWS SSO/profile setup |
| Startup SSO check | "🤖 Fix with AI (Claude Code)" | `sso` | same |
| Profile validator fix prompt | choice `2` | `sso` | same |

On the plan dashboard (before any apply) `a` is Apply. After an apply has run, `a`
never starts another apply: on a failed apply it is the handoff, otherwise it does
nothing.

The TUIs do not launch anything themselves. `a` sets a `launchClaude` flag on the model
and quits the Bubble Tea program; the function that ran the program
(`showModernTerraformPlanTUI`, `runTerraformPlanWithProgress`,
`runTerraformDestroyWithProgress`) builds the context and calls `offerClaudeHandoff`
once the terminal is back in normal mode.

## What the briefing contains

`BuildBriefing` renders `app/claude_briefing/briefing.md` (a Go `text/template`) with a
`HandoffContext`:

- **Goal** for this operation.
- **The rules** (below).
- **Where it ran**: operation, env name, absolute project root, AWS account id, AWS
  profile, AWS region, meroku version, time.
- **What failed**:
  - apply: every failed resource (address, action, summary, detail, `file:line`
    location) with meroku's name-collision hint when it applies; resources cancelled
    "due to previous errors" are left out. Every Terraform diagnostic not tied to a
    failed resource (provider, backend and configuration errors). The exit error and
    Terraform's stderr, which the apply TUI now keeps instead of dropping.
  - plan and destroy: the `Error:`/`Warning:` headlines and the full stderr output.
  - SSO: profile name, whether it exists, its type, the missing fields, the AWS config
    file path, and the validator's messages. No tokens, keys or cached credentials.
- **How meroku works** and **how to rerun**: YAML → migrations → `infrastructure/env/main.hbs`
  → `env/<env>/*.tf` → terraform in `env/<env>/`, and the real commands
  (`meroku generate <env>`, `meroku migrate`, `meroku sync <env>`, `meroku dns status`).

Long error output keeps its last 16 KiB (`maxBriefingErrorOutput`).

### Where the env context comes from

`handoffTarget{ProjectRoot, EnvName, Env}` is captured at the project root:

- deploy: in `runCommandToDeploy` (`app/deploy.go`) before it changes into `env/<env>`,
  and passed through `runTerraformApply` → `runTerraformPlanWithProgress` /
  `showModernTerraformPlanTUI`.
- destroy: in `nukeMenu` (`app/nuke.go`) via `handoffTargetFromProjectRoot`.
- menu and SSO: from the working directory at the menu (the project root) and the loaded
  env YAML; the menu also prefers `selectedAWSProfile` / `selectedAWSRegion`, which the
  `--profile` flag and environment selection set.

Nothing reads `$AWS_PROFILE` or parses the working directory to find the environment.
The tests set `$AWS_PROFILE`/`$AWS_REGION` to other values and check they never appear.

## The rules

These are the user's rules, stated in the briefing:

1. Do NOT fix the problem directly in AWS: no console changes, no CLI or SDK writes.
2. Do NOT edit the generated Terraform in `env/<env>/`. meroku regenerates it, so the
   edit is lost or drifts from the YAML.
3. Investigate and explain first; ask before any write.

Allowed fixes, in this order:

- (a) The cause is a meroku bug: say so with evidence and fix it in meroku
  (github.com/MadAppGang/meroku), in the user's local checkout.
- (b) An edge case meroku cannot migrate by itself: interview the user, do the one-off
  manual step together, and write down exactly what was done.
- (c) A configuration problem: change the env YAML and rerun meroku.

Read-only inspection (`describe`/`list`/`get`, `aws sts get-caller-identity`,
`terraform state list/show`, `terraform plan`) is allowed at any time.

## Launch

`LaunchClaudeHandoff` (`app/claude_handoff.go`):

1. Writes the briefing to `<project root>/.meroku/claude/<YYYYMMDD-HHMMSS>-<operation>.md`
   (mode 0600). Creates `.meroku/.gitignore` with `*` if there is none, so the directory
   ignores itself in git. The briefing can contain raw error output; the printed
   message says so.
2. If `$TMUX` is set and `claude` is on PATH, runs
   `tmux split-window -h -c <project root> '<claude path>' '<prompt>'`. The prompt is one
   sentence pointing at the briefing file; the briefing is never on the command line.
   Every word is single-quoted for the shell, so paths with spaces, quotes, `$` or
   backticks are safe.
3. Otherwise, or if tmux fails, nothing is executed. meroku prints the briefing path
   and `cd '<root>' && claude '<prompt>'`, then waits for Enter.

## Changing the briefing

- Edit `app/claude_briefing/briefing.md`. It is embedded with `go:embed`, so the change
  ships with the next build and always matches the binary.
- Template fields are those of `briefingView` in `app/claude_handoff.go`. Use the
  helpers there for untrusted text: `fence` (code block longer than any backtick run
  inside), `indent` (fenced and indented under a list item), `code` and `cell`
  (inline code, table-safe).
- Keep commands real. `TestBuildBriefing_UsesRealCommandsOnly` rejects the old
  `--generate` flag and hard-coded paths.
- Run `go test ./app/...` (from `app/`: `go test -run 'Briefing|Handoff' .`).

## Adding a new entry point

Build a `HandoffContext` (start from `handoffTarget.context(operation, goal)` so env,
profile, region and account come from the loaded YAML), fill `Failures`, `Diagnostics`
and `ErrorOutput`, and call `offerClaudeHandoff(ctx)` outside any running Bubble Tea
program. Tests must never spawn tmux or claude: replace the package variable
`handoffSystem` (see `fakeHandoffSystem` in `app/claude_handoff_test.go`).

## Tests

- `app/claude_handoff_test.go`: briefing content, rules, quoting, file layout, tmux and
  non-tmux paths.
- `app/claude_handoff_context_test.go`: context extraction from the apply, plan and
  destroy models, menu and SSO contexts, and the `a` / `s` / `e` key behaviour and
  footers.
