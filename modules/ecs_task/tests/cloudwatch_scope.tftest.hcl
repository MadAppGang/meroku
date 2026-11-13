# What a scheduled task's TASK role may do in CloudWatch (iam.tf,
# aws_iam_role_policy.task_cloudwatch).
#
# The role used to carry the AWS-managed CloudWatchFullAccess, which also
# grants events:* and sns:*. IAM allows add up, so with it attached the
# per-entry PutEvents grant of modules/events narrowed nothing. The replacement
# grants PutMetricData and writes to the task's own log group, and nothing else.
#
# Runs APPLY against the mocks: the log group's ARN is unknown during a plan. No
# AWS is reached. Account 000000000000 is synthetic.
#
# Run: terraform test  (from modules/ecs_task)

mock_provider "aws" {
  mock_data "aws_iam_policy_document" {
    defaults = {
      json = "{\"Version\":\"2012-10-17\",\"Statement\":[]}"
    }
  }

  mock_data "aws_organizations_organization" {
    defaults = {
      id = "o-000000000000"
    }
  }

  mock_data "aws_caller_identity" {
    defaults = {
      account_id = "000000000000"
    }
  }

  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::000000000000:role/mock"
    }
  }

  mock_resource "aws_iam_policy" {
    defaults = {
      arn = "arn:aws:iam::000000000000:policy/mock"
    }
  }

  mock_resource "aws_cloudwatch_log_group" {
    defaults = {
      arn = "arn:aws:logs:us-east-1:000000000000:log-group:acme_task_cleanup_dev"
    }
  }

  mock_resource "aws_ecs_task_definition" {
    defaults = {
      arn                  = "arn:aws:ecs:us-east-1:000000000000:task-definition/mock:1"
      arn_without_revision = "arn:aws:ecs:us-east-1:000000000000:task-definition/mock"
    }
  }
}

variables {
  project    = "acme"
  env        = "dev"
  task       = "cleanup"
  vpc_id     = "vpc-00000000000000000"
  subnet_ids = ["subnet-00000000000000001", "subnet-00000000000000002"]
  cluster    = "arn:aws:ecs:us-east-1:000000000000:cluster/acme_cluster_dev"

  docker_image = "public.ecr.aws/nginx/nginx:stable"

  api_domain       = "api.example.com"
  private_dns_name = "acme_dev.private"
}

run "the_task_role_gets_metrics_and_its_own_logs_and_no_events" {
  command = apply

  override_data {
    target = data.aws_ssm_parameters_by_path.task
    values = {
      names = ["/dev/acme/task/cleanup/env"]
    }
  }

  assert {
    condition     = aws_iam_role_policy.task_cloudwatch.role == aws_iam_role.task.name
    error_message = "The CloudWatch policy belongs on the task role, the one the container assumes."
  }

  assert {
    condition = sort(flatten([
      for s in jsondecode(aws_iam_role_policy.task_cloudwatch.policy).Statement : s.Action
      ])) == sort([
      "cloudwatch:PutMetricData",
      "logs:CreateLogStream",
      "logs:DescribeLogStreams",
      "logs:PutLogEvents",
    ])
    error_message = "The task role's CloudWatch policy grants exactly PutMetricData and writes to its log group; no events:* or sns:* action. Got ${aws_iam_role_policy.task_cloudwatch.policy}."
  }

  assert {
    condition = [
      for s in jsondecode(aws_iam_role_policy.task_cloudwatch.policy).Statement : s.Resource if s.Sid == "OwnLogs"
      ] == [[
        "arn:aws:logs:us-east-1:000000000000:log-group:acme_task_cleanup_dev",
        "arn:aws:logs:us-east-1:000000000000:log-group:acme_task_cleanup_dev:*",
    ]]
    error_message = "Log writes are limited to the task's own log group."
  }
}
