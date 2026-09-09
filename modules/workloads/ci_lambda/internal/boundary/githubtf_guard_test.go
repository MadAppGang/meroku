package boundary_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// The TOTAL permission surface of the GitHub Actions deploy role.
//
// The ECR scoping property itself is not here any more, and this file used to
// be four times its current size because it was. It asserted, as text over
// modules/workloads/github.tf, that a policy statement consumed two locals,
// that eight action names were present, and that three ARN patterns were
// spelled a particular way — every one of those strings copied out of the file
// it was searching, so every assertion was true by construction on the day it
// was written and could only ever detect an EDIT, never a wrongness. It also
// rejected two correct refactors outright.
//
// modules/github_policy now renders the real policy under `terraform test` with
// no credentials and no network, and
// modules/github_policy/tests/ecr_scope.tftest.hcl asserts the requirement
// against that JSON by MATCHING a candidate repository ARN — acmecorp_backend,
// circl_backend, the account's real unprefixed `backend` — against every
// Resource granted. That is a strictly stronger statement than any text search
// over the source, and it retires all four of the tests that were here.
//
// TWO PROPERTIES SURVIVE IT, and they are the two the leaf module structurally
// cannot see. It renders ONE document and knows nothing about (a) what else is
// attached to the role that carries it, or (b) what values its caller passes in.
// Both are absences and mis-wirings rather than values, so no `terraform test`
// anywhere can express them — you cannot plan a resource that is not there, and
// a module whose inputs are strings cannot tell which expression produced them.
//
//	1. aws_iam_role_policy.github_access is the ONLY policy on the role, and it
//	   is module.github_policy.json.        (TestGithubRoleCarriesExactlyOneScopedPolicy)
//	   Permissions may equally be hung on the aws_iam_role block itself, via
//	   managed_policy_arns or inline_policy, which no count of sibling resources
//	   sees.                                (TestGithubRoleGrantsNothingOnItsOwnBlock)
//	2. The module is handed THIS project, THIS account and THIS region.
//	                                       (TestGithubPolicyModuleIsWiredToThisAccountAndRegion)
//
// Both name real edits, not hypotheticals. Attaching
// AmazonEC2ContainerRegistryPowerUser and repointing the role at a wider
// document are the two things an engineer does when a deploy fails with
// AccessDenied, and either restores account-wide ECR write on top of a perfectly
// scoped document with the entire Terraform suite green. Passing
// var.ecr_account_region — which defaults to "" — where the current region
// belongs yields ARNs with an empty region field: valid syntax, a real-looking
// plan diff, and a grant that matches nothing in any account.
//
// The convention inherited from lambdatf_guard_test.go is load-bearing:
// workloadsTF() blanks comment lines before anything here reads the source, and
// github.tf's comments name `AmazonEC2ContainerRegistryPowerUser` and
// `var.ecr_account_id` explicitly while explaining why neither may appear. A
// naive search over the raw bytes would match the prose that warns against the
// defect and pass while the defect is present.
// ---------------------------------------------------------------------------

// The role every assertion below is about, and the module that must be the sole
// source of its permissions. Spelled once so a rename surfaces as a failure here
// rather than as a silently-skipped test.
const (
	githubRoleRef   = "aws_iam_role.github_role"
	githubPolicyRef = "module.github_policy.json"
)

// workloadsTFFiles returns every .tf file in modules/workloads, comments
// blanked, keyed by file name.
//
// The whole directory rather than github.tf alone, and that is the assertion
// rather than thoroughness for its own sake: a policy attachment is a top-level
// resource that may be declared in ANY file of the module and still land on the
// role. A guard that only reads github.tf can be defeated by putting the
// attachment in ecr.tf, which is also the most natural place someone would put
// it.
func workloadsTFFiles(t *testing.T) map[string]string {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "*.tf"))
	require.NoError(t, err)
	require.NotEmpty(t, paths, "no .tf files found in modules/workloads")

	out := map[string]string{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		require.NoError(t, err)
		out[filepath.Base(p)] = commentLine.ReplaceAllString(string(b), "")
	}
	return out
}

