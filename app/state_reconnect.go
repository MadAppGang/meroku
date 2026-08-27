package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/charmbracelet/huh"
	"github.com/mattn/go-isatty"
)

// Connecting a local environment to the state that is already in its S3
// backend.
//
// env/ is generated output and is not committed, so every fresh checkout of a
// deployed project starts with no local Terraform state. That is the ordinary
// first-run path for anyone joining a project, not an incident. What made it
// look like one: meroku read env/<env>/ to decide what was deployed, so it
// reported nothing, and every terraform command then failed with "Backend
// initialization required, please run terraform init".
//
// The gate is therefore deliberately not "does env/<env>/ exist". <env>.yaml is
// committed and carries state_bucket, state_file and region, so meroku can
// always ask the only authority that knows.
//
// So there is exactly one local short-circuit — env/<env>/.terraform exists,
// therefore this directory is already connected — and everything else consults
// the remote state. Whether env/<env>/ is missing entirely or merely
// uninitialised is a detail of *how* to sync (generate first, or not), never of
// *whether* to. Splitting those two is what produced the silent case where a
// missing env/ directory looked like missing infrastructure.
//
// The cost is one S3 read on a fresh clone of a project that has never been
// deployed: a single failed lookup against a bucket that does not exist, which
// fails fast and degrades to silence.

// merokuSkipStateReconnect disables the remote-state probe entirely. Intended
// for CI and for anyone who does not want meroku reaching out to S3.
const merokuSkipStateReconnect = "MEROKU_SKIP_STATE_RECONNECT"

// remoteStateTimeout bounds the probe. Unreachable credentials or a bucket in
// another account must cost a few seconds, not a hung command.
const remoteStateTimeout = 20 * time.Second

// maxRemoteStateBytes caps what we read from S3. Real states are a few hundred
// KB; this stops a wrong or malformed object from being pulled into memory
// unbounded, and we only need the resource inventory anyway.
const maxRemoteStateBytes = 128 << 20

// errRemoteStateAbsent means the backend was reachable and simply has no state
// object yet. That is the normal condition of a project that has never been
// deployed, and it must stay silent.
var errRemoteStateAbsent = errors.New("remote terraform state not found")

// errTerraformNotInstalled is returned instead of exploding when the terraform
// binary is missing.
var errTerraformNotInstalled = errors.New("terraform executable not found in PATH")

// remoteStateSummary is what meroku learned about the state in the backend.
type remoteStateSummary struct {
	Bucket           string
	Key              string
	Region           string
	ResourceCount    int // instances, matching what `terraform state list` prints
	ManagedCount     int
	DataCount        int
	Serial           int64
	TerraformVersion string

	// ManagedTypes is the set of managed resource types this state tracks.
	//
	// It answers "does this environment own resource X", which no AWS read can
	// settle: an account-level resource can exist and belong entirely to another
	// project's state. Asking AWS returns existence; only the state returns
	// ownership, and acting on existence alone destroys the resource for
	// whichever project does own it.
	ManagedTypes map[string]bool
}

// owns reports whether this state manages any instance of a resource type.
func (s remoteStateSummary) owns(resourceType string) bool {
	return s.ManagedTypes[resourceType]
}

// remoteStateLookup reads the remote state for an environment.
//
// It is a function type, not a method, so tests can substitute a fake and never
// touch AWS.
type remoteStateLookup func(ctx context.Context, env Env) (remoteStateSummary, error)

// terraformInitializer runs `terraform init` for a directory. Injectable for the
// same reason.
type terraformInitializer func(dir string) error

// terraformPlanner runs `terraform plan` for a directory and reports what it
// found. Injectable so the failure paths can be tested without terraform.
type terraformPlanner func(dir string) (planSummary, error)

// environmentGenerator recreates env/<env>/ from <env>.yaml — exactly what
// `meroku generate` writes to disk, and nothing else. Injectable so tests never
// render a template or touch the filesystem outside their temp dir.
type environmentGenerator func(envName string) error

type stateConnectionStatus int

const (
	// stateAlreadyInitialised: env/<env>/.terraform exists. Left strictly alone,
	// and the only classification that costs no AWS call.
	stateAlreadyInitialised stateConnectionStatus = iota
	// stateNoBackendConfigured: the config names no bucket/key/region, so there
	// is no remote state to be connected to.
	stateNoBackendConfigured
	// stateFresh: the backend holds no state, or a state with no resources. A
	// genuinely new project. Silent.
	stateFresh
	// stateUnreadable: the bucket could not be read. Not fatal on the automatic
	// paths — we say why and carry on.
	stateUnreadable
	// stateDeployedButDisconnected: resources exist remotely and this directory
	// is not connected to them — whether because it was never initialised or
	// because it is not there at all. See stateConnection.Generated.
	stateDeployedButDisconnected
	// stateSkipped: the user opted out. Nothing is read, printed or run.
	stateSkipped
)

