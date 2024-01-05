data "aws_organizations_organization" "org" {}

// mockoon 
resource "aws_ecr_repository" "mockoon" {
  name  = "${var.project}_mockoon"
  count = var.env == "dev" ? 1 : 0

  tags = {
    terraform = "true"
  }
}

resource "aws_ecr_repository_policy" "mockoon" {
  repository = join("", aws_ecr_repository.mockoon.*.name)
  policy     = data.aws_iam_policy_document.default_ecr_policy.json
  count      = var.env == "dev" ? 1 : 0
}

resource "aws_ecr_lifecycle_policy" "mockoon" {
  repository = join("", aws_ecr_repository.mockoon.*.name)
  policy     = var.ecr_lifecycle_policy
  count      = var.env == "dev" ? 1 : 0
}

// backend 
resource "aws_ecr_repository" "backend" {
  name  = "${var.project}_backend"
  count = var.env == "dev" ? 1 : 0

  tags = {
    terraform = "true"
  }
}

resource "aws_ecr_repository_policy" "backend" {
  repository = join("", aws_ecr_repository.backend.*.name)
  policy     = data.aws_iam_policy_document.default_ecr_policy.json
  count      = var.env == "dev" ? 1 : 0
}


// policies
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



