package main

// Reading what one IAM role's trust policy grants to GitHub Actions.
//
// A meroku GitHub Actions role is fenced off from every other project by
// exactly one thing: the sub condition on this document. To find two projects
// in an account whose fences overlap, we first have to read the fence — and the
// only honest way to read it is to answer in five values, not two:
//
//	IsGitHub=false                 this role has nothing to do with GitHub
//	IsGitHub, Subjects=[...]       it trusts these subject patterns
//	IsGitHub, Unrestricted         it trusts every repository on GitHub
//	IsGitHub, RestrictedByOther..  it is fenced, but not by a fence I can read
//	error                          I could not read it
//
// The fourth value exists because sub is not the only claim a trust policy can
// pin. A role conditioned on token.actions.githubusercontent.com:repository_owner
// or :job_workflow_ref is properly hardened and simply not expressible as a
// subject glob. Folding it into Unrestricted would fire this feature's loudest
// alert — "this role trusts all of GitHub" — at a role that trusts one
// organisation, and a security check that cries wolf on correct configuration
// is a security check people learn to click past. Folding it into "no conflict"
// would be the opposite lie. It gets its own answer.
//
// getTrustedAccountsFromRole (app/dns_setup_tui.go) answers in one value: it
// returns an empty slice on a bad decode, a bad unmarshal and an unexpected
// shape alike, so "this role trusts nobody" and "I could not read this role"
// are the same answer. That is the bug isIAMNotFound exists to prevent, one
// layer up. A scan built on it would report "no conflicts" for a document it
// never understood, and an empty conflicts list rendered as reassurance is the
// worst thing this feature can ship. So: the url-decode sniff and the
// string-or-list technique are reused, the error handling is not.
//
// The other half of the job is that IAM's grammar is looser than the documents
// people actually write. Every place a JSON list is legal, a bare scalar is
// legal too; the condition operator and the condition key are matched
// case-insensitively by IAM and may carry a set-operator prefix. Every one of
// those looser forms, read literally, degrades to "no subject condition" —
// which this parser reports as Unrestricted, the loudest verdict it has. A
// parser that is stricter than IAM does not fail quietly here. It cries wolf on
// a correctly configured role.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

const (
	// githubOIDCProviderARNSuffix is how a GitHub Actions provider appears as a
	// Federated principal. The account digits differ per account and the
	// partition differs in GovCloud and China, so the suffix is the only stable
	// part and matching on it is deliberate.
	githubOIDCProviderARNSuffix = ":oidc-provider/" + githubOIDCIssuerHost

	// githubOIDCClaimPrefix prefixes every condition key that pins a claim out
	// of a GitHub token: :sub, :repository, :repository_owner,
	// :job_workflow_ref, :environment, :aud and the rest. Compared
	// case-insensitively, because IAM compares condition keys that way and an
	// exact lookup against a document that spells one differently reads as "no
	// condition at all".
	githubOIDCClaimPrefix = githubOIDCIssuerHost + ":"

	// githubOIDCSubClaim is the claim this scan can actually reason about: it
	// is a glob, and globs intersect.
	githubOIDCSubClaim = "sub"

	// githubOIDCAudClaim is present in every GitHub trust policy AWS's own
	// documentation shows, and it pins the audience rather than the caller. It
	// restricts nothing about *which repository* may assume the role, so it
	// does not count when deciding whether a statement is fenced.
	githubOIDCAudClaim = "aud"

	// assumeRoleWithWebIdentity is the only action that lets a GitHub token
	// become this role, lowercased for case-insensitive comparison.
	assumeRoleWithWebIdentity = "sts:assumerolewithwebidentity"
)

// githubTrustGrant is what one IAM role's trust policy grants to GitHub Actions.
type githubTrustGrant struct {
	// IsGitHub is true when some Allow statement federates against
	// oidc-provider/token.actions.githubusercontent.com.
	IsGitHub bool
	// Subjects is the union of sub patterns across Allow statements.
	Subjects []string
	// Unrestricted is true when the role is a GitHub role that pins no GitHub
	// claim at all beyond :aud: it trusts every repository on GitHub.
	Unrestricted bool
	// RestrictedByOtherClaims is true when some Allow statement has no sub
	// condition but does constrain another GitHub claim (:repository,
	// :repository_owner, :job_workflow_ref, :environment), or constrains sub
	// negatively with StringNotLike. The role is NOT unrestricted, but this
	// scan cannot reason about the overlap either — it only understands sub.
	// The caller must treat it as "cannot evaluate": neither "trusts
	// everything" nor "no conflict".
	RestrictedByOtherClaims bool
	// OtherClaimKeys names the claims that made RestrictedByOtherClaims true,
	// lowercased with the token.actions.githubusercontent.com: prefix stripped,
	// so a message can say which fence it could not read.
	OtherClaimKeys []string
}

