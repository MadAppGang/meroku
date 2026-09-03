package main

// The deploy path's half of the subject overlap scan.
//
// app/github_oidc_conflicts.go answers "does another project in this account
// trust the same GitHub subjects". This file answers the separate question of
// what a deploy should do about the answer, and the two are separated because
// the second one has to be testable without a terminal.
//
// The policy has five arms and they are not symmetric:
//
//	our own role trusts all of GitHub  abort, terminal or not, with no question
//	an org-wide subject of ours        ask; abort when there is nobody to ask
//	a found conflict, interactive      ask, and default to no
//	a found conflict, non-interactive  abort, unconditionally
//	a scan that failed                 say so loudly, never block
//
// The asymmetry is the whole design. A found conflict is evidence — a concrete
// sub claim that assumes two projects' roles — so continuing past it deserves a
// deliberate keystroke, and in CI there is no keystroke to be had. A CI run that
// auto-continues past a confirmed conflict is indistinguishable from having no
// check at all.
//
// The first arm is stricter still, and deliberately so: a cross-project overlap
// CAN be intentional and meroku cannot tell, so it asks. A role that trusts
// every repository on GitHub never is, so there is nothing to ask about. No
// prompt, no override flag, no interactivity check — the question would only be
// "do you want your deploy role to be assumable by strangers", and offering it
// makes the answer look like a matter of taste.
//
// The second arm sits between the two and takes the confirmation path, which is
// the deliberate part: an org-wide subject CAN be what somebody meant — a
// monorepo organisation where every repository legitimately deploys the same
// service — whereas a role trusting all of GitHub never is. The prompt is
// exactly the difference between "never correct" and "rarely correct", and
// spending a hard block on "rarely" would teach people to route around it.
//
// A failed scan is not evidence of anything, and blocking on it would break the
// contract resolveGithubOIDCForEnv has documented since it was written: a read
// it could not make is reported and skipped. But it must still be unmistakable,
// or the provider check's ✅ two lines above gets read as an overall security
// result.
//
// A reason that is merely not-applicable — OIDC switched off, no subjects to
// compare — prints nothing at all. A warning on every single deploy of an
// environment that does not use the feature is a warning that gets tuned out,
// and then it is not there for the real case.

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// The decision
// ---------------------------------------------------------------------------

// githubOIDCSeverity is how loudly the CLI should speak about one scan.
type githubOIDCSeverity int

const (
	// githubOIDCSaysNothing is the not-applicable case: the scan did not need
	// to run, so there is nothing to report and nothing to warn about.
	githubOIDCSaysNothing githubOIDCSeverity = iota
	// githubOIDCSaysClear is a completed scan that found no overlap.
	githubOIDCSaysClear
	// githubOIDCSaysUnverified is the scan that could not finish. Yellow, and
	// never blocking.
	githubOIDCSaysUnverified
	// githubOIDCSaysConflict is a found overlap. Red.
	githubOIDCSaysConflict
	// githubOIDCSaysOwnOrgWide is a subject of ours that accepts an entire
	// GitHub organisation. Red, above a conflict — a conflict is bounded by
	// whatever the other project claimed, and this is bounded by nothing
	// narrower than an organisation — but it still asks rather than refusing.
	githubOIDCSaysOwnOrgWide
	// githubOIDCSaysOwnUnrestricted is one of our own roles trusting every
	// repository on GitHub. The top of the scale, and the only tier that
	// refuses without asking.
	githubOIDCSaysOwnUnrestricted
)

// githubOIDCConflictDecision is what to print and what to do, decided from a
// scan response and one flag.
//
// Report is plain text with no styling and no leading or trailing newline. The
// colour is applied at the print boundary rather than here, so a test can read
// the words without stripping escape sequences — and so the decision, which is
// the part that can be wrong in a way that matters, is a pure function.
type githubOIDCConflictDecision struct {
	Severity githubOIDCSeverity
	Report   string
	// Prompt reports that a human must answer before the deploy continues. It
	// is set only when a conflict was found AND there is a terminal to ask on.
	Prompt bool
	// Block is the decision when nothing is asked. A declined prompt blocks
	// too, but that is the caller's business, not this struct's.
	Block bool
}

