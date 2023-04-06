
resource "aws_iam_openid_connect_provider" "github" {
  url = "https://token.actions.githubusercontent.com" 

  client_id_list = [
    "sts.amazonaws.com"
  ]

  thumbprint_list = ["6938fd4d98bab03faadb97b34396831e3780aea1"]
}

data "aws_iam_policy_document" "github_trust_relationship" {
  statement {
    effect = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github.arn]
    }
    condition {
      test = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "github_role" {
   name               = "GithubActionsRole"
   assume_role_policy = data.aws_iam_policy_document.github_trust_relationship.json

  inline_policy {
    name = "GithubAccessPolicy"
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
                "ecs:DescribeTaskDefinition",
                "ecs:UpdateService",
                "iam:PassRole"
            ]
            resources = ["*"]
  }
}
