package boundary_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var commentLine = regexp.MustCompile(`(?m)^\s*(#|//).*$`)

// lambdaTF returns modules/workloads/lambda.tf with comment lines blanked out,
// three levels up from modules/workloads/ci_lambda/internal/boundary.
//
// Comments are stripped so the guards below can talk about what the file
// *does* without tripping over prose that mentions the very thing being
// guarded against.
func lambdaTF(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "lambda.tf"))
	require.NoError(t, err)
	return commentLine.ReplaceAllString(string(b), "")
}

func requireAbsent(t *testing.T, src, needle, why string) {
	t.Helper()
	require.Falsef(t, strings.Contains(src, needle), "lambda.tf still contains %q: %s", needle, why)
}

func requirePresent(t *testing.T, src, needle, why string) {
	t.Helper()
	require.Truef(t, strings.Contains(src, needle), "lambda.tf is missing %q: %s", needle, why)
}

// TestLambdaTFDerivesIdentifiersFromTheModule fails if the identifier maps stop
// coming from the shared module. Hardcoding a key here is exactly how the
// backend deploy path came to be broken: Terraform wrote "backend", Go expected
// "", and nothing compared them.
func TestLambdaTFDerivesIdentifiersFromTheModule(t *testing.T) {
	src := lambdaTF(t)

	requirePresent(t, src, `module "ci_identifiers"`, "the identifiers module must be called")
	for _, ref := range []string{
		"module.ci_identifiers.backend_id",
		"module.ci_identifiers.service_ids",
		"module.ci_identifiers.task_ids",
		"module.ci_identifiers.ecr_repo_ids",
		"module.ci_identifiers.ssm_prefix_ids",
	} {
		requirePresent(t, src, ref, "every identifier map must be built from the module")
	}
}

// TestLambdaTFContainsNoIdentifierLiterals fails if an identifier is written
// out by hand in a map-key position instead of being taken from the module.
func TestLambdaTFContainsNoIdentifierLiterals(t *testing.T) {
	src := lambdaTF(t)

	forbidden := []struct {
		what string
		re   *regexp.Regexp
	}{
		{`"backend" used as a map key`, regexp.MustCompile(`"backend"\s*=`)},
		{`the "task:" identifier prefix`, regexp.MustCompile(`"task:`)},
	}

	for _, f := range forbidden {
		require.Falsef(t, f.re.MatchString(src),
			"lambda.tf contains %s; identifiers must come from module.ci_identifiers", f.what)
	}
}

// TestLambdaTFDoesNotArchiveAtPlanTime fails if data.archive_file comes back.
//
// A data source is read during the plan walk, so on any checkout without a
// prebuilt artifact — every fresh clone, every CI runner, because the binary is
// gitignored — plan, apply and destroy all fail before they start. The build
// provisioner produces the zip and the function reads it by filename, which the
// provider only touches during Create/Update.
func TestLambdaTFDoesNotArchiveAtPlanTime(t *testing.T) {
	src := lambdaTF(t)

	requireAbsent(t, src, `data "archive_file"`, "a data source is read during the plan walk")
	requireAbsent(t, src, "archive_file.lambda", "a data source is read during the plan walk")
	requirePresent(t, src, "null_resource.build_ci_lambda", "the artifact must be built by the provisioner")
}

// TestLambdaTFNeverReadsTheArtifactAtPlanTime is the other half of the same
// property, and the one that lets `terraform destroy` run on a machine that
// never built anything.
//
// archive_file is not the only way to read a file during the plan walk: every
// file*() function except fileexists() opens the path and errors when it is
// missing, and they are evaluated wherever they appear — including in a destroy
// plan. meroku used to paper over exactly this by writing a placeholder
// "bootstrap" before every destroy and deleting it afterwards; that writer is
// gone, so the invariant it protected has to be asserted here instead.
//
// Reading the hash off null_resource.build_ci_lambda.triggers.src keeps the
// ordering edge (build before create/update) without touching the filesystem.
func TestLambdaTFNeverReadsTheArtifactAtPlanTime(t *testing.T) {
	src := lambdaTF(t)

	// filesha1() over the *sources* is fine and is how the build trigger is
	// computed: those files are committed and always present. What must never
	// appear is a read of anything under the build directory.
	readsBuildDir := regexp.MustCompile(`file(base64sha256|md5|sha1|sha256|sha512|base64)?\([^)]*ci_lambda_(zip|build_dir)`)
	require.NotRegexp(t, readsBuildDir, src,
		"lambda.tf must not read the build artifact with a file*() function: it is evaluated during "+
			"the plan walk, so a checkout without .build/ could not even plan a destroy")

	require.Regexp(t, `source_code_hash\s*=\s*null_resource\.build_ci_lambda\.triggers\.src`, src,
		"the function's source hash must come from the build resource, not from a file read")

	// fileexists() is the one exception and is deliberately used for the staging
	// probe: it returns false rather than failing on a missing path.
	requirePresent(t, src, "fileexists(local.ci_lambda_zip)",
		"the build must re-run when the artifact is absent even though the sources are unchanged")
}

