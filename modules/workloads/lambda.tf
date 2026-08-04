# ============================================================================
# CI Lambda — build
# ============================================================================
#
# The binary is built by Terraform at apply time and is never committed. There
# is no `data.archive_file` here on purpose: a data source is read during the
# plan walk, so on any checkout without a prebuilt artifact (every fresh clone,
# every CI runner) `plan`, `apply` and `destroy` all fail before they start.
# The provisioner produces the zip itself and `filename` is only read by the
# provider during Create/Update.

locals {
  ci_lambda_src_dir   = "${path.module}/ci_lambda"
  ci_lambda_build_dir = "${path.module}/ci_lambda/.build/${var.env}"
  ci_lambda_zip       = "${path.module}/ci_lambda/.build/${var.env}/ci_lambda.zip"

  # arm64 is cheaper and cold-starts faster. It is part of the build trigger
  # below: `architectures` is an in-place update on the function, so flipping
  # it without rebuilding would leave an x86_64 binary behind a function
  # declared as arm64 and every invocation would die on exec format.
  ci_lambda_goos   = "linux"
  ci_lambda_goarch = "arm64"

  # Source files that determine the artifact. Build outputs and any local
  # .terraform directory are excluded so the hash cannot chase itself.
  #
  # Tests are excluded too: they are not linked into the binary, so hashing them
  # made every test edit re-upload byte-identical code to Lambda and churn the
  # plan for reviewers. testdata goes with them — the boundary golden file lives
  # there and changes whenever Terraform's outputs change, which would rebuild
  # for a reason that never reaches the artifact.
  ci_lambda_src_files = [
    for f in sort(tolist(fileset(local.ci_lambda_src_dir, "**/*.{go,json,mod,sum,tmpl}"))) :
    f if length(regexall("(^|/)[.](build|terraform)/", f)) == 0
    && !endswith(f, "_test.go")
    && length(regexall("(^|/)testdata/", f)) == 0
  ]

  ci_lambda_src_sha = sha1(join("", [
    for f in local.ci_lambda_src_files : filesha1("${local.ci_lambda_src_dir}/${f}")
  ]))

  # `go run ./tools/mkzip` rather than the `zip` CLI: Go is already a hard
  # requirement here, `zip` is missing from plenty of build images, and mkzip
  # sets the 0755 bit that a bootstrap needs to be executable.
  #
  # This pre-flight is the only place a missing toolchain is reported. meroku
  # used to fail earlier with its own message while building a second, unused
  # amd64 copy of the binary; that build is gone, so this text carries the whole
  # explanation on its own — what is missing, how to install it on either kind of
  # machine, and why nothing is substituted for it.
  ci_lambda_build_command = <<-EOT
    set -e
    if ! command -v go >/dev/null 2>&1; then
      echo "ERROR: building the CI/CD Lambda needs a Go toolchain (1.22+) on the machine running 'terraform apply'." >&2
      echo "       'go' was not found in PATH. Install it with 'brew install go' on macOS, or from" >&2
      echo "       https://go.dev/dl/ , then re-run the deploy -- the build is retried automatically." >&2
      echo "       Nothing is deployed without it: meroku will not substitute a placeholder artifact," >&2
      echo "       because a placeholder deploys green and then fails every invocation with" >&2
      echo "       Runtime.InvalidEntrypoint." >&2
      exit 1
    fi
    mkdir -p ".build/${var.env}"
    GOOS=${local.ci_lambda_goos} GOARCH=${local.ci_lambda_goarch} CGO_ENABLED=0 \
      go build -trimpath -ldflags="-s -w" -o ".build/${var.env}/bootstrap" .
    go run ./tools/mkzip -in ".build/${var.env}/bootstrap" -out ".build/${var.env}/ci_lambda.zip"
  EOT
}

resource "null_resource" "build_ci_lambda" {
  triggers = {
    src       = local.ci_lambda_src_sha
    goos      = local.ci_lambda_goos
    goarch    = local.ci_lambda_goarch
    build_cmd = md5(local.ci_lambda_build_command)

    # Whether the artifact is actually on disk, not just whether the sources
    # changed. Every other trigger is a pure function of committed files, so on a
    # fresh clone they all match the state and the provisioner does not run --
    # while .build/ is gitignored and empty. plan and destroy survive that (they
    # never read `filename`), but anything that makes the provider re-upload the
    # code does: state loss, `state rm`, the function deleted out of band,
    # `-replace=`. Each fails with "unable to load ci_lambda.zip: no such file or
    # directory" at apply time, on a checkout where nothing is wrong.
    #
    # modules/appsync/auth_lambda.tf:56-64 carries the same probe for the same
    # reason. This is the half of that pattern the CI Lambda originally copied
    # without.
    staged = fileexists(local.ci_lambda_zip) ? "present" : "absent"
  }

  provisioner "local-exec" {
    working_dir = local.ci_lambda_src_dir
    command     = local.ci_lambda_build_command
  }
}

