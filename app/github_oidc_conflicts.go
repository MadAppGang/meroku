package main

// Finding two projects in one AWS account whose GitHub Actions roles trust the
// same repositories.
//
// A project's role is fenced off from every other project by exactly one thing:
// the sub condition on its trust policy. Nothing stops two projects claiming
// overlapping subjects — both applies succeed and AWS raises nothing — so a
// workflow in one repository can assume both roles, and the second role grants
// iam:PassRole over its task roles, ECR push and ecs:UpdateService. The evidence
// is in AWS, never in YAML: the conflicting project lives in a checkout meroku
// has never seen.
//
// The whole feature turns on one invariant, and every structural decision below
// serves it:
//
//	checked is true ONLY if the asserted account was scanned to completion and
//	every candidate role and subject was evaluated or safely excluded.
//
// The worst thing this code can ship is an empty conflicts array rendered as
// reassurance. AccessDenied on page two of the walk, a policy that would not
// parse, a subject too long to compare, a role fenced by a claim this scan
// cannot read — every one of them hides exactly as much as a real conflict
// would. So checked is never assigned. It is derived, at marshal time, from the
// degraded list: append a scan-incomplete reason anywhere and the response
// cannot claim to be checked. There is no field to set wrongly.
//
// Degraded reasons are partitioned in two, because "the scan did not need to
// run" and "the scan could not finish" are different messages. Both leave
// checked false — neither verified anything — and the partition decides only
// how loudly the UI says so:
//
//	not_applicable    oidc_disabled, no_subjects — the UI stays silent
//	scan_incomplete   everything else            — the UI goes yellow
//
// Conflicts found before a failure are always preserved. checked:false with a
// non-empty conflicts array is a normal response, and it is red.
//
// Two findings are neither conflicts nor degradations, because a conflict
// relates two roles and both of these are properties of one configuration —
// ours. Each rides in its own array and each leaves checked alone: the scan saw
// everything, and what it saw was bad.
//
//	githubOIDCOwnUnrestrictedRole  one of OUR OWN roles pins no GitHub claim at
//	                              all, so every repository on GitHub can assume
//	                              it. The worst state available.
//	githubOIDCOrgWideSubject      one of OUR OWN subjects wildcards the
//	                              repository segment, so it accepts an entire
//	                              organisation. One tier below, and the tier the
//	                              shipped default landed in.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// ---------------------------------------------------------------------------
// Ports
// ---------------------------------------------------------------------------

// iamRoleLister is the slice of the IAM API this scan uses. An interface so the
// tests never reach AWS, following iamOIDCReader in app/github_oidc.go.
//
// ListRoleTags is on the same port as ListRoles deliberately: they are the same
// credentials against the same account, and separating them would suggest the
// tag read is part of the verdict. It is not — see attribution below.
type iamRoleLister interface {
	ListRoles(ctx context.Context, in *iam.ListRolesInput, optFns ...func(*iam.Options)) (*iam.ListRolesOutput, error)
	ListRoleTags(ctx context.Context, in *iam.ListRoleTagsInput, optFns ...func(*iam.Options)) (*iam.ListRoleTagsOutput, error)
}

