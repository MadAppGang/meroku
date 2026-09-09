
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

# ---------------------------------------------------------------------------
# The deploy role's permissions live in ../github_policy, not here.
#
# The reason is testability, and it is not a preference. Every test file in
# modules/workloads/tests/ must declare `mock_data "aws_iam_policy_document"`
# with a stub json string — a generated mock value is not a policy document and
# the AWS provider validates that CLIENT-side, so without the stub seven
# resources in this module fail with `"policy" contains an invalid JSON policy`
# before a single assertion runs. While the document was declared in this file,
# `data.aws_iam_policy_document.github.json` was therefore structurally
# unreadable from any test in this directory: whatever the statement blocks
# said, the attribute returned the stub. `override_data` does not rescue it
# either — it REPLACES a mocked address's values with hand-written ones, moving
# in the same direction as the mock, and there is no construct that un-mocks an
# address back to the real provider.
#
# A leaf module has none of that. It declares no other policy resource, so it
# needs no mock, and `aws_iam_policy_document` renders client-side — so
# ../github_policy/tests/ can plan the real provider with no credentials and no
# network and read the JSON the role actually receives. That is the only place
# the ECR scoping property can be checked against the rendered policy rather
# than against the locals it is built from.
#
# The account ID and region are passed IN for the same reason: a data source in
# the leaf module would make it unplannable without AWS, which is the whole
# problem this move solves. See ../github_policy/main.tf for the ECR reasoning
# that used to be here.
# ---------------------------------------------------------------------------
module "github_policy" {
  source = "../github_policy"

  project            = var.project
  env                = var.env
  aws_account_id     = local.aws_account_id
  region             = data.aws_region.current.name
  ecr_strategy       = var.ecr_strategy
  ecr_account_id     = var.ecr_account_id
  ecr_account_region = var.ecr_account_region
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

# Separate policy attachment (replaces deprecated inline_policy).
#
# This is the ONLY policy on the role, and it must stay that way: an
# aws_iam_role_policy_attachment to a managed policy such as
# AmazonEC2ContainerRegistryPowerUser — the reflex fix when a deploy fails with
# AccessDenied — would restore account-wide ECR write on top of the scoped grant
# in ../github_policy, and no test in that module can see it: it renders one
# document and knows nothing about what else is attached to the role.
resource "aws_iam_role_policy" "github_access" {
  count  = var.github_oidc_enabled ? 1 : 0
  name   = "GithubAccessPolicy"
  role   = aws_iam_role.github_role[0].id
  policy = module.github_policy.json
}