// stateConnection is the relationship between a local environment directory and
// the terraform state in its configured backend.
type stateConnection struct {
	Env     string
	Status  stateConnectionStatus
	Summary remoteStateSummary
	Err     error // populated only for stateUnreadable

	// Generated records whether env/<env>/ existed when the environment was
	// inspected. It is not a status of its own: a missing directory and an
	// uninitialised one are the same situation to the user, and this only decides
	// whether a sync has to regenerate before it can init.
	Generated bool

	// AWSProfile and AccountID name the account this state belongs to. They are
	// carried so the screen can show them permanently: which account you are
	// working in is the fact that makes a wrong run wrong.
	AWSProfile string
	AccountID  string
}

// defaultRemoteStateLookup and friends are the production wiring. Tests replace
// the arguments, not these.
var (
	defaultRemoteStateLookup    remoteStateLookup    = lookupRemoteStateFromS3
	defaultTerraformInitializer terraformInitializer = terraformInitInDir
	defaultTerraformPlanner     terraformPlanner     = terraformPlanInDir
	defaultEnvironmentGenerator environmentGenerator = generateEnvironmentFiles
)

// inspectStateConnection classifies an environment without printing or changing
// anything.
//
// The order matters. The one local check comes first and is cheap, so an
// environment that is already connected — the overwhelmingly common case — costs
// no network traffic at all. Everything else asks the backend, because the
// backend is the only thing that knows what is deployed.
func inspectStateConnection(ctx context.Context, envName string, env Env, envDir string, lookup remoteStateLookup) stateConnection {
	c := stateConnection{Env: envName, AWSProfile: env.AWSProfile, AccountID: env.AccountID}

	// An existing .terraform is the caller's state, not ours to touch. This
	// function reconnects; it never reconciles, migrates or overwrites.
	if _, err := os.Stat(filepath.Join(envDir, ".terraform")); err == nil {
		c.Status = stateAlreadyInitialised
		return c
	}

	// Deliberately not an early return. A missing directory is how a fresh clone
	// of a deployed project looks, so it decides how to recover, not whether.
	info, err := os.Stat(envDir)
	c.Generated = err == nil && info.IsDir()

	if env.StateBucket == "" || env.StateFile == "" || env.Region == "" {
		c.Status = stateNoBackendConfigured
		return c
	}

	summary, err := lookup(ctx, env)
	if err != nil {
		if errors.Is(err, errRemoteStateAbsent) {
			c.Status = stateFresh
			return c
		}
		c.Status = stateUnreadable
		c.Err = err
		return c
	}

	// A state with zero resources is a destroyed or never-applied environment.
	// Telling someone about resources that do not exist would be worse than
	// saying nothing.
	if summary.ResourceCount == 0 {
		c.Status = stateFresh
		return c
	}

	c.Status = stateDeployedButDisconnected
	c.Summary = summary
	return c
}

// The three sentences this feature is built around, in the order a reader needs
// them: what is true, what is missing and why, what syncing does.
//
// They are written once, here, because the screen and the plain fallback have
// to say the same thing, and because the wording is the point. This is a fresh
// checkout of a deployed project — the ordinary first run — so the text
// describes the reader's situation and stops. It does not reassure (reassurance
// only reassures someone who already suspects a problem; otherwise it invents
// one) and it does not narrate meroku's procedure.

// deployedLine: what is true.
func deployedLine(c stateConnection) string {
	noun := "resources"
	if c.Summary.ResourceCount == 1 {
		noun = "resource"
	}
	where := c.Summary.Region
	if where == "" {
		where = "AWS"
	}
	return fmt.Sprintf("%s is deployed — %d %s in %s.", c.Env, c.Summary.ResourceCount, noun, where)
}

// missingLine: what is missing, and why that is normal. "Link" is the reader's
// word for it; the sentence then says exactly which directory holds it and why
// they do not have one.
func missingLine(c stateConnection) string {
	if c.Generated {
		return fmt.Sprintf("env/%s/ is here but isn't linked to them yet. meroku keeps that link "+
			"in env/%s/.terraform, which git doesn't track.", c.Env, c.Env)
	}
	return fmt.Sprintf("This checkout has no link to them yet. meroku keeps that link in env/%s/, "+
		"which git doesn't track, so a fresh checkout never has one.", c.Env)
}

// syncingLine: what syncing does, and the guarantee that makes it safe to say
// yes to.
func syncingLine(c stateConnection) string {
	if c.Generated {
		return fmt.Sprintf("Syncing connects env/%s/ to what's already deployed. "+
			"It reads your infrastructure; it doesn't change it.", c.Env)
	}
	return fmt.Sprintf("Syncing writes env/%s/ from %s.yaml and connects it to what's already "+
		"deployed. It reads your infrastructure; it doesn't change it.", c.Env, c.Env)
}

