package boundary_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// data "aws_iam_policy_document" "default_ecr_policy" — the ECR REPOSITORY
// policy, which is the one an attacker reads.
//
// Its first statement grants ecr:PutImage, ecr:BatchDeleteImage,
// ecr:DeleteRepository and ecr:SetRepositoryPolicy, and it carried
// `type = "*"` / `identifiers = ["*"]` with no condition. An unconditioned
// wildcard principal on a resource policy is not a loose default, it is public:
// every principal in every AWS account could overwrite the image behind a tag
// this infrastructure pulls — code execution here at the next task start — or
// delete the repository. It reached production in three modules.
//
// WHY A GO TEXT GUARD AND NOT A .tftest.hcl. The honest answer is that the
// stronger mechanism is unavailable here. github_policy proves its property by
// rendering real JSON under `terraform test`, which works because that module
// is a credential-free leaf. This document is not: it sits in a file beside
// `data "aws_organizations_organization" "org"` and resolves
// `data "aws_caller_identity" "current"`, and BOTH are live AWS calls. A plan
// of any of the three modules needs credentials and network, so there is no
// rendered JSON to assert on without them. Extracting the document into a leaf
// module the way github_policy was extracted would fix that and is the right
// eventual shape — see the note on consolidation below — but it is a refactor
// of three deployed modules, not a guard.
//
// WHAT IS ASSERTED, in three layers, because the failure mode being guarded is
// not only "the wildcard comes back" but "the wildcard comes back in ONE of
// three copies":
//
//  1. TestDefaultECRPolicyFirstStatementIsAccountScoped — positional. The FIRST
//     statement of each copy names this account's root and no wildcard.
//  2. TestDefaultECRPolicyWildcardPrincipalsAreConfined — semantic, and it
//     survives a reorder or a rename that defeats (1). ANY statement holding a
//     wildcard principal must be read-only AND carry aws:PrincipalOrgID.
//  3. TestDefaultECRPolicyCopiesHaveNotDrifted — the three documents are one
//     document, so a fix applied to one file is not enough to go green.
//
// (2) is why this file does not simply search for `identifiers = ["*"]`. The
// SECOND statement legitimately has one: it is read-only and confined to this
// AWS Organization. A file-wide ban on wildcard principals would fail on
// correct code, which is the fastest way to get a guard deleted.
// ---------------------------------------------------------------------------

// The three byte-identical copies, keyed by the repository-relative path a
// failure message should print. Values are relative to this package's directory
// (modules/workloads/ci_lambda/internal/boundary): three levels up reaches
// modules/workloads, four reaches modules/ and so the sibling modules.
//
// workloadsTF() in lambdatf_guard_test.go cannot be reused — it is hardcoded to
// three levels up — but its comment-stripping convention is, and here it is
// load-bearing for the same reason: the prose now standing above each of these
// statements quotes `type = "*"` and `identifiers = ["*"]` verbatim while
// explaining why they must not appear.
var defaultECRPolicyCopies = map[string]string{
	"modules/workloads/ecr.tf":               filepath.Join("..", "..", "..", "ecr.tf"),
	"modules/ecs_task/variable.tf":           filepath.Join("..", "..", "..", "..", "ecs_task", "variable.tf"),
	"modules/event_bridge_task/variables.tf": filepath.Join("..", "..", "..", "..", "event_bridge_task", "variables.tf"),
}

// Actions that mutate the repository or its contents. A statement granting any
// of these to an unknown principal is the whole vulnerability.
var ecrWriteActions = []string{
	"ecr:PutImage",
	"ecr:InitiateLayerUpload",
	"ecr:UploadLayerPart",
	"ecr:CompleteLayerUpload",
	"ecr:DeleteRepository",
	"ecr:BatchDeleteImage",
	"ecr:SetRepositoryPolicy",
	"ecr:DeleteRepositoryPolicy",
}

const (
	defaultECRPolicyHeader = `data "aws_iam_policy_document" "default_ecr_policy"`
	callerIdentityRef      = "data.aws_caller_identity.current.account_id"
)

// readTFStripped reads a .tf file at a path relative to this package and blanks
// its comment lines.
func readTFStripped(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(rel)
	require.NoErrorf(t, err, "reading %s", rel)
	return commentLine.ReplaceAllString(string(b), "")
}