# ============================================================================
# CI Lambda — identifiers
# ============================================================================
#
# Every identifier the Lambda can see arrives as a lookup map. The Lambda never
# derives one from a repository name or a parameter path, so an unmapped key is
# simply not ours and is ignored. Repository names and SSM prefixes below come
# from the resources that actually create those objects, so the map and the
# reality cannot disagree.

locals {
  # Backend repository, when this environment has one at all. In
  # ecr_strategy = "cross_account" there is none, and no local ECR event is
  # ever emitted for the backend.
  ci_backend_repo = try(aws_ecr_repository.backend[0].name, "")

  # Which repository does each service actually pull from?
  #
  # ecr.tf already answers this for all three ecr_config modes, and its answer is
  # the one the ECS task definition uses — so it is the only answer that can be
  # right. Deriving it a second time here is what let the two drift: this file
  # used to resolve use_existing as aws_ecr_repository.services[source_service_name],
  # while ecr.tf keys its lookup by "${source_service_type}-${source_service_name}".
  # The two agree only when source_service_type is "services". Anywhere else the
  # try()/lookup() fallbacks both returned "", the entry was dropped, the service
  # silently vanished from ECR_REPO_MAP, and every push to it was logged as
  # unmapped forever. That is defect D1's shape - one name, two derivations - so
  # it gets D1's fix: derive it once, from the authority.
  #
  # service_ecr_urls holds full URIs; an ECR event carries the bare repository
  # name, so strip the registry host and any :tag / @digest suffix:
  #   123456789012.dkr.ecr.us-east-1.amazonaws.com/acme_service_api -> acme_service_api
  #   registry.example.com/team/legacy-api:v1                       -> team/legacy-api
  ci_service_repos = {
    for name, uri in local.service_ecr_urls : name => replace(
      replace(uri, "/^[^/]+\\//", ""),
      "/[:@][^/]*$/", ""
    )
  }

  # modules/ecs_task creates {project}_task_{name} only when env == "dev";
  # every other environment pulls from a cross-account URL.
  ci_task_repos = var.env == "dev" ? {
    for name in var.scheduled_task_names : name => "${var.project}_task_${name}"
  } : {}

  # Scheduled-task SSM parameters live in modules/ecs_task/env.tf, which this
  # module cannot reference. The format is mirrored here; the two must move
  # together.
  ci_task_ssm_prefixes = {
    for name in var.scheduled_task_names : name => "/${var.env}/${var.project}/task/${name}"
  }

  # Per-service auto-deploy policy, straight off the service object. Absent is
  # true (see var.backend_auto_deploy): a project that predates the setting
  # auto-deploys everything, and it must keep doing so.
  ci_service_auto_deploy = { for name, service in local.service_names : name => service.auto_deploy }
}

module "ci_identifiers" {
  source = "./ci_lambda/tf_identifiers"

  project              = var.project
  env                  = var.env
  service_names        = keys(local.service_names)
  scheduled_task_names = var.scheduled_task_names

  backend_repo  = local.ci_backend_repo
  service_repos = local.ci_service_repos
  task_repos    = local.ci_task_repos

  backend_ssm_prefix   = trimsuffix(aws_ssm_parameter.backend_env.name, "/env")
  service_ssm_prefixes = { for name, param in aws_ssm_parameter.services_env : name => trimsuffix(param.name, "/env") }
  task_ssm_prefixes    = local.ci_task_ssm_prefixes

  backend_auto_deploy = var.backend_auto_deploy
  service_auto_deploy = local.ci_service_auto_deploy
  task_auto_deploy    = var.scheduled_task_auto_deploy
}

# ============================================================================
# CI Lambda — IAM
# ============================================================================

data "aws_iam_policy_document" "lambda_deploy_assume_role" {
  statement {
    effect = "Allow"

    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }

    actions = ["sts:AssumeRole"]
  }
}

