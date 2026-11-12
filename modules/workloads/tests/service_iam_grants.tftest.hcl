# Which of a named service's TWO IAM roles each grant is attached to.
#
# ECS gives every service a pair, and they are not interchangeable:
#
#   aws_iam_role.services_task_execution — assumed by ECS itself: pull the
#     image, write logs, read the SSM parameters that become secrets.
#   aws_iam_role.services_task           — assumed by the container's own code,
#     through the credentials the agent injects, when it calls an AWS API.
#
# A grant on the wrong half of that pair is invisible at every gate meroku has.
# The plan renders it, the apply succeeds, the console shows the policy
# attached to a role, and the failure arrives as an AccessDenied at runtime from
# a service nobody touched. Nothing downstream can catch it either: IAM does not
# care which of the two roles holds a permission, and the container does not
# learn the difference until the call is refused. So the wiring is asserted
# here, against the names the two roles actually carry.
#
#   1. SES. The container sends the email, so ses:SendEmail has to be on the
#      task role. (the_ses_grant_lands_on_the_task_role_not_the_execution_role)
#
# Run: terraform test  (from modules/workloads)

mock_provider "aws" {
  # Identical to tests/github_policy_scope.tftest.hcl — see the long comment
  # there for why each entry exists. Briefly: a generated mock string is not a
  # policy document, a CIDR block or an ARN, and the AWS provider validates all
  # three CLIENT-side, before anything would reach a network. Account
  # 000000000000 is synthetic; this repository is public and no real identifier
  # belongs in it.
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

# The smallest input set modules/workloads accepts. Every value is synthetic.
variables {
  project = "acme"
  env     = "dev"

  vpc_id           = "vpc-00000000000000000"
  subnet_ids       = ["subnet-00000000000000001", "subnet-00000000000000002"]
  private_dns_name = "acme.internal"
  api_domain       = "api.example.com"

  # Required with no default (variables.tf): an omitted value must fail rather
  # than fall back to a wildcard trust policy. Nothing here is about GitHub.
  github_subjects = ["repo:acme/acme:ref:refs/heads/main"]

  # Two services, so a per-service mix-up between them is also visible. The SES
  # attachment is unconditional and covers both.
  services = [
    { name = "orders" },
    { name = "billing" },
  ]
}

# ---------------------------------------------------------------------------
# 1. SES goes on the task role.
#
# The comparison is against module.naming, not against the two mocked roles.
# That is what makes it capable of failing: modules/naming is md5 and string
# functions with no provider resource in it, so the mock does not reach it and
# both names are the real strings the roles will carry —
# acme_service_orders_task_dev and acme_service_orders_task_execution_dev. Two
# mocked aws_iam_role instances would have been compared against values the
# mock itself generated.
# ---------------------------------------------------------------------------

run "the_ses_grant_lands_on_the_task_role_not_the_execution_role" {
  command = plan

  assert {
    condition = alltrue([
      for k in keys(local.service_names) :
      aws_iam_role_policy_attachment.service_task_ses[k].role == module.naming.names["service_task_role_${k}"]
    ])
    error_message = "send_emails (ses:SendEmail, ses:SendRawEmail) must be attached to each service's TASK role, aws_iam_role.services_task — the one the container's code assumes when it calls SES. The execution role is ECS's own, used to pull the image, write logs and read SSM, and a service that sends email through it gets AccessDenied at runtime while the console still shows the policy attached to a role. Got ${jsonencode({ for k in keys(local.service_names) : k => aws_iam_role_policy_attachment.service_task_ses[k].role })} where the task roles are ${jsonencode({ for k in keys(local.service_names) : k => module.naming.names["service_task_role_${k}"] })}."
  }

  # The same fact stated as the defect it rules out. The two names differ only
  # by "_execution", so a change that moves the attachment back is one word in
  # services.tf and this is the assertion that names it.
  assert {
    condition = alltrue([
      for k in keys(local.service_names) :
      aws_iam_role_policy_attachment.service_task_ses[k].role != module.naming.names["service_task_execution_role_${k}"]
    ])
    error_message = "send_emails is attached to a service's EXECUTION role, which ECS assumes and the container never does, so the grant reaches nothing that sends email. Got ${jsonencode({ for k in keys(local.service_names) : k => aws_iam_role_policy_attachment.service_task_ses[k].role })}."
  }
}
