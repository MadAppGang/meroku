
data "archive_file" "lambda" {
  type        = "zip"
  source_file = var.lambda_path
  output_path = "ci_lambda.zip"
}


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

# Lambda function names are account+region-global — see the log group above.
resource "aws_lambda_function" "lambda_deploy" {
  filename         = "ci_lambda.zip"
  function_name    = "${var.project}_ci_lambda_${var.env}"
  handler          = "bootstrap"
  role             = aws_iam_role.lambda_deploy_iam.arn
  source_code_hash = data.archive_file.lambda.output_base64sha256
  runtime          = "provided.al2"

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

      # Slack Configuration (if set, notifications are enabled)
      SLACK_WEBHOOK_URL = var.slack_deployment_webhook

      # Legacy S3 Service Configuration (kept for backward compatibility)
      SERVICE_CONFIG = local.service_config

      # Deployment Configuration
      # Note: DEPLOYMENT_TIMEOUT_SECONDS is currently unused — the Lambda does
      # fire-and-forget deployments (no waiter). Reserved for future use if we
      # add deployment stability polling.
      DEPLOYMENT_TIMEOUT_SECONDS = "600"   # 10 minutes (reserved for future waiter)
      MAX_DEPLOYMENT_RETRIES     = "2"     # Retry failed UpdateService/RegisterTaskDefinition calls
      DRY_RUN                    = "false" # Set to true for testing without actual deployments

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
  statement {
    effect = "Allow"
    actions = [
      "ecs:DescribeTaskDefinition",
      "ecs:ListTaskDefinitions",
      "ecs:RegisterTaskDefinition",
      "ecs:TagResource",
      "ecs:UpdateService",
      "iam:PassRole"
    ]
    resources = ["*"]
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

# EventBus For ECR
# NOTE: this name was hardcoded, with neither project nor env. EventBridge
# PutRule/PutTargets are upserts rather than creates, so a colliding apply did
# not error — it silently repointed another project's deploy trigger at this
# project's Lambda. Scoping the name is what makes that impossible.
resource "aws_cloudwatch_event_rule" "ecr_event" {
  name        = "${var.project}_ecr_events_cicd_${var.env}"
  description = "Emmit ECR event on new image push"
  event_pattern = jsonencode({
    source = [
      "aws.ecr",
      "aws.ecs",
      "aws.ssm",
      "action.production",
      "action.deploy",
    ]
    detail-type = [
      "ECR Image Action",
      "ECS Deployment State Change",
      "ECS Service Action",
      "Parameter Store Change",
      "DEPLOY",
    ]
  })

  tags = {
    Name        = "${var.project}_ecr_events_cicd_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_cloudwatch_event_target" "lambda" {
  rule      = aws_cloudwatch_event_rule.ecr_event.name
  target_id = aws_lambda_function.lambda_deploy.function_name
  arn       = aws_lambda_function.lambda_deploy.arn
}

resource "aws_lambda_permission" "ecr_event_call_deploy_lambda" {
  statement_id  = "AllowExecutionFromCloudWatch"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.lambda_deploy.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.ecr_event.arn
}


# Add S3 bucket notification
resource "aws_cloudwatch_event_rule" "s3_env_file_change_rule" {
  for_each = { for file in local.env_files_s3 : "${file.bucket}-${file.key}" => file }

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
  for_each = { for file in local.env_files_s3 : "${file.bucket}-${file.key}" => file }

  rule      = aws_cloudwatch_event_rule.s3_env_file_change_rule[each.key].name
  target_id = aws_lambda_function.lambda_deploy.function_name
  arn       = aws_lambda_function.lambda_deploy.arn
}

resource "aws_lambda_permission" "allow_eventbridge" {
  for_each = { for file in local.env_files_s3 : "${file.bucket}-${file.key}" => file }

  statement_id  = "AllowExecutionFromEventBridge_${replace(each.key, "/[^a-zA-Z0-9_-]/", "_")}"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.lambda_deploy.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.s3_env_file_change_rule[each.key].arn
}


// pass a list of services and env files to the lambda
// to know which service to restart on file change
locals {
  # Create sanitized names for CloudWatch Event Rules
  # AWS CloudWatch Event Rule names must:
  # 1. Match pattern ^[0-9A-Za-z_.-]+$
  # 2. Be 64 characters or less
  s3_event_rule_keys = {
    for file in local.env_files_s3 : "${file.bucket}-${file.key}" => {
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

  service_config = jsonencode({
    "${var.project}" = [
      for file in local.env_files_s3 : {
        bucket = file.bucket
        key    = file.key
      }
    ]
  })

  // Map service identifiers to actual ECS resource names
  // This eliminates the need for pattern-based name construction
  ecs_service_map = jsonencode(merge(
    {
      // Backend service (identifier: "backend")
      "backend" = {
        service_name = aws_ecs_service.backend.name
        task_family  = aws_ecs_task_definition.backend.family
      }
    },
    {
      // Named services (identifier: service name like "api", "worker")
      for key, service in local.service_names : key => {
        service_name = aws_ecs_service.services[key].name
        task_family  = aws_ecs_task_definition.services[key].family
      }
    }
  ))

  // S3 file to service mapping for faster lookups
  s3_to_service_map = jsonencode({
    for service_name, files in local.services_env_files_s3 : service_name => [
      for file in files : {
        bucket = "${var.project}-${file.bucket}-${var.env}"
        key    = file.key
      }
    ]
  })

  // Scheduled task map — maps "task:{name}" identifiers to task definition families.
  // The Lambda uses this to register a new task definition revision when an ECR image
  // is pushed to a {project}_task_{name} repository.
  // Keys must match the identifier returned by GetServiceNameFromRepoName, i.e. "task:{name}".
  scheduled_task_map = jsonencode({
    for name in var.scheduled_task_names : "task:${name}" => {
      task_family = "${var.project}_task_${name}_${var.env}"
      type        = "scheduled_task"
    }
  })
}