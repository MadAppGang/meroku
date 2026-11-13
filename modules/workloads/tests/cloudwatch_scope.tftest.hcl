# What the backend's, each service's and pgAdmin's TASK role may do in
# CloudWatch (cloudwatch_access.tf).
#
# These roles used to carry the AWS-managed CloudWatchFullAccess, which also
# grants events:* and sns:*. IAM allows add up, so with it attached the strict,
# per-entry PutEvents grant of modules/events narrowed nothing: any workload
# could publish anything to any bus. The replacement is an inline policy with
# metrics and the role's own log group, and nothing from events or SNS.
#
#   1. No events:* or sns:* action on any of these task roles' CloudWatch
#      policies, and PutMetricData plus the own log group on each.
#      (task_roles_get_metrics_and_their_own_logs_and_no_events)
#   2. X-Ray: the backend's ADOT collector gets the Container Insights log
#      groups, a service with the X-Ray daemon gets the X-Ray write actions,
#      and neither appears when X-Ray is off.
#      (xray_adds_only_what_the_collector_and_daemon_use)
#
# The modules' source is also checked statically by
# app/iam_scope_test.go: no CloudWatchFullAccess on a task role anywhere, and
# events:PutEvents only where a role that is not a workload's needs it.
#
# Runs APPLY against the mocks: a log group's ARN is unknown during a plan. No
# AWS is reached; the apply and its teardown are mocked.
#
# Run: terraform test  (from modules/workloads)

mock_provider "aws" {
  # As tests/service_iam_grants.tftest.hcl: a generated mock string is not a
  # policy document, a CIDR block or an ARN, and the provider validates all
  # three client-side. Account 000000000000 is synthetic.
  mock_data "aws_iam_policy_document" {
    defaults = {
      json = "{\"Version\":\"2012-10-17\",\"Statement\":[]}"
    }
  }

  mock_data "aws_vpc" {
    defaults = {
      cidr_block = "10.0.0.0/16"
    }
  }

  mock_data "aws_region" {
    defaults = {
      region = "us-east-1"
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
      arn = "arn:aws:logs:us-east-1:000000000000:log-group:mock"
    }
  }
  mock_resource "aws_service_discovery_service" {
    defaults = {
      arn = "arn:aws:servicediscovery:us-east-1:000000000000:service/srv-mock"
    }
  }
  mock_resource "aws_lambda_function" {
    defaults = {
      arn = "arn:aws:lambda:us-east-1:000000000000:function:mock"
    }
  }
  mock_resource "aws_cloudwatch_event_rule" {
    defaults = {
      arn = "arn:aws:events:us-east-1:000000000000:rule/mock"
    }
  }
}

mock_provider "null" {}
mock_provider "random" {}

variables {
  project          = "acme"
  env              = "dev"
  vpc_id           = "vpc-00000000000000000"
  subnet_ids       = ["subnet-00000000000000001", "subnet-00000000000000002"]
  private_dns_name = "acme.internal"
  api_domain       = "api.example.com"
  github_subjects  = ["repo:acme/acme:ref:refs/heads/main"]

  pgadmin_enabled = true

  services = [
    { name = "orders" },
    { name = "traced", xray_enabled = true },
  ]
}

run "task_roles_get_metrics_and_their_own_logs_and_no_events" {
  command = apply

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition = length([
      for a in flatten(concat(
        [for s in jsondecode(aws_iam_role_policy.backend_task_cloudwatch.policy).Statement : s.Action],
        [for p in values(aws_iam_role_policy.services_task_cloudwatch) : [for s in jsondecode(p.policy).Statement : s.Action]],
        [for p in aws_iam_role_policy.pgadmin_task_cloudwatch : [for s in jsondecode(p.policy).Statement : s.Action]],
      )) : a if startswith(a, "events:") || startswith(a, "sns:") || a == "*" || endswith(a, ":*")
    ]) == 0
    error_message = "A task role's CloudWatch policy grants an events, SNS or wildcard action; any of them defeats the per-entry PutEvents grant of modules/events."
  }

  assert {
    condition = alltrue([
      for p in concat(
        [aws_iam_role_policy.backend_task_cloudwatch.policy],
        [for v in values(aws_iam_role_policy.services_task_cloudwatch) : v.policy],
        [for v in aws_iam_role_policy.pgadmin_task_cloudwatch : v.policy],
        ) : length([
          for s in jsondecode(p).Statement : s
          if s.Sid == "OwnLogs" && s.Resource == [
            "arn:aws:logs:us-east-1:000000000000:log-group:mock",
            "arn:aws:logs:us-east-1:000000000000:log-group:mock:*",
          ]
          ]) == 1 && length([
          for s in jsondecode(p).Statement : s
          if s.Sid == "PutMetrics" && s.Action == ["cloudwatch:PutMetricData"]
      ]) == 1
    ])
    error_message = "Each task role's CloudWatch policy must grant PutMetricData and writes to its own log group only."
  }

  assert {
    condition     = aws_iam_role_policy.backend_task_cloudwatch.role == module.naming.names["backend_task_role"]
    error_message = "The backend's CloudWatch policy belongs on its task role."
  }

  assert {
    condition = alltrue([
      for k, p in aws_iam_role_policy.services_task_cloudwatch :
      p.role == module.naming.names["service_task_role_${k}"]
    ])
    error_message = "Each service's CloudWatch policy belongs on that service's task role."
  }
}

run "xray_adds_only_what_the_collector_and_daemon_use" {
  command = apply

  variables {
    xray_enabled = true
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition = contains(
      [for s in jsondecode(aws_iam_role_policy.backend_task_cloudwatch.policy).Statement : s.Sid],
      "ContainerInsightsLogs",
    )
    error_message = "With xray_enabled the backend's ADOT collector needs the Container Insights log groups."
  }

  assert {
    condition = contains(
      [for s in jsondecode(aws_iam_role_policy.services_task_cloudwatch["traced"].policy).Statement : s.Sid],
      "XRayWrite",
    )
    error_message = "A service with xray_enabled runs the X-Ray daemon, which sends traces as the task role."
  }

  assert {
    condition = !contains(
      [for s in jsondecode(aws_iam_role_policy.services_task_cloudwatch["orders"].policy).Statement : s.Sid],
      "XRayWrite",
    )
    error_message = "A service without xray_enabled gets no X-Ray grant."
  }
}
