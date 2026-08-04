package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/smithy-go"
)

// Every test here drives inspectStateConnection/applyStateConnection with an
// injected lookup and an injected initializer. Nothing in this file can reach
// AWS or run terraform.

func testEnv() Env {
	return Env{
		Env:         "dev",
		Region:      "ap-southeast-2",
		StateBucket: "state-bucket-example-dev-00000",
		StateFile:   "state.tfstate",
	}
}

// generatedEnvDir creates env/<name> the way `meroku generate` leaves it:
// terraform files present, no .terraform.
func generatedEnvDir(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "env", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating env dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("# generated\n"), 0o644); err != nil {
		t.Fatalf("writing main.tf: %v", err)
	}
	return dir
}

func lookupReturning(summary remoteStateSummary) remoteStateLookup {
	return func(context.Context, Env) (remoteStateSummary, error) { return summary, nil }
}

func lookupFailing(err error) remoteStateLookup {
	return func(context.Context, Env) (remoteStateSummary, error) { return remoteStateSummary{}, err }
}

// lookupNeverCalled fails the test if the state is read at all. Used to prove
// which situations cost no AWS call.
func lookupNeverCalled(t *testing.T) remoteStateLookup {
	t.Helper()
	return func(context.Context, Env) (remoteStateSummary, error) {
		t.Fatal("remote state was read when it should not have been")
		return remoteStateSummary{}, nil
	}
}

// recordingInit stands in for terraform init.
type recordingInit struct {
	calls []string
	err   error
}

func (r *recordingInit) run(dir string) error {
	r.calls = append(r.calls, dir)
	return r.err
}

// recordingPlan stands in for terraform plan.
type recordingPlan struct {
	calls   []string
	summary planSummary
	err     error
}

func (r *recordingPlan) run(dir string) (planSummary, error) {
	r.calls = append(r.calls, dir)
	return r.summary, r.err
}

// recordingGen stands in for `meroku generate`. It writes what real generation
// leaves behind — env/<env>/main.tf under root — so the rest of the recovery
// sees the same filesystem it would in production.
type recordingGen struct {
	root  string
	calls []string
	err   error
}

func (r *recordingGen) run(envName string) error {
	r.calls = append(r.calls, envName)
	if r.err != nil {
		return r.err
	}
	dir := filepath.Join(r.root, "env", envName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "main.tf"), []byte("# generated\n"), 0o644)
}

// genNeverCalled fails the test if anything is generated. Used to prove that a
// directory which is already on disk is never rewritten.
func genNeverCalled(t *testing.T) environmentGenerator {
	t.Helper()
	return func(envName string) error {
		t.Fatalf("env/%s was regenerated when it should not have been", envName)
		return nil
	}
}

// flattenText collapses wrapping so a sentence can be asserted on as a
// sentence. The copy is wrapped for the terminal; the wording is what matters.
func flattenText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// assertNoRescueLanguage fails if the text frames a routine first run as an
// incident. env/ is generated output and is not committed, so a checkout
// without it is the ordinary case — reassurance here would invent the problem
// it then soothes.
func assertNoRescueLanguage(t *testing.T, text string) {
	t.Helper()
	for _, banned := range []string{
		"still there",
		"Nothing has been lost",
		"nothing has been lost",
		"Recovering",
		"recovered",
		"Don't worry",
		"don't panic",
	} {
		if strings.Contains(text, banned) {
			t.Errorf("rescue language %q in routine setup copy:\n%s", banned, text)
		}
	}
}

// captureOutput collects everything printed to stdout while fn runs, so the
// wording a worried user sees can be asserted on.
func captureOutput(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf strings.Builder
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	os.Stdout = original
	w.Close()
	out := <-done
	r.Close()
	return out
}

// --- detection -------------------------------------------------------------