// TestGithubRoleCarriesExactlyOneScopedPolicy is the assertion no terraform test
// can make, because what it forbids is an ABSENCE: a resource that is not there.
//
// modules/github_policy/tests/ proves the document the role receives stops at
// this project's ECR namespace. It proves nothing about the role, which is an
// IAM principal whose effective permissions are the UNION of every policy
// attached to it. One `aws_iam_role_policy_attachment` to
// `arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryPowerUser` — three lines,
// and the first result for "github actions ecr access denied" — restores
// account-wide ECR write, including every other meroku project's repositories,
// while the scoped document sits underneath it unchanged and every assertion
// about it still passes. Repointing `policy` at a second, wider document does
// the same thing and leaves the guarded one in the file, correct and orphaned.
func TestGithubRoleCarriesExactlyOneScopedPolicy(t *testing.T) {
	files := workloadsTFFiles(t)

	// Half one: exactly one inline policy, and it is the scoped module's output.
	//
	// The COUNT is checked before the contents, and the order matters: with two
	// policies on the role, "the second one does not read module.github_policy.json"
	// is a true statement that describes the wrong problem. The problem is that
	// there are two.
	inline := map[string]string{}
	for _, file := range sortedKeys(files) {
		for name, body := range topLevelBlocks(files[file], "aws_iam_role_policy") {
			if strings.Contains(body, githubRoleRef) {
				inline[file+":aws_iam_role_policy."+name] = body
			}
		}
	}

	require.Lenf(t, inline, 1,
		"expected exactly one aws_iam_role_policy on %s across modules/workloads, found %d: %v. "+
			"Zero means the role lost its permissions and every deploy fails. More than one means "+
			"the role's effective ECR reach is the UNION of several documents, and only the one "+
			"modules/github_policy renders is checked — the other is free to grant ecr:PutImage on "+
			"a wildcard, which is the original cross-project defect with the fix still visibly in "+
			"place",
		githubRoleRef, len(inline), sortedKeys(inline))

	for addr, body := range inline {
		// The reference must be BARE. A rendered string, a jsonencode() or a
		// second document inlined here puts the policy somewhere
		// modules/github_policy/tests/ does not look, and that suite is the only
		// thing checking the grant stops at this project.
		require.Regexpf(t, `(?m)^\s*policy\s*=\s*`+regexp.QuoteMeta(githubPolicyRef)+`\s*$`, body,
			"%s does not read %s. The scoped document can be entirely correct and entirely "+
				"unattached: point the role at another aws_iam_policy_document — the reflex fix when "+
				"a deploy fails with AccessDenied — and the role gets whatever that one says while "+
				"modules/github_policy/tests/ keeps passing on a document nothing consumes. Body "+
				"was:\n%s",
			addr, githubPolicyRef, body)
	}

	// Half two: no managed or customer-managed policy attached alongside it.
	// aws_iam_policy_attachment is the deprecated account-wide form and is
	// checked with the modern one because it reaches the same role by the same
	// union.
	for _, typ := range []string{"aws_iam_role_policy_attachment", "aws_iam_policy_attachment"} {
		for _, file := range sortedKeys(files) {
			blocks := topLevelBlocks(files[file], typ)
			for _, name := range sortedNames(blocks) {
				require.NotContainsf(t, blocks[name], githubRoleRef,
					"%s:%s.%s attaches a policy to %s. IAM takes the UNION of everything attached to a "+
						"role, so an attachment sits ON TOP of the scoped document and cannot narrow it — "+
						"AmazonEC2ContainerRegistryPowerUser here hands the deploy role write access to "+
						"every ECR repository in the account, including every other meroku project's, and "+
						"modules/github_policy/tests/ cannot see it: that module renders one document and "+
						"knows nothing about what else the role carries. If the pipeline needs a permission "+
						"it does not have, add it to modules/github_policy, scoped. Body was:\n%s",
					file, typ, name, githubRoleRef, blocks[name])
			}
		}
	}
}