// describeStateConnection renders what was found as plain lines, for terminals
// that cannot show the screen. Empty string means there is nothing worth
// saying, which is most of the time.
func describeStateConnection(c stateConnection) string {
	switch c.Status {
	case stateDeployedButDisconnected:
		var b strings.Builder
		fmt.Fprintf(&b, "\n📦 %s\n\n", deployedLine(c))
		for _, line := range strings.Split(wordWrap(missingLine(c), 72), "\n") {
			fmt.Fprintf(&b, "   %s\n", line)
		}
		fmt.Fprintf(&b, "\n   State: s3://%s/%s\n\n", c.Summary.Bucket, c.Summary.Key)
		for _, line := range strings.Split(wordWrap(syncingLine(c), 72), "\n") {
			fmt.Fprintf(&b, "   %s\n", line)
		}
		return b.String()

	case stateUnreadable:
		var b strings.Builder
		fmt.Fprintf(&b, "\n⚠️  Could not read the remote Terraform state for '%s': %v\n", c.Env, c.Err)
		b.WriteString("   This does not stop the current command — it only means meroku cannot\n")
		b.WriteString("   tell you whether this environment is already deployed.\n")
		b.WriteString("   If it is, fix AWS access (profile, SSO login, region) and run:\n")
		if c.Generated {
			fmt.Fprintf(&b, "     terraform -chdir=env/%s init\n", c.Env)
		} else {
			// env/<env>/ is not there, so telling them to run terraform in it would
			// send them to a directory that does not exist.
			fmt.Fprintf(&b, "     meroku sync %s\n", c.Env)
		}
		return b.String()
	}

	return ""
}

// announceStateConnection prints the finding without acting on it. For callers
// that run their own terraform init immediately afterwards.
func announceStateConnection(ctx context.Context, envName string, env Env, envDir string) stateConnection {
	c := probeStateConnection(ctx, envName, env, envDir, defaultRemoteStateLookup)
	if msg := describeStateConnection(c); msg != "" {
		fmt.Print(msg)
	}
	return c
}

// reconnectStateIfNeeded is the whole feature for callers that do not otherwise
// run terraform: classify, show what was found, ask, and — only on a yes —
// generate, init and check the result against what is deployed.
func reconnectStateIfNeeded(ctx context.Context, envName string, env Env, envDir string) error {
	applyAWSEnvFromConfig(env)
	c := probeStateConnection(ctx, envName, env, envDir, defaultRemoteStateLookup)
	_, err := performSync(syncRequest{
		conn:   c,
		envDir: envDir,
		// Syncing writes files and calls AWS. Nobody asked for it on this path —
		// they ran `meroku generate`, or picked an environment from a menu — so it
		// is a question, not an announcement. `meroku sync` sets this false:
		// running that command is the consent.
		ask:  true,
		gen:  defaultEnvironmentGenerator,
		init: defaultTerraformInitializer,
		plan: defaultTerraformPlanner,
	})
	return err
}

// syncDecision is the answer to "sync now?".
type syncDecision int

const (
	// syncApproved: yes.
	syncApproved syncDecision = iota
	// syncDeclined: no. Not an error, and not remembered — the question comes
	// back next time. A refusal that sticks is a preference, and nobody asked to
	// set one.
	syncDeclined
	// syncNotAsked: there is no terminal to ask on. A prompt that blocks a CI run
	// is worse than no feature, so this neither asks nor syncs.
	syncNotAsked
)

// syncConsent answers the question without a terminal. Tests inject one; it is
// also how the plain fallback is driven.
type syncConsent func(c stateConnection) syncDecision

// syncRequest is one sync: what was found, where it lives, and the four things
// that touch the outside world.
type syncRequest struct {
	conn   stateConnection
	envDir string

	// ask shows the decision screen first. False means the caller already has
	// consent.
	ask bool

	gen  environmentGenerator
	init terraformInitializer
	plan terraformPlanner

	// compareWhenLinked runs the read-only comparison even for an environment
	// that is already linked and therefore needs no sync at all.
	//
	// Only `meroku sync` sets it. On the automatic paths a linked directory is
	// left strictly alone: a terraform plan costs a minute or two and network
	// calls, and charging that to every `meroku generate` would be a tax nobody
	// asked to pay.
	compareWhenLinked bool

	// decide overrides the inline prompt. Set by tests so they never block on a
	// terminal; nil in production, where a real terminal gets the prompt and
	// everything else is never asked at all.
	decide syncConsent

	// described records that the situation has already been printed, so the
	// plain path does not repeat the paragraph the question was asked under.
	described bool
}