// The incident: 85 resources in the backend, no local .terraform.
func TestDeployedButDisconnectedTriggersReconnect(t *testing.T) {
	dir := generatedEnvDir(t, "dev")
	lookup := lookupReturning(remoteStateSummary{
		Bucket:           "state-bucket-example-dev-00000",
		Key:              "state.tfstate",
		Region:           "ap-southeast-2",
		ResourceCount:    85,
		ManagedCount:     67,
		DataCount:        18,
		Serial:           8,
		TerraformVersion: "1.15.8",
	})

	c := inspectStateConnection(context.Background(), "dev", testEnv(), dir, lookup)
	if c.Status != stateDeployedButDisconnected {
		t.Fatalf("status = %v, want stateDeployedButDisconnected", c.Status)
	}
	if c.Summary.ResourceCount != 85 {
		t.Fatalf("ResourceCount = %d, want 85", c.Summary.ResourceCount)
	}

	// What is true, what is missing and why, what syncing does — and nothing
	// about rescue.
	msg := describeStateConnection(c)
	for _, want := range []string{
		"dev is deployed — 85 resources in ap-southeast-2",
		"git doesn't track",
		"s3://state-bucket-example-dev-00000/state.tfstate",
		"It reads your infrastructure; it doesn't change it",
	} {
		if !strings.Contains(flattenText(msg), want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
	assertNoRescueLanguage(t, msg)

	init := &recordingInit{}
	plan := &recordingPlan{}
	if _, err := applyStateConnection(c, dir, genNeverCalled(t), init.run, plan.run); err != nil {
		t.Fatalf("applyStateConnection: %v", err)
	}
	if len(init.calls) != 1 || init.calls[0] != dir {
		t.Fatalf("init calls = %v, want exactly [%s]", init.calls, dir)
	}
	if len(plan.calls) != 1 || plan.calls[0] != dir {
		t.Fatalf("plan calls = %v, want exactly [%s]", plan.calls, dir)
	}
}

// A project that has never been deployed must hear nothing and be blocked by
// nothing.
func TestFreshProjectNoStateStaysSilent(t *testing.T) {
	dir := generatedEnvDir(t, "dev")

	c := inspectStateConnection(context.Background(), "dev", testEnv(), dir, lookupFailing(errRemoteStateAbsent))
	if c.Status != stateFresh {
		t.Fatalf("status = %v, want stateFresh", c.Status)
	}
	if msg := describeStateConnection(c); msg != "" {
		t.Fatalf("fresh project produced output:\n%s", msg)
	}

	init := &recordingInit{}
	plan := &recordingPlan{}
	if _, err := applyStateConnection(c, dir, genNeverCalled(t), init.run, plan.run); err != nil {
		t.Fatalf("applyStateConnection: %v", err)
	}
	if len(init.calls) != 0 {
		t.Fatalf("init was run for a fresh project: %v", init.calls)
	}
	if len(plan.calls) != 0 {
		t.Fatalf("plan was run for a fresh project: %v", plan.calls)
	}
}

// A state that exists but tracks nothing (destroyed, or applied to zero
// resources) is the same story: say nothing about resources that do not exist.
func TestEmptyStateCountsAsFresh(t *testing.T) {
	dir := generatedEnvDir(t, "dev")

	c := inspectStateConnection(context.Background(), "dev", testEnv(), dir,
		lookupReturning(remoteStateSummary{Bucket: "b", Key: "k", Region: "r", ResourceCount: 0}))
	if c.Status != stateFresh {
		t.Fatalf("status = %v, want stateFresh", c.Status)
	}
	if msg := describeStateConnection(c); msg != "" {
		t.Fatalf("empty state produced output:\n%s", msg)
	}
}

// Expired credentials, wrong profile, a bucket in another account: report it,
// do not fail the command.
func TestUnreachableBucketDegradesWithoutFailing(t *testing.T) {
	dir := generatedEnvDir(t, "dev")
	lookupErr := errors.New("operation error S3: GetObject, https response error StatusCode: 403, AccessDenied")

	c := inspectStateConnection(context.Background(), "dev", testEnv(), dir, lookupFailing(lookupErr))
	if c.Status != stateUnreadable {
		t.Fatalf("status = %v, want stateUnreadable", c.Status)
	}

	msg := describeStateConnection(c)
	for _, want := range []string{"Could not read", "AccessDenied", "does not stop the current command", "terraform -chdir=env/dev init"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}

	init := &recordingInit{}
	plan := &recordingPlan{}
	if _, err := applyStateConnection(c, dir, genNeverCalled(t), init.run, plan.run); err != nil {
		t.Fatalf("an unreadable bucket must not fail the command, got: %v", err)
	}
	if len(init.calls) != 0 {
		t.Fatalf("init was run despite an unreadable bucket: %v", init.calls)
	}
	if len(plan.calls) != 0 {
		t.Fatalf("plan was run despite an unreadable bucket: %v", plan.calls)
	}
}

// An initialised directory owns its own state. We do not read it, touch it or
// re-init it.
func TestAlreadyInitialisedIsLeftAlone(t *testing.T) {
	dir := generatedEnvDir(t, "dev")
	if err := os.MkdirAll(filepath.Join(dir, ".terraform"), 0o755); err != nil {
		t.Fatalf("creating .terraform: %v", err)
	}
	stateFile := filepath.Join(dir, "terraform.tfstate")
	if err := os.WriteFile(stateFile, []byte(`{"serial":1}`), 0o644); err != nil {
		t.Fatalf("writing local state: %v", err)
	}

	c := inspectStateConnection(context.Background(), "dev", testEnv(), dir, lookupNeverCalled(t))
	if c.Status != stateAlreadyInitialised {
		t.Fatalf("status = %v, want stateAlreadyInitialised", c.Status)
	}
	if msg := describeStateConnection(c); msg != "" {
		t.Fatalf("initialised directory produced output:\n%s", msg)
	}

	init := &recordingInit{}
	plan := &recordingPlan{}
	if _, err := applyStateConnection(c, dir, genNeverCalled(t), init.run, plan.run); err != nil {
		t.Fatalf("applyStateConnection: %v", err)
	}
	if len(init.calls) != 0 {
		t.Fatalf("init was run on an initialised directory: %v", init.calls)
	}
	// A plan on every command would be slow and would make network calls nobody
	// asked for. It happens only after an actual reconnect.
	if len(plan.calls) != 0 {
		t.Fatalf("plan was run on an initialised directory: %v", plan.calls)
	}

	// And nothing on disk moved.
	if _, err := os.Stat(filepath.Join(dir, ".terraform")); err != nil {
		t.Fatalf(".terraform disappeared: %v", err)
	}
	data, err := os.ReadFile(stateFile)
	if err != nil || string(data) != `{"serial":1}` {
		t.Fatalf("local state was modified: %q, err %v", string(data), err)
	}
}

// THE HEADLINE CASE. A fresh `git clone` of a deployed project: dev.yaml is
// committed and names the backend, env/ has never existed on this machine, and
// 85 resources are running in AWS.
//
// meroku used to give up here — env/dev/ was missing, so it said nothing and the
// infrastructure looked gone. A missing directory is now what a new checkout
// looks like, not evidence of an empty account, so the backend gets asked and the
// environment is recovered end to end: generate, init, plan.
func TestFreshCloneWithNoEnvDirectoryRecoversFully(t *testing.T) {
	root := t.TempDir() // a clone: <env>.yaml present, no env/ directory at all
	envDir := filepath.Join(root, "env", "dev")

	c := inspectStateConnection(context.Background(), "dev", testEnv(), envDir,
		lookupReturning(remoteStateSummary{
			Bucket:           "state-bucket-example-dev-00000",
			Key:              "state.tfstate",
			Region:           "ap-southeast-2",
			ResourceCount:    85,
			ManagedCount:     67,
			DataCount:        18,
			Serial:           8,
			TerraformVersion: "1.15.8",
		}))

	if c.Status != stateDeployedButDisconnected {
		t.Fatalf("status = %v, want stateDeployedButDisconnected", c.Status)
	}
	if c.Generated {
		t.Fatal("Generated = true, but env/dev does not exist")
	}

	// The message states the situation and what syncing would do, in that
	// order, and says why a checkout has no link — without implying a loss.
	msg := describeStateConnection(c)
	for _, want := range []string{
		"dev is deployed — 85 resources in ap-southeast-2",
		"This checkout has no link to them yet",
		"env/dev/, which git doesn't track",
		"s3://state-bucket-example-dev-00000/state.tfstate",
		"Syncing writes env/dev/ from dev.yaml",
		"It reads your infrastructure; it doesn't change it",
	} {
		if !strings.Contains(flattenText(msg), want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
	assertNoRescueLanguage(t, msg)

	// Facts that cannot change a yes/no answer stay off the decision screen.
	for _, noise := range []string{"Serial", "Written by", "data sources"} {
		if strings.Contains(msg, noise) {
			t.Errorf("%q does not inform the decision and should not be on this screen:\n%s", noise, msg)
		}
	}

	gen := &recordingGen{root: root}
	init := &recordingInit{}
	plan := &recordingPlan{}

	var out string
	var outcome reconnectOutcome
	var err error
	out = captureOutput(t, func() {
		outcome, err = applyStateConnection(c, envDir, gen.run, init.run, plan.run)
	})
	if err != nil {
		t.Fatalf("applyStateConnection: %v", err)
	}

	if len(gen.calls) != 1 || gen.calls[0] != "dev" {
		t.Fatalf("generate calls = %v, want exactly [dev]", gen.calls)
	}
	if len(init.calls) != 1 || init.calls[0] != envDir {
		t.Fatalf("init calls = %v, want exactly [%s]", init.calls, envDir)
	}
	if len(plan.calls) != 1 || plan.calls[0] != envDir {
		t.Fatalf("plan calls = %v, want exactly [%s]", plan.calls, envDir)
	}
	if _, err := os.Stat(filepath.Join(envDir, "main.tf")); err != nil {
		t.Fatalf("env/dev/main.tf was not restored: %v", err)
	}
	if !outcome.Generated || !outcome.Initialised {
		t.Fatalf("outcome = %+v, want generated and initialised", outcome)
	}
	if !strings.Contains(outcome.Verdict, "Synced.") {
		t.Errorf("verdict = %q, want it to say the environment was synced", outcome.Verdict)
	}

	// Order matters: write, then init, then the comparison.
	written := strings.Index(out, "Writing env/dev")
	inited := strings.Index(out, "Running terraform init")
	linked := strings.Index(out, "Linked — env/dev now tracks the 85 deployed resources")
	if written < 0 || inited < 0 || linked < 0 {
		t.Fatalf("the sync did not report its steps:\n%s", out)
	}
	if !(written < inited && inited < linked) {
		t.Fatalf("sync steps reported out of order:\n%s", out)
	}
}

// The same gap, one step further along: the directory is there but was never
// initialised. Recovery is identical minus the generate.
func TestMissingAndUninitialisedRecoverTheSameWay(t *testing.T) {
	generated := generatedEnvDir(t, "dev")
	missing := filepath.Join(t.TempDir(), "env", "dev")

	summary := remoteStateSummary{Bucket: "b", Key: "k", Region: "r", ResourceCount: 85}

	a := inspectStateConnection(context.Background(), "dev", testEnv(), generated, lookupReturning(summary))
	b := inspectStateConnection(context.Background(), "dev", testEnv(), missing, lookupReturning(summary))

	if a.Status != b.Status {
		t.Fatalf("a missing directory (%v) and an uninitialised one (%v) must classify the same", b.Status, a.Status)
	}
	if a.Status != stateDeployedButDisconnected {
		t.Fatalf("status = %v, want stateDeployedButDisconnected", a.Status)
	}
	if !a.Generated || b.Generated {
		t.Fatalf("Generated flags wrong: existing=%v missing=%v", a.Generated, b.Generated)
	}
}

// A missing directory with nothing deployed is a genuinely new environment, and
// the one thing meroku must not do is start writing files into it.
func TestMissingEnvDirectoryWithNoRemoteStateGeneratesNothing(t *testing.T) {
	root := t.TempDir()
	envDir := filepath.Join(root, "env", "dev")

	for name, lookup := range map[string]remoteStateLookup{
		"no state object": lookupFailing(errRemoteStateAbsent),
		"empty state":     lookupReturning(remoteStateSummary{Bucket: "b", Key: "k", Region: "r"}),
	} {
		t.Run(name, func(t *testing.T) {
			c := inspectStateConnection(context.Background(), "dev", testEnv(), envDir, lookup)
			if c.Status != stateFresh {
				t.Fatalf("status = %v, want stateFresh", c.Status)
			}
			if msg := describeStateConnection(c); msg != "" {
				t.Fatalf("a new environment produced output:\n%s", msg)
			}

			init := &recordingInit{}
			plan := &recordingPlan{}
			if _, err := applyStateConnection(c, envDir, genNeverCalled(t), init.run, plan.run); err != nil {
				t.Fatalf("applyStateConnection: %v", err)
			}
			if len(init.calls) != 0 || len(plan.calls) != 0 {
				t.Fatalf("terraform ran for a new environment: init=%v plan=%v", init.calls, plan.calls)
			}
			if _, err := os.Stat(envDir); !os.IsNotExist(err) {
				t.Fatalf("env/dev was created for an environment that has never been deployed (err %v)", err)
			}
		})
	}
}

// Recovery that cannot regenerate must stop before terraform, say so, and leave
// the state alone.
func TestGenerateFailureStopsBeforeInit(t *testing.T) {
	root := t.TempDir()
	envDir := filepath.Join(root, "env", "dev")

	c := inspectStateConnection(context.Background(), "dev", testEnv(), envDir,
		lookupReturning(remoteStateSummary{Bucket: "b", Key: "k", Region: "r", ResourceCount: 85}))

	genErr := errors.New("environment file 'dev.yaml' not found")
	gen := &recordingGen{root: root, err: genErr}
	init := &recordingInit{}
	plan := &recordingPlan{}

	var outcome reconnectOutcome
	var err error
	out := captureOutput(t, func() {
		outcome, err = applyStateConnection(c, envDir, gen.run, init.run, plan.run)
	})

	if !errors.Is(err, genErr) {
		t.Fatalf("err = %v, want it to wrap %v", err, genErr)
	}
	if len(init.calls) != 0 || len(plan.calls) != 0 {
		t.Fatalf("terraform ran after a failed generate: init=%v plan=%v", init.calls, plan.calls)
	}
	if outcome.Generated || outcome.Initialised {
		t.Fatalf("outcome = %+v, want nothing done", outcome)
	}
	if !strings.Contains(out, "untouched") {
		t.Errorf("output must say the infrastructure is untouched:\n%s", out)
	}
}

// A missing directory with no backend in the config costs no AWS call: there is
// nothing to look up.
func TestMissingEnvDirectoryWithoutBackendMakesNoAWSCall(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "env", "dev")

	c := inspectStateConnection(context.Background(), "dev", Env{Env: "dev"}, missing, lookupNeverCalled(t))
	if c.Status != stateNoBackendConfigured {
		t.Fatalf("status = %v, want stateNoBackendConfigured", c.Status)
	}
	if msg := describeStateConnection(c); msg != "" {
		t.Fatalf("an unconfigured environment produced output:\n%s", msg)
	}
}

func TestNoBackendConfiguredMakesNoAWSCall(t *testing.T) {
	dir := generatedEnvDir(t, "dev")

	for name, env := range map[string]Env{
		"no bucket": {Region: "ap-southeast-2", StateFile: "state.tfstate"},
		"no key":    {Region: "ap-southeast-2", StateBucket: "b"},
		"no region": {StateBucket: "b", StateFile: "state.tfstate"},
	} {
		t.Run(name, func(t *testing.T) {
			c := inspectStateConnection(context.Background(), "dev", env, dir, lookupNeverCalled(t))
			if c.Status != stateNoBackendConfigured {
				t.Fatalf("status = %v, want stateNoBackendConfigured", c.Status)
			}
			if msg := describeStateConnection(c); msg != "" {
				t.Fatalf("unconfigured backend produced output:\n%s", msg)
			}
		})
	}
}

// --- init failures ---------------------------------------------------------

func TestReconnectFailureIsReturnedNotSwallowed(t *testing.T) {
	dir := generatedEnvDir(t, "dev")
	c := inspectStateConnection(context.Background(), "dev", testEnv(), dir,
		lookupReturning(remoteStateSummary{Bucket: "b", Key: "k", Region: "r", ResourceCount: 85}))

	initErr := errors.New("terraform init failed: exit status 1")
	init := &recordingInit{err: initErr}
	plan := &recordingPlan{}

	_, err := applyStateConnection(c, dir, genNeverCalled(t), init.run, plan.run)
	if !errors.Is(err, initErr) {
		t.Fatalf("err = %v, want it to wrap %v", err, initErr)
	}
	if len(init.calls) != 1 {
		t.Fatalf("init calls = %v, want 1", init.calls)
	}
	if len(plan.calls) != 0 {
		t.Fatalf("plan was run after a failed init: %v", plan.calls)
	}
}

func TestMissingTerraformIsReportedNotPanicked(t *testing.T) {
	dir := generatedEnvDir(t, "dev")
	c := inspectStateConnection(context.Background(), "dev", testEnv(), dir,
		lookupReturning(remoteStateSummary{Bucket: "b", Key: "k", Region: "r", ResourceCount: 85}))

	init := &recordingInit{err: errTerraformNotInstalled}
	plan := &recordingPlan{}
	_, err := applyStateConnection(c, dir, genNeverCalled(t), init.run, plan.run)
	if !errors.Is(err, errTerraformNotInstalled) {
		t.Fatalf("err = %v, want errTerraformNotInstalled", err)
	}
	if len(plan.calls) != 0 {
		t.Fatalf("plan was run without terraform: %v", plan.calls)
	}
}

// terraformInitInDir must surface a missing binary as an error rather than
// panicking or reporting success.
func TestTerraformInitInDirWithoutBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	err := terraformInitInDir(t.TempDir())
	if !errors.Is(err, errTerraformNotInstalled) {
		t.Fatalf("err = %v, want errTerraformNotInstalled", err)
	}
}

// --- post-reconnect plan ---------------------------------------------------

// deployedConnection is the stranded-deployment classification the plan step
// hangs off.
func deployedConnection(t *testing.T, dir string, resources int) stateConnection {
	t.Helper()
	return inspectStateConnection(context.Background(), "dev", testEnv(), dir,
		lookupReturning(remoteStateSummary{
			Bucket: "state-bucket-example-dev-00000", Key: "state.tfstate",
			Region: "ap-southeast-2", ResourceCount: resources,
		}))
}

// "No changes" is the sentence that says the environment is genuinely intact.
func TestPlanNoChangesReadsAsReassurance(t *testing.T) {
	dir := generatedEnvDir(t, "dev")
	c := deployedConnection(t, dir, 85)

	out := captureOutput(t, func() {
		if _, err := applyStateConnection(c, dir, genNeverCalled(t), (&recordingInit{}).run, (&recordingPlan{}).run); err != nil {
			t.Errorf("applyStateConnection: %v", err)
		}
	})

	for _, want := range []string{"No changes", "matches all 85 deployed resources", "nothing to apply"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "NOTHING HAS BEEN APPLIED") {
		t.Errorf("the no-change case should not carry a change warning:\n%s", out)
	}
}

// A plan takes a while; the user is told it is starting and why.
func TestPlanAnnouncesItselfBeforeRunning(t *testing.T) {
	dir := generatedEnvDir(t, "dev")
	c := deployedConnection(t, dir, 85)

	out := captureOutput(t, func() {
		_, _ = applyStateConnection(c, dir, genNeverCalled(t), (&recordingInit{}).run, (&recordingPlan{}).run)
	})

	announcement := strings.Index(out, "terraform plan")
	verdict := strings.Index(out, "No changes")
	if announcement < 0 {
		t.Fatalf("plan was never announced:\n%s", out)
	}
	if verdict < announcement {
		t.Fatalf("the verdict printed before the announcement:\n%s", out)
	}
	if !strings.Contains(out, "changes nothing") {
		t.Errorf("output should say the plan is harmless:\n%s", out)
	}
}

// Destruction leads. It must not be buried under a total.
func TestPlanWithDestroyLeadsWithDestruction(t *testing.T) {
	dir := generatedEnvDir(t, "dev")
	c := deployedConnection(t, dir, 85)
	plan := &recordingPlan{summary: planSummary{Add: 5, Change: 1, Destroy: 3, Replace: 2}}

	out := captureOutput(t, func() {
		_, _ = applyStateConnection(c, dir, genNeverCalled(t), (&recordingInit{}).run, plan.run)
	})

	destroyLine := strings.Index(out, "DESTROY")
	countLine := strings.Index(out, "Plan: 5 to add, 1 to change, 3 to destroy.")
	if destroyLine < 0 {
		t.Fatalf("destruction was not called out:\n%s", out)
	}
	if countLine < 0 {
		t.Fatalf("counts missing:\n%s", out)
	}
	if destroyLine > countLine {
		t.Errorf("destruction must lead, not follow the total:\n%s", out)
	}
	if !strings.Contains(out, "2 to replace") {
		t.Errorf("replacements not reported:\n%s", out)
	}
	if !strings.Contains(out, "1 to destroy outright") {
		t.Errorf("outright destroys should be separated from replacements:\n%s", out)
	}
	if !strings.Contains(out, "NOTHING HAS BEEN APPLIED") {
		t.Errorf("output must make clear nothing was applied:\n%s", out)
	}
}

func TestPlanWithAdditiveChangesDoesNotShout(t *testing.T) {
	dir := generatedEnvDir(t, "dev")
	c := deployedConnection(t, dir, 85)
	plan := &recordingPlan{summary: planSummary{Add: 4, Change: 2}}

	out := captureOutput(t, func() {
		_, _ = applyStateConnection(c, dir, genNeverCalled(t), (&recordingInit{}).run, plan.run)
	})

	if strings.Contains(out, "DESTROY") {
		t.Errorf("no destruction in this plan, do not claim otherwise:\n%s", out)
	}
	if !strings.Contains(out, "differs from what is deployed") {
		t.Errorf("drift not reported:\n%s", out)
	}
	if !strings.Contains(out, "Plan: 4 to add, 2 to change, 0 to destroy.") {
		t.Errorf("counts missing:\n%s", out)
	}
}

// A plan that cannot run is information, not a failure of the triggering
// command.
func TestPlanFailureDoesNotFailTheCommand(t *testing.T) {
	dir := generatedEnvDir(t, "dev")
	c := deployedConnection(t, dir, 85)
	plan := &recordingPlan{err: errors.New("exit status 1: Error: error configuring S3 Backend: ExpiredToken: The provided token has expired")}

	var err error
	out := captureOutput(t, func() {
		_, err = applyStateConnection(c, dir, genNeverCalled(t), (&recordingInit{}).run, plan.run)
	})

	if err != nil {
		t.Fatalf("a failed plan must not fail the command, got: %v", err)
	}
	for _, want := range []string{
		"Could not run terraform plan",
		"ExpiredToken",
		"sync above still finished",
		"terraform -chdir=",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

// A held lock is common and recoverable, and deserves its own sentence.
func TestPlanStateLockIsNamedNotDumped(t *testing.T) {
	dir := generatedEnvDir(t, "dev")
	c := deployedConnection(t, dir, 85)
	plan := &recordingPlan{err: &stateLockedError{ID: "00000000-0000-0000-0000-000000000000"}}

	var err error
	out := captureOutput(t, func() {
		_, err = applyStateConnection(c, dir, genNeverCalled(t), (&recordingInit{}).run, plan.run)
	})

	if err != nil {
		t.Fatalf("a locked state must not fail the command, got: %v", err)
	}
	for _, want := range []string{
		"state is locked",
		"the comparison was skipped",
		"The sync above still finished",
		"Lock ID: 00000000-0000-0000-0000-000000000000",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Could not run terraform plan") {
		t.Errorf("a lock should not be reported as a generic plan failure:\n%s", out)
	}
}

// --- plan parsing ----------------------------------------------------------

func TestSummarizePlanJSON(t *testing.T) {
	planJSON := `{
      "format_version": "1.2",
      "resource_changes": [
        {"address":"aws_s3_bucket.a","change":{"actions":["create"]}},
        {"address":"aws_ecs_service.b","change":{"actions":["update"]}},
        {"address":"aws_iam_role.c","change":{"actions":["delete"]}},
        {"address":"aws_ecs_task_definition.d","change":{"actions":["delete","create"]}},
        {"address":"aws_ecs_task_definition.e","change":{"actions":["create","delete"]}},
        {"address":"aws_vpc.f","change":{"actions":["no-op"]}},
        {"address":"data.aws_region.g","change":{"actions":["read"]}}
      ]
    }`

	s, err := summarizePlanJSON([]byte(planJSON))
	if err != nil {
		t.Fatalf("summarizePlanJSON: %v", err)
	}
	// 1 plain create + 2 replacements.
	if s.Add != 3 {
		t.Errorf("Add = %d, want 3", s.Add)
	}
	if s.Change != 1 {
		t.Errorf("Change = %d, want 1", s.Change)
	}
	// 1 plain delete + 2 replacements.
	if s.Destroy != 3 {
		t.Errorf("Destroy = %d, want 3", s.Destroy)
	}
	if s.Replace != 2 {
		t.Errorf("Replace = %d, want 2", s.Replace)
	}
	if s.Total() != 7 {
		t.Errorf("Total = %d, want 7", s.Total())
	}
}

func TestSummarizePlanJSONNoChanges(t *testing.T) {
	s, err := summarizePlanJSON([]byte(`{"format_version":"1.2","resource_changes":[]}`))
	if err != nil {
		t.Fatalf("summarizePlanJSON: %v", err)
	}
	if s.Total() != 0 {
		t.Fatalf("Total = %d, want 0", s.Total())
	}
}

func TestParsePlanTextSummary(t *testing.T) {
	s, err := parsePlanTextSummary("Plan: 5 to add, 1 to change, 3 to destroy.\n")
	if err != nil {
		t.Fatalf("parsePlanTextSummary: %v", err)
	}
	if s.Add != 5 || s.Change != 1 || s.Destroy != 3 {
		t.Fatalf("summary = %+v, want 5/1/3", s)
	}

	s, err = parsePlanTextSummary("No changes. Your infrastructure matches the configuration.\n")
	if err != nil {
		t.Fatalf("parsePlanTextSummary: %v", err)
	}
	if s.Total() != 0 {
		t.Fatalf("Total = %d, want 0", s.Total())
	}

	if _, err = parsePlanTextSummary("something else entirely"); err == nil {
		t.Fatal("expected an error when no summary can be read")
	}
}

func TestIsStateLockFailure(t *testing.T) {
	locked := `Error: Error acquiring the state lock
Lock Info:
  ID:        00000000-0000-0000-0000-000000000000
  Operation: OperationTypePlan`
	if !isStateLockFailure(locked) {
		t.Error("a lock error was not recognised")
	}
	if isStateLockFailure("Error: error configuring S3 Backend: ExpiredToken") {
		t.Error("an expired token is not a lock")
	}

	if m := stateLockIDRe.FindStringSubmatch(locked); m == nil || m[1] != "00000000-0000-0000-0000-000000000000" {
		t.Errorf("lock ID not extracted from:\n%s", locked)
	}
}

func TestTerraformArgsPutChdirFirst(t *testing.T) {
	got := terraformArgs("env/dev", "plan", "-input=false")
	want := []string{"-chdir=env/dev", "plan", "-input=false"}
	if len(got) != len(want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args = %v, want %v", got, want)
		}
	}

	if got := terraformArgs(".", "plan"); len(got) != 1 || got[0] != "plan" {
		t.Fatalf("args = %v, want [plan]", got)
	}
}

// --- opt out ---------------------------------------------------------------

func TestSkipEnvVarDisablesTheProbe(t *testing.T) {
	dir := generatedEnvDir(t, "dev")
	t.Setenv(merokuSkipStateReconnect, "1")

	c := probeStateConnection(context.Background(), "dev", testEnv(), dir, lookupNeverCalled(t))
	if c.Status != stateSkipped {
		t.Fatalf("status = %v, want stateSkipped", c.Status)
	}
	if msg := describeStateConnection(c); msg != "" {
		t.Fatalf("opting out still produced output:\n%s", msg)
	}

	init := &recordingInit{}
	plan := &recordingPlan{}
	if _, err := applyStateConnection(c, dir, genNeverCalled(t), init.run, plan.run); err != nil {
		t.Fatalf("applyStateConnection: %v", err)
	}
	if len(init.calls) != 0 || len(plan.calls) != 0 {
		t.Fatalf("opting out still ran terraform: init=%v plan=%v", init.calls, plan.calls)
	}
}

// --- AWS environment -------------------------------------------------------

// `meroku generate` runs before profile selection, so the config has to supply
// the profile — but an explicit shell setting must always win.
func TestApplyAWSEnvFromConfigOnlyFillsGaps(t *testing.T) {
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")

	env := Env{AWSProfile: "from-config", Region: "ap-southeast-2"}
	applyAWSEnvFromConfig(env)
	if got := os.Getenv("AWS_PROFILE"); got != "from-config" {
		t.Errorf("AWS_PROFILE = %q, want from-config", got)
	}
	if got := os.Getenv("AWS_REGION"); got != "ap-southeast-2" {
		t.Errorf("AWS_REGION = %q, want ap-southeast-2", got)
	}

	t.Setenv("AWS_PROFILE", "from-shell")
	applyAWSEnvFromConfig(env)
	if got := os.Getenv("AWS_PROFILE"); got != "from-shell" {
		t.Errorf("AWS_PROFILE = %q, an explicit shell profile must not be overwritten", got)
	}
}

// --- state parsing ---------------------------------------------------------

// The headline number has to match `terraform state list`: instances, across
// modules, including expansions and data sources.
func TestSummarizeTerraformStateCountsInstances(t *testing.T) {
	state := `{
      "version": 4,
      "terraform_version": "1.15.8",
      "serial": 8,
      "lineage": "00000000-0000-0000-0000-000000000000",
      "resources": [
        {"mode":"managed","type":"aws_ecs_cluster","name":"cluster","instances":[{"schema_version":0}]},
        {"module":"module.workloads","mode":"managed","type":"aws_ecs_service","name":"svc",
         "instances":[{"index_key":"api"},{"index_key":"web"}]},
        {"module":"module.workloads","mode":"data","type":"aws_region","name":"current",
         "instances":[{"schema_version":0}]},
        {"mode":"managed","type":"aws_s3_bucket","name":"gone","instances":[]}
      ]
    }`

	s, err := summarizeTerraformState([]byte(state))
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if s.ManagedCount != 3 {
		t.Errorf("ManagedCount = %d, want 3", s.ManagedCount)
	}
	if s.DataCount != 1 {
		t.Errorf("DataCount = %d, want 1", s.DataCount)
	}
	if s.ResourceCount != 4 {
		t.Errorf("ResourceCount = %d, want 4", s.ResourceCount)
	}
	if s.Serial != 8 {
		t.Errorf("Serial = %d, want 8", s.Serial)
	}
	if s.TerraformVersion != "1.15.8" {
		t.Errorf("TerraformVersion = %q, want 1.15.8", s.TerraformVersion)
	}
}

func TestSummarizeTerraformStateEmpty(t *testing.T) {
	s, err := summarizeTerraformState([]byte(`{"version":4,"serial":1,"resources":[]}`))
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if s.ResourceCount != 0 {
		t.Fatalf("ResourceCount = %d, want 0", s.ResourceCount)
	}
}

// A wrong object in the bucket is an error, not "zero resources" — otherwise a
// deployed environment would be silently reported as fresh.
func TestSummarizeTerraformStateRejectsGarbage(t *testing.T) {
	if _, err := summarizeTerraformState([]byte("not json at all")); err == nil {
		t.Fatal("expected an error for a non-JSON state object")
	}
}

// A malformed state object must be classified as unreadable, never as fresh.
func TestMalformedStateIsUnreadableNotFresh(t *testing.T) {
	dir := generatedEnvDir(t, "dev")
	c := inspectStateConnection(context.Background(), "dev", testEnv(), dir,
		lookupFailing(fmt.Errorf("remote state is not valid Terraform state JSON: unexpected token")))
	if c.Status != stateUnreadable {
		t.Fatalf("status = %v, want stateUnreadable", c.Status)
	}
}

// --- error classification --------------------------------------------------

func TestIsRemoteStateAbsentError(t *testing.T) {
	if isRemoteStateAbsentError(errors.New("AccessDenied")) {
		t.Error("AccessDenied must not be treated as an absent state")
	}
	if isRemoteStateAbsentError(errors.New("unable to refresh SSO token")) {
		t.Error("an expired SSO token must not be treated as an absent state")
	}
	if isRemoteStateAbsentError(&stubAPIError{code: "NoSuchKey"}) == false {
		t.Error("NoSuchKey must be treated as an absent state")
	}
	if isRemoteStateAbsentError(&stubAPIError{code: "NoSuchBucket"}) == false {
		t.Error("NoSuchBucket must be treated as an absent state")
	}
	if isRemoteStateAbsentError(&stubAPIError{code: "PermanentRedirect"}) {
		t.Error("a redirect (bucket in another region) is a read failure, not an absent state")
	}
}

// stubAPIError implements smithy.APIError.
type stubAPIError struct{ code string }

func (e *stubAPIError) Error() string                 { return "api error " + e.code }
func (e *stubAPIError) ErrorCode() string             { return e.code }
func (e *stubAPIError) ErrorMessage() string          { return e.code }
func (e *stubAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }

// --- meroku sync -----------------------------------------------------------
//
// The automatic paths are quiet by design. This command is not: whatever it
// finds, it says, and it ends with a verdict. Each test below is one of the
// states a user can be in.

// isolateAWSEnv keeps applyStateConnection's environment filling from leaking
// out of a test.
func isolateAWSEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
}

// syncFixture is a sync run with everything injected: no AWS, no terraform, no
// generation.
type syncFixture struct {
	env  Env
	dir  string
	gen  *recordingGen
	init *recordingInit
	plan *recordingPlan
	deps syncDeps
}

func newSyncFixture(t *testing.T, dir string, root string, lookup remoteStateLookup, summary planSummary, planErr error) *syncFixture {
	t.Helper()
	isolateAWSEnv(t)

	f := &syncFixture{
		env:  testEnv(),
		dir:  dir,
		gen:  &recordingGen{root: root},
		init: &recordingInit{},
		plan: &recordingPlan{summary: summary, err: planErr},
	}
	f.deps = syncDeps{lookup: lookup, gen: f.gen.run, init: f.init.run, plan: f.plan.run}
	return f
}

func (f *syncFixture) run(t *testing.T) (string, error) {
	t.Helper()
	var err error
	out := captureOutput(t, func() {
		err = runSync(context.Background(), "dev", f.env, f.dir, f.deps)
	})
	return out, err
}

// (a) already connected and matching. The quiet automatic path stops at
// "initialised"; an explicit sync has to say what it is connected to.
func TestSyncReportsAnAlreadyConnectedEnvironment(t *testing.T) {
	dir := generatedEnvDir(t, "dev")
	if err := os.MkdirAll(filepath.Join(dir, ".terraform"), 0o755); err != nil {
		t.Fatalf("creating .terraform: %v", err)
	}

	f := newSyncFixture(t, dir, t.TempDir(), lookupReturning(remoteStateSummary{
		Bucket: "state-bucket-example-dev-00000", Key: "state.tfstate", Region: "ap-southeast-2",
		ResourceCount: 85, ManagedCount: 67, DataCount: 18, Serial: 8,
	}), planSummary{}, nil)

	out, err := f.run(t)
	if err != nil {
		t.Fatalf("a healthy environment must not fail the command: %v", err)
	}
	for _, want := range []string{
		"meroku sync — environment 'dev'",
		"never applies, never destroys and never migrates state",
		"is linked",
		"85 resources (67 managed, 18 data sources)",
		"Verdict: Connected. the configuration matches what is deployed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if len(f.plan.calls) != 1 {
		t.Errorf("plan calls = %v, want exactly one drift check", f.plan.calls)
	}
	if len(f.init.calls) != 0 || len(f.gen.calls) != 0 {
		t.Errorf("a connected environment was rewritten: init=%v gen=%v", f.init.calls, f.gen.calls)
	}
}

// (b) connected but drifted. Reported, never acted on, and still exit 0 —
// drift is information.
func TestSyncReportsDriftWithoutActingOnIt(t *testing.T) {
	dir := generatedEnvDir(t, "dev")
	if err := os.MkdirAll(filepath.Join(dir, ".terraform"), 0o755); err != nil {
		t.Fatalf("creating .terraform: %v", err)
	}

	f := newSyncFixture(t, dir, t.TempDir(),
		lookupReturning(remoteStateSummary{Bucket: "b", Key: "k", Region: "r", ResourceCount: 85}),
		planSummary{Add: 5, Change: 1, Destroy: 3, Replace: 2}, nil)

	out, err := f.run(t)
	if err != nil {
		t.Fatalf("drift is not a command failure, got: %v", err)
	}
	for _, want := range []string{
		"DESTROY",
		"NOTHING HAS BEEN APPLIED",
		"Verdict: Connected. the configuration would DESTROY 3 deployed resources",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

// (c) disconnected and recovered — the fresh clone, driven through the command.
func TestSyncRecoversADisconnectedEnvironment(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "env", "dev")

	f := newSyncFixture(t, dir, root, lookupReturning(remoteStateSummary{
		Bucket: "state-bucket-example-dev-00000", Key: "state.tfstate", Region: "ap-southeast-2",
		ResourceCount: 85, ManagedCount: 67, DataCount: 18,
	}), planSummary{}, nil)

	out, err := f.run(t)
	if err != nil {
		t.Fatalf("runSync: %v", err)
	}
	for _, want := range []string{
		"(missing)",
		"dev is deployed — 85 resources in ap-southeast-2",
		"85 resources",
		"Linked — env/dev now tracks the 85 deployed resources",
		"Verdict: Synced. the configuration matches",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if len(f.gen.calls) != 1 || len(f.init.calls) != 1 || len(f.plan.calls) != 1 {
		t.Fatalf("recovery steps = gen%v init%v plan%v, want one of each", f.gen.calls, f.init.calls, f.plan.calls)
	}
}

// (d) genuinely new. Nothing is generated, nothing is run, and the command still
// says something useful.
func TestSyncReportsAGenuinelyNewEnvironment(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "env", "dev")

	f := newSyncFixture(t, dir, root, lookupFailing(errRemoteStateAbsent), planSummary{}, nil)

	out, err := f.run(t)
	if err != nil {
		t.Fatalf("a new environment is not a failure, got: %v", err)
	}
	for _, want := range []string{
		"is a new environment",
		"nothing deployed and nothing to link to",
		"meroku generate dev",
		"Verdict: 'dev' has never been deployed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if len(f.gen.calls) != 0 || len(f.init.calls) != 0 || len(f.plan.calls) != 0 {
		t.Fatalf("something ran for a new environment: gen%v init%v plan%v", f.gen.calls, f.init.calls, f.plan.calls)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("env/dev was created for a new environment (err %v)", err)
	}
}

// (e) backend unreadable. The command could not do the one thing it was asked
// to do, so it exits non-zero — unlike drift, which is an answer.
func TestSyncFailsWhenTheBackendCannotBeRead(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "env", "dev")
	lookupErr := errors.New("operation error S3: GetObject, https response error StatusCode: 403, AccessDenied")

	f := newSyncFixture(t, dir, root, lookupFailing(lookupErr), planSummary{}, nil)

	out, err := f.run(t)
	if !errors.Is(err, lookupErr) {
		t.Fatalf("err = %v, want it to wrap %v", err, lookupErr)
	}
	for _, want := range []string{
		"Could not read the remote Terraform state",
		"AccessDenied",
		"says nothing about your infrastructure",
		"aws sts get-caller-identity",
		"Verdict: Could not reach the state backend",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if len(f.gen.calls) != 0 || len(f.init.calls) != 0 {
		t.Fatalf("something ran despite an unreadable backend: gen%v init%v", f.gen.calls, f.init.calls)
	}
}

func TestSyncReportsAnUnconfiguredBackend(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "env", "dev")

	f := newSyncFixture(t, dir, root, lookupNeverCalled(t), planSummary{}, nil)
	f.env = Env{Env: "dev"} // no bucket, key or region

	out, err := f.run(t)
	if err != nil {
		t.Fatalf("an unconfigured backend is not a command failure, got: %v", err)
	}
	for _, want := range []string{
		"No S3 backend is configured",
		"state_bucket, state_file and region",
		"Verdict: Nothing to sync",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

// The opt-out is honoured even for an explicit command — but it says so, rather
// than looking like a check that passed.
func TestSyncHonoursTheOptOutAndSaysSo(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "env", "dev")

	f := newSyncFixture(t, dir, root, lookupNeverCalled(t), planSummary{}, nil)
	t.Setenv(merokuSkipStateReconnect, "1")

	out, err := f.run(t)
	if err != nil {
		t.Fatalf("opting out is not a failure, got: %v", err)
	}
	for _, want := range []string{merokuSkipStateReconnect, "Verdict: Nothing was checked"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if len(f.gen.calls) != 0 || len(f.init.calls) != 0 || len(f.plan.calls) != 0 {
		t.Fatalf("opting out still ran something: gen%v init%v plan%v", f.gen.calls, f.init.calls, f.plan.calls)
	}
}

// Every branch ends with a verdict. A command run on purpose that prints no
// conclusion is a broken-feeling command.
func TestSyncAlwaysPrintsAVerdict(t *testing.T) {
	newEnvDir := func() (string, string) {
		root := t.TempDir()
		return root, filepath.Join(root, "env", "dev")
	}
	initialised := func() (string, string) {
		dir := generatedEnvDir(t, "dev")
		if err := os.MkdirAll(filepath.Join(dir, ".terraform"), 0o755); err != nil {
			t.Fatalf("creating .terraform: %v", err)
		}
		return t.TempDir(), dir
	}

	cases := map[string]struct {
		dirs   func() (string, string)
		lookup remoteStateLookup
	}{
		"connected":    {initialised, lookupReturning(remoteStateSummary{ResourceCount: 85})},
		"disconnected": {newEnvDir, lookupReturning(remoteStateSummary{ResourceCount: 85})},
		"new":          {newEnvDir, lookupFailing(errRemoteStateAbsent)},
		"unreadable":   {newEnvDir, lookupFailing(errors.New("AccessDenied"))},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			root, dir := tc.dirs()
			f := newSyncFixture(t, dir, root, tc.lookup, planSummary{}, nil)
			out, _ := f.run(t)
			if !strings.Contains(out, "Verdict: ") {
				t.Fatalf("no verdict printed:\n%s", out)
			}
			if strings.Contains(out, "No conclusion could be reached") {
				t.Fatalf("the verdict was empty:\n%s", out)
			}
		})
	}
}

// --- environment resolution ------------------------------------------------

func TestResolveSyncEnvironment(t *testing.T) {
	listing := func(envs ...string) func() ([]string, error) {
		return func() ([]string, error) { return envs, nil }
	}

	// An explicit argument wins, exactly as `meroku generate dev` reads.
	if got, err := resolveSyncEnvironment([]string{"prod"}, "dev", listing("dev", "prod")); err != nil || got != "prod" {
		t.Errorf("argument: got %q, %v; want prod", got, err)
	}
	// Then --env, which the interactive path already honours.
	if got, err := resolveSyncEnvironment(nil, "staging", listing("dev", "staging")); err != nil || got != "staging" {
		t.Errorf("--env: got %q, %v; want staging", got, err)
	}
	// One environment is not a guess.
	out := captureOutput(t, func() {
		if got, err := resolveSyncEnvironment(nil, "", listing("dev")); err != nil || got != "dev" {
			t.Errorf("single environment: got %q, %v; want dev", got, err)
		}
	})
	if !strings.Contains(out, "dev") {
		t.Errorf("the chosen environment should be stated:\n%s", out)
	}

	// Several is genuine ambiguity: list them, do not pick one.
	_, err := resolveSyncEnvironment(nil, "", listing("dev", "prod", "staging"))
	if err == nil {
		t.Fatal("expected an error when the environment is ambiguous")
	}
	for _, want := range []string{"dev, prod, staging", "meroku sync <environment>"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}

	// None at all: say where it expected to find one.
	_, err = resolveSyncEnvironment(nil, "", listing())
	if err == nil {
		t.Fatal("expected an error when there are no environments")
	}
	if !strings.Contains(err.Error(), "dev.yaml") {
		t.Errorf("error should name the file it looked for: %v", err)
	}
}

// --- recursion -------------------------------------------------------------

// Recovery regenerates by calling generateEnvironmentFiles, never
// handleGenerateCommand — which ends by running the reconnect and would
// therefore re-enter the code that called it.
//
// The property that makes the loop impossible is that the generation step is
// inert: it writes files and consults nothing. This runs the real one, with the
// production state lookup replaced by a fuse, and proves it never probes.
func TestGenerationStepDoesNotReenterTheReconnect(t *testing.T) {
	root := t.TempDir()
	chdir(t, root)
	writeStaleConfig(t, root, "dev", CurrentSchemaVersion)

	templateDir := filepath.Join(root, "infrastructure", "env")
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		t.Fatalf("creating template dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "main.hbs"), []byte("# {{env}}\n"), 0o644); err != nil {
		t.Fatalf("writing template: %v", err)
	}

	original := defaultRemoteStateLookup
	defaultRemoteStateLookup = func(context.Context, Env) (remoteStateSummary, error) {
		t.Fatal("generation reached the remote state — recovery can now loop")
		return remoteStateSummary{}, nil
	}
	t.Cleanup(func() { defaultRemoteStateLookup = original })

	out := captureOutput(t, func() {
		if err := generateEnvironmentFiles("dev"); err != nil {
			t.Errorf("generateEnvironmentFiles: %v", err)
		}
	})

	if _, err := os.Stat(filepath.Join(root, "env", "dev", "main.tf")); err != nil {
		t.Fatalf("generation wrote no main.tf: %v (output: %s)", err, out)
	}
}

// And the recovery path calls it exactly once, whatever the plan then says.
func TestRecoveryGeneratesExactlyOnce(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "env", "dev")

	c := inspectStateConnection(context.Background(), "dev", testEnv(), dir,
		lookupReturning(remoteStateSummary{Bucket: "b", Key: "k", Region: "r", ResourceCount: 85}))

	gen := &recordingGen{root: root}
	plan := &recordingPlan{summary: planSummary{Add: 1, Destroy: 1}}
	if _, err := applyStateConnection(c, dir, gen.run, (&recordingInit{}).run, plan.run); err != nil {
		t.Fatalf("applyStateConnection: %v", err)
	}
	if len(gen.calls) != 1 {
		t.Fatalf("generate calls = %v, want exactly one", gen.calls)
	}

	// Second pass: the directory is now on disk, so a re-run must not rewrite it.
	c2 := inspectStateConnection(context.Background(), "dev", testEnv(), dir,
		lookupReturning(remoteStateSummary{Bucket: "b", Key: "k", Region: "r", ResourceCount: 85}))
	if !c2.Generated {
		t.Fatal("the recovered directory was not recognised as generated")
	}
	if _, err := applyStateConnection(c2, dir, genNeverCalled(t), (&recordingInit{}).run, plan.run); err != nil {
		t.Fatalf("second applyStateConnection: %v", err)
	}
}

// generate must not be able to recurse into itself through the reconnect: by the
// time it runs, env/<env>/ exists, so recovery has nothing to regenerate.
func TestGeneratedDirectoryIsNeverRegeneratedByRecovery(t *testing.T) {
	dir := generatedEnvDir(t, "dev")

	c := inspectStateConnection(context.Background(), "dev", testEnv(), dir,
		lookupReturning(remoteStateSummary{Bucket: "b", Key: "k", Region: "r", ResourceCount: 85}))
	if !c.Generated {
		t.Fatal("Generated = false for a directory that exists")
	}

	// genNeverCalled fails the test if generation is attempted.
	if _, err := applyStateConnection(c, dir, genNeverCalled(t), (&recordingInit{}).run, (&recordingPlan{}).run); err != nil {
		t.Fatalf("applyStateConnection: %v", err)
	}
}

// --- consent ---------------------------------------------------------------
//
// Syncing writes files and calls AWS, so on the paths nobody asked for — a
// generate, or picking an environment from a menu — it is a question. These
// tests drive the question with an injected answer, so none of them can block
// on a terminal.

// recordingConsent answers the question and counts how often it was asked.
type recordingConsent struct {
	answer syncDecision
	asked  int
	envs   []string
}

func (r *recordingConsent) decide(c stateConnection) syncDecision {
	r.asked++
	r.envs = append(r.envs, c.Env)
	return r.answer
}

// consentNeverAsked fails the test if the question is put at all.
func consentNeverAsked(t *testing.T) syncConsent {
	t.Helper()
	return func(c stateConnection) syncDecision {
		t.Fatalf("the user was asked whether to sync '%s' when they should not have been", c.Env)
		return syncDeclined
	}
}

// pretendTerminal makes the interactive branch reachable in a test, and swaps
// the full screen for a recorder. Without this the whole screen route is
// unreachable under `go test`, which has no terminal — and the route is exactly
// what these tests are about.
type screenRecorder struct {
	calls []syncRequest
}

func pretendTerminal(t *testing.T, screen *screenRecorder) {
	t.Helper()

	origTerm := terminalIsInteractive
	origScreen := startSyncScreen
	terminalIsInteractive = func() bool { return true }
	startSyncScreen = func(req syncRequest) (reconnectOutcome, error) {
		screen.calls = append(screen.calls, req)
		return reconnectOutcome{Initialised: true, Verdict: "Synced. (recorded)"}, nil
	}
	t.Cleanup(func() {
		terminalIsInteractive = origTerm
		startSyncScreen = origScreen
	})
}

// altScreenEnter is what a Bubble Tea program with tea.WithAltScreen writes to
// take over the terminal.
const altScreenEnter = "\x1b[?1049h"

// disconnectedFixture is a fresh checkout of a deployed project: <env>.yaml
// only, env/ has never existed here, 85 resources in the backend.
func disconnectedFixture(t *testing.T) (root string, envDir string, c stateConnection) {
	t.Helper()
	root = t.TempDir()
	envDir = filepath.Join(root, "env", "dev")
	c = inspectStateConnection(context.Background(), "dev", testEnv(), envDir,
		lookupReturning(remoteStateSummary{
			Bucket: "state-bucket-example-dev-00000", Key: "state.tfstate",
			Region: "ap-southeast-2", ResourceCount: 85, ManagedCount: 67, DataCount: 18,
		}))
	if c.Status != stateDeployedButDisconnected {
		t.Fatalf("fixture status = %v, want stateDeployedButDisconnected", c.Status)
	}
	return root, envDir, c
}

// treeSnapshot lists every path under root, so "nothing changed on disk" can be
// asserted rather than assumed.
func treeSnapshot(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return paths
}

// Yes runs the whole sync: write env/dev, init, compare.
func TestConsentYesSyncs(t *testing.T) {
	isolateAWSEnv(t)
	root, envDir, c := disconnectedFixture(t)

	gen := &recordingGen{root: root}
	init := &recordingInit{}
	plan := &recordingPlan{}
	consent := &recordingConsent{answer: syncApproved}

	var outcome reconnectOutcome
	var err error
	out := captureOutput(t, func() {
		outcome, err = performSync(syncRequest{
			conn: c, envDir: envDir, ask: true,
			gen: gen.run, init: init.run, plan: plan.run, decide: consent.decide,
		})
	})
	if err != nil {
		t.Fatalf("performSync: %v", err)
	}

	if consent.asked != 1 {
		t.Fatalf("asked %d times, want exactly once", consent.asked)
	}
	if len(gen.calls) != 1 || len(init.calls) != 1 || len(plan.calls) != 1 {
		t.Fatalf("steps = gen%v init%v plan%v, want one of each", gen.calls, init.calls, plan.calls)
	}
	if !outcome.Generated || !outcome.Initialised {
		t.Fatalf("outcome = %+v, want generated and initialised", outcome)
	}
	if _, err := os.Stat(filepath.Join(envDir, "main.tf")); err != nil {
		t.Fatalf("env/dev/main.tf was not written: %v", err)
	}
	if !strings.Contains(outcome.Verdict, "Synced.") {
		t.Errorf("verdict = %q, want it to lead with Synced.", outcome.Verdict)
	}
	if !strings.Contains(out, "Linked — env/dev now tracks the 85 deployed resources") {
		t.Errorf("output did not report the link:\n%s", out)
	}
}

// No skips, says how to do it later, is not an error, and leaves the disk
// exactly as it was.
func TestConsentNoSkipsAndChangesNothing(t *testing.T) {
	isolateAWSEnv(t)
	root, envDir, c := disconnectedFixture(t)
	before := treeSnapshot(t, root)

	consent := &recordingConsent{answer: syncDeclined}

	var outcome reconnectOutcome
	var err error
	out := captureOutput(t, func() {
		outcome, err = performSync(syncRequest{
			conn: c, envDir: envDir, ask: true,
			gen:    genNeverCalled(t),
			init:   func(string) error { t.Fatal("terraform init ran after a no"); return nil },
			plan:   func(string) (planSummary, error) { t.Fatal("terraform plan ran after a no"); return planSummary{}, nil },
			decide: consent.decide,
		})
	})

	if err != nil {
		t.Fatalf("declining is not an error, got: %v", err)
	}
	if consent.asked != 1 {
		t.Fatalf("asked %d times, want exactly once", consent.asked)
	}
	if outcome.Generated || outcome.Initialised {
		t.Fatalf("outcome = %+v, want nothing done", outcome)
	}
	if !strings.Contains(out, "meroku sync dev") {
		t.Errorf("a skip must say how to do it later:\n%s", out)
	}
	if !strings.Contains(out, "Not synced") {
		t.Errorf("a skip must say what was skipped:\n%s", out)
	}
	if !strings.Contains(outcome.Verdict, "meroku sync dev") {
		t.Errorf("verdict = %q, want the command to run later", outcome.Verdict)
	}

	after := treeSnapshot(t, root)
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatalf("declining changed the disk:\nbefore %v\nafter  %v", before, after)
	}
}

