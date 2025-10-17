
resource "aws_iam_openid_connect_provider" "github" {
  url   = "https://token.actions.githubusercontent.com"
  count = var.github_oidc_enabled ? 1 : 0

  client_id_list = [
    "sts.amazonaws.com"
  ]

  thumbprint_list = [
    "6938fd4d98bab03faadb97b34396831e3780aea1",
    "1c58a3a8518e8759bf075b76b750d4f2df264fcd" # https://github.blog/changelog/2023-06-27-github-actions-update-on-oidc-integration-with-aws/
  ]

  tags = {
    Name        = "github-actions-oidc-${var.project}-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

data "aws_iam_policy_document" "github_trust_relationship" {
  count = var.github_oidc_enabled ? 1 : 0
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github[0].arn]
    }
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }
    condition {
      test     = "StringLike"
      variable = "token.actions.githubusercontent.com:sub"
      values   = var.github_subjects
    }
  }
}

resource "aws_iam_role" "github_role" {
  count              = var.github_oidc_enabled ? 1 : 0
  name               = "GithubActionsRole"
  assume_role_policy = data.aws_iam_policy_document.github_trust_relationship[0].json

  tags = {
    Name        = "GithubActionsRole"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# Separate policy attachment (replaces deprecated inline_policy)
resource "aws_iam_role_policy" "github_access" {
  count  = var.github_oidc_enabled ? 1 : 0
  name   = "GithubAccessPolicy"
  role   = aws_iam_role.github_role[0].id
  policy = data.aws_iam_policy_document.github.json
}

data "aws_iam_policy_document" "github" {
  statement {
    effect = "Allow"
    actions = [
      "ecr:CompleteLayerUpload",
      "ecr:GetAuthorizationToken",
      "ecr:UploadLayerPart",
      "ecr:BatchGetImage",
      "ecr:InitiateLayerUpload",
      "ecr:BatchCheckLayerAvailability",
      "ecr:PutImage",
      "ecr:DescribeRepositories",
      "ecr:CreateRepository",
      "ecs:UpdateService",
      "ecs:DescribeServices",
      "events:PutEvents"
    ]
    resources = ["*"]
  }
}
