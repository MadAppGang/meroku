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

variable "vpc_id" {
  type = string
}

variable "cluster" {
  type = string
}

variable "detail_types" {
  type = list(string)
}

variable "sources" {
  type = list(string)
}

variable "task_count" {
  type    = number
  default = 1
}

variable "rule_name" {
  type = string
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

data "aws_ssm_parameters_by_path" "task" {
  path      = "/${var.env}/${var.project}/task/${var.task}"
  recursive = true
}

locals {
  task_env_ssm = [
    for i in range(length(data.aws_ssm_parameters_by_path.task.names)) : {
      name      = reverse(split("/", data.aws_ssm_parameters_by_path.task.names[i]))[0]
      valueFrom = data.aws_ssm_parameters_by_path.task.names[i]
    }
  ]
}