// The refusal is not remembered. Two runs in a row both ask, and nothing is
// written anywhere to record the first answer.
func TestRefusalIsNotRemembered(t *testing.T) {
	isolateAWSEnv(t)
	root, envDir, c := disconnectedFixture(t)
	before := treeSnapshot(t, root)

	consent := &recordingConsent{answer: syncDeclined}
	run := func() {
		captureOutput(t, func() {
			if _, err := performSync(syncRequest{
				conn: c, envDir: envDir, ask: true,
				gen:    genNeverCalled(t),
				init:   func(string) error { t.Fatal("terraform init ran after a no"); return nil },
				plan:   func(string) (planSummary, error) { t.Fatal("terraform plan ran after a no"); return planSummary{}, nil },
				decide: consent.decide,
			}); err != nil {
				t.Errorf("performSync: %v", err)
			}
		})
	}

	run()
	run()

	if consent.asked != 2 {
		t.Fatalf("asked %d times across two runs, want 2 — a refusal must not stick", consent.asked)
	}

	// A preference file would be the obvious way to make it stick, so prove
	// there isn't one.
	after := treeSnapshot(t, root)
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatalf("a refusal was recorded on disk:\nbefore %v\nafter  %v", before, after)
	}

	// And a third run, answering yes, syncs — the earlier no blocks nothing.
	gen := &recordingGen{root: root}
	init := &recordingInit{}
	plan := &recordingPlan{}
	consent.answer = syncApproved
	captureOutput(t, func() {
		if _, err := performSync(syncRequest{
			conn: c, envDir: envDir, ask: true,
			gen: gen.run, init: init.run, plan: plan.run, decide: consent.decide,
		}); err != nil {
			t.Errorf("performSync: %v", err)
		}
	})
	if len(gen.calls) != 1 || len(init.calls) != 1 || len(plan.calls) != 1 {
		t.Fatalf("a yes after a no did not sync: gen%v init%v plan%v", gen.calls, init.calls, plan.calls)
	}
}

