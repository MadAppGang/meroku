package main

// The deploy path's decision, tested without a terminal.
//
// decideGithubOIDCConflicts is a pure function of a scan response and one
// boolean precisely so these cases can be asserted directly: the interesting
// half of P5 is the policy, and a policy that can only be exercised through a
// TTY is a policy nobody tests.
//
// Every fixture is synthetic. Account IDs are 000000000000 throughout.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// conflictResponse is a scan that found one overlap, attributed by tags.
func conflictResponse() githubOIDCConflictsResponse {
	resp := newGithubOIDCConflictsResponse()
	resp.AccountID = testConflictAccount
	resp.RolesScanned = 12
	resp.OwnSubjects = []string{"repo:acme/api:*"}
	resp.Conflicts = []githubOIDCConflict{{
		RoleName:     "billing-prod-github-actions-role",
		RoleARN:      "arn:aws:iam::" + testConflictAccount + ":role/billing-prod-github-actions-role",
		OwnerProject: "billing",
		OwnerEnv:     "prod",
		Attribution:  githubOIDCAttributionTags,
		Overlaps: []githubOIDCOverlap{{
			OwnSubject:   "repo:acme/api:*",
			OtherSubject: "repo:acme/*:ref:refs/heads/main",
			Witness:      "repo:acme/api:ref:refs/heads/main",
		}},
	}}
	return resp
}

// ownUnrestrictedResponse is a completed scan that found one of OUR OWN roles
// trusting every repository on GitHub. Nothing is degraded: the scan worked
// perfectly and the answer is the worst one there is.
func ownUnrestrictedResponse() githubOIDCConflictsResponse {
	resp := newGithubOIDCConflictsResponse()
	resp.AccountID = testConflictAccount
	resp.RolesScanned = 12
	resp.OwnSubjects = []string{"repo:acme/api:*"}
	resp.ExcludedRoles = []string{"acme-dev-github-actions-role"}
	resp.OwnUnrestrictedRoles = []githubOIDCOwnUnrestrictedRole{{
		RoleName: "acme-dev-github-actions-role",
		RoleARN:  "arn:aws:iam::" + testConflictAccount + ":role/acme-dev-github-actions-role",
		Env:      "dev",
	}}
	return resp
}

// deniedResponse is the scan that could not finish: iam:ListRoles refused.
func deniedResponse() githubOIDCConflictsResponse {
	resp := newGithubOIDCConflictsResponse()
	resp.AccountID = testConflictAccount
	resp.OwnSubjects = []string{"repo:acme/api:*"}
	resp.degrade(githubOIDCReasonAccessDenied, "User is not authorized to perform iam:ListRoles")
	return resp
}

// disabledResponse is the not-applicable case: the feature is switched off.
func disabledResponse() githubOIDCConflictsResponse {
	resp := newGithubOIDCConflictsResponse()
	resp.degrade(githubOIDCReasonDisabled,
		"workload.enable_github_oidc is false, so this environment has no GitHub Actions role to overlap with")
	return resp
}

// cleanResponse is a completed scan that found nothing.
func cleanResponse() githubOIDCConflictsResponse {
	resp := newGithubOIDCConflictsResponse()
	resp.AccountID = testConflictAccount
	resp.RolesScanned = 128
	resp.OwnSubjects = []string{"repo:acme/api:*"}
	return resp
}

var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*m")

// plainText strips styling so an assertion is about the words rather than about
// whichever colour profile the test runner's stdout happened to negotiate.
func plainText(s string) string { return ansiEscape.ReplaceAllString(s, "") }

// ---------------------------------------------------------------------------
// 1. A found conflict, with no terminal to ask on
// ---------------------------------------------------------------------------

// The requirement in one test: a CI run that auto-continues past a confirmed
// conflict is the same as having no check at all, so there is no prompt and no
// discretion. Abort.
func TestDecideGithubOIDCConflictsNonInteractiveAborts(t *testing.T) {
	d := decideGithubOIDCConflicts(conflictResponse(), false)

	if !d.Block {
		t.Error("Block = false; a confirmed conflict with nobody to ask must abort")
	}
	if d.Prompt {
		t.Error("Prompt = true; there is no terminal to put a question on")
	}
	if d.Severity != githubOIDCSaysConflict {
		t.Errorf("Severity = %v, want conflict", d.Severity)
	}

	for _, want := range []string{
		"billing-prod-github-actions-role",
		`project "billing"`,
		`environment "prod"`,
		`"repo:acme/api:ref:refs/heads/main"`,
		"iam:PassRole",
		"ecs:UpdateService",
		"no terminal to ask on",
	} {
		if !strings.Contains(d.Report, want) {
			t.Errorf("report is missing %q\n---\n%s", want, d.Report)
		}
	}
}