// decideGithubOIDCConflicts turns a scan response into what the deploy should
// do about it. Pure: no printing, no prompting, no terminal.
func decideGithubOIDCConflicts(resp githubOIDCConflictsResponse, interactive bool) githubOIDCConflictDecision {
	incomplete := githubOIDCIncompleteReasons(resp)

	// One of our own roles trusting all of GitHub outranks every other tier,
	// including a cross-project conflict found in the same scan. It is a hard
	// block: Block is set without consulting interactive, and Prompt is left
	// false, so the caller never reaches the confirmation. There is nothing to
	// confirm — see the asymmetry at the top of this file.
	if len(resp.OwnUnrestrictedRoles) > 0 {
		return githubOIDCConflictDecision{
			Severity: githubOIDCSaysOwnUnrestricted,
			Report:   githubOIDCOwnUnrestrictedReport(resp, incomplete),
			Block:    true,
		}
	}

	// An org-wide subject of ours ranks below that hard block and above a
	// cross-project conflict, and takes the CONFIRMATION path rather than
	// becoming a second hard block.
	//
	// The reason is the one at the top of this file: an org-wide subject can be
	// intended — a monorepo organisation where every repository legitimately
	// deploys the same service — while a role trusting all of GitHub cannot.
	// Non-interactive still aborts, on the same grounds as the conflict path: a
	// CI run that auto-continues past a finding is the same as no check at all.
	if len(resp.OwnOrgWideSubjects) > 0 {
		d := githubOIDCConflictDecision{
			Severity: githubOIDCSaysOwnOrgWide,
			Report:   githubOIDCOrgWideReport(resp, incomplete, interactive),
		}
		if interactive {
			d.Prompt = true
			return d
		}
		d.Block = true
		return d
	}

	// A found conflict outranks everything, including an incomplete scan. The
	// evidence is already in hand; that the walk did not finish only means
	// there may be more of it.
	if len(resp.Conflicts) > 0 {
		d := githubOIDCConflictDecision{
			Severity: githubOIDCSaysConflict,
			Report:   githubOIDCConflictReport(resp, incomplete, interactive),
		}
		if interactive {
			d.Prompt = true
			return d
		}
		// Nobody to ask. Abort, and say why the question was not put.
		d.Block = true
		return d
	}

	// An unevaluated role is checked here as well as through its degraded
	// entry. The scan emits both together today; asserting only the reason
	// would make a green line depend on that staying true.
	if len(incomplete) > 0 || len(resp.UnevaluatedRoles) > 0 {
		return githubOIDCConflictDecision{
			Severity: githubOIDCSaysUnverified,
			Report:   githubOIDCUnverifiedReport(resp, incomplete),
		}
	}

	// Everything left in the degraded list is not-applicable, which is silent.
	if len(resp.Degraded) > 0 {
		return githubOIDCConflictDecision{Severity: githubOIDCSaysNothing}
	}

	return githubOIDCConflictDecision{
		Severity: githubOIDCSaysClear,
		Report: fmt.Sprintf("   ✅ No other project in this account trusts these GitHub subjects (%s scanned).",
			githubOIDCPluralRoles(resp.RolesScanned)),
	}
}

