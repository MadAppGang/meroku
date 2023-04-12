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

# https://docs.aws.amazon.com/scheduler/latest/UserGuide/schedule-types.html?icmpid=docs_console_unmapped#rate-based
variable "schedule" {
  type = string
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