// performSync asks, then does the work.
//
// The two halves are deliberately different shapes. The question is an ordinary
// inline prompt in the normal flow of the terminal — the same huh select the
// environment picker uses — because taking over the screen to ask yes or no is
// a heavy answer to a small question, and it lands immediately after the picker,
// so a full-screen switch reads as the app having jumped somewhere unasked.
//
// The work is the opposite: terraform init and plan together take a minute or
// two, and a frozen cursor for that long looks like a hang, so once the user has
// said yes the full screen earns its place. Anything without a terminal — a
// pipe, a CI job, a test — gets the same information as lines.
func performSync(req syncRequest) (reconnectOutcome, error) {
	// An already-linked directory owns its own state. Unless the caller
	// explicitly wants the comparison, there is nothing here to do, show or
	// spend a terraform plan on.
	if req.conn.Status == stateAlreadyInitialised && !req.compareWhenLinked {
		return reconnectOutcome{}, nil
	}
	if req.conn.Status != stateAlreadyInitialised && req.conn.Status != stateDeployedButDisconnected {
		if msg := describeStateConnection(req.conn); msg != "" {
			fmt.Print(msg)
		}
		return reconnectOutcome{}, nil
	}

	if req.ask {
		// The facts first, as plain prose. There is no chrome to lean on here,
		// so they have to read as a paragraph — which is the same paragraph the
		// pipe and CI paths get.
		if msg := describeStateConnection(req.conn); msg != "" {
			fmt.Print(msg)
			// The prompt renders at the cursor, so without this it butts against
			// the last sentence and reads as part of the paragraph.
			fmt.Println()
			req.described = true
		}

		decide := req.decide
		if decide == nil {
			decide = promptForSyncConsent
		}
		switch decide(req.conn) {
		case syncDeclined:
			fmt.Printf("\n   Not synced. Run `meroku sync %s` when you want to.\n\n", req.conn.Env)
			return reconnectOutcome{Verdict: notSyncedVerdict(req.conn.Env)}, nil
		case syncNotAsked:
			fmt.Printf("\n   Not synced — no terminal to ask on. Run: meroku sync %s\n", req.conn.Env)
			return reconnectOutcome{Verdict: notSyncedVerdict(req.conn.Env)}, nil
		}
		// Answered. The screen below starts on the work, never on a question.
		req.ask = false
	}

	if terminalIsInteractive() {
		return startSyncScreen(req)
	}
	return plainSync(req)
}

// startSyncScreen is a variable so tests can prove what does and does not reach
// the full screen without running a Bubble Tea program.
var startSyncScreen = runSyncScreen

// terminalIsInteractive is the same seam for the terminal check itself.
var terminalIsInteractive = interactiveTerminal

// promptForSyncConsent asks the question inline, with the huh select the rest of
// meroku asks with. huh renders in the normal buffer — no alt screen — so the
// question appears under the facts and the answer stays in the scrollback.
func promptForSyncConsent(c stateConnection) syncDecision {
	if !terminalIsInteractive() {
		return syncNotAsked
	}

	const (
		answerSyncNow = "sync-now"
		answerNotNow  = "not-now"
	)

	var answer string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Sync %s now?", c.Env)).
				Options(
					huh.NewOption(fmt.Sprintf("Sync now — links env/%s to the %d resources already deployed",
						c.Env, c.Summary.ResourceCount), answerSyncNow),
					huh.NewOption(fmt.Sprintf("Not now — nothing is written; meroku asks again next time, or run `meroku sync %s`",
						c.Env), answerNotNow),
				).
				Value(&answer),
		),
	)

	if err := form.Run(); err != nil {
		// Ctrl-C or a terminal that turned out not to be usable. Either way it is
		// not a yes, and it is not something to fail the caller's command over.
		return syncDeclined
	}
	if answer == answerSyncNow {
		return syncApproved
	}
	return syncDeclined
}

// interactiveTerminal reports whether there is a human at a terminal.
//
// Both ends matter: a piped stdin has nobody to answer, and a redirected stdout
// means the screen would render into a file. CI is checked first because a
// build agent can allocate a TTY and still have nobody watching it.
func interactiveTerminal() bool {
	if os.Getenv("CI") != "" {
		return false
	}
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
}

// plainSync does the work as lines, for everything with no terminal to draw on.
func plainSync(req syncRequest) (reconnectOutcome, error) {
	c := req.conn
	if !req.described {
		if msg := describeStateConnection(c); msg != "" {
			fmt.Print(msg)
		}
	}

	var outcome reconnectOutcome
	switch c.Status {
	case stateDeployedButDisconnected:
		return runSyncSteps(c, req.envDir, req.gen, req.init, req.plan)
	case stateAlreadyInitialised:
		// Already linked, so there is nothing to write or init — only the
		// comparison, which is the half of the answer "linked" leaves out.
		outcome.Verdict = syncVerdictLead(false) + reportConfigDrift(c, req.envDir, req.plan)
	}
	return outcome, nil
}

