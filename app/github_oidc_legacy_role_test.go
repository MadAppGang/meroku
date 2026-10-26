package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// An environment deployed before modules/workloads stopped hard-coding the role
// name holds a GitHub Actions role called "GithubActionsRole", untagged, with
// whatever subjects it was given then. The computed own-role name is the new
// one, so the scan used to treat the environment's own role as another
// project's, and the deploy that would rename it stopped at a conflict prompt.
//
// The shape below is the case that hit it: the YAML narrowed to one repository
// and branch, the live role still on meroku's old org-wide default.

const legacyGitHubRoleName = "GithubActionsRole"

func legacyRoleAccount(t *testing.T) (Env, *fakeRoleLister) {
	t.Helper()
	env := conflictEnv("repo:acme/api:ref:refs/heads/dev")
	roles := &fakeRoleLister{pages: []listRolesPage{
		rolePage(false, "",
			iamRole(legacyGitHubRoleName, subjectPolicy("repo:acme/*")),
			iamRole("acme_backend_task_dev", `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}`)),
	}}
	return env, roles
}

func TestScanTreatsARoleInThisEnvironmentsStateAsOwn(t *testing.T) {
	env, roles := legacyRoleAccount(t)

	resp := scanInWithState(t, t.TempDir(), env, nil, roles, callerIn(testConflictAccount),
		stateWithRoles(legacyGitHubRoleName, "acme_backend_task_dev"))

	if len(resp.Conflicts) != 0 {
		t.Errorf("conflicts = %v, want none: the role is in this environment's own state", conflictNames(resp))
	}
	if len(resp.ExcludedRoles) != 1 || resp.ExcludedRoles[0] != legacyGitHubRoleName {
		t.Errorf("excluded = %v, want only %s: a task role federates nothing and is not worth naming",
			resp.ExcludedRoles, legacyGitHubRoleName)
	}

	// Its live subjects still count as ours: until the apply, the role accepts
	// them, and the org-wide one is still worth saying.
	if len(resp.OwnOrgWideSubjects) != 1 || resp.OwnOrgWideSubjects[0].Subject != "repo:acme/*" {
		t.Fatalf("own org-wide = %+v, want the deployed repo:acme/*", resp.OwnOrgWideSubjects)
	}
	if !resp.OwnOrgWideSubjects[0].DeployedOnly {
		t.Error("repo:acme/* is not in the configured list, so it must be marked DeployedOnly")
	}
}

// Without the state the role cannot be told apart from another project's, and
// the scan has to keep reporting it. Excluding it by name alone would hide a
// real conflict in an account where a DIFFERENT project kept the old name.
func TestScanStillReportsALegacyNamedRoleThisStateDoesNotHold(t *testing.T) {
	env, roles := legacyRoleAccount(t)

	resp := scanInWithState(t, t.TempDir(), env, nil, roles, callerIn(testConflictAccount),
		stateWithRoles("acme_backend_task_dev"))

	if got := conflictNames(resp); len(got) != 1 || got[0] != legacyGitHubRoleName {
		t.Errorf("conflicts = %v, want [%s]", got, legacyGitHubRoleName)
	}
}

func TestScanSaysSoWhenThisEnvironmentsStateIsUnreadable(t *testing.T) {
	env, roles := legacyRoleAccount(t)
	unreadable := func(context.Context, Env) (remoteStateSummary, error) {
		return remoteStateSummary{}, errors.New("AccessDenied: s3:GetObject")
	}

	resp := scanInWithState(t, t.TempDir(), env, nil, roles, callerIn(testConflictAccount), unreadable)

	wantDegraded(t, resp, githubOIDCReasonStateUnreadable)
	wantNotChecked(t, resp)
}

// The deploy decision for that same scan: the only org-wide subject lives on
// the deployed role, and the deploy is what removes it. Stopping here would
// keep the org-wide grant live.
func TestDeployedOnlyOrgWideSubjectDoesNotStopTheDeploy(t *testing.T) {
	env, roles := legacyRoleAccount(t)
	resp := scanInWithState(t, t.TempDir(), env, nil, roles, callerIn(testConflictAccount),
		stateWithRoles(legacyGitHubRoleName))

	for _, interactive := range []bool{true, false} {
		d := decideGithubOIDCConflicts(resp, interactive)
		if d.Prompt || d.Block {
			t.Errorf("interactive=%v: Prompt=%v Block=%v, want neither", interactive, d.Prompt, d.Block)
		}
		if d.Severity != githubOIDCSaysDeployedOrgWide {
			t.Errorf("interactive=%v: severity = %v, want githubOIDCSaysDeployedOrgWide", interactive, d.Severity)
		}
		if !strings.Contains(d.Report, `"repo:acme/*"`) || !strings.Contains(d.Report, "This deploy replaces") {
			t.Errorf("interactive=%v: report does not name the subject and say the deploy removes it:\n%s", interactive, d.Report)
		}
	}
}

// The configured list still carrying an org-wide subject is the original
// finding, and it still asks.
func TestConfiguredOrgWideSubjectStillAsks(t *testing.T) {
	resp := newGithubOIDCConflictsResponse()
	resp.OwnOrgWideSubjects = []githubOIDCOrgWideSubject{
		{Subject: "repo:acme/*", Org: "acme"},
		{Subject: "repo:old/*", Org: "old", DeployedOnly: true},
	}

	d := decideGithubOIDCConflicts(resp, true)
	if d.Severity != githubOIDCSaysOwnOrgWide || !d.Prompt {
		t.Errorf("severity=%v Prompt=%v, want the org-wide prompt", d.Severity, d.Prompt)
	}
	if !strings.Contains(d.Report, "Only the deployed role still trusts this") {
		t.Errorf("the deployed-only subject beside it is not marked as such:\n%s", d.Report)
	}
}

func TestSummarizeTerraformStateCollectsIAMRoleNames(t *testing.T) {
	state := []byte(`{
	  "version": 4,
	  "resources": [
	    {"module": "module.workloads", "mode": "managed", "type": "aws_iam_role", "name": "github_role",
	     "instances": [{"index_key": 0, "attributes": {"name": "GithubActionsRole"}}]},
	    {"module": "module.workloads", "mode": "managed", "type": "aws_iam_role", "name": "backend_task",
	     "instances": [{"attributes": {"name": "acme_backend_task_dev"}}]},
	    {"mode": "managed", "type": "aws_iam_policy", "name": "p",
	     "instances": [{"attributes": {"name": "not-a-role"}}]},
	    {"mode": "data", "type": "aws_iam_role", "name": "lookup",
	     "instances": [{"attributes": {"name": "someone-elses-role"}}]}
	  ]
	}`)

	s, err := summarizeTerraformState(state)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"GithubActionsRole": true, "acme_backend_task_dev": true}
	if len(s.IAMRoleNames) != len(want) {
		t.Errorf("IAMRoleNames = %v, want %v", s.IAMRoleNames, want)
	}
	for name := range want {
		if !s.IAMRoleNames[name] {
			t.Errorf("IAMRoleNames is missing %s", name)
		}
	}
}