// No terminal to ask on: no question, no sync, and one line saying what to run.
// A prompt that blocks a CI job is worse than no feature.
func TestNonInteractiveNeitherAsksNorSyncs(t *testing.T) {
	isolateAWSEnv(t)
	root, envDir, c := disconnectedFixture(t)
	before := treeSnapshot(t, root)

	// decide is nil, exactly as in production. `go test` has no controlling
	// terminal, so this exercises the real routing rather than a stub.
	var outcome reconnectOutcome
	var err error
	out := captureOutput(t, func() {
		outcome, err = performSync(syncRequest{
			conn: c, envDir: envDir, ask: true,
			gen:  genNeverCalled(t),
			init: func(string) error { t.Fatal("terraform init ran without consent"); return nil },
			plan: func(string) (planSummary, error) {
				t.Fatal("terraform plan ran without consent")
				return planSummary{}, nil
			},
		})
	})

	if err != nil {
		t.Fatalf("a non-interactive run is not a failure, got: %v", err)
	}
	if outcome.Generated || outcome.Initialised {
		t.Fatalf("outcome = %+v, want nothing done", outcome)
	}
	if !strings.Contains(out, "meroku sync dev") {
		t.Errorf("output must name the command to run instead:\n%s", out)
	}
	if strings.Join(before, "\n") != strings.Join(treeSnapshot(t, root), "\n") {
		t.Fatal("a non-interactive run changed the disk")
	}
}

