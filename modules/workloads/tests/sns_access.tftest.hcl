# What the backend's task role may do in SNS (sns.tf).
#
# The task role used to carry the AWS-managed CloudWatchFullAccess, whose sns:*
# on every resource was the only grant two meroku features had:
#
#   1. FCM push (setup_FCM_SNS). SNS checks creating a device endpoint, and
#      publishing to, reading, updating or deleting one, against the platform
#      application ARN, arn:aws:sns:<region>:<account>:app/GCM/<app>. A grant
#      on the endpoint ARNs (endpoint/GCM/<app>/*) authorises none of them.
#      The policy names that application and nothing else.
#      (fcm_policy_grants_on_the_platform_application)
#   2. extensions.sns_topics with add_to_backend_env. The backend gets the
#      topic's ARN in its environment, so it gets sns:Publish on exactly those
#      topics, and no SNS policy at all when there are none.
#      (backend_publishes_to_the_topics_it_is_given, no_topics_no_sns_policy)
#
# Runs APPLY against the mocks. No AWS is reached.
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
  mock_resource "aws_sns_platform_application" {
    defaults = {
      arn = "arn:aws:sns:us-east-1:000000000000:app/GCM/acme-fcm-dev"
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
}

run "fcm_policy_grants_on_the_platform_application" {
  command = apply

  variables {
    setup_FCM_SNS = true
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition = length([
      for s in jsondecode(aws_iam_policy.backend_fcm_policies[0].policy).Statement : s
      if s.Effect == "Allow" && s.Resource == "arn:aws:sns:us-east-1:000000000000:app/GCM/acme-fcm-dev" && toset(s.Action) == toset([
        "sns:CreatePlatformEndpoint",
        "sns:Publish",
        "sns:GetEndpointAttributes",
        "sns:SetEndpointAttributes",
        "sns:DeleteEndpoint",
      ])
    ]) == 1
    error_message = "SNS authorises creating, publishing to and managing device endpoints against the platform application ARN, app/GCM/<app>."
  }

  assert {
    condition = alltrue([
      for s in jsondecode(aws_iam_policy.backend_fcm_policies[0].policy).Statement :
      alltrue([for r in flatten([s.Resource]) : !strcontains(r, ":endpoint/")])
    ])
    error_message = "A grant on endpoint ARNs authorises none of the FCM actions; the policy names the application ARN only."
  }

  assert {
    condition = alltrue([
      for s in jsondecode(aws_iam_policy.backend_fcm_policies[0].policy).Statement :
      alltrue([for r in flatten([s.Resource]) : r != "*" && !endswith(r, ":*")])
    ])
    error_message = "The FCM policy must not widen to every SNS resource."
  }
}

run "backend_publishes_to_the_topics_it_is_given" {
  command = apply

  variables {
    backend_sns_publish_topic_arns = [
      "arn:aws:sns:us-east-1:000000000000:acme-dev-orders",
      "arn:aws:sns:us-east-1:000000000000:acme-dev-audit.fifo",
    ]
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition     = length(aws_iam_role_policy.backend_task_sns_publish) == 1
    error_message = "A topic whose ARN the backend receives needs a publish grant on the backend's task role."
  }

  assert {
    condition     = aws_iam_role_policy.backend_task_sns_publish[0].role == module.naming.names["backend_task_role"]
    error_message = "The SNS publish policy belongs on the backend's task role."
  }

  assert {
    condition = jsonencode(jsondecode(aws_iam_role_policy.backend_task_sns_publish[0].policy).Statement) == jsonencode([{
      Sid    = "PublishExtensionTopics"
      Effect = "Allow"
      Action = ["sns:Publish"]
      Resource = [
        "arn:aws:sns:us-east-1:000000000000:acme-dev-orders",
        "arn:aws:sns:us-east-1:000000000000:acme-dev-audit.fifo",
      ]
    }])
    error_message = "The backend may publish to exactly the topics it is given, and do nothing else in SNS."
  }
}

run "no_topics_no_sns_policy" {
  command = apply

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition     = length(aws_iam_role_policy.backend_task_sns_publish) == 0
    error_message = "Without topics in the backend's environment there is nothing to publish to."
  }
}
