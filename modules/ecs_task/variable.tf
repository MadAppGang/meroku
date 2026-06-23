
data "aws_organizations_organization" "org" {}


variable "env" {
  type    = string
  default = "dev"
}

variable "project" {
  type = string
}


variable "task" {
  type = string
}

variable "ecr_url" {
  type    = string
  default = ""
}

variable "docker_image" {
  type    = string
  default = ""
}

variable "container_command" {
  type    = list(string)
  default = []
}


# https://docs.aws.amazon.com/scheduler/latest/UserGuide/schedule-types.html?icmpid=docs_console_unmapped#rate-based
variable "schedule" {
  type    = string
  default = "rate(1 days)"
}

# IANA timezone for the schedule expression (e.g. "Australia/Sydney" for DST-aware local runs).
# https://docs.aws.amazon.com/scheduler/latest/UserGuide/managing-schedule-scheduleexpressiontimezone.html
variable "schedule_expression_timezone" {
  description = "IANA timezone the schedule expression is evaluated in (DST-aware)"
  type        = string
  default     = "UTC"
}

# EventBridge Scheduler retry policy for the ECS RunTask target.
# https://docs.aws.amazon.com/scheduler/latest/UserGuide/managing-schedule-retries.html
variable "max_retry_attempts" {
  description = "Maximum number of retry attempts for the schedule target invocation"
  type        = number
  default     = 3
}

# Optional dead-letter queue ARN for events the scheduler fails to deliver after retries.
variable "dlq_arn" {
  description = "SQS queue ARN to use as the schedule target dead-letter destination (empty = disabled)"
  type        = string
  default     = ""
}

variable "subnet_ids" {
  type = list(string)
}

variable "vpc_id" {
  type = string
}

variable "cluster" {
  type = string
}

variable "sqs_policy_arn" {
  default = ""
}

variable "sqs_queue_url" {
  default = ""
}

variable "sqs_enable" {
  default = false
}

variable "ses_enabled" {
  description = "Enable SES email sending permissions for this task"
  type        = bool
  default     = false
}

variable "api_domain" {
  description = "External API domain name"
  type        = string
  default     = ""
}

variable "private_dns_name" {
  description = "Private DNS namespace for service discovery"
  type        = string
  default     = ""
}

variable "cpu" {
  description = "CPU units for the task (Fargate)"
  type        = number
  default     = 256
}

variable "memory" {
  description = "Memory in MB for the task (Fargate)"
  type        = number
  default     = 512
}

variable "custom_env_vars" {
  description = "Custom environment variables to pass to the container"
  type = list(object({
    name  = string
    value = string
  }))
  default = []
}

data "aws_iam_policy_document" "default_ecr_policy" {
  statement {
    sid = "Default ECR policy"
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    actions = [
      "ecr:GetDownloadUrlForLayer",
      "ecr:BatchGetImage",
      "ecr:BatchCheckLayerAvailability",
      "ecr:PutImage",
      "ecr:InitiateLayerUpload",
      "ecr:UploadLayerPart",
      "ecr:CompleteLayerUpload",
      "ecr:DescribeRepositories",
      "ecr:GetRepositoryPolicy",
      "ecr:ListImages",
      "ecr:DeleteRepository",
      "ecr:BatchDeleteImage",
      "ecr:SetRepositoryPolicy",
      "ecr:DeleteRepositoryPolicy",
      "ssmmessages:CreateControlChannel",
      "ssmmessages:CreateDataChannel",
      "ssmmessages:OpenControlChannel",
      "ssmmessages:OpenDataChannel"
    ]
  }

  statement {
    sid = "External read ECR policy"
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:BatchGetImage",
      "ecr:DescribeImages",
      "ecr:DescribeRepositories",
      "ecr:GetDownloadUrlForLayer"
    ]
    condition {
      test     = "StringEquals"
      variable = "aws:PrincipalOrgID"
      values   = [data.aws_organizations_organization.org.id]
    }
  }
}