// CI is not interactive even when it hands out a TTY.
func TestInteractiveTerminalIsFalseInCI(t *testing.T) {
	t.Setenv("CI", "true")
	if interactiveTerminal() {
		t.Error("CI must never be treated as a terminal to ask on")
	}
}

// `meroku sync` never asks: typing the command is the consent, and asking again
// teaches people to stop reading prompts. The fuse fails the test if it is.
func TestSyncCommandNeverAsks(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "env", "dev")

	f := newSyncFixture(t, dir, root, lookupReturning(remoteStateSummary{
		Bucket: "state-bucket-example-dev-00000", Key: "state.tfstate", Region: "ap-southeast-2",
		ResourceCount: 85, ManagedCount: 67, DataCount: 18,
	}), planSummary{}, nil)
	f.deps.decide = consentNeverAsked(t)

	out, err := f.run(t)
	if err != nil {
		t.Fatalf("runSync: %v", err)
	}
	if len(f.gen.calls) != 1 || len(f.init.calls) != 1 || len(f.plan.calls) != 1 {
		t.Fatalf("sync did not run its steps: gen%v init%v plan%v", f.gen.calls, f.init.calls, f.plan.calls)
	}
	if strings.Contains(out, "Not synced") {
		t.Errorf("meroku sync must not skip its own work:\n%s", out)
	}
}