func notSyncedVerdict(env string) string {
	return fmt.Sprintf("Not synced. Run `meroku sync %s` when you want to.", env)
}

// syncVerdictLead opens the verdict with what actually happened. An environment
// that was already linked was not synced, and saying so would be a small lie
// that costs trust in every other line.
func syncVerdictLead(initialised bool) string {
	if initialised {
		return "Synced. "
	}
	return "Connected. "
}

// applyAWSEnvFromConfig fills in AWS_PROFILE/AWS_REGION from the environment
// config when the shell has not set them.
//
// `meroku generate` runs before any profile selection, so without this both the
// state lookup and the terraform child process would fall back to the default
// profile — which is usually the wrong account, and would turn a perfectly
// readable state into a spurious "could not read". Only fills gaps: an explicit
// AWS_PROFILE in the shell always wins. Same rule AWSPreflightCheck already
// applies.
func applyAWSEnvFromConfig(env Env) {
	if os.Getenv("AWS_PROFILE") == "" && env.AWSProfile != "" {
		os.Setenv("AWS_PROFILE", env.AWSProfile)
	}
	if os.Getenv("AWS_REGION") == "" && env.Region != "" {
		os.Setenv("AWS_REGION", env.Region)
	}
	if os.Getenv("AWS_DEFAULT_REGION") == "" && env.Region != "" {
		os.Setenv("AWS_DEFAULT_REGION", env.Region)
	}
}

// probeStateConnection wraps inspection with the opt-out and the timeout.
func probeStateConnection(ctx context.Context, envName string, env Env, envDir string, lookup remoteStateLookup) stateConnection {
	if os.Getenv(merokuSkipStateReconnect) != "" {
		return stateConnection{Env: envName, Status: stateSkipped}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, remoteStateTimeout)
	defer cancel()
	return inspectStateConnection(ctx, envName, env, envDir, lookup)
}

// reconnectOutcome is what a reconnect ended up doing. Returned for callers that
// have to state a verdict — `meroku sync` — rather than only succeed or fail.
type reconnectOutcome struct {
	Generated   bool   // env/<env>/ was regenerated from <env>.yaml
	Initialised bool   // terraform init ran and succeeded
	Verdict     string // one line the user can act on; empty when nothing was done
}

// applyStateConnection reports the finding and, for a deployed-but-unlinked
// environment, syncs it. Callers that have already described the situation call
// runSyncSteps directly.
func applyStateConnection(c stateConnection, envDir string, gen environmentGenerator, init terraformInitializer, plan terraformPlanner) (reconnectOutcome, error) {
	if msg := describeStateConnection(c); msg != "" {
		fmt.Print(msg)
	}
	if c.Status != stateDeployedButDisconnected {
		return reconnectOutcome{}, nil
	}
	return runSyncSteps(c, envDir, gen, init, plan)
}

// runSyncSteps performs the sync as plain lines: write env/<env>/ if it is
// missing, terraform init, then a read-only plan. Errors are returned rather
// than hidden; callers decide whether their own command should still be
// considered successful.
func runSyncSteps(c stateConnection, envDir string, gen environmentGenerator, init terraformInitializer, plan terraformPlanner) (reconnectOutcome, error) {
	var outcome reconnectOutcome

	// The directory is not there, so there is nothing for terraform init to init.
	// Writing it is what `meroku generate` does and no more — it renders
	// env/<env>/ from <env>.yaml and touches nothing else.
	//
	// This calls the generation step directly, never handleGenerateCommand: that
	// command ends by calling the reconnect, so routing a sync through it would
	// re-enter the code path that is currently running. The sync calls the step;
	// only the command calls the command.
	if !c.Generated {
		fmt.Printf("\n   Writing env/%s from %s.yaml...\n", c.Env, c.Env)
		if err := gen(c.Env); err != nil {
			fmt.Printf("\n❌ Could not write env/%s: %v\n", c.Env, err)
			fmt.Println("   Your infrastructure and its state are untouched.")
			fmt.Printf("   Fix the problem above, then run: meroku generate %s\n", c.Env)
			outcome.Verdict = fmt.Sprintf("env/%s could not be written, so it is still unlinked.", c.Env)
			return outcome, err
		}
		outcome.Generated = true
		fmt.Printf("   ✓ Wrote env/%s/main.tf\n", c.Env)
	}

	fmt.Println("\n   Running terraform init...")
	if err := init(envDir); err != nil {
		if errors.Is(err, errTerraformNotInstalled) {
			fmt.Println("\n❌ Terraform is not installed, so meroku cannot finish the sync.")
			fmt.Println("   Your infrastructure and its state are untouched.")
			fmt.Println("   Install Terraform (https://developer.hashicorp.com/terraform/install),")
			fmt.Printf("   then run: terraform -chdir=%s init\n", envDir)
			outcome.Verdict = "Terraform is not installed, so this environment is still unlinked."
			return outcome, err
		}
		fmt.Printf("\n❌ Sync failed: %v\n", err)
		fmt.Println("   Your infrastructure and its state are untouched.")
		fmt.Printf("   Try manually: terraform -chdir=%s init\n", envDir)
		outcome.Verdict = "terraform init failed, so this environment is still unlinked."
		return outcome, err
	}
	outcome.Initialised = true

	fmt.Printf("\n✅ Linked — env/%s now tracks the %d deployed resources.\n", c.Env, c.Summary.ResourceCount)

	// init restored the link; it says nothing about whether the local
	// configuration still describes what is actually deployed. That gap is real
	// here: env/<env> is written from a YAML that may have been migrated several
	// schema versions since the last apply, so the terraform just written is not
	// necessarily the terraform that produced those resources. Only a plan can
	// tell the difference.
	//
	// Diagnosis only — nothing on this path ever applies.
	outcome.Verdict = syncVerdictLead(true) + reportConfigDrift(c, envDir, plan)
	return outcome, nil
}