// matchBrace returns the contents of the brace-delimited block whose opening
// brace is the first one at or after from, and the offset just past its close.
//
// Counting braces rather than pattern-matching the block: HCL nests, and the
// interpolation this fix introduces — ${data.aws_caller_identity...} — is
// itself a balanced pair, so it costs nothing here and a regexp would have to
// know about it.
func matchBrace(t *testing.T, src string, from int) (string, int) {
	t.Helper()

	open := strings.IndexByte(src[from:], '{')
	require.NotEqualf(t, -1, open, "no opening brace at or after offset %d", from)
	open += from

	depth := 0
	for i := open; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[open+1 : i], i + 1
			}
		}
	}
	t.Fatalf("unbalanced braces starting at offset %d", open)
	return "", 0
}

// blockBodies returns the body of every `name { ... }` block that begins a line
// in src, in source order.
func blockBodies(t *testing.T, src, name string) []string {
	t.Helper()

	header := regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(name) + `[ \t]*\{`)
	var out []string
	for from := 0; from < len(src); {
		loc := header.FindStringIndex(src[from:])
		if loc == nil {
			break
		}
		body, end := matchBrace(t, src, from+loc[0])
		out = append(out, body)
		from = end
	}
	return out
}

// defaultECRPolicyStatements returns the statement blocks of the
// default_ecr_policy document in the named copy, in source order.
func defaultECRPolicyStatements(t *testing.T, label, rel string) []string {
	t.Helper()

	src := readTFStripped(t, rel)

	i := strings.Index(src, defaultECRPolicyHeader)
	require.NotEqualf(t, -1, i, "%s no longer declares %s — if it moved, this guard must move with it", label, defaultECRPolicyHeader)
	require.Equalf(t, -1, strings.Index(src[i+len(defaultECRPolicyHeader):], defaultECRPolicyHeader),
		"%s declares %s more than once; this guard reads only the first", label, defaultECRPolicyHeader)

	doc, _ := matchBrace(t, src, i)
	stmts := blockBodies(t, doc, "statement")
	require.NotEmptyf(t, stmts, "%s: default_ecr_policy has no statement blocks", label)
	return stmts
}

// hasWildcardPrincipal reports whether any principals block in a statement body
// is the unconditioned-everyone form.
func hasWildcardPrincipal(t *testing.T, stmt string) bool {
	t.Helper()

	for _, p := range blockBodies(t, stmt, "principals") {
		if strings.Contains(p, `type        = "*"`) || strings.Contains(p, `type = "*"`) {
			return true
		}
		if strings.Contains(p, `identifiers = ["*"]`) {
			return true
		}
	}
	return false
}

// TestDefaultECRPolicyFirstStatementIsAccountScoped pins the fix itself: the
// write-granting first statement of every copy is delegated to this account's
// root and reaches no one outside it.
//
// The account-root form is a delegation, not a grant — a same-account caller is
// still allowed only if its own identity policy allows the action. Nothing lost
// access to these repositories, because nothing was leaning on the resource
// policy alone: every ECS execution role attaches
// AmazonECSTaskExecutionRolePolicy for the pull actions, and the GitHub Actions
// deploy role gets its push actions from modules/github_policy.
func TestDefaultECRPolicyFirstStatementIsAccountScoped(t *testing.T) {
	for label, rel := range defaultECRPolicyCopies {
		t.Run(label, func(t *testing.T) {
			first := defaultECRPolicyStatements(t, label, rel)[0]

			require.Falsef(t, hasWildcardPrincipal(t, first),
				"%s: the FIRST statement of default_ecr_policy has an unconditioned wildcard principal.\n"+
					"On a resource policy that is public: it grants %v to every principal in every AWS "+
					"account, which is image replacement — code execution at the next task start — or "+
					"repository deletion. Scope it to "+
					"arn:aws:iam::${%s}:root.\n\nStatement:\n%s",
				label, ecrWriteActions, callerIdentityRef, first)

			require.Containsf(t, first, callerIdentityRef,
				"%s: the FIRST statement of default_ecr_policy no longer names this account.\n"+
					"Its principal must be arn:aws:iam::${%s}:root — a literal account ID must never be "+
					"committed to this public repository.\n\nStatement:\n%s",
				label, callerIdentityRef, first)

			require.Containsf(t, first, `type        = "AWS"`,
				"%s: the FIRST statement of default_ecr_policy must use an AWS principal type.\n\nStatement:\n%s",
				label, first)
		})
	}
}

