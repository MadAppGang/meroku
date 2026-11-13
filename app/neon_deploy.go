package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"gopkg.in/yaml.v2"
)

// The Neon part of a deploy, a destroy, a reconnect plan and an import.
//
// A deploy of an env with Neon runs in two Terraform steps in one root and
// one state. Step 1 is a targeted plan and apply of the project and databases
// meroku creates (module.neon); it needs the Neon token only, because the
// postgresql providers are pruned from a plan targeted at module.neon. Step 2
// is the full plan and apply; the postgresql providers are configured from
// the ephemeral TF_VAR_neon_pg_credentials, which meroku fills from the Neon
// API between the two steps and hands to terraform processes only.

const (
	neonStep1Title = "Neon — step 1 of 2: project and databases"
	neonStep2Title = "Step 2 of 2: everything else"
)

// errNeonStopped is a Neon run that stopped on purpose with nothing more to
// report: the message was printed, or the user cancelled.
var errNeonStopped = errors.New("neon: deploy stopped")

// neonDeployOptions shape prepareNeonForDeploy.
type neonDeployOptions struct {
	// Interactive allows prompts (the V-N29 "create it again" question).
	Interactive bool
	// NoAWS limits step 2 to the Neon addresses and skips every SSM check.
	NoAWS bool
	// ListSSM overrides the SSM lister built from the env's AWS profile.
	ListSSM ssmLister
	// Step1 runs step 1; nil is runNeonStep1.
	Step1 func(ctx context.Context, target handoffTarget, opts planStepOptions) (neonStep1Outcome, error)
}

// neonStep1Outcome is what step 1 did.
type neonStep1Outcome struct {
	// NoChanges: the targeted plan had nothing to do, and nothing was shown.
	NoChanges bool
	// Applied: the plan was shown and applied without errors.
	Applied bool
}

// NeonDeployResult is what step 2 needs.
type NeonDeployResult struct {
	// Step2Targets limit step 2 (NoAWS only); nil means the whole root.
	Step2Targets []string
	// Step2Title names step 2 when step 1 applied changes.
	Step2Title string
	// Kept is the Kept report after the releases.
	Kept []string
	// Cleanup deletes the deploy-time files; call it when step 2 returns.
	Cleanup func()
}

// neonEnvName is the env's name: as selected, else from the YAML.
func neonEnvName(t handoffTarget) string {
	if t.EnvName != "" {
		return t.EnvName
	}
	return t.Env.Env
}

// neonEnvDir is env/<env> under the target's project root.
func neonEnvDir(t handoffTarget) string {
	return filepath.Join(t.ProjectRoot, "env", neonEnvName(t))
}

// isNeonRun reports whether a deploy or destroy of env needs a Neon session:
// the YAML has `neon:`, or the lock file says Neon state may exist.
func isNeonRun(env Env, envDir string) bool {
	return env.Neon != nil || neonStateMayExist(envDir)
}