// parseGitHubTrustPolicy reads a role's AssumeRolePolicyDocument.
//
// A zero grant with a nil error means one specific thing — this document was
// understood and it federates nothing to GitHub. Anything not understood is an
// error, so no caller can mistake a failed read for an empty one.
func parseGitHubTrustPolicy(doc string) (githubTrustGrant, error) {
	decoded, err := decodePolicyDocument(doc)
	if err != nil {
		return githubTrustGrant{}, err
	}

	var policy map[string]json.RawMessage
	if err := json.Unmarshal([]byte(decoded), &policy); err != nil {
		return githubTrustGrant{}, fmt.Errorf("trust policy is not a JSON object: %w", err)
	}

	raw, ok := policy["Statement"]
	if !ok {
		return githubTrustGrant{}, fmt.Errorf("trust policy has no Statement")
	}

	// Statement is a list, or one statement written bare. Copying the existing
	// helper, which only ever reads it as a list, would classify a perfectly
	// valid singleton-Statement GitHub role as "not a GitHub role" — silently,
	// which is the failure mode this whole file is arranged against.
	statements, err := policyList(raw)
	if err != nil {
		return githubTrustGrant{}, fmt.Errorf("trust policy Statement: %w", err)
	}

	var grant githubTrustGrant
	seenSubject := map[string]bool{}
	seenClaim := map[string]bool{}

	for i, rawStmt := range statements {
		var stmt map[string]json.RawMessage
		if err := json.Unmarshal(rawStmt, &stmt); err != nil {
			return githubTrustGrant{}, fmt.Errorf("trust policy statement %d is not an object: %w", i, err)
		}

		relevant, err := statementFederatesGitHub(stmt)
		if err != nil {
			return githubTrustGrant{}, fmt.Errorf("trust policy statement %d: %w", i, err)
		}
		if !relevant {
			continue
		}

		claims, err := statementConditions(stmt)
		if err != nil {
			return githubTrustGrant{}, fmt.Errorf("trust policy statement %d: %w", i, err)
		}

		grant.IsGitHub = true

		switch {
		case claims.hasSub:
			// A sub condition is what this scan evaluates. Any other claim on
			// the same statement only narrows it further, so it changes no
			// verdict and is deliberately not recorded: reporting "cannot
			// evaluate" alongside subjects we can evaluate would be noise.
			for _, s := range claims.subjects {
				if seenSubject[s] {
					continue
				}
				seenSubject[s] = true
				grant.Subjects = append(grant.Subjects, s)
			}

		case len(claims.otherClaims) > 0:
			// Fenced, but by a claim that is not a glob over repositories. Not
			// unrestricted and not evaluable — see the file comment.
			grant.RestrictedByOtherClaims = true
			for _, c := range claims.otherClaims {
				if seenClaim[c] {
					continue
				}
				seenClaim[c] = true
				grant.OtherClaimKeys = append(grant.OtherClaimKeys, c)
			}

		default:
			// No GitHub claim pinned at all. The classic misconfiguration and
			// the highest-value thing the scan can find: this statement lets
			// every repository on GitHub assume the role, so it overlaps every
			// other project by construction.
			grant.Unrestricted = true
		}
	}

	return grant, nil
}

// decodePolicyDocument undoes the percent-encoding IAM applies on some paths.
//
// GetRole returns the document encoded; ListRoles has been observed both ways,
// so the sniff is on the content rather than on which call produced it. Same
// technique as getTrustedAccountsFromRole: a raw policy document cannot contain
// a literal "%7B" or "%22" outside a string, so their presence is decisive.
func decodePolicyDocument(doc string) (string, error) {
	if strings.TrimSpace(doc) == "" {
		return "", fmt.Errorf("trust policy document is empty")
	}
	if !strings.Contains(doc, "%7B") && !strings.Contains(doc, "%22") {
		return doc, nil
	}
	decoded, err := url.QueryUnescape(doc)
	if err != nil {
		return "", fmt.Errorf("trust policy document is not valid percent-encoding: %w", err)
	}
	return decoded, nil
}

