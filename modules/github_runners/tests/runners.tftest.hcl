# modules/github_runners under mock_provider: plan only, no credentials, no
# network. Account 000000000000 and every ARN below are synthetic; this
# repository is public.
#
# Run: terraform init -backend=false && terraform test  (from modules/github_runners)

mock_provider "aws" {
  mock_data "aws_caller_identity" {
    defaults = {
      account_id = "000000000000"
    }
  }

  mock_data "aws_region" {
    defaults = {
      region = "us-east-1"
      name   = "us-east-1"
    }
  }

  mock_data "aws_partition" {
    defaults = {
      partition = "aws"
    }
  }

  mock_data "aws_secretsmanager_secret" {
    defaults = {
      arn = "arn:aws:secretsmanager:us-east-1:000000000000:secret:meroku/github-runners/acme-dev-AbCdEf"
    }
  }

  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::000000000000:role/acme_gh_runners_dev"
    }
  }

  mock_resource "aws_security_group" {
    defaults = {
      id = "sg-00000000000000000"
    }
  }
}

mock_provider "github" {}

variables {
  project      = "acme"
  env          = "dev"
  token_secret = "meroku/github-runners/acme-dev"

  runners = [
    {
      name       = "default"
      repository = "example-org/some-repo"
      env_variables = [
        { name = "FOO", value = "bar" },
      ]
      secrets = [
        { name = "DB_PASSWORD", ssm = "/acme/dev/db_password" },
        { name = "API_KEY", secrets_manager = "acme/dev/api:key" },
      ]
    },
  ]
}

# ---------------------------------------------------------------------------
# Environment variables: one block per entry, typed by where the value lives.
# ---------------------------------------------------------------------------
run "env_variables_and_secrets_map_to_codebuild_types" {
  command = plan

  assert {
    condition = {
      for v in aws_codebuild_project.runner["default"].environment[0].environment_variable :
      v.name => "${v.type}=${v.value}"
      } == {
      FOO         = "PLAINTEXT=bar"
      DB_PASSWORD = "PARAMETER_STORE=/acme/dev/db_password"
      API_KEY     = "SECRETS_MANAGER=acme/dev/api:key"
    }
    error_message = "env_variables must render PLAINTEXT, ssm secrets PARAMETER_STORE, secrets_manager secrets SECRETS_MANAGER, each with the value as written."
  }

  assert {
    condition     = aws_codebuild_project.runner["default"].name == "acme-gh-default-dev"
    error_message = "The project name is <project>-gh-<runner>-<env>."
  }

  assert {
    condition     = aws_cloudwatch_log_group.runner["default"].name == "acme_gh_runner_default_dev" && aws_cloudwatch_log_group.runner["default"].retention_in_days == 7
    error_message = "The log group is <project>_gh_runner_<runner>_<env>, kept 7 days."
  }

  assert {
    condition = (
      aws_codebuild_project.runner["default"].environment[0].type == "LINUX_CONTAINER"
      && aws_codebuild_project.runner["default"].environment[0].compute_type == "BUILD_GENERAL1_SMALL"
      && aws_codebuild_project.runner["default"].environment[0].image == "aws/codebuild/amazonlinux-x86_64-standard:5.0"
      && aws_codebuild_project.runner["default"].environment[0].privileged_mode == false
      && aws_codebuild_project.runner["default"].environment[0].image_pull_credentials_type == "CODEBUILD"
      && aws_codebuild_project.runner["default"].build_timeout == 60
    )
    error_message = "An unset runner defaults to SMALL, the x86_64 standard:5.0 image, no Docker and 60 minutes."
  }

  assert {
    condition     = output.runs_on_labels["default"] == "codebuild-acme-gh-default-dev-$${{ github.run_id }}-$${{ github.run_attempt }}"
    error_message = "The runs-on label must be codebuild-<project name>-$${{ github.run_id }}-$${{ github.run_attempt }}, verbatim."
  }
}

run "aarch64_image_and_docker" {
  command = plan

  variables {
    runners = [
      {
        name            = "arm"
        repository      = "example-org/some-repo"
        image           = "aws/codebuild/amazonlinux-aarch64-standard:3.0"
        compute_type    = "BUILD_GENERAL1_LARGE"
        docker          = true
        timeout_minutes = 120
      },
    ]
  }

  assert {
    condition = (
      aws_codebuild_project.runner["arm"].environment[0].type == "ARM_CONTAINER"
      && aws_codebuild_project.runner["arm"].environment[0].compute_type == "BUILD_GENERAL1_LARGE"
      && aws_codebuild_project.runner["arm"].environment[0].privileged_mode == true
      && aws_codebuild_project.runner["arm"].build_timeout == 120
    )
    error_message = "An aarch64 image runs on ARM_CONTAINER; docker sets privileged_mode."
  }
}

