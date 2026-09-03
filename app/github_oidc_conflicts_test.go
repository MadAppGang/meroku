package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
)

// Every fixture here is synthetic. Account IDs are 000000000000 throughout —
// this repository is public and its CLAUDE.md forbids real infrastructure data
// in any committed file. Nothing below was copied out of an AWS console.

const testConflictAccount = "000000000000"

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// listRolesPage is one scripted answer to ListRoles: a page, or a failure in
// place of one.
type listRolesPage struct {
	out *iam.ListRolesOutput
	err error
}

// fakeRoleLister answers ListRoles and ListRoleTags from a script rather than
// AWS, following fakeIAM in app/github_oidc_test.go.
//
// It records what it was asked as well as how often, because two of the
// requirements are about the request rather than the answer: that the walk
// echoes the marker IAM handed back, and that it asks for 1,000 roles a page
// instead of the SDK default of 100.
type fakeRoleLister struct {
	pages []listRolesPage

	listCalls   int
	gotMarkers  []string
	gotMaxItems []int32

	tags     map[string][]iamtypes.Tag
	tagErrs  map[string]error
	tagCalls []string
}

func (f *fakeRoleLister) ListRoles(_ context.Context, in *iam.ListRolesInput, _ ...func(*iam.Options)) (*iam.ListRolesOutput, error) {
	f.gotMarkers = append(f.gotMarkers, aws.ToString(in.Marker))
	f.gotMaxItems = append(f.gotMaxItems, aws.ToInt32(in.MaxItems))

	if f.listCalls >= len(f.pages) {
		f.listCalls++
		return nil, fmt.Errorf("fakeRoleLister: unscripted ListRoles call %d", f.listCalls)
	}
	page := f.pages[f.listCalls]
	f.listCalls++
	return page.out, page.err
}

func (f *fakeRoleLister) ListRoleTags(_ context.Context, in *iam.ListRoleTagsInput, _ ...func(*iam.Options)) (*iam.ListRoleTagsOutput, error) {
	name := aws.ToString(in.RoleName)
	f.tagCalls = append(f.tagCalls, name)
	if err, ok := f.tagErrs[name]; ok {
		return nil, err
	}
	return &iam.ListRoleTagsOutput{Tags: f.tags[name]}, nil
}

// fakeCallerIdentity answers "which account are these credentials in".
type fakeCallerIdentity struct {
	account string
	err     error
	calls   int
}

func (f *fakeCallerIdentity) GetCallerIdentity(_ context.Context, _ *sts.GetCallerIdentityInput, _ ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &sts.GetCallerIdentityOutput{Account: aws.String(f.account)}, nil
}

func callerIn(account string) *fakeCallerIdentity { return &fakeCallerIdentity{account: account} }

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// conflictEnv is an environment with OIDC on, an account declared, and the
// given subjects — the shape every scan test starts from.
func conflictEnv(subjects ...string) Env {
	e := Env{
		Project:   "acme",
		Env:       "dev",
		Region:    "us-east-1",
		AccountID: testConflictAccount,
	}
	e.Workload.EnableGithubOIDC = true
	e.Workload.GithubOIDCSubjects = subjects
	return e
}

// subjectPolicy is a GitHub trust policy fenced to the given sub patterns.
func subjectPolicy(subjects ...string) string {
	quoted := make([]string, len(subjects))
	for i, s := range subjects {
		quoted[i] = strconv.Quote(s)
	}
	return githubTrustPolicy(`,
	    "Condition": { "StringLike": {
	      "token.actions.githubusercontent.com:sub": [` + strings.Join(quoted, ", ") + `]
	    }}`)
}

// unrestrictedPolicy pins no GitHub claim at all: every repository on GitHub
// can assume this role.
func unrestrictedPolicy() string {
	return githubTrustPolicy(``)
}

// otherClaimsPolicy is properly hardened, just not by anything this scan can
// intersect.
func otherClaimsPolicy() string {
	return githubTrustPolicy(`,
	    "Condition": { "StringEquals": {
	      "token.actions.githubusercontent.com:repository_owner": "acme"
	    }}`)
}