// TestGithubRoleGrantsNothingOnItsOwnBlock closes the other two doors into the
// same union, both of which live INSIDE the aws_iam_role resource and so are
// invisible to a guard that only inspects sibling resources.
//
// The test above reads separate top-level resources — aws_iam_role_policy and
// the two attachment types. Under the provider modules/workloads/versions.tf
// pins (hashicorp/aws >= 5.34.0, < 6.0.0) the role block itself takes two more
// arguments that reach the identical union, verified against the resolved
// provider schema rather than from memory:
//
//   - `managed_policy_arns = [...]`, an attribute on aws_iam_role. Deprecated in
//     5.x — it warns, it does not fail — and it is precisely the reflex fix when
//     a deploy fails with AccessDenied, because it is one line inside a block
//     already open in the editor and needs no new resource anywhere.
//   - `inline_policy { name policy }`, a set-nested block on aws_iam_role,
//     likewise deprecated-not-removed in 5.x. Attribute syntax
//     (`inline_policy = [...]`) is rejected by the provider, so block form and
//     `dynamic "inline_policy"` are the whole surface.
//
// Either one restores account-wide ECR write with modules/github_policy's
// document still attached, still correct, and every other assertion in this file
// and in modules/github_policy/tests/ still green — the role's effective
// permissions are the UNION, and neither construct is a resource anything else
// here counts.
func TestGithubRoleGrantsNothingOnItsOwnBlock(t *testing.T) {
	files := workloadsTFFiles(t)

	// The whole directory, for the reason workloadsTFFiles gives: the role may be
	// declared in any file of the module. Finding it is itself the first
	// assertion — a guard that silently matches nothing is worse than no guard.
	roleName := strings.TrimPrefix(githubRoleRef, "aws_iam_role.")
	bodies := map[string]string{}
	for _, file := range sortedKeys(files) {
		for name, body := range topLevelBlocks(files[file], "aws_iam_role") {
			if name == roleName {
				bodies[file+":"+githubRoleRef] = body
			}
		}
	}

	require.Lenf(t, bodies, 1,
		"expected exactly one %s declaration across modules/workloads, found %d: %v. Zero means "+
			"the role was renamed or moved and every assertion in this file now searches for a "+
			"resource that does not exist — the guards pass by matching nothing while the deploy "+
			"role is free to carry whatever the new one does",
		githubRoleRef, len(bodies), sortedKeys(bodies))

	for addr, body := range bodies {
		require.NotRegexpf(t, `(?m)^\s*managed_policy_arns\s*=`, body,
			"%s sets managed_policy_arns on the role itself. A role's effective permissions are the "+
				"UNION of the scoped inline document and every managed policy in this list, so the list "+
				"can only widen the grant and never narrow it: one entry of "+
				"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryPowerUser hands the deploy role write "+
				"access to every ECR repository in the account, including every other meroku project's, "+
				"while modules/github_policy/tests/ keeps proving the inline document stops at this "+
				"project — that module renders one document and cannot see what else the role carries. "+
				"This is the cheapest possible AccessDenied fix (one line, inside a block already open) "+
				"and the provider only deprecation-warns on it under the pinned 5.x. If the pipeline "+
				"needs a permission it does not have, add it to modules/github_policy, scoped. Body "+
				"was:\n%s",
			addr, body)

		require.NotRegexpf(t, `(?m)^\s*(?:dynamic\s+"inline_policy"|inline_policy)\s*\{`, body,
			"%s declares an inline_policy block on the role itself. It is a SET of documents: the "+
				"block does not replace aws_iam_role_policy.github_access, it is added to it, and the "+
				"role ends up with the union of both — an ecr:* on a wildcard Resource here reaches "+
				"every repository in the account with the scoped document sitting underneath it "+
				"unchanged and every other assertion in this file still passing. It is also invisible "+
				"to modules/github_policy/tests/, which renders one document and knows nothing about "+
				"the role. Deprecated but fully functional under the pinned 5.x provider. Permissions "+
				"belong in modules/github_policy, where they are scoped and tested. Body was:\n%s",
			addr, body)
	}
}

