# User policy JSON and its task-role attachment.
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
}

variables {
  project          = "myapp"
  env              = "dev"
  task             = "mailer"
  vpc_id           = "vpc-00000000000000000"
  subnet_ids       = ["subnet-00000000000000001", "subnet-00000000000000002"]
  cluster          = "arn:aws:ecs:us-east-1:000000000000:cluster/myapp_cluster_dev"
  docker_image     = "example.com/myapp/task:synthetic"
  api_domain       = "api.example.com"
  private_dns_name = "myapp.internal"
  rules            = { orders = { sources = ["myapp.orders"], detail_types = ["order.created"] } }
}

run "custom_policy_preserves_effect_conditions_and_literals" {
  command = plan

  variables {
    custom_policy = [
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
  }

  assert {
    condition = (
      length(jsondecode(aws_iam_policy.custom[0].policy).Statement) == 2 &&
      jsondecode(aws_iam_policy.custom[0].policy).Statement[0].Effect == "Deny" &&
      jsonencode(jsondecode(aws_iam_policy.custom[0].policy).Statement[0].Condition) == jsonencode({
        StringEquals = {
          "aws:RequestedRegion"   = ["us-east-1"]
          "aws:PrincipalTag/team" = ["payments"]
        }
      }) &&
      toset(try(tolist(jsondecode(aws_iam_policy.custom[0].policy).Statement[0].Action), [tostring(jsondecode(aws_iam_policy.custom[0].policy).Statement[0].Action)])) == toset(["s3:DeleteObject"])
    )
    error_message = "Deny must remain Deny and two StringEquals conditions with different variables must both survive."
  }

  assert {
    condition = (
      jsondecode(aws_iam_policy.custom[0].policy).Statement[1].Effect == "Allow" &&
      !contains(keys(jsondecode(aws_iam_policy.custom[0].policy).Statement[1]), "Condition") &&
      toset(try(tolist(jsondecode(aws_iam_policy.custom[0].policy).Statement[1].Resource), [tostring(jsondecode(aws_iam_policy.custom[0].policy).Statement[1].Resource)])) ==
      toset(["arn:aws:s3:::myapp-data-dev/teams/$${aws:PrincipalTag/team}/*"])
    )
    error_message = "An unconditional statement must have no Condition key, default to Allow and retain the literal IAM principal-tag interpolation."
  }

  assert {
    condition = (
      length(aws_iam_policy.custom) == 1 &&
      length(aws_iam_role_policy_attachment.custom) == 1 &&
      aws_iam_role_policy_attachment.custom[0].role == aws_iam_role.task.name
    )
    error_message = "The customer-managed policy must attach to the task role, not the execution role."
  }
}

run "empty_policy_creates_no_custom_attachment" {
  command = plan

  variables {
    custom_policy = []
  }

  assert {
    condition     = length(aws_iam_policy.custom) == 0 && length(aws_iam_role_policy_attachment.custom) == 0
    error_message = "An empty custom_policy must not create an IAM policy or attachment."
  }
}