# ---------------------------------------------------------------------------
# Repository vs organization: the source location and the webhook scope.
# ---------------------------------------------------------------------------
run "repository_and_organization_sources" {
  command = plan

  variables {
    runners = [
      { name = "repo", repository = "example-org/some-repo" },
      { name = "org", organization = "example-org" },
    ]
  }

  assert {
    condition     = aws_codebuild_project.runner["repo"].source[0].location == "https://github.com/example-org/some-repo.git"
    error_message = "A repository runner points its source at that repository's clone URL."
  }

  assert {
    condition     = aws_codebuild_project.runner["org"].source[0].location == "CODEBUILD_DEFAULT_WEBHOOK_SOURCE_LOCATION"
    error_message = "An organization runner uses CODEBUILD_DEFAULT_WEBHOOK_SOURCE_LOCATION."
  }

  assert {
    condition = alltrue([
      for name in ["repo", "org"] :
      aws_codebuild_project.runner[name].source[0].type == "GITHUB"
      && aws_codebuild_project.runner[name].source[0].buildspec == ""
      && aws_codebuild_project.runner[name].source[0].auth[0].type == "SECRETS_MANAGER"
      && aws_codebuild_project.runner[name].source[0].auth[0].resource == "arn:aws:secretsmanager:us-east-1:000000000000:secret:meroku/github-runners/acme-dev-AbCdEf"
    ])
    error_message = "Every runner authenticates with the project-level token secret."
  }

  assert {
    condition     = length(aws_codebuild_webhook.runner["repo"].scope_configuration) == 0
    error_message = "A repository webhook has no organization scope."
  }

  assert {
    condition = (
      length(aws_codebuild_webhook.runner["org"].scope_configuration) == 1
      && aws_codebuild_webhook.runner["org"].scope_configuration[0].name == "example-org"
      && aws_codebuild_webhook.runner["org"].scope_configuration[0].scope == "GITHUB_ORGANIZATION"
    )
    error_message = "An organization webhook is scoped to GITHUB_ORGANIZATION with the organization's name."
  }

  assert {
    condition = alltrue([
      for name in ["repo", "org"] :
      aws_codebuild_webhook.runner[name].build_type == "BUILD"
      && length(aws_codebuild_webhook.runner[name].filter_group) == 1
      && [for f in tolist(aws_codebuild_webhook.runner[name].filter_group)[0].filter : "${f.type}=${f.pattern}"] == ["EVENT=WORKFLOW_JOB_QUEUED"]
    ])
    error_message = "Every webhook fires only on WORKFLOW_JOB_QUEUED."
  }
}

# ---------------------------------------------------------------------------
# vpc_config exists only with vpc_access.
# ---------------------------------------------------------------------------
run "no_vpc_config_without_vpc_access" {
  command = plan

  assert {
    condition     = length(aws_codebuild_project.runner["default"].vpc_config) == 0 && length(aws_security_group.runner) == 0
    error_message = "Without vpc_access there is no vpc_config and no security group."
  }

  assert {
    condition     = !strcontains(aws_iam_role_policy.runner.policy, "ec2:")
    error_message = "Without vpc_access the role gets no EC2 permissions."
  }
}

run "vpc_config_with_vpc_access" {
  command = plan

  variables {
    vpc_access = true
    vpc_id     = "vpc-00000000000000000"
    subnet_ids = ["subnet-00000000000000001", "subnet-00000000000000002"]
  }

  assert {
    condition = (
      length(aws_codebuild_project.runner["default"].vpc_config) == 1
      && aws_codebuild_project.runner["default"].vpc_config[0].vpc_id == "vpc-00000000000000000"
      && toset(aws_codebuild_project.runner["default"].vpc_config[0].subnets) == toset(["subnet-00000000000000001", "subnet-00000000000000002"])
    )
    error_message = "With vpc_access the project runs in the given private subnets."
  }

  # `ingress` is computed when the block is absent, so it is unknown at plan.
  # The module declares no ingress block and no aws_security_group_rule.
  assert {
    condition = (
      length(aws_security_group.runner) == 1
      && aws_security_group.runner[0].vpc_id == "vpc-00000000000000000"
      && [for e in aws_security_group.runner[0].egress : "${e.protocol}:${join(",", e.cidr_blocks)}"] == ["-1:0.0.0.0/0"]
    )
    error_message = "The runners' security group allows all egress."
  }

  assert {
    condition = jsondecode(aws_iam_role_policy.runner.policy).Statement[
      index(jsondecode(aws_iam_role_policy.runner.policy).Statement[*].Sid, "AttachBuildNetworkInterfaces")
      ].Condition.StringEquals == {
      "ec2:Subnet" = [
        "arn:aws:ec2:us-east-1:000000000000:subnet/subnet-00000000000000001",
        "arn:aws:ec2:us-east-1:000000000000:subnet/subnet-00000000000000002",
      ]
      "ec2:AuthorizedService" = "codebuild.amazonaws.com"
    }
    error_message = "CreateNetworkInterfacePermission is limited to the runners' subnets and to CodeBuild."
  }
}