// statementFederatesGitHub reports whether this statement is an Allow that
// grants GitHub Actions the right to assume the role.
//
// A statement is skipped, not rejected, when it is understood and irrelevant:
// a Deny, a NotPrincipal or NotAction whose inverted semantics this parser does
// not model, a non-GitHub principal, or an action list that does not include
// assume-role-with-web-identity. That last one is not pedantry. Policies
// commonly carry a second statement with the same Federated principal granting
// sts:TagSession alone and no Condition; counted as a grant, it would report
// Unrestricted on a correctly fenced role.
func statementFederatesGitHub(stmt map[string]json.RawMessage) (bool, error) {
	effect, err := policyString(stmt["Effect"])
	if err != nil {
		return false, fmt.Errorf("Effect: %w", err)
	}
	switch strings.ToLower(effect) {
	case "deny":
		return false, nil
	case "allow":
	default:
		return false, fmt.Errorf("Effect is %q, expected Allow or Deny", effect)
	}

	// NotPrincipal and NotAction invert the set being described. Modelling them
	// wrongly is worse than declining to model them, and neither appears in a
	// role meroku generates.
	if _, ok := stmt["NotPrincipal"]; ok {
		return false, nil
	}
	if _, ok := stmt["NotAction"]; ok {
		return false, nil
	}

	if !principalFederatesGitHub(stmt["Principal"]) {
		return false, nil
	}

	// Only now is Action worth insisting on: a GitHub-federated statement with
	// no Action grants nothing and is not a document we can reason about.
	rawAction, ok := stmt["Action"]
	if !ok {
		return false, fmt.Errorf("statement federates GitHub but has no Action")
	}
	actions, err := policyStrings(rawAction)
	if err != nil {
		return false, fmt.Errorf("Action: %w", err)
	}
	for _, a := range actions {
		// Actions are globs in IAM ("sts:*", "*"), matched case-insensitively.
		if globMatch([]byte(strings.ToLower(a)), []byte(assumeRoleWithWebIdentity)) {
			return true, nil
		}
	}
	return false, nil
}

// principalFederatesGitHub reports whether Principal.Federated names the
// account's GitHub Actions OIDC provider.
//
// The provider ARN is the identity, never the role's name: a role called
// "deploy" is a GitHub role if it federates against the provider, and one
// called "github-actions-role" is not if it does not.
//
// A Principal that is a bare string ("*"), absent, or carries only AWS or
// Service keys is understood and simply is not GitHub, so it is not an error.
// Nor is a Federated value that is neither a string nor a list of them: it
// cannot be a provider ARN, so "not GitHub" is provably the right answer rather
// than a guess, and this is the one place a shape we cannot read is not a
// failure to read.
func principalFederatesGitHub(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var principal map[string]json.RawMessage
	if err := json.Unmarshal(raw, &principal); err != nil {
		return false
	}
	federated, ok := principal["Federated"]
	if !ok {
		return false
	}
	// Federated is one ARN, or a list of them. Both forms are legal and both
	// occur; reading only the list form misses the common single-provider case.
	arns, err := policyStrings(federated)
	if err != nil {
		return false
	}
	for _, arn := range arns {
		if strings.HasSuffix(arn, githubOIDCProviderARNSuffix) {
			return true
		}
	}
	return false
}

// statementClaims is which GitHub claims one Allow statement pins.
type statementClaims struct {
	// subjects are the sub patterns, when sub is pinned positively.
	subjects []string
	// hasSub reports a positive sub condition, the only fence this scan reads.
	hasSub bool
	// otherClaims names every GitHub claim that fences the statement in a way
	// this scan cannot intersect, lowercased and prefix-stripped.
	otherClaims []string
}

