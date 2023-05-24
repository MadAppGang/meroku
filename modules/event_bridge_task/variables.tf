
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
  type  = string
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
  type  = list(string)
}

variable "task_count" {
  type = number
  default = 1
}

variable "rule_name" {
  type = string
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
                "ecr:DeleteRepositoryPolicy"
            ]
        }
    ]
}
EOF
}



data "aws_ssm_parameters_by_path" "task" {
  path = "/${var.env}/${var.project}/task/${var.task}"
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