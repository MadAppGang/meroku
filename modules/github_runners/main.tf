# CodeBuild-hosted GitHub Actions runners.
#
# One CodeBuild project per runner. Its webhook fires on WORKFLOW_JOB_QUEUED,
# and CodeBuild then starts a build that registers itself with GitHub as an
# ephemeral self-hosted runner for that one job. A workflow selects it with
#
#   runs-on: codebuild-<project name>-${{ github.run_id }}-${{ github.run_attempt }}
#
# which the runs_on_labels output spells out per runner.
#
# GitHub credentials are project-level (source.auth), never an
# aws_codebuild_source_credential: that resource is one per account and
# server type, so two environments in one account would overwrite each
# other's token.
#
# Each webhook has two halves. aws_codebuild_webhook is created with
# manual_creation, so CodeBuild returns a payload URL and a secret and does
# not call GitHub. The GitHub half is a github_repository_webhook (or
# github_organization_webhook) pointed at that URL. When CodeBuild creates the
# GitHub hook itself, CreateWebhook reads the token through the service role,
# and on the apply that creates the role IAM has not propagated it yet, so the
# call fails. With the split, nothing uses the role until a build starts.

data "aws_caller_identity" "current" {}
data "aws_region" "current" {}
data "aws_partition" "current" {}

# Created outside Terraform so the token never lands in state.
data "aws_secretsmanager_secret" "token" {
  name = startswith(var.token_secret, "arn:") ? null : var.token_secret
  arn  = startswith(var.token_secret, "arn:") ? var.token_secret : null
}

locals {
  runners = { for r in var.runners : r.name => r }

  account_id = data.aws_caller_identity.current.account_id
  region     = data.aws_region.current.region
  partition  = data.aws_partition.current.partition

  project_names = { for name, r in local.runners : name => "${var.project}-gh-${name}-${var.env}" }
  log_groups    = { for name, r in local.runners : name => "${var.project}_gh_runner_${name}_${var.env}" }
}

resource "aws_cloudwatch_log_group" "runner" {
  for_each = local.runners

  name              = local.log_groups[each.key]
  retention_in_days = 7
}

# Builds in the VPC need a security group. Nothing connects to a runner, so it
# has no ingress; all egress, because a CI job downloads from anywhere.
resource "aws_security_group" "runner" {
  count = var.vpc_access ? 1 : 0

  name        = "${var.project}_gh_runners_${var.env}"
  description = "GitHub Actions runners on CodeBuild: egress only"
  vpc_id      = var.vpc_id

  egress {
    description      = "All outbound, through the NAT Gateway"
    from_port        = 0
    to_port          = 0
    protocol         = "-1"
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }
}

resource "aws_codebuild_project" "runner" {
  for_each = local.runners

  name          = local.project_names[each.key]
  description   = "GitHub Actions runner for ${each.value.organization != "" ? "the ${each.value.organization} organization" : each.value.repository}"
  service_role  = aws_iam_role.runner.arn
  build_timeout = each.value.timeout_minutes

  artifacts {
    type = "NO_ARTIFACTS"
  }

  environment {
    type                        = strcontains(each.value.image, "aarch64") ? "ARM_CONTAINER" : "LINUX_CONTAINER"
    compute_type                = each.value.compute_type
    image                       = each.value.image
    privileged_mode             = each.value.docker
    image_pull_credentials_type = "CODEBUILD"

    dynamic "environment_variable" {
      for_each = each.value.env_variables
      content {
        name  = environment_variable.value.name
        value = environment_variable.value.value
        type  = "PLAINTEXT"
      }
    }

    dynamic "environment_variable" {
      for_each = [for s in each.value.secrets : s if s.ssm != ""]
      content {
        name  = environment_variable.value.name
        value = environment_variable.value.ssm
        type  = "PARAMETER_STORE"
      }
    }

    dynamic "environment_variable" {
      for_each = [for s in each.value.secrets : s if s.secrets_manager != ""]
      content {
        name  = environment_variable.value.name
        value = environment_variable.value.secrets_manager
        type  = "SECRETS_MANAGER"
      }
    }
  }

  source {
    type = "GITHUB"
    # An organization webhook serves every repository in the organization, so
    # the project has no single repository to point at.
    location  = each.value.organization != "" ? "CODEBUILD_DEFAULT_WEBHOOK_SOURCE_LOCATION" : "https://github.com/${each.value.repository}.git"
    buildspec = ""

    auth {
      type     = "SECRETS_MANAGER"
      resource = data.aws_secretsmanager_secret.token.arn
    }
  }

  logs_config {
    cloudwatch_logs {
      status     = "ENABLED"
      group_name = aws_cloudwatch_log_group.runner[each.key].name
    }
  }

  dynamic "vpc_config" {
    for_each = var.vpc_access ? [1] : []
    content {
      vpc_id             = var.vpc_id
      subnets            = var.subnet_ids
      security_group_ids = [aws_security_group.runner[0].id]
    }
  }

  # CodeBuild checks the role can read the token and write logs when the
  # project is created, so the policy must exist first.
  depends_on = [aws_iam_role_policy.runner]
}

resource "aws_codebuild_webhook" "runner" {
  for_each = local.runners

  project_name    = aws_codebuild_project.runner[each.key].name
  build_type      = "BUILD"
  manual_creation = true

  filter_group {
    filter {
      type    = "EVENT"
      pattern = "WORKFLOW_JOB_QUEUED"
    }
  }

  dynamic "scope_configuration" {
    for_each = each.value.organization != "" ? [each.value.organization] : []
    content {
      name  = scope_configuration.value
      scope = "GITHUB_ORGANIZATION"
    }
  }
}

# The GitHub half of each webhook. The caller (env/main.hbs) configures the
# github provider with the token from token_secret and the runners' owner.
resource "github_repository_webhook" "runner" {
  for_each = { for name, r in local.runners : name => r if r.repository != "" }

  repository = split("/", each.value.repository)[1]
  events     = ["workflow_job"]
  active     = true

  configuration {
    url          = aws_codebuild_webhook.runner[each.key].payload_url
    secret       = aws_codebuild_webhook.runner[each.key].secret
    content_type = "json"
    insecure_ssl = false
  }
}

resource "github_organization_webhook" "runner" {
  for_each = { for name, r in local.runners : name => r if r.organization != "" }

  events = ["workflow_job"]
  active = true

  configuration {
    url          = aws_codebuild_webhook.runner[each.key].payload_url
    secret       = aws_codebuild_webhook.runner[each.key].secret
    content_type = "json"
    insecure_ssl = false
  }
}