// callerIdentityReader asks AWS which account these credentials belong to.
//
// Its only caller asserts the answer against env.AccountID. Without that
// assertion the scan can list a completely different account — the server's
// ambient AWS_PROFILE — and report a confident, meaningless "no conflicts".
type callerIdentityReader interface {
	GetCallerIdentity(ctx context.Context, in *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// ---------------------------------------------------------------------------
// Vocabulary
// ---------------------------------------------------------------------------

const (
	// Not applicable: the scan did not need to run. The UI is silent.
	githubOIDCReasonDisabled   = "oidc_disabled"
	githubOIDCReasonNoSubjects = "no_subjects"

	// Scan incomplete: something was not evaluated. The UI is yellow.
	githubOIDCReasonNoAccountID     = "no_account_id"
	githubOIDCReasonNoCredentials   = "no_credentials"
	githubOIDCReasonWrongAccount    = "wrong_account"
	githubOIDCReasonAccessDenied    = "access_denied"
	githubOIDCReasonThrottled       = "throttled"
	githubOIDCReasonTimeout         = "timeout"
	githubOIDCReasonEnvUnreadable   = "env_unreadable"
	githubOIDCReasonUnparseable     = "unparseable_policy"
	githubOIDCReasonPatternTooLong  = "pattern_too_long"
	githubOIDCReasonPagination      = "pagination_incomplete"
	githubOIDCReasonOtherClaims     = "unevaluatable_claims"
	githubOIDCReasonBudgetExhausted = "pair_budget_exhausted"
	// githubOIDCReasonAWSError is the catch-all for an AWS failure that is
	// neither a denial, a throttle nor a deadline. It exists so an unclassified
	// error is never reported as one of the specific reasons: calling a network
	// partition "access_denied" would send the user to rewrite an IAM policy
	// that was never the problem.
	githubOIDCReasonAWSError = "aws_error"
)

const (
	githubOIDCDegradedNotApplicable  = "not_applicable"
	githubOIDCDegradedScanIncomplete = "scan_incomplete"
)

// githubOIDCNotApplicableReasons is the entire not-applicable partition. Every
// other reason is scan-incomplete, so a reason added later without being
// classified degrades safely: it makes checked false rather than hiding.
var githubOIDCNotApplicableReasons = map[string]bool{
	githubOIDCReasonDisabled:   true,
	githubOIDCReasonNoSubjects: true,
}

const (
	// githubOIDCAttributionTags means both meroku tags were read off the role.
	githubOIDCAttributionTags = "tags"
	// githubOIDCAttributionUntagged means the tag read succeeded and the role
	// carries no meroku tags: it was created outside meroku, or by hand.
	githubOIDCAttributionUntagged = "untagged"
	// githubOIDCAttributionUnavailable means the tag read failed. The conflict
	// still stands — only the name of its owner is missing.
	githubOIDCAttributionUnavailable = "unavailable"
)

const (
	githubOIDCSubjectsFromYAML    = "yaml"
	githubOIDCSubjectsFromRequest = "request"
	// githubOIDCSubjectsDeployedSuffix marks that the own role's DEPLOYED trust
	// policy contributed to the set that was compared. Narrowing dev.yaml
	// without applying it must not turn the badge green while the live role
	// still overlaps.
	githubOIDCSubjectsDeployedSuffix = "+deployed"
)

// githubOIDCShippedDefaultSubject is the subject list entry this repository
// shipped as the default for every new environment, until it was replaced by an
// obvious placeholder in app/model.go.
//
// It is a constant, in one place, because two things read it and they must not
// drift apart: the detection below, and the copy that tells a user their config
// is meroku's untouched default rather than something they chose. Getting that
// distinction wrong in either direction sends somebody the wrong message about
// their own AWS account.
//
// What it does: as a StringLike sub condition it matches every token whose
// subject begins "repo:MadAppGang/", and those tokens are issued to workflows
// running in MadAppGang's OWN repositories. For any user of this tool who is not
// MadAppGang, leaving it in place grants a third-party organisation's CI the
// ability to assume their deploy role in their own AWS account.
const githubOIDCShippedDefaultSubject = "repo:MadAppGang/*"

// githubOIDCExcludedNote is the declared blind spot. Own roles are excluded
// from conflicts, but the response names them and says so: dev and prod of one
// project sharing a repository is the normal case, not a warning.
const githubOIDCExcludedNote = "same project and account; overlap between your own environments is not evaluated"

const (
	// githubOIDCListRolesPageSize is what MaxItems is set to. IAM defaults to
	// 100, which makes a 5,000-role account roughly fifty sequential calls
	// inside a twenty-second budget. IAM may still return fewer — the
	// documents are fat and it truncates on response size — so the loop never
	// assumes a page count.
	githubOIDCListRolesPageSize = 1000

	// githubOIDCMaxRolePages bounds the walk. IAM will not return this many
	// pages of 1,000 roles for any real account; the cap is there so a
	// misbehaving marker cannot spin forever, and it reports itself as an
	// incomplete walk rather than as a clean end.
	githubOIDCMaxRolePages = 100

	// githubOIDCMaxPatternBytes is the defensive skip from the architecture. A
	// sub claim is nowhere near this long; a pattern that is has come from
	// somewhere unexpected and a 1024x1024 table is not worth filling for it.
	githubOIDCMaxPatternBytes = 1024

	// githubOIDCMaxPairs caps total pattern comparisons.
	//
	// 250,000 is 5,000 roles carrying ten subjects each against five of our
	// own — an account far larger than any this tool has met, comparing every
	// role in it. At a realistic ~64-byte subject that is ~4,000 table cells a
	// pair, on the order of a second of CPU, well inside the handler's twenty
	// seconds. context.WithTimeout does not preempt CPU-bound work, so this cap
	// and the ctx.Err() checks around it are the only things that honour the
	// deadline once the DP starts running.
	githubOIDCMaxPairs = 250_000
)

// ---------------------------------------------------------------------------
// Response
// ---------------------------------------------------------------------------

// githubOIDCConflictsRequest is the POST body.
//
// OwnSubjects carries the caller's candidate subjects so an in-flight edit is
// evaluated against what the user is looking at rather than what the last save
// happened to land on disk. It is echoed back so the client can bind the result
// to that exact snapshot and discard a stale response.
type githubOIDCConflictsRequest struct {
	Env         string   `json:"env"`
	OwnSubjects []string `json:"own_subjects"`
}

// githubOIDCOverlap is one pair of patterns that intersect, with a concrete sub
// claim that satisfies both. The witness is what turns "these might overlap"
// into "a token carrying this exact claim assumes both roles".
type githubOIDCOverlap struct {
	OwnSubject   string `json:"own_subject"`
	OtherSubject string `json:"other_subject"`
	Witness      string `json:"witness"`
}

// githubOIDCConflict is one foreign role this project's subjects reach.
type githubOIDCConflict struct {
	RoleName     string `json:"role_name"`
	RoleARN      string `json:"role_arn,omitempty"`
	OwnerProject string `json:"owner_project,omitempty"`
	OwnerEnv     string `json:"owner_env,omitempty"`
	// Attribution says how much the owner fields are worth: tags, untagged or
	// unavailable. It never affects whether the conflict is reported.
	Attribution string `json:"attribution"`
	// Unrestricted is the loudest finding available: this role pins no GitHub
	// claim at all, so every repository on GitHub can assume it.
	Unrestricted bool                `json:"unrestricted"`
	Overlaps     []githubOIDCOverlap `json:"overlaps"`
}

// githubOIDCOwnUnrestrictedRole is one of THIS project's own GitHub Actions
// roles whose deployed trust policy pins no GitHub claim at all.
//
// It is not a conflict. A conflict is a relation between two roles; this is a
// property of one, and it is the worst property a role in this scan can have:
// every repository on GitHub can assume it. It needs its own array because the
// own-role union asks "which subjects does our role accept?", and an
// unrestricted role answers "all of them" — which the union represents as an
// EMPTY subject list. Empty contributes no pairs, so before this field existed
// the most dangerous state the scan could encounter compared nothing and
// returned clean.
//
// Finding one does NOT make the response unchecked. The scan ran to completion
// and produced a definite, correct answer; the answer is simply the worst one
// available. It is a finding, not a degradation.
type githubOIDCOwnUnrestrictedRole struct {
	RoleName string `json:"role_name"`
	RoleARN  string `json:"role_arn,omitempty"`
	// Env is the environment of ours whose computed role name matched, when it
	// is known. Empty rather than guessed: ownRoleNames returns names, not the
	// environments behind them, and a wrong environment in a security warning
	// sends somebody to edit the wrong file.
	Env string `json:"env,omitempty"`
}

// githubOIDCOrgWideSubject is one of THIS project's own subjects that accepts
// an entire GitHub organisation.
//
// One tier below githubOIDCOwnUnrestrictedRole and reported for the same reason:
// it is a property of our own configuration rather than a relation between two
// roles, so it has no home in Conflicts, and the subject compares perfectly well
// against everybody else's while granting far more than the project enumerates.
// A scan could return zero conflicts, entirely correctly, over a subject that
// hands the whole of some organisation a deploy role.
//
// Finding one does NOT make the response unchecked. The scan ran to completion
// and produced a definite, correct answer. It is a finding, not a degradation.
type githubOIDCOrgWideSubject struct {
	Subject string `json:"subject"`
	// Org is the organisation segment, and only when it is a literal with no
	// metacharacters. "repo:*/api:*" leaves it empty: the pattern is broader
	// still, and naming an organisation that the pattern does not actually pin
	// would be worse than naming none.
	Org string `json:"org,omitempty"`
	// ShippedDefault marks the subject as meroku's untouched default. It changes
	// only the copy, never the verdict — but it changes it a lot: this is the
	// case where the grant is to a third party the user has never heard of.
	ShippedDefault bool `json:"shipped_default"`
}

// githubOIDCUnevaluatedRole is a GitHub role the scan looked at and could not
// judge. Listing it is the honest alternative to a green badge covering a role
// nobody evaluated.
type githubOIDCUnevaluatedRole struct {
	RoleName  string   `json:"role_name"`
	Reason    string   `json:"reason"`
	ClaimKeys []string `json:"claim_keys,omitempty"`
}

// githubOIDCUnevaluatedOtherClaims is the only reason value in use today: the
// role is fenced by :repository_owner, :job_workflow_ref or another claim that
// is not a subject glob, so there is nothing to intersect.
const githubOIDCUnevaluatedOtherClaims = "restricted_by_other_claims"

// githubOIDCDegraded is one thing the scan did not do.
//
// Kind is not a field. It is derived from Reason at marshal time so a caller
// cannot label a scan-incomplete reason as not-applicable and turn the UI
// silent on a scan that missed roles.
type githubOIDCDegraded struct {
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

// Kind partitions the reason. Anything not explicitly not-applicable is
// scan-incomplete, which is the safe direction.
func (d githubOIDCDegraded) Kind() string {
	if githubOIDCNotApplicableReasons[d.Reason] {
		return githubOIDCDegradedNotApplicable
	}
	return githubOIDCDegradedScanIncomplete
}

func (d githubOIDCDegraded) MarshalJSON() ([]byte, error) {
	type alias githubOIDCDegraded
	return json.Marshal(struct {
		Kind string `json:"kind"`
		alias
	}{Kind: d.Kind(), alias: alias(d)})
}

// githubOIDCConflictsResponse is the whole answer.
//
// Note what is absent: there is no Checked field. See Checked and MarshalJSON.
type githubOIDCConflictsResponse struct {
	AccountID    string `json:"account_id,omitempty"`
	RolesScanned int    `json:"roles_scanned"`
	// OwnSubjects is the set actually compared, after the request/YAML choice
	// and the deployed-policy union.
	OwnSubjects       []string `json:"own_subjects"`
	OwnSubjectsSource string   `json:"own_subjects_source,omitempty"`
	// ExcludedRoles are this project's own GitHub roles, found in the account
	// and deliberately not evaluated. Declared, never hidden.
	ExcludedRoles []string `json:"excluded_roles"`
	ExcludedNote  string   `json:"excluded_note,omitempty"`
	// OwnUnrestrictedRoles are our own roles that trust all of GitHub. It
	// outranks Conflicts everywhere it is rendered or acted on, so it sits
	// above it here too.
	OwnUnrestrictedRoles []githubOIDCOwnUnrestrictedRole `json:"own_unrestricted_roles"`
	// OwnOrgWideSubjects are subjects of ours that accept an entire GitHub
	// organisation. Ranked below OwnUnrestrictedRoles and above Conflicts, and
	// ordered here to match.
	OwnOrgWideSubjects []githubOIDCOrgWideSubject  `json:"own_org_wide_subjects"`
	Conflicts          []githubOIDCConflict        `json:"conflicts"`
	UnevaluatedRoles   []githubOIDCUnevaluatedRole `json:"unevaluated_roles"`
	Degraded           []githubOIDCDegraded        `json:"degraded"`
}

// Checked is the completeness invariant, computed rather than remembered.
//
// True only when the degraded list is empty: the asserted account was scanned
// to completion and every candidate role and subject was evaluated or safely
// excluded. Both partitions make it false — a scan that did not need to run did
// not verify anything either — and the partition decides only whether the UI
// says so out loud.
//
// Deriving it here is the entire defence. Setting a boolean at each failure
// site means every future failure site is a chance to forget, and the one that
// forgets renders a partial scan as reassurance.
//
// Findings do not enter into it. A checked:true response with conflicts, or
// with own unrestricted roles, is the normal shape of a scan that worked and
// found something: checked answers "did the scan see everything", never "was
// the news good".
func (r githubOIDCConflictsResponse) Checked() bool {
	return len(r.Degraded) == 0
}

// MarshalJSON emits checked ahead of everything else, from Checked().
func (r githubOIDCConflictsResponse) MarshalJSON() ([]byte, error) {
	type alias githubOIDCConflictsResponse
	return json.Marshal(struct {
		Checked bool `json:"checked"`
		alias
	}{Checked: r.Checked(), alias: alias(r)})
}

// degrade records something the scan did not do. The only way to add one.
func (r *githubOIDCConflictsResponse) degrade(reason, detail string) {
	r.Degraded = append(r.Degraded, githubOIDCDegraded{Reason: reason, Detail: detail})
}

// newGithubOIDCConflictsResponse initialises every slice.
//
// A nil slice marshals as null and the UI calls .length on every one of these.
// This is the only constructor for that reason.
func newGithubOIDCConflictsResponse() githubOIDCConflictsResponse {
	return githubOIDCConflictsResponse{
		OwnSubjects:          []string{},
		ExcludedRoles:        []string{},
		OwnUnrestrictedRoles: []githubOIDCOwnUnrestrictedRole{},
		OwnOrgWideSubjects:   []githubOIDCOrgWideSubject{},
		Conflicts:            []githubOIDCConflict{},
		UnevaluatedRoles:     []githubOIDCUnevaluatedRole{},
		Degraded:             []githubOIDCDegraded{},
	}
}

// ---------------------------------------------------------------------------
// Refusals that cost nothing
// ---------------------------------------------------------------------------

// githubOIDCConflictRefusal answers the cases that must not reach AWS at all.
//
// The empty-AccountID refusal is the load-bearing one, and it is a refusal
// rather than a skipped assertion because both profile mitigations fail
// together on a fresh environment: WithSharedConfigProfile("") is a documented
// no-op, and app/model.go seeds AccountID and AWSProfile as "" alike. So with no
// account declared, nothing pins the credentials and nothing checks them, and
// the scan would report confidently about whichever account the ambient
// AWS_PROFILE names.
//
// no_subjects is deliberately NOT decided here. The set compared is the union
// of the configured subjects with the own role's deployed trust policy, and the
// deployed half only arrives with the ListRoles pass — so an empty configured
// set is not yet an empty set. When OIDC is off the union is moot and the first
// branch already returns without a call, which is the zero-call guarantee that
// matters: an environment with the feature switched off never touches AWS.
func githubOIDCConflictRefusal(env Env, requested []string) (githubOIDCConflictsResponse, bool) {
	resp := newGithubOIDCConflictsResponse()
	resp.AccountID = env.AccountID
	resp.OwnSubjects, resp.OwnSubjectsSource = githubOIDCConfiguredSubjects(env, requested)

	switch {
	case !env.Workload.EnableGithubOIDC:
		resp.degrade(githubOIDCReasonDisabled,
			"workload.enable_github_oidc is false, so this environment has no GitHub Actions role to overlap with")
		return resp, true

	case env.AccountID == "":
		resp.degrade(githubOIDCReasonNoAccountID,
			"account_id is not set in this environment's config, so there is no account to scan or to assert against")
		return resp, true
	}

	return resp, false
}

// githubOIDCConfiguredSubjects picks the subject list to start from and names
// where it came from.
//
// A request that supplies subjects replaces the YAML read entirely: it is what
// the user is looking at, and a scan bound to a stale disk read can answer
// about a value the user changed a keystroke ago. An empty list in the request
// is not a supplied list — the client omitting the field and the client sending
// [] are the same intent, and neither is "compare nothing".
func githubOIDCConfiguredSubjects(env Env, requested []string) ([]string, string) {
	if len(requested) > 0 {
		return githubOIDCDedupe(requested), githubOIDCSubjectsFromRequest
	}
	return githubOIDCDedupe(env.Workload.GithubOIDCSubjects), githubOIDCSubjectsFromYAML
}

// githubOIDCDedupe removes blanks and repeats, preserving order. Order is
// preserved so the reported overlaps line up with the list the user is editing.
func githubOIDCDedupe(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// ---------------------------------------------------------------------------
// Org-wide subjects
// ---------------------------------------------------------------------------

// githubOIDCSubjectMetacharacters are the two IAM StringLike wildcards. There is
// no escape character, so a literal '*' in a pattern is not expressible and
// every occurrence is a wildcard.
const githubOIDCSubjectMetacharacters = "*?"

// githubOIDCOrgWideSubjectFor classifies one subject pattern, and is the whole
// of the org-wide rule.
//
// Pure, and deliberately NOT built on globsIntersect or globMatch. Those answer
// a matching question — "is there a string both patterns accept" — and this is a
// structural question about where in the pattern the wildcards fall. Phrasing it
// as a match would need an oracle set of repository names to test against, which
// does not exist: the point is precisely that the pattern accepts repositories
// nobody has enumerated.
//
// The grammar is fixed: a GitHub sub is "repo:<org>/<repo>:<ref-spec>". A
// pattern accepts an entire organisation the moment a wildcard appears at or
// before the <repo> segment, because from there the pattern stops naming a
// repository:
//
//	repo:acme/*                        org-wide, the repository segment is a wildcard
//	repo:acme/*:ref:refs/heads/main    org-wide, same, with the ref pinned
//	repo:*/api:*                       org-wide, and broader — the ORG is a wildcard
//	*                                  org-wide, it accepts every subject there is
//	repo:acme/api:*                    NOT org-wide, only the ref varies
//	repo:acme/api:ref:refs/heads/main  NOT org-wide, nothing varies
func githubOIDCOrgWideSubjectFor(subject string) (githubOIDCOrgWideSubject, bool) {
	if !strings.ContainsAny(githubOIDCRepoIdentity(subject), githubOIDCSubjectMetacharacters) {
		return githubOIDCOrgWideSubject{}, false
	}
	return githubOIDCOrgWideSubject{
		Subject:        subject,
		Org:            githubOIDCLiteralOrg(subject),
		ShippedDefault: subject == githubOIDCShippedDefaultSubject,
	}, true
}

// githubOIDCRepoIdentity returns the part of the pattern that names a
// repository: everything up to, but not including, the second ':'.
//
// "repo:acme/api:ref:refs/heads/main" gives "repo:acme/api" — the identity — and
// the ref-spec after it is what a narrow subject is allowed to wildcard.
//
// A pattern with fewer segments than the grammar expects is returned whole
// rather than skipped, and that is the safe direction: "repo:acme/*" has no
// second ':' at all, and treating its missing ref-spec as a reason to skip the
// check would miss the single most important case there is. A pattern with no
// ':' whatsoever — "*" — is likewise its own identity, and a wildcard one.
func githubOIDCRepoIdentity(subject string) string {
	first := strings.Index(subject, ":")
	if first < 0 {
		return subject
	}
	second := strings.Index(subject[first+1:], ":")
	if second < 0 {
		return subject
	}
	return subject[:first+1+second]
}

// githubOIDCLiteralOrg returns the organisation the pattern pins, and "" when it
// does not pin one.
//
// Only a literal counts. "repo:*/api:*" is org-wide and then some, but it names
// no organisation, and printing "every repository in the * organisation" would
// be worse than printing nothing. An org segment carrying a ':' is malformed
// against the grammar and is not reported either.
func githubOIDCLiteralOrg(subject string) string {
	const prefix = "repo:"
	if !strings.HasPrefix(subject, prefix) {
		return ""
	}
	rest := subject[len(prefix):]
	slash := strings.Index(rest, "/")
	if slash <= 0 {
		return "" // no repository segment, or an empty organisation
	}
	org := rest[:slash]
	if strings.ContainsAny(org, githubOIDCSubjectMetacharacters+":") {
		return ""
	}
	return org
}

// githubOIDCFindOrgWideSubjects classifies a whole subject set, preserving order.
//
// Order is the caller's, which is the union: the configured list as the user
// wrote it, then anything the deployed policy added. That keeps the report lined
// up with the list being edited, exactly as githubOIDCDedupe does.
func githubOIDCFindOrgWideSubjects(subjects []string) []githubOIDCOrgWideSubject {
	out := make([]githubOIDCOrgWideSubject, 0, len(subjects))
	for _, s := range subjects {
		if found, ok := githubOIDCOrgWideSubjectFor(s); ok {
			out = append(out, found)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// The scan
// ---------------------------------------------------------------------------

// githubOIDCCandidate is one foreign GitHub role, held until the own-subject
// union is complete.
//
// The walk and the comparison are two phases rather than one because the own
// subjects are not fully known until the walk ends: the own role's deployed
// policy is read out of the same ListRoles response. Holding candidates costs
// nothing worth counting — only roles that federate GitHub are kept, and an
// account has a handful.
type githubOIDCCandidate struct {
	name         string
	arn          string
	subjects     []string
	unrestricted bool
}

// scanGitHubSubjectConflicts is the whole read-only scan.
//
// It never returns an error. Every failure is a degraded entry on the response,
// because the caller has three outcomes to render and an error has one: a
// conflict, a clean bill, and a scan that could not finish. Collapsing the
// third into either of the others is the defect this feature exists to prevent.
func scanGitHubSubjectConflicts(ctx context.Context, env Env, requested []string, roles iamRoleLister, ident callerIdentityReader) githubOIDCConflictsResponse {
	resp, refused := githubOIDCConflictRefusal(env, requested)
	if refused {
		return resp
	}

	// The account assertion comes first and nothing lists until it passes.
	// Listing an account we have not identified produces an answer about the
	// wrong account, which is worse than no answer.
	identity, err := ident.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		reason, detail := githubOIDCFailureReason(ctx, err)
		resp.degrade(reason, "could not confirm which AWS account these credentials belong to: "+detail)
		return resp
	}
	scanned := aws.ToString(identity.Account)
	if scanned != env.AccountID {
		resp.degrade(githubOIDCReasonWrongAccount, fmt.Sprintf(
			"these credentials belong to account %s, but this environment declares %s; nothing was listed",
			githubOIDCAccountLabel(scanned), env.AccountID))
		return resp
	}

	// This environment's own role is always excluded, computed without reading
	// anything. The sibling read can fail; that must not turn our own roles
	// into reported conflicts.
	//
	// The map is name -> environment rather than a set because an own role that
	// trusts all of GitHub is reported, and a report reads better with the
	// environment on it. Membership is the exclusion; the value only decorates.
	// The environment is known for this one and unknown for the siblings —
	// ownRoleNames returns names, and the cascade that produced them is not
	// invertible — so those carry "" and the report says nothing rather than
	// guessing.
	own := map[string]string{ownGitHubRoleName(env.Project, env.Env): env.Env}
	siblings, err := ownRoleNames(env.Project, env.AccountID)
	if err != nil {
		resp.degrade(githubOIDCReasonEnvUnreadable, fmt.Sprintf(
			"could not read this project's other environments, so their roles may be reported as conflicts: %v", err))
	}
	for name := range siblings {
		if _, known := own[name]; !known {
			own[name] = ""
		}
	}

	candidates, deployed := githubOIDCWalkRoles(ctx, &resp, roles, own)

	// The union. Without it, narrowing the subject list in YAML and not
	// applying it renders green while the deployed role still overlaps.
	//
	// One thing this union deliberately does not carry: an own role whose
	// deployed policy is itself unrestricted contributes no subjects, so it
	// widens nothing here. That role trusts every repository on GitHub — a real
	// finding, but a different one, about our own configuration rather than
	// about a collision with somebody else's. It is reported on its own terms in
	// OwnUnrestrictedRoles, which the walk above has already filled in, and is
	// never inferred into a subject the union could compare.
	for _, s := range deployed {
		if !githubOIDCContains(resp.OwnSubjects, s) {
			resp.OwnSubjects = append(resp.OwnSubjects, s)
		}
	}
	if len(deployed) > 0 {
		// +deployed means the live policy was read and folded in, whether or
		// not it added a pattern the configured list did not already have. The
		// field says what was compared, and what was compared is the union.
		resp.OwnSubjectsSource += githubOIDCSubjectsDeployedSuffix
	}

	if len(resp.OwnSubjects) == 0 {
		// Nothing to compare, so nothing can overlap. Not-applicable rather
		// than scan-incomplete: this is a complete answer to a question with no
		// content, and the UI stays silent.
		resp.degrade(githubOIDCReasonNoSubjects,
			"this environment declares no github_oidc_subjects and its deployed role pins none, so there is nothing to compare")
		return resp
	}

	// What the union grants, asked of the union rather than of the configured
	// list alone — for the same reason the comparison below is. A subject
	// narrowed in YAML but never applied leaves the deployed role accepting the
	// whole organisation, and a check that read only the file would miss it.
	//
	// Costs nothing and reaches no API: the answer is a property of the strings.
	resp.OwnOrgWideSubjects = githubOIDCFindOrgWideSubjects(resp.OwnSubjects)

	githubOIDCCompare(ctx, &resp, candidates)
	githubOIDCAttribute(ctx, &resp, roles)
	return resp
}

// githubOIDCWalkRoles paginates ListRoles once, classifying every role.
//
// It returns the foreign GitHub roles worth comparing and the subject patterns
// found on this project's own DEPLOYED roles. Both come out of the same pass:
// the exclusion set needs each own role's name, and the union needs the
// AssumeRolePolicyDocument sitting beside it in the very same response.
//
// The same document answers a third question for free, which is why
// resp.OwnUnrestrictedRoles is filled here and not by a second read: whether
// one of our own roles pins no GitHub claim at all.
//
// own maps each of this project's role names to its environment, or to "" when
// the environment is not known. Membership decides the exclusion; the value
// only decorates a finding.
func githubOIDCWalkRoles(ctx context.Context, resp *githubOIDCConflictsResponse, roles iamRoleLister, own map[string]string) ([]githubOIDCCandidate, []string) {
	var candidates []githubOIDCCandidate
	var deployed []string
	var excluded []string
	otherClaimRoles := 0

	in := &iam.ListRolesInput{MaxItems: aws.Int32(githubOIDCListRolesPageSize)}

	for page := 0; ; page++ {
		if err := ctx.Err(); err != nil {
			resp.degrade(githubOIDCReasonTimeout, fmt.Sprintf(
				"the scan ran out of time after %d roles; later roles were never listed", resp.RolesScanned))
			break
		}
		if page >= githubOIDCMaxRolePages {
			resp.degrade(githubOIDCReasonPagination, fmt.Sprintf(
				"stopped after %d pages of roles; the account has more than this scan will walk", githubOIDCMaxRolePages))
			break
		}

		out, err := roles.ListRoles(ctx, in)
		if err != nil {
			reason, detail := githubOIDCFailureReason(ctx, err)
			// Whatever was found before the failure stands. An AccessDenied
			// here is the case this whole file is arranged against: it must
			// never render as checked:true with an empty conflicts list.
			resp.degrade(reason, fmt.Sprintf("listing IAM roles failed after %d roles: %s", resp.RolesScanned, detail))
			break
		}
		if out == nil {
			resp.degrade(githubOIDCReasonAWSError, "IAM returned no response to ListRoles")
			break
		}

		for _, role := range out.Roles {
			if err := ctx.Err(); err != nil {
				resp.degrade(githubOIDCReasonTimeout, fmt.Sprintf(
					"the scan ran out of time after %d roles; later roles were never evaluated", resp.RolesScanned))
				return candidates, deployed
			}

			resp.RolesScanned++
			name := aws.ToString(role.RoleName)
			doc := aws.ToString(role.AssumeRolePolicyDocument)

			ownEnv, isOwn := own[name]

			grant, err := parseGitHubTrustPolicy(doc)
			if err != nil {
				// Unreadable, so unjudged. Naming the role is the difference
				// between a warning a user can act on and a shrug.
				resp.degrade(githubOIDCReasonUnparseable, fmt.Sprintf(
					"could not read the trust policy of role %q, so it was not evaluated: %v", name, err))
				if isOwn {
					excluded = append(excluded, name)
				}
				continue
			}

			if isOwn {
				// Ours. Excluded from conflicts, but its deployed subjects are
				// exactly what we must compare against everybody else.
				excluded = append(excluded, name)
				deployed = append(deployed, grant.Subjects...)

				// And the one thing the subjects cannot say. An unrestricted
				// own role has no subjects at all, so it contributes nothing to
				// the union, compares against nobody, and would otherwise leave
				// the account's most dangerous role rendering as a clean scan.
				if grant.Unrestricted {
					resp.OwnUnrestrictedRoles = append(resp.OwnUnrestrictedRoles, githubOIDCOwnUnrestrictedRole{
						RoleName: name,
						RoleARN:  aws.ToString(role.Arn),
						Env:      ownEnv,
					})
				}
				continue
			}

			if !grant.IsGitHub {
				continue // not a GitHub role; nothing to say about it
			}

			if grant.RestrictedByOtherClaims {
				// Fenced by :repository_owner or similar. Not unrestricted and
				// not cleared: this scan reasons about sub alone, and a role
				// pinned to our own organisation plausibly does overlap.
				otherClaimRoles++
				resp.UnevaluatedRoles = append(resp.UnevaluatedRoles, githubOIDCUnevaluatedRole{
					RoleName:  name,
					Reason:    githubOIDCUnevaluatedOtherClaims,
					ClaimKeys: grant.OtherClaimKeys,
				})
			}

			if !grant.Unrestricted && len(grant.Subjects) == 0 {
				continue // nothing evaluable on this role
			}
			candidates = append(candidates, githubOIDCCandidate{
				name:         name,
				arn:          aws.ToString(role.Arn),
				subjects:     githubOIDCDedupe(grant.Subjects),
				unrestricted: grant.Unrestricted,
			})
		}

		if !out.IsTruncated {
			break
		}
		marker := aws.ToString(out.Marker)
		if marker == "" {
			// IsTruncated with no marker is an INCOMPLETE walk, not a clean
			// end. Breaking silently here is how a first page with no conflict
			// becomes checked:true while every later role went unread.
			resp.degrade(githubOIDCReasonPagination,
				"IAM reported more roles but returned no pagination marker, so the account was only partly listed")
			break
		}
		in.Marker = aws.String(marker)
	}

	if otherClaimRoles > 0 {
		resp.degrade(githubOIDCReasonOtherClaims, fmt.Sprintf(
			"%s restrict access by GitHub claims other than sub, which this scan cannot intersect",
			githubOIDCPluralRoles(otherClaimRoles)))
	}

	sort.Strings(excluded)
	resp.ExcludedRoles = append(resp.ExcludedRoles, excluded...)
	if len(resp.ExcludedRoles) > 0 {
		resp.ExcludedNote = githubOIDCExcludedNote
	}

	// ListRoles order is not guaranteed, and a refusal a user is meant to
	// compare against the last one must not reshuffle between runs.
	sort.Slice(resp.OwnUnrestrictedRoles, func(i, j int) bool {
		return resp.OwnUnrestrictedRoles[i].RoleName < resp.OwnUnrestrictedRoles[j].RoleName
	})

	return candidates, deployed
}

// githubOIDCCompare intersects every own subject with every candidate subject.
//
// The two budget checks are not decoration. context.WithTimeout does not
// preempt CPU-bound work, so a deadline means nothing to the DP unless
// something between the pairs looks at it.
func githubOIDCCompare(ctx context.Context, resp *githubOIDCConflictsResponse, candidates []githubOIDCCandidate) {
	// Deterministic order: ListRoles order is not guaranteed, and a warning a
	// user is meant to compare against the last one must not reshuffle.
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].name < candidates[j].name })

	skipped := 0
	ownPatterns := githubOIDCUsablePatterns(resp.OwnSubjects, &skipped)

	pairs := 0
	timedOut := false
	budgetGone := false

compare:
	for _, cand := range candidates {
		if ctx.Err() != nil {
			timedOut = true
			break
		}

		conflict := githubOIDCConflict{
			RoleName:     cand.name,
			RoleARN:      cand.arn,
			Attribution:  githubOIDCAttributionUnavailable,
			Unrestricted: cand.unrestricted,
			Overlaps:     []githubOIDCOverlap{},
		}

		for _, other := range githubOIDCUsablePatterns(cand.subjects, &skipped) {
			for _, mine := range ownPatterns {
				if ctx.Err() != nil {
					timedOut = true
					break compare
				}
				if pairs >= githubOIDCMaxPairs {
					budgetGone = true
					break compare
				}
				pairs++

				if !globsIntersect([]byte(mine), []byte(other)) {
					continue
				}
				conflict.Overlaps = append(conflict.Overlaps, githubOIDCOverlap{
					OwnSubject:   mine,
					OtherSubject: other,
					Witness:      githubOIDCWitness(mine, other),
				})
			}
		}

		// An unrestricted role trusts every repository on GitHub, so it
		// conflicts whether or not any pattern pair intersects.
		if conflict.Unrestricted || len(conflict.Overlaps) > 0 {
			resp.Conflicts = append(resp.Conflicts, conflict)
		}
	}

	if skipped > 0 {
		resp.degrade(githubOIDCReasonPatternTooLong, fmt.Sprintf(
			"%d subject pattern(s) longer than %d bytes were not compared",
			skipped, githubOIDCMaxPatternBytes))
	}
	if budgetGone {
		resp.degrade(githubOIDCReasonBudgetExhausted, fmt.Sprintf(
			"stopped after %d subject comparisons; some roles were not compared", githubOIDCMaxPairs))
	}
	if timedOut {
		resp.degrade(githubOIDCReasonTimeout, fmt.Sprintf(
			"the scan ran out of time after %d subject comparisons; some roles were not compared", pairs))
	}
}

// githubOIDCUsablePatterns drops patterns too long to compare, counting them.
// The count becomes a pattern_too_long entry, because a silently skipped
// pattern reads as reassurance exactly like a silently skipped role.
func githubOIDCUsablePatterns(in []string, skipped *int) []string {
	out := make([]string, 0, len(in))
	for _, p := range in {
		if len(p) > githubOIDCMaxPatternBytes {
			*skipped++
			continue
		}
		out = append(out, p)
	}
	return out
}

// githubOIDCAttribute names the owner of each confirmed conflict.
//
// One ListRoleTags per conflict, and never anything else. Attribution is who to
// go and talk to; it is not evidence, so a failed read leaves the conflict
// standing, marks it unavailable, and does not touch checked. A tag read that
// could downgrade a red finding would be a tag read that could hide one.
func githubOIDCAttribute(ctx context.Context, resp *githubOIDCConflictsResponse, roles iamRoleLister) {
	for i := range resp.Conflicts {
		if ctx.Err() != nil {
			// Out of time. The conflicts are already found and reported; only
			// their owners' names are missing.
			resp.Conflicts[i].Attribution = githubOIDCAttributionUnavailable
			continue
		}
		out, err := roles.ListRoleTags(ctx, &iam.ListRoleTagsInput{
			RoleName: aws.String(resp.Conflicts[i].RoleName),
		})
		if err != nil || out == nil {
			resp.Conflicts[i].Attribution = githubOIDCAttributionUnavailable
			continue
		}
		project, env := merokuTagsOf(out.Tags)
		resp.Conflicts[i].OwnerProject = project
		resp.Conflicts[i].OwnerEnv = env
		if project != "" && env != "" {
			resp.Conflicts[i].Attribution = githubOIDCAttributionTags
		} else {
			resp.Conflicts[i].Attribution = githubOIDCAttributionUntagged
		}
	}
}

// ---------------------------------------------------------------------------
// Witnesses
// ---------------------------------------------------------------------------

// githubOIDCExampleSub is a syntactically real GitHub sub claim, used when the
// intersection is so wide that the DP's witness is the empty string — which
// happens whenever both patterns are "*".
const githubOIDCExampleSub = "repo:example-org/example-repo:ref:refs/heads/main"

// githubOIDCWitness returns a concrete sub claim that both patterns accept, in
// a form GitHub could actually issue.
//
// The DP's witness is mathematically correct and sometimes rhetorically false.
// "repo:MadAppGang/*" against itself yields "repo:MadAppGang/", and "*" against
// "*" yields "". Both match. Neither is a claim any token has ever carried, and
// quoting one under the sentence "a token with this exact claim assumes both
// roles" is a false statement in a security warning.
//
// So a witness that stops at a separator, or is empty, is extended into the
// shape of a real subject — and the extension is re-verified with globMatch
// against both patterns before it is used. Nothing is ever reported that
// globMatch rejects: an unverifiable extension falls back to the raw witness,
// and a raw witness that somehow fails falls back to the empty string, which
// the UI renders as "any subject" rather than as a quotable claim.
func githubOIDCWitness(a, b string) string {
	raw, ok := globWitness([]byte(a), []byte(b))
	if !ok {
		return ""
	}

	pa, pb := []byte(a), []byte(b)
	verify := func(s string) bool { return globMatch(pa, []byte(s)) && globMatch(pb, []byte(s)) }

	witness := string(raw)
	if !githubOIDCWitnessIsIssuable(witness) {
		for _, candidate := range githubOIDCWitnessExtensions(witness) {
			if verify(candidate) {
				return candidate
			}
		}
	}
	if verify(witness) {
		return witness
	}
	return ""
}

// githubOIDCWitnessIsIssuable reports whether a witness reads as a claim GitHub
// could mint.
//
// GitHub's sub is "repo:OWNER/REPO:<context>" — a ref, a pull request, an
// environment. A witness ending at a separator, or missing the context
// entirely, is one the DP truncated where a trailing "*" began. Anything that
// is not a repo: subject is left alone: an unrecognised shape cannot be
// improved, only guessed at.
func githubOIDCWitnessIsIssuable(w string) bool {
	if w == "" {
		return false
	}
	if strings.HasSuffix(w, "/") || strings.HasSuffix(w, ":") {
		return false
	}
	if !strings.HasPrefix(w, "repo:") {
		return true
	}
	rest := strings.TrimPrefix(w, "repo:")
	slash := strings.Index(rest, "/")
	if slash < 0 {
		return false // repo:OWNER, with no repository
	}
	return strings.Contains(rest[slash+1:], ":") // repo:OWNER/REPO:<context>
}

// githubOIDCWitnessExtensions offers completions for a truncated witness, most
// plausible first. Each is verified by the caller before use.
func githubOIDCWitnessExtensions(w string) []string {
	switch {
	case w == "":
		return []string{githubOIDCExampleSub}
	case strings.HasSuffix(w, "/"):
		return []string{w + "example-repo:ref:refs/heads/main", w + "example-repo"}
	case strings.HasSuffix(w, ":"):
		return []string{w + "ref:refs/heads/main", w + githubOIDCExampleSub}
	default:
		return []string{w + ":ref:refs/heads/main", w + "/example-repo:ref:refs/heads/main"}
	}
}

// ---------------------------------------------------------------------------
// Error classification
// ---------------------------------------------------------------------------

// githubOIDCFailureReason maps an AWS failure to a degraded reason.
//
// Matching on the smithy error code rather than on concrete exception types
// follows classifyComputeError (app/compute_catalog.go): IAM and STS model
// these differently and two type switches would be two chances to miss one.
// An unrecognised failure becomes aws_error, never one of the specific reasons
// — a guess here sends the user to fix something that was never broken.
func githubOIDCFailureReason(ctx context.Context, err error) (reason, detail string) {
	if err == nil {
		return "", ""
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || ctx.Err() != nil {
		return githubOIDCReasonTimeout, summarizeAWSError(err)
	}

	switch awsErrorCode(err) {
	case "AccessDenied", "AccessDeniedException", "UnauthorizedOperation", "AuthorizationError", "AuthFailure":
		return githubOIDCReasonAccessDenied, summarizeAWSError(err)
	case "Throttling", "ThrottlingException", "TooManyRequestsException", "RequestLimitExceeded", "SlowDown":
		return githubOIDCReasonThrottled, summarizeAWSError(err)
	case "ExpiredToken", "ExpiredTokenException", "RequestExpired", "InvalidClientTokenId", "UnrecognizedClientException":
		return githubOIDCReasonNoCredentials, summarizeAWSError(err)
	case "RequestCanceled":
		return githubOIDCReasonTimeout, summarizeAWSError(err)
	}

	if looksLikeExpiredSession(err) {
		return githubOIDCReasonNoCredentials, summarizeAWSError(err)
	}
	return githubOIDCReasonAWSError, summarizeAWSError(err)
}

// githubOIDCAccountLabel renders an account id for a message, without inventing
// one when STS returned none.
func githubOIDCAccountLabel(id string) string {
	if id == "" {
		return "(unknown)"
	}
	return id
}

func githubOIDCPluralRoles(n int) string {
	if n == 1 {
		return "1 role"
	}
	return fmt.Sprintf("%d roles", n)
}

func githubOIDCContains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Web UI
// ---------------------------------------------------------------------------

// githubOIDCConflictScanTimeout bounds the whole scan, following
// app/dns_handlers.go rather than the bare r.Context() the provider check uses.
// A scan that paginates and then runs a DP over every pattern pair is a
// different cost from two reads on node open.
const githubOIDCConflictScanTimeout = 20 * time.Second

// getGitHubOIDCSubjectConflicts scans the account for GitHub Actions roles
// whose subjects overlap this environment's.
//
// It is READ-ONLY. The sibling endpoint getGithubOIDCStatus writes YAML, which
// is exactly why this is a separate route: the scan is re-run after every
// subject edit, and an endpoint that mutates on every keystroke of a security
// check is a mutation path nobody asked for.
//
// The status codes depart from this file's 502 convention on purpose. An HTTP
// status carries two outcomes and this response carries three: a conflict, a
// clean bill, and a scan that could not finish. Every AWS-side failure is
// therefore 200 with checked:false and a degraded entry, and only a bad request
// or a missing environment is an error status.
func getGitHubOIDCSubjectConflicts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req githubOIDCConflictsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}
	defer r.Body.Close()

	if req.Env == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "env is required"})
		return
	}

	env, err := loadEnv(req.Env)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("environment not found: %v", err)})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), githubOIDCConflictScanTimeout)
	defer cancel()

	// The refusals are answered before any client is built, so an environment
	// with OIDC off or no account id cannot reach AWS even by accident.
	if resp, refused := githubOIDCConflictRefusal(env, req.OwnSubjects); refused {
		writeGithubOIDCConflicts(w, resp)
		return
	}

	iamClient, stsClient, err := newAWSClientsForEnv(ctx, env)
	if err != nil {
		resp := newGithubOIDCConflictsResponse()
		resp.AccountID = env.AccountID
		resp.OwnSubjects, resp.OwnSubjectsSource = githubOIDCConfiguredSubjects(env, req.OwnSubjects)
		resp.degrade(githubOIDCReasonNoCredentials, err.Error())
		writeGithubOIDCConflicts(w, resp)
		return
	}

	writeGithubOIDCConflicts(w, scanGitHubSubjectConflicts(ctx, env, req.OwnSubjects, iamClient, stsClient))
}

func writeGithubOIDCConflicts(w http.ResponseWriter, resp githubOIDCConflictsResponse) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
