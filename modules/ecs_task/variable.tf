
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

variable "schedule_expression_timezone" {
  description = "IANA timezone the schedule expression is evaluated in. DST-aware, so a cron set for 09:00 stays at 09:00 local across the change."
  type        = string
  default     = "UTC"
}

variable "max_retry_attempts" {
  description = "Maximum retry attempts for the schedule target. null (the default) omits the retry_policy block entirely so AWS keeps its own default of 185; setting a bare default here would silently cut every existing task's retry budget to that number on the next apply."
  type        = number
  default     = null

  validation {
    condition     = var.max_retry_attempts == null || (var.max_retry_attempts >= 0 && var.max_retry_attempts <= 185 && floor(var.max_retry_attempts) == var.max_retry_attempts)
    error_message = "max_retry_attempts must be a whole number from 0 to 185."
  }
}

variable "dlq_arn" {
  description = "SQS queue ARN used as the schedule target's dead-letter destination. Empty (the default) disables the DLQ and the IAM grant that goes with it."
  type        = string
  default     = ""
}

variable "subnet_ids" {
  type = list(string)
}

# See modules/workloads/variables.tf for the full rationale. Scheduled tasks are
# short-lived, so their share of the public IPv4 charge is small either way, but
# they must follow the environment's egress strategy: in a NAT environment
# subnet_ids are private and a public address would be unroutable.
variable "assign_public_ip" {
  description = "Give the task a public IPv4 address. False when a NAT Gateway provides egress."
  type        = bool
  default     = true
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