// TestLambdaTFBuildTriggersCoverTheArtifactInputs fails if the build trigger
// stops covering something that changes the binary. `architectures` is an
// in-place update on the function: flipping it without rebuilding would deploy
// a binary for the wrong architecture behind a correctly declared function.
func TestLambdaTFBuildTriggersCoverTheArtifactInputs(t *testing.T) {
	src := lambdaTF(t)

	block := regexp.MustCompile(`(?s)resource "null_resource" "build_ci_lambda" \{(.*?)\n\}`).FindStringSubmatch(src)
	require.Len(t, block, 2, "null_resource.build_ci_lambda not found in lambda.tf")

	for _, trigger := range []string{"src", "goos", "goarch", "build_cmd"} {
		require.Containsf(t, block[1], trigger+" ", "build trigger %q is missing", trigger)
	}
}

// TestLambdaTFManualRulesAreScopedTwoWays guards the manual deploy path.
//
// There are two rules on purpose and each needs its own property:
//
//   - the scoped rule must NOT filter on detail. EventBridge requires every key
//     named in a pattern to be present, and payloads already in the wild send
//     only {"service": "..."}; a filter would kill them. It is safe unfiltered
//     only because its source list names this environment.
//   - the global rule must filter on detail.project and detail.env. Its sources
//     name no environment, so there is nothing else that can scope them.
//
// Losing either property reintroduces "one production deploy redeploys dev,
// staging and every other project in the account".
func TestLambdaTFManualRulesAreScopedTwoWays(t *testing.T) {
	src := lambdaTF(t)

	scoped := ruleBlock(t, src, "ci_manual_deploy")
	require.NotContains(t, scoped, "detail =",
		"the environment-scoped manual rule must not filter on detail: legacy payloads carry no project")
	require.Contains(t, scoped, "SERVICE_DEPLOY",
		"the per-service workflow generator emits detail-type SERVICE_DEPLOY")
	require.Contains(t, scoped, "ci_manual_sources_scoped",
		"the rule must take its sources from the environment-scoped list")

	global := ruleBlock(t, src, "ci_manual_deploy_global")
	require.Contains(t, global, "ci_manual_sources_global")
	require.Contains(t, global, "detail =",
		"an environment-agnostic source can only be scoped by the detail")
	require.Regexp(t, `project\s*=\s*\[var\.project\]`, global)
	require.Regexp(t, `env\s*=\s*\[var\.env\]`, global)
}

// TestLambdaTFManualSourcesAreEnvironmentScoped is the H4 regression test on
// the Terraform side.
//
// Every environment's rule used to list "action.production" unconditionally, so
// a production deploy event matched the dev rule and the staging rule too, and
// each of those Lambdas redeployed its own backend. An environment-scoped source
// list is what makes that structurally impossible; only a production-named
// environment may accept the legacy fixed "action.production" source.
func TestLambdaTFManualSourcesAreEnvironmentScoped(t *testing.T) {
	src := lambdaTF(t)

	block := regexp.MustCompile(`(?s)ci_manual_sources_scoped\s*=\s*distinct\(concat\((.*?)\n  \)\)`).
		FindStringSubmatch(src)
	require.Len(t, block, 2, "local.ci_manual_sources_scoped not found in lambda.tf")

	require.Contains(t, block[1], `"action.${var.env}"`)
	require.Contains(t, block[1], `"github.actions.${var.env}"`)
	require.Regexp(t, `contains\(local\.ci_production_envs, var\.env\)\s*\?\s*\["action\.production"\]\s*:\s*\[\]`,
		block[1],
		"\"action.production\" must be conditional on the environment being a production one; "+
			"listing it unconditionally is what made a production deploy redeploy dev and staging")

	require.NotRegexp(t, `ci_manual_sources_scoped\s*=\s*distinct\(concat\(\s*\[\s*"action\.deploy"`, src,
		"an environment-agnostic source must not appear in the unfiltered rule")
}

