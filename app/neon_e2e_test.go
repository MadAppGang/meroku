//go:build neon_e2e

package main

// Real-Neon end-to-end test. It creates throwaway Neon projects named
// meroku-e2e-<run id>-<suffix>, drives meroku's own Neon deploy and destroy
// code against them, checks the result with real Postgres connections, and
// deletes every project it created, also when a step fails.
//
// Run with `task neon:e2e`, or:
//
//	MEROKU_NEON_E2E=1 go test -tags neon_e2e -run TestNeonE2E -count=1 -timeout 180m -v ./
//
// Needs: a Neon CLI login (`neon auth`), Terraform >= 1.14, Go. NEON_API_KEY
// must be unset: the test proves the CLI login is enough. AWS is never called:
// every deploy runs with NoAWS (step 2 targets only the Neon modules), the S3
// backend is overridden with a local one, and the AWS SDK's config and
// credential sources are pointed at nothing.
//
// Optional: MEROKU_NEON_E2E_ORG (the Neon organization; default: the only one
// the login belongs to), MEROKU_NEON_E2E_EVIDENCE (directory for the redacted
// log and the evidence JSON), MEROKU_NEON_E2E_REGION (default aws-us-east-2).
//
// Postgres connections go through the helper in testdata/neon_e2e_pgq, a
// separate Go module, so the app module does not depend on a Postgres driver.
// Secrets (the Neon token, owner passwords, service URLs) are registered with
// registerSecret and every logged line goes through redactSecrets.

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v2"
)

const (
	e2eNamePrefix = "meroku-e2e-"
	e2eProject    = "mke2e"
	e2eOwner      = "neondb_owner"
)

// ---------------------------------------------------------------------------
// Evidence
// ---------------------------------------------------------------------------

type e2eCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

type e2eStep struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Status     string     `json:"status"` // pass, fail, blocked, skipped
	Error      string     `json:"error,omitempty"`
	Simplified string     `json:"simplified,omitempty"`
	Notes      []string   `json:"notes,omitempty"`
	Checks     []e2eCheck `json:"checks"`
	Seconds    float64    `json:"seconds"`
}

type e2eEvidence struct {
	RunID      string            `json:"run_id"`
	Started    time.Time         `json:"started"`
	Finished   time.Time         `json:"finished"`
	Terraform  string            `json:"terraform"`
	NeonCLI    string            `json:"neon_cli"`
	Region     string            `json:"region"`
	Steps      []*e2eStep        `json:"steps"`
	Created    map[string]string `json:"created_projects"`
	Deleted    map[string]string `json:"deleted_projects"`
	Cleanup    []e2eCheck        `json:"cleanup"`
	AllPassed  bool              `json:"all_passed"`
	Simplified []string          `json:"simplifications"`
	// Findings are implementation problems the run hit, with the harness
	// workaround that let the later steps run.
	Findings []string `json:"findings"`
}

var errE2EBlocked = errors.New("blocked: a step this one depends on did not complete")

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

type e2eHarness struct {
	t        *testing.T
	ctx      context.Context
	repo     string
	work     string
	evidence string
	runID    string
	org      string
	region   string
	pgq      string
	log      *os.File
	logMu    sync.Mutex
	ev       e2eEvidence
	cur      *e2eStep
	deps     neonAuthDeps
	tok      NeonToken

	// Projects: p1 = create env (made by Terraform), p2 = reuse env (made by
	// the CLI), p3 = small create env for 9e (made by Terraform).
	p1, p2, p3 string

	// yRole is 9c's dropped-database role, whose Kept entry 9d's runs prune.
	yRole string
}

func TestNeonE2E(t *testing.T) {
	if os.Getenv("MEROKU_NEON_E2E") != "1" {
		t.Skip("set MEROKU_NEON_E2E=1 to run the real-Neon end-to-end test")
	}
	if os.Getenv("NEON_API_KEY") != "" {
		t.Fatal("NEON_API_KEY is set; this test proves the Neon CLI login alone works (R1.2). Unset it.")
	}

	h := newE2EHarness(t)
	defer h.finish()

	h.step("0", "U-6: plan with only the ephemeral variable and a postgresql provider", h.step0)
	h.step("1", "NEON_API_KEY unset: resolveNeonToken uses the Neon CLI login (R1.2)", h.step1)
	h.step("2", "Create env (PG 17): project, app + app2, services r/w/x", h.step2)
	h.step("3", "Second plan has no changes (R4.6)", h.step3)
	h.step("4", "Enforcement through the service URLs", h.step4)
	h.step("5", "Re-plan, access change, schema change, password_version", h.step5)
	h.step("6", "Reuse env against a CLI-created project", h.step6)
	h.step("7", "Shared project: env stage with its own database", h.step7)
	h.step("8", "Move x from app2 to app", h.step8)
	h.step("9", "Retire x's access", h.step9)
	h.step("9b", "Remove database app2 from YAML", h.step9b)
	h.step("9c", "Remove a service whose database was dropped first (K-H1)", h.step9c)
	h.step("9d", "Remove a created database and decline step 1", h.step9d)
	h.step("9e", "Remove neon: from a small create-mode env (R6-3)", h.step9e)
	h.step("10", "Reuse env destroy", h.step10)
	h.step("11", "Never delete (Issue 3)", h.step11)
	h.skip("12", "Optional PG14 reuse env (V-N28)", "optional; not run")

	for _, s := range h.ev.Steps {
		if s.Status == "fail" || s.Status == "blocked" {
			t.Errorf("step %s (%s): %s %s", s.ID, s.Title, s.Status, s.Error)
		}
	}
}

func newE2EHarness(t *testing.T) *e2eHarness {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Dir(wd)

	b := make([]byte, 3)
	_, _ = rand.Read(b)
	runID := hex.EncodeToString(b)

	evidence := os.Getenv("MEROKU_NEON_E2E_EVIDENCE")
	if evidence == "" {
		evidence = filepath.Join(os.TempDir(), "meroku-neon-e2e-evidence-"+runID)
	}
	if err := os.MkdirAll(evidence, 0o700); err != nil {
		t.Fatal(err)
	}
	work, err := os.MkdirTemp("", "meroku-neon-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	logf, err := os.OpenFile(filepath.Join(evidence, "neon-e2e.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	region := os.Getenv("MEROKU_NEON_E2E_REGION")
	if region == "" {
		region = "aws-us-east-2"
	}

	// No AWS: nothing to find credentials or a region in, and no IMDS.
	for _, k := range []string{"AWS_PROFILE", "AWS_DEFAULT_PROFILE", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_REGION", "AWS_DEFAULT_REGION"} {
		_ = os.Unsetenv(k)
	}
	_ = os.Setenv("AWS_CONFIG_FILE", filepath.Join(work, "no-aws-config"))
	_ = os.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(work, "no-aws-credentials"))
	_ = os.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	_ = os.Setenv("TF_IN_AUTOMATION", "1")
	_ = os.Unsetenv("TF_VAR_neon_pg_credentials")
	// What main() does first: NEON_API_KEY (unset here) moves into memory.
	captureNeonEnv(os.Getenv, os.Unsetenv)

	ctx, cancel := context.WithTimeout(context.Background(), 175*time.Minute)
	t.Cleanup(cancel)

	h := &e2eHarness{
		t: t, ctx: ctx, repo: repo, work: work, evidence: evidence, runID: runID,
		region: region, log: logf, deps: defaultNeonAuthDeps(),
	}
	h.ev = e2eEvidence{RunID: runID, Started: time.Now().UTC(), Region: region,
		Created: map[string]string{}, Deleted: map[string]string{}}
	h.logf("run %s, work dir %s, evidence %s", runID, work, evidence)

	if out, err := exec.Command("terraform", "version").Output(); err == nil {
		h.ev.Terraform = strings.SplitN(string(out), "\n", 2)[0]
	}
	if out, err := exec.Command(neonCLI(), "--version").Output(); err == nil {
		h.ev.NeonCLI = neonCLI() + " " + strings.TrimSpace(string(out))
	}

	// Project layout meroku generates from: infrastructure/{env/main.hbs,
	// modules}, project/<env>.yaml, env/<env>/.
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(work, "infrastructure", "env"), 0o755))
	must(os.MkdirAll(filepath.Join(work, "project"), 0o755))
	tpl, err := os.ReadFile(filepath.Join(repo, "env", "main.hbs"))
	must(err)
	must(os.WriteFile(filepath.Join(work, "infrastructure", "env", "main.hbs"), tpl, 0o644))
	must(os.Symlink(filepath.Join(repo, "modules"), filepath.Join(work, "infrastructure", "modules")))
	if v, err := os.ReadFile(filepath.Join(repo, "version.txt")); err == nil {
		must(os.WriteFile(filepath.Join(work, "infrastructure", "version.txt"), v, 0o644))
	}

	// The Postgres helper (its own module).
	h.pgq = filepath.Join(work, "pgq")
	build := exec.Command("go", "build", "-o", h.pgq, ".")
	build.Dir = filepath.Join(wd, "testdata", "neon_e2e_pgq")
	build.Env = childEnv()
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the pgx helper: %v\n%s", err, out)
	}

	// Every generate and load is relative to the project root.
	must(os.Chdir(work))
	t.Cleanup(func() { _ = os.Chdir(wd) })
	return h
}

// logf writes one redacted line to the log and to the test output.
func (h *e2eHarness) logf(format string, args ...interface{}) {
	line := redactSecrets(fmt.Sprintf(format, args...))
	h.logMu.Lock()
	defer h.logMu.Unlock()
	fmt.Fprintf(h.log, "%s %s\n", time.Now().UTC().Format("15:04:05"), line)
	h.t.Log(line)
}

// logBlock writes captured output, redacted and indented.
func (h *e2eHarness) logBlock(title, text string) {
	text = strings.TrimRight(redactSecrets(text), "\n")
	if text == "" {
		return
	}
	h.logMu.Lock()
	fmt.Fprintf(h.log, "----- %s\n", title)
	for _, l := range strings.Split(text, "\n") {
		fmt.Fprintf(h.log, "    %s\n", l)
	}
	h.logMu.Unlock()
}

func (h *e2eHarness) check(name string, ok bool, detail string, args ...interface{}) bool {
	d := redactSecrets(fmt.Sprintf(detail, args...))
	h.cur.Checks = append(h.cur.Checks, e2eCheck{Name: name, OK: ok, Detail: d})
	mark := "PASS"
	if !ok {
		mark = "FAIL"
		h.cur.Status = "fail"
	}
	h.logf("  [%s] %s — %s", mark, name, d)
	return ok
}

func (h *e2eHarness) note(format string, args ...interface{}) {
	n := redactSecrets(fmt.Sprintf(format, args...))
	h.cur.Notes = append(h.cur.Notes, n)
	h.logf("  note: %s", n)
}

func (h *e2eHarness) simplified(format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...)
	if h.cur.Simplified != "" {
		h.cur.Simplified += " "
	}
	h.cur.Simplified += s
	h.ev.Simplified = append(h.ev.Simplified, h.cur.ID+": "+s)
}

func (h *e2eHarness) step(id, title string, fn func() error) {
	s := &e2eStep{ID: id, Title: title, Status: "pass", Checks: []e2eCheck{}}
	h.ev.Steps = append(h.ev.Steps, s)
	h.cur = s
	h.logf("===== step %s: %s", id, title)
	start := time.Now()
	err := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
			}
		}()
		return fn()
	}()
	s.Seconds = time.Since(start).Round(time.Second).Seconds()
	if errors.Is(err, errE2EBlocked) {
		s.Status = "blocked"
		s.Error = err.Error()
	} else if err != nil {
		s.Status = "fail"
		s.Error = redactSecrets(err.Error())
	}
	h.logf("===== step %s: %s (%s)", id, strings.ToUpper(s.Status), time.Since(start).Round(time.Second))
	if s.Error != "" {
		h.logf("      error: %s", s.Error)
	}
}