// terraformOutputRunner runs terraform through newTerraformCmd and returns
// stdout. A failure carries the last lines of stderr, redacted.
func terraformOutputRunner(ctx context.Context, purpose tfPurpose, dir string, args ...string) ([]byte, error) {
	cmd, err := newTerraformCmd(ctx, purpose, dir, args...)
	if err != nil {
		return nil, err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		name := "terraform"
		if len(args) > 0 {
			name += " " + args[0]
		}
		if tail := strings.TrimSpace(lastLines(stderr.String(), 8)); tail != "" {
			return out, fmt.Errorf("%s: %w\n%s", name, err, redactSecrets(tail))
		}
		return out, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

// neonStateRm releases addresses from state in one `terraform state rm`.
func neonStateRm(ctx context.Context, tf terraformRunner, dir string, addresses []string) error {
	if len(addresses) == 0 {
		return nil
	}
	args := append([]string{"state", "rm"}, addresses...)
	if _, err := tf(ctx, tfState, dir, args...); err != nil {
		return fmt.Errorf("neon: releasing %d addresses from Terraform state failed: %w", len(addresses), err)
	}
	return nil
}

// neonRelease records kept first, then releases addresses. A failed Kept
// write releases nothing.
func neonRelease(ctx context.Context, out io.Writer, tf terraformRunner, projectRoot, env, dir string, kept []NeonKeptEntry, addresses []string) error {
	if err := recordNeonKept(projectRoot, env, kept); err != nil {
		return fmt.Errorf("%w; nothing was released from Terraform state", err)
	}
	if err := neonStateRm(ctx, tf, dir, addresses); err != nil {
		return err
	}
	if len(addresses) > 0 {
		fmt.Fprintln(out, "🔓 Released from Terraform state (nothing is deleted in Neon):")
		for _, a := range addresses {
			fmt.Fprintf(out, "   %s\n", a)
		}
	}
	if len(kept) > 0 {
		for _, l := range neonKeptReport(kept) {
			fmt.Fprintln(out, l)
		}
	}
	return nil
}

// neonProblemsError is the deploy-time rule failures as one error.
func neonProblemsError(problems []string) error {
	return fmt.Errorf("neon: stopped before anything was changed:\n  %s", strings.Join(problems, "\n  "))
}

func neonSortedUnique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// newNeonSSMLister lists SSM parameter names under a path, recursively, with
// the env's AWS profile and region. Values are never read.
func newNeonSSMLister(env Env) ssmLister {
	return func(ctx context.Context, path string) ([]string, error) {
		opts := []func(*config.LoadOptions) error{}
		if env.AWSProfile != "" {
			opts = append(opts, config.WithSharedConfigProfile(env.AWSProfile))
		}
		if env.Region != "" {
			opts = append(opts, config.WithRegion(env.Region))
		}
		cfg, err := config.LoadDefaultConfig(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("loading the AWS config: %w", err)
		}
		client := ssm.NewFromConfig(cfg)
		var names []string
		p := ssm.NewGetParametersByPathPaginator(client, &ssm.GetParametersByPathInput{
			Path:           aws.String(path),
			Recursive:      aws.Bool(true),
			WithDecryption: aws.Bool(false),
		})
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				return nil, err
			}
			for _, prm := range page.Parameters {
				if prm.Name != nil {
					names = append(names, *prm.Name)
				}
			}
		}
		return names, nil
	}
}

// neonSessionToken is the session's token, or "" when it has none.
func neonSessionToken(s *neonSession) string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mode != sessionToken {
		return ""
	}
	return s.tok.Value
}

// neonSessionDegraded reports whether s is a destroy without a Neon login.
func neonSessionDegraded(s *neonSession) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mode == sessionDegraded
}

// beginNeonRunSession resolves the Neon token for env and makes it the
// active session. clear empties the holder; call it on every return.
func beginNeonRunSession(ctx context.Context, env Env, interactive bool) (*neonSession, func(), error) {
	tok, err := ensureNeonSession(ctx, env, interactive)
	if err != nil {
		return nil, func() {}, err
	}
	sess, clear := beginNeonSession(tok, neonTokenRefresher(defaultNeonAuthDeps()))
	return sess, clear, nil
}

// ---------------------------------------------------------------------------
// Deploy
// ---------------------------------------------------------------------------