// The same for an environment that is already linked: no question, and only the
// comparison runs.
func TestSyncCommandNeverAsksWhenAlreadyLinked(t *testing.T) {
	dir := generatedEnvDir(t, "dev")
	if err := os.MkdirAll(filepath.Join(dir, ".terraform"), 0o755); err != nil {
		t.Fatalf("creating .terraform: %v", err)
	}

	f := newSyncFixture(t, dir, t.TempDir(),
		lookupReturning(remoteStateSummary{Bucket: "b", Key: "k", Region: "r", ResourceCount: 85}),
		planSummary{}, nil)
	f.deps.decide = consentNeverAsked(t)

	if _, err := f.run(t); err != nil {
		t.Fatalf("runSync: %v", err)
	}
	if len(f.plan.calls) != 1 {
		t.Fatalf("plan calls = %v, want exactly one comparison", f.plan.calls)
	}
	if len(f.gen.calls) != 0 || len(f.init.calls) != 0 {
		t.Fatalf("a linked environment was rewritten: gen%v init%v", f.gen.calls, f.init.calls)
	}
}

// A linked directory costs the automatic paths nothing: no question, no
// terraform, no output. A `meroku generate` that quietly spent two minutes on a
// plan would be a tax nobody agreed to.
func TestAutomaticPathLeavesALinkedEnvironmentAlone(t *testing.T) {
	isolateAWSEnv(t)
	dir := generatedEnvDir(t, "dev")
	if err := os.MkdirAll(filepath.Join(dir, ".terraform"), 0o755); err != nil {
		t.Fatalf("creating .terraform: %v", err)
	}

	c := inspectStateConnection(context.Background(), "dev", testEnv(), dir, lookupNeverCalled(t))
	if c.Status != stateAlreadyInitialised {
		t.Fatalf("status = %v, want stateAlreadyInitialised", c.Status)
	}

	var err error
	out := captureOutput(t, func() {
		_, err = performSync(syncRequest{
			conn: c, envDir: dir, ask: true,
			gen:  genNeverCalled(t),
			init: func(string) error { t.Fatal("terraform init ran on a linked environment"); return nil },
			plan: func(string) (planSummary, error) {
				t.Fatal("terraform plan ran on a linked environment")
				return planSummary{}, nil
			},
			decide: consentNeverAsked(t),
		})
	})
	if err != nil {
		t.Fatalf("performSync: %v", err)
	}
	if out != "" {
		t.Fatalf("a linked environment produced output on an automatic path:\n%s", out)
	}
}