// nonGitHubPolicy is a perfectly ordinary service role. The scan must say
// nothing at all about it.
func nonGitHubPolicy() string {
	return `{"Version":"2012-10-17","Statement":[{"Effect":"Allow",
	  "Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
}

func iamRole(name, doc string) iamtypes.Role {
	return iamtypes.Role{
		RoleName:                 aws.String(name),
		Arn:                      aws.String("arn:aws:iam::" + testConflictAccount + ":role/" + name),
		AssumeRolePolicyDocument: aws.String(doc),
	}
}

// rolePage builds one ListRoles answer. nextMarker "" with truncated true is
// the incomplete walk IAM is documented never to send and which this scan must
// not treat as a clean end.
func rolePage(truncated bool, nextMarker string, roles ...iamtypes.Role) listRolesPage {
	out := &iam.ListRolesOutput{Roles: roles, IsTruncated: truncated}
	if nextMarker != "" {
		out.Marker = aws.String(nextMarker)
	}
	return listRolesPage{out: out}
}

func failedPage(err error) listRolesPage { return listRolesPage{err: err} }

func accessDenied() error {
	return &smithy.GenericAPIError{Code: "AccessDenied", Message: "User is not authorized to perform iam:ListRoles"}
}

// scanIn runs a scan from an empty temp directory, so ownRoleNames sees exactly
// the sibling environments a test wrote and never the ones in this repository.
func scanIn(t *testing.T, dir string, env Env, requested []string, roles *fakeRoleLister, ident *fakeCallerIdentity) githubOIDCConflictsResponse {
	t.Helper()
	chdir(t, dir)
	return scanGitHubSubjectConflicts(context.Background(), env, requested, roles, ident)
}

// ---------------------------------------------------------------------------
// Assertions
// ---------------------------------------------------------------------------

func degradedReasons(resp githubOIDCConflictsResponse) []string {
	out := make([]string, 0, len(resp.Degraded))
	for _, d := range resp.Degraded {
		out = append(out, d.Reason)
	}
	return out
}

func hasDegraded(resp githubOIDCConflictsResponse, reason string) bool {
	for _, d := range resp.Degraded {
		if d.Reason == reason {
			return true
		}
	}
	return false
}

func wantDegraded(t *testing.T, resp githubOIDCConflictsResponse, reason string) {
	t.Helper()
	if !hasDegraded(resp, reason) {
		t.Errorf("degraded = %v, want it to contain %q", degradedReasons(resp), reason)
	}
}

func wantNotChecked(t *testing.T, resp githubOIDCConflictsResponse) {
	t.Helper()
	if resp.Checked() {
		t.Errorf("Checked() = true with degraded %v; a scan that missed something may never claim to be checked",
			degradedReasons(resp))
	}
}

func conflictNames(resp githubOIDCConflictsResponse) []string {
	out := make([]string, 0, len(resp.Conflicts))
	for _, c := range resp.Conflicts {
		out = append(out, c.RoleName)
	}
	return out
}

// ---------------------------------------------------------------------------
// 1. Pagination
// ---------------------------------------------------------------------------

// The walk must echo back the marker IAM handed it, page after page. Getting
// this wrong re-reads page one forever or stops after it; either way roles go
// unread inside an answer that claims to be complete.
func TestScanGitHubSubjectConflictsWalksEveryPage(t *testing.T) {
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(true, "page-2", iamRole("first", nonGitHubPolicy())),
		rolePage(true, "page-3", iamRole("second", subjectPolicy("repo:billing/api:*"))),
		rolePage(false, "", iamRole("third", subjectPolicy("repo:acme/api:*"))),
	}}

	resp := scanIn(t, t.TempDir(), conflictEnv("repo:acme/api:*"), nil, roles, callerIn(testConflictAccount))

	if !resp.Checked() {
		t.Fatalf("Checked() = false, degraded %v; a complete three-page walk is complete", degradedReasons(resp))
	}
	if resp.RolesScanned != 3 {
		t.Errorf("RolesScanned = %d, want 3", resp.RolesScanned)
	}
	if got, want := resp.Conflicts, 1; len(got) != want {
		t.Fatalf("conflicts = %v, want the one role on page three", conflictNames(resp))
	}
	if resp.Conflicts[0].RoleName != "third" {
		t.Errorf("conflict role = %q, want %q; the last page was not evaluated", resp.Conflicts[0].RoleName, "third")
	}

	wantMarkers := []string{"", "page-2", "page-3"}
	if !reflect.DeepEqual(roles.gotMarkers, wantMarkers) {
		t.Errorf("markers sent = %v, want %v", roles.gotMarkers, wantMarkers)
	}
	for i, got := range roles.gotMaxItems {
		if got != githubOIDCListRolesPageSize {
			t.Errorf("call %d asked for MaxItems %d, want %d; the SDK default of 100 turns a large account into fifty sequential calls",
				i+1, got, githubOIDCListRolesPageSize)
		}
	}
}

// ---------------------------------------------------------------------------
// 2. IsTruncated with no marker
// ---------------------------------------------------------------------------

// IsTruncated:true with a nil Marker is an incomplete walk, not a clean end.
// Both plan reviewers flagged the same defect independently: breaking silently
// here turns a first page with no conflict into checked:true, conflicts:[]
// while every later role went unread.
func TestScanGitHubSubjectConflictsTruncatedWithoutMarker(t *testing.T) {
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(true, "", iamRole("billing-role", subjectPolicy("repo:acme/api:*"))),
	}}

	resp := scanIn(t, t.TempDir(), conflictEnv("repo:acme/api:*"), nil, roles, callerIn(testConflictAccount))

	wantDegraded(t, resp, githubOIDCReasonPagination)
	wantNotChecked(t, resp)
	if roles.listCalls != 1 {
		t.Errorf("ListRoles calls = %d, want 1; with no marker there is nothing to ask for next", roles.listCalls)
	}
	// The page that was read still counts.
	if len(resp.Conflicts) != 1 {
		t.Errorf("conflicts = %v, want the role found on the page that was read", conflictNames(resp))
	}
}

// ---------------------------------------------------------------------------
// 3. AccessDenied mid-walk
// ---------------------------------------------------------------------------

// The failure this whole file is arranged against. A denial on page two must
// preserve page one's findings and must never render as a clean bill.
func TestScanGitHubSubjectConflictsAccessDeniedMidWalk(t *testing.T) {
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(true, "page-2", iamRole("billing-role", subjectPolicy("repo:acme/*"))),
		failedPage(accessDenied()),
	}}

	resp := scanIn(t, t.TempDir(), conflictEnv("repo:acme/api:*"), nil, roles, callerIn(testConflictAccount))

	wantDegraded(t, resp, githubOIDCReasonAccessDenied)
	wantNotChecked(t, resp)

	if len(resp.Conflicts) != 1 || resp.Conflicts[0].RoleName != "billing-role" {
		t.Errorf("conflicts = %v, want the conflict found before the denial", conflictNames(resp))
	}

	// Stated the other way round, because this is the exact shape the feature
	// exists to prevent and it deserves to fail by name.
	if resp.Checked() && len(resp.Conflicts) == 0 {
		t.Fatal("AccessDenied rendered as checked:true with conflicts:[]; a refusal to answer is not evidence of absence")
	}
}

// ---------------------------------------------------------------------------
// 4-6. Refusals
// ---------------------------------------------------------------------------

// STS says one account, the config declares another. Listing anyway produces a
// confident answer about the wrong account, so nothing is listed at all.
func TestScanGitHubSubjectConflictsWrongAccount(t *testing.T) {
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "", iamRole("billing-role", subjectPolicy("repo:acme/*"))),
	}}
	ident := callerIn("111111111111")

	resp := scanIn(t, t.TempDir(), conflictEnv("repo:acme/api:*"), nil, roles, ident)

	wantDegraded(t, resp, githubOIDCReasonWrongAccount)
	wantNotChecked(t, resp)
	if roles.listCalls != 0 {
		t.Errorf("ListRoles calls = %d, want 0; the account assertion failed, so there is no account worth listing", roles.listCalls)
	}
	if len(resp.Conflicts) != 0 {
		t.Errorf("conflicts = %v, want none", conflictNames(resp))
	}
}

// A fresh environment has no account_id, and WithSharedConfigProfile("") is a
// documented no-op — so neither the profile pinning nor the account assertion
// would run, and the scan would report about whatever account the ambient
// AWS_PROFILE names. It is a hard refusal, before any call.
func TestScanGitHubSubjectConflictsNoAccountID(t *testing.T) {
	env := conflictEnv("repo:acme/api:*")
	env.AccountID = ""

	roles := &fakeRoleLister{}
	ident := callerIn(testConflictAccount)

	resp := scanIn(t, t.TempDir(), env, nil, roles, ident)

	wantDegraded(t, resp, githubOIDCReasonNoAccountID)
	wantNotChecked(t, resp)
	if roles.listCalls != 0 || ident.calls != 0 {
		t.Errorf("AWS calls = %d ListRoles / %d GetCallerIdentity, want 0 / 0", roles.listCalls, ident.calls)
	}
}

// OIDC off means there is no GitHub role for this environment, so there is
// nothing to overlap with. Not-applicable, and the UI stays silent rather than
// painting a permanent "could not verify" on every environment with the feature
// switched off.
func TestScanGitHubSubjectConflictsOIDCDisabled(t *testing.T) {
	env := conflictEnv("repo:acme/api:*")
	env.Workload.EnableGithubOIDC = false

	roles := &fakeRoleLister{}
	ident := callerIn(testConflictAccount)

	resp := scanIn(t, t.TempDir(), env, nil, roles, ident)

	wantDegraded(t, resp, githubOIDCReasonDisabled)
	if resp.Checked() {
		t.Error("Checked() = true; the scan did not run")
	}
	if roles.listCalls != 0 || ident.calls != 0 {
		t.Errorf("AWS calls = %d ListRoles / %d GetCallerIdentity, want 0 / 0", roles.listCalls, ident.calls)
	}
	if got := resp.Degraded[0].Kind(); got != githubOIDCDegradedNotApplicable {
		t.Errorf("kind = %q, want %q; a disabled feature is not a failed scan", got, githubOIDCDegradedNotApplicable)
	}
}

// ---------------------------------------------------------------------------
// 7. Own-role exclusion
// ---------------------------------------------------------------------------

// dev and prod of one project legitimately share a repository. Their roles come
// out of conflicts — but they are named in the response rather than dropped, so
// the blind spot is declared instead of hidden.
func TestScanGitHubSubjectConflictsExcludesOwnRoles(t *testing.T) {
	dir := t.TempDir()
	envYAML(t, dir, "dev.yaml", "acme", "dev", testConflictAccount)
	envYAML(t, dir, "prod.yaml", "acme", "prod", testConflictAccount)

	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "",
			iamRole("acme-dev-github-actions-role", subjectPolicy("repo:acme/api:*")),
			iamRole("acme-prod-github-actions-role", subjectPolicy("repo:acme/api:*")),
			iamRole("billing-prod-github-actions-role", subjectPolicy("repo:acme/api:*")),
		),
	}}

	resp := scanIn(t, dir, conflictEnv("repo:acme/api:*"), nil, roles, callerIn(testConflictAccount))

	if !resp.Checked() {
		t.Fatalf("Checked() = false, degraded %v", degradedReasons(resp))
	}
	if got := conflictNames(resp); len(got) != 1 || got[0] != "billing-prod-github-actions-role" {
		t.Errorf("conflicts = %v, want only the foreign role", got)
	}

	want := []string{"acme-dev-github-actions-role", "acme-prod-github-actions-role"}
	got := append([]string(nil), resp.ExcludedRoles...)
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExcludedRoles = %v, want %v", got, want)
	}
	if resp.ExcludedNote != githubOIDCExcludedNote {
		t.Errorf("ExcludedNote = %q, want %q", resp.ExcludedNote, githubOIDCExcludedNote)
	}
}

// ---------------------------------------------------------------------------
// 7b. Our own role trusting all of GitHub
// ---------------------------------------------------------------------------

// The state the own-role union cannot express. An unrestricted role accepts
// every subject, which the union represents as an EMPTY subject list — so it
// contributes no pairs, compares against nobody, and used to come back as a
// clean scan. It is reported on its own terms instead.
//
// And it is a FINDING, not a degradation: checked stays true and degraded stays
// empty, because the scan saw everything. What it saw was the worst answer
// available, which is a different fact from not having looked.
func TestScanGitHubSubjectConflictsOwnRoleUnrestricted(t *testing.T) {
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "", iamRole("acme-dev-github-actions-role", unrestrictedPolicy())),
	}}

	resp := scanIn(t, t.TempDir(), conflictEnv("repo:acme/api:*"), nil, roles, callerIn(testConflictAccount))

	if len(resp.OwnUnrestrictedRoles) != 1 {
		t.Fatalf("OwnUnrestrictedRoles = %+v, want this project's own wide-open role", resp.OwnUnrestrictedRoles)
	}
	got := resp.OwnUnrestrictedRoles[0]
	if got.RoleName != "acme-dev-github-actions-role" {
		t.Errorf("RoleName = %q, want %q", got.RoleName, "acme-dev-github-actions-role")
	}
	if want := "arn:aws:iam::" + testConflictAccount + ":role/acme-dev-github-actions-role"; got.RoleARN != want {
		t.Errorf("RoleARN = %q, want %q", got.RoleARN, want)
	}
	if got.Env != "dev" {
		t.Errorf("Env = %q, want %q; the scanned environment's own role name is computed, not guessed", got.Env, "dev")
	}

	// The point of the whole test. A scan that ran to completion reports
	// checked:true whatever it found; conflating "the news is bad" with "the
	// scan is unreliable" would put this finding in the yellow tier, where the
	// UI says "could not verify" about something it verified perfectly.
	if !resp.Checked() {
		t.Errorf("Checked() = false, degraded %v; nothing failed — the scan simply found the worst answer",
			degradedReasons(resp))
	}
	if len(resp.Degraded) != 0 {
		t.Errorf("Degraded = %v, want empty; an own unrestricted role is a finding, not a degradation",
			degradedReasons(resp))
	}

	// The exclusion is unchanged: it is still our role, so it is still named in
	// excluded_roles and still absent from conflicts.
	if len(resp.Conflicts) != 0 {
		t.Errorf("conflicts = %v, want none; our own role is never a conflict with itself", conflictNames(resp))
	}
	if !githubOIDCContains(resp.ExcludedRoles, "acme-dev-github-actions-role") {
		t.Errorf("ExcludedRoles = %v, want the own role still declared", resp.ExcludedRoles)
	}

	// And the pair travels over the wire together: checked:true alongside a
	// populated own_unrestricted_roles is a legal, and important, response.
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(body), `"checked":true`) {
		t.Errorf("marshalled %s, want checked:true", body)
	}
	if !strings.Contains(string(body), `"own_unrestricted_roles":[{`) {
		t.Errorf("marshalled %s, want a populated own_unrestricted_roles", body)
	}
}

// A sibling environment's role is ours too, and it is excluded the same way —
// but ownRoleNames returns names, not the environments behind them, and the
// naming cascade is not invertible. So the environment is left empty rather
// than guessed: a wrong environment in a security warning sends somebody to
// edit the wrong file.
func TestScanGitHubSubjectConflictsOwnSiblingUnrestricted(t *testing.T) {
	dir := t.TempDir()
	envYAML(t, dir, "dev.yaml", "acme", "dev", testConflictAccount)
	envYAML(t, dir, "prod.yaml", "acme", "prod", testConflictAccount)

	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "",
			iamRole("acme-dev-github-actions-role", subjectPolicy("repo:acme/api:*")),
			iamRole("acme-prod-github-actions-role", unrestrictedPolicy()),
		),
	}}

	resp := scanIn(t, dir, conflictEnv("repo:acme/api:*"), nil, roles, callerIn(testConflictAccount))

	if len(resp.OwnUnrestrictedRoles) != 1 {
		t.Fatalf("OwnUnrestrictedRoles = %+v, want the sibling's wide-open role", resp.OwnUnrestrictedRoles)
	}
	got := resp.OwnUnrestrictedRoles[0]
	if got.RoleName != "acme-prod-github-actions-role" {
		t.Errorf("RoleName = %q, want the sibling role", got.RoleName)
	}
	if got.Env != "" {
		t.Errorf("Env = %q, want empty; an environment that cannot be attributed is not guessed at", got.Env)
	}
	if !resp.Checked() {
		t.Errorf("Checked() = false, degraded %v", degradedReasons(resp))
	}
}

// The two findings are independent. An own role trusting all of GitHub does not
// suppress a cross-project conflict, and a cross-project conflict does not hide
// it: they answer different questions and both are reported.
func TestScanGitHubSubjectConflictsOwnUnrestrictedAndForeignConflict(t *testing.T) {
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "",
			iamRole("acme-dev-github-actions-role", unrestrictedPolicy()),
			iamRole("billing-prod-github-actions-role", subjectPolicy("repo:acme/api:*")),
		),
	}}

	resp := scanIn(t, t.TempDir(), conflictEnv("repo:acme/api:*"), nil, roles, callerIn(testConflictAccount))

	if len(resp.OwnUnrestrictedRoles) != 1 || resp.OwnUnrestrictedRoles[0].RoleName != "acme-dev-github-actions-role" {
		t.Errorf("OwnUnrestrictedRoles = %+v, want our own wide-open role", resp.OwnUnrestrictedRoles)
	}
	if got := conflictNames(resp); len(got) != 1 || got[0] != "billing-prod-github-actions-role" {
		t.Errorf("conflicts = %v, want the foreign overlap reported alongside it", got)
	}
	if !resp.Checked() {
		t.Errorf("Checked() = false, degraded %v; both findings came out of a complete scan",
			degradedReasons(resp))
	}
}

// The control. A normal own role — one that actually pins a sub condition —
// reports nothing here, so the array is evidence of a real state rather than of
// merely having an own role in the account.
func TestScanGitHubSubjectConflictsOwnRoleWithSubjectsIsNotReported(t *testing.T) {
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "",
			iamRole("acme-dev-github-actions-role", subjectPolicy("repo:acme/api:*")),
			iamRole("unrelated", nonGitHubPolicy()),
		),
	}}

	resp := scanIn(t, t.TempDir(), conflictEnv("repo:acme/api:*"), nil, roles, callerIn(testConflictAccount))

	if len(resp.OwnUnrestrictedRoles) != 0 {
		t.Errorf("OwnUnrestrictedRoles = %+v, want none; this role is fenced by a sub condition",
			resp.OwnUnrestrictedRoles)
	}
	if !resp.Checked() {
		t.Errorf("Checked() = false, degraded %v", degradedReasons(resp))
	}
}

// ---------------------------------------------------------------------------
// 7c. Our own subject granting an entire organisation
// ---------------------------------------------------------------------------

// The rule, as a table. It is a structural question about where the wildcards
// fall in "repo:<org>/<repo>:<ref-spec>", never a matching question, so it is
// tested as one: no roles, no AWS, no glob engine.
func TestGithubOIDCOrgWideSubjectFor(t *testing.T) {
	tests := []struct {
		name           string
		subject        string
		orgWide        bool
		org            string
		shippedDefault bool
	}{
		// The cases from the finding.
		{"repository segment wildcarded", "repo:acme/*", true, "acme", false},
		{"repository wildcarded, ref pinned", "repo:acme/*:ref:refs/heads/main", true, "acme", false},
		{"organisation itself wildcarded", "repo:*/api:*", true, "", false},
		{"the shipped default", "repo:MadAppGang/*", true, "MadAppGang", true},
		{"only the ref varies", "repo:acme/api:*", false, "", false},
		{"nothing varies", "repo:acme/api:ref:refs/heads/main", false, "", false},
		{"everything", "*", true, "", false},

		// The module's own former default is org-wide too, and is NOT the
		// shipped default: ShippedDefault is an exact string, and a near miss
		// must not claim meroku put it there.
		{"the module default, one segment longer", "repo:MadAppGang/*:*", true, "MadAppGang", false},
		{"the other half of the old CLI default", "repo:MadAppGang/project_backend:ref:refs/heads/main", false, "", false},

		// '?' counts wherever '*' does — it is a wildcard with a length rule,
		// not a literal.
		{"single-character wildcard in the repository", "repo:acme/ap?:*", true, "acme", false},
		{"single-character wildcard in the organisation", "repo:acm?/api:*", true, "", false},

		// Fewer segments than the grammar expects. None of these accepts a
		// repository nobody enumerated: each matches one exact string.
		{"empty", "", false, "", false},
		{"no separators at all", "repo", false, "", false},
		{"organisation only", "repo:acme", false, "", false},
		{"trailing slash, no repository", "repo:acme/", false, "", false},
		{"identity with no ref-spec", "repo:acme/api", false, "", false},
		{"empty organisation", "repo:/api:*", false, "", false},

		// A wildcard AFTER the identity never counts, however deep.
		{"wildcard in the branch", "repo:acme/api:ref:refs/heads/*", false, "", false},
		{"wildcard in a pull-request ref", "repo:acme/api:pull_request*", false, "", false},

		// A wildcard before "repo:" swallows the identity, so it counts.
		{"leading wildcard", "*:ref:refs/heads/main", true, "", false},

		// Not a GitHub subject at all. The rule is positional and applies
		// anyway: "arn:aws:*" wildcards only what sits after the second ':',
		// which under this grammar is the ref-spec, so it is not org-wide — and
		// it matches no GitHub token either way.
		{"foreign shape, wildcard after the identity", "arn:aws:*", false, "", false},
		{"foreign shape, wildcard inside the identity", "arn:*:iam", true, "", false},
		{"foreign shape, literal", "arn:aws:iam", false, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := githubOIDCOrgWideSubjectFor(tt.subject)

			if ok != tt.orgWide {
				t.Fatalf("githubOIDCOrgWideSubjectFor(%q) org-wide = %t, want %t", tt.subject, ok, tt.orgWide)
			}
			if !ok {
				// The zero value, so a caller that ignores ok cannot read a
				// subject off a subject that was never flagged.
				if got != (githubOIDCOrgWideSubject{}) {
					t.Errorf("githubOIDCOrgWideSubjectFor(%q) = %+v with ok=false, want the zero value", tt.subject, got)
				}
				return
			}
			if got.Subject != tt.subject {
				t.Errorf("Subject = %q, want %q", got.Subject, tt.subject)
			}
			if got.Org != tt.org {
				t.Errorf("Org = %q, want %q; only a literal organisation is named", got.Org, tt.org)
			}
			if got.ShippedDefault != tt.shippedDefault {
				t.Errorf("ShippedDefault = %t, want %t for %q", got.ShippedDefault, tt.shippedDefault, tt.subject)
			}
		})
	}
}

// The constant is the one place the shipped default's literal lives, and the
// copy that says "meroku put this here" turns on it. Pinning it here means a
// change to it is a deliberate edit to a test rather than a silent reclassification.
func TestGithubOIDCShippedDefaultSubjectIsExact(t *testing.T) {
	if githubOIDCShippedDefaultSubject != "repo:MadAppGang/*" {
		t.Errorf("githubOIDCShippedDefaultSubject = %q, want the literal this repository shipped",
			githubOIDCShippedDefaultSubject)
	}
	if _, ok := githubOIDCOrgWideSubjectFor(githubOIDCShippedDefaultSubject); !ok {
		t.Error("the shipped default is not classified as org-wide, which is the whole finding")
	}
}

// githubOIDCFindOrgWideSubjects reports in the caller's order, so the list lines
// up with the subject list the user is editing.
func TestGithubOIDCFindOrgWideSubjectsPreservesOrder(t *testing.T) {
	got := githubOIDCFindOrgWideSubjects([]string{
		"repo:acme/api:ref:refs/heads/main",
		"repo:zeta/*",
		"repo:acme/web:*",
		"repo:alpha/*:ref:refs/heads/main",
	})

	want := []string{"repo:zeta/*", "repo:alpha/*:ref:refs/heads/main"}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %v", got, want)
	}
	for i := range want {
		if got[i].Subject != want[i] {
			t.Errorf("[%d] = %q, want %q; order follows the caller's list, not sorted", i, got[i].Subject, want[i])
		}
	}

	// Nothing to report marshals as [] rather than nil, so the response's
	// contract holds even when this is assigned wholesale.
	if empty := githubOIDCFindOrgWideSubjects(nil); empty == nil {
		t.Error("githubOIDCFindOrgWideSubjects(nil) = nil, want an empty slice")
	}
}

// End to end. A configured org-wide subject is reported, and — the point of the
// tier — the scan still says it checked everything, because it did.
func TestScanGitHubSubjectConflictsOwnOrgWideSubject(t *testing.T) {
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "",
			iamRole("acme-dev-github-actions-role", subjectPolicy("repo:acme/*")),
			iamRole("unrelated", nonGitHubPolicy()),
		),
	}}

	resp := scanIn(t, t.TempDir(), conflictEnv("repo:acme/*"), nil, roles, callerIn(testConflictAccount))

	if len(resp.OwnOrgWideSubjects) != 1 {
		t.Fatalf("OwnOrgWideSubjects = %+v, want the org-wide subject reported", resp.OwnOrgWideSubjects)
	}
	got := resp.OwnOrgWideSubjects[0]
	if got.Subject != "repo:acme/*" {
		t.Errorf("Subject = %q, want %q", got.Subject, "repo:acme/*")
	}
	if got.Org != "acme" {
		t.Errorf("Org = %q, want %q", got.Org, "acme")
	}
	if got.ShippedDefault {
		t.Error("ShippedDefault = true; this subject is the user's own, not meroku's")
	}

	// The whole reason this is a separate tier rather than a degraded entry.
	if !resp.Checked() {
		t.Errorf("Checked() = false, degraded %v; nothing failed — the scan simply found something bad",
			degradedReasons(resp))
	}
	if len(resp.Degraded) != 0 {
		t.Errorf("Degraded = %v, want empty; an org-wide subject is a finding, not a degradation",
			degradedReasons(resp))
	}

	// And the pair travels over the wire together.
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(body), `"checked":true`) {
		t.Errorf("marshalled %s, want checked:true", body)
	}
	if !strings.Contains(string(body), `"own_org_wide_subjects":[{`) {
		t.Errorf("marshalled %s, want a populated own_org_wide_subjects", body)
	}
}

// The shipped default, arriving from a config nobody edited. Distinguished from
// any other org-wide subject because the copy has to be: for anyone who is not
// MadAppGang this grants a third-party organisation their AWS account.
func TestScanGitHubSubjectConflictsShippedDefaultSubject(t *testing.T) {
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "", iamRole("unrelated", nonGitHubPolicy())),
	}}

	resp := scanIn(t, t.TempDir(), conflictEnv(githubOIDCShippedDefaultSubject), nil, roles, callerIn(testConflictAccount))

	if len(resp.OwnOrgWideSubjects) != 1 {
		t.Fatalf("OwnOrgWideSubjects = %+v, want the untouched default reported", resp.OwnOrgWideSubjects)
	}
	if got := resp.OwnOrgWideSubjects[0]; !got.ShippedDefault || got.Org != "MadAppGang" {
		t.Errorf("got %+v, want ShippedDefault true and Org MadAppGang", got)
	}
	if !resp.Checked() {
		t.Errorf("Checked() = false, degraded %v", degradedReasons(resp))
	}
}

// The deployed half of the union, which is the case a YAML-only check misses.
// Narrow the subject list in the file, never apply it, and the live role still
// accepts the whole organisation.
func TestScanGitHubSubjectConflictsOrgWideFromDeployedPolicy(t *testing.T) {
	env := conflictEnv("repo:acme/api:ref:refs/heads/main")

	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "", iamRole("acme-dev-github-actions-role", subjectPolicy("repo:acme/*"))),
	}}

	resp := scanIn(t, t.TempDir(), env, nil, roles, callerIn(testConflictAccount))

	if len(resp.OwnOrgWideSubjects) != 1 || resp.OwnOrgWideSubjects[0].Subject != "repo:acme/*" {
		t.Fatalf("OwnOrgWideSubjects = %+v, want the DEPLOYED org-wide subject; the file alone would look clean",
			resp.OwnOrgWideSubjects)
	}
	if !resp.Checked() {
		t.Errorf("Checked() = false, degraded %v", degradedReasons(resp))
	}
}

// The control. A properly narrow subject reports nothing, so a populated array
// is evidence of a real state rather than of merely having subjects.
func TestScanGitHubSubjectConflictsNarrowSubjectIsNotOrgWide(t *testing.T) {
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "",
			iamRole("acme-dev-github-actions-role", subjectPolicy("repo:acme/api:*")),
			iamRole("unrelated", nonGitHubPolicy()),
		),
	}}

	resp := scanIn(t, t.TempDir(), conflictEnv("repo:acme/api:*"), nil, roles, callerIn(testConflictAccount))

	if len(resp.OwnOrgWideSubjects) != 0 {
		t.Errorf("OwnOrgWideSubjects = %+v, want none; only the ref varies in this subject",
			resp.OwnOrgWideSubjects)
	}
	if !resp.Checked() {
		t.Errorf("Checked() = false, degraded %v", degradedReasons(resp))
	}
}

// ---------------------------------------------------------------------------
// 8. The deployed-policy union
// ---------------------------------------------------------------------------

// Narrow the subject list in YAML, do not apply it, and the deployed role still
// trusts everything it used to. A scan that compared only the file would render
// green over a live overlap.
func TestScanGitHubSubjectConflictsUnionsDeployedPolicy(t *testing.T) {
	dir := t.TempDir()

	// YAML has been narrowed to one repository...
	env := conflictEnv("repo:acme/api:ref:refs/heads/main")

	// ...but the role deployed in AWS still trusts the whole organisation.
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "",
			iamRole("acme-dev-github-actions-role", subjectPolicy("repo:acme/*")),
			iamRole("billing-prod-github-actions-role", subjectPolicy("repo:acme/web:*")),
		),
	}}

	resp := scanIn(t, dir, env, nil, roles, callerIn(testConflictAccount))

	if len(resp.Conflicts) != 1 {
		t.Fatalf("conflicts = %v, want the overlap the DEPLOYED policy still has", conflictNames(resp))
	}
	if got, want := resp.OwnSubjectsSource, githubOIDCSubjectsFromYAML+githubOIDCSubjectsDeployedSuffix; got != want {
		t.Errorf("OwnSubjectsSource = %q, want %q", got, want)
	}
	if !githubOIDCContains(resp.OwnSubjects, "repo:acme/*") {
		t.Errorf("OwnSubjects = %v, want the deployed pattern unioned in", resp.OwnSubjects)
	}

	// The same fixture with the YAML value alone finds nothing, which is what
	// makes the union the thing under test rather than an incidental extra.
	quiet := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "", iamRole("billing-prod-github-actions-role", subjectPolicy("repo:acme/web:*"))),
	}}
	if resp2 := scanIn(t, dir, env, nil, quiet, callerIn(testConflictAccount)); len(resp2.Conflicts) != 0 {
		t.Fatalf("control: conflicts = %v, want none from the narrowed YAML alone", conflictNames(resp2))
	}
}

// A request carrying candidate subjects replaces the YAML read, so a scan
// answers about what the user is looking at rather than about the last save.
func TestScanGitHubSubjectConflictsUsesRequestSubjects(t *testing.T) {
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "", iamRole("billing-role", subjectPolicy("repo:acme/web:*"))),
	}}

	resp := scanIn(t, t.TempDir(), conflictEnv("repo:acme/api:*"), []string{"repo:acme/web:*"}, roles, callerIn(testConflictAccount))

	if got := resp.OwnSubjectsSource; got != githubOIDCSubjectsFromRequest {
		t.Errorf("OwnSubjectsSource = %q, want %q", got, githubOIDCSubjectsFromRequest)
	}
	if len(resp.Conflicts) != 1 {
		t.Errorf("conflicts = %v, want the overlap with the requested subject", conflictNames(resp))
	}
}

// ---------------------------------------------------------------------------
// 9-11. Classifying foreign roles
// ---------------------------------------------------------------------------

// A role with no GitHub claim at all trusts every repository on GitHub, so it
// overlaps by construction and needs no witness.
func TestScanGitHubSubjectConflictsUnrestrictedRole(t *testing.T) {
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "", iamRole("wide-open", unrestrictedPolicy())),
	}}

	resp := scanIn(t, t.TempDir(), conflictEnv("repo:acme/api:*"), nil, roles, callerIn(testConflictAccount))

	if !resp.Checked() {
		t.Fatalf("Checked() = false, degraded %v; nothing failed here", degradedReasons(resp))
	}
	if len(resp.Conflicts) != 1 {
		t.Fatalf("conflicts = %v, want the unrestricted role", conflictNames(resp))
	}
	if !resp.Conflicts[0].Unrestricted {
		t.Error("Unrestricted = false; this role pins no GitHub claim and trusts all of GitHub")
	}
}

// A role fenced by :repository_owner is properly hardened and simply not
// expressible as a subject glob. It is neither a conflict nor a clean role: it
// is one nobody evaluated, and saying so is the honest answer.
func TestScanGitHubSubjectConflictsRestrictedByOtherClaims(t *testing.T) {
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "", iamRole("hardened", otherClaimsPolicy())),
	}}

	resp := scanIn(t, t.TempDir(), conflictEnv("repo:acme/api:*"), nil, roles, callerIn(testConflictAccount))

	wantDegraded(t, resp, githubOIDCReasonOtherClaims)
	wantNotChecked(t, resp)
	if len(resp.Conflicts) != 0 {
		t.Errorf("conflicts = %v, want none; an unevaluated role is not a confirmed conflict", conflictNames(resp))
	}
	if len(resp.UnevaluatedRoles) != 1 {
		t.Fatalf("UnevaluatedRoles = %v, want the hardened role listed", resp.UnevaluatedRoles)
	}
	got := resp.UnevaluatedRoles[0]
	if got.RoleName != "hardened" || got.Reason != githubOIDCUnevaluatedOtherClaims {
		t.Errorf("UnevaluatedRoles[0] = %+v, want hardened / %s", got, githubOIDCUnevaluatedOtherClaims)
	}
	if !reflect.DeepEqual(got.ClaimKeys, []string{"repository_owner"}) {
		t.Errorf("ClaimKeys = %v, want [repository_owner]", got.ClaimKeys)
	}
}

// A policy that will not parse hides a role exactly as thoroughly as a denial
// does, and the message has to name the role or it is a shrug.
func TestScanGitHubSubjectConflictsUnparseablePolicy(t *testing.T) {
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "",
			iamRole("mangled", `{"Statement": [ this is not JSON`),
			iamRole("billing-role", subjectPolicy("repo:acme/api:*")),
		),
	}}

	resp := scanIn(t, t.TempDir(), conflictEnv("repo:acme/api:*"), nil, roles, callerIn(testConflictAccount))

	wantDegraded(t, resp, githubOIDCReasonUnparseable)
	wantNotChecked(t, resp)

	var detail string
	for _, d := range resp.Degraded {
		if d.Reason == githubOIDCReasonUnparseable {
			detail = d.Detail
		}
	}
	if !strings.Contains(detail, "mangled") {
		t.Errorf("detail = %q, want it to name the role that could not be read", detail)
	}
	// The rest of the page is still evaluated.
	if len(resp.Conflicts) != 1 {
		t.Errorf("conflicts = %v, want the readable role still evaluated", conflictNames(resp))
	}
}

// ---------------------------------------------------------------------------
// 12. Attribution never gates the verdict
// ---------------------------------------------------------------------------

func TestScanGitHubSubjectConflictsAttribution(t *testing.T) {
	page := rolePage(false, "",
		iamRole("tagged-role", subjectPolicy("repo:acme/api:*")),
		iamRole("untagged-role", subjectPolicy("repo:acme/api:*")),
		iamRole("denied-role", subjectPolicy("repo:acme/api:*")),
	)
	roles := &fakeRoleLister{
		pages: []listRolesPage{page},
		tags: map[string][]iamtypes.Tag{
			"tagged-role": {tag("Project", "billing"), tag("Environment", "prod")},
		},
		tagErrs: map[string]error{"denied-role": accessDenied()},
	}

	resp := scanIn(t, t.TempDir(), conflictEnv("repo:acme/api:*"), nil, roles, callerIn(testConflictAccount))

	// The tag read failed on one role and the verdict is untouched: three
	// conflicts, and a scan that is still complete.
	if !resp.Checked() {
		t.Errorf("Checked() = false, degraded %v; a failed tag read is not a failed scan", degradedReasons(resp))
	}
	if len(resp.Conflicts) != 3 {
		t.Fatalf("conflicts = %v, want all three", conflictNames(resp))
	}

	want := map[string]string{
		"denied-role":   githubOIDCAttributionUnavailable,
		"tagged-role":   githubOIDCAttributionTags,
		"untagged-role": githubOIDCAttributionUntagged,
	}
	for _, c := range resp.Conflicts {
		if got := c.Attribution; got != want[c.RoleName] {
			t.Errorf("%s attribution = %q, want %q", c.RoleName, got, want[c.RoleName])
		}
	}
	for _, c := range resp.Conflicts {
		if c.RoleName == "tagged-role" && (c.OwnerProject != "billing" || c.OwnerEnv != "prod") {
			t.Errorf("tagged-role owner = %q/%q, want billing/prod", c.OwnerProject, c.OwnerEnv)
		}
	}
	if len(roles.tagCalls) != 3 {
		t.Errorf("ListRoleTags calls = %d, want one per confirmed conflict", len(roles.tagCalls))
	}
}

// ---------------------------------------------------------------------------
// 13. Witness realism
// ---------------------------------------------------------------------------

// The DP's witness is mathematically correct and sometimes rhetorically false.
// "repo:MadAppGang/*" against itself yields "repo:MadAppGang/" — a valid
// witness that no token has ever carried. Quoting it under "a token with this
// exact claim assumes both roles" is a false statement in a security warning.
func TestGitHubOIDCWitnessIsIssuable(t *testing.T) {
	cases := []struct {
		name, a, b, want string
	}{
		{
			name: "the default subject list against itself",
			a:    "repo:MadAppGang/*",
			b:    "repo:MadAppGang/*",
			want: "repo:MadAppGang/example-repo:ref:refs/heads/main",
		},
		{
			name: "two stars, whose mathematical witness is the empty string",
			a:    "*",
			b:    "*",
			want: githubOIDCExampleSub,
		},
		{
			name: "a witness that is already a real subject is left alone",
			a:    "repo:acme/api:*",
			b:    "repo:acme/*:ref:refs/heads/main",
			want: "repo:acme/api:ref:refs/heads/main",
		},
		{
			name: "truncated after the repository name",
			a:    "repo:acme/api*",
			b:    "repo:acme/*",
			want: "repo:acme/api:ref:refs/heads/main",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := githubOIDCWitness(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("githubOIDCWitness(%q, %q) = %q, want %q", tc.a, tc.b, got, tc.want)
			}
			// Whatever is reported must be accepted by both patterns. A witness
			// globMatch rejects is worse than no witness at all.
			if !globMatch([]byte(tc.a), []byte(got)) || !globMatch([]byte(tc.b), []byte(got)) {
				t.Errorf("witness %q is not matched by both %q and %q", got, tc.a, tc.b)
			}
			if !githubOIDCWitnessIsIssuable(got) {
				t.Errorf("witness %q is not a subject GitHub could issue", got)
			}
		})
	}

	if got := githubOIDCWitness("repo:acme/api:*", "repo:billing/*"); got != "" {
		t.Errorf("witness for two disjoint patterns = %q, want the empty string", got)
	}
}

// Every witness the scan reports, on a fixture built from the shapes that
// actually occur, survives globMatch against both patterns.
func TestScanGitHubSubjectConflictsWitnessesVerify(t *testing.T) {
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "",
			iamRole("org-wide", subjectPolicy("repo:MadAppGang/*")),
			iamRole("branch-pinned", subjectPolicy("repo:MadAppGang/*:ref:refs/heads/main")),
		),
	}}

	resp := scanIn(t, t.TempDir(), conflictEnv("repo:MadAppGang/*"), nil, roles, callerIn(testConflictAccount))

	if len(resp.Conflicts) != 2 {
		t.Fatalf("conflicts = %v, want both roles", conflictNames(resp))
	}
	for _, c := range resp.Conflicts {
		for _, o := range c.Overlaps {
			if o.Witness == "" {
				t.Errorf("%s: empty witness for %q x %q", c.RoleName, o.OwnSubject, o.OtherSubject)
				continue
			}
			if !globMatch([]byte(o.OwnSubject), []byte(o.Witness)) || !globMatch([]byte(o.OtherSubject), []byte(o.Witness)) {
				t.Errorf("%s: witness %q is not matched by both %q and %q",
					c.RoleName, o.Witness, o.OwnSubject, o.OtherSubject)
			}
			if !githubOIDCWitnessIsIssuable(o.Witness) {
				t.Errorf("%s: witness %q is not a subject GitHub could issue", c.RoleName, o.Witness)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 14. The completeness invariant
// ---------------------------------------------------------------------------

// checked is derived, never assigned. This is the test that makes it so: there
// is no field to set, and every scan-incomplete reason forces false on its own.
func TestGitHubOIDCCheckedIsDerived(t *testing.T) {
	// A settable Checked field is exactly the design this test forbids: it puts
	// the invariant back at every failure site, where the one site that forgets
	// renders a partial scan as reassurance.
	if _, found := reflect.TypeOf(githubOIDCConflictsResponse{}).FieldByName("Checked"); found {
		t.Fatal("githubOIDCConflictsResponse has a Checked field; checked must be derived from the degraded list, not assigned")
	}

	scanIncomplete := []string{
		githubOIDCReasonNoAccountID, githubOIDCReasonNoCredentials, githubOIDCReasonWrongAccount,
		githubOIDCReasonAccessDenied, githubOIDCReasonThrottled, githubOIDCReasonTimeout,
		githubOIDCReasonEnvUnreadable, githubOIDCReasonUnparseable, githubOIDCReasonPatternTooLong,
		githubOIDCReasonPagination, githubOIDCReasonOtherClaims, githubOIDCReasonBudgetExhausted,
		githubOIDCReasonAWSError,
		"a_reason_nobody_has_invented_yet",
	}
	for _, reason := range scanIncomplete {
		resp := newGithubOIDCConflictsResponse()
		resp.degrade(reason, "")
		if resp.Checked() {
			t.Errorf("%s: Checked() = true; a scan-incomplete reason must force false", reason)
		}
		body, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !strings.Contains(string(body), `"checked":false`) {
			t.Errorf("%s: marshalled %s, want checked:false", reason, body)
		}
		if !strings.Contains(string(body), `"kind":"`+githubOIDCDegradedScanIncomplete+`"`) {
			t.Errorf("%s: marshalled %s, want the entry marked scan_incomplete", reason, body)
		}
	}

	// The not-applicable partition is equally unchecked — a scan that did not
	// need to run verified nothing either. What differs is the marking, which
	// is how the client knows to stay silent instead of going yellow.
	for _, reason := range []string{githubOIDCReasonDisabled, githubOIDCReasonNoSubjects} {
		resp := newGithubOIDCConflictsResponse()
		resp.degrade(reason, "")
		if resp.Checked() {
			t.Errorf("%s: Checked() = true; nothing was verified", reason)
		}
		body, _ := json.Marshal(resp)
		if !strings.Contains(string(body), `"kind":"`+githubOIDCDegradedNotApplicable+`"`) {
			t.Errorf("%s: marshalled %s, want the entry marked not_applicable", reason, body)
		}
	}
}

// ---------------------------------------------------------------------------
// 15. Marshalling
// ---------------------------------------------------------------------------

// The UI calls .length on every one of these arrays. A nil slice marshals as
// null, and null.length throws before anything is rendered at all.
func TestGitHubOIDCConflictsResponseMarshalsEmptyArrays(t *testing.T) {
	body, err := json.Marshal(newGithubOIDCConflictsResponse())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(body)

	for _, field := range []string{
		"conflicts", "degraded", "excluded_roles", "unevaluated_roles",
		"own_subjects", "own_unrestricted_roles", "own_org_wide_subjects",
	} {
		if !strings.Contains(got, `"`+field+`":[]`) {
			t.Errorf("marshalled %s, want %q to be []", got, field)
		}
		if strings.Contains(got, `"`+field+`":null`) {
			t.Errorf("marshalled %s, want %q not to be null", got, field)
		}
	}
	if !strings.Contains(got, `"checked":true`) {
		t.Errorf("marshalled %s, want checked:true on a response with nothing degraded", got)
	}

	// A real zero-conflict scan marshals the same way.
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "", iamRole("unrelated", nonGitHubPolicy())),
	}}
	resp := scanIn(t, t.TempDir(), conflictEnv("repo:acme/api:*"), nil, roles, callerIn(testConflictAccount))
	body, err = json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, field := range []string{"conflicts", "own_unrestricted_roles", "own_org_wide_subjects"} {
		if !strings.Contains(string(body), `"`+field+`":[]`) || strings.Contains(string(body), `"`+field+`":null`) {
			t.Errorf("marshalled %s, want %s:[]", body, field)
		}
	}
}

// ---------------------------------------------------------------------------
// 16-17. Handler
// ---------------------------------------------------------------------------

// handlerEnvYAML writes an environment at the current schema version, so
// loading it cannot trigger a migration rewrite and any change to the file
// afterwards is unambiguously this endpoint's doing.
func handlerEnvYAML(t *testing.T, dir, name string, oidc bool, accountID string) string {
	t.Helper()
	body := fmt.Sprintf(`schema_version: %d
project: acme
env: %s
account_id: "%s"
region: us-east-1
workload:
  enable_github_oidc: %t
  github_oidc_subjects:
    - "repo:acme/api:*"
`, CurrentSchemaVersion, name, accountID, oidc)
	path := filepath.Join(dir, name+".yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func postConflicts(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/environments/github-oidc-subject-conflicts", strings.NewReader(body))
	getGitHubOIDCSubjectConflicts(rec, req)
	return rec
}

func TestGetGitHubOIDCSubjectConflictsHandler(t *testing.T) {
	dir := t.TempDir()
	handlerEnvYAML(t, dir, "dev", false, testConflictAccount)
	chdir(t, dir)

	t.Run("GET is not allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		getGitHubOIDCSubjectConflicts(rec, httptest.NewRequest(http.MethodGet,
			"/api/environments/github-oidc-subject-conflicts", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("a body that is not JSON is a bad request", func(t *testing.T) {
		if rec := postConflicts(t, `{not json`); rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("a missing env field is a bad request", func(t *testing.T) {
		if rec := postConflicts(t, `{}`); rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("an environment that does not exist is not found", func(t *testing.T) {
		if rec := postConflicts(t, `{"env":"nosuchenv"}`); rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	// The departure from this file's 502 convention: an HTTP status carries two
	// outcomes and this response carries three, so everything past a bad
	// request is 200 with checked and degraded doing the talking.
	t.Run("oidc disabled is 200 with a not-applicable degraded entry", func(t *testing.T) {
		rec := postConflicts(t, `{"env":"dev"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		var got struct {
			Checked   bool       `json:"checked"`
			Conflicts []struct{} `json:"conflicts"`
			Degraded  []struct {
				Reason string `json:"reason"`
				Kind   string `json:"kind"`
			} `json:"degraded"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode %s: %v", rec.Body.String(), err)
		}
		if got.Checked {
			t.Error("checked = true; the scan did not run")
		}
		if len(got.Degraded) != 1 || got.Degraded[0].Reason != githubOIDCReasonDisabled {
			t.Fatalf("degraded = %+v, want one %s entry", got.Degraded, githubOIDCReasonDisabled)
		}
		if got.Degraded[0].Kind != githubOIDCDegradedNotApplicable {
			t.Errorf("kind = %q, want %q", got.Degraded[0].Kind, githubOIDCDegradedNotApplicable)
		}
		if got.Conflicts == nil {
			t.Error("conflicts decoded as null; the UI calls .length on it")
		}
	})
}

// The endpoint is read-only. Its sibling getGithubOIDCStatus writes YAML, and
// this scan is re-run after every subject edit — an endpoint that mutates on
// every keystroke of a security check is a mutation path nobody asked for.
//
// writeGithubOIDCCreateProvider is package-level and cannot be injected, so the
// assertion is on what it would leave behind: a rewritten environment file and
// a <file>.backup_<timestamp> beside it.
func TestGetGitHubOIDCSubjectConflictsNeverWritesYAML(t *testing.T) {
	dir := t.TempDir()
	devPath := handlerEnvYAML(t, dir, "dev", false, testConflictAccount)
	// OIDC on but no account id: a refusal that still makes no AWS call, so it
	// exercises a second path through the handler without a network.
	freshPath := handlerEnvYAML(t, dir, "fresh", true, "")
	chdir(t, dir)

	before := map[string][]byte{}
	for _, p := range []string{devPath, freshPath} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		before[p] = data
	}
	entriesBefore := dirEntryNames(t, dir)

	// Every path the handler can take without reaching AWS.
	rec := httptest.NewRecorder()
	getGitHubOIDCSubjectConflicts(rec, httptest.NewRequest(http.MethodGet, "/api/x", nil))
	postConflicts(t, `{not json`)
	postConflicts(t, `{}`)
	postConflicts(t, `{"env":"nosuchenv"}`)
	postConflicts(t, `{"env":"dev"}`)
	if rec := postConflicts(t, `{"env":"fresh"}`); rec.Code != http.StatusOK {
		t.Errorf("fresh env status = %d, want 200", rec.Code)
	}

	for p, want := range before {
		got, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if string(got) != string(want) {
			t.Errorf("%s was rewritten; this endpoint is read-only", p)
		}
	}
	if got := dirEntryNames(t, dir); !reflect.DeepEqual(got, entriesBefore) {
		t.Errorf("directory contents changed from %v to %v; a backup file means something wrote YAML",
			entriesBefore, got)
	}
}

func dirEntryNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// ---------------------------------------------------------------------------
// Budgets
// ---------------------------------------------------------------------------

// A pattern too long to compare hides a role as effectively as a denial does,
// so the defensive skip has to report itself.
func TestScanGitHubSubjectConflictsPatternTooLong(t *testing.T) {
	long := "repo:acme/" + strings.Repeat("a", githubOIDCMaxPatternBytes) + "*"
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "", iamRole("enormous", subjectPolicy(long))),
	}}

	resp := scanIn(t, t.TempDir(), conflictEnv("repo:acme/api:*"), nil, roles, callerIn(testConflictAccount))

	wantDegraded(t, resp, githubOIDCReasonPatternTooLong)
	wantNotChecked(t, resp)
}

// The pair cap is the other half of honouring a deadline that cannot preempt
// the DP. Exhausting it means roles went uncompared, so it degrades rather than
// quietly returning what it managed.
//
// 100 own subjects x 50 subjects x 50 roles is exactly the cap, so the 51st
// role is the one that cannot be reached. The patterns are short and pairwise
// disjoint on purpose: this test is about the counter, not about the DP.
func TestScanGitHubSubjectConflictsPairBudget(t *testing.T) {
	const ownCount, subjectsPerRole = 100, 50
	roleCount := githubOIDCMaxPairs/(ownCount*subjectsPerRole) + 1

	ownSubjects := make([]string, ownCount)
	for i := range ownSubjects {
		ownSubjects[i] = fmt.Sprintf("a%d", i)
	}
	subjects := make([]string, subjectsPerRole)
	for i := range subjects {
		subjects[i] = fmt.Sprintf("b%d", i)
	}

	roles := make([]iamtypes.Role, roleCount)
	for i := range roles {
		roles[i] = iamRole(fmt.Sprintf("role-%03d", i), subjectPolicy(subjects...))
	}

	lister := &fakeRoleLister{pages: []listRolesPage{rolePage(false, "", roles...)}}
	resp := scanIn(t, t.TempDir(), conflictEnv(ownSubjects...), nil, lister, callerIn(testConflictAccount))

	wantDegraded(t, resp, githubOIDCReasonBudgetExhausted)
	wantNotChecked(t, resp)
}

// context.WithTimeout does not preempt CPU-bound work, so the ctx.Err() checks
// between roles are the only thing that honours the handler's deadline once the
// walk is underway.
func TestScanGitHubSubjectConflictsHonoursCancellation(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "", iamRole("billing-role", subjectPolicy("repo:acme/*"))),
	}}
	resp := scanGitHubSubjectConflicts(ctx, conflictEnv("repo:acme/api:*"), nil, roles, callerIn(testConflictAccount))

	wantDegraded(t, resp, githubOIDCReasonTimeout)
	wantNotChecked(t, resp)
}

// An unrecognised AWS failure must not be dressed up as one of the specific
// reasons. Calling a network partition "access_denied" sends the user off to
// rewrite an IAM policy that was never the problem.
func TestGitHubOIDCFailureReason(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"denial", accessDenied(), githubOIDCReasonAccessDenied},
		{"throttle", &smithy.GenericAPIError{Code: "ThrottlingException", Message: "slow down"}, githubOIDCReasonThrottled},
		{"expired credentials", &smithy.GenericAPIError{Code: "ExpiredToken", Message: "token expired"}, githubOIDCReasonNoCredentials},
		{"deadline", context.DeadlineExceeded, githubOIDCReasonTimeout},
		{"anything else", fmt.Errorf("dial tcp: connection refused"), githubOIDCReasonAWSError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := githubOIDCFailureReason(context.Background(), tc.err)
			if got != tc.want {
				t.Errorf("githubOIDCFailureReason(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// An account with no subjects anywhere — neither configured nor deployed — has
// nothing to compare. That is a complete answer to an empty question, so the UI
// stays silent rather than going yellow.
func TestScanGitHubSubjectConflictsNoSubjects(t *testing.T) {
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "", iamRole("billing-role", subjectPolicy("repo:acme/*"))),
	}}

	resp := scanIn(t, t.TempDir(), conflictEnv(), nil, roles, callerIn(testConflictAccount))

	wantDegraded(t, resp, githubOIDCReasonNoSubjects)
	if got := resp.Degraded[0].Kind(); got != githubOIDCDegradedNotApplicable {
		t.Errorf("kind = %q, want %q; there was nothing to compare, which is not a failed scan",
			got, githubOIDCDegradedNotApplicable)
	}
	if len(resp.Conflicts) != 0 {
		t.Errorf("conflicts = %v, want none", conflictNames(resp))
	}
}