// reportConfigDrift runs a plan after a sync and summarises it. It returns a
// one-line verdict for callers that print one; the detail is printed here.
//
// Never returns an error: the command that triggered the sync has already done
// its job, and a plan that cannot run is information, not a failure.
func reportConfigDrift(c stateConnection, envDir string, plan terraformPlanner) string {
	fmt.Println("\n🔍 Comparing the local configuration with what is deployed. This runs")
	fmt.Println("   `terraform plan`, which takes a minute or two and changes nothing.")

	summary, err := plan(envDir)
	if err != nil {
		var locked *stateLockedError
		if errors.As(err, &locked) {
			fmt.Println("\n🔒 The Terraform state is locked, so the comparison was skipped.")
			fmt.Println("   Someone else is running terraform against this environment, or a")
			fmt.Println("   previous run did not release its lock. The sync above still finished.")
			if locked.ID != "" {
				fmt.Printf("   Lock ID: %s\n", locked.ID)
			}
			fmt.Printf("   Retry later with: terraform -chdir=%s plan\n", envDir)
			return driftVerdict(summary, err)
		}

		fmt.Printf("\n⚠️  Could not run terraform plan: %v\n", err)
		fmt.Println("   The sync above still finished. This only means meroku could not")
		fmt.Println("   compare — your infrastructure and its state are untouched.")
		fmt.Printf("   Try it yourself: terraform -chdir=%s plan\n", envDir)
		return driftVerdict(summary, err)
	}

	if summary.Total() == 0 {
		// The resource count is only known when the remote state was read, which
		// the already-linked path does not always manage. Do not print "all 0".
		if c.Summary.ResourceCount > 0 {
			fmt.Printf("\n✅ No changes. The local configuration matches all %d deployed resources.\n", c.Summary.ResourceCount)
		} else {
			fmt.Println("\n✅ No changes. The local configuration matches what is deployed.")
		}
		fmt.Println("   There is nothing to apply.")
		return driftVerdict(summary, nil)
	}

	// Lead with destruction. A total that buries "3 to destroy" under "12 changes"
	// is the kind of summary someone approves without reading.
	if summary.Destroy > 0 || summary.Replace > 0 {
		fmt.Println("\n🛑 The local configuration would DESTROY existing resources.")
		if summary.Replace > 0 {
			fmt.Printf("   %d to replace (destroyed and recreated)\n", summary.Replace)
		}
		if destroyOnly := summary.Destroy - summary.Replace; destroyOnly > 0 {
			fmt.Printf("   %d to destroy outright\n", destroyOnly)
		}
	} else {
		fmt.Println("\nℹ️  The local configuration differs from what is deployed.")
	}

	fmt.Printf("\n   Plan: %d to add, %d to change, %d to destroy.\n", summary.Add, summary.Change, summary.Destroy)
	fmt.Println("\n   NOTHING HAS BEEN APPLIED. Review before deploying:")
	fmt.Printf("     terraform -chdir=%s plan\n", envDir)

	return driftVerdict(summary, nil)
}

// driftVerdict is the one line that states what the comparison found. It is
// shared by the screen and by the plain path so the two can never disagree
// about what a plan meant.
func driftVerdict(summary planSummary, err error) string {
	if err != nil {
		var locked *stateLockedError
		if errors.As(err, &locked) {
			return "The comparison was skipped because the state is locked — retry it later."
		}
		return "meroku could not compare the configuration with what is deployed."
	}
	if summary.Total() == 0 {
		return "the configuration matches what is deployed — nothing to apply."
	}
	if summary.Destroy > 0 {
		return fmt.Sprintf("the configuration would DESTROY %d deployed resources (%d to add, %d to change) — review before applying.",
			summary.Destroy, summary.Add, summary.Change)
	}
	return fmt.Sprintf("the configuration differs from what is deployed (%d to add, %d to change) — review before applying.",
		summary.Add, summary.Change)
}