// prepareNeonForDeploy runs everything between init and step 2: the Kept
// file is pruned, YAML, state and Neon are reconciled (pre), released
// objects are recorded and released, step 1 runs, state is read again and
// reconciled (post), dropped connections are recorded and released, the
// credentials go into the session, the deploy-time files are written and the
// lock is checked. On success the caller runs step 2 and then
// result.Cleanup.
func prepareNeonForDeploy(ctx context.Context, target handoffTarget, api neonAPI, tf terraformRunner, sess *neonSession, opts neonDeployOptions) (NeonDeployResult, error) {
	envName := neonEnvName(target)
	root := target.ProjectRoot
	envDir := neonEnvDir(target)
	env := target.Env
	result := NeonDeployResult{Cleanup: func() { _ = removeNeonDeployFiles(envDir) }}
	if sess == nil || api == nil {
		return result, errors.New("neon: the deploy needs a Neon login")
	}

	if err := removeNeonDeployFiles(envDir); err != nil {
		return result, fmt.Errorf("neon: removing old deploy-time files: %w", err)
	}

	st0, err := readNeonState(ctx, tf, envDir)
	if err != nil {
		return result, err
	}

	if entries, err := loadNeonKept(root, envName); err == nil && len(entries) > 0 {
		pruned := pruneNeonKept(ctx, api, &env, entries)
		if len(pruned) != len(entries) {
			if err := saveNeonKept(root, envName, pruned); err != nil {
				fmt.Printf("⚠️  %v\n", err)
			}
		}
	}

	listSSM := opts.ListSSM
	if listSSM == nil && !opts.NoAWS {
		listSSM = newNeonSSMLister(env)
	}
	if opts.NoAWS {
		listSSM = nil
	}

	pre, err := resolveNeon(ctx, api, listSSM, env, st0, neonPhasePre)
	if err != nil {
		return result, err
	}
	if len(pre.Problems) > 0 {
		return result, neonProblemsError(pre.Problems)
	}
	for _, n := range pre.Notices {
		fmt.Printf("ℹ️  %s\n", n)
	}

	releases := pre.Releases
	if len(pre.OutOfBand) > 0 {
		what := strings.Join(pre.OutOfBand, ", ")
		if !opts.Interactive {
			return result, fmt.Errorf("neon: %s was created by meroku but deleted outside it [V-N29]; run the deploy interactively to create it again (empty), or remove it from %s.yaml", what, envName)
		}
		choice := neonAuthPrompt(
			fmt.Sprintf("⚠️  %s was created by meroku but has been deleted outside it.", what),
			[]string{"Create it again (empty)", "Cancel"})
		if choice != 0 {
			return result, errNeonStopped
		}
		releases = append(releases, pre.OutOfBandReleases...)
	}
	releases = neonSortedUnique(releases)
	if err := neonRelease(ctx, os.Stdout, tf, root, envName, envDir, pre.KeptAdd, releases); err != nil {
		return result, err
	}

	// Connections that left the YAML keep objects in state until step 2 drops
	// their roles, and Terraform plans nothing, not even step 1's targeted
	// plan, while state holds objects whose provider configuration is gone.
	leaving, err := neonLeavingConnections(env, st0)
	if err != nil {
		return result, err
	}
	if len(leaving) > 0 {
		mainTF, err := os.ReadFile(filepath.Join(envDir, "main.tf"))
		if err != nil {
			return result, fmt.Errorf("neon: reading main.tf: %w", err)
		}
		remove, err := writeNeonRetiredFiles(envDir, leaving, nil, mainTF)
		result.Cleanup = remove
		if err != nil {
			return result, err
		}
		if err := initIfLockIncompleteWith(ctx, envDir, os.Stdout, tf); err != nil {
			return result, err
		}
	}

	if targets := expectedNeonAddresses(env.Neon); len(targets) > 0 {
		step1 := opts.Step1
		if step1 == nil {
			step1 = runNeonStep1
		}
		outcome, err := step1(ctx, target, planStepOptions{
			Targets:  targets,
			PlanFile: neonStep1PlanFile,
			Title:    neonStep1Title,
			Dir:      envDir,
		})
		if err != nil {
			neonPrintStep1Stop(releases, pre.KeptAdd)
			return result, err
		}
		if !outcome.NoChanges && !outcome.Applied {
			fmt.Println("\nNeon step 1 was not applied.")
			neonPrintStep1Stop(releases, pre.KeptAdd)
			return result, errNeonStopped
		}
		if outcome.Applied {
			result.Step2Title = neonStep2Title
		}
	}

	st2, err := readNeonState(ctx, tf, envDir)
	if err != nil {
		return result, err
	}
	post, err := resolveNeon(ctx, api, nil, env, st2, neonPhasePost)
	if err != nil {
		return result, err
	}
	if len(post.Problems) > 0 {
		return result, neonProblemsError(post.Problems)
	}
	if len(post.Dropped) > 0 {
		fmt.Println("ℹ️  The database of these Neon connections is gone; their roles are recorded as kept:")
		for _, c := range post.Dropped {
			fmt.Printf("   %s (%s)\n", neonAccessModuleAddress(c.Alias), c.Database)
		}
	}
	if err := neonRelease(ctx, os.Stdout, tf, root, envName, envDir, post.KeptAdd, neonSortedUnique(post.Releases)); err != nil {
		return result, err
	}

	creds, err := neonCredentialsJSON(post.Credentials)
	if err != nil {
		return result, err
	}
	sess.SetCredentials(creds)

	mainTF, err := os.ReadFile(filepath.Join(envDir, "main.tf"))
	if err != nil {
		return result, fmt.Errorf("neon: reading main.tf: %w", err)
	}
	remove, err := writeNeonRetiredFiles(envDir, post.Retired, post.SSMExclude, mainTF)
	result.Cleanup = remove
	if err != nil {
		return result, err
	}
	if err := initIfLockIncompleteWith(ctx, envDir, os.Stdout, tf); err != nil {
		return result, err
	}

	if opts.NoAWS {
		targets := []string{"module.neon"}
		d, err := neonDesiredFromEnv(env)
		if err != nil {
			return result, err
		}
		if d != nil {
			for alias := range d.connections {
				targets = append(targets, neonAccessModuleAddress(alias))
			}
		}
		for _, c := range post.Retired {
			targets = append(targets, neonAccessModuleAddress(c.Alias))
		}
		result.Step2Targets = neonSortedUnique(targets)
	}

	if entries, err := loadNeonKept(root, envName); err == nil {
		result.Kept = neonKeptReport(entries)
	}
	return result, nil
}