// TestGithubPolicyModuleIsWiredToThisAccountAndRegion pins the four inputs that
// decide whose repositories the rendered policy is about.
//
// modules/github_policy takes `project`, `aws_account_id` and `region` as plain
// strings — deliberately, because a data source there would make the module
// unplannable without credentials and destroy the one property the extraction
// bought. The consequence is that its test suite asserts a correct scope around
// whatever values IT passes in, and cannot notice that the caller passes
// something else.
//
// Two edits it catches, both one word wide:
//
//   - `region = var.ecr_account_region` — which defaults to "" — renders
//     `arn:aws:ecr::000000000000:repository/acme_backend`. IAM accepts it, the
//     plan diff shows a grant, and it matches nothing in any account, so every
//     push in every project is denied with no hint as to why.
//   - `region = "us-east-1"` — correct in one region and silently denying in
//     every other, which no test that plans a single region can see.
//
// `aws_account_id = var.ecr_account_id` is the same failure aimed at the account
// field: it names repositories that do not exist here, so the grant covers
// nothing while looking complete.
func TestGithubPolicyModuleIsWiredToThisAccountAndRegion(t *testing.T) {
	body := moduleBlock(t, githubTF(t), "github_policy")

	for _, wiring := range []struct {
		input string
		want  string
		why   string
	}{
		{
			input: "project",
			want:  "var.project",
			why: "the project name is the ONLY thing separating this project's ECR namespace from a " +
				"neighbouring project's in a shared account. A literal, or another module's name, " +
				"renders a policy that is perfectly scoped to the wrong project — and every assertion " +
				"in modules/github_policy/tests/ still passes, because that suite supplies its own",
		},
		{
			input: "env",
			want:  "var.env",
			why: "the iam:PassRole patterns end in the environment. A wrong or empty value widens them " +
				"across every environment in the account, which lets a dev pipeline hand production's " +
				"task execution role to ECS",
		},
		{
			input: "aws_account_id",
			want:  "local.aws_account_id",
			why: "the grant has to name THIS account. var.ecr_account_id names the cross-account SOURCE " +
				"registry, whose repositories do not exist here, so the policy would look complete in " +
				"the plan and match nothing at the call — and it defaults to \"\", which produces an " +
				"ARN with an empty account field: valid syntax, real-looking diff, zero reach",
		},
		{
			input: "region",
			want:  "data.aws_region.current.name",
			why: "the region field of an ECR ARN is part of the scope, and this one must be the region " +
				"the role is deployed into. var.ecr_account_region defaults to \"\" and yields " +
				"arn:aws:ecr::<account>:repository/... — accepted by IAM, shown as a grant in the plan, " +
				"matching nothing anywhere. A hardcoded region is the same denial in every region but " +
				"one, and neither failure appears until a pipeline runs",
		},
	} {
		require.Regexpf(t, `(?m)^\s*`+regexp.QuoteMeta(wiring.input)+`\s*=\s*`+regexp.QuoteMeta(wiring.want)+`\s*$`, body,
			"module \"github_policy\" must pass %s = %s: %s. Module block was:\n%s",
			wiring.input, wiring.want, wiring.why, body)
	}
}

// githubTF returns modules/workloads/github.tf, stripped of comments.
func githubTF(t *testing.T) string {
	t.Helper()
	return workloadsTF(t, "github.tf")
}

// moduleBlock returns the body of a top-level `module "name" { ... }` block.
//
// Anchored to column zero and to the block's own closing brace for the reason
// topLevelBlocks is: a pattern loose enough to span one block is loose enough to
// span two, and "these inputs are wired correctly" is worthless if the inputs
// being read belong to a different module.
func moduleBlock(t *testing.T, src, name string) string {
	t.Helper()

	m := regexp.MustCompile(`(?ms)^module "` + regexp.QuoteMeta(name) + `" \{$(.*?)^\}$`).
		FindStringSubmatch(src)
	require.Lenf(t, m, 2,
		"module %q not found in github.tf. The deploy role's permissions come from it and from "+
			"nothing else; if it moved, this guard has stopped guarding anything", name)
	return m[1]
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
