data "aws_organizations_organization" "org" {}

// backend
resource "aws_ecr_repository" "backend" {
  name  = "${var.project}_backend"
  count = var.ecr_strategy == "local" ? 1 : 0

  tags = {
    Name        = "${var.project}_backend"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_ecr_repository_policy" "backend" {
  repository = join("", aws_ecr_repository.backend.*.name)
  policy     = data.aws_iam_policy_document.default_ecr_policy.json
  count      = var.ecr_strategy == "local" && length(var.ecr_trusted_accounts) == 0 ? 1 : 0
}

// Trusted accounts policy for backend ECR repository
resource "aws_ecr_repository_policy" "backend_trusted" {
  repository = join("", aws_ecr_repository.backend.*.name)
  policy     = data.aws_iam_policy_document.ecr_trusted_accounts_policy[0].json
  count      = var.ecr_strategy == "local" && length(var.ecr_trusted_accounts) > 0 ? 1 : 0
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

// Policy document for trusted accounts (cross-account pull access)
data "aws_iam_policy_document" "ecr_trusted_accounts_policy" {
  count = var.ecr_strategy == "local" && length(var.ecr_trusted_accounts) > 0 ? 1 : 0

  # Allow same-account full access
  statement {
    sid    = "AllowSameAccountAccess"
    effect = "Allow"

    principals {
      type        = "AWS"
      identifiers = ["arn:aws:iam::${data.aws_caller_identity.current.account_id}:root"]
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
      "ecr:DescribeImages",
      "ecr:ListImages"
    ]
  }

  # Allow cross-account pull-only access
  statement {
    sid    = "AllowCrossAccountPullOnly"
    effect = "Allow"

    principals {
      type        = "AWS"
      identifiers = [for acct in var.ecr_trusted_accounts : "arn:aws:iam::${acct.account_id}:root"]
    }

    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:BatchGetImage",
      "ecr:GetDownloadUrlForLayer",
      "ecr:DescribeImages",
      "ecr:DescribeRepositories"
    ]
  }
}