// neonPrintStep1Stop says what had already been released when step 1 did not
// go through.
func neonPrintStep1Stop(releases []string, kept []NeonKeptEntry) {
	if len(releases) == 0 {
		return
	}
	fmt.Println("Released from state before step 1:")
	for _, a := range releases {
		fmt.Printf("   %s\n", a)
	}
	for _, l := range neonKeptReport(kept) {
		fmt.Println(l)
	}
}

// runNeonStep1 plans the step-1 targets into their own plan file, shows the
// plan when it has changes, and applies that file. The plan file is removed
// on every return: in create mode it holds the project's owner password.
func runNeonStep1(ctx context.Context, target handoffTarget, opts planStepOptions) (neonStep1Outcome, error) {
	planPath := filepath.Join(opts.Dir, opts.planFile())
	defer os.Remove(planPath)

	if err := runTerraformPlanWithOptions(target, opts); err != nil {
		return neonStep1Outcome{}, err
	}
	cmd, err := newTerraformCmd(ctx, tfState, opts.Dir, "show", "-json", opts.planFile())
	if err != nil {
		return neonStep1Outcome{}, err
	}
	out, err := cmd.Output()
	if err != nil {
		return neonStep1Outcome{}, fmt.Errorf("neon: reading the step-1 plan failed: %w", err)
	}
	var plan TerraformPlanVisual
	if err := json.Unmarshal(out, &plan); err != nil {
		return neonStep1Outcome{}, errors.New("neon: the step-1 plan is not the JSON meroku expects")
	}
	if !planHasChanges(plan) {
		fmt.Println("✅ Neon step 1: the project and databases need no changes.")
		return neonStep1Outcome{NoChanges: true}, nil
	}
	res, err := showModernTerraformPlanTUIWithOptions(string(out), target, planTUIOptions{
		PlanFile:     opts.planFile(),
		Targets:      opts.Targets,
		AllowReplace: false,
		Title:        opts.Title,
	})
	if err != nil {
		return neonStep1Outcome{}, err
	}
	if res.Failed {
		return neonStep1Outcome{}, errors.New("neon: step 1 (project and databases) failed to apply")
	}
	return neonStep1Outcome{Applied: res.Applied}, nil
}

