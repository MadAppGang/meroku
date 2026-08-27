
# The OIDC provider is account-scoped, not project-scoped: AWS keys it on the
# issuer URL, which the ARN embeds as its resource path
# (arn:aws:iam::<account>:oidc-provider/token.actions.githubusercontent.com).
# There is no field left over to distinguish two of them, so the first project
# in an account creates it and every later project federates against that one
# by setting github_oidc_create_provider = false.
#
# thumbprint_list is deliberately absent. AWS verifies this issuer's JWKS
# endpoint against its own trusted root CAs and consults thumbprints only when
# the IdP's certificate is signed by some other CA, which GitHub's is not. The
# two hashes pinned here until now were read by nothing.
resource "aws_iam_openid_connect_provider" "github" {
  url   = "https://token.actions.githubusercontent.com"
  count = var.github_oidc_enabled && var.github_oidc_create_provider ? 1 : 0

  client_id_list = [
    "sts.amazonaws.com"
  ]

  tags = {
    Name        = "github-actions-oidc-${var.project}-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# Resolves the provider another project in this account already owns. Counted
# to the exact complement of the resource above, so precisely one of the two
# exists whenever OIDC is enabled.
data "aws_iam_openid_connect_provider" "github" {
  count = var.github_oidc_enabled && !var.github_oidc_create_provider ? 1 : 0
  url   = "https://token.actions.githubusercontent.com"
}

locals {
  # The create branch must keep referencing the resource attribute rather than
  # building the ARN from the account ID. The reference is what gives Terraform
  # the dependency edge that orders the provider before the role; a derived
  # string would supply the same value with no edge, letting the role be created
  # first and failing the apply with "MalformedPolicyDocument: Invalid principal
  # in policy".
  github_oidc_provider_arn = (
    var.github_oidc_create_provider
    ? one(aws_iam_openid_connect_provider.github[*].arn)
    : one(data.aws_iam_openid_connect_provider.github[*].arn)
  )
}

data "aws_iam_policy_document" "github_trust_relationship" {
  count = var.github_oidc_enabled ? 1 : 0
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = [local.github_oidc_provider_arn]
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
  name               = module.naming.names["github_actions_role"]
  assume_role_policy = data.aws_iam_policy_document.github_trust_relationship[0].json

  tags = {
    Name        = module.naming.names["github_actions_role"]
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
      "ecs:RegisterTaskDefinition",
      "ecs:DescribeTaskDefinition",
      "events:PutEvents"
    ]
    resources = ["*"]
  }

  # iam:PassRole is required when calling ecs:UpdateService or ecs:RegisterTaskDefinition
  # because ECS needs to assume the task execution role to pull images from ECR.
  # Scoped to this project's task and execution roles only.
  statement {
    effect = "Allow"
    actions = [
      "iam:PassRole"
    ]
    resources = [
      "arn:aws:iam::${local.aws_account_id}:role/${var.project}_*_task_${var.env}",
      "arn:aws:iam::${local.aws_account_id}:role/${var.project}_*_task_execution_${var.env}",
      "arn:aws:iam::${local.aws_account_id}:role/${var.project}_scheduler_*_task_execution_${var.env}"
    ]
  }
}