func (h *e2eHarness) skip(id, title, why string) {
	h.ev.Steps = append(h.ev.Steps, &e2eStep{ID: id, Title: title, Status: "skipped", Notes: []string{why}, Checks: []e2eCheck{}})
	h.logf("===== step %s: SKIPPED (%s)", id, why)
}

// ---------------------------------------------------------------------------
// Neon token, API and CLI
// ---------------------------------------------------------------------------

// token is the CLI login's token with at least 5 minutes left, refreshed
// the way meroku does (wait for expiry, then `neon me`).
func (h *e2eHarness) token() (NeonToken, error) {
	if h.tok.Value != "" && (h.tok.ExpiresAt.IsZero() || time.Until(h.tok.ExpiresAt) > neonMinTerraformLife+time.Minute) {
		return h.tok, nil
	}
	tok, err := resolveNeonToken(h.ctx, h.deps, neonMinTerraformLife+time.Minute)
	var ae *NeonAuthError
	if err != nil && errors.As(err, &ae) && (ae.Kind == neonAuthShort || ae.Kind == neonAuthExpired) && tok.Value != "" {
		h.logf("token has %s left; waiting for it to expire, then refreshing", ae.Remaining.Round(time.Second))
		tok, err = waitAndRefreshNeonToken(h.ctx, h.deps, tok, io.Discard)
	}
	if err != nil {
		return NeonToken{}, err
	}
	registerSecret(tok.Value)
	h.tok = tok
	return tok, nil
}

func (h *e2eHarness) api() neonAPI {
	tok, err := h.token()
	if err != nil {
		panic(fmt.Sprintf("no Neon token: %v", err))
	}
	return newNeonAPI(tok.Value)
}

// session starts meroku's Neon session with a fresh token.
func (h *e2eHarness) session() (*neonSession, func(), error) {
	tok, err := h.token()
	if err != nil {
		return nil, func() {}, err
	}
	s, clear := beginNeonSession(tok, neonTokenRefresher(h.deps))
	return s, clear, nil
}

// cli runs the Neon CLI with its own login (no NEON_API_KEY). The output is
// returned, never logged: `projects create` prints connection URIs.
func (h *e2eHarness) cli(args ...string) ([]byte, error) {
	full := append(append([]string{}, args...), "--no-analytics")
	cmd := exec.CommandContext(h.ctx, neonCLI(), full...)
	cmd.Env = append(childEnv(), "CI=true")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	h.logf("$ %s %s → %v", neonCLI(), strings.Join(full, " "), errString(err))
	if err != nil {
		return out, fmt.Errorf("%s %s: %w: %s", neonCLI(), args[0], err, redactSecrets(lastLines(stderr.String(), 5)))
	}
	return out, nil
}

func errString(err error) string {
	if err == nil {
		return "ok"
	}
	return err.Error()
}

// defaultBranch is the project's default branch id.
func (h *e2eHarness) defaultBranch(project string) (string, error) {
	brs, err := h.api().ListBranches(h.ctx, project)
	if err != nil {
		return "", err
	}
	for _, b := range brs {
		if b.Default {
			return b.ID, nil
		}
	}
	return "", fmt.Errorf("project %s has no default branch", project)
}

func (h *e2eHarness) roles(project string) (map[string]bool, error) {
	br, err := h.defaultBranch(project)
	if err != nil {
		return nil, err
	}
	rs, err := h.api().ListRoles(h.ctx, project, br)
	if err != nil {
		return nil, err
	}
	m := map[string]bool{}
	for _, r := range rs {
		m[r.Name] = true
	}
	return m, nil
}

func (h *e2eHarness) databases(project string) (map[string]bool, error) {
	br, err := h.defaultBranch(project)
	if err != nil {
		return nil, err
	}
	ds, err := h.api().ListDatabases(h.ctx, project, br)
	if err != nil {
		return nil, err
	}
	m := map[string]bool{}
	for _, d := range ds {
		m[d.Name] = true
	}
	return m, nil
}

// ownerDSN is the owner role's direct connection string for db.
func (h *e2eHarness) ownerDSN(project, db string) (string, error) {
	br, err := h.defaultBranch(project)
	if err != nil {
		return "", err
	}
	eps, err := h.api().ListEndpoints(h.ctx, project, br)
	if err != nil {
		return "", err
	}
	host := ""
	for _, e := range eps {
		if e.Type == "read_write" {
			host = e.Host
		}
	}
	if host == "" {
		return "", fmt.Errorf("project %s has no read_write endpoint", project)
	}
	pw, err := h.api().RevealRolePassword(h.ctx, project, br, e2eOwner)
	if err != nil {
		return "", err
	}
	registerSecret(pw)
	dsn := fmt.Sprintf("postgresql://%s:%s@%s/%s?sslmode=require", e2eOwner, url.QueryEscape(pw), host, db)
	registerSecret(dsn)
	return dsn, nil
}

func (h *e2eHarness) ownerPassword(project string) (string, error) {
	br, err := h.defaultBranch(project)
	if err != nil {
		return "", err
	}
	pw, err := h.api().RevealRolePassword(h.ctx, project, br, e2eOwner)
	if err == nil {
		registerSecret(pw)
	}
	return pw, err
}