// TestDefaultECRPolicyWildcardPrincipalsAreConfined is the assertion that
// survives an edit clever enough to defeat the positional one above. It does
// not care which statement is first, what its sid says, or how many statements
// there are: wherever a wildcard principal appears in this document it must be
// read-only AND confined to this AWS Organization.
//
// The second statement passes today and is meant to. Its wildcard is paired
// with aws:PrincipalOrgID and a five-action read list, which is exactly the
// guard the first statement was missing.
func TestDefaultECRPolicyWildcardPrincipalsAreConfined(t *testing.T) {
	for label, rel := range defaultECRPolicyCopies {
		t.Run(label, func(t *testing.T) {
			for i, stmt := range defaultECRPolicyStatements(t, label, rel) {
				if !hasWildcardPrincipal(t, stmt) {
					continue
				}

				require.Containsf(t, stmt, "aws:PrincipalOrgID",
					"%s: statement %d of default_ecr_policy has a wildcard principal and no "+
						"aws:PrincipalOrgID condition. That is a public grant on a resource "+
						"policy.\n\nStatement:\n%s",
					label, i, stmt)

				for _, action := range ecrWriteActions {
					require.NotContainsf(t, stmt, `"`+action+`"`,
						"%s: statement %d of default_ecr_policy grants the write action %s to a "+
							"wildcard principal. An organization condition confines who, not what: "+
							"any account in the org could then replace a running image. Wildcard "+
							"statements here stay read-only.\n\nStatement:\n%s",
						label, i, action, stmt)
				}
			}
		})
	}
}

// TestDefaultECRPolicyCopiesHaveNotDrifted is the reason this file guards three
// paths rather than one. The document is copy-pasted into three modules, and the
// realistic regression is not that someone re-adds the wildcard on purpose — it
// is that someone edits the copy their change happens to touch and leaves the
// other two behind, exactly as the original wildcard was fixed nowhere for as
// long as it existed everywhere.
//
// modules/naming and app/aws_name.go are this repository's precedent: duplicated
// logic pinned together by a shared assertion, because copies drift.
func TestDefaultECRPolicyCopiesHaveNotDrifted(t *testing.T) {
	space := regexp.MustCompile(`\s+`)
	normalize := func(stmts []string) string {
		return space.ReplaceAllString(strings.Join(stmts, "\n"), " ")
	}

	var refLabel, refDoc string
	for label, rel := range defaultECRPolicyCopies {
		doc := normalize(defaultECRPolicyStatements(t, label, rel))
		if refDoc == "" {
			refLabel, refDoc = label, doc
			continue
		}
		require.Equalf(t, refDoc, doc,
			"default_ecr_policy has drifted between %s and %s.\n"+
				"These three copies must stay one document: a security fix applied to one of "+
				"them leaves the other two deployed with the defect. If the divergence is "+
				"intentional, extract the document into a shared module instead.",
			refLabel, label)
	}
}

// TestDefaultECRPolicyModulesDeclareCallerIdentity closes the gap between the
// guard above and a module that will not plan. The account-scoped principal
// interpolates data.aws_caller_identity.current, and each of the three modules
// must declare that data source somewhere in its own directory — a module
// cannot borrow a sibling's. `terraform validate` catches this too, but it is
// not what runs on every `go test ./...`, and a guard that passes while the
// module is unplannable is worse than no guard.
func TestDefaultECRPolicyModulesDeclareCallerIdentity(t *testing.T) {
	for label, rel := range defaultECRPolicyCopies {
		t.Run(label, func(t *testing.T) {
			paths, err := filepath.Glob(filepath.Join(filepath.Dir(rel), "*.tf"))
			require.NoError(t, err)
			require.NotEmptyf(t, paths, "%s: no .tf files beside it", label)

			declared := false
			for _, p := range paths {
				if strings.Contains(readTFStripped(t, p), `data "aws_caller_identity" "current"`) {
					declared = true
					break
				}
			}
			require.Truef(t, declared,
				"the module containing %s references %s but declares no "+
					"`data \"aws_caller_identity\" \"current\"`; the module will not plan",
				label, callerIdentityRef)
		})
	}
}