# ---------------------------------------------------------------------------
# IAM: exactly what the builds need, scoped to this module's resources.
# ---------------------------------------------------------------------------
run "iam_policy_is_scoped" {
  command = plan

  variables {
    runners = [
      {
        name       = "default"
        repository = "example-org/some-repo"
        secrets = [
          { name = "DB_PASSWORD", ssm = "/acme/dev/db_password" },
          { name = "API_KEY", secrets_manager = "acme/dev/api:key" },
          { name = "FULL_ARN", secrets_manager = "arn:aws:secretsmanager:us-east-1:000000000000:secret:shared/token-XyZ123:token" },
        ]
      },
      { name = "org", organization = "example-org" },
    ]
  }

  assert {
    condition = {
      for st in jsondecode(aws_iam_role_policy.runner.policy).Statement :
      st.Sid => { actions = st.Action, resources = toset(st.Resource) }
      } == {
      WriteRunnerLogs = {
        actions = ["logs:CreateLogStream", "logs:PutLogEvents"]
        resources = toset([
          "arn:aws:logs:us-east-1:000000000000:log-group:acme_gh_runner_default_dev",
          "arn:aws:logs:us-east-1:000000000000:log-group:acme_gh_runner_default_dev:*",
          "arn:aws:logs:us-east-1:000000000000:log-group:acme_gh_runner_org_dev",
          "arn:aws:logs:us-east-1:000000000000:log-group:acme_gh_runner_org_dev:*",
        ])
      }
      ReadSecrets = {
        actions = ["secretsmanager:GetSecretValue"]
        resources = toset([
          "arn:aws:secretsmanager:us-east-1:000000000000:secret:meroku/github-runners/acme-dev-AbCdEf",
          "arn:aws:secretsmanager:us-east-1:000000000000:secret:acme/dev/api-??????",
          "arn:aws:secretsmanager:us-east-1:000000000000:secret:shared/token-XyZ123",
          "arn:aws:secretsmanager:us-east-1:000000000000:secret:shared/token-XyZ123-??????",
        ])
      }
      ReadParameters = {
        actions   = ["ssm:GetParameters"]
        resources = toset(["arn:aws:ssm:us-east-1:000000000000:parameter/acme/dev/db_password"])
      }
    }
    error_message = "The policy must be exactly: logs on the runner log groups, the token and referenced secrets, the referenced parameters."
  }

  assert {
    condition     = alltrue([for st in jsondecode(aws_iam_role_policy.runner.policy).Statement : st.Effect == "Allow"])
    error_message = "Every statement is an Allow."
  }

  assert {
    condition = (
      jsondecode(aws_iam_role.runner.assume_role_policy).Statement[0].Principal.Service == "codebuild.amazonaws.com"
      && jsondecode(aws_iam_role.runner.assume_role_policy).Statement[0].Condition.StringEquals["aws:SourceAccount"] == "000000000000"
    )
    error_message = "Only CodeBuild acting for this account may assume the role."
  }

  assert {
    condition     = aws_iam_role.runner.name == "acme_gh_runners_dev"
    error_message = "The role name comes from modules/naming: <project>_gh_runners_<env>."
  }
}

run "no_parameter_statement_without_ssm_secrets" {
  command = plan

  variables {
    runners = [{ name = "default", repository = "example-org/some-repo" }]
  }

  assert {
    condition     = !contains(jsondecode(aws_iam_role_policy.runner.policy).Statement[*].Sid, "ReadParameters")
    error_message = "With no ssm secrets there is no ssm statement, rather than one with an empty Resource IAM would reject."
  }

  assert {
    condition     = toset(jsondecode(aws_iam_role_policy.runner.policy).Statement[1].Resource) == toset(["arn:aws:secretsmanager:us-east-1:000000000000:secret:meroku/github-runners/acme-dev-AbCdEf"])
    error_message = "With no secrets_manager secrets the role reads the token secret only."
  }
}

# ---------------------------------------------------------------------------
# Variable validations refuse a malformed runner at plan.
# ---------------------------------------------------------------------------
run "refuses_repository_and_organization_together" {
  command = plan

  variables {
    runners = [{ name = "both", repository = "example-org/some-repo", organization = "example-org" }]
  }

  expect_failures = [var.runners]
}

run "refuses_a_secret_with_both_sources" {
  command = plan

  variables {
    runners = [{
      name       = "default"
      repository = "example-org/some-repo"
      secrets    = [{ name = "X", ssm = "/a", secrets_manager = "b" }]
    }]
  }

  expect_failures = [var.runners]
}

run "refuses_vpc_access_without_subnets" {
  command = plan

  variables {
    vpc_access = true
    vpc_id     = "vpc-00000000000000000"
  }

  expect_failures = [var.subnet_ids]
}
