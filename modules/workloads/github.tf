
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

  inline_policy {
    name   = "GithubAccessPolicy"
    policy = data.aws_iam_policy_document.github.json
  }
}

data "aws_iam_policy_document" "github" {
  statement {
    effect = "Allow"
    actions = [
      "ecr:CompleteLayerUpload",
      "ecr:GetAuthorizationToken",
      "ecr:UploadLayerPart",
      "ecr:InitiateLayerUpload",
      "ecr:BatchCheckLayerAvailability",
      "ecr:PutImage",
      "events:PutEvents"
    ]
    resources = ["*"]
  }
}
