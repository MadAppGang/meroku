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
