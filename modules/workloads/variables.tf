locals {
  aws_account_id = data.aws_caller_identity.current.account_id  
}

variable "env" {
  type = string
}

variable "project" {
  type = string
}

variable "vpc_id" {
  type = string
}


variable "subnet_ids" {
  type = list(string)
}

variable "backend_image_port" {
  default = 8080
  type    = number
}

variable "mockoon_image_port" {
  default = 80
  type    = number
}

variable "private_dns_name" {
  type    = string
}


variable "backend_health_endpoint" {
  default = "/health/live"
}

variable "ecr_repository_policy" {
  type    = string
  default = <<EOF
{
    "Version": "2008-10-17",
    "Statement": [
        {
            "Sid": "Default ECR policy",
            "Effect": "Allow",
            "Principal": "*",
            "Action": [
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
    ]
}
EOF
}

variable "domain" {
  type    = string
}

variable "ecr_url" {
  default = ""
}

variable "db_endpoint" {
  default = ""
}

variable "db_user" {
  default = ""
}

variable "db_name" {
  default = ""
}

variable "ecr_lifecycle_policy" {
  type    = string
  default = <<EOF
{
    "rules": [
        {
            "rulePriority": 1,
            "description": "Delete untagged images",
            "selection": {
                "tagStatus": "untagged",
                "countType": "imageCountMoreThan",
                "countNumber": 1
            },
            "action": {
                "type": "expire"
            }
        },
        {
            "rulePriority": 2,
            "description": "Keep no more than 10 recent images",
            "selection": {
                "tagStatus": "any",
                "countType": "imageCountMoreThan",
                "countNumber": 10
            },
            "action": {
                "type": "expire"
            }
        }
    ]
}
EOF
}


locals {


  backend_env = concat([
    for k, v in nonsensitive(jsondecode(data.aws_ssm_parameter.backend_env.value)) : {
      name  = k
      value = v
    }
  ], [
    { "name" : "DATABASE_PASSWORD", "value" : nonsensitive(data.aws_ssm_parameter.postgres_password.value) },
    { "name" : "DATABASE_HOST", "value" : var.db_endpoint },
    { "name" : "DATABASE_USERNAME", "value" : var.db_user },
    { "name" : "PORT", "value" : tostring(var.backend_image_port) },
    { "name" : "DATABASE_NAME", "value" : var.db_name },
    { "name" : "AWS_S3_BUCKET", "value" : "${var.project}-images-${var.env}"},
    { "name" : "AWS_REGION", "value": data.aws_region.current.name },
    { "name" : "URL", "value": "https://api.${var.env}.${var.domain}" },
    { "name" : "PROXY", "value": "true" },
  ])
}


data "aws_ssm_parameter" "postgres_password" {
  name = "/${var.env}/${var.project}/postgres_password"
}


data "aws_ssm_parameter" "backend_env" {
  name = "/${var.env}/${var.project}/backend_env"
}