// --- the question is inline, the work is a screen --------------------------
//
// The complaint these guard: answering "sync or not" used to take over the
// whole terminal, arriving straight after the environment picker, so it read as
// the app having jumped somewhere unasked. The question is now an ordinary
// prompt in the normal flow; only a yes opens the screen.

// Yes: the facts are printed inline, then the full screen opens on the work.
func TestConsentYesOpensTheScreenOnTheWork(t *testing.T) {
	isolateAWSEnv(t)
	_, envDir, c := disconnectedFixture(t)

	screen := &screenRecorder{}
	pretendTerminal(t, screen)
	consent := &recordingConsent{answer: syncApproved}

	var outcome reconnectOutcome
	out := captureOutput(t, func() {
		outcome, _ = performSync(syncRequest{
			conn: c, envDir: envDir, ask: true,
			gen:    genNeverCalled(t),
			init:   func(string) error { t.Fatal("the plain path ran while a screen was available"); return nil },
			plan:   func(string) (planSummary, error) { return planSummary{}, nil },
			decide: consent.decide,
		})
	})

	if consent.asked != 1 {
		t.Fatalf("asked %d times, want exactly once", consent.asked)
	}
	if len(screen.calls) != 1 {
		t.Fatalf("the screen opened %d times, want once after the yes", len(screen.calls))
	}
	// The screen must open on the work. If it still carried the question it
	// would ask a second time, full-screen — the exact thing being removed.
	if screen.calls[0].ask {
		t.Error("the screen was handed the question again instead of starting on the work")
	}
	if !outcome.Initialised {
		t.Errorf("outcome = %+v, want the screen's result", outcome)
	}

	// The facts were shown inline, before the screen.
	if !strings.Contains(flattenText(out), "dev is deployed — 85 resources in ap-southeast-2") {
		t.Errorf("the situation was not printed inline before the question:\n%s", out)
	}
	assertNoAltScreen(t, out)
}