resource "aws_iam_role" "lambda_deploy_iam" {
  # IAM role names are account-global: a second meroku project deployed into the
  # same AWS account collides unless the name carries ${var.project}.
  name               = "${var.project}_lambda_deploy_iam_${var.env}"
  assume_role_policy = data.aws_iam_policy_document.lambda_deploy_assume_role.json

  tags = {
    Name        = "${var.project}_lambda_deploy_iam_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}


resource "aws_iam_role_policy_attachment" "lambda_basic_esecution" {
  role       = aws_iam_role.lambda_deploy_iam.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

# CloudWatch Log Group for Lambda with retention
# Log group names are account+region-global, and must stay exactly
# "/aws/lambda/<function_name>" or the Lambda writes to an untracked group —
# so this name and aws_lambda_function.lambda_deploy.function_name move together.
resource "aws_cloudwatch_log_group" "lambda_deploy" {
  name              = "/aws/lambda/${var.project}_ci_lambda_${var.env}"
  retention_in_days = 7

  tags = {
    Name        = "/aws/lambda/${var.project}_ci_lambda_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# ============================================================================
# CI Lambda — function
# ============================================================================

# Lambda function names are account+region-global — see the log group above.
resource "aws_lambda_function" "lambda_deploy" {
  filename      = local.ci_lambda_zip
  function_name = "${var.project}_ci_lambda_${var.env}"
  handler       = "bootstrap"
  role          = aws_iam_role.lambda_deploy_iam.arn
  runtime       = "provided.al2"
  architectures = [local.ci_lambda_goarch]

  # Reading the hash off the build resource is also the ordering edge: the
  # artifact is always produced before the function is created or updated.
  source_code_hash = null_resource.build_ci_lambda.triggers.src

  # Disable KMS encryption for environment variables
  kms_key_arn = null

  # Lambda execution timeout (in seconds).
  # 60s is sufficient because ECS deployments are async: the Lambda calls
  # UpdateService (or RegisterTaskDefinition for scheduled tasks) and returns
  # immediately — it does NOT wait for the deployment to stabilize.
  timeout = 60

  # Ensure log group is created first
  depends_on = [aws_cloudwatch_log_group.lambda_deploy]

  environment {
    variables = {
      # Core Configuration
      PROJECT_NAME = var.project
      PROJECT_ENV  = var.env

      # Logging Configuration
      LOG_LEVEL = "info" # Options: debug, info, warn, error

      # ECS Resource Names (ACTUAL resource names from Terraform)
      ECS_CLUSTER_NAME   = aws_ecs_cluster.main.name
      ECS_SERVICE_MAP    = local.ecs_service_map
      S3_SERVICE_MAP     = local.s3_to_service_map
      SCHEDULED_TASK_MAP = local.scheduled_task_map

      # Per-target auto-deploy policy. A separate map, not a field inside the
      # two above — see local.auto_deploy_map.
      AUTO_DEPLOY_MAP = local.auto_deploy_map

      # Event source -> identifier lookups. These replace the repository-name
      # parsing and the SSM path regex that used to guess an identifier.
      ECR_REPO_MAP    = jsonencode(module.ci_identifiers.ecr_repo_ids)
      SSM_SERVICE_MAP = jsonencode(module.ci_identifiers.ssm_prefix_ids)

      # Slack Configuration (if set, notifications are enabled)
      SLACK_WEBHOOK_URL = var.slack_deployment_webhook

      # Deployment Configuration
      MAX_DEPLOYMENT_RETRIES = "2"     # Retry UpdateService/RegisterTaskDefinition on transient AWS errors
      RETRY_BASE_DELAY_MS    = "1000"  # First retry waits ~1s, then ~2s, with jitter
      DRY_RUN                = "false" # Set to true to log deployments without performing them

      # Feature Flags - Enable/disable specific event monitoring
      ENABLE_ECR_MONITORING = "true" # Auto-deploy on ECR image push
      ENABLE_SSM_MONITORING = "true" # Auto-deploy on SSM parameter changes
      ENABLE_S3_MONITORING  = "true" # Auto-deploy on S3 env file changes
      ENABLE_MANUAL_DEPLOY  = "true" # Allow manual deployment triggers
    }
  }

  tags = {
    Name        = "${var.project}_ci_lambda_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}



data "aws_iam_policy_document" "lambda_ecs" {
  # ecs:ListTaskDefinitions is deliberately absent: the Lambda hands ECS a bare
  # family name and lets ECS resolve the latest ACTIVE revision.
  statement {
    sid    = "DescribeAndRegisterTaskDefinitions"
    effect = "Allow"
    actions = [
      "ecs:DescribeTaskDefinition",
      "ecs:RegisterTaskDefinition",
      "ecs:TagResource",
    ]
    # These three actions have no resource-level permission support.
    resources = ["*"]
  }

  statement {
    sid       = "UpdateThisProjectsServices"
    effect    = "Allow"
    actions   = ["ecs:UpdateService"]
    resources = concat([aws_ecs_service.backend.id], values(aws_ecs_service.services)[*].id)
  }

  statement {
    sid     = "PassTaskRolesToECS"
    effect  = "Allow"
    actions = ["iam:PassRole"]
    # Scheduled-task roles are created in modules/ecs_task, so a resource-scoped
    # PassRole would need cross-module plumbing. The service condition is the
    # meaningful restriction: these roles can only be handed to ECS tasks.
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "iam:PassedToService"
      values   = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_policy" "lambda_ecs" {
  # IAM policy names are account-global. ("Dev" in the old name was a misnomer —
  # it was applied to every environment.)
  name   = "LambdaECSPolicy_${var.project}_${var.env}"
  policy = data.aws_iam_policy_document.lambda_ecs.json

  tags = {
    Name        = "LambdaECSPolicy_${var.project}_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_role_policy_attachment" "lambda_ecs" {
  role       = aws_iam_role.lambda_deploy_iam.name
  policy_arn = aws_iam_policy.lambda_ecs.arn
}

# KMS policy for decrypting environment variables
data "aws_iam_policy_document" "lambda_kms" {
  statement {
    effect = "Allow"
    actions = [
      "kms:Decrypt",
      "kms:DescribeKey"
    ]
    resources = ["*"]
  }
}

resource "aws_iam_policy" "lambda_kms" {
  # IAM policy names are account-global.
  name   = "LambdaKMSPolicy_${var.project}_${var.env}"
  policy = data.aws_iam_policy_document.lambda_kms.json

  tags = {
    Name        = "LambdaKMSPolicy_${var.project}_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_role_policy_attachment" "lambda_kms" {
  role       = aws_iam_role.lambda_deploy_iam.name
  policy_arn = aws_iam_policy.lambda_kms.arn
}

# ============================================================================
# CI Lambda — event rules
# ============================================================================
#
# One rule per source. EventBridge requires every field named in a pattern to
# be present in the event, so a single rule carrying the union of five sources
# cannot be filtered at all: adding detail.repository-name to it would silently
# drop every ECS, SSM and manual event.

locals {
  ci_ecr_repos = keys(module.ci_identifiers.ecr_repo_ids)

  # Repositories whose name does NOT start with "${project}_".
  #
  # ecr_config mode = manual_repo points a service at an arbitrary registry URI,
  # which strips down to names like "team/legacy-api" — nothing about them
  # carries the project. mode = use_existing pointed at such a service inherits
  # the same name. The prefix fallback below cannot see them, so they are always
  # listed by hand.
  ci_ecr_offprefix_repos = [for r in local.ci_ecr_repos : r if !startswith(r, "${var.project}_")]

  # An event pattern is capped at 2,048 characters (an EventBridge service
  # quota, raised only by a support ticket). Past roughly 60 repositories the
  # exhaustive list stops fitting, so fall back to a project-prefix filter
  # rather than emitting a rule AWS will reject. ECR_REPO_MAP stays the
  # authority either way: anything the looser filter lets through resolves to
  # no target and is ignored.
  ci_ecr_pattern_explicit = jsonencode({
    source      = ["aws.ecr"]
    detail-type = ["ECR Image Action"]
    detail = {
      action-type     = ["PUSH"]
      result          = ["SUCCESS"]
      repository-name = local.ci_ecr_repos
    }
  })

  # The fallback is a prefix match PLUS the off-prefix repositories in full.
  #
  # It used to be the bare prefix, which quietly narrowed the rule: every
  # manual_repo service stopped receiving ECR events the moment a project grew
  # past the character limit (~92 repositories at 18-character names, ~69 at 25,
  # ~51 at 35), while everything project-prefixed carried on working. A trigger
  # that disappears as a side effect of project size is the worst kind: nothing
  # fails, nothing logs, and the cause is a length nobody is watching.
  ci_ecr_pattern_prefix = jsonencode({
    source      = ["aws.ecr"]
    detail-type = ["ECR Image Action"]
    detail = {
      action-type     = ["PUSH"]
      result          = ["SUCCESS"]
      repository-name = concat([{ prefix = "${var.project}_" }], local.ci_ecr_offprefix_repos)
    }
  })

  ci_ecr_pattern = length(local.ci_ecr_pattern_explicit) <= 2048 ? local.ci_ecr_pattern_explicit : local.ci_ecr_pattern_prefix

  # arn:...:cluster/acme_cluster_dev -> arn:...:service/acme_cluster_dev/
  ci_ecs_service_arn_prefix = "${replace(aws_ecs_cluster.main.arn, ":cluster/", ":service/")}/"

  # Manual deploy sources, split by whether the source *name* identifies an
  # environment.
  #
  # This split is the whole of the cross-environment fix. Listing every variant
  # in every environment's rule meant a production deploy — Source =
  # action.production, no detail — matched the dev rule, the staging rule and
  # every other meroku project's rule in the account, and each of those Lambdas
  # redeployed its own backend. Verified live before the fix.

  # Environment names that the legacy prod deploy receipt hardcodes as
  # "action.production" regardless of what the environment is actually called
  # (receipts/github/prod-deploy.yml ships with ENV: prod). Only these
  # environments may accept that source.
  ci_production_envs = ["prod", "production"]

  # Scoped sources: the source string itself names the environment, so another
  # environment's deploy event can never reach this rule. Because the rule is
  # already environment-safe, it carries NO detail filter — EventBridge
  # requires every key named in a pattern to be present in the event, and the
  # payloads already in the wild send only {"service": "..."}. Filtering here
  # would kill every existing pipeline. Cross-*project* scoping for these is the
  # handler's job (handler/manual.go), which rejects a mismatched project/env
  # whenever the payload carries one.
  ci_manual_sources_scoped = distinct(concat(
    [
      "action.${var.env}",
      "github.actions.${var.env}",
    ],
    contains(local.ci_production_envs, var.env) ? ["action.production"] : [],
  ))

  # Global sources: the source string names no environment, so an event on one
  # is ambiguous by construction and cannot be attributed to an environment
  # without help. These get their own rule WITH a detail filter requiring both
  # project and env, which is the only way such an event can be scoped at all.
  # A payload that omits them matches nothing rather than matching everything.
  #
  # Requiring the fields here breaks no generated pipeline: no meroku generator,
  # receipt, doc or template has ever emitted Source=action.deploy — it only ever
  # appeared in the rule's accept list (`git log -S'action.deploy' -- web/
  # receipts/ docs/ env/ templates/` is empty). Anything hand-written that does
  # emit it was, by construction, deploying every project in the account.
  ci_manual_sources_global = ["action.deploy"]
}

resource "aws_cloudwatch_event_rule" "ci_ecr_push" {
  # No repositories in this account (ecr_strategy = "cross_account" with no
  # locally built services) means no ECR event can ever fire, and an empty
  # repository-name list is not a valid pattern.
  count = length(local.ci_ecr_repos) > 0 ? 1 : 0

  name          = "${var.project}_ci_ecr_${var.env}"
  description   = "CI/CD: image pushed to a repository this project deploys from"
  event_pattern = local.ci_ecr_pattern

  tags = {
    Name        = "${var.project}_ci_ecr_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }

  lifecycle {
    # Enough off-prefix repositories overflow the quota on their own, and there
    # is no third fallback: dropping them is what this rule exists to stop.
    # Roughly 90 of them at 18-character names, 50 at 35. Fail the apply and say
    # what to do rather than silently shipping a rule that ignores some of them.
    precondition {
      condition     = length(local.ci_ecr_pattern) <= 2048
      error_message = <<-EOT
        The ECR event pattern for ${var.project}/${var.env} is ${length(local.ci_ecr_pattern)} characters, over EventBridge's 2,048 limit.

        ${length(local.ci_ecr_offprefix_repos)} of this project's ${length(local.ci_ecr_repos)} repositories are not named "${var.project}_*" (ecr_config
        mode = manual_repo, or use_existing pointed at one), so they cannot be covered by the
        project-prefix fallback and have to be listed in full.

        Do one of:
          - rename those repositories to start with "${var.project}_" so the prefix filter covers them;
          - split the project so fewer services share one CI Lambda;
          - request a PutRule event-pattern quota increase from AWS Support.
      EOT
    }
  }
}

resource "aws_cloudwatch_event_target" "ci_ecr_push" {
  count = length(local.ci_ecr_repos) > 0 ? 1 : 0

  rule      = aws_cloudwatch_event_rule.ci_ecr_push[0].name
  target_id = aws_lambda_function.lambda_deploy.function_name
  arn       = aws_lambda_function.lambda_deploy.arn
}

resource "aws_lambda_permission" "ci_ecr_push" {
  count = length(local.ci_ecr_repos) > 0 ? 1 : 0

  statement_id  = "AllowEventBridgeECR"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.lambda_deploy.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.ci_ecr_push[0].arn
}

# ECS deployment state changes are notification-only. Scoping to this cluster's
# service ARNs is what stops a second project in the same account producing
# Slack noise here.
resource "aws_cloudwatch_event_rule" "ci_ecs_state" {
  name        = "${var.project}_ci_ecs_${var.env}"
  description = "CI/CD: ECS deployment state changes for this cluster"
  event_pattern = jsonencode({
    source      = ["aws.ecs"]
    detail-type = ["ECS Deployment State Change", "ECS Service Action"]
    resources   = [{ prefix = local.ci_ecs_service_arn_prefix }]
  })

  tags = {
    Name        = "${var.project}_ci_ecs_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_cloudwatch_event_target" "ci_ecs_state" {
  rule      = aws_cloudwatch_event_rule.ci_ecs_state.name
  target_id = aws_lambda_function.lambda_deploy.function_name
  arn       = aws_lambda_function.lambda_deploy.arn
}

resource "aws_lambda_permission" "ci_ecs_state" {
  statement_id  = "AllowEventBridgeECS"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.lambda_deploy.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.ci_ecs_state.arn
}

resource "aws_cloudwatch_event_rule" "ci_ssm_change" {
  name        = "${var.project}_ci_ssm_${var.env}"
  description = "CI/CD: SSM parameter updates under this project's path"
  event_pattern = jsonencode({
    source      = ["aws.ssm"]
    detail-type = ["Parameter Store Change"]
    detail = {
      name = [{ prefix = "/${var.env}/${var.project}/" }]
      # Create is Terraform's own parameter creation and Delete removes the
      # configuration a service needs; neither is a reason to deploy.
      operation = ["Update"]
    }
  })

  tags = {
    Name        = "${var.project}_ci_ssm_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_cloudwatch_event_target" "ci_ssm_change" {
  rule      = aws_cloudwatch_event_rule.ci_ssm_change.name
  target_id = aws_lambda_function.lambda_deploy.function_name
  arn       = aws_lambda_function.lambda_deploy.arn
}

resource "aws_lambda_permission" "ci_ssm_change" {
  statement_id  = "AllowEventBridgeSSM"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.lambda_deploy.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.ci_ssm_change.arn
}

# Manual deploys on an environment-scoped source carry NO detail filter,
# deliberately — the "legacy" half of the two paths.
#
# EventBridge requires every key named in a pattern to be present in the event.
# Payloads already in the wild send {"service": "..."} and nothing else, so a
# detail = { project = [...], env = [...] } filter here would match nothing and
# would kill every manual deploy that works today. It is safe to leave the
# filter off precisely because local.ci_manual_sources_scoped lists only sources
# whose name carries this environment: another environment's event cannot arrive
# here in the first place. Cross-project separation for a legacy payload is the
# handler's project check, which can only act on fields the payload carries.
resource "aws_cloudwatch_event_rule" "ci_manual_deploy" {
  name        = "${var.project}_ci_manual_${var.env}"
  description = "CI/CD: manual deployment triggers on an environment-scoped source"
  event_pattern = jsonencode({
    source      = local.ci_manual_sources_scoped
    detail-type = ["DEPLOY", "SERVICE_DEPLOY"]
  })

  tags = {
    Name        = "${var.project}_ci_manual_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_cloudwatch_event_target" "ci_manual_deploy" {
  rule      = aws_cloudwatch_event_rule.ci_manual_deploy.name
  target_id = aws_lambda_function.lambda_deploy.function_name
  arn       = aws_lambda_function.lambda_deploy.arn
}

resource "aws_lambda_permission" "ci_manual_deploy" {
  statement_id  = "AllowEventBridgeManual"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.lambda_deploy.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.ci_manual_deploy.arn
}

# Manual deploys on an environment-agnostic source DO carry a detail filter —
# the "explicit" half of the two paths.
#
# "action.deploy" says nothing about which environment or project it means, so
# there is no source list that can scope it. Requiring detail.project and
# detail.env is the only thing that can: an event that names them reaches
# exactly one environment of one project, and an event that omits them reaches
# none instead of reaching all of them. Both paths exist because the scoped one
# cannot be filtered without breaking existing pipelines, and this one cannot be
# left unfiltered without recreating the defect.
resource "aws_cloudwatch_event_rule" "ci_manual_deploy_global" {
  name        = "${var.project}_ci_manual_global_${var.env}"
  description = "CI/CD: manual deployment triggers on an environment-agnostic source, scoped by detail"
  event_pattern = jsonencode({
    source      = local.ci_manual_sources_global
    detail-type = ["DEPLOY", "SERVICE_DEPLOY"]
    detail = {
      project = [var.project]
      env     = [var.env]
    }
  })

  tags = {
    Name        = "${var.project}_ci_manual_global_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_cloudwatch_event_target" "ci_manual_deploy_global" {
  rule      = aws_cloudwatch_event_rule.ci_manual_deploy_global.name
  target_id = aws_lambda_function.lambda_deploy.function_name
  arn       = aws_lambda_function.lambda_deploy.arn
}

resource "aws_lambda_permission" "ci_manual_deploy_global" {
  statement_id  = "AllowEventBridgeManualGlobal"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.lambda_deploy.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.ci_manual_deploy_global.arn
}

# ============================================================================
# CI Lambda — S3 env file rules
# ============================================================================
#
# Driven by every env file in the project, backend and per-service alike. The
# rules used to iterate the backend list only, so a per-service env file got no
# rule at all and could never trigger anything.

resource "aws_cloudwatch_event_rule" "s3_env_file_change_rule" {
  for_each = { for file in local.all_env_files_s3 : "${file.bucket}-${file.key}" => file }

  # Account+region-global, and PutRule is an upsert — so two projects declaring
  # the same bucket/key silently shared one rule. The name is derived from the
  # raw YAML bucket/key, which carries no project, hence the explicit prefix.
  name        = "s3-env-${var.project}-${var.env}-${local.s3_event_rule_names[each.key]}"
  description = "Event rule for S3 env file changes for ${each.value.bucket}/${each.value.key}"
  event_pattern = jsonencode({
    "source" : ["aws.s3"],
    "detail-type" : ["AWS API Call via CloudTrail"],
    "detail" : {
      "eventSource" : ["s3.amazonaws.com"],
      "eventName" : ["PutObject", "DeleteObject"],
      "requestParameters" : {
        "bucketName" : [each.value.bucket],
        "key" : [each.value.key]
      }
    }
  })

  tags = {
    Name        = "s3-env-${local.s3_event_rule_names[each.key]}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_cloudwatch_event_target" "lambda_target" {
  for_each = { for file in local.all_env_files_s3 : "${file.bucket}-${file.key}" => file }

  rule      = aws_cloudwatch_event_rule.s3_env_file_change_rule[each.key].name
  target_id = aws_lambda_function.lambda_deploy.function_name
  arn       = aws_lambda_function.lambda_deploy.arn
}

resource "aws_lambda_permission" "allow_eventbridge" {
  for_each = { for file in local.all_env_files_s3 : "${file.bucket}-${file.key}" => file }

  # AddPermission caps StatementId at 100 characters. each.key is
  # "${bucket}-${key}", which is unbounded: a 35-char bucket plus a 44-char
  # nested key already overflows, and terraform surfaces it as a bare
  # ValidationException with nothing naming the length. for_each now runs over
  # all_env_files_s3 rather than the backend-only list, so the deeper
  # per-service keys are newly reachable here. Reuse the same bounded, md5-unique
  # name the rule above is built from (<=21 chars) instead of the raw key.
  statement_id  = "AllowEventBridgeS3_${local.s3_event_rule_names[each.key]}"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.lambda_deploy.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.s3_env_file_change_rule[each.key].arn
}


# ============================================================================
# CI Lambda — environment variable maps
# ============================================================================

locals {
  # Every env file in the project: the backend's and every service's.
  all_env_files_s3 = distinct(concat(
    local.env_files_s3,
    flatten([for _, files in local.services_env_files_s3 : files]),
  ))

  # Create sanitized names for CloudWatch Event Rules
  # AWS CloudWatch Event Rule names must:
  # 1. Match pattern ^[0-9A-Za-z_.-]+$
  # 2. Be 64 characters or less
  s3_event_rule_keys = {
    for file in local.all_env_files_s3 : "${file.bucket}-${file.key}" => {
      # Sanitize: replace / and other invalid chars with _
      sanitized = replace(replace("${file.bucket}-${file.key}", "/", "_"), ".", "_")
      # Short hash for uniqueness. Includes project and env so that two projects
      # declaring the same bucket/key still produce distinct rule names.
      hash = substr(md5("${var.project}-${var.env}-${file.bucket}-${file.key}"), 0, 8)
    }
  }

  # The rule name is assembled as:
  #   "s3-env-" (7) + project + "-" + env + "-" + sanitized (<=12) + "-" + hash (8)
  # which stays inside the 64-character EventBridge limit for any realistic
  # project and environment name. The hash is always appended rather than only
  # on truncation, so uniqueness never depends on the readable part surviving.
  s3_event_rule_names = {
    for key, val in local.s3_event_rule_keys : key => "${substr(val.sanitized, 0, 12)}-${val.hash}"
  }

  // Identifier -> actual ECS resource names. Keys come from the identifiers
  // module; no identifier string is written literally here.
  //
  // This map answers one question — what ECS resources does this identifier
  // name — and the auto-deploy policy is deliberately not folded into it. See
  // local.auto_deploy_map.
  ecs_service_map = jsonencode(merge(
    {
      (module.ci_identifiers.backend_id) = {
        service_name = aws_ecs_service.backend.name
        task_family  = aws_ecs_task_definition.backend.family
      }
    },
    {
      for key, service in local.service_names : (module.ci_identifiers.service_ids[key]) => {
        service_name = aws_ecs_service.services[key].name
        task_family  = aws_ecs_task_definition.services[key].family
      }
    }
  ))

  // Identifier -> may an event redeploy this target on its own?
  //
  // A map of its own, a sibling of the four above, for three reasons:
  //   - each of the other maps answers exactly one question, and a policy edit
  //     should not rewrite a map that describes resource identity;
  //   - `aws lambda get-function-configuration` shows the whole policy in one
  //     value, with no nested objects to read through;
  //   - its key set can be asserted against the union of ECS_SERVICE_MAP and
  //     SCHEDULED_TASK_MAP, in the boundary test and again at runtime in
  //     config.SelfCheck, so a target present in one and missing from the other
  //     fails instead of quietly defaulting.
  //
  // It is a flag, never a filter. Every target stays in every map and every
  // repository stays in the ECR event rule, so a push to a disabled target
  // still invokes the Lambda and still writes a line naming the reason:
  // "auto_deploy is disabled for api", not "no target uses repository
  // acme_service_api", which would be a lie that reads like a naming bug.
  auto_deploy_map = jsonencode(module.ci_identifiers.auto_deploy)

  // Identifier -> the env files it consumes. Bucket names are used verbatim,
  // exactly as backend.tf and services.tf spell them in environmentFiles and in
  // the S3 IAM policies.
  s3_to_service_map = jsonencode(merge(
    length(local.env_files_s3) > 0 ? {
      (module.ci_identifiers.backend_id) = local.env_files_s3
    } : {},
    {
      for service_name, files in local.services_env_files_s3 :
      (module.ci_identifiers.service_ids[service_name]) => files
      if length(files) > 0
    }
  ))

  // Scheduled tasks have no ECS service; a deploy registers a new task
  // definition revision and the scheduler picks it up on the next run.
  //
  // Outside dev this map lists targets that no *automatic* trigger can reach:
  // modules/ecs_task creates the task's ECR repository only in dev (so
  // local.ci_task_repos is empty elsewhere and no task appears in ECR_REPO_MAP),
  // and handler/ssm.go deliberately never redeploys a task on a parameter
  // change. The entries are still correct — they name a real task family and a
  // real manual-deploy target — but see ci_lambda/README.md's event table for
  // which trigger exists where, and do not read a populated map here as "prod
  // auto-deploys its scheduled tasks".
  scheduled_task_map = jsonencode({
    for name in var.scheduled_task_names : (module.ci_identifiers.task_ids[name]) => {
      task_family = "${var.project}_task_${name}_${var.env}"
      type        = "scheduled_task"
    }
  })
}
