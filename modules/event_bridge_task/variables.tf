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

variable "subnet_ids" {
  type = list(string)
}

# See modules/workloads/variables.tf for the full rationale. Event-driven tasks
# are short-lived, but they must follow the environment's egress strategy: in a
# NAT environment subnet_ids are private and a public address would be
# unroutable.
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

variable "detail_types" {
  description = "Legacy: Detail types for single rule (use 'rules' variable instead for multiple rules)"
  type        = list(string)
  default     = []
}

variable "sources" {
  description = "Legacy: Sources for single rule (use 'rules' variable instead for multiple rules)"
  type        = list(string)
  default     = []
}

variable "task_count" {
  type    = number
  default = 1
}

variable "rule_name" {
  description = "Legacy: Rule name for single rule (use 'rules' variable instead for multiple rules)"
  type        = string
  default     = ""
}

# New multi-rule support (Schema v13)
variable "rules" {
  description = "Map of EventBridge rules. Each rule has sources and detail_types. Preferred over legacy single-rule variables."
  type = map(object({
    sources      = list(string)
    detail_types = list(string)
  }))
  default = {}
}

variable "sqs_queue_url" {
  default = ""
}

variable "sqs_policy_arn" {
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
