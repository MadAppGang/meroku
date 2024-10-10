
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

variable "subnet_ids" {
  type = list(string)
}

variable "vpc_id" {
  type = string
}

variable "cluster" {
  type = string
}

variable "allow_public_access" {
  type    = bool
  default = false
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