// TestLambdaTFEcrFallbackKeepsOffPrefixRepositories is the M5 regression test.
//
// Past 2,048 characters the explicit repository list is swapped for a
// project-prefix filter. That filter cannot see a repository reached through
// ecr_config mode = manual_repo, because such a repository is an arbitrary URI
// that strips to something like "team/legacy-api" and carries no project name.
// The fallback therefore used to narrow the rule as a side effect of project
// size — roughly 92 repositories at 18-character names, 69 at 25, 51 at 35 —
// and every manual_repo service stopped receiving ECR events while everything
// project-prefixed kept working. Nothing failed and nothing logged.
func TestLambdaTFEcrFallbackKeepsOffPrefixRepositories(t *testing.T) {
	src := lambdaTF(t)

	requirePresent(t, src, "ci_ecr_offprefix_repos",
		"the fallback must know which repositories the project prefix cannot cover")
	require.Regexp(t, `ci_ecr_offprefix_repos\s*=\s*\[for r in local\.ci_ecr_repos : r if !startswith\(r, "\$\{var\.project\}_"\)\]`,
		src, "the off-prefix set must be derived from the repository list itself, not restated")

	fallback := jsonencodeLocal(t, src, "ci_ecr_pattern_prefix")
	require.Contains(t, fallback, "local.ci_ecr_offprefix_repos",
		"the prefix fallback must list the off-prefix repositories explicitly; a bare "+
			"prefix filter silently drops every manual_repo service once a project outgrows "+
			"the 2,048-character event-pattern limit")
	require.Regexp(t, `repository-name\s*=\s*concat\(\[\{ prefix = "\$\{var\.project\}_" \}\], local\.ci_ecr_offprefix_repos\)`,
		fallback, "the fallback must be prefix + off-prefix, in that shape")
}

// TestLambdaTFEcrPatternOverflowFailsLoudly covers the case the fallback cannot
// fix: enough off-prefix repositories to blow the quota on their own. There is
// no third fallback that keeps the rule complete, and a narrowed rule is exactly
// what this file exists to prevent, so the apply has to stop.
func TestLambdaTFEcrPatternOverflowFailsLoudly(t *testing.T) {
	src := lambdaTF(t)

	rule := ruleBlock(t, src, "ci_ecr_push")
	require.Contains(t, rule, "precondition",
		"an event pattern that cannot be made to fit must fail the apply, not degrade")
	require.Regexp(t, `condition\s*=\s*length\(local\.ci_ecr_pattern\)\s*<=\s*2048`, rule,
		"the precondition must measure the pattern that is actually shipped")
	require.Contains(t, rule, "error_message",
		"the failure must say what to do about it")
}

// TestLambdaTFAutoDeployIsAFlagNotAFilter guards the shape of the setting.
//
// Excluding a disabled target from the maps would make the Lambda answer "no
// target uses repository X" — untrue, and indistinguishable from a typo. The
// repository allow-list and the target maps must therefore stay complete, with
// the policy travelling separately.
func TestLambdaTFAutoDeployIsAFlagNotAFilter(t *testing.T) {
	src := lambdaTF(t)

	requirePresent(t, src, "AUTO_DEPLOY_MAP", "the policy must reach the Lambda as data")
	requirePresent(t, src, "auto_deploy_map = jsonencode(module.ci_identifiers.auto_deploy)",
		"the policy map must come from the identifiers module, keyed by the same identifiers")

	require.Regexp(t, `ci_ecr_repos\s*=\s*keys\(module\.ci_identifiers\.ecr_repo_ids\)`, src,
		"every repository must stay in the ECR event rule, including a disabled target's: "+
			"the invocation is what produces the log line that explains the silence")

	// The target maps are built by iterating the full sets, with no policy
	// predicate anywhere in the comprehension.
	for _, name := range []string{"ecs_service_map", "scheduled_task_map", "s3_to_service_map"} {
		body, ok := balancedParens(src, name+" = jsonencode(")
		require.Truef(t, ok, "local.%s not found in lambda.tf", name)
		require.NotContainsf(t, body, "auto_deploy",
			"local.%s must not filter or annotate on auto_deploy; the policy is its own map", name)
	}
}

// balancedParens returns the text between the parenthesis that opens at the end
// of `after` and its match. jsonencode(...) takes a call, not always an object
// literal, so brace balancing cannot find the end of one.
func balancedParens(src, after string) (string, bool) {
	i := strings.Index(src, after)
	if i < 0 {
		return "", false
	}
	start := i + len(after)
	depth := 1
	for j := start; j < len(src); j++ {
		switch src[j] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return src[start:j], true
			}
		}
	}
	return "", false
}

// ruleBlock returns the body of an aws_cloudwatch_event_rule resource.
func ruleBlock(t *testing.T, src, name string) string {
	t.Helper()
	block := regexp.MustCompile(`(?s)resource "aws_cloudwatch_event_rule" "` + name + `" \{(.*?)\n\}\n`).
		FindStringSubmatch(src)
	require.Lenf(t, block, 2, "aws_cloudwatch_event_rule.%s not found in lambda.tf", name)
	return block[1]
}

// TestLambdaTFDropsDeadConfiguration fails if variables the Lambda never reads,
// or an IAM permission it no longer needs, come back.
func TestLambdaTFDropsDeadConfiguration(t *testing.T) {
	src := lambdaTF(t)

	requireAbsent(t, src, "SERVICE_CONFIG", "it was passed and never read")
	requireAbsent(t, src, "DEPLOYMENT_TIMEOUT_SECONDS", "it was validated and never used")
	requireAbsent(t, src, "service_config", "it was passed and never read")
	requireAbsent(t, src, "ecs:ListTaskDefinitions", "ECS resolves the latest revision from the family name")
}
