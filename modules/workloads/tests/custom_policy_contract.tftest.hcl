# Backend and service policy semantics without document churn.
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
  mock_data "aws_vpc" {
    defaults = { cidr_block = "10.0.0.0/16" }
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

# Suppress the CI Lambda build provisioner; no shell build belongs in a module test.
override_resource {
  target = null_resource.build_ci_lambda
}

variables {
  project          = "myapp"
  env              = "dev"
  vpc_id           = "vpc-00000000000000000"
  subnet_ids       = ["subnet-00000000000000001", "subnet-00000000000000002"]
  private_dns_name = "myapp.internal"
  api_domain       = "api.example.com"
  github_subjects  = ["repo:myapp/app:ref:refs/heads/main"]
  docker_image     = "example.com/myapp/backend:synthetic"
  services = [
    { name = "api", docker_image = "example.com/myapp/api:synthetic", env_vars = {} },
    { name = "worker", docker_image = "example.com/myapp/worker:synthetic", env_vars = {} }
  ]
}

run "backend_and_service_preserve_effect_conditions_and_literals" {
  command = plan

  variables {
    backend_policy = [
      {
        effect    = "Deny"
        actions   = ["s3:DeleteObject"]
        resources = ["arn:aws:s3:::myapp-data-dev/*"]
        conditions = [
          { test = "StringEquals", variable = "aws:RequestedRegion", values = ["us-east-1"] },
          { test = "StringEquals", variable = "aws:PrincipalTag/team", values = ["payments"] }
        ]
      },
      {
        actions   = ["s3:PutObject"]
        resources = ["arn:aws:s3:::myapp-data-dev/teams/$${aws:PrincipalTag/team}/*"]
      }
    ]
    service_policies = { api = [
      {
        effect    = "Deny"
        actions   = ["s3:DeleteObject"]
        resources = ["arn:aws:s3:::myapp-data-dev/*"]
        conditions = [
          { test = "StringEquals", variable = "aws:RequestedRegion", values = ["us-east-1"] },
          { test = "StringEquals", variable = "aws:PrincipalTag/team", values = ["payments"] }
        ]
      },
      {
        actions   = ["s3:PutObject"]
        resources = ["arn:aws:s3:::myapp-data-dev/teams/$${aws:PrincipalTag/team}/*"]
      }
    ] }
  }

  assert {
    condition = (
      length(jsondecode(aws_iam_policy.backend_custom_policy[0].policy).Statement) == 2 &&
      jsondecode(aws_iam_policy.backend_custom_policy[0].policy).Statement[0].Effect == "Deny" &&
      jsonencode(jsondecode(aws_iam_policy.backend_custom_policy[0].policy).Statement[0].Condition) == jsonencode({
        StringEquals = {
          "aws:RequestedRegion"   = ["us-east-1"]
          "aws:PrincipalTag/team" = ["payments"]
        }
      }) &&
      toset(try(tolist(jsondecode(aws_iam_policy.backend_custom_policy[0].policy).Statement[0].Action), [tostring(jsondecode(aws_iam_policy.backend_custom_policy[0].policy).Statement[0].Action)])) == toset(["s3:DeleteObject"])
    )
    error_message = "Deny must remain Deny and two StringEquals conditions with different variables must both survive."
  }

  assert {
    condition = (
      jsondecode(aws_iam_policy.backend_custom_policy[0].policy).Statement[1].Effect == "Allow" &&
      !contains(keys(jsondecode(aws_iam_policy.backend_custom_policy[0].policy).Statement[1]), "Condition") &&
      toset(try(tolist(jsondecode(aws_iam_policy.backend_custom_policy[0].policy).Statement[1].Resource), [tostring(jsondecode(aws_iam_policy.backend_custom_policy[0].policy).Statement[1].Resource)])) ==
      toset(["arn:aws:s3:::myapp-data-dev/teams/$${aws:PrincipalTag/team}/*"])
    )
    error_message = "An unconditional statement must have no Condition key, default to Allow and retain the literal IAM principal-tag interpolation."
  }

  assert {
    condition = (
      length(jsondecode(aws_iam_policy.services_custom["api"].policy).Statement) == 2 &&
      jsondecode(aws_iam_policy.services_custom["api"].policy).Statement[0].Effect == "Deny" &&
      jsonencode(jsondecode(aws_iam_policy.services_custom["api"].policy).Statement[0].Condition) == jsonencode({
        StringEquals = {
          "aws:RequestedRegion"   = ["us-east-1"]
          "aws:PrincipalTag/team" = ["payments"]
        }
      }) &&
      toset(try(tolist(jsondecode(aws_iam_policy.services_custom["api"].policy).Statement[0].Action), [tostring(jsondecode(aws_iam_policy.services_custom["api"].policy).Statement[0].Action)])) == toset(["s3:DeleteObject"])
    )
    error_message = "Deny must remain Deny and two StringEquals conditions with different variables must both survive."
  }

  assert {
    condition = (
      jsondecode(aws_iam_policy.services_custom["api"].policy).Statement[1].Effect == "Allow" &&
      !contains(keys(jsondecode(aws_iam_policy.services_custom["api"].policy).Statement[1]), "Condition") &&
      toset(try(tolist(jsondecode(aws_iam_policy.services_custom["api"].policy).Statement[1].Resource), [tostring(jsondecode(aws_iam_policy.services_custom["api"].policy).Statement[1].Resource)])) ==
      toset(["arn:aws:s3:::myapp-data-dev/teams/$${aws:PrincipalTag/team}/*"])
    )
    error_message = "An unconditional statement must have no Condition key, default to Allow and retain the literal IAM principal-tag interpolation."
  }

  assert {
    condition = (
      toset(keys(aws_iam_policy.services_custom)) == toset(["api"]) &&
      toset(keys(aws_iam_role_policy_attachment.services_custom)) == toset(["api"]) &&
      aws_iam_role_policy_attachment.services_custom["api"].role == aws_iam_role.services_task["api"].name &&
      aws_iam_role_policy_attachment.backend_custom_policy_attachment[0].role == aws_iam_role.backend_task.name
    )
    error_message = "Managed user policies attach only to their task roles; worker has no policy and must get no policy or attachment."
  }
}

run "allow_only_backend_policy_keeps_its_original_document" {
  command = plan

  variables {
    backend_policy = [{
      actions   = ["dynamodb:GetItem", "dynamodb:PutItem"]
      resources = ["arn:aws:dynamodb:us-east-1:000000000000:table/example"]
    }]
  }

  assert {
    condition = aws_iam_policy.backend_custom_policy[0].policy == jsonencode({
      Version = "2012-10-17"
      Statement = [{
        Effect   = "Allow"
        Action   = ["dynamodb:GetItem", "dynamodb:PutItem"]
        Resource = ["arn:aws:dynamodb:us-east-1:000000000000:table/example"]
      }]
    })
    error_message = "A legacy actions/resources-only backend policy must keep its compact JSON shape, with Allow and no Condition key."
  }

  assert {
    condition = (
      length(aws_iam_policy.backend_custom_policy) == 1 &&
      length(aws_iam_role_policy_attachment.backend_custom_policy_attachment) == 1 &&
      aws_iam_role_policy_attachment.backend_custom_policy_attachment[0].role == aws_iam_role.backend_task.name
    )
    error_message = "The backend must retain its existing managed-policy attachment on the task role."
  }
}