// planSummary is the shape of a plan, not its contents.
type planSummary struct {
	Add     int
	Change  int
	Destroy int
	// Replace counts resources that appear in both Add and Destroy because
	// terraform intends to recreate them.
	Replace int
}

func (p planSummary) Total() int { return p.Add + p.Change + p.Destroy }

// stateLockedError marks the one plan failure worth naming: a common,
// recoverable situation that a raw terraform error describes badly.
type stateLockedError struct {
	ID string
}

func (e *stateLockedError) Error() string {
	if e.ID != "" {
		return fmt.Sprintf("terraform state is locked (lock ID %s)", e.ID)
	}
	return "terraform state is locked"
}

var (
	planSummaryLineRe = regexp.MustCompile(`Plan:\s+(\d+) to add,\s+(\d+) to change,\s+(\d+) to destroy`)
	stateLockIDRe     = regexp.MustCompile(`(?m)^\s*ID:\s+([0-9a-fA-F-]{8,})`)
)

// terraformPlanInDir runs a read-only plan and summarises it.
//
// The plan is written to a file only so `terraform show -json` can give exact
// counts — replacements in particular are invisible in the human summary line.
// The file is removed either way, and no apply ever reads it.
func terraformPlanInDir(dir string) (planSummary, error) {
	if _, err := exec.LookPath("terraform"); err != nil {
		return planSummary{}, errTerraformNotInstalled
	}

	const planFile = ".meroku-reconnect.tfplan"
	defer os.Remove(filepath.Join(dir, planFile))

	out, err := exec.Command("terraform", terraformArgs(dir, "plan", "-input=false", "-no-color", "-out="+planFile)...).CombinedOutput()
	clean := stripAnsiEscapeCodes(string(out))
	if err != nil {
		if m := stateLockIDRe.FindStringSubmatch(clean); isStateLockFailure(clean) {
			locked := &stateLockedError{}
			if len(m) > 1 {
				locked.ID = m[1]
			}
			return planSummary{}, locked
		}
		return planSummary{}, fmt.Errorf("%w: %s", err, lastMeaningfulLines(clean, 12))
	}

	jsonOut, showErr := exec.Command("terraform", terraformArgs(dir, "show", "-json", planFile)...).Output()
	if showErr != nil {
		// The plan itself succeeded, so fall back to its printed summary rather
		// than reporting a failure we do not have.
		return parsePlanTextSummary(clean)
	}
	return summarizePlanJSON(jsonOut)
}

// terraformArgs builds an argv with -chdir in front, which has to precede the
// subcommand.
func terraformArgs(dir string, args ...string) []string {
	if dir == "" || dir == "." {
		return args
	}
	return append([]string{"-chdir=" + dir}, args...)
}

func isStateLockFailure(output string) bool {
	for _, marker := range []string{
		"Error acquiring the state lock",
		"state blob is already locked",
		"ConditionalCheckFailedException",
		"Lock Info:",
	} {
		if strings.Contains(output, marker) {
			return true
		}
	}
	return false
}

// summarizePlanJSON counts actions in `terraform show -json` output. A resource
// terraform intends to recreate carries both delete and create, and is counted
// in all three of Add, Destroy and Replace.
func summarizePlanJSON(data []byte) (planSummary, error) {
	var plan TerraformPlanVisual
	if err := json.Unmarshal(data, &plan); err != nil {
		return planSummary{}, fmt.Errorf("could not parse the plan JSON: %w", err)
	}

	var s planSummary
	for _, change := range plan.ResourceChanges {
		actions := change.Change.Actions
		if len(actions) == 0 {
			continue
		}
		if len(actions) == 2 && actions[0] != actions[1] &&
			(actions[0] == "delete" || actions[1] == "delete") &&
			(actions[0] == "create" || actions[1] == "create") {
			s.Replace++
			s.Add++
			s.Destroy++
			continue
		}
		switch actions[0] {
		case "create":
			s.Add++
		case "update":
			s.Change++
		case "delete":
			s.Destroy++
		}
	}
	return s, nil
}

// parsePlanTextSummary reads terraform's own one-line summary. Used only when
// the JSON path is unavailable; it cannot see replacements.
func parsePlanTextSummary(output string) (planSummary, error) {
	if m := planSummaryLineRe.FindStringSubmatch(output); m != nil {
		add, _ := strconv.Atoi(m[1])
		change, _ := strconv.Atoi(m[2])
		destroy, _ := strconv.Atoi(m[3])
		return planSummary{Add: add, Change: change, Destroy: destroy}, nil
	}
	if strings.Contains(output, "No changes.") || strings.Contains(output, "no changes are needed") {
		return planSummary{}, nil
	}
	return planSummary{}, errors.New("could not read the plan summary from terraform's output")
}