// planHasChanges reports whether a plan does anything.
func planHasChanges(plan TerraformPlanVisual) bool {
	for _, rc := range plan.ResourceChanges {
		for _, a := range rc.Change.Actions {
			if a != "no-op" && a != "read" {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Destroy
// ---------------------------------------------------------------------------

// neonDestroyPrep gets a Neon env's state ready for `terraform destroy`. It
// works from state and the main.tf on disk, reads no YAML, prompts for
// nothing and can run again after a restart.
//
// With a Neon login: V-N39 (nothing is released when it fails); credentials
// for every connection whose database still exists, as the admin role that
// created its roles; roles of a connection whose database is gone recorded
// as kept and the module released. Without one (degraded): every role is
// recorded as kept and every Neon address released. Then every created
// object is recorded as kept, and module.neon and every schema are released
// in one state rm: schemas are never dropped, and nothing under module.neon
// may be destroyed. Last, the deploy-time files for connections main.tf does
// not have, the lock check and the credentials. cleanup deletes the files;
// call it after the destroy.
func neonDestroyPrep(ctx context.Context, out io.Writer, target handoffTarget, tf terraformRunner, api neonAPI, sess *neonSession) (kept []string, cleanup func(), err error) {
	envName := neonEnvName(target)
	root := target.ProjectRoot
	envDir := neonEnvDir(target)
	cleanup = func() { _ = removeNeonDeployFiles(envDir) }
	if sess == nil {
		return nil, cleanup, errNeonSessionRequired
	}
	degraded := neonSessionDegraded(sess)
	if !degraded && api == nil {
		return nil, cleanup, errors.New("neon: the destroy needs a Neon login")
	}

	if err := removeNeonDeployFiles(envDir); err != nil {
		return nil, cleanup, fmt.Errorf("neon: removing old deploy-time files: %w", err)
	}
	st, err := readNeonState(ctx, tf, envDir)
	if err != nil {
		return nil, cleanup, err
	}
	// The env as state knows it: no YAML is read.
	env := Env{Project: target.Env.Project, Env: envName}

	var keptAdd []NeonKeptEntry
	var releases []string
	var retired []NeonConnectionState
	creds := map[string]PGCredential{}

	if degraded {
		releases = neonReleaseSet(st, func(string) bool { return true })
		keptAdd = neonKeptAllCreated(st)
		for _, alias := range neonSortedConnections(st) {
			keptAdd = append(keptAdd, neonKeptRoles(st, st.Connections[alias], NeonProjectInfo{})...)
		}
	} else {
		pre, err := resolveNeon(ctx, api, nil, env, st, neonPhasePre)
		if err != nil {
			return nil, cleanup, err
		}
		if len(pre.Problems) > 0 {
			return nil, cleanup, neonProblemsError(pre.Problems)
		}
		post, err := resolveNeon(ctx, api, nil, env, st, neonPhasePost)
		if err != nil {
			return nil, cleanup, err
		}
		if len(post.Problems) > 0 {
			return nil, cleanup, neonProblemsError(post.Problems)
		}
		releases = append(append(releases, pre.Releases...), post.Releases...)
		keptAdd = append(append(keptAdd, pre.KeptAdd...), post.KeptAdd...)
		retired = post.Retired
		creds = post.Credentials
	}

	if err := neonRelease(ctx, out, tf, root, envName, envDir, keptAdd, neonSortedUnique(releases)); err != nil {
		return nil, cleanup, err
	}

	mainTF, err := os.ReadFile(filepath.Join(envDir, "main.tf"))
	if err != nil {
		return nil, cleanup, fmt.Errorf("neon: reading main.tf: %w", err)
	}
	inMain := neonMainTFModules(mainTF)
	var outside []NeonConnectionState
	for _, c := range retired {
		if !inMain["neon_access_"+c.Alias] {
			outside = append(outside, c)
		}
	}
	remove, err := writeNeonRetiredFiles(envDir, outside, nil, mainTF)
	cleanup = remove
	if err != nil {
		return nil, cleanup, err
	}
	if err := initIfLockIncompleteWith(ctx, envDir, out, tf); err != nil {
		return nil, cleanup, err
	}
	if !degraded {
		json, err := neonCredentialsJSON(creds)
		if err != nil {
			return nil, cleanup, err
		}
		sess.SetCredentials(json)
	}

	entries, err := loadNeonKept(root, envName)
	if err != nil {
		return nil, cleanup, err
	}
	return neonKeptReport(entries), cleanup, nil
}

// mainTFHasNeonModule reports whether envDir's main.tf declares module
// "neon". Its data sources need a configured Neon provider, so a destroy
// without a Neon login is only possible when it does not.
func mainTFHasNeonModule(envDir string) bool {
	data, err := os.ReadFile(filepath.Join(envDir, "main.tf"))
	if err != nil {
		return false
	}
	return neonMainTFModules(data)["neon"]
}

// beginNeonDestroySession starts the session of a destroy of target before
// the destroy screen. Without a usable login it offers: log in, destroy
// without Neon (only when main.tf has no module "neon"), or cancel. It
// returns a no-op clear and a nil session when the env has no Neon.
func beginNeonDestroySession(ctx context.Context, target handoffTarget) (*neonSession, func(), error) {
	envDir := neonEnvDir(target)
	if !isNeonRun(target.Env, envDir) {
		return nil, func() {}, nil
	}
	env := target.Env
	if env.Env == "" {
		env.Env = neonEnvName(target)
	}
	tok, err := ensureNeonSession(ctx, env, false)
	if err == nil {
		sess, clear := beginNeonSession(tok, neonTokenRefresher(defaultNeonAuthDeps()))
		return sess, clear, nil
	}
	var authErr *NeonAuthError
	if !errors.As(err, &authErr) || !interactiveTerminal() {
		return nil, func() {}, err
	}
	if authErr.Kind == neonAuthShort {
		return beginNeonRunSession(ctx, env, true)
	}

	degradedOK := !mainTFHasNeonModule(envDir)
	fmt.Printf("⚠️  %s.\n", authErr.Error())
	var options []string
	var actions []string
	if authErr.Kind != neonAuthNeonctlMissing {
		options = append(options, "Log in (opens a browser)")
		actions = append(actions, "login")
	}
	if degradedOK {
		options = append(options, "Destroy without Neon: forget the Neon roles in state — they stay in Neon, listed")
		actions = append(actions, "degraded")
	} else {
		fmt.Printf("   To destroy without a Neon login, remove `neon:` from %s.yaml, run `meroku generate %s`, then destroy again.\n", env.Env, env.Env)
	}
	options = append(options, "Cancel")
	actions = append(actions, "cancel")

	choice := neonAuthPrompt("How do you want to continue?", options)
	if choice < 0 || choice >= len(actions) {
		return nil, func() {}, err
	}
	switch actions[choice] {
	case "login":
		if lerr := neonLogin(ctx); lerr != nil {
			return nil, func() {}, &NeonAuthError{Kind: authErr.Kind, Err: lerr}
		}
		return beginNeonRunSession(ctx, env, true)
	case "degraded":
		sess, clear := beginDegradedNeonSession()
		return sess, clear, nil
	}
	return nil, func() {}, err
}

// ---------------------------------------------------------------------------
// Reconnect plan and import
// ---------------------------------------------------------------------------

// ensureNeonTerraformSession makes a terraform command outside a deploy or
// destroy (the reconnect plan, an import) possible in a Neon env: Neon login,
// session, credentials and deploy-time files, without step 1. When step 1
// would create something, it stops: deploy this env first. With a session
// already active (an import inside a deploy) it does nothing. cleanup clears
// what it set up.
func ensureNeonTerraformSession(ctx context.Context, target handoffTarget) (cleanup func(), err error) {
	noop := func() {}
	if currentNeonSession() != nil {
		return noop, nil
	}
	envDir := neonEnvDir(target)
	if !isNeonRun(target.Env, envDir) && !isNeonDir(envDir) {
		return noop, nil
	}
	env := target.Env
	if env.Env == "" {
		env.Env = neonEnvName(target)
	}
	sess, clear, err := beginNeonRunSession(ctx, env, interactiveTerminal())
	if err != nil {
		return noop, err
	}
	fail := func(err error) (func(), error) {
		_ = removeNeonDeployFiles(envDir)
		clear()
		return noop, err
	}

	st, err := readNeonState(ctx, terraformOutputRunner, envDir)
	if err != nil {
		return fail(err)
	}
	for _, addr := range expectedNeonAddresses(env.Neon) {
		if !st.Managed[addr] {
			return fail(fmt.Errorf("neon: %s is not created yet; deploy this env first", addr))
		}
	}
	api := newNeonAPI(neonSessionToken(sess))
	post, err := resolveNeon(ctx, api, nil, env, st, neonPhasePost)
	if err != nil {
		return fail(err)
	}
	if len(post.Problems) > 0 {
		return fail(neonProblemsError(post.Problems))
	}
	if len(post.Dropped) > 0 {
		return fail(errors.New("neon: a Neon connection in state lost its database; deploy this env first"))
	}
	creds, err := neonCredentialsJSON(post.Credentials)
	if err != nil {
		return fail(err)
	}
	sess.SetCredentials(creds)
	mainTF, err := os.ReadFile(filepath.Join(envDir, "main.tf"))
	if err != nil {
		return fail(fmt.Errorf("neon: reading main.tf: %w", err))
	}
	remove, err := writeNeonRetiredFiles(envDir, post.Retired, post.SSMExclude, mainTF)
	if err != nil {
		remove()
		return fail(err)
	}
	return func() {
		remove()
		clear()
	}, nil
}

// neonTargetForDir is the target of a terraform dir env/<env> under a
// project root, with the env YAML read but never migrated or written.
func neonTargetForDir(dir string) handoffTarget {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	envName := filepath.Base(abs)
	root := filepath.Dir(filepath.Dir(abs))
	t := handoffTarget{ProjectRoot: root, EnvName: envName}
	for _, rel := range envFileSearchPaths(envName) {
		path := filepath.Join(root, rel)
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(root)) {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var e Env
		if err := yaml.Unmarshal(data, &e); err != nil {
			t.EnvLoadError = fmt.Sprintf("parsing %s: %v", path, err)
		}
		t.Env = e
		return t
	}
	t.EnvLoadError = fmt.Sprintf("no %s.yaml found under %s", envName, root)
	return t
}