// The same case end to end, through the seams, to prove the prompt is not
// merely defaulted to no but never reached.
func TestCheckGithubOIDCSubjectConflictsNeverPromptsWithoutATerminal(t *testing.T) {
	asked := false
	stubGithubOIDCCLI(t, conflictResponse(), false, func() bool {
		asked = true
		return true // would continue, if anything ever called it
	})

	var block bool
	out := captureOutput(t, func() {
		block = checkGithubOIDCSubjectConflicts(context.Background(), conflictEnv("repo:acme/api:*"))
	})

	if asked {
		t.Error("the confirmation prompt was reached with no terminal to answer it")
	}
	if !block {
		t.Error("block = false; a confirmed conflict in a non-interactive run must stop the deploy")
	}
	if !strings.Contains(plainText(out), "billing-prod-github-actions-role") {
		t.Errorf("the conflicting role was not named:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// 2. A found conflict, with somebody to ask
// ---------------------------------------------------------------------------

func TestDecideGithubOIDCConflictsInteractiveAsks(t *testing.T) {
	d := decideGithubOIDCConflicts(conflictResponse(), true)

	if !d.Prompt {
		t.Error("Prompt = false; a found conflict must be confirmed before continuing")
	}
	if d.Block {
		t.Error("Block = true before the question was put; the answer decides this one")
	}
	if strings.Contains(d.Report, "no terminal") {
		t.Errorf("the non-interactive wording leaked into an interactive report:\n%s", d.Report)
	}
}

// Declining is the default and it stops the deploy; accepting continues.
func TestCheckGithubOIDCSubjectConflictsHonoursTheAnswer(t *testing.T) {
	for _, tc := range []struct {
		name      string
		answer    bool
		wantBlock bool
	}{
		{"declined", false, true},
		{"accepted", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			asked := false
			stubGithubOIDCCLI(t, conflictResponse(), true, func() bool {
				asked = true
				return tc.answer
			})

			var block bool
			captureOutput(t, func() {
				block = checkGithubOIDCSubjectConflicts(context.Background(), conflictEnv("repo:acme/api:*"))
			})

			if !asked {
				t.Fatal("the confirmation prompt was never reached")
			}
			if block != tc.wantBlock {
				t.Errorf("block = %t, want %t", block, tc.wantBlock)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 2b. Our own role trusting all of GitHub: a hard block, with nothing to ask
// ---------------------------------------------------------------------------

// The asymmetry, stated as a test. A cross-project overlap CAN be intentional
// and meroku cannot tell, so it asks. A role that trusts every repository on
// GitHub never is, so it does not ask — and it does not consult interactivity
// at all, because the answer to "is there a terminal" cannot change the answer
// to "should this deploy".
func TestDecideGithubOIDCConflictsOwnUnrestrictedBlocksEitherWay(t *testing.T) {
	for _, interactive := range []bool{true, false} {
		t.Run(fmt.Sprintf("interactive=%t", interactive), func(t *testing.T) {
			d := decideGithubOIDCConflicts(ownUnrestrictedResponse(), interactive)

			if !d.Block {
				t.Error("Block = false; our own role trusting all of GitHub is never deployable")
			}
			if d.Prompt {
				t.Error("Prompt = true; there is no question to put — no prompt, no override")
			}
			if d.Severity != githubOIDCSaysOwnUnrestricted {
				t.Errorf("Severity = %v, want own-unrestricted, the top of the scale", d.Severity)
			}

			for _, want := range []string{
				"acme-dev-github-actions-role",
				`environment "dev"`,
				"arn:aws:iam::" + testConflictAccount + ":role/acme-dev-github-actions-role",
				"any repository on GitHub",
				"iam:PassRole",
				"ECR push",
				"ecs:UpdateService",
				"no override",
			} {
				if !strings.Contains(d.Report, want) {
					t.Errorf("report is missing %q\n---\n%s", want, d.Report)
				}
			}
		})
	}
}

// End to end through the seams, which is the assertion that matters: the
// confirmation is not merely defaulted to no, it is never reached. A prompt
// offered here would make an unassailable refusal look like a preference.
func TestCheckGithubOIDCSubjectConflictsOwnUnrestrictedNeverAsks(t *testing.T) {
	asked := false
	stubGithubOIDCCLI(t, ownUnrestrictedResponse(), true, func() bool {
		asked = true
		return true // would continue, if anything ever called it
	})

	var block bool
	out := captureOutput(t, func() {
		block = checkGithubOIDCSubjectConflicts(context.Background(), conflictEnv("repo:acme/api:*"))
	})

	if asked {
		t.Error("a confirmation was offered for a role that trusts all of GitHub; there is nothing to confirm")
	}
	if !block {
		t.Error("block = false; the deploy must stop")
	}
	if !strings.Contains(plainText(out), "acme-dev-github-actions-role") {
		t.Errorf("the wide-open role was not named:\n%s", out)
	}
}

// Both findings at once. The own-unrestricted block outranks the conflict path
// entirely: no prompt is offered even though a conflict alone would have
// offered one, because the deploy is already refused.
func TestDecideGithubOIDCConflictsOwnUnrestrictedOutranksAConflict(t *testing.T) {
	both := ownUnrestrictedResponse()
	both.Conflicts = conflictResponse().Conflicts

	d := decideGithubOIDCConflicts(both, true)

	if !d.Block {
		t.Error("Block = false; the own unrestricted role decides this on its own")
	}
	if d.Prompt {
		t.Error("Prompt = true; the conflict path's confirmation must not survive a hard block")
	}
	if d.Severity != githubOIDCSaysOwnUnrestricted {
		t.Errorf("Severity = %v, want own-unrestricted", d.Severity)
	}
	// The lesser finding is still disclosed, just not as the reason.
	if !strings.Contains(d.Report, "billing-prod-github-actions-role") {
		t.Errorf("the conflict found in the same scan was dropped from the report:\n%s", d.Report)
	}

	// The regression control: the same conflict, without the own unrestricted
	// role, still asks. Adding a hard block must not have turned the
	// cross-project path into one.
	only := decideGithubOIDCConflicts(conflictResponse(), true)
	if !only.Prompt || only.Block {
		t.Errorf("conflict alone: Prompt=%t Block=%t, want a question and no verdict yet", only.Prompt, only.Block)
	}
	if only.Severity != githubOIDCSaysConflict {
		t.Errorf("conflict alone: Severity = %v, want conflict", only.Severity)
	}
}

// The same end to end, with a terminal available and a conflict alongside: the
// prompt seam is never touched.
func TestCheckGithubOIDCSubjectConflictsOwnUnrestrictedWithConflictNeverAsks(t *testing.T) {
	both := ownUnrestrictedResponse()
	both.Conflicts = conflictResponse().Conflicts

	asked := false
	stubGithubOIDCCLI(t, both, true, func() bool {
		asked = true
		return true
	})

	var block bool
	captureOutput(t, func() {
		block = checkGithubOIDCSubjectConflicts(context.Background(), conflictEnv("repo:acme/api:*"))
	})

	if asked {
		t.Error("a confirmation was offered while a hard block was in force")
	}
	if !block {
		t.Error("block = false; a hard block must reach the deploy path")
	}
}

// ---------------------------------------------------------------------------
// 2c. An org-wide subject of ours: a question, not a refusal
// ---------------------------------------------------------------------------

// orgWideResponse is a completed scan whose own subject accepts a whole
// organisation the user chose. Nothing degraded.
func orgWideResponse() githubOIDCConflictsResponse {
	resp := newGithubOIDCConflictsResponse()
	resp.AccountID = testConflictAccount
	resp.RolesScanned = 12
	resp.OwnSubjects = []string{"repo:acme/*"}
	resp.OwnOrgWideSubjects = []githubOIDCOrgWideSubject{{
		Subject: "repo:acme/*",
		Org:     "acme",
	}}
	return resp
}

// shippedDefaultResponse is the same tier reached the other way: the config was
// never edited, so the subject is meroku's own default and the organisation it
// trusts belongs to somebody else entirely.
func shippedDefaultResponse() githubOIDCConflictsResponse {
	resp := newGithubOIDCConflictsResponse()
	resp.AccountID = testConflictAccount
	resp.RolesScanned = 12
	resp.OwnSubjects = []string{githubOIDCShippedDefaultSubject}
	resp.OwnOrgWideSubjects = []githubOIDCOrgWideSubject{{
		Subject:        githubOIDCShippedDefaultSubject,
		Org:            "MadAppGang",
		ShippedDefault: true,
	}}
	return resp
}

// The tier's defining property, and the one place it differs from
// own-unrestricted: it asks. An org-wide subject can be what somebody meant, so
// spending a hard block on it would teach people to route around the check.
func TestDecideGithubOIDCConflictsOrgWideAsksWhenInteractive(t *testing.T) {
	d := decideGithubOIDCConflicts(orgWideResponse(), true)

	if !d.Prompt {
		t.Error("Prompt = false; an org-wide subject can be deliberate, so it is a question")
	}
	if d.Block {
		t.Error("Block = true; the question has not been answered yet")
	}
	if d.Severity != githubOIDCSaysOwnOrgWide {
		t.Errorf("Severity = %v, want own-org-wide", d.Severity)
	}
	for _, want := range []string{
		`"repo:acme/*"`,
		`the "acme" organisation`,
		"iam:PassRole",
		"ECR push",
		"ecs:UpdateService",
		"github_oidc_subjects",
	} {
		if !strings.Contains(d.Report, want) {
			t.Errorf("report is missing %q\n---\n%s", want, d.Report)
		}
	}
	// This is not the third-party case, and must not borrow its words.
	if strings.Contains(d.Report, "MadAppGang") {
		t.Errorf("a subject the user chose was described as meroku's default:\n%s", d.Report)
	}
}

// No terminal, so nothing can be asked — and a CI run that auto-continues past
// a finding is the same as having no check at all.
func TestDecideGithubOIDCConflictsOrgWideAbortsWithoutATerminal(t *testing.T) {
	d := decideGithubOIDCConflicts(orgWideResponse(), false)

	if !d.Block {
		t.Error("Block = false; there is nobody to ask, so the deploy stops")
	}
	if d.Prompt {
		t.Error("Prompt = true; there is no terminal to put the question on")
	}
	if !strings.Contains(d.Report, "no terminal") {
		t.Errorf("the report does not say why no question was put:\n%s", d.Report)
	}
}

// The important case. The subject is meroku's untouched default, so the
// organisation being trusted is a third party's — and the copy has to say that
// rather than "every repository in your organisation", which is the reassuring
// misreading.
func TestDecideGithubOIDCConflictsOrgWideShippedDefaultNamesTheThirdParty(t *testing.T) {
	d := decideGithubOIDCConflicts(shippedDefaultResponse(), true)

	if d.Severity != githubOIDCSaysOwnOrgWide {
		t.Errorf("Severity = %v, want own-org-wide", d.Severity)
	}
	if !d.Prompt || d.Block {
		t.Errorf("Prompt=%t Block=%t, want a question; the shipped default is still the confirmation tier",
			d.Prompt, d.Block)
	}
	for _, want := range []string{
		"meroku's default",
		"third-party",
		"MadAppGang",
		"YOUR AWS account",
	} {
		if !strings.Contains(d.Report, want) {
			t.Errorf("report is missing %q — this is the case that matters\n---\n%s", want, d.Report)
		}
	}
}

// Ranking, asserted from both sides. Own-unrestricted swallows an org-wide
// subject found in the same scan, and an org-wide subject outranks a
// cross-project conflict.
func TestDecideGithubOIDCConflictsOrgWideRanking(t *testing.T) {
	t.Run("own unrestricted outranks it", func(t *testing.T) {
		both := ownUnrestrictedResponse()
		both.OwnOrgWideSubjects = orgWideResponse().OwnOrgWideSubjects

		d := decideGithubOIDCConflicts(both, true)

		if d.Severity != githubOIDCSaysOwnUnrestricted {
			t.Errorf("Severity = %v, want own-unrestricted at the top of the scale", d.Severity)
		}
		if !d.Block || d.Prompt {
			t.Errorf("Block=%t Prompt=%t; a hard block must not be softened into a question by a lesser finding",
				d.Block, d.Prompt)
		}
	})

	t.Run("it outranks a cross-project conflict", func(t *testing.T) {
		both := orgWideResponse()
		both.Conflicts = conflictResponse().Conflicts

		d := decideGithubOIDCConflicts(both, true)

		if d.Severity != githubOIDCSaysOwnOrgWide {
			t.Errorf("Severity = %v, want own-org-wide", d.Severity)
		}
		if !d.Prompt || d.Block {
			t.Errorf("Prompt=%t Block=%t, want a question", d.Prompt, d.Block)
		}
		// The lesser finding is still disclosed, just not as the reason.
		if !strings.Contains(d.Report, "billing-prod-github-actions-role") {
			t.Errorf("the conflict found in the same scan was dropped from the report:\n%s", d.Report)
		}
	})
}

// End to end through the seams. The org-wide tier reaches a confirmation, uses
// the one whose words describe it, and honours the answer either way.
func TestCheckGithubOIDCSubjectConflictsOrgWideHonoursTheAnswer(t *testing.T) {
	for _, tc := range []struct {
		name      string
		answer    bool
		wantBlock bool
	}{
		{"declined", false, true},
		{"accepted", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			asked := false
			stubGithubOIDCCLI(t, orgWideResponse(), true, func() bool {
				asked = true
				return tc.answer
			})

			var block bool
			out := captureOutput(t, func() {
				block = checkGithubOIDCSubjectConflicts(context.Background(), conflictEnv("repo:acme/*"))
			})

			if !asked {
				t.Fatal("the confirmation was never reached; an org-wide subject is a question")
			}
			if block != tc.wantBlock {
				t.Errorf("block = %t, want %t", block, tc.wantBlock)
			}
			if !strings.Contains(plainText(out), "repo:acme/*") {
				t.Errorf("the org-wide subject was not named:\n%s", out)
			}
		})
	}
}

// The routing itself, which is the part a shared stub cannot see: each tier gets
// the confirmation whose words describe it. A prompt that says "these subjects
// reach another project's role" is false of an org-wide subject, and a
// confirmation that misdescribes what is being accepted is answered anyway.
func TestGithubOIDCConfirmForRoutesByTier(t *testing.T) {
	orgWide := githubOIDCConfirmFor(githubOIDCSaysOwnOrgWide)
	conflict := githubOIDCConfirmFor(githubOIDCSaysConflict)

	// Without this the two assertions below are vacuous: if both seams happened
	// to compile to one code pointer, every routing would satisfy them.
	if fmt.Sprintf("%p", confirmGithubOIDCOrgWideSubject) == fmt.Sprintf("%p", confirmGithubOIDCOverlap) {
		t.Fatal("the two confirmations are the same function, so this test proves nothing")
	}

	if fmt.Sprintf("%p", orgWide) != fmt.Sprintf("%p", confirmGithubOIDCOrgWideSubject) {
		t.Error("the org-wide tier was routed to the overlap confirmation, which describes a different finding")
	}
	if fmt.Sprintf("%p", conflict) != fmt.Sprintf("%p", confirmGithubOIDCOverlap) {
		t.Error("the conflict tier was routed away from the overlap confirmation")
	}
}

// ---------------------------------------------------------------------------
// 3. A scan that failed is advisory, and says so unmistakably
// ---------------------------------------------------------------------------

// AccessDenied on iam:ListRoles must never stop a deploy — that is the contract
// resolveGithubOIDCForEnv has documented since it was written. But it must not
// be quiet either: the provider check's ✅ sits two lines above, and an empty
// conflicts array read as reassurance is the worst thing this feature can ship.
func TestDecideGithubOIDCConflictsFailedScanIsAdvisory(t *testing.T) {
	d := decideGithubOIDCConflicts(deniedResponse(), false)

	if d.Block {
		t.Error("Block = true; a diagnostic that could not run must never refuse a deploy")
	}
	if d.Prompt {
		t.Error("Prompt = true; there is nothing to confirm, only something to report")
	}
	if d.Severity != githubOIDCSaysUnverified {
		t.Errorf("Severity = %v, want unverified", d.Severity)
	}
	if !strings.Contains(strings.ToLower(d.Report), "could not verify") {
		t.Errorf("report does not say it could not verify:\n%s", d.Report)
	}
	if !strings.Contains(d.Report, githubOIDCReasonAccessDenied) {
		t.Errorf("report does not name the reason:\n%s", d.Report)
	}
}

// The same, non-blocking, through the whole CLI path.
func TestCheckGithubOIDCSubjectConflictsFailedScanDoesNotBlock(t *testing.T) {
	stubGithubOIDCCLI(t, deniedResponse(), true, func() bool {
		t.Error("a failed scan must not ask anything")
		return false
	})

	var block bool
	out := captureOutput(t, func() {
		block = checkGithubOIDCSubjectConflicts(context.Background(), conflictEnv("repo:acme/api:*"))
	})

	if block {
		t.Error("block = true; a failed scan must never stop a deploy")
	}
	if !strings.Contains(strings.ToLower(plainText(out)), "could not verify") {
		t.Errorf("the deploy printed no could-not-verify line:\n%s", out)
	}
}

// A role fenced by claims this scan cannot intersect is not a clean bill of
// health either, even though it is nobody's conflict.
func TestDecideGithubOIDCConflictsUnevaluatedRolesAreNotClean(t *testing.T) {
	resp := cleanResponse()
	resp.UnevaluatedRoles = []githubOIDCUnevaluatedRole{{
		RoleName:  "svc-deploy-role",
		Reason:    githubOIDCUnevaluatedOtherClaims,
		ClaimKeys: []string{"repository_owner"},
	}}
	resp.degrade(githubOIDCReasonOtherClaims, "1 role restricts by claims other than sub")

	d := decideGithubOIDCConflicts(resp, true)

	if d.Block || d.Prompt {
		t.Errorf("Block=%t Prompt=%t; an unevaluated role is advisory", d.Block, d.Prompt)
	}
	if d.Severity != githubOIDCSaysUnverified {
		t.Errorf("Severity = %v, want unverified", d.Severity)
	}
}

// ---------------------------------------------------------------------------
// 4. Not-applicable is silent
// ---------------------------------------------------------------------------

// An environment with OIDC switched off, or with no subjects to compare, must
// not emit a warning on every single deploy. A banner that is always there is a
// banner nobody reads, and then it is not there for the real case.
func TestDecideGithubOIDCConflictsNotApplicableSaysNothing(t *testing.T) {
	noSubjects := newGithubOIDCConflictsResponse()
	noSubjects.degrade(githubOIDCReasonNoSubjects, "nothing to compare")

	for name, resp := range map[string]githubOIDCConflictsResponse{
		"oidc_disabled": disabledResponse(),
		"no_subjects":   noSubjects,
	} {
		t.Run(name, func(t *testing.T) {
			d := decideGithubOIDCConflicts(resp, true)

			if d.Block || d.Prompt {
				t.Errorf("Block=%t Prompt=%t; a scan that did not need to run decides nothing", d.Block, d.Prompt)
			}
			if d.Severity != githubOIDCSaysNothing {
				t.Errorf("Severity = %v, want nothing", d.Severity)
			}
			if d.Report != "" {
				t.Errorf("a not-applicable scan printed something:\n%s", d.Report)
			}
		})
	}
}

// And nothing reaches stdout either.
func TestCheckGithubOIDCSubjectConflictsSilentWhenNotApplicable(t *testing.T) {
	stubGithubOIDCCLI(t, disabledResponse(), true, func() bool {
		t.Error("a not-applicable scan must not ask anything")
		return false
	})

	var block bool
	out := captureOutput(t, func() {
		block = checkGithubOIDCSubjectConflicts(context.Background(), conflictEnv())
	})

	if block {
		t.Error("block = true; oidc_disabled decides nothing")
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("oidc_disabled printed on a deploy:\n%q", out)
	}
}

// The partition is read from the scan's own Kind, never from a second copy of
// the reason list living here. A reason added to the backend and classified as
// not-applicable must go quiet without this file being touched.
func TestDecideGithubOIDCConflictsPartitionsByKind(t *testing.T) {
	for reason, wantSilent := range map[string]bool{
		githubOIDCReasonDisabled:          true,
		githubOIDCReasonNoSubjects:        true,
		githubOIDCReasonAccessDenied:      false,
		githubOIDCReasonPagination:        false,
		githubOIDCReasonAWSError:          false,
		"a_reason_nobody_has_written_yet": false,
	} {
		resp := newGithubOIDCConflictsResponse()
		resp.degrade(reason, "")

		d := decideGithubOIDCConflicts(resp, true)
		gotSilent := d.Report == ""
		if gotSilent != wantSilent {
			t.Errorf("%s: silent = %t, want %t (kind %s)",
				reason, gotSilent, wantSilent, githubOIDCDegraded{Reason: reason}.Kind())
		}
	}
}

// ---------------------------------------------------------------------------
// 5. A clean scan
// ---------------------------------------------------------------------------

func TestDecideGithubOIDCConflictsCleanScan(t *testing.T) {
	d := decideGithubOIDCConflicts(cleanResponse(), true)

	if d.Block || d.Prompt {
		t.Errorf("Block=%t Prompt=%t on a clean scan", d.Block, d.Prompt)
	}
	if d.Severity != githubOIDCSaysClear {
		t.Errorf("Severity = %v, want clear", d.Severity)
	}
	if !strings.Contains(d.Report, "128 roles") {
		t.Errorf("the clean line does not say what was scanned:\n%s", d.Report)
	}
}

// ---------------------------------------------------------------------------
// 6. The witness
// ---------------------------------------------------------------------------

// An empty witness is legitimate — "*" against "*" intersects on every string —
// and it must never render as a blank gap in a sentence that reads "a token
// whose sub is  assumes both roles".
func TestGithubOIDCWitnessLabelRendersAnEmptyWitness(t *testing.T) {
	if got := githubOIDCWitnessLabel(""); got != "<any subject>" {
		t.Errorf("githubOIDCWitnessLabel(\"\") = %q, want %q", got, "<any subject>")
	}

	resp := conflictResponse()
	resp.Conflicts[0].Overlaps[0].Witness = ""

	d := decideGithubOIDCConflicts(resp, false)
	if !strings.Contains(d.Report, "sub is <any subject> assumes both roles") {
		t.Errorf("an empty witness left a gap in the sentence:\n%s", d.Report)
	}
}

// A role that pins no GitHub claim at all is the loudest finding available and
// conflicts with no overlap pair to quote.
func TestDecideGithubOIDCConflictsUnrestrictedRole(t *testing.T) {
	resp := newGithubOIDCConflictsResponse()
	resp.Conflicts = []githubOIDCConflict{{
		RoleName:     "legacy-deploy-role",
		Attribution:  githubOIDCAttributionUntagged,
		Unrestricted: true,
		Overlaps:     []githubOIDCOverlap{},
	}}

	d := decideGithubOIDCConflicts(resp, false)

	if !d.Block {
		t.Error("Block = false; an unrestricted role in a non-interactive run must abort")
	}
	if !strings.Contains(d.Report, "every repository on GitHub") {
		t.Errorf("the unrestricted role was not called out:\n%s", d.Report)
	}
	if !strings.Contains(d.Report, "no meroku tags") {
		t.Errorf("an untagged owner was not distinguished from an unreadable one:\n%s", d.Report)
	}
}

// A conflict found on a walk that then failed still blocks, and says there may
// be more.
func TestDecideGithubOIDCConflictsPartialWalkStillBlocks(t *testing.T) {
	resp := conflictResponse()
	resp.degrade(githubOIDCReasonAccessDenied, "denied on page two")

	d := decideGithubOIDCConflicts(resp, false)

	if !d.Block {
		t.Error("Block = false; conflicts found before a failure are still conflicts")
	}
	if !strings.Contains(d.Report, "did not finish") {
		t.Errorf("the partial walk was not disclosed:\n%s", d.Report)
	}
}

// ---------------------------------------------------------------------------
// 7. Regression: Regenerate still means exactly what the old bool meant
// ---------------------------------------------------------------------------

// resolveGithubOIDCForEnv used to return one bool, "the YAML changed, so
// regenerate". Widening it to githubOIDCOutcome must not have altered that
// half by a single case. These four are the whole of the provider path.
func TestResolveGithubOIDCForEnvRegenerateIsUnchanged(t *testing.T) {
	tests := []struct {
		name           string
		iamClient      *fakeIAM
		lookup         remoteStateLookup
		createProvider string // the value already in the YAML
		wantRegenerate bool
		wantWritten    string // the value expected in the YAML afterwards
	}{
		{
			// Owns it and it exists: still creates it, so nothing changes.
			name:           "already correct writes nothing",
			iamClient:      providerFound(),
			lookup:         stateOwning(githubOIDCResourceType),
			createProvider: "true",
			wantRegenerate: false,
			wantWritten:    "github_oidc_create_provider: true",
		},
		{
			// Exists but another project's state owns it: federate instead.
			name:           "a changed decision is written back",
			iamClient:      providerFound(),
			lookup:         stateOwning(),
			createProvider: "true",
			wantRegenerate: true,
			wantWritten:    "github_oidc_create_provider: false",
		},
		{
			// First project in the account.
			name:           "absent provider flips back to creating it",
			iamClient:      providerAbsent(),
			lookup:         stateOwning(),
			createProvider: "false",
			wantRegenerate: true,
			wantWritten:    "github_oidc_create_provider: true",
		},
		{
			// A read that failed is not evidence, so nothing is written.
			name:           "an unreadable state writes nothing",
			iamClient:      providerFound(),
			lookup:         stateFailing(errStateUnreadableForTest),
			createProvider: "true",
			wantRegenerate: false,
			wantWritten:    "github_oidc_create_provider: true",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeOIDCEnvYAML(t, dir, "dev", tc.createProvider)
			chdir(t, dir)
			stubGithubOIDCProvider(t, tc.iamClient, tc.lookup)
			stubGithubOIDCConflictCheck(t, false)

			e := oidcCLIEnv(tc.createProvider)
			var outcome githubOIDCOutcome
			captureOutput(t, func() {
				outcome = resolveGithubOIDCForEnv(context.Background(), "dev", &e)
			})

			if outcome.Regenerate != tc.wantRegenerate {
				t.Errorf("Regenerate = %t, want %t", outcome.Regenerate, tc.wantRegenerate)
			}
			if outcome.Block {
				t.Error("Block = true with no conflict reported")
			}

			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if !strings.Contains(string(body), tc.wantWritten) {
				t.Errorf("%s.yaml does not contain %q:\n%s", "dev", tc.wantWritten, body)
			}
		})
	}
}

// OIDC switched off is still the zero outcome, and still costs nothing: the
// early return has to come before the scan, or an environment that does not use
// the feature pays for a paginated ListRoles on every deploy.
func TestResolveGithubOIDCForEnvSkipsEverythingWhenDisabled(t *testing.T) {
	scanned := false
	stubGithubOIDCProvider(t, providerFound(), stateOwning())
	restore := runGithubOIDCConflictCheck
	runGithubOIDCConflictCheck = func(context.Context, Env) bool {
		scanned = true
		return true
	}
	t.Cleanup(func() { runGithubOIDCConflictCheck = restore })

	e := oidcCLIEnv("true")
	e.Workload.EnableGithubOIDC = false

	var outcome githubOIDCOutcome
	out := captureOutput(t, func() {
		outcome = resolveGithubOIDCForEnv(context.Background(), "dev", &e)
	})

	if outcome != (githubOIDCOutcome{}) {
		t.Errorf("outcome = %+v, want the zero value", outcome)
	}
	if scanned {
		t.Error("the subject scan ran for an environment with OIDC switched off")
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("a disabled environment printed on a deploy:\n%q", out)
	}
}

// The new half: a blocked scan reaches the caller through Block, and it does so
// without disturbing Regenerate.
func TestResolveGithubOIDCForEnvCarriesTheBlock(t *testing.T) {
	dir := t.TempDir()
	writeOIDCEnvYAML(t, dir, "dev", "true")
	chdir(t, dir)
	stubGithubOIDCProvider(t, providerFound(), stateOwning())
	stubGithubOIDCConflictCheck(t, true)

	e := oidcCLIEnv("true")
	var outcome githubOIDCOutcome
	captureOutput(t, func() {
		outcome = resolveGithubOIDCForEnv(context.Background(), "dev", &e)
	})

	if !outcome.Block {
		t.Error("Block = false; the scan's refusal did not reach the deploy path")
	}
	if !outcome.Regenerate {
		t.Error("Regenerate = false; a blocked deploy must not swallow the provider write")
	}
}

// ---------------------------------------------------------------------------
// Test plumbing
// ---------------------------------------------------------------------------

var errStateUnreadableForTest = errors.New("the state bucket is unreachable")

// oidcCLIEnv is the environment the CLI tests deploy.
func oidcCLIEnv(createProvider string) Env {
	e := conflictEnv("repo:acme/api:*")
	e.Workload.EnableGithubOIDC = true
	value := createProvider == "true"
	e.Workload.GithubOIDCCreateProvider = &value
	return e
}

// writeOIDCEnvYAML writes an environment at the current schema version, so
// loading it can never trigger a migration rewrite and any change to the file
// is unambiguously the provider check's doing.
func writeOIDCEnvYAML(t *testing.T, dir, name, createProvider string) string {
	t.Helper()
	body := fmt.Sprintf(`schema_version: %d
project: acme
env: %s
account_id: "%s"
region: us-east-1
workload:
  enable_github_oidc: true
  github_oidc_create_provider: %s
  github_oidc_subjects:
    - "repo:acme/api:*"
`, CurrentSchemaVersion, name, testConflictAccount, createProvider)

	path := filepath.Join(dir, name+".yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// stubGithubOIDCProvider points the provider check at a fake IAM and a scripted
// state lookup, restoring both afterwards.
func stubGithubOIDCProvider(t *testing.T, client iamOIDCReader, lookup remoteStateLookup) {
	t.Helper()
	origReader, origLookup := newGithubOIDCReader, lookupGithubOIDCRemoteState
	newGithubOIDCReader = func(context.Context, string) (iamOIDCReader, error) { return client, nil }
	lookupGithubOIDCRemoteState = lookup
	t.Cleanup(func() {
		newGithubOIDCReader = origReader
		lookupGithubOIDCRemoteState = origLookup
	})
}

// stubGithubOIDCConflictCheck replaces the subject scan with a fixed verdict.
func stubGithubOIDCConflictCheck(t *testing.T, block bool) {
	t.Helper()
	orig := runGithubOIDCConflictCheck
	runGithubOIDCConflictCheck = func(context.Context, Env) bool { return block }
	t.Cleanup(func() { runGithubOIDCConflictCheck = orig })
}

// stubGithubOIDCCLI scripts the three seams checkGithubOIDCSubjectConflicts
// reaches through: the scan, the terminal check, and the confirmation.
func stubGithubOIDCCLI(t *testing.T, resp githubOIDCConflictsResponse, interactive bool, confirm func() bool) {
	t.Helper()
	origScan, origTerm := scanGithubOIDCConflictsForCLI, terminalIsInteractive
	origConfirm, origOrgWide := confirmGithubOIDCOverlap, confirmGithubOIDCOrgWideSubject

	scanGithubOIDCConflictsForCLI = func(context.Context, Env) githubOIDCConflictsResponse { return resp }
	terminalIsInteractive = func() bool { return interactive }
	// Both prompt seams get the same stub, so `asked` in a caller means "a
	// confirmation was reached" whichever tier reached it. A test that cares
	// which one was used asserts on the tier's severity instead.
	confirmGithubOIDCOverlap = confirm
	confirmGithubOIDCOrgWideSubject = confirm

	t.Cleanup(func() {
		scanGithubOIDCConflictsForCLI = origScan
		terminalIsInteractive = origTerm
		confirmGithubOIDCOverlap = origConfirm
		confirmGithubOIDCOrgWideSubject = origOrgWide
	})
}