// createProjectCLI creates a throwaway project with the Neon CLI and returns
// its id. Only names under this run's prefix are ever created.
func (h *e2eHarness) createProjectCLI(suffix string) (string, error) {
	name := h.projectName(suffix)
	out, err := h.cli("projects", "create", "--name", name, "--org-id", h.org, "--region-id", h.region, "--pg-version", "18", "-o", "json")
	var resp struct {
		Project struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"project"`
		ConnectionURIs []struct {
			URI string `json:"connection_uri"`
		} `json:"connection_uris"`
	}
	if jerr := json.Unmarshal(out, &resp); jerr == nil {
		for _, c := range resp.ConnectionURIs {
			if u, perr := url.Parse(c.URI); perr == nil {
				if pw, ok := u.User.Password(); ok {
					registerSecret(pw)
				}
			}
			registerSecret(c.URI)
		}
	}
	if resp.Project.ID != "" {
		h.ev.Created[resp.Project.ID] = name
	}
	if err != nil {
		return "", err
	}
	if resp.Project.ID == "" || resp.Project.Name != name {
		return "", errors.New("neon projects create returned no project")
	}
	h.logf("created project %s (%s) with the CLI", resp.Project.ID, name)
	return resp.Project.ID, nil
}

func (h *e2eHarness) projectName(suffix string) string {
	return e2eNamePrefix + h.runID + "-" + suffix
}

// ---------------------------------------------------------------------------
// Postgres
// ---------------------------------------------------------------------------

type pgResult struct {
	SQL   string     `json:"sql"`
	OK    bool       `json:"ok"`
	Code  string     `json:"code"`
	Error string     `json:"error"`
	Rows  [][]string `json:"rows"`
	Tag   string     `json:"tag"`
}

func (r pgResult) first() string {
	if len(r.Rows) > 0 && len(r.Rows[0]) > 0 {
		return r.Rows[0][0]
	}
	return ""
}

// sql connects with dsn and runs stmts on one connection. conn is the
// connection result; results has one entry per statement.
func (h *e2eHarness) sql(who, dsn string, stmts ...string) (conn pgResult, results []pgResult) {
	cmd := exec.CommandContext(h.ctx, h.pgq, stmts...)
	cmd.Env = append(childEnv(), "PGQ_DSN="+dsn)
	out, _ := cmd.Output()
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	first := true
	for sc.Scan() {
		var r pgResult
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			continue
		}
		if first {
			conn = r
			first = false
			continue
		}
		results = append(results, r)
	}
	if first {
		conn = pgResult{Error: "no output from the pgx helper"}
	}
	if !conn.OK {
		h.logf("  sql %s: connect refused: %s %s", who, conn.Code, conn.Error)
	}
	for _, r := range results {
		res := "ok"
		if !r.OK {
			res = r.Code + " " + r.Error
		} else if v := r.first(); v != "" {
			res = "ok → " + v
		}
		h.logf("  sql %s: %s ⇒ %s", who, r.SQL, res)
	}
	return conn, results
}

// sql1 runs one statement and returns its result (a failed connection is
// reported as the result).
func (h *e2eHarness) sql1(who, dsn, stmt string) pgResult {
	conn, rs := h.sql(who, dsn, stmt)
	if !conn.OK {
		conn.SQL = stmt
		return conn
	}
	if len(rs) == 0 {
		return pgResult{SQL: stmt, Error: "no result"}
	}
	return rs[0]
}

func denied(r pgResult) bool { return !r.OK && r.Code == "42501" }

// withDB returns u with its database path replaced.
func withDB(u, db string) string {
	p, err := url.Parse(u)
	if err != nil {
		return u
	}
	p.Path = "/" + db
	return p.String()
}

// direct returns u with the pooler host replaced by the direct host.
func direct(u string) string { return strings.Replace(u, "-pooler.", ".", 1) }

// ---------------------------------------------------------------------------
// Env YAML, generate, terraform
// ---------------------------------------------------------------------------

type e2eService struct {
	Name string
	Neon *NeonAccess
}

type e2eEnv struct {
	Name     string
	Neon     *Neon
	Services []e2eService
}

const e2eBaseYAML = `account_id: "000000000000"
alb:
  enabled: false
auto_deploy: false
aws_profile: ""
cloudfront_distributions: []
cognito:
  enabled: false
domain:
  enabled: false
ecr_strategy: local
ecr_trusted_accounts: []
egress_strategy: public_ip
postgres:
  enabled: false
project: %s
env: %s
pubsub_appsync:
  enabled: false
region: us-east-2
schema_version: %d
ses:
  enabled: false
sqs:
  enabled: false
state_bucket: %s-terraform-state-%s
state_file: null
use_default_vpc: true
vpc_cidr: 10.0.0.0/16
workload:
  backend_cpu: "256"
  backend_desired_count: 1
  backend_image_port: 8080
  backend_memory: "512"
  backend_runtime: fargate
  bucket_postfix: null
  bucket_public: false
  enable_github_oidc: false
  github_oidc_subjects: []
  install_pg_admin: false
  setup_fcnsns: false
  xray_enabled: false
`

func (e e2eEnv) yaml() (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, e2eBaseYAML, e2eProject, e.Name, CurrentSchemaVersion, e2eProject, e.Name)
	rest := yaml.MapSlice{}
	if e.Neon != nil {
		rest = append(rest, yaml.MapItem{Key: "neon", Value: e.Neon})
	}
	var svcs []yaml.MapSlice
	for _, s := range e.Services {
		m := yaml.MapSlice{
			{Key: "name", Value: s.Name},
			{Key: "enabled", Value: true},
			{Key: "container_port", Value: 3000},
			{Key: "host_port", Value: 3000},
			{Key: "cpu", Value: 256},
			{Key: "memory", Value: 512},
			{Key: "desired_count", Value: 1},
			{Key: "runtime", Value: "fargate"},
			{Key: "network_access", Value: "full_open"},
			{Key: "ecr_config", Value: yaml.MapSlice{{Key: "mode", Value: "create_ecr"}}},
		}
		if s.Neon != nil {
			m = append(m, yaml.MapItem{Key: "neon", Value: s.Neon})
		}
		svcs = append(svcs, m)
	}
	if len(svcs) > 0 {
		rest = append(rest, yaml.MapItem{Key: "services", Value: svcs})
	} else {
		rest = append(rest, yaml.MapItem{Key: "services", Value: []interface{}{}})
	}
	out, err := yaml.Marshal(rest)
	if err != nil {
		return "", err
	}
	b.Write(out)
	return b.String(), nil
}

func (h *e2eHarness) envDir(env string) string { return filepath.Join(h.work, "env", env) }

// generate writes the env YAML and runs meroku's generate, then overrides the
// S3 backend with a local one (no AWS).
func (h *e2eHarness) generate(e e2eEnv) error {
	y, err := e.yaml()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(h.work, "project", e.Name+".yaml"), []byte(y), 0o600); err != nil {
		return err
	}
	var genErr error
	out := captureStdout(func() { genErr = generateEnvironmentFiles(e.Name) })
	h.logBlock("generate "+e.Name, out)
	if genErr != nil {
		return fmt.Errorf("generate %s: %w", e.Name, genErr)
	}
	return os.WriteFile(filepath.Join(h.envDir(e.Name), "backend_override.tf"),
		[]byte("terraform {\n  backend \"local\" {}\n}\n"), 0o644)
}

// tf runs terraform through meroku's newTerraformCmd and returns the combined
// output and the exit code.
func (h *e2eHarness) tf(purpose tfPurpose, dir string, args ...string) (string, int, error) {
	cmd, err := newTerraformCmd(h.ctx, purpose, dir, args...)
	if err != nil {
		return "", -1, err
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err = cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		return buf.String(), -1, err
	}
	h.logf("$ terraform %s (in env/%s) → exit %d", strings.Join(args, " "), filepath.Base(dir), code)
	return buf.String(), code, nil
}

func (h *e2eHarness) tfOK(purpose tfPurpose, dir string, args ...string) (string, error) {
	out, code, err := h.tf(purpose, dir, args...)
	if err != nil {
		return out, err
	}
	if code != 0 {
		h.logBlock("terraform "+args[0]+" output", lastLines(out, 40))
		return out, fmt.Errorf("terraform %s exited %d: %s", args[0], code, redactSecrets(lastLines(out, 12)))
	}
	return out, nil
}

func (h *e2eHarness) init(env string) error {
	_, err := h.tfOK(tfState, h.envDir(env), "init", "-input=false", "-no-color")
	return err
}

func (h *e2eHarness) loadEnv(env string) (Env, error) {
	var e Env
	var err error
	out := captureStdout(func() { e, err = loadEnv(env) })
	h.logBlock("load "+env, out)
	return e, err
}

// orphanFinding records the step-1 orphaned-provider failure.
func (h *e2eHarness) orphanFinding(d e2eDeploy) {
	if !d.Step1OrphanProvider {
		return
	}
	h.check("step 1 plans while a connection leaves the render", false,
		"Error: Provider configuration not present for module.neon_access_<alias> orphans")
	for _, f := range h.ev.Findings {
		if strings.HasPrefix(f, "Step 9b") {
			return
		}
	}
	h.ev.Findings = append(h.ev.Findings, "Step 9b: removing a created database that has a Neon connection from the YAML breaks every later deploy of the env. prepareNeonForDeploy releases the database, then runs step 1 (plan -target=module.neon...) before writeNeonRetiredFiles; the retired connection's resources are still in state while main.tf has no provider \"postgresql\" alias for them, and Terraform fails the targeted plan with \"Error: Provider configuration not present ... module.neon_access_<alias>.postgresql_grant.public_database[0] (orphan)\". The release has already happened, so the next deploy fails the same way. Fix direction: write the retired provider blocks (neon_retired.tf, plus the override when main.tf lacks the source) before step 1; the postgresql providers are pruned from the step-1 plan, so placeholder credentials are enough.")
}

// stateList is `terraform state list`.
func (h *e2eHarness) stateList(env string) ([]string, error) {
	out, err := h.tfOK(tfState, h.envDir(env), "state", "list")
	if err != nil {
		return nil, err
	}
	var addrs []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			addrs = append(addrs, l)
		}
	}
	return addrs, nil
}

// connections reads env's Neon connections (alias → state) from state.
func (h *e2eHarness) connections(env string) (map[string]NeonConnectionState, error) {
	st, err := readNeonState(h.ctx, terraformOutputRunner, h.envDir(env))
	if err != nil {
		return nil, err
	}
	return st.Connections, nil
}

// aliasFor is the connection alias of database db in env.
func (h *e2eHarness) aliasFor(env, db string) (string, error) {
	cs, err := h.connections(env)
	if err != nil {
		return "", err
	}
	for a, c := range cs {
		if c.Database == db {
			return a, nil
		}
	}
	return "", fmt.Errorf("env %s has no Neon connection for database %s in state", env, db)
}

// console evaluates expr with `terraform console` in env's dir.
func (h *e2eHarness) console(env, expr string) (string, error) {
	cmd, err := newTerraformCmd(h.ctx, tfState, h.envDir(env), "console", "-no-color")
	if err != nil {
		return "", err
	}
	cmd.Stdin = strings.NewReader(expr + "\n")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("terraform console: %w: %s", err, redactSecrets(lastLines(stderr.String(), 6)))
	}
	var s string
	if err := json.Unmarshal(bytes.TrimSpace(out), &s); err != nil {
		return "", fmt.Errorf("terraform console printed something other than a string")
	}
	return s, nil
}

// serviceURLs reads a connection's service URLs and role names.
func (h *e2eHarness) serviceURLs(env, db string) (urls, roles map[string]string, err error) {
	alias, err := h.aliasFor(env, db)
	if err != nil {
		return nil, nil, err
	}
	// terraform console prints "(known after apply)" for a module output
	// built over a resource map with no instances, so a connection with no
	// service roles is answered from its state record.
	cs, err := h.connections(env)
	if err != nil {
		return nil, nil, err
	}
	if len(cs[alias].Roles) == 0 {
		return map[string]string{}, map[string]string{}, nil
	}
	mod := "module." + "neon_access_" + alias
	// jsonencode carries every nested sensitive mark onto its one string, so
	// nonsensitive on that string unmarks it all. A connection with no
	// services has an unmarked empty map, where nonsensitive errors: try()
	// falls back to the plain encoding.
	m := mod + ".database_urls"
	s, err := h.console(env, "try(nonsensitive(jsonencode("+m+")), jsonencode("+m+"))")
	if err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal([]byte(s), &urls); err != nil {
		return nil, nil, err
	}
	for _, u := range urls {
		registerSecret(u)
		if p, perr := url.Parse(u); perr == nil {
			if pw, ok := p.User.Password(); ok {
				registerSecret(pw)
				registerSecret(url.QueryEscape(pw))
			}
		}
	}
	s, err = h.console(env, "jsonencode("+mod+".role_names)")
	if err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal([]byte(s), &roles); err != nil {
		return nil, nil, err
	}
	return urls, roles, nil
}

// captureStdout runs fn with os.Stdout redirected and returns what it wrote.
func captureStdout(fn func()) string {
	r, w, err := os.Pipe()
	if err != nil {
		fn()
		return ""
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() { os.Stdout = old }()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-done
}

// ---------------------------------------------------------------------------
// Deploy and destroy through meroku's Neon code
// ---------------------------------------------------------------------------

type e2eDeployOpts struct {
	// PlanOnly plans step 2 with -detailed-exitcode and does not apply it.
	PlanOnly bool
	// DeclineStep1 declines a step-1 plan that has changes, as a user would.
	DeclineStep1 bool
	// OnStep2Plan runs between the step-2 plan and its apply, while the
	// deploy-time files exist.
	OnStep2Plan func(planPath string, planJSON []byte)
}

type e2eDeploy struct {
	Output string
	// Step1OrphanProvider: the step-1 plan failed with "Provider configuration
	// not present" (an orphaned connection's provider block is missing).
	Step1OrphanProvider bool
	Step1Changes        bool
	Step1Applied        bool
	Step1Plan           []byte
	Step1Refusal        string
	Step2Exit           int
	Step2Plan           []byte
	Step2Targets        []string
	Kept                []string
	Err                 error
}

// deploy runs a deploy the way deploy.go does after generate: Kept report,
// init, Neon session, prepareNeonForDeploy (NoAWS), step 2, Kept report.
// Step 1 and step 2 are plain terraform runs instead of the TUI screens.
func (h *e2eHarness) deploy(env string, o e2eDeployOpts) (r e2eDeploy) {
	var out strings.Builder
	defer func() {
		r.Output = out.String()
		h.logBlock("deploy "+env+" output", r.Output)
		if r.Err != nil {
			h.logf("deploy %s: %v", env, r.Err)
		}
	}()
	resetNeonStateCache()
	defer resetNeonStateCache()

	e, err := h.loadEnv(env)
	if err != nil {
		r.Err = err
		return r
	}
	envDir := h.envDir(env)
	out.WriteString(captureStdout(func() { printNeonKeptReport(h.work, env) }))
	if err := h.init(env); err != nil {
		r.Err = err
		return r
	}
	if !isNeonRun(e, envDir) {
		r.Err = errors.New("not a Neon run: no neon: and no Neon state")
		return r
	}

	sess, clear, err := h.session()
	if err != nil {
		r.Err = err
		return r
	}
	defer clear()
	target := handoffTarget{ProjectRoot: h.work, EnvName: env, Env: e}

	step1 := func(ctx context.Context, target handoffTarget, opts planStepOptions) (neonStep1Outcome, error) {
		planFile := opts.planFile()
		defer os.Remove(filepath.Join(opts.Dir, planFile))
		if err := sess.EnsureFresh(ctx, io.Discard); err != nil {
			return neonStep1Outcome{}, err
		}
		args := []string{"plan", "-no-color", "-input=false", "-out=" + planFile}
		for _, t := range opts.Targets {
			args = append(args, "-target="+t)
		}
		po, code, err := h.tf(tfConfig, opts.Dir, args...)
		if err != nil {
			return neonStep1Outcome{}, err
		}
		if code != 0 {
			if strings.Contains(po, "Provider configuration not present") {
				r.Step1OrphanProvider = true
			}
			fmt.Println(lastLines(po, 30))
			if address, ok := neonPreventDestroyError(po); ok {
				r.Step1Refusal = neonRefusalMessage(address, env+".yaml")
				fmt.Println(r.Step1Refusal)
				return neonStep1Outcome{}, errors.New("neon step 1: replacement refused")
			}
			return neonStep1Outcome{}, fmt.Errorf("neon step 1 plan exited %d", code)
		}
		js, err := h.tfOK(tfState, opts.Dir, "show", "-json", planFile)
		if err != nil {
			return neonStep1Outcome{}, err
		}
		r.Step1Plan = []byte(js)
		var plan TerraformPlanVisual
		if err := json.Unmarshal([]byte(js), &plan); err != nil {
			return neonStep1Outcome{}, err
		}
		if !planHasChanges(plan) {
			fmt.Println("✅ Neon step 1: the project and databases need no changes.")
			return neonStep1Outcome{NoChanges: true}, nil
		}
		r.Step1Changes = true
		if o.DeclineStep1 {
			fmt.Println("(harness) step 1 plan declined")
			return neonStep1Outcome{}, nil
		}
		if err := sess.EnsureFresh(ctx, io.Discard); err != nil {
			return neonStep1Outcome{}, err
		}
		if _, err := h.tfOK(tfConfig, opts.Dir, "apply", "-no-color", "-input=false", planFile); err != nil {
			return neonStep1Outcome{}, err
		}
		r.Step1Applied = true
		return neonStep1Outcome{Applied: true}, nil
	}

	var res NeonDeployResult
	out.WriteString(captureStdout(func() {
		res, err = prepareNeonForDeploy(h.ctx, target, newNeonAPI(neonSessionToken(sess)), terraformOutputRunner, sess, neonDeployOptions{
			NoAWS: true,
			Step1: step1,
		})
	}))
	if res.Cleanup != nil {
		defer res.Cleanup()
	}
	if err != nil {
		r.Err = err
		return r
	}
	r.Step2Targets = res.Step2Targets
	r.Kept = res.Kept

	// Step 2.
	if err := sess.EnsureFresh(h.ctx, io.Discard); err != nil {
		r.Err = err
		return r
	}
	args := []string{"plan", "-no-color", "-input=false", "-out=tfplan"}
	if o.PlanOnly {
		args = append(args, "-detailed-exitcode")
	}
	for _, t := range res.Step2Targets {
		args = append(args, "-target="+t)
	}
	planPath := filepath.Join(envDir, "tfplan")
	defer os.Remove(planPath)
	po, code, err := h.tf(tfConfig, envDir, args...)
	if err != nil {
		r.Err = err
		return r
	}
	r.Step2Exit = code
	if code == 1 || (!o.PlanOnly && code != 0) {
		h.logBlock("step 2 plan", lastLines(po, 40))
		r.Err = fmt.Errorf("step 2 plan exited %d: %s", code, redactSecrets(lastLines(po, 12)))
		return r
	}
	js, err := h.tfOK(tfState, envDir, "show", "-json", "tfplan")
	if err != nil {
		r.Err = err
		return r
	}
	r.Step2Plan = []byte(js)
	if o.OnStep2Plan != nil {
		o.OnStep2Plan(planPath, r.Step2Plan)
	}
	if o.PlanOnly {
		return r
	}
	if err := sess.EnsureFresh(h.ctx, io.Discard); err != nil {
		r.Err = err
		return r
	}
	ao, code, err := h.tf(tfConfig, envDir, "apply", "-no-color", "-input=false", "tfplan")
	if err == nil && code != 0 {
		h.logBlock("step 2 apply", lastLines(ao, 40))
		if remedy := neonRoleInUseRemedy(ao, e2eOwner); remedy != "" {
			out.WriteString(remedy + "\n")
		}
		err = fmt.Errorf("step 2 apply exited %d: %s", code, redactSecrets(lastLines(ao, 12)))
	}
	if err != nil {
		r.Err = err
		return r
	}
	out.WriteString(captureStdout(func() { printNeonKeptReport(h.work, env) }))
	return r
}

type e2eDestroy struct {
	Output string
	Kept   []string
	Err    error
}

// destroy runs neonDestroyPrep and `terraform destroy` as the destroy screen
// does (-refresh=false), limited to the Neon modules left in state (NoAWS).
func (h *e2eHarness) destroy(env string, degraded bool) (r e2eDestroy) {
	var out bytes.Buffer
	defer func() {
		r.Output = out.String()
		h.logBlock("destroy "+env+" output", r.Output)
		if r.Err != nil {
			h.logf("destroy %s: %v", env, r.Err)
		}
	}()
	resetNeonStateCache()
	defer resetNeonStateCache()
	e, err := h.loadEnv(env)
	if err != nil {
		r.Err = err
		return r
	}
	envDir := h.envDir(env)
	out.WriteString(captureStdout(func() { printNeonKeptReport(h.work, env) }))
	if err := h.init(env); err != nil {
		r.Err = err
		return r
	}
	var sess *neonSession
	var clear func()
	var api neonAPI
	if degraded {
		sess, clear = beginDegradedNeonSession()
	} else {
		sess, clear, err = h.session()
		if err != nil {
			r.Err = err
			return r
		}
		api = newNeonAPI(neonSessionToken(sess))
	}
	defer clear()
	target := handoffTarget{ProjectRoot: h.work, EnvName: env, Env: e}
	var cleanup func()
	var kept []string
	out.WriteString(captureStdout(func() {
		kept, cleanup, err = neonDestroyPrep(h.ctx, os.Stdout, target, terraformOutputRunner, api, sess)
	}))
	if cleanup != nil {
		defer cleanup()
	}
	r.Kept = kept
	if err != nil {
		r.Err = err
		return r
	}
	addrs, err := h.stateList(env)
	if err != nil {
		r.Err = err
		return r
	}
	mods := map[string]bool{}
	for _, a := range addrs {
		if strings.HasPrefix(a, "module.neon_access_") {
			mods[strings.SplitN(a, ".", 3)[0]+"."+strings.SplitN(a, ".", 3)[1]] = true
		}
	}
	if len(mods) > 0 {
		if !degraded {
			if err := sess.EnsureFresh(h.ctx, io.Discard); err != nil {
				r.Err = err
				return r
			}
		}
		args := []string{"destroy", "-auto-approve", "-no-color", "-refresh=false"}
		for m := range mods {
			args = append(args, "-target="+m)
		}
		do, code, err := h.tf(tfConfig, envDir, args...)
		out.WriteString(lastLines(do, 15) + "\n")
		if err == nil && code != 0 {
			if remedy := neonRoleInUseRemedy(do, e2eOwner); remedy != "" {
				out.WriteString(remedy + "\n")
			}
			err = fmt.Errorf("terraform destroy exited %d: %s", code, redactSecrets(lastLines(do, 12)))
		}
		if err != nil {
			r.Err = err
			return r
		}
	} else {
		out.WriteString("(harness) no Neon module left in state after prep; terraform destroy not needed\n")
	}
	out.WriteString(captureStdout(func() { printNeonKeptReport(h.work, env) }))
	return r
}

// planChanges lists "<address> <actions>" for every resource change that is
// not a no-op or read.
func planChanges(js []byte) []string {
	var plan struct {
		ResourceChanges []struct {
			Address string `json:"address"`
			Type    string `json:"type"`
			Change  struct {
				Actions []string `json:"actions"`
			} `json:"change"`
		} `json:"resource_changes"`
	}
	if json.Unmarshal(js, &plan) != nil {
		return []string{"(unreadable plan)"}
	}
	var out []string
	for _, rc := range plan.ResourceChanges {
		acts := strings.Join(rc.Change.Actions, ",")
		if acts == "no-op" || acts == "read" {
			continue
		}
		out = append(out, rc.Address+" "+acts)
	}
	sort.Strings(out)
	return out
}

func anyContains(list []string, sub string) bool {
	for _, s := range list {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// keptEntries is env's Kept file.
func (h *e2eHarness) keptEntries(env string) []NeonKeptEntry {
	e, _ := loadNeonKept(h.work, env)
	return e
}

func keptHas(entries []NeonKeptEntry, kind, name string) bool {
	for _, e := range entries {
		if e.Kind == kind && (e.Database == name || e.Role == name || e.ProjectID == name) {
			return true
		}
	}
	return false
}

// printedCommand finds the printed delete command containing want and returns
// its arguments after the CLI name, with the trailing comment removed.
func printedCommand(output, want string) ([]string, bool) {
	for _, l := range strings.Split(output, "\n") {
		if !strings.Contains(l, want) {
			continue
		}
		if i := strings.Index(l, "#"); i >= 0 {
			l = l[:i]
		}
		f := strings.Fields(l)
		if len(f) > 2 && (f[0] == "neon" || f[0] == "neonctl") {
			return f[1:], true
		}
	}
	return nil, false
}

func (h *e2eHarness) need(ids ...string) error {
	for _, id := range ids {
		if id == "" {
			return errE2EBlocked
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

func (h *e2eHarness) createEnv() e2eEnv {
	return e2eEnv{
		Name: "e2e",
		Neon: &Neon{
			Mode: "create",
			Project: &NeonProject{
				Name: h.projectName("c"), OrgID: h.org, RegionID: h.region, PGVersion: 17,
			},
			OwnerRole: e2eOwner,
			Databases: []NeonDatabase{{Name: "app", Create: true}, {Name: "app2", Create: true}},
		},
		Services: []e2eService{
			{Name: "r", Neon: &NeonAccess{Database: "app", Schemas: []string{"app"}, Access: "readonly"}},
			{Name: "w", Neon: &NeonAccess{Database: "app", Schemas: []string{"app", "public"}, Access: "readwrite"}},
			{Name: "x", Neon: &NeonAccess{Database: "app2", Access: "readonly"}},
		},
	}
}

// e2e is the create env as the steps change it.
var e2eCur e2eEnv

func (h *e2eHarness) svc(e *e2eEnv, name string) *e2eService {
	for i := range e.Services {
		if e.Services[i].Name == name {
			return &e.Services[i]
		}
	}
	return nil
}

func (h *e2eHarness) redeploy(e e2eEnv, o e2eDeployOpts) (e2eDeploy, error) {
	if err := h.generate(e); err != nil {
		return e2eDeploy{}, err
	}
	d := h.deploy(e.Name, o)
	h.orphanFinding(d)
	return d, nil
}

// ---------------------------------------------------------------------------
// Steps
// ---------------------------------------------------------------------------

// Step 0 (U-6): Terraform accepts an ephemeral variable feeding a postgresql
// provider block, and the variable's value stays out of the saved plan.
func (h *e2eHarness) step0() error {
	dir := filepath.Join(h.work, "u6")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	main := `terraform {
  required_version = ">= 1.14.0"
  required_providers {
    postgresql = {
      source  = "cyrilgdn/postgresql"
      version = "1.27.0"
    }
  }
}

variable "neon_pg_credentials" {
  type = map(object({
    host       = string
    user       = string
    password   = string
    pg_version = string
  }))
  default   = {}
  sensitive = true
  ephemeral = true
}

provider "postgresql" {
  alias            = "app"
  host             = try(var.neon_pg_credentials["app"].host, "unresolved.invalid")
  username         = try(var.neon_pg_credentials["app"].user, "unresolved")
  password         = try(var.neon_pg_credentials["app"].password, "")
  database         = "app"
  sslmode          = "require"
  superuser        = false
  expected_version = try(var.neon_pg_credentials["app"].pg_version, "17")
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(main), 0o644); err != nil {
		return err
	}
	marker := "marker-" + h.runID + "-u6-password"
	creds := fmt.Sprintf(`{"app":{"host":"unresolved.invalid","user":"u","password":%q,"pg_version":"17"}}`, marker)
	run := func(args ...string) (string, int) {
		cmd := exec.CommandContext(h.ctx, "terraform", args...)
		cmd.Dir = dir
		cmd.Env = append(childEnv(), "TF_VAR_neon_pg_credentials="+creds)
		out, err := cmd.CombinedOutput()
		code := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else if err != nil {
			code = -1
		}
		h.logf("$ terraform %s (u6) → exit %d", strings.Join(args, " "), code)
		return string(out), code
	}
	if out, code := run("init", "-input=false", "-no-color"); code != 0 {
		return fmt.Errorf("init: %s", lastLines(out, 8))
	}
	out, code := run("plan", "-input=false", "-no-color", "-detailed-exitcode", "-out=u6.tfplan")
	h.check("plan with the ephemeral variable succeeds", code == 0, "exit %d %s", code, lastLines(out, 3))
	found, members := zipContains(filepath.Join(dir, "u6.tfplan"), marker)
	h.check("the variable's value is in no plan-file member", !found && members > 0, "%d members checked", members)
	return nil
}

// zipContains reports whether any member of the zip at path contains s.
func zipContains(path, s string) (bool, int) {
	z, err := zip.OpenReader(path)
	if err != nil {
		data, rerr := os.ReadFile(path)
		return rerr == nil && bytes.Contains(data, []byte(s)), 0
	}
	defer z.Close()
	n := 0
	for _, f := range z.File {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		n++
		if bytes.Contains(data, []byte(s)) {
			return true, n
		}
	}
	return false, n
}

func (h *e2eHarness) step1() error {
	h.check("NEON_API_KEY absent from the test process", os.Getenv("NEON_API_KEY") == "" && neonUserKey == "", "")
	tok, err := resolveNeonToken(h.ctx, h.deps, 0)
	if err != nil {
		var ae *NeonAuthError
		if !errors.As(err, &ae) || (ae.Kind != neonAuthShort && ae.Kind != neonAuthExpired) {
			return err
		}
	}
	h.check("resolveNeonToken source is the CLI login", tok.Source == neonTokenFromNeonctl && tok.Value != "",
		"source=%q, expiry known=%v", tok.Source, !tok.ExpiresAt.IsZero())
	registerSecret(tok.Value)
	if _, err := h.token(); err != nil {
		return err
	}
	// The organization the throwaway projects go in.
	h.org = os.Getenv("MEROKU_NEON_E2E_ORG")
	if h.org == "" {
		orgs, err := h.api().ListOrganizations(h.ctx)
		if err != nil {
			return err
		}
		if len(orgs) != 1 {
			return fmt.Errorf("the login belongs to %d organizations; set MEROKU_NEON_E2E_ORG", len(orgs))
		}
		h.org = orgs[0].ID
	}
	h.note("organization %s", h.org)
	return nil
}

func (h *e2eHarness) step2() error {
	if h.org == "" {
		return errE2EBlocked
	}
	e2eCur = h.createEnv()
	d, err := h.redeploy(e2eCur, e2eDeployOpts{})
	if err != nil {
		return err
	}
	fresh := d.Err == nil
	h.check("first deploy of a fresh env (no state yet) runs", fresh, "%v", errString(d.Err))
	if !fresh && strings.Contains(d.Err.Error(), "state list") {
		h.ev.Findings = append(h.ev.Findings, "Step 2: the first deploy of a new Neon env stops with \"neon: reading Terraform state (state list) failed: terraform state: exit status 1\". readNeonState (app/neon_resolve.go) runs `terraform state list`, which exits 1 with \"No state file was found!\" while the backend holds no state (local backend here; the S3 backend's remote state is nil for a missing object too, so the same applies). prepareNeonForDeploy calls readNeonState before step 1, so nothing is created. Fix direction: treat that exit as an empty state.")
	}
	// The project is created by Terraform: record it as soon as state has it.
	if cs, serr := readNeonState(h.ctx, terraformOutputRunner, h.envDir("e2e")); serr == nil && cs.Created.Project != nil {
		h.p1 = cs.Created.Project.ID
		h.ev.Created[h.p1] = h.projectName("c")
		h.note("create-mode project %s", h.p1)
	}
	if err != nil {
		return err
	}
	h.check("deploy (step 1 + step 2) succeeds", d.Err == nil, "%v", errString(d.Err))
	h.check("step 1 applied the project and databases", d.Step1Applied, "")
	h.check("NoAWS step 2 targets only Neon modules", len(d.Step2Targets) > 0 && !anyContains(d.Step2Targets, "workloads") && !anyContains(d.Step2Targets, "neon_ssm"), "%v", d.Step2Targets)
	if h.p1 == "" {
		return errors.New("no project id in state after the deploy")
	}
	p, err := h.api().GetProject(h.ctx, h.p1)
	if err != nil {
		return err
	}
	h.check("project is PG 17 and named meroku-e2e-*", p.PGVersion == 17 && p.Name == h.projectName("c"), "pg %d, name %s", p.PGVersion, p.Name)
	dbs, err := h.databases(h.p1)
	if err != nil {
		return err
	}
	h.check("databases app and app2 exist", dbs["app"] && dbs["app2"], "%v", e2eKeys(dbs))
	dsn, err := h.ownerDSN(h.p1, "app")
	if err != nil {
		return err
	}
	r := h.sql1("owner@app", dsn, "SELECT pg_get_userbyid(nspowner) FROM pg_namespace WHERE nspname = 'app'")
	h.check("schema app exists, owned by neondb_owner", r.OK && r.first() == e2eOwner, "owner=%q", r.first())
	return d.Err
}

func e2eKeys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (h *e2eHarness) step3() error {
	if err := h.need(h.p1); err != nil {
		return err
	}
	d := h.deploy("e2e", e2eDeployOpts{PlanOnly: true})
	if d.Err != nil {
		return d.Err
	}
	h.check("step-1 plan has no changes", !d.Step1Changes, "")
	h.check("step-2 plan -detailed-exitcode = 0", d.Step2Exit == 0, "exit %d, changes %v", d.Step2Exit, planChanges(d.Step2Plan))
	return nil
}

// urlsE2E are the create env's service URLs and role names.
type e2eURLs struct {
	app, app2         map[string]string
	appRoles, a2Roles map[string]string
}

func (h *e2eHarness) urlsE2E() (e2eURLs, error) {
	var u e2eURLs
	var err error
	if u.app, u.appRoles, err = h.serviceURLs("e2e", "app"); err != nil {
		return u, err
	}
	u.app2, u.a2Roles, err = h.serviceURLs("e2e", "app2")
	if err != nil && strings.Contains(err.Error(), "has no Neon connection") {
		return u, nil
	}
	return u, err
}

func (h *e2eHarness) step4() error {
	if err := h.need(h.p1); err != nil {
		return err
	}
	u, err := h.urlsE2E()
	if err != nil {
		return err
	}
	rURL, wURL, xURL := u.app["r"], u.app["w"], u.app2["x"]
	h.check("URLs use the pooled host, sslmode=require, no options", strings.Contains(rURL, "-pooler.") && strings.Contains(rURL, "sslmode=require") && !strings.Contains(rURL, "options="), "r/w in app, x in app2")
	ownApp, err := h.ownerDSN(h.p1, "app")
	if err != nil {
		return err
	}
	ownApp2, err := h.ownerDSN(h.p1, "app2")
	if err != nil {
		return err
	}

	// Migration as the owner, after the deploy: default privileges must cover it.
	conn, rs := h.sql("owner@app", ownApp,
		"CREATE TABLE app.t (id serial PRIMARY KEY, v text)",
		"CREATE TABLE public.p (id serial PRIMARY KEY, v text)",
		"INSERT INTO app.t (v) VALUES ('one'), ('two')",
		"INSERT INTO public.p (v) VALUES ('one')")
	ok := conn.OK
	for _, r := range rs {
		ok = ok && r.OK
	}
	if !h.check("migration as the owner creates app.t and public.p", ok, "") {
		return errors.New("migration failed")
	}

	// r: readonly on [app].
	conn, rs = h.sql("r@app", rURL,
		"SELECT count(*) FROM app.t",
		"INSERT INTO app.t (v) VALUES ('r')",
		"CREATE TABLE app.r_new (i int)",
		"CREATE TABLE public.r_new (i int)")
	if conn.OK && len(rs) == 4 {
		h.check("r SELECT app.t (created after deploy) ok", rs[0].OK, "%s", rs[0].first())
		h.check("r INSERT app.t denied", denied(rs[1]), "%s %s", rs[1].Code, rs[1].Error)
		h.check("r CREATE TABLE in app denied", denied(rs[2]), "%s %s", rs[2].Code, rs[2].Error)
		h.check("r CREATE TABLE in public denied", denied(rs[3]), "%s %s", rs[3].Code, rs[3].Error)
	} else {
		h.check("r connects", false, "%s %s", conn.Code, conn.Error)
	}

	// w: readwrite on [app, public].
	conn, rs = h.sql("w@app", wURL,
		"INSERT INTO app.t (v) VALUES ('w')",
		"UPDATE app.t SET v = 'w2' WHERE v = 'w'",
		"DELETE FROM app.t WHERE v = 'w2'",
		"INSERT INTO public.p (v) VALUES ('w')",
		"UPDATE public.p SET v = 'w2' WHERE v = 'w'",
		"DELETE FROM public.p WHERE v = 'w2'",
		"SELECT nextval('app.t_id_seq')",
		"CREATE TABLE app.w_new (i int)",
		"ALTER TABLE app.t ADD COLUMN z int",
		"DROP TABLE app.t")
	if conn.OK && len(rs) == 10 {
		dml := true
		for _, r := range rs[:6] {
			dml = dml && r.OK
		}
		h.check("w INSERT/UPDATE/DELETE on app.t and public.p ok", dml, "")
		h.check("w nextval ok", rs[6].OK, "%s", rs[6].first())
		h.check("w CREATE TABLE denied", denied(rs[7]), "%s %s", rs[7].Code, rs[7].Error)
		h.check("w ALTER TABLE denied", denied(rs[8]), "%s %s", rs[8].Code, rs[8].Error)
		h.check("w DROP TABLE denied", denied(rs[9]), "%s %s", rs[9].Code, rs[9].Error)
	} else {
		h.check("w connects", false, "%s %s", conn.Code, conn.Error)
	}

	// x: readonly in app2; cannot reach app.t (its database refuses it).
	xa := h.sql1("x@app", withDB(xURL, "app"), "SELECT count(*) FROM app.t")
	h.check("x SELECT app.t denied (x is refused by database app)", !xa.OK && xa.Code == "42501", "%s %s", xa.Code, xa.Error)
	xo := h.sql1("x@app2", xURL, "SELECT 1")
	h.check("x connects to its own database app2", xo.OK, "%s %s", xo.Code, xo.Error)

	// No CREATE on public for any service role (R2-15).
	_, rs = h.sql("owner@app", ownApp,
		fmt.Sprintf("SELECT has_schema_privilege('%s', 'public', 'CREATE')", u.appRoles["r"]),
		fmt.Sprintf("SELECT has_schema_privilege('%s', 'public', 'CREATE')", u.appRoles["w"]))
	_, rs2 := h.sql("owner@app2", ownApp2, fmt.Sprintf("SELECT has_schema_privilege('%s', 'public', 'CREATE')", u.a2Roles["x"]))
	all := append(rs, rs2...)
	noCreate := len(all) == 3
	for _, r := range all {
		noCreate = noCreate && r.OK && r.first() == "false"
	}
	h.check("has_schema_privilege(public, CREATE) false for r, w, x", noCreate, "")

	// search_path through the pooler, two transactions [U-14, U-17].
	_, rs = h.sql("r@app pooled", rURL, "BEGIN", "SHOW search_path", "COMMIT", "BEGIN", "SHOW search_path", "COMMIT",
		"SELECT current_schema()", "SELECT count(*) FROM t")
	if len(rs) == 8 {
		h.check("r search_path = app in two pooled transactions", rs[1].first() == "app" && rs[4].first() == "app", "%q, %q", rs[1].first(), rs[4].first())
		h.check("unqualified SELECT resolves to app.t", rs[6].first() == "app" && rs[7].OK, "current_schema=%q", rs[6].first())
	} else {
		h.check("r search_path queries ran", false, "")
	}
	_, rs = h.sql("w@app pooled", wURL, "BEGIN", "SHOW search_path", "COMMIT", "BEGIN", "SHOW search_path", "COMMIT")
	if len(rs) == 6 {
		h.check("w search_path = app, public in two pooled transactions", rs[1].first() == "app, public" && rs[4].first() == "app, public", "%q, %q", rs[1].first(), rs[4].first())
	}

	// PUBLIC on created databases (R6-24).
	rt := h.sql1("r@app", rURL, "CREATE TEMP TABLE tt (i int)")
	wt := h.sql1("w@app", wURL, "CREATE TEMP TABLE tt (i int)")
	h.check("r CREATE TEMP TABLE denied", denied(rt), "%s %s", rt.Code, rt.Error)
	h.check("w CREATE TEMP TABLE denied", denied(wt), "%s %s", wt.Code, wt.Error)
	ra2 := h.sql1("r@app2", withDB(rURL, "app2"), "SELECT 1")
	h.check("r connecting to app2 is refused", !ra2.OK && ra2.Code == "42501", "%s %s", ra2.Code, ra2.Error)
	o1 := h.sql1("owner@app", ownApp, "CREATE TEMP TABLE tt (i int)")
	o2 := h.sql1("owner@app2", ownApp2, "CREATE TEMP TABLE tt (i int)")
	h.check("owner connects to app and app2 and creates a temp table in each", o1.OK && o2.OK, "%s %s / %s %s", o1.Code, o1.Error, o2.Code, o2.Error)
	return nil
}

func (h *e2eHarness) step5() error {
	if err := h.need(h.p1); err != nil {
		return err
	}
	// 5a: re-plan after tables exist; the step-1 plan has no changes (Issue 38).
	d := h.deploy("e2e", e2eDeployOpts{PlanOnly: true})
	if d.Err != nil {
		return d.Err
	}
	h.check("re-plan after tables exist: step-2 exit 0", d.Step2Exit == 0, "exit %d %v", d.Step2Exit, planChanges(d.Step2Plan))
	h.check("second step-1 plan: no changes (Issue 38)", !d.Step1Changes, "")

	before, err := h.urlsE2E()
	if err != nil {
		return err
	}

	// 5b: r → readwrite: grants only, same role name, r can INSERT.
	h.svc(&e2eCur, "r").Neon.Access = "readwrite"
	d, err = h.redeploy(e2eCur, e2eDeployOpts{})
	if err != nil {
		return err
	}
	if d.Err != nil {
		h.check("deploy r → readwrite", false, "%v", d.Err)
	} else {
		ch := planChanges(d.Step2Plan)
		grantsOnly := len(ch) > 0
		for _, c := range ch {
			if !strings.Contains(c, "postgresql_grant.") && !strings.Contains(c, "postgresql_default_privileges.") {
				grantsOnly = false
			}
		}
		h.check("r → readwrite plans grant changes only", grantsOnly, "%v", ch)
		after, err := h.urlsE2E()
		if err != nil {
			return err
		}
		h.check("r keeps its role name", after.appRoles["r"] == before.appRoles["r"], "%s", after.appRoles["r"])
		ins := h.sql1("r@app", after.app["r"], "INSERT INTO app.t (v) VALUES ('r-rw')")
		h.check("r can INSERT as readwrite", ins.OK, "%s %s", ins.Code, ins.Error)
	}
	h.svc(&e2eCur, "r").Neon.Access = "readonly"
	d, err = h.redeploy(e2eCur, e2eDeployOpts{})
	if err != nil {
		return err
	}
	h.check("r changed back to readonly", d.Err == nil, "%v", errString(d.Err))
	if cur, err := h.urlsE2E(); err == nil {
		ins := h.sql1("r@app", cur.app["r"], "INSERT INTO app.t (v) VALUES ('r-ro')")
		h.check("r INSERT denied again", denied(ins), "%s %s", ins.Code, ins.Error)
	}

	// 5c: w's schemas → [public] (R6-2).
	h.svc(&e2eCur, "w").Neon.Schemas = []string{"public"}
	d, err = h.redeploy(e2eCur, e2eDeployOpts{})
	if err != nil {
		return err
	}
	if h.check("deploy w schemas → [public]", d.Err == nil, "%v", errString(d.Err)) {
		cur, err := h.urlsE2E()
		if err != nil {
			return err
		}
		h.note("the search_path notice is printed by meroku's apply screen after a step-2 apply; this harness applies step 2 itself, so the notice is not checked here")
		dr := h.sql1("w@app direct", direct(cur.app["w"]), "SHOW search_path")
		h.check("w SHOW search_path = public (direct host)", dr.first() == "public", "%q", dr.first())
		pr := h.sql1("w@app pooled", cur.app["w"], "SHOW search_path")
		if pr.first() == "public" {
			h.check("w SHOW search_path = public (pooled host)", true, "%q", pr.first())
		} else {
			h.note("pooled host still shows %q: the documented pooler lag (U-21, NEON.md); not a failure", pr.first())
		}
		sch := h.sql1("owner@app", mustDSN(h.ownerDSN(h.p1, "app")), "SELECT count(*) FROM pg_namespace WHERE nspname = 'app'")
		h.check("schema app still exists (r still lists it)", sch.first() == "1", "%s", sch.first())
	}

	// 5d: bump r's password_version (R6-8).
	old, err := h.urlsE2E()
	if err != nil {
		return err
	}
	h.svc(&e2eCur, "r").Neon.PasswordVersion = 2
	d, err = h.redeploy(e2eCur, e2eDeployOpts{})
	if err != nil {
		return err
	}
	if h.check("deploy r password_version 2", d.Err == nil, "%v", errString(d.Err)) {
		cur, err := h.urlsE2E()
		if err != nil {
			return err
		}
		h.check("r's URL changed", cur.app["r"] != old.app["r"], "")
		h.check("r keeps its role name", cur.appRoles["r"] == old.appRoles["r"], "%s", cur.appRoles["r"])
		of := h.sql1("r@app old URL", direct(old.app["r"]), "SELECT 1")
		h.check("old URL fails", !of.OK && of.Code == "28P01", "%s %s", of.Code, of.Error)
		nw := h.sql1("r@app new URL", cur.app["r"], "SELECT 1")
		h.check("new URL works", nw.OK, "%s %s", nw.Code, nw.Error)
	}
	return nil
}

func mustDSN(s string, err error) string {
	if err != nil {
		return "postgresql://invalid"
	}
	return s
}

func (h *e2eHarness) reuseEnv(name, project string, pg int, dbs []NeonDatabase, svcs []e2eService) e2eEnv {
	return e2eEnv{
		Name:     name,
		Neon:     &Neon{Mode: "reuse", ProjectID: project, PGVersion: pg, OwnerRole: e2eOwner, Databases: dbs},
		Services: svcs,
	}
}

var e2eReuse e2eEnv

func (h *e2eHarness) step6() error {
	if h.org == "" {
		return errE2EBlocked
	}
	id, err := h.createProjectCLI("r")
	if err != nil {
		return err
	}
	h.p2 = id
	h.simplified("The reuse project is created on Postgres 18, the Neon CLI default, so the reuse env runs with pg_version 18.")
	p, err := h.api().GetProject(h.ctx, h.p2)
	if err != nil {
		return err
	}
	own, err := h.ownerDSN(h.p2, "neondb")
	if err != nil {
		return err
	}
	conn, rs := h.sql("owner@neondb", own, "CREATE TABLE legacy (id int, v text)", "INSERT INTO legacy VALUES (1, 'old')")
	if !conn.OK || len(rs) != 2 || !rs[0].OK || !rs[1].OK {
		return errors.New("creating table legacy failed")
	}

	e2eReuse = h.reuseEnv("reuse", h.p2, p.PGVersion,
		[]NeonDatabase{{Name: "neondb", Create: false}},
		[]e2eService{{Name: "rd", Neon: &NeonAccess{Database: "neondb", Schemas: []string{"public", "reuse"}, Access: "readonly"}}})

	pw, err := h.ownerPassword(h.p2)
	if err != nil {
		return err
	}
	var planHasPw, planHasURI bool
	var members int
	d, err := h.redeploy(e2eReuse, e2eDeployOpts{OnStep2Plan: func(path string, _ []byte) {
		planHasPw, members = zipContains(path, pw)
		planHasURI, _ = zipContains(path, "postgresql://"+e2eOwner)
	}})
	if err != nil {
		return err
	}
	if !h.check("reuse deploy succeeds", d.Err == nil, "%v", errString(d.Err)) {
		return nil
	}
	sch := h.sql1("owner@neondb", own, "SELECT pg_get_userbyid(nspowner) FROM pg_namespace WHERE nspname = 'reuse'")
	h.check("schema reuse created, owned by neondb_owner", sch.first() == e2eOwner, "owner=%q", sch.first())
	urls, _, err := h.serviceURLs("reuse", "neondb")
	if err != nil {
		return err
	}
	rd := h.sql1("rd@neondb", urls["rd"], "SELECT v FROM public.legacy")
	h.check("readonly service reads legacy", rd.OK && rd.first() == "old", "%s %s", rd.Code, rd.Error)
	state, err := h.tfOK(tfState, h.envDir("reuse"), "state", "pull")
	if err != nil {
		return err
	}
	h.check("state holds no owner password", !strings.Contains(state, pw), "")
	h.check("state holds no postgresql://neondb_owner URI", !strings.Contains(state, "postgresql://"+e2eOwner), "")
	h.check("step-2 plan file holds no owner password", !planHasPw && members > 0, "%d members", members)
	h.check("step-2 plan file holds no postgresql://neondb_owner URI", !planHasURI, "")
	h.simplified("The plan-file check reads the harness's own step-2 plan file (meroku's TUI plan is replaced by a plain terraform plan with the same targets and the same session env).")
	tt := h.sql1("rd@neondb direct", direct(urls["rd"]), "CREATE TEMP TABLE tt (i int)")
	h.check("rd can create a temp table in the reused database (PUBLIC untouched)", tt.OK, "%s %s", tt.Code, tt.Error)
	return nil
}

func (h *e2eHarness) step7() error {
	if err := h.need(h.p1); err != nil {
		return err
	}
	stage := h.reuseEnv("stage", h.p1, 17,
		[]NeonDatabase{{Name: "stage_app", Create: true}},
		[]e2eService{{Name: "w", Neon: &NeonAccess{Database: "stage_app", Access: "readwrite"}}})
	d, err := h.redeploy(stage, e2eDeployOpts{})
	if err != nil {
		return err
	}
	if !h.check("stage deploy succeeds", d.Err == nil, "%v", errString(d.Err)) {
		return nil
	}
	sURLs, sRoles, err := h.serviceURLs("stage", "stage_app")
	if err != nil {
		return err
	}
	cur, err := h.urlsE2E()
	if err != nil {
		return err
	}
	h.check("role names differ between envs", sRoles["w"] != "" && sRoles["w"] != cur.appRoles["w"], "stage %s, e2e %s", sRoles["w"], cur.appRoles["w"])
	s1 := h.sql1("stage w", sURLs["w"], "SELECT 1")
	e1 := h.sql1("e2e w", cur.app["w"], "SELECT 1")
	h.check("both w roles work", s1.OK && e1.OK, "")

	ds := h.destroy("stage", false)
	if !h.check("stage destroy succeeds", ds.Err == nil, "%v", errString(ds.Err)) {
		return nil
	}
	rs, err := h.roles(h.p1)
	if err != nil {
		return err
	}
	h.check("stage's role is gone", sRoles["w"] != "" && !rs[sRoles["w"]], "")
	r2 := h.sql1("e2e r", cur.app["r"], "SELECT count(*) FROM app.t")
	w2 := h.sql1("e2e w", cur.app["w"], "SELECT 1")
	h.check("e2e's roles still work", r2.OK && w2.OK, "")
	dbs, err := h.databases(h.p1)
	if err != nil {
		return err
	}
	h.check("stage_app kept", dbs["stage_app"], "")
	h.check("stage_app printed in the Kept report", strings.Contains(ds.Output, "databases delete stage_app"), "")
	return nil
}

func (h *e2eHarness) step8() error {
	if err := h.need(h.p1); err != nil {
		return err
	}
	before, err := h.urlsE2E()
	if err != nil {
		return err
	}
	oldRole := before.a2Roles["x"]
	h.svc(&e2eCur, "x").Neon = &NeonAccess{Database: "app", Schemas: []string{"app"}, Access: "readonly"}
	d, err := h.redeploy(e2eCur, e2eDeployOpts{})
	if err != nil {
		return err
	}
	if !h.check("move deploy succeeds", d.Err == nil, "%v", errString(d.Err)) {
		return nil
	}
	rs, err := h.roles(h.p1)
	if err != nil {
		return err
	}
	h.check("old role (app2) gone", oldRole != "" && !rs[oldRole], "%s", oldRole)
	after, err := h.urlsE2E()
	if err != nil {
		return err
	}
	_, inApp2 := after.app2["x"]
	h.check("x's URL is under the same key, now in the app connection", after.app["x"] != "" && !inApp2, "")
	u, _ := url.Parse(after.app["x"])
	h.check("x's URL value changed to database app", u != nil && u.Path == "/app" && after.app["x"] != before.app2["x"], "")
	xs := h.sql1("x@app", after.app["x"], "SELECT count(*) FROM app.t")
	h.check("x reads app.t after the move", xs.OK, "%s %s", xs.Code, xs.Error)
	h.simplified("SSM is stubbed: the key check is on the merged neon_ssm urls input (service name), not on an SSM parameter; the task-definition redeploy is §7.2's.")
	return nil
}

func (h *e2eHarness) step9() error {
	if err := h.need(h.p1); err != nil {
		return err
	}
	before, err := h.urlsE2E()
	if err != nil {
		return err
	}
	role := before.appRoles["x"]
	h.svc(&e2eCur, "x").Neon = nil
	retiredDuring := false
	d, err := h.redeploy(e2eCur, e2eDeployOpts{OnStep2Plan: func(string, []byte) {
		_, err := os.Stat(filepath.Join(h.envDir("e2e"), neonRetiredFile))
		retiredDuring = err == nil
	}})
	if err != nil {
		return err
	}
	if !h.check("retire deploy succeeds", d.Err == nil, "%v", errString(d.Err)) {
		return nil
	}
	rs, err := h.roles(h.p1)
	if err != nil {
		return err
	}
	h.check("x's role dropped", role != "" && !rs[role], "%s", role)
	h.note("neon_retired.tf during step 2: %v (x's database app keeps r and w, so its connection stays in main.tf and the role is dropped by the module itself)", retiredDuring)
	_, statErr := os.Stat(filepath.Join(h.envDir("e2e"), neonRetiredFile))
	h.check("no neon_retired.tf after the deploy", os.IsNotExist(statErr), "")
	if retiredDuring {
		h.check("the retired path was used", true, "")
	} else {
		h.simplified("The retired-file path for a connection leaving the render is exercised in 9b (app2) and 9e, not by x, whose database keeps other services.")
	}
	return nil
}

func (h *e2eHarness) step9b() error {
	if err := h.need(h.p1); err != nil {
		return err
	}
	var dbs []NeonDatabase
	for _, db := range e2eCur.Neon.Databases {
		if db.Name != "app2" {
			dbs = append(dbs, db)
		}
	}
	e2eCur.Neon.Databases = dbs
	retiredDuring := false
	d, err := h.redeploy(e2eCur, e2eDeployOpts{OnStep2Plan: func(string, []byte) {
		_, err := os.Stat(filepath.Join(h.envDir("e2e"), neonRetiredFile))
		retiredDuring = err == nil
	}})
	if err != nil {
		return err
	}
	if !h.check("deploy without app2 succeeds", d.Err == nil, "%v", errString(d.Err)) {
		return nil
	}
	ch := append(planChanges(d.Step1Plan), planChanges(d.Step2Plan)...)
	h.check("no database deletion planned", !anyContains(ch, "neon_database"), "%v", ch)
	h.note("app2's connection retired through neon_retired.tf during step 2: %v", retiredDuring)
	h.check("neon databases delete app2 printed", strings.Contains(d.Output, "databases delete app2"), "")
	dbset, err := h.databases(h.p1)
	if err != nil {
		return err
	}
	h.check("app2 still exists", dbset["app2"], "")
	args, ok := printedCommand(d.Output, "databases delete app2")
	if !h.check("printed command names this project", ok && strings.Contains(strings.Join(args, " "), h.p1), "%v", args) {
		return nil
	}
	if _, err := h.cli(args...); err != nil {
		h.check("harness runs the printed command", false, "%v", err)
		return nil
	}
	dbset, err = h.databases(h.p1)
	if err != nil {
		return err
	}
	h.check("harness ran the printed command: app2 deleted", !dbset["app2"], "")
	return nil
}

func (h *e2eHarness) step9c() error {
	if err := h.need(h.p1); err != nil {
		return err
	}
	// Setup: app3 with service y, app4 without services.
	e2eCur.Neon.Databases = append(e2eCur.Neon.Databases, NeonDatabase{Name: "app3", Create: true}, NeonDatabase{Name: "app4", Create: true})
	e2eCur.Services = append(e2eCur.Services, e2eService{Name: "y", Neon: &NeonAccess{Database: "app3", Access: "readonly"}})
	d, err := h.redeploy(e2eCur, e2eDeployOpts{})
	if err != nil {
		return err
	}
	if !h.check("setup deploy (app3 + y, app4) succeeds", d.Err == nil, "%v", errString(d.Err)) {
		return nil
	}
	_, yRoles, err := h.serviceURLs("e2e", "app3")
	if err != nil {
		return err
	}
	yRole := yRoles["y"]
	br, err := h.defaultBranch(h.p1)
	if err != nil {
		return err
	}
	if _, err := h.cli("databases", "delete", "app3", "--project-id", h.p1, "--branch", br); err != nil {
		return err
	}
	// Remove y and app3's entry.
	var dbs []NeonDatabase
	for _, db := range e2eCur.Neon.Databases {
		if db.Name != "app3" {
			dbs = append(dbs, db)
		}
	}
	e2eCur.Neon.Databases = dbs
	var svcs []e2eService
	for _, s := range e2eCur.Services {
		if s.Name != "y" {
			svcs = append(svcs, s)
		}
	}
	e2eCur.Services = svcs
	d, err = h.redeploy(e2eCur, e2eDeployOpts{})
	if err != nil {
		return err
	}
	if !h.check("deploy after the database was dropped succeeds", d.Err == nil, "%v", errString(d.Err)) {
		return nil
	}
	h.check("Kept file records y's role", yRole != "" && keptHas(h.keptEntries("e2e"), neonKeptRole, yRole), "%s", yRole)
	args, ok := printedCommand(d.Output, "roles delete "+yRole)
	ok = ok && yRole != ""
	if !h.check("neon roles delete printed", ok && strings.Contains(strings.Join(args, " "), h.p1), "%v", args) {
		return nil
	}
	_, cerr := h.cli(args...)
	h.check("running the printed roles delete works [U-22]", cerr == nil, "%v", errString(cerr))
	rs, err := h.roles(h.p1)
	if err != nil {
		return err
	}
	h.check("y's role is gone from Neon", !rs[yRole], "")
	h.note("the prune of y's entry is checked by the next run (step 9d)")
	h.yRole = yRole
	return nil
}

func (h *e2eHarness) step9d() error {
	if err := h.need(h.p1); err != nil {
		return err
	}
	// Remove app4 and add app5, so step 1 has a plan to decline.
	var dbs []NeonDatabase
	for _, db := range e2eCur.Neon.Databases {
		if db.Name != "app4" {
			dbs = append(dbs, db)
		}
	}
	e2eCur.Neon.Databases = append(dbs, NeonDatabase{Name: "app5", Create: true})
	d, err := h.redeploy(e2eCur, e2eDeployOpts{DeclineStep1: true})
	if err != nil {
		return err
	}
	if h.yRole != "" {
		h.check("9c: the next run pruned y's role from the Kept file", !keptHas(h.keptEntries("e2e"), neonKeptRole, h.yRole), "")
	}
	h.check("declined step 1 stops the deploy", errors.Is(d.Err, errNeonStopped) && d.Step1Changes, "%v", errString(d.Err))
	h.check("Kept file records app4", keptHas(h.keptEntries("e2e"), neonKeptDatabase, "app4"), "")
	if fi, err := os.Stat(neonKeptPath(h.work, "e2e")); err == nil {
		h.check("Kept file is 0600", fi.Mode().Perm() == 0o600, "%v", fi.Mode().Perm())
	} else {
		h.check("Kept file exists", false, "%v", err)
	}
	d = h.deploy("e2e", e2eDeployOpts{})
	if !h.check("next deploy succeeds", d.Err == nil, "%v", errString(d.Err)) {
		return nil
	}
	h.check("next deploy prints app4 from the Kept file", strings.Contains(d.Output, "databases delete app4"), "")
	br, err := h.defaultBranch(h.p1)
	if err != nil {
		return err
	}
	if _, err := h.cli("databases", "delete", "app4", "--project-id", h.p1, "--branch", br); err != nil {
		return err
	}
	d = h.deploy("e2e", e2eDeployOpts{})
	if !h.check("deploy after deleting app4 succeeds", d.Err == nil, "%v", errString(d.Err)) {
		return nil
	}
	h.check("app4's entry pruned from the Kept file", !keptHas(h.keptEntries("e2e"), neonKeptDatabase, "app4"), "")
	h.simplified("Declining step 1 is simulated by the harness's step-1 runner returning not-applied for a plan with changes (the TUI is not driven).")
	return nil
}

func (h *e2eHarness) step9e() error {
	if h.org == "" {
		return errE2EBlocked
	}
	small := e2eEnv{
		Name: "small",
		Neon: &Neon{
			Mode:      "create",
			Project:   &NeonProject{Name: h.projectName("s"), OrgID: h.org, RegionID: h.region, PGVersion: 17},
			OwnerRole: e2eOwner,
			Databases: []NeonDatabase{{Name: "sapp", Create: true}},
		},
		Services: []e2eService{{Name: "s1", Neon: &NeonAccess{Database: "sapp", Access: "readonly"}}},
	}
	d, err := h.redeploy(small, e2eDeployOpts{})
	if st, serr := readNeonState(h.ctx, terraformOutputRunner, h.envDir("small")); serr == nil && st.Created.Project != nil {
		h.p3 = st.Created.Project.ID
		h.ev.Created[h.p3] = h.projectName("s")
		h.note("small project %s", h.p3)
	}
	if err != nil {
		return err
	}
	if !h.check("small env deploy succeeds", d.Err == nil, "%v", errString(d.Err)) {
		return nil
	}
	_, roles, err := h.serviceURLs("small", "sapp")
	if err != nil {
		return err
	}
	role := roles["s1"]

	small.Neon = nil
	small.Services[0].Neon = nil
	var overrideOK, retiredOK bool
	d, err = h.redeploy(small, e2eDeployOpts{OnStep2Plan: func(string, []byte) {
		dir := h.envDir("small")
		ov, oerr := os.ReadFile(filepath.Join(dir, neonRetiredOverrideFile))
		overrideOK = oerr == nil && strings.Contains(string(ov), "cyrilgdn/postgresql")
		_, rerr := os.Stat(filepath.Join(dir, neonRetiredFile))
		retiredOK = rerr == nil
	}})
	if err != nil {
		return err
	}
	if !h.check("deploy with neon: removed succeeds", d.Err == nil, "%v", errString(d.Err)) {
		return nil
	}
	h.check("override file with the postgresql requirement written", overrideOK, "")
	h.check("neon_retired.tf written", retiredOK, "")
	rs, err := h.roles(h.p3)
	if err != nil {
		return err
	}
	h.check("role dropped (absent in ListRoles)", role != "" && !rs[role], "%s", role)
	_, perr := h.api().GetProject(h.ctx, h.p3)
	h.check("project kept", perr == nil, "%v", errString(perr))
	h.check("project printed in the Kept report", strings.Contains(d.Output, "projects delete "+h.p3), "")
	_, e1 := os.Stat(filepath.Join(h.envDir("small"), neonRetiredFile))
	_, e2 := os.Stat(filepath.Join(h.envDir("small"), neonRetiredOverrideFile))
	h.check("both deploy-time files gone after the run", os.IsNotExist(e1) && os.IsNotExist(e2), "")
	addrs, err := h.stateList("small")
	if err != nil {
		return err
	}
	h.check("state holds no Neon address", !anyContains(addrs, "module.neon.") && !anyContains(addrs, "module.neon_access_"), "%v", addrs)
	return nil
}

func (h *e2eHarness) step10() error {
	if err := h.need(h.p2); err != nil {
		return err
	}
	urls, roles, err := h.serviceURLs("reuse", "neondb")
	_ = urls
	if err != nil {
		return err
	}
	ds := h.destroy("reuse", false)
	h.check("reuse destroy succeeds", ds.Err == nil, "%v", errString(ds.Err))
	h.check("no \"objects depend on it\" [U-23]", !strings.Contains(ds.Output, "depend on it"), "")
	rs, err := h.roles(h.p2)
	if err != nil {
		return err
	}
	h.check("rd's role dropped", roles["rd"] != "" && !rs[roles["rd"]], "%s", roles["rd"])
	_, perr := h.api().GetProject(h.ctx, h.p2)
	h.check("project remains", perr == nil, "%v", errString(perr))
	own, err := h.ownerDSN(h.p2, "neondb")
	if err != nil {
		return err
	}
	_, res := h.sql("owner@neondb", own,
		"SELECT pg_get_userbyid(nspowner) FROM pg_namespace WHERE nspname = 'reuse'",
		"SELECT count(*) FROM public.legacy")
	if len(res) == 2 {
		h.check("database and schema reuse remain, owner unchanged", res[0].first() == e2eOwner, "owner=%q", res[0].first())
		h.check("table legacy and its rows remain", res[1].first() == "1", "rows=%s", res[1].first())
	}
	return nil
}

func (h *e2eHarness) step11() error {
	if err := h.need(h.p1, h.p2); err != nil {
		return err
	}
	// (0) A hand-run destroy plan of module.neon is refused by prevent_destroy.
	sess, clear, err := h.session()
	if err != nil {
		return err
	}
	out, code, err := h.tf(tfConfig, h.envDir("e2e"), "plan", "-destroy", "-refresh=false", "-no-color", "-input=false", "-target=module.neon")
	clear()
	_ = sess
	if err != nil {
		return err
	}
	h.check("(0) plan -destroy -refresh=false fails: Instance cannot be destroyed", code != 0 && strings.Contains(out, "Instance cannot be destroyed"), "exit %d", code)
	h.simplified("(0) targets module.neon: an untargeted hand-run plan would also plan the AWS modules, which this test may not touch.")

	// (a) pg_version change ⇒ the neon: refusal, nothing changed.
	e2eCur.Neon.Project.PGVersion = 16
	d, err := h.redeploy(e2eCur, e2eDeployOpts{})
	e2eCur.Neon.Project.PGVersion = 17
	if err != nil {
		h.note("(a) generate refused the change: %v", err)
		h.check("(a) pg_version change refused", strings.Contains(err.Error(), "pg_version"), "%v", err)
	} else {
		refused := d.Err != nil && (d.Step1Refusal != "" || strings.Contains(d.Err.Error(), "pg_version"))
		h.check("(a) pg_version change refused", refused, "err=%v refusal=%q", errString(d.Err), firstLine(d.Step1Refusal))
		h.check("(a) nothing applied", !d.Step1Applied, "")
	}
	if p, err := h.api().GetProject(h.ctx, h.p1); err == nil {
		h.check("(a) project still PG 17", p.PGVersion == 17, "%d", p.PGVersion)
	}
	if err := h.generate(e2eCur); err != nil {
		return err
	}

	// (b) Degraded destroy of a small reuse env.
	deg := h.reuseEnv("deg", h.p2, 0, []NeonDatabase{{Name: "neondb", Create: false}},
		[]e2eService{{Name: "d1", Neon: &NeonAccess{Database: "neondb", Access: "readonly"}}})
	if p, err := h.api().GetProject(h.ctx, h.p2); err == nil {
		deg.Neon.PGVersion = p.PGVersion
	}
	d, err = h.redeploy(deg, e2eDeployOpts{})
	if err != nil {
		return err
	}
	if h.check("(b) deg env deploy succeeds", d.Err == nil, "%v", errString(d.Err)) {
		_, roles, err := h.serviceURLs("deg", "neondb")
		if err != nil {
			return err
		}
		role := roles["d1"]
		h.check("(b) [2] not offered while neon: is present (main.tf has module \"neon\")", mainTFHasNeonModule(h.envDir("deg")), "")
		deg.Neon = nil
		deg.Services[0].Neon = nil
		if err := h.generate(deg); err != nil {
			return err
		}
		h.check("(b) after removing neon: and regenerating, [2] is offered", !mainTFHasNeonModule(h.envDir("deg")), "")
		ds := h.destroy("deg", true)
		h.check("(b) degraded destroy completes without a login", ds.Err == nil, "%v", errString(ds.Err))
		h.check("(b) the role left behind is in the Kept file", role != "" && keptHas(h.keptEntries("deg"), neonKeptRole, role), "%s", role)
		rs, err := h.roles(h.p2)
		if err == nil {
			h.check("(b) the role is still in Neon", rs[role], "")
		}
		h.simplified("(b) The [1]/[2]/[3] prompt is not driven; its condition (mainTFHasNeonModule) is checked, and the degraded session is started directly.")
	}

	// (c) Destroy the create env.
	before, err := h.urlsE2E()
	if err != nil {
		return err
	}
	ds := h.destroy("e2e", false)
	if !h.check("(c) create env destroy succeeds", ds.Err == nil, "%v", errString(ds.Err)) {
		return nil
	}
	rs, err := h.roles(h.p1)
	if err != nil {
		return err
	}
	gone := len(before.appRoles) > 0
	for _, r := range before.appRoles {
		gone = gone && !rs[r]
	}
	h.check("(c) service roles gone", gone, "%v", before.appRoles)
	_, perr := h.api().GetProject(h.ctx, h.p1)
	h.check("(c) project kept", perr == nil, "%v", errString(perr))
	own, err := h.ownerDSN(h.p1, "app")
	if err != nil {
		return err
	}
	rows := h.sql1("owner@app", own, "SELECT count(*) FROM app.t")
	h.check("(c) databases kept with rows", rows.OK && rows.first() != "0", "rows=%s", rows.first())
	var cmds []string
	for _, l := range ds.Kept {
		t := strings.TrimSpace(l)
		if (strings.HasPrefix(t, "neon ") || strings.HasPrefix(t, "neonctl ")) && strings.Contains(t, " delete ") {
			cmds = append(cmds, t)
		}
	}
	h.check("(c) report has one line: projects delete <id>", len(cmds) == 1 && strings.Contains(cmds[0], "projects delete "+h.p1), "%v", cmds)
	args, ok := printedCommand(strings.Join(ds.Kept, "\n"), "projects delete "+h.p1)
	if !ok {
		return nil
	}
	if _, err := h.cli(args...); err != nil {
		h.check("(c) harness runs the printed projects delete", false, "%v", err)
		return nil
	}
	h.ev.Deleted[h.p1] = h.ev.Created[h.p1]
	_, gerr := h.api().GetProject(h.ctx, h.p1)
	h.check("(c) harness ran the printed command: project gone (404)", errors.Is(gerr, ErrNeonNotFound), "%v", errString(gerr))
	return nil
}

func firstLine(s string) string {
	return strings.SplitN(s, "\n", 2)[0]
}

// ---------------------------------------------------------------------------
// Cleanup and evidence
// ---------------------------------------------------------------------------

var e2eIDRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// finish deletes every project this run created, confirms each is gone, and
// writes the evidence.
func (h *e2eHarness) finish() {
	h.logf("===== cleanup")
	clean := func(name string, ok bool, detail string, args ...interface{}) {
		d := redactSecrets(fmt.Sprintf(detail, args...))
		h.ev.Cleanup = append(h.ev.Cleanup, e2eCheck{Name: name, OK: ok, Detail: d})
		h.logf("  [%v] %s — %s", ok, name, d)
	}
	prefix := e2eNamePrefix + h.runID + "-"

	// Projects Terraform created but the run did not record: find by this run's
	// name prefix only.
	if h.org != "" {
		if ps, err := h.api().ListProjects(h.ctx, h.org); err == nil {
			for _, p := range ps {
				if strings.HasPrefix(p.Name, prefix) {
					h.ev.Created[p.ID] = p.Name
				}
			}
		} else {
			clean("list projects before cleanup", false, "%v", err)
		}
	}
	for id, name := range h.ev.Created {
		if !strings.HasPrefix(name, prefix) || !e2eIDRe.MatchString(id) {
			clean("refuse to delete "+id, false, "name %q is not this run's", name)
			continue
		}
		if _, err := h.api().GetProject(h.ctx, id); errors.Is(err, ErrNeonNotFound) {
			h.ev.Deleted[id] = name
			continue
		}
		_, err := h.cli("projects", "delete", id)
		clean("delete "+id+" ("+name+")", err == nil, "%v", errString(err))
		if err == nil {
			h.ev.Deleted[id] = name
		}
	}
	// Confirm: the CLI list has none of this run's names, and each id is 404.
	if h.org != "" {
		out, err := h.cli("projects", "list", "--org-id", h.org, "-o", "json")
		left := []string{}
		if err == nil {
			var list []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}
			if json.Unmarshal(out, &list) != nil {
				var wrapped struct {
					Projects []struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"projects"`
				}
				_ = json.Unmarshal(out, &wrapped)
				for _, p := range wrapped.Projects {
					list = append(list, struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					}{p.ID, p.Name})
				}
			}
			for _, p := range list {
				if strings.HasPrefix(p.Name, e2eNamePrefix+h.runID) {
					left = append(left, p.ID)
				}
			}
		}
		clean("neon projects list has no "+prefix+"* project", err == nil && len(left) == 0, "left: %v %v", left, errString(err))
	}
	for id := range h.ev.Created {
		_, err := h.api().GetProject(h.ctx, id)
		clean("GET /projects/"+id+" is 404", errors.Is(err, ErrNeonNotFound), "%v", errString(err))
	}

	// The work dir holds Terraform state with create-mode owner passwords.
	if err := os.RemoveAll(h.work); err != nil {
		clean("remove the work dir", false, "%v", err)
	}

	h.ev.Finished = time.Now().UTC()
	h.ev.AllPassed = true
	for _, s := range h.ev.Steps {
		if s.Status == "fail" || s.Status == "blocked" {
			h.ev.AllPassed = false
		}
	}
	for _, c := range h.ev.Cleanup {
		if !c.OK {
			h.ev.AllPassed = false
			h.t.Errorf("cleanup: %s: %s", c.Name, c.Detail)
		}
	}
	data, _ := json.MarshalIndent(h.ev, "", "  ")
	data = []byte(redactSecrets(string(data)))
	_ = os.WriteFile(filepath.Join(h.evidence, "neon-e2e.json"), append(data, '\n'), 0o600)
	h.logf("evidence written to %s", h.evidence)
	_ = h.log.Close()
}
