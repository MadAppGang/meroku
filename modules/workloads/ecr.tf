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

# ============================================================================
# Per-Service ECR Repositories (Schema v10)
# ============================================================================

# Create local variables for ECR management
locals {
  # Services that need ECR repositories created (mode = create_ecr)
  services_needing_ecr = [
    for svc in var.services :
    svc if try(svc.ecr_config.mode, "create_ecr") == "create_ecr"
  ]

  # Build a map of all ECR repository URLs (for lookup by use_existing services)
  # Format: { "services-api" = "123456789012.dkr.ecr.us-east-1.amazonaws.com/project_service_api", ... }
  ecr_repository_map = merge(
    # Service repositories
    {
      for svc in local.services_needing_ecr :
      "services-${svc.name}" => aws_ecr_repository.services[svc.name].repository_url
    }
  )

  # Resolve ECR URL for each service based on its configuration
  service_ecr_urls = {
    for svc in var.services :
    svc.name => (
      # Mode: create_ecr - use the repository we created
      try(svc.ecr_config.mode, "create_ecr") == "create_ecr" ?
      aws_ecr_repository.services[svc.name].repository_url :

      # Mode: manual_repo - use the provided URI
      try(svc.ecr_config.mode, "create_ecr") == "manual_repo" ?
      svc.ecr_config.repository_uri :

      # Mode: use_existing - lookup from the repository map
      lookup(
        local.ecr_repository_map,
        "${svc.ecr_config.source_service_type}-${svc.ecr_config.source_service_name}",
        ""
      )
    )
  }
}

# Create ECR repositories for services with mode=create_ecr
resource "aws_ecr_repository" "services" {
  for_each = { for svc in local.services_needing_ecr : svc.name => svc }

  name = "${var.project}_service_${each.value.name}"

  tags = {
    Name        = "${var.project}_service_${each.value.name}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
    ServiceName = each.value.name
  }
}

# Apply default ECR policy to service repositories (no trusted accounts)
resource "aws_ecr_repository_policy" "services_default" {
  for_each = {
    for svc in local.services_needing_ecr :
    svc.name => svc
    if length(var.ecr_trusted_accounts) == 0
  }

  repository = aws_ecr_repository.services[each.key].name
  policy     = data.aws_iam_policy_document.default_ecr_policy.json
}

# Apply trusted accounts policy to service repositories (with trusted accounts)
resource "aws_ecr_repository_policy" "services_trusted" {
  for_each = {
    for svc in local.services_needing_ecr :
    svc.name => svc
    if length(var.ecr_trusted_accounts) > 0
  }

  repository = aws_ecr_repository.services[each.key].name
  policy     = data.aws_iam_policy_document.ecr_trusted_accounts_policy[0].json
}

