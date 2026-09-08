package boundary_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"madappgang.com/infrastructure/ci_lambda/internal/handler"
)

var commentLine = regexp.MustCompile(`(?m)^\s*(#|//).*$`)

// workloadsTF returns a file from modules/workloads — three levels up from
// modules/workloads/ci_lambda/internal/boundary — with comment lines blanked
// out.
//
// Comments are stripped so the guards below can talk about what a file *does*
// without tripping over prose that mentions the very thing being guarded
// against. Several of those comments quote the forbidden construct by name.
func workloadsTF(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", name))
	require.NoError(t, err)
	return commentLine.ReplaceAllString(string(b), "")
}

// lambdaTF returns modules/workloads/lambda.tf, stripped of comments.
func lambdaTF(t *testing.T) string {
	t.Helper()
	return workloadsTF(t, "lambda.tf")
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

// ecrTF returns modules/workloads/ecr.tf, stripped of comments.
func ecrTF(t *testing.T) string {
	t.Helper()
	return workloadsTF(t, "ecr.tf")
}

// TestLambdaTFEcrRepoSetIsKnownAtPlanTime guards the count on the ECR event rule.
//
// aws_ecr_repository.repository_url is Computed, so on the apply that creates a
// repository it is unknown at plan time. That unknown reaches
// aws_cloudwatch_event_rule.ci_ecr_push through:
//
//	service_ecr_urls -> ci_service_repos -> ci_identifiers.ecr_repo_ids
//	  -> ci_ecr_repos -> count = length(...) > 0 ? 1 : 0
//
// and a count Terraform cannot resolve fails the entire plan before anything is
// created — so every first deploy of an environment with services was
// unplannable, with an error naming a line that is not the problem.
//
// The repository *name* is set from configuration and is therefore known on the
// same plan, and a bare name is what an ECR event carries anyway. Nothing on
// this path may go back to reading the URL.
func TestLambdaTFEcrRepoSetIsKnownAtPlanTime(t *testing.T) {
	src := lambdaTF(t)

	requirePresent(t, src, "ci_service_repos = local.service_ecr_repo_names",
		"the ECR event rule's count depends on this map, so it must come from the "+
			"names ecr.tf resolves at plan time, not from anything Computed")

	repos, ok := balancedBraces(src, "ci_ecr_repos")
	if ok {
		require.NotContains(t, repos, "service_ecr_urls",
			"the repository set feeding count must not trace back to a URL")
	}

	// The other half of the contract lives in ecr.tf: the names map must resolve
	// each mode from a config-set attribute.
	ecr := ecrTF(t)
	names, ok := balancedBraces(ecr, "service_ecr_repo_names")
	require.True(t, ok, "local.service_ecr_repo_names not found in ecr.tf")
	require.NotContains(t, names, "repository_url",
		"service_ecr_repo_names exists precisely to avoid the Computed URL; "+
			"reading it here reintroduces the unplannable count")
	require.Contains(t, names, ".name",
		"each mode must resolve to the repository name set in configuration")
}

// balancedBraces returns the text of the first brace-delimited body that follows
// `after`, or false when there is none on that assignment.
func balancedBraces(src, after string) (string, bool) {
	i := strings.Index(src, after)
	if i < 0 {
		return "", false
	}
	open := strings.Index(src[i:], "{")
	if open < 0 {
		return "", false
	}
	open += i

	depth := 0
	for j := open; j < len(src); j++ {
		switch src[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[open : j+1], true
			}
		}
	}
	return "", false
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

// TestSSMRuleExcludesDelete pins an ABSENCE, which is why it exists at all.
//
// handler.PatternContracts states which operation values the rule must SELECT,
// and TestEventPatternsMatchWhatTheHandlerParses enforces that. A contract of
// required values structurally cannot say "and nothing else": adding "Delete"
// to the rule would satisfy every existing assertion.
//
// It must stay out. A deletion cannot be repaired by a deployment — it makes
// one destructive. The revision the service is running still lists the deleted
// parameter in `secrets`, so every task launched after the delete fails on
// ResourceInitializationError; a redeploy therefore turns a running service into
// a stopped one, and the running tasks that would have survived are killed to do
// it. Terraform's next apply removes the entry from the list, and it is the only
// thing that can.
func TestSSMRuleExcludesDelete(t *testing.T) {
	body := inlinePattern(t, lambdaTF(t), "ci_ssm_change")

	require.NotContainsf(t, body, `"`+handler.SSMOperationDelete+`"`,
		"aws_cloudwatch_event_rule.ci_ssm_change selects operation %q. A deployment cannot restore "+
			"a deleted parameter, and the revision the service is running still lists it in "+
			"`secrets`, so the redeploy would replace tasks that were still working with ones that "+
			"cannot start", handler.SSMOperationDelete)
}

// TestECRRuleExcludesTheMutableTag pins the second ABSENCE at this boundary,
// and it exists for the same structural reason TestSSMRuleExcludesDelete does:
// handler.PatternContracts can require that the rule filters on `image-tag`, and
// can require that particular values are SELECTED, but no contract of required
// values can say "and this one must not match".
//
// It has to stay out. Every pipeline this repo generates pushes two tags per
// build — the immutable one, then handler.ECRMutableTag — so one build emits two
// ECR events. While a service deploy was an idempotent UpdateService(family)
// that cost nothing but a duplicate rolling deployment. It stopped being free
// when an ECR push began registering a revision that PINS the pushed image
// (deploy.Deployer.call): two events register two revisions, and which one the
// service ends on is a race — EventBridge does not order deliveries. Lose it and
// the service is pinned to a reference that resolves to a different image
// tomorrow, which is precisely the property the pin exists to remove.
//
// The assertion is deliberately shaped, not a bare NotContains: `anything-but`
// is the only construct that can express this. A positive allow-list cannot,
// because a tag is arbitrary — a SHA, a semver, a branch name — so the excluded
// value necessarily appears in the pattern text, and "does not contain latest"
// would be satisfied by deleting the filter entirely.
func TestECRRuleExcludesTheMutableTag(t *testing.T) {
	src := lambdaTF(t)

	exclusion := regexp.MustCompile(
		`"?image-tag"?\s*=\s*\[\s*\{\s*"?anything-but"?\s*=\s*\[[^]]*"` +
			regexp.QuoteMeta(handler.ECRMutableTag) + `"`)

	// Both patterns, because the 2,048-character fallback is the one that only
	// appears on large projects — exactly where nobody is watching.
	for _, name := range []string{"ci_ecr_pattern_explicit", "ci_ecr_pattern_prefix"} {
		t.Run(name, func(t *testing.T) {
			body := jsonencodeLocal(t, src, name)

			require.Regexpf(t, exclusion, body,
				"local.%s does not exclude image-tag %q with anything-but. An ECR push now "+
					"registers a task-definition revision pinning the pushed image, and every "+
					"generated pipeline pushes %q in addition to its immutable tag — so without "+
					"this filter one build registers two revisions and the service ends up pinned "+
					"to a mutable reference, which is not a rollback point",
				name, handler.ECRMutableTag, handler.ECRMutableTag)
		})
	}
}

// TestEveryAutoDeployableServiceNotifiesOnANewRevision pins the edge that was
// missing entirely, and the defect it pins is an ABSENCE — so only a structural
// test over the Terraform can express it.
//
// Every ECS service here carries ignore_changes = [task_definition]: Terraform
// owns the service's shape, CI owns which revision runs in it. That is right for
// an image push and it left NOBODY owning a configuration change. Adding an SSM
// parameter changes the `secrets` LIST, which lives in the task definition, and
// only Terraform can register a revision that lists it — the Lambda's
// RegisterRevisionWithImage clones the latest ACTIVE revision and copies
// container_definitions wholesale, carrying the old list forward. So Terraform
// registered the revision and deliberately did not deploy it, the Lambda was
// never told it existed, and every component reported success while the
// container kept running the old configuration.
//
// Nor can the SSM Create event close the gap on its own: Parameter Store emits
// it at PutParameter time, BEFORE the apply that registers the revision listing
// the parameter, so the deploy it triggers lands on the revision already
// running. "Terraform registered a revision" is the only event that means the
// configuration change is deployable, and this test requires something to emit
// it.
func TestEveryAutoDeployableServiceNotifiesOnANewRevision(t *testing.T) {
	detailFields := manualDetailTags(t)

	for _, file := range []string{"backend.tf", "services.tf"} {
		t.Run(file, func(t *testing.T) {
			src := workloadsTF(t, file)

			services := topLevelBlocks(src, "aws_ecs_service")
			require.NotEmptyf(t, services, "no aws_ecs_service resource found in %s", file)

			invocations := topLevelBlocks(src, "aws_lambda_invocation")

			checked := 0
			for _, name := range sortedNames(services) {
				body := services[name]
				if !strings.Contains(body, "ignore_changes = [task_definition]") {
					continue
				}
				checked++

				family := taskDefinitionRef(t, file, name, body)
				notifier := invocationFor(t, file, name, family, invocations)

				require.Containsf(t, notifier, "auto_deploy",
					"%s: the invocation for aws_ecs_service.%s is not gated on auto_deploy. "+
						"handler/manual.go deliberately does NOT consult the flag — a DEPLOY event is "+
						"somebody asking for that exact deployment — so the gate has to be here, or an "+
						"apply deploys a service whose operator switched automatic deploys off", file, name)

				requireDeployablePayload(t, file, name, notifier, detailFields)
			}

			require.NotZerof(t, checked,
				"%s has no aws_ecs_service carrying ignore_changes = [task_definition]; either the "+
					"ownership split changed or this test has stopped finding the resources it "+
					"reasons about", file)
		})
	}
}

// TestRevisionNotifiersAreOrderedAfterTheirServices pins the ordering edge that
// aws_lambda_invocation.{backend,services}_revision deliberately do NOT state
// with depends_on.
//
// Those resources tell the CI Lambda to deploy a service, so the service has to
// exist first — and on the very first apply of an environment it is created in
// the same run. The ordering is real today, but it is INDIRECT and nothing
// declares it:
//
//	aws_lambda_invocation.*_revision
//	  -> aws_lambda_function.lambda_deploy        (function_name)
//	    -> local.ecs_service_map                  (the ECS_SERVICE_MAP variable)
//	      -> aws_ecs_service.backend.name, aws_ecs_service.services[key].name
//
// A depends_on at the invocation would restate an edge that already exists and
// would hide the fact that it does, so the comments there point here instead.
// The risk that leaves is a refactor of the LAST link: building ECS_SERVICE_MAP
// from module.ci_identifiers, or from var inputs, or from a name template, is a
// perfectly reasonable-looking change that produces an identical map and
// silently deletes the ordering guarantee. The first apply of a new environment
// would then invoke the Lambda before the service exists, the Lambda would
// answer ServiceNotFoundException — which retry.go classifies as non-retryable,
// so the handler reports "ignored" with a nil error — and the apply would go
// GREEN with the service never deployed. Every unit test would still pass.
//
// That is the shape this whole boundary package exists for, so the reference is
// asserted rather than assumed.
func TestRevisionNotifiersAreOrderedAfterTheirServices(t *testing.T) {
	src := lambdaTF(t)

	// Link 2: the invocation reaches the function by name.
	for _, file := range []string{"backend.tf", "services.tf"} {
		invocations := topLevelBlocks(workloadsTF(t, file), "aws_lambda_invocation")
		require.NotEmptyf(t, invocations, "no aws_lambda_invocation resource found in %s", file)

		for _, name := range sortedNames(invocations) {
			require.Containsf(t, invocations[name], "aws_lambda_function.lambda_deploy",
				"%s: aws_lambda_invocation.%s must name the function through the resource, not "+
					"through a rendered string. The reference is the only thing that orders the "+
					"invocation after the function — and, through the function's ECS_SERVICE_MAP, "+
					"after the ECS service it is about to deploy", file, name)
		}
	}

	// Link 1: the function's ECS_SERVICE_MAP is built from the service resources.
	body, ok := balancedParens(src, "ecs_service_map = jsonencode(")
	require.True(t, ok, "local.ecs_service_map not found in lambda.tf")

	const why = "local.ecs_service_map must read %s. It is not decoration: it is the whole ordering " +
		"guarantee behind aws_lambda_invocation.{backend,services}_revision, which carry no " +
		"depends_on precisely because this reference already makes the Lambda function depend on " +
		"every ECS service. Rebuild this map from module.ci_identifiers, from var inputs or from a " +
		"name template and the map stays byte-identical while the edge disappears — then the first " +
		"apply of a new environment invokes the Lambda before the service exists, the Lambda answers " +
		"ServiceNotFoundException (non-retryable, so reported as \"ignored\" with a nil error), and " +
		"the apply goes green with nothing deployed"

	require.Containsf(t, body, "aws_ecs_service.backend.name", why, "aws_ecs_service.backend.name")
	require.Regexpf(t, `aws_ecs_service\.services\[[^\]]+\]\.name`, body, why,
		"aws_ecs_service.services[...].name")
}

// invocationFor returns the body of the aws_lambda_invocation keyed on the
// revision of the given task definition, failing when there is none.
func invocationFor(t *testing.T, file, service, family string, invocations map[string]string) string {
	t.Helper()

	// Keyed on .revision rather than .arn on purpose: the revision is the number
	// that changes exactly when the rendered content did, so an apply that
	// re-renders identical content invokes nothing.
	keyed := regexp.MustCompile(`aws_ecs_task_definition\.` + regexp.QuoteMeta(family) + `(\[[^\]]*\])?\.revision`)

	for _, name := range sortedNames(invocations) {
		if keyed.MatchString(invocations[name]) {
			return invocations[name]
		}
	}

	require.FailNowf(t, "no revision notifier", ""+
		"%s: aws_ecs_service.%s ignores task_definition, so Terraform registers a revision of "+
		"aws_ecs_task_definition.%s and then deliberately does not deploy it — and no "+
		"aws_lambda_invocation in that file is keyed on "+
		"aws_ecs_task_definition.%s[...].revision. Nothing tells CI the revision exists, so a "+
		"configuration change (a new SSM parameter, which changes the `secrets` LIST and can only "+
		"be rendered by Terraform) reaches no container while every component reports success",
		file, service, family, family)
	return ""
}

// requireDeployablePayload checks that the invocation sends something the
// Lambda will actually act on.
//
// An invocation that fires on every revision and carries a payload the handler
// drops is the same silence this whole test exists to remove, one layer down. So
// the detail-type comes from the handler's own exported constant, and every key
// of the detail object is required to be a field manualDetail decodes —
// encoding/json discards an unknown key without a word.
func requireDeployablePayload(t *testing.T, file, service, notifier string, detailFields []string) {
	t.Helper()

	require.Containsf(t, notifier, `"`+handler.DetailTypeServiceDeploy+`"`,
		"%s: the invocation for aws_ecs_service.%s must send detail-type %q; handler.Handle routes "+
			"a deploy on the detail-type rather than the source, because the set of sources the "+
			"generators emit has changed over time",
		file, service, handler.DetailTypeServiceDeploy)

	requireTerraformSource(t, file, service, notifier)

	detail, ok := balanced(notifier, "detail = {")
	require.Truef(t, ok, "%s: the invocation for aws_ecs_service.%s has no detail = { ... } object",
		file, service)

	keys := detailKeys(detail)
	require.Containsf(t, keys, "service",
		"%s: the invocation for aws_ecs_service.%s sends no `service` key; manual.go answers "+
			"\"manual deploy detail must include a service field\" and deploys nothing", file, service)

	for _, k := range keys {
		require.Containsf(t, detailFields, k,
			"%s: the invocation for aws_ecs_service.%s sends detail key %q, which no field of "+
				"handler.manualDetail decodes; encoding/json drops it silently, so a renamed json "+
				"tag would disable this notification without failing anything", file, service, k)
	}
}

// requireTerraformSource pins the `source` these invocations send, which the
// Lambda reads as a discriminator rather than as documentation.
//
// handler.manual promotes a request whose event source is
// handler.TerraformInvocationSource(env) — "terraform.{env}" — to
// deploy.SourceTerraform, and that is the ONLY source permitted to poll through
// an IAM permission-propagation race instead of reporting AccessDenied as
// permanent. The permission is safe to grant here and nowhere else because no
// rule in lambda.tf accepts a "terraform.*" source (local.ci_manual_sources_scoped
// is action.{env} / github.actions.{env}, local.ci_manual_sources_global is
// action.deploy), so an event carrying it cannot have come from EventBridge:
// it arrived by direct RequestResponse Invoke, from a caller already blocked on
// the answer, with no asynchronous redelivery behind it.
//
// The failure this pins is silent in both directions and in both files.
// Rewriting this argument as "action.${var.env}" — a perfectly reasonable-looking
// tidy-up, since that is what every other emitter sends — keeps the deployment
// working, keeps every unit test passing, and switches the poll off: the first
// apply of a new environment goes back to racing the policy attachment (the
// recorded gap was 6.4s, against a ~3s ordinary retry budget), answering
// AccessDenied, and reporting a GREEN apply with nothing deployed. Adding the
// source to a rule in lambda.tf breaks it the other way, by letting an
// asynchronous event claim a budget that assumes a synchronous caller.
func requireTerraformSource(t *testing.T, file, service, notifier string) {
	t.Helper()

	// One derivation, two readers: the Go constant and the HCL literal. "${var.env}"
	// stands in for the environment, which Terraform interpolates and the Lambda
	// compares against PROJECT_ENV.
	want := handler.TerraformInvocationSource("${var.env}")
	re := regexp.MustCompile(`(?m)^\s*source\s*=\s*"` + regexp.QuoteMeta(want) + `"\s*$`)

	require.Regexpf(t, re, notifier,
		"%s: the invocation for aws_ecs_service.%s must send source = %q. handler.manual reads "+
			"that exact string to promote the request to deploy.SourceTerraform, which is the only "+
			"source allowed to wait out an IAM propagation race — see deploy.awaitingPropagation. "+
			"Change it and the deploy keeps working, every Go test keeps passing, and the first "+
			"apply of a new environment silently goes back to answering AccessDenied and reporting "+
			"a green apply with nothing deployed",
		file, service, want)
}

// detailKeys returns the argument names of an HCL object body, one level deep.
var objectKey = regexp.MustCompile(`(?m)^\s*([a-z][a-z0-9_-]*)\s*=`)

func detailKeys(body string) []string {
	var out []string
	for _, m := range objectKey.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

// taskDefinitionRef returns the resource name of the aws_ecs_task_definition an
// ECS service points at.
func taskDefinitionRef(t *testing.T, file, service, body string) string {
	t.Helper()

	m := regexp.MustCompile(`task_definition\s*=\s*aws_ecs_task_definition\.([a-z0-9_]+)`).
		FindStringSubmatch(body)
	require.Lenf(t, m, 2, "%s: aws_ecs_service.%s does not point at an aws_ecs_task_definition",
		file, service)
	return m[1]
}

// topLevelBlocks returns the body of every top-level `resource "<typ>" "<name>"`
// block, keyed by resource name. A top-level block is the only one whose closing
// brace sits in column zero, which is what terminates the match.
func topLevelBlocks(src, typ string) map[string]string {
	re := regexp.MustCompile(`(?ms)^resource "` + regexp.QuoteMeta(typ) + `" "([a-z0-9_]+)" \{$(.*?)^\}$`)

	out := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		out[m[1]] = m[2]
	}
	return out
}

func sortedNames(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// manualDetailTags returns the JSON field names handler.manualDetail decodes.
//
// Read out of the AST of the real source file, for the reason configGetenvNames
// gives in lambdatf_names_test.go: a list kept alongside the type is one more
// thing that can be updated in one place and not the other, which is the exact
// failure mode under test. The type is unexported, so reflection cannot reach it
// from here.
func manualDetailTags(t *testing.T) []string {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join("..", "handler", "manual.go"), nil, 0)
	require.NoError(t, err)

	var tags []string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok || spec.Name.Name != "manualDetail" {
			return true
		}
		st, ok := spec.Type.(*ast.StructType)
		require.True(t, ok, "handler.manualDetail is not a struct type")

		for _, f := range st.Fields.List {
			require.NotNilf(t, f.Tag, "manualDetail.%s has no json tag, so its wire name is its Go "+
				"name by accident", f.Names[0].Name)
			raw, err := strconv.Unquote(f.Tag.Value)
			require.NoError(t, err)

			tag := reflect.StructTag(raw).Get("json")
			require.NotEmptyf(t, tag, "manualDetail.%s has no json tag", f.Names[0].Name)
			if comma := strings.IndexByte(tag, ','); comma >= 0 {
				tag = tag[:comma]
			}
			tags = append(tags, tag)
		}
		return false
	})

	require.NotEmpty(t, tags, "handler.manualDetail was not found in internal/handler/manual.go")
	sort.Strings(tags)
	return tags
}