// lastMeaningfulLines keeps error output short enough to read.
func lastMeaningfulLines(output string, n int) string {
	var kept []string
	for _, line := range strings.Split(output, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	if len(kept) > n {
		kept = kept[len(kept)-n:]
	}
	return strings.Join(kept, " | ")
}

// terraformInitInDir runs a plain `terraform init` against dir.
//
// Deliberately plain: no -reconfigure, no -migrate-state, no -force-copy. This
// reconnects a directory to the backend it already declares; anything that could
// move, copy or overwrite state is out of scope and would risk the very thing
// the user is afraid of losing.
func terraformInitInDir(dir string) error {
	if _, err := exec.LookPath("terraform"); err != nil {
		return errTerraformNotInstalled
	}

	args := []string{}
	if dir != "" && dir != "." {
		args = append(args, "-chdir="+dir)
	}
	args = append(args, "init", "-input=false")

	cmd := exec.Command("terraform", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			fmt.Println("   " + strings.TrimSpace(line))
		}
		return fmt.Errorf("terraform init failed: %w", err)
	}
	return nil
}

// terraformStateFile is the subset of the state format (v4) we read. Decoding
// only these fields keeps us tolerant of format additions.
type terraformStateFile struct {
	Version          int    `json:"version"`
	TerraformVersion string `json:"terraform_version"`
	Serial           int64  `json:"serial"`
	Resources        []struct {
		Module    string            `json:"module"`
		Mode      string            `json:"mode"`
		Type      string            `json:"type"`
		Name      string            `json:"name"`
		Instances []json.RawMessage `json:"instances"`
	} `json:"resources"`
}

// summarizeTerraformState counts what a state tracks.
//
// It counts instances, not resource blocks, so the number matches what
// `terraform state list` prints: count/for_each expansions and module resources
// each appear once, and data sources are included exactly as that command
// includes them.
func summarizeTerraformState(data []byte) (remoteStateSummary, error) {
	var f terraformStateFile
	if err := json.Unmarshal(data, &f); err != nil {
		return remoteStateSummary{}, fmt.Errorf("remote state is not valid Terraform state JSON: %w", err)
	}

	s := remoteStateSummary{
		Serial:           f.Serial,
		TerraformVersion: f.TerraformVersion,
		ManagedTypes:     map[string]bool{},
	}
	for _, r := range f.Resources {
		n := len(r.Instances)
		if n == 0 {
			continue
		}
		if r.Mode == "data" {
			s.DataCount += n
		} else {
			s.ManagedCount += n
			s.ManagedTypes[r.Type] = true
		}
	}
	s.ResourceCount = s.ManagedCount + s.DataCount
	return s, nil
}

// lookupRemoteStateFromS3 reads env.StateFile out of env.StateBucket.
//
// Read-only by construction: a single GetObject, no writes, no locks.
func lookupRemoteStateFromS3(ctx context.Context, env Env) (remoteStateSummary, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(env.Region))
	if err != nil {
		return remoteStateSummary{}, fmt.Errorf("failed to load AWS configuration: %w", err)
	}

	client := s3.NewFromConfig(cfg)
	out, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(env.StateBucket),
		Key:    aws.String(env.StateFile),
	})
	if err != nil {
		if isRemoteStateAbsentError(err) {
			return remoteStateSummary{}, errRemoteStateAbsent
		}
		return remoteStateSummary{}, err
	}
	defer out.Body.Close()

	data, err := io.ReadAll(io.LimitReader(out.Body, maxRemoteStateBytes))
	if err != nil {
		return remoteStateSummary{}, fmt.Errorf("failed to read s3://%s/%s: %w", env.StateBucket, env.StateFile, err)
	}

	summary, err := summarizeTerraformState(data)
	if err != nil {
		return remoteStateSummary{}, err
	}
	summary.Bucket = env.StateBucket
	summary.Key = env.StateFile
	summary.Region = env.Region
	return summary, nil
}

// isRemoteStateAbsentError distinguishes "there is nothing there yet" (a new
// project — stay quiet) from "something is wrong" (say so). Only genuine
// absence counts; AccessDenied, expired SSO and redirects are all failures to
// read, not evidence of a fresh project.
func isRemoteStateAbsentError(err error) bool {
	var noSuchKey *s3types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return true
	}
	var noSuchBucket *s3types.NoSuchBucket
	if errors.As(err, &noSuchBucket) {
		return true
	}
	var notFound *s3types.NotFound
	if errors.As(err, &notFound) {
		return true
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NoSuchBucket", "NotFound":
			return true
		}
	}
	return false
}
