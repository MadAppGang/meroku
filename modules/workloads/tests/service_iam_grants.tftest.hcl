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
#   2. Env files. ECS reads them while it assembles the container definition,
#      so s3:GetObject belongs on the execution role — and on that service's
#      execution role only, for that service's own files.
#      (a_service_reads_its_own_env_files_and_nothing_elses)
#      (a_service_with_no_env_files_of_its_own_gets_no_env_file_grant)
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

  # The backend's own env files, which are a separate grant on a separate pair
  # of roles (env.tf). They are set here so that a service grant which reaches
  # them is visible as reaching them.
  env_files_s3 = [
    { bucket = "acme-backend-dev", key = "env/backend.env" },
  ]

  # Three services, and every combination the env-file grant has to get right:
  # two with files of their own — different files, so a mix-up between them
  # shows — and one with none at all.
  services = [
    {
      name = "orders"
      env_files_s3 = [
        { bucket = "acme-backend-dev", key = "env/orders.env" },
        { bucket = "acme-backend-dev", key = "env/orders-shared.env" },
      ]
    },
    {
      name = "billing"
      env_files_s3 = [
        { bucket = "acme-backend-dev", key = "env/billing.env" },
      ]
    },
    { name = "search" },
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

# ---------------------------------------------------------------------------
# 2. Env files: this service's own, read-only, on the execution role.
#
# ECS fetches --env-file while it assembles the container definition, before
# anything of the service's own runs, so this is the one grant of the two that
# genuinely belongs on the execution role. What it does not belong on is every
# other service's files, or an action beyond reading one.
#
# The policy document is decoded rather than matched as a string: jsonencode's
# key order is an implementation detail, and an assertion over it would pass or
# fail on formatting while saying nothing about the grant.
# ---------------------------------------------------------------------------

run "a_service_reads_its_own_env_files_and_nothing_elses" {
  command = plan

  assert {
    condition     = [for a in jsondecode(aws_iam_role_policy.services_s3_env["orders"].policy).Statement[0].Action : a] == ["s3:GetObject", "s3:GetBucketLocation"]
    error_message = "A service's env-file grant must be exactly s3:GetObject and s3:GetBucketLocation — reading the one object ECS fetches, and the bucket-region lookup the SDK needs to do it. s3:* on those ARNs also grants PutObject and DeleteObject, so one service can rewrite the configuration of another, and these files are where meroku's own env and secrets wiring is set up. Got ${jsonencode(jsondecode(aws_iam_role_policy.services_s3_env["orders"].policy).Statement[0].Action)}."
  }

  assert {
    condition     = [for a in jsondecode(aws_iam_role_policy.services_s3_env["billing"].policy).Statement[0].Action : a] == ["s3:GetObject", "s3:GetBucketLocation"]
    error_message = "Same rule for the second service: exactly s3:GetObject and s3:GetBucketLocation. Got ${jsonencode(jsondecode(aws_iam_role_policy.services_s3_env["billing"].policy).Statement[0].Action)}."
  }

  # Both of orders' files and neither of anybody else's. billing's file and the
  # backend's are in the same bucket, so a grant built from the wrong list is
  # not caught by the bucket name — only by the key.
  assert {
    condition = [for r in jsondecode(aws_iam_role_policy.services_s3_env["orders"].policy).Statement[0].Resource : r] == [
      "arn:aws:s3:::acme-backend-dev/env/orders.env",
      "arn:aws:s3:::acme-backend-dev/env/orders-shared.env",
    ]
    error_message = "\"orders\" must be granted its OWN two env files and nothing else. Every other ARN in that list is a file another workload reads: billing's configuration, or the backend's. Got ${jsonencode(jsondecode(aws_iam_role_policy.services_s3_env["orders"].policy).Statement[0].Resource)}."
  }

  assert {
    condition     = [for r in jsondecode(aws_iam_role_policy.services_s3_env["billing"].policy).Statement[0].Resource : r] == ["arn:aws:s3:::acme-backend-dev/env/billing.env"]
    error_message = "\"billing\" must be granted its own env file only. An orders ARN here means the grant was built from a flat list of every service's files instead of this service's. Got ${jsonencode(jsondecode(aws_iam_role_policy.services_s3_env["billing"].policy).Statement[0].Resource)}."
  }

  # The execution role, and this is the deliberate exception to run 1. Unlike
  # SES, nothing the container's own code does reads these files — ECS does,
  # while it builds the container definition, and it does it with this role.
  assert {
    condition     = aws_iam_role_policy.services_s3_env["orders"].role == module.naming.names["service_task_execution_role_orders"]
    error_message = "The env-file grant belongs on the EXECUTION role: ECS reads --env-file while it assembles the container definition, before any of the service's own code runs, and it does that as the execution role. On the task role the fetch is denied and the service never starts. Got ${aws_iam_role_policy.services_s3_env["orders"].role}."
  }
}

# ---------------------------------------------------------------------------
# 3. A service with no env files of its own gets no grant at all.
#
# The condition for creating this policy used to be `does ANY env file exist
# anywhere` — the backend's, or another service's — which handed every service
# in the project a policy over files it has no part in. A service that declares
# no env_files_s3 reads none, so it needs no policy, and an entry for it is
# either a leftover of that condition or a grant built from somebody else's
# list.
# ---------------------------------------------------------------------------

run "a_service_with_no_env_files_of_its_own_gets_no_env_file_grant" {
  command = plan

  assert {
    condition     = toset(keys(aws_iam_role_policy.services_s3_env)) == toset(["billing", "orders"])
    error_message = "Only services that declare env_files_s3 of their own get an env-file policy. \"search\" declares none, so an entry for it is a grant over files it never reads — and its presence is what let one service's list reach every other service. Got ${jsonencode(keys(aws_iam_role_policy.services_s3_env))}."
  }

  assert {
    condition     = length(local.services_env_files_s3["search"]) == 0
    error_message = "The fixture is wrong, not the module: \"search\" must declare no env files for this run to mean anything. Got ${jsonencode(local.services_env_files_s3["search"])}."
  }
}