// githubOIDCIncompleteReasons returns the degraded entries that mean something
// was not evaluated.
//
// It asks each entry for its Kind rather than testing membership of a list.
// The partition is the scan's to decide — it derives Kind from the reason at
// one place — and a second copy of the list here would be a second place to
// forget when a reason is added.
func githubOIDCIncompleteReasons(resp githubOIDCConflictsResponse) []githubOIDCDegraded {
	out := make([]githubOIDCDegraded, 0, len(resp.Degraded))
	for _, d := range resp.Degraded {
		if d.Kind() == githubOIDCDegradedScanIncomplete {
			out = append(out, d)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// The words
// ---------------------------------------------------------------------------

// githubOIDCOwnUnrestrictedReport names each of our own roles that trusts all
// of GitHub, and says what that hands out.
//
// It is written as a statement of what has already happened rather than as a
// warning about what might: the role is deployed, and anybody who knows the
// account number can use it today.
func githubOIDCOwnUnrestrictedReport(resp githubOIDCConflictsResponse, incomplete []githubOIDCDegraded) string {
	var b strings.Builder

	if len(resp.OwnUnrestrictedRoles) == 1 {
		b.WriteString("🚨 This project's own GitHub Actions role trusts every repository on GitHub.\n")
	} else {
		fmt.Fprintf(&b, "🚨 %d of this project's own GitHub Actions roles trust every repository on GitHub.\n",
			len(resp.OwnUnrestrictedRoles))
	}

	for _, r := range resp.OwnUnrestrictedRoles {
		fmt.Fprintf(&b, "\n   %s%s\n", r.RoleName, githubOIDCOwnRoleEnvSuffix(r))
		if r.RoleARN != "" {
			fmt.Fprintf(&b, "      %s\n", r.RoleARN)
		}
		b.WriteString("      Its trust policy pins no GitHub claim at all — no sub, nothing.\n")
	}

	b.WriteString("\n   What that means: any repository on GitHub, belonging to anybody, can assume\n")
	b.WriteString("   this role. Whoever does gets iam:PassRole over this project's task roles,\n")
	b.WriteString("   ECR push and ecs:UpdateService — the whole deploy.\n")

	if n := len(resp.Conflicts); n > 0 {
		fmt.Fprintf(&b, "\n   The scan also found %s in this account whose subjects overlap this\n",
			githubOIDCPluralRoles(n))
		b.WriteString("   project's. That is the smaller problem, and not why the deploy stopped:\n")
		for _, c := range resp.Conflicts {
			fmt.Fprintf(&b, "      • %s%s\n", c.RoleName, githubOIDCOwnerSuffix(c))
		}
	}

	if len(incomplete) > 0 {
		b.WriteString("\n   The scan also did not finish, so there may be more than this:\n")
		githubOIDCWriteReasons(&b, incomplete, "      ")
	}

	b.WriteString("\n   The deploy stops here and nothing has been applied. There is no prompt and\n")
	b.WriteString("   no override: an overlap between two projects can be deliberate, but a role\n")
	b.WriteString("   trusting all of GitHub is not. Put a sub condition back on it — set\n")
	b.WriteString("   workload.github_oidc_subjects and apply — then run this again.\n")

	return strings.TrimRight(b.String(), "\n")
}

// githubOIDCOwnRoleEnvSuffix names the environment the role belongs to when the
// scan could attribute it, and says nothing at all when it could not. A guessed
// environment in a security warning sends somebody to edit the wrong file.
func githubOIDCOwnRoleEnvSuffix(r githubOIDCOwnUnrestrictedRole) string {
	if r.Env == "" {
		return ""
	}
	return fmt.Sprintf(" — environment %q", r.Env)
}

// githubOIDCOrgWideReport names every subject of ours that accepts a whole
// organisation, and says what each one hands out.
//
// Two cases, and the difference between them is the point of the tier. An
// org-wide subject somebody chose grants every repository in an organisation
// they presumably control. meroku's untouched default grants a THIRD-PARTY
// organisation — MadAppGang's own repositories — access to this AWS account,
// which is a different sentence and a different reaction. Reporting them with
// the same words would bury the one that matters.
func githubOIDCOrgWideReport(resp githubOIDCConflictsResponse, incomplete []githubOIDCDegraded, interactive bool) string {
	var b strings.Builder

	if githubOIDCAnyShippedDefault(resp.OwnOrgWideSubjects) {
		b.WriteString("🚨 This project still carries meroku's default GitHub OIDC subject, which\n")
		b.WriteString("   trusts a third-party organisation.\n")
	} else if len(resp.OwnOrgWideSubjects) == 1 {
		b.WriteString("🚨 One of this project's GitHub OIDC subjects trusts an entire organisation.\n")
	} else {
		fmt.Fprintf(&b, "🚨 %d of this project's GitHub OIDC subjects trust an entire organisation.\n",
			len(resp.OwnOrgWideSubjects))
	}

	for _, s := range resp.OwnOrgWideSubjects {
		fmt.Fprintf(&b, "\n   %s\n", strconv.Quote(s.Subject))
		if s.ShippedDefault {
			b.WriteString("      This is meroku's default, unchanged. It matches every token issued to\n")
			b.WriteString("      a workflow in MadAppGang's own repositories — an organisation that is\n")
			b.WriteString("      not yours. Unless you ARE MadAppGang, this grants a third party the\n")
			b.WriteString("      ability to assume this project's deploy role in YOUR AWS account.\n")
			continue
		}
		fmt.Fprintf(&b, "      It matches every repository in %s, on every branch, tag and pull\n",
			githubOIDCOrgLabel(s.Org))
		b.WriteString("      request — not only the repositories this project deploys from.\n")
	}

	b.WriteString("\n   What that means: any workflow matching one of the subjects above can assume\n")
	b.WriteString("   this project's GitHub Actions role, which grants iam:PassRole over its task\n")
	b.WriteString("   roles, ECR push and ecs:UpdateService — the whole deploy.\n")

	if n := len(resp.Conflicts); n > 0 {
		fmt.Fprintf(&b, "\n   The scan also found %s in this account whose subjects overlap this\n",
			githubOIDCPluralRoles(n))
		b.WriteString("   project's:\n")
		for _, c := range resp.Conflicts {
			fmt.Fprintf(&b, "      • %s%s\n", c.RoleName, githubOIDCOwnerSuffix(c))
		}
	}

	if len(incomplete) > 0 {
		b.WriteString("\n   The scan also did not finish, so there may be more than this:\n")
		githubOIDCWriteReasons(&b, incomplete, "      ")
	}

	b.WriteString("\n   Narrow workload.github_oidc_subjects to the repositories that actually\n")
	b.WriteString("   deploy this project — for example \"repo:your-org/your-repo:ref:refs/heads/main\".\n")

	if !interactive {
		b.WriteString("\n   There is no terminal to ask on, so the deploy stops here and nothing has\n")
		b.WriteString("   been applied. An organisation-wide subject can be deliberate, so this is a\n")
		b.WriteString("   question rather than a refusal — run the deploy from a terminal to answer it.\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// githubOIDCAnyShippedDefault reports whether any of these is meroku's untouched
// default. It decides the headline, because that case deserves the loudest line
// available even when other org-wide subjects sit beside it.
func githubOIDCAnyShippedDefault(subjects []githubOIDCOrgWideSubject) bool {
	for _, s := range subjects {
		if s.ShippedDefault {
			return true
		}
	}
	return false
}

// githubOIDCOrgLabel names the organisation, and says "that organisation" when
// the pattern wildcards the organisation segment too. Quoting a "*" back at
// somebody as if it were a name reads as a bug at the moment the tool is
// reporting the broadest pattern it has found.
func githubOIDCOrgLabel(org string) string {
	if org == "" {
		return "any matching organisation"
	}
	return "the " + strconv.Quote(org) + " organisation"
}

// githubOIDCConflictReport names every conflicting role, its owner where the
// tags gave one, and a concrete sub claim that reaches both roles.
func githubOIDCConflictReport(resp githubOIDCConflictsResponse, incomplete []githubOIDCDegraded, interactive bool) string {
	var b strings.Builder

	b.WriteString("🚨 Another project in this account trusts the same GitHub subjects.\n")

	for _, c := range resp.Conflicts {
		fmt.Fprintf(&b, "\n   %s%s\n", c.RoleName, githubOIDCOwnerSuffix(c))
		if c.Unrestricted {
			b.WriteString("      It pins no GitHub claim at all, so every repository on GitHub can\n")
			b.WriteString("      assume it — including every one of yours.\n")
		}
		for _, o := range c.Overlaps {
			fmt.Fprintf(&b, "      your %s overlaps its %s\n",
				strconv.Quote(o.OwnSubject), strconv.Quote(o.OtherSubject))
			fmt.Fprintf(&b, "         a token whose sub is %s assumes both roles\n",
				githubOIDCWitnessLabel(o.Witness))
		}
	}

	b.WriteString("\n   What that means: a workflow whose token carries that sub can assume this\n")
	b.WriteString("   project's GitHub Actions role AND the role above. The second role grants\n")
	b.WriteString("   iam:PassRole over its task roles, ECR push and ecs:UpdateService, so the\n")
	b.WriteString("   privilege boundary between the two projects is not there.\n")

	if len(incomplete) > 0 {
		b.WriteString("\n   The scan also did not finish, so there may be more than this:\n")
		githubOIDCWriteReasons(&b, incomplete, "      ")
	}

	if !interactive {
		b.WriteString("\n   There is no terminal to ask on, so the deploy stops here and nothing has\n")
		b.WriteString("   been applied. Narrow workload.github_oidc_subjects, or run the deploy from\n")
		b.WriteString("   a terminal where the overlap can be accepted deliberately.\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// githubOIDCUnverifiedReport is the "could not verify" line and its reasons.
//
// It exists to stop the provider check's ✅ being read as an overall verdict.
// The wording is deliberately about what was NOT established.
func githubOIDCUnverifiedReport(resp githubOIDCConflictsResponse, incomplete []githubOIDCDegraded) string {
	var b strings.Builder

	b.WriteString("   ⚠️  Could not verify whether another project in this account trusts the same\n")
	b.WriteString("      GitHub subjects. This is not a clean bill of health.\n")
	githubOIDCWriteReasons(&b, incomplete, "      ")

	if n := len(resp.UnevaluatedRoles); n > 0 {
		fmt.Fprintf(&b, "      • %s restrict access by claims other than sub, which this scan does\n",
			githubOIDCPluralRoles(n))
		b.WriteString("        not reason about\n")
	}

	b.WriteString("      The provider check above says nothing about subject overlap. Continuing.\n")
	return strings.TrimRight(b.String(), "\n")
}

// githubOIDCWriteReasons renders degraded entries as bullets.
func githubOIDCWriteReasons(b *strings.Builder, entries []githubOIDCDegraded, indent string) {
	for _, d := range entries {
		if d.Detail == "" {
			fmt.Fprintf(b, "%s• %s\n", indent, d.Reason)
			continue
		}
		fmt.Fprintf(b, "%s• %s: %s\n", indent, d.Reason, d.Detail)
	}
}

// githubOIDCOwnerSuffix names who owns the other role, and is honest about the
// difference between "it has no meroku tags" and "its tags could not be read".
// Attribution never decides whether a conflict is reported; it only decides who
// to go and talk to.
func githubOIDCOwnerSuffix(c githubOIDCConflict) string {
	switch {
	case c.OwnerProject != "" && c.OwnerEnv != "":
		return fmt.Sprintf(" — project %q, environment %q", c.OwnerProject, c.OwnerEnv)
	case c.OwnerProject != "":
		return fmt.Sprintf(" — project %q", c.OwnerProject)
	case c.OwnerEnv != "":
		return fmt.Sprintf(" — environment %q", c.OwnerEnv)
	case c.Attribution == githubOIDCAttributionUnavailable:
		return " — owner unknown, its tags could not be read"
	default:
		return " — owner unknown, it carries no meroku tags"
	}
}

// githubOIDCWitnessLabel renders a witness for a sentence.
//
// An empty witness is legitimate and means the two patterns intersect so widely
// that every subject satisfies both — "*" against "*". Rendering it as an empty
// pair of quotes, or worse as a blank gap mid-sentence, would read as a bug in
// the tool at exactly the moment the tool is telling the truth.
func githubOIDCWitnessLabel(w string) string {
	if w == "" {
		return "<any subject>"
	}
	return strconv.Quote(w)
}

// ---------------------------------------------------------------------------
// Printing and asking
// ---------------------------------------------------------------------------

// renderGithubOIDCReport applies the tier's colour. Plain text in, styled text
// out; nothing about the decision depends on it.
func renderGithubOIDCReport(d githubOIDCConflictDecision) string {
	switch d.Severity {
	case githubOIDCSaysOwnUnrestricted, githubOIDCSaysOwnOrgWide, githubOIDCSaysConflict:
		return lipgloss.NewStyle().Foreground(theme.Error).Render(d.Report)
	case githubOIDCSaysUnverified:
		return lipgloss.NewStyle().Foreground(theme.Warning).Render(d.Report)
	default:
		return d.Report
	}
}

// githubOIDCAskToContinue is the shared body of the confirmations.
//
// A select rather than a confirm, matching promptForSyncConsent, and with the
// refusal first: huh highlights the first option, so a bare Enter stops. The
// default has to be no. Somebody hammering Enter through a deploy must not
// accept a privilege boundary that is not there.
//
// A form that fails to run is not a yes either.
func githubOIDCAskToContinue(title, continueLabel string) bool {
	const (
		answerStop     = "stop"
		answerContinue = "continue"
	)

	var answer string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(
					huh.NewOption("Stop — nothing is applied; narrow the subjects and run again", answerStop),
					huh.NewOption(continueLabel, answerContinue),
				).
				Value(&answer),
		),
	)

	if err := form.Run(); err != nil {
		return false
	}
	return answer == answerContinue
}

// confirmGithubOIDCOverlap asks whether to deploy past a confirmed overlap.
var confirmGithubOIDCOverlap = func() bool {
	return githubOIDCAskToContinue(
		"Deploy anyway, past the subject overlap above?",
		"Deploy anyway — I accept that these subjects reach another project's role",
	)
}

// confirmGithubOIDCOrgWideSubject asks whether to deploy with a subject that
// accepts a whole organisation.
//
// A separate seam from confirmGithubOIDCOverlap, with its own words, because the
// two questions are not the same one. "I accept that these subjects reach
// another project's role" is false of an org-wide subject — there may be no
// other project at all — and a confirmation that misdescribes what is being
// accepted is worse than no confirmation, since it is answered anyway.
var confirmGithubOIDCOrgWideSubject = func() bool {
	return githubOIDCAskToContinue(
		"Deploy anyway, with a subject that trusts a whole GitHub organisation?",
		"Deploy anyway — I accept that every repository matched above can deploy this project",
	)
}

// ---------------------------------------------------------------------------
// The scan, from the deploy path
// ---------------------------------------------------------------------------

// scanGithubOIDCConflictsForCLI is the seam for the AWS-facing scan.
var scanGithubOIDCConflictsForCLI = githubOIDCScanFromAWS

// githubOIDCScanFromAWS builds the clients and runs the scan.
//
// It bounds the work with the same twenty seconds the HTTP handler uses.
// deploy.go hands down a context.Background() with no deadline, so without this
// the pagination and the pattern DP would run unbounded on the one code path
// where a user is waiting at a prompt.
//
// A client that will not build is a scan that did not happen, so it comes back
// as a degraded response rather than an error: the caller renders three
// outcomes and an error is only one of them.
func githubOIDCScanFromAWS(ctx context.Context, e Env) githubOIDCConflictsResponse {
	ctx, cancel := context.WithTimeout(ctx, githubOIDCConflictScanTimeout)
	defer cancel()

	iamClient, stsClient, err := newAWSClientsForEnv(ctx, e)
	if err != nil {
		resp := newGithubOIDCConflictsResponse()
		resp.AccountID = e.AccountID
		resp.OwnSubjects, resp.OwnSubjectsSource = githubOIDCConfiguredSubjects(e, nil)
		resp.degrade(githubOIDCReasonNoCredentials, summarizeAWSError(err))
		return resp
	}

	return scanGitHubSubjectConflicts(ctx, e, nil, iamClient, stsClient)
}

// checkGithubOIDCSubjectConflicts runs the scan, reports it, and returns
// whether the deploy must stop.
//
// Interactivity is read once, through the repository's existing helper —
// interactiveTerminal in app/state_reconnect.go, which requires a TTY on both
// stdin and stdout and treats a set CI variable as non-interactive whatever the
// file descriptors say.
func checkGithubOIDCSubjectConflicts(ctx context.Context, e Env) bool {
	resp := scanGithubOIDCConflictsForCLI(ctx, e)
	decision := decideGithubOIDCConflicts(resp, terminalIsInteractive())

	if decision.Report != "" {
		fmt.Println()
		fmt.Println(renderGithubOIDCReport(decision))
	}

	if !decision.Prompt {
		return decision.Block
	}

	if githubOIDCConfirmFor(decision.Severity)() {
		fmt.Println("   Continuing at your confirmation. The finding above is unchanged.")
		return false
	}
	return true
}

// githubOIDCConfirmFor picks the confirmation that matches the tier being
// confirmed. Routing on severity rather than inside the decision keeps
// decideGithubOIDCConflicts pure and keeps each prompt's words next to the
// finding they describe.
func githubOIDCConfirmFor(severity githubOIDCSeverity) func() bool {
	if severity == githubOIDCSaysOwnOrgWide {
		return confirmGithubOIDCOrgWideSubject
	}
	return confirmGithubOIDCOverlap
}
