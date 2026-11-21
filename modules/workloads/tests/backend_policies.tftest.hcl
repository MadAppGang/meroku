# backend_policy merged with a custom/pre module's backend_policies output.
#
# env/main.hbs renders, when custom/pre declares the output:
#
#   backend_policy = concat(<workload.policy>, module.custom_pre.backend_policies)
#
# A custom/pre statement's resources often name something created in the same
# apply, so they are unknown at plan. The custom policy's count is
# length(var.backend_policy), which stays known as long as the list reaches the
# module without passing through try(): try() makes a partly unknown value
# wholly unknown, and the plan stops with "Invalid count argument".
#
# A run's variables cannot carry an unknown value, so the helper module in
# tests/unknown_at_plan makes one and calls this module the way env/main.hbs
# does. The test is that the plan succeeds. The helper's custom/pre stand-in
# outputs {actions, resources} only, while workload.policy carries effect and
# conditions, so the run also checks that the two shapes merge into the
# variable's type.
#
# All runs PLAN against mocks; no AWS is reached. Account 000000000000 is
# synthetic; this repository is public.
#
# Run: terraform test -filter=tests/backend_policies.tftest.hcl  (from modules/workloads)

mock_provider "aws" {
  mock_data "aws_iam_policy_document" {
    defaults = { json = "{\"Version\":\"2012-10-17\",\"Statement\":[]}" }
  }
  mock_data "aws_caller_identity" {
    defaults = { account_id = "000000000000" }
  }
  mock_data "aws_region" {
    defaults = { region = "us-east-1", name = "us-east-1" }
  }
  mock_data "aws_organizations_organization" {
    defaults = { id = "o-000000000000" }
  }
  mock_data "aws_ssm_parameters_by_path" {
    defaults = { names = [] }
  }
  mock_data "aws_vpc" {
    defaults = { cidr_block = "10.0.0.0/16" }
  }
  mock_resource "aws_iam_role" {
    defaults = { arn = "arn:aws:iam::000000000000:role/mock" }
  }
  mock_resource "aws_iam_policy" {
    defaults = { arn = "arn:aws:iam::000000000000:policy/mock" }
  }
  mock_resource "aws_ecs_task_definition" {
    defaults = {
      arn                  = "arn:aws:ecs:us-east-1:000000000000:task-definition/mock:1"
      arn_without_revision = "arn:aws:ecs:us-east-1:000000000000:task-definition/mock"
    }
  }
  mock_resource "aws_cloudwatch_log_group" {
    defaults = { arn = "arn:aws:logs:us-east-1:000000000000:log-group:mock" }
  }
  mock_resource "aws_service_discovery_service" {
    defaults = { arn = "arn:aws:servicediscovery:us-east-1:000000000000:service/srv-mock" }
  }
  mock_resource "aws_lambda_function" {
    defaults = { arn = "arn:aws:lambda:us-east-1:000000000000:function:mock" }
  }
  mock_resource "aws_cloudwatch_event_rule" {
    defaults = { arn = "arn:aws:events:us-east-1:000000000000:rule/mock" }
  }
}

mock_provider "null" {}
mock_provider "random" {}

run "custom_pre_statements_unknown_at_plan_keep_the_policy_count_known" {
  command = plan

  module {
    source = "./tests/unknown_at_plan"
  }

  providers = {
    aws    = aws
    null   = null
    random = random
  }

  override_resource {
    target = module.workloads.null_resource.build_ci_lambda
  }
}

# ---------------------------------------------------------------------------
# The variable's own validation covers every statement, including those from
# a custom/pre output, which the Go validator never sees.
# ---------------------------------------------------------------------------
variables {
  project          = "acme"
  env              = "dev"
  vpc_id           = "vpc-00000000000000000"
  subnet_ids       = ["subnet-00000000000000001", "subnet-00000000000000002"]
  private_dns_name = "acme.internal"
  api_domain       = "api.example.com"
  github_subjects  = ["repo:acme/acme:ref:refs/heads/main"]
}

run "statement_with_empty_actions_is_refused" {
  command = plan

  variables {
    backend_policy = [
      { actions = ["s3:GetObject"], resources = ["arn:aws:s3:::example-bucket/*"] },
      { actions = [], resources = ["arn:aws:kms:us-east-1:000000000000:key/example"] },
    ]
  }

  # This file only plans, and a plan never runs provisioners, so the local Go
  # build in lambda.tf's ci_lambda_build_command never runs here.
  override_resource {
    target = null_resource.build_ci_lambda
  }

  expect_failures = [var.backend_policy]
}

run "statement_with_empty_resources_is_refused" {
  command = plan

  variables {
    backend_policy = [
      { actions = ["kms:Decrypt"], resources = [] },
    ]
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  expect_failures = [var.backend_policy]
}

run "repeated_condition_test_and_variable_is_refused" {
  command = plan

  variables {
    backend_policy = [
      {
        actions   = ["s3:GetObject"]
        resources = ["*"]
        conditions = [
          { test = "StringEquals", variable = "aws:RequestedRegion", values = ["us-east-1"] },
          { test = "StringEquals", variable = "aws:RequestedRegion", values = ["us-west-2"] },
        ]
      },
    ]
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  expect_failures = [var.backend_policy]
}

run "distinct_conditions_are_accepted" {
  command = plan

  variables {
    backend_policy = [
      {
        actions   = ["s3:GetObject"]
        resources = ["*"]
        conditions = [
          { test = "StringEquals", variable = "aws:RequestedRegion", values = ["us-east-1"] },
          { test = "Bool", variable = "aws:SecureTransport", values = ["true"] },
        ]
      },
    ]
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition     = length(aws_iam_policy.backend_custom_policy) == 1
    error_message = "A statement with distinct conditions must still create the custom policy."
  }
}