// statementConditions reads one statement's Condition block.
//
// The three outcomes it separates are the reason this function exists rather
// than a bare lookup of the sub key:
//
//	hasSub                             evaluable
//	!hasSub, len(otherClaims) > 0      fenced by something else; not evaluable
//	!hasSub, no claims                 genuinely trusts all of GitHub
//
// :aud is ignored throughout. Every trust policy AWS documents carries it, it
// pins the audience rather than the caller, and counting it as a fence would
// mean this parser never reports the misconfiguration it exists to find.
func statementConditions(stmt map[string]json.RawMessage) (statementClaims, error) {
	var claims statementClaims

	raw, ok := stmt["Condition"]
	if !ok {
		return claims, nil
	}
	var conditions map[string]json.RawMessage
	if err := json.Unmarshal(raw, &conditions); err != nil {
		return statementClaims{}, fmt.Errorf("Condition is not an object: %w", err)
	}

	// Walk operators and keys in a fixed order. Go randomises map iteration, and
	// a statement pinning several claims would otherwise report them in a
	// different order on every run — straight into a warning a user is meant to
	// compare against the last one.
	for _, operator := range sortedMapKeys(conditions) {
		var block map[string]json.RawMessage
		if err := json.Unmarshal(conditions[operator], &block); err != nil {
			return statementClaims{}, fmt.Errorf("Condition %q is not an object: %w", operator, err)
		}

		for _, key := range sortedMapKeys(block) {
			// IAM compares condition keys case-insensitively, so a document
			// spelling it Token.Actions.GitHubUserContent.com:Sub fences the
			// role exactly as much as the canonical spelling does. An exact map
			// lookup would find nothing, report Unrestricted, and raise the
			// loudest alarm this parser has against a correctly fenced role.
			if len(key) <= len(githubOIDCClaimPrefix) || !strings.EqualFold(key[:len(githubOIDCClaimPrefix)], githubOIDCClaimPrefix) {
				// Not a GitHub claim. aws:SourceIp and friends live here, and
				// they say nothing about which repository may assume the role.
				continue
			}
			claim := strings.ToLower(key[len(githubOIDCClaimPrefix):])

			if claim == githubOIDCAudClaim {
				continue
			}

			// Any claim other than sub — :repository, :repository_owner,
			// :job_workflow_ref, :environment — is a real fence that is not a
			// repository glob. So is a negative operator on sub: StringNotLike
			// carves a hole rather than drawing a boundary, and there is no
			// pattern to intersect. Both are recorded, neither is guessed at.
			if claim != githubOIDCSubClaim || !subOperatorIsSupported(operator) {
				claims.otherClaims = append(claims.otherClaims, claim)
				continue
			}

			// The value is one pattern or a list of them.
			values, err := policyStrings(block[key])
			if err != nil {
				return statementClaims{}, fmt.Errorf("Condition %q on %s: %w", operator, key, err)
			}
			// StringEquals counts as much as StringLike: it is a glob that
			// happens to carry no metacharacters, and the intersection engine
			// handles a literal correctly with no translation or escaping.
			claims.subjects = append(claims.subjects, values...)
			claims.hasSub = true
		}
	}

	return claims, nil
}

// subOperatorIsSupported reports whether an operator pins sub positively, in a
// way this parser models as a glob list.
//
// ForAllValues: and ForAnyValue: are set operators that prefix the real one. An
// unstripped comparison skips the condition entirely and reports the role as
// unrestricted, which is a false positive on a policy written the way AWS's own
// documentation writes multi-value conditions. The IfExists suffix is stripped
// for the same reason and is safe to ignore here: a GitHub token always carries
// a sub claim, so ...IfExists on sub always fires.
func subOperatorIsSupported(operator string) bool {
	op := strings.ToLower(operator)
	if i := strings.LastIndex(op, ":"); i >= 0 {
		op = op[i+1:]
	}
	op = strings.TrimSuffix(op, "ifexists")
	return op == "stringlike" || op == "stringequals"
}

// policyList normalises IAM's singleton-or-list into a list.
//
// IAM permits a bare object anywhere a list of objects is legal, and documents
// in the wild use both. Handling only the list form does not fail — it finds
// nothing, which is the shape of every bug this file is written to avoid.
func policyList(raw json.RawMessage) ([]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("is missing")
	}
	switch trimmed[0] {
	case '[':
		var list []json.RawMessage
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return nil, fmt.Errorf("is not a well-formed list: %w", err)
		}
		return list, nil
	case '{':
		return []json.RawMessage{trimmed}, nil
	default:
		return nil, fmt.Errorf("is neither a list nor an object")
	}
}

// policyStrings normalises IAM's singleton-or-list of strings into a slice.
func policyStrings(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("is missing")
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	var one string
	if err := json.Unmarshal(raw, &one); err != nil {
		return nil, fmt.Errorf("is neither a string nor a list of strings: %w", err)
	}
	return []string{one}, nil
}

// sortedMapKeys gives a JSON object's keys in a fixed order, so what this
// parser reports does not depend on Go's randomised map iteration.
func sortedMapKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// policyString reads a required scalar string field.
func policyString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("is missing")
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("is not a string: %w", err)
	}
	return s, nil
}