// No: nothing opens, nothing runs, nothing is written, and the reminder is one
// line in the normal flow.
func TestConsentNoOpensNothing(t *testing.T) {
	isolateAWSEnv(t)
	root, envDir, c := disconnectedFixture(t)
	before := treeSnapshot(t, root)

	screen := &screenRecorder{}
	pretendTerminal(t, screen)
	consent := &recordingConsent{answer: syncDeclined}

	var outcome reconnectOutcome
	var err error
	out := captureOutput(t, func() {
		outcome, err = performSync(syncRequest{
			conn: c, envDir: envDir, ask: true,
			gen:    genNeverCalled(t),
			init:   func(string) error { t.Fatal("terraform init ran after a no"); return nil },
			plan:   func(string) (planSummary, error) { t.Fatal("terraform plan ran after a no"); return planSummary{}, nil },
			decide: consent.decide,
		})
	})

	if err != nil {
		t.Fatalf("declining is not an error, got: %v", err)
	}
	if len(screen.calls) != 0 {
		t.Fatalf("the screen opened after a no: %+v", screen.calls)
	}
	if outcome.Generated || outcome.Initialised {
		t.Fatalf("outcome = %+v, want nothing done", outcome)
	}
	if !strings.Contains(out, "meroku sync dev") {
		t.Errorf("a no must say how to do it later:\n%s", out)
	}
	assertNoAltScreen(t, out)

	if strings.Join(before, "\n") != strings.Join(treeSnapshot(t, root), "\n") {
		t.Fatal("declining changed the disk")
	}
}

// The question itself never takes over the terminal, whatever the answer.
func assertNoAltScreen(t *testing.T, out string) {
	t.Helper()
	if strings.Contains(out, altScreenEnter) {
		t.Errorf("the decision entered the alt screen — it must stay in the normal flow:\n%q", out)
	}
}

// The real prompt, with no terminal to ask on, asks nothing and prints nothing.
func TestPromptForSyncConsentIsSilentWithoutATerminal(t *testing.T) {
	_, _, c := disconnectedFixture(t)

	var got syncDecision
	out := captureOutput(t, func() { got = promptForSyncConsent(c) })

	if got != syncNotAsked {
		t.Errorf("decision = %v, want syncNotAsked with no terminal", got)
	}
	if out != "" {
		t.Errorf("the prompt printed with nobody to ask:\n%q", out)
	}
}

// `meroku sync` reaches the screen without ever being asked anything.
func TestSyncCommandGoesStraightToTheScreen(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "env", "dev")

	screen := &screenRecorder{}
	pretendTerminal(t, screen)

	f := newSyncFixture(t, dir, root, lookupReturning(remoteStateSummary{
		Bucket: "b", Key: "k", Region: "ap-southeast-2", ResourceCount: 85,
	}), planSummary{}, nil)
	f.deps.decide = consentNeverAsked(t)

	if _, err := f.run(t); err != nil {
		t.Fatalf("runSync: %v", err)
	}
	if len(screen.calls) != 1 {
		t.Fatalf("the screen opened %d times, want once", len(screen.calls))
	}
	if screen.calls[0].ask {
		t.Error("meroku sync asked for permission to do the thing it was told to do")
	}
}
