
data "aws_caller_identity" "current" {}

resource "aws_iam_role" "task" {
  name               = module.naming.names["task_role"]
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json

  tags = {
    Name        = "${var.project}-${var.task}-task-role-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_role_policy_attachment" "task_cloudwatch" {
  role       = aws_iam_role.task.name
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchFullAccess"
}

resource "aws_iam_role" "task_execution" {
  name               = module.naming.names["task_execution_role"]
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json

  tags = {
    Name        = "${var.project}-scheduler-${var.task}-execution-role-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}


resource "aws_iam_role_policy_attachment" "task_execution" {
  role       = aws_iam_role.task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# EventBridge launches this task, and it launches it as the execution role:
# aws_cloudwatch_event_target.target (main.tf) passes
# role_arn = aws_iam_role.task_execution.arn. The managed policy attached above
# carries image pull, log writes and SSM reads — neither ecs:RunTask nor
# iam:PassRole — so without this grant the rule exists, matches every event, and
# starts nothing.
#
# Inline rather than managed: an inline policy's name is scoped to its role, so
# unlike the account-global aws_iam_policy names in this file it cannot collide
# with modules/ecs_task when one task name is used for both kinds of task.
#
# RunTask is granted on the task definition's family with a ":*" revision
# suffix, so registering a new revision — which every deploy does — needs no new
# grant, and the ecs:cluster condition keeps it to the cluster these rules
# target. PassRole covers both roles because ECS is handed both when it starts
# the task, and iam:PassedToService confines it to handing them to ECS.
resource "aws_iam_role_policy" "event_task_launch" {
  name = "EventTaskLaunch_${var.project}_${var.task}_${var.env}"
  role = aws_iam_role.task_execution.name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = ["ecs:RunTask"]
        Resource = ["${aws_ecs_task_definition.task.arn_without_revision}:*"]
        Condition = {
          ArnEquals = {
            "ecs:cluster" = [var.cluster]
          }
        }
      },
      {
        Effect   = "Allow"
        Action   = ["iam:PassRole"]
        Resource = [aws_iam_role.task.arn, aws_iam_role.task_execution.arn]
        Condition = {
          StringEquals = {
            "iam:PassedToService" = ["ecs-tasks.amazonaws.com"]
          }
        }
      },
    ]
  })
}

data "aws_iam_policy_document" "ecs_tasks_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }


  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["events.amazonaws.com"]
    }
  }
}

// SSM IAM access policy
resource "aws_iam_role_policy_attachment" "ssm_parameter_access" {
  role       = aws_iam_role.task.name
  policy_arn = aws_iam_policy.ssm_parameter_access.arn
}

# The same policy on the execution role, because ECS resolves every entry in the
# container's `secrets` AS THE EXECUTION ROLE — while it assembles the container
# definition, before any of the task's own code runs. The attachment above
# covers the container reading parameters itself; without this one the task
# stops with "unable to pull secrets or registry auth: access denied".
resource "aws_iam_role_policy_attachment" "ssm_parameter_access_task_execution" {
  role       = aws_iam_role.task_execution.name
  policy_arn = aws_iam_policy.ssm_parameter_access.arn
}

resource "aws_iam_role_policy_attachment" "sqs_access" {
  count      = var.sqs_enable == true ? 1 : 0
  role       = aws_iam_role.task.name
  policy_arn = var.sqs_policy_arn
}

# SES email sending permissions
resource "aws_iam_policy" "send_emails" {
  count = var.ses_enabled ? 1 : 0
  name  = "SendSESEmails_${var.project}_event_${var.task}_${var.env}"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "ses:SendEmail",
          "ses:SendRawEmail"
        ]
        Resource = "*"
      }
    ]
  })

  tags = {
    Name        = "SendSESEmails_${var.project}_event_${var.task}_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_role_policy_attachment" "ses_access" {
  count      = var.ses_enabled ? 1 : 0
  role       = aws_iam_role.task.name
  policy_arn = aws_iam_policy.send_emails[0].arn
}

# EventBridge read access: the buses and rules the task may describe.
# PutEvents is not here; modules/events grants it per events.produces entry.
resource "aws_iam_policy" "eventbridge_access" {
  name = "EventBridgeAccess_${var.project}_event_${var.task}_${var.env}"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "events:DescribeEventBus",
          "events:ListRules",
          "events:DescribeRule",
          "events:ListTargetsByRule",
          "events:ListEventBuses"
        ]
        Resource = local.eventbridge_read_resources
      }
    ]
  })

  tags = {
    Name        = "EventBridgeAccess_${var.project}_event_${var.task}_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_role_policy_attachment" "task_eventbridge" {
  role       = aws_iam_role.task.name
  policy_arn = aws_iam_policy.eventbridge_access.arn
}
resource "aws_iam_policy" "ssm_parameter_access" {
  # Account-global, and additionally collides with modules/ecs_task when the
  # same task name is used for both kinds of task in one project.
  name   = "Task${var.task}SSMAccessPolicy_${var.project}_${var.env}"
  policy = data.aws_iam_policy_document.ssm_parameter_access.json

  tags = {
    Name        = "Task-${var.task}-SSM-Access-Policy"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

data "aws_iam_policy_document" "ssm_parameter_access" {
  statement {
    actions = ["ssm:GetParameter", "ssm:GetParameters", "ssm:GetParametersByPath"]
    # `parameter` is the resource-type segment of an SSM parameter ARN. Without
    # it the ARN denotes nothing SSM has, the statement matches no parameter in
    # the account, and IAM still accepts the policy.
    resources = ["arn:aws:ssm:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:parameter/${var.env}/${var.project}/task/${var.task}/*"]
  }
}
