package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// This test encodes the rule stated in CLAUDE.md: "Project prefix is always first."
//
// It exists because the rule was broken silently for over a year. Commit 022c0d4
// ("make two envs in one account") added _${var.env} to modules/workloads/lambda.tf
// while adding the full _${var.project}_${var.env} to backend.tf in the same
// commit. Nothing caught the difference, and the result was that deploying a
// second meroku project into one AWS account failed on six resources — two of
// which (EventBridge rule and target) do not error at all, because PutRule and
// PutTargets are upserts, and instead silently repoint the first project's
// deploy trigger.
//
// Any name that AWS scopes to the account (or account+region) must therefore
// carry ${var.project}.

// resourceTypesRequiringProjectScope are the AWS resource types whose names are
// unique across an account, or across an account and region.
var resourceTypesRequiringProjectScope = map[string]string{
	"aws_iam_role":                      "IAM role names are account-global",
	"aws_iam_policy":                    "IAM policy names are account-global",
	"aws_iam_instance_profile":          "instance profile names are account-global",
	"aws_lambda_function":               "Lambda function names are account+region-global",
	"aws_cloudwatch_event_rule":         "EventBridge rule names are account+region-global (and PutRule is an upsert, so collisions hijack silently)",
	"aws_cloudwatch_event_bus":          "event bus names are account+region-global",
	"aws_cloudwatch_log_group":          "log group names are account+region-global",
	"aws_ecs_cluster":                   "cluster names are account+region-global",
	"aws_sqs_queue":                     "queue names are account+region-global",
	"aws_sns_topic":                     "topic names are account+region-global",
	"aws_scheduler_schedule_group":      "schedule group names are account+region-global",
	"aws_service_discovery_namespace":   "namespace names are account+region-global",
	"aws_secretsmanager_secret":         "secret names are account+region-global",
	"aws_elasticache_replication_group": "replication group ids are account+region-global",
}

// allowedUnscoped lists deliberate exceptions, each with the reason it is safe.
// Keep this list short and justified — it is the escape hatch that lets the rule
// stay strict everywhere else.
var allowedUnscoped = map[string]string{
	// ECR repositories are intentionally shared across the environments of one
	// project, so they carry ${var.project} but no ${var.env}. They are checked
	// for the project prefix like everything else; this note is for the env part.
	"modules/workloads/ecr.tf:aws_ecr_repository.backend":  "shared across envs by design",
	"modules/workloads/ecr.tf:aws_ecr_repository.services": "shared across envs by design",

	// Transitively scoped: the name interpolates the API Gateway's own name,
	// which modules/workloads/api_gateway.tf sets to "${var.project}-${var.env}".
	// The regex cannot follow a resource reference, so record it here.
	"modules/workloads/domain.tf:aws_cloudwatch_log_group.api_gateway_logs": "derives from aws_apigatewayv2_api.api_gateway.name, which is project-scoped",

	// The delegation role is per root domain, not per project: several projects
	// and accounts assume the same role to manage one parent zone. Adding a
	// project prefix here would be actively wrong. (modules/dns-root has no
	// var.project at all.)
	"modules/dns-root/main.tf:aws_iam_role.dns_delegation": "one role per root domain, shared across projects by design",
}

// nameAttr matches a top-level name/family assignment inside a resource block.
var nameAttr = regexp.MustCompile(`(?m)^\s{2}(name|family|function_name)\s*=\s*(.+)$`)

// resourceHeader matches `resource "type" "label" {`.
var resourceHeader = regexp.MustCompile(`(?m)^resource\s+"([a-z0-9_]+)"\s+"([A-Za-z0-9_-]+)"\s*\{`)

func modulesDir(t *testing.T) string {
	t.Helper()
	// Tests run from app/; modules/ is a sibling.
	dir := filepath.Join("..", "modules")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("modules directory not found at %s: %v", dir, err)
	}
	return dir
}

func TestModuleNames_AreProjectScoped(t *testing.T) {
	root := modulesDir(t)

	var violations []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".tf") {
			return err
		}

		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		src := string(body)
		rel := filepath.ToSlash(strings.TrimPrefix(path, ".."+string(filepath.Separator)))

		headers := resourceHeader.FindAllStringSubmatchIndex(src, -1)
		for i, h := range headers {
			resType := src[h[2]:h[3]]
			resName := src[h[4]:h[5]]

			reason, guarded := resourceTypesRequiringProjectScope[resType]
			if !guarded {
				continue
			}

			// Body of this resource block runs to the start of the next resource.
			end := len(src)
			if i+1 < len(headers) {
				end = headers[i+1][0]
			}
			block := src[h[1]:end]

			for _, m := range nameAttr.FindAllStringSubmatch(block, -1) {
				attr, value := m[1], strings.TrimSpace(m[2])

				// Only literal/interpolated strings are checked; references to
				// other resources or locals are resolved elsewhere.
				if !strings.HasPrefix(value, `"`) {
					continue
				}
				if strings.Contains(value, "var.project") {
					continue
				}
				if _, ok := allowedUnscoped[rel+":"+resType+"."+resName]; ok {
					continue
				}

				violations = append(violations, strings.Join([]string{
					rel + ": " + resType + "." + resName,
					"    " + attr + " = " + value,
					"    " + reason,
					"    fix: include ${var.project} in the name",
				}, "\n"))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking modules: %v", err)
	}

	if len(violations) > 0 {
		t.Errorf("%d account-global name(s) missing ${var.project}:\n\n%s\n\n"+
			"Deploying a second meroku project into the same AWS account will collide on these.",
			len(violations), strings.Join(violations, "\n\n"))
	}
}

// The Lambda's SCHEDULED_TASK_MAP looks up task definitions by family name. If
// modules/ecs_task builds a different family than lambda.tf expects, ECR-push
// auto-deploy for scheduled tasks silently does nothing — which was the case
// until the family was project-scoped.
func TestScheduledTaskFamily_MatchesLambdaLookup(t *testing.T) {
	root := modulesDir(t)

	lambdaSrc, err := os.ReadFile(filepath.Join(root, "workloads", "lambda.tf"))
	if err != nil {
		t.Fatalf("reading lambda.tf: %v", err)
	}
	taskSrc, err := os.ReadFile(filepath.Join(root, "ecs_task", "main.tf"))
	if err != nil {
		t.Fatalf("reading ecs_task/main.tf: %v", err)
	}

	// What the Lambda expects, from local.scheduled_task_map.
	const expected = `"${var.project}_task_${name}_${var.env}"`
	if !strings.Contains(string(lambdaSrc), expected) {
		t.Fatalf("lambda.tf no longer builds SCHEDULED_TASK_MAP with %s — update this test if the convention changed", expected)
	}

	// What ecs_task actually creates. Same shape, with var.task in place of name.
	const actual = `"${var.project}_task_${var.task}_${var.env}"`
	if !strings.Contains(string(taskSrc), actual) {
		t.Errorf("modules/ecs_task/main.tf must set family = %s so it matches the "+
			"SCHEDULED_TASK_MAP lookup in lambda.tf (%s); otherwise ECR-push "+
			"auto-deploy for scheduled tasks looks up families that do not exist",
			actual, expected)
	}
}
