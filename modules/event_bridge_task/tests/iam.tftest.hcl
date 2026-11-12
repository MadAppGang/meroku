# Which of an event processor task's TWO IAM roles each grant is attached to,
# and the one grant EventBridge needs before it will start the task at all.
#
# ECS gives this task a pair, and they are not interchangeable:
#
#   aws_iam_role.task_execution — assumed by ECS itself: pull the image, write
#     logs, resolve the container's `secrets` from SSM. AND, for an event
#     processor task only, assumed by EventBridge to launch the task:
#     aws_cloudwatch_event_target.target (main.tf) passes
#     role_arn = aws_iam_role.task_execution.arn, and nothing in this module
#     ever calls RunTask itself.
#   aws_iam_role.task           — assumed by the container's own code, through
#     the credentials the agent injects, when it calls an AWS API.
#
# A grant on the wrong half of that pair, or a grant whose resource ARN matches
# nothing, is invisible at every gate meroku has. The plan renders it, the apply
# succeeds, the console shows a policy attached to a role, and the failure
# arrives later from a task nobody touched. What the user actually sees:
#
#   * The rule exists, matches, and nothing starts. The target's failed
#     invocation carries an authorization error naming `ecs:RunTask` or
#     `iam:PassRole` on a role that is, visibly, the one the rule points at.
#     AmazonECSTaskExecutionRolePolicy — the execution role's only other grant —
#     carries neither.
#     (eventbridge_can_launch_the_task_as_the_execution_role)
#   * The task starts and dies before the container runs:
#     "ResourceInitializationError: unable to pull secrets or registry auth:
#     access denied to one or more secrets or registry images". ECS resolves
#     every entry in the container's `secrets` AS THE EXECUTION ROLE, so a
#     grant on the task role alone never reaches the read.
#     (the_ssm_policy_is_attached_to_both_roles)
#   * A ResourceInitializationError that no policy edit seems to fix, because
#     the ARN inside the grant omits the `parameter` resource-type segment an
#     SSM parameter ARN carries. The statement then matches no parameter in the
#     account, IAM still accepts it, and no error names it.
#     (the_ssm_grant_names_a_real_parameter_arn)
#
# The contract, in the order it matters. The two SSM runs come first because
# they are the half where a green apply still produces a task that dies on its
# first secret; the two launch runs follow because their failure is silent
# except in the EventBridge console.
#
#   1. The SSM statement names a real parameter ARN, scoped to this task's own
#      path and to nothing wider.
#      (the_ssm_grant_names_a_real_parameter_arn)
#   2. That one policy is attached to BOTH roles: the execution role for ECS to
#      resolve `secrets`, the task role for the container's own reads.
#      (the_ssm_policy_is_attached_to_both_roles)
#   3. The execution role can launch this task into this cluster: ecs:RunTask
#      on the task definition with an ecs:cluster condition, and iam:PassRole
#      on both roles with iam:PassedToService pinned to ECS.
#      (eventbridge_can_launch_the_task_as_the_execution_role)
#   4. It can do that and nothing else — no action beyond the two, no statement
#      beyond the two, no resource beyond this task's own task definition and
#      its two roles.
#      (the_launch_policy_grants_nothing_beyond_launching)
#
# Run 1 plans; runs 2-4 APPLY against the mocks. Computed attributes of a
# resource the mock provider is creating are unknown during a plan, so the three
# this file asserts on — both role ARNs and the task definition's
# arn_without_revision — are pinned with `override_resource`, which takes effect
# at apply. `override_during = plan` would do it at plan, but it needs Terraform
# >= 1.11 and CI pins an older one (.github/workflows/ci.yml), which rejects the
# argument and fails the whole file. Pinning also gives the two roles DIFFERENT
# ARNs: left to the mock they would share one generated value, and a PassRole
# statement covering only the task role would pass an assertion that both ARNs
# are present. No AWS is reached; the apply and its teardown are mocked.
#
# Run: terraform test  (from modules/event_bridge_task)

mock_provider "aws" {
  # The AWS provider validates a policy document, an ARN and a CIDR block
  # CLIENT-side, before anything would reach a network, and a generated mock
  # string is none of the three. Account 000000000000 is synthetic; this
  # repository is public.
  mock_data "aws_iam_policy_document" {
    defaults = {
      json = "{\"Version\":\"2012-10-17\",\"Statement\":[]}"
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

  mock_resource "aws_ecs_task_definition" {
    defaults = {
      arn = "arn:aws:ecs:us-east-1:000000000000:task-definition/mock:1"
      # The launch policy's RunTask resource is built from this attribute and
      # not from `arn`, so that the grant covers the whole family: a new
      # revision, which every deploy registers, then needs no new grant. A
      # mock's defaults do not reach a resource the plan is still creating; this
      # is the value an apply-mode run without an override sees.
      arn_without_revision = "arn:aws:ecs:us-east-1:000000000000:task-definition/mock"
    }
  }
}

variables {
  project    = "acme"
  env        = "dev"
  task       = "ingest"
  vpc_id     = "vpc-00000000000000000"
  subnet_ids = ["subnet-00000000000000001", "subnet-00000000000000002"]
  cluster    = "arn:aws:ecs:us-east-1:000000000000:cluster/acme_cluster_dev"

  # Set so the container definition does not depend on a mocked ECR repository
  # URL. Nothing here is about ECR.
  docker_image = "public.ecr.aws/nginx/nginx:stable"

  api_domain       = "api.example.com"
  private_dns_name = "acme_dev.private"

  rules = {
    "orders" = {
      sources      = ["acme.orders"]
      detail_types = ["OrderPlaced"]
    }
  }
}

# ---------------------------------------------------------------------------
# 1. The SSM statement names a real parameter ARN.
#
# An SSM parameter ARN is
# arn:aws:ssm:<region>:<account>:parameter/<path> — the `parameter` segment is
# the resource type, and without it the ARN denotes nothing SSM has. The
# statement is still syntactically valid, IAM still accepts it, the console
# still shows a policy attached to the role, and every ssm:GetParameter the
# task makes is denied.
#
# region and account are pinned here because a mock data source returns a
# generated string for both, different on every run; without the override the
# expected ARN could not be written down.
#
# This run reads the data source's CONFIGURED `statement` blocks, never its
# `.json`. `aws_iam_policy_document` is mocked in this directory, so `.json` is
# the one fixed default declared at the top of the file for every instance of it
# in the module and says nothing about iam.tf — while the mock passes a data
# source's configured arguments through untouched, which makes `statement` the
# half that holds the ARN iam.tf actually built. Comparing `.json` would look
# more authoritative and would leave this run green whatever iam.tf says.
# ---------------------------------------------------------------------------

run "the_ssm_grant_names_a_real_parameter_arn" {
  command = plan

  override_data {
    target = data.aws_ssm_parameters_by_path.task
    values = {
      names = ["/dev/acme/task/ingest/env"]
    }
  }

  override_data {
    target = data.aws_region.current
    values = {
      region = "us-east-1"
    }
  }

  override_data {
    target = data.aws_caller_identity.current
    values = {
      account_id = "000000000000"
    }
  }

  # The whole resource list, not one entry of it: this pins both the ARN's
  # shape and the fact that the grant reaches this task's own path and nothing
  # wider. `resources` is a set, so it is compared as one.
  assert {
    condition     = toset(data.aws_iam_policy_document.ssm_parameter_access.statement[0].resources) == toset(["arn:aws:ssm:us-east-1:000000000000:parameter/dev/acme/task/ingest/*"])
    error_message = "The SSM grant must name exactly one parameter ARN: arn:aws:ssm:<region>:<account>:parameter/<env>/<project>/task/<task>/*. Missing the `parameter` resource-type segment, the ARN denotes nothing, the statement matches no parameter in the account, and every secret the task reads is denied by a policy that looks correct in the console. A second entry widens the grant to another task's parameters or to the whole account. Got ${jsonencode(data.aws_iam_policy_document.ssm_parameter_access.statement[0].resources)}."
  }

  # ssm:GetParametersByPath is what env.tf's own read of the task path needs,
  # and the other two are what the container's code calls.
  assert {
    condition     = toset(data.aws_iam_policy_document.ssm_parameter_access.statement[0].actions) == toset(["ssm:GetParameter", "ssm:GetParameters", "ssm:GetParametersByPath"])
    error_message = "The three read actions and no more. Got ${jsonencode(data.aws_iam_policy_document.ssm_parameter_access.statement[0].actions)}."
  }

  # IAM policy names are account-global, so this one is scoped by project and
  # env — and it is already deployed under this exact string everywhere. A
  # renamed policy is a destroy-and-create in every environment.
  assert {
    condition     = aws_iam_policy.ssm_parameter_access.name == "TaskingestSSMAccessPolicy_acme_dev"
    error_message = "The managed policy's name must stay Task<task>SSMAccessPolicy_<project>_<env>. Got ${aws_iam_policy.ssm_parameter_access.name}."
  }
}

# ---------------------------------------------------------------------------
# 2. That one policy, on BOTH roles.
#
# The container's own code reads parameters as the task role. ECS resolves the
# container's `secrets` as the EXECUTION role, while it assembles the container
# definition and before any of the task's code runs — so a grant on the task
# role alone leaves every secret unreadable and the task stopped with
# ResourceInitializationError before it starts.
#
# The comparison is against module.naming, not against the two mocked roles.
# modules/naming is md5 and string functions with no provider resource in it, so
# the mock does not reach it and both names are the real strings the roles carry
# — acme_ingest_task_dev and acme_scheduler_ingest_task_execution_dev, which
# differ by more than one word and so cannot be mistaken for each other.
# ---------------------------------------------------------------------------

run "the_ssm_policy_is_attached_to_both_roles" {
  command = apply

  override_data {
    target = data.aws_ssm_parameters_by_path.task
    values = {
      names = ["/dev/acme/task/ingest/env"]
    }
  }

  # A managed policy's ARN is computed, so during a plan that creates it both
  # attachments' policy_arn are unknown. Pinning it is what lets this run assert
  # the two halves point at the SAME policy rather than at two different ones.
  override_resource {
    target = aws_iam_policy.ssm_parameter_access
    values = {
      arn = "arn:aws:iam::000000000000:policy/TaskingestSSMAccessPolicy_acme_dev"
    }
  }

  assert {
    condition     = aws_iam_role_policy_attachment.ssm_parameter_access.role == module.naming.names["task_role"]
    error_message = "The SSM policy stays on the task role: the container's code reads parameters with the credentials ECS injects for that role. Got ${aws_iam_role_policy_attachment.ssm_parameter_access.role} where the task role is ${module.naming.names["task_role"]}."
  }

  assert {
    condition     = aws_iam_role_policy_attachment.ssm_parameter_access_task_execution.role == module.naming.names["task_execution_role"]
    error_message = "The same SSM policy must also be attached to the EXECUTION role, which is the identity ECS uses to resolve the container's secrets. Without it the task stops with ResourceInitializationError before the container runs, and the console shows a policy attached to a role. Got ${aws_iam_role_policy_attachment.ssm_parameter_access_task_execution.role} where the execution role is ${module.naming.names["task_execution_role"]}."
  }

  assert {
    condition     = aws_iam_role_policy_attachment.ssm_parameter_access_task_execution.policy_arn == aws_iam_role_policy_attachment.ssm_parameter_access.policy_arn
    error_message = "Both attachments must carry the one policy from data.aws_iam_policy_document.ssm_parameter_access. A second, separately scoped policy on the execution role is a grant that can drift from the one the task role reads. Got ${aws_iam_role_policy_attachment.ssm_parameter_access_task_execution.policy_arn} and ${aws_iam_role_policy_attachment.ssm_parameter_access.policy_arn}."
  }
}

# ---------------------------------------------------------------------------
# 3. EventBridge can launch the task, as the role it is told to assume.
#
# main.tf hands the event target role_arn = aws_iam_role.task_execution.arn.
# That role's only other grant is the AWS managed
# AmazonECSTaskExecutionRolePolicy, which carries image pull, log writes and SSM
# reads — not ecs:RunTask, and not iam:PassRole. So without this policy the rule
# is created, matches every event, and starts nothing.
#
# The first assertion is the wiring the rest of the run depends on: if the event
# target ever pointed at a different role, a launch policy here would be granted
# to a role EventBridge never assumes.
# ---------------------------------------------------------------------------

run "eventbridge_can_launch_the_task_as_the_execution_role" {
  command = apply

  override_data {
    target = data.aws_ssm_parameters_by_path.task
    values = {
      names = ["/dev/acme/task/ingest/env"]
    }
  }

  # Distinct ARNs for the two roles. Left to the mock both would be the same
  # generated value, and a PassRole statement naming only one of them would pass
  # an assertion that both are present.
  override_resource {
    target = aws_iam_role.task
    values = {
      arn = "arn:aws:iam::000000000000:role/acme_ingest_task_dev"
    }
  }

  override_resource {
    target = aws_iam_role.task_execution
    values = {
      arn = "arn:aws:iam::000000000000:role/acme_scheduler_ingest_task_execution_dev"
    }
  }

  # The task definition's ARN WITHOUT its revision. Only this attribute is
  # pinned: a policy built from the revision-suffixed arn would leave it unknown
  # at plan and fail this run outright, which is the point — that grant stops
  # working at the next revision.
  override_resource {
    target = aws_ecs_task_definition.task
    values = {
      arn_without_revision = "arn:aws:ecs:us-east-1:000000000000:task-definition/acme_event_task_ingest_dev"
    }
  }

  assert {
    condition     = aws_cloudwatch_event_target.target["orders"].role_arn == aws_iam_role.task_execution.arn
    error_message = "The event target must name the execution role, which is the role EventBridge assumes to launch the task. Every assertion below is about that role. Got ${aws_cloudwatch_event_target.target["orders"].role_arn}."
  }

  assert {
    condition     = aws_iam_role_policy.event_task_launch.role == module.naming.names["task_execution_role"]
    error_message = "The launch policy belongs on the EXECUTION role — the one the event target names. On the task role it grants nothing to the principal that calls RunTask, and the rule keeps failing silently. Got ${aws_iam_role_policy.event_task_launch.role} where the execution role is ${module.naming.names["task_execution_role"]}."
  }

  assert {
    condition     = aws_iam_role_policy.event_task_launch.name == "EventTaskLaunch_acme_ingest_dev"
    error_message = "The inline policy's name must be EventTaskLaunch_<project>_<task>_<env>, so two tasks in one environment carry two readable policies on the same role. Got ${aws_iam_role_policy.event_task_launch.name}."
  }

  # Which statement is which, before the four assertions below read them by
  # position. Pinning the pair here means reordering them in iam.tf fails with a
  # message about the policy's shape rather than with one about a cluster ARN.
  assert {
    condition     = [for s in jsondecode(aws_iam_role_policy.event_task_launch.policy).Statement : [for a in s.Action : a]] == [["ecs:RunTask"], ["iam:PassRole"]]
    error_message = "The launch policy is two statements in this order: ecs:RunTask, then iam:PassRole. Got ${jsonencode([for s in jsondecode(aws_iam_role_policy.event_task_launch.policy).Statement : s.Action])}."
  }

  assert {
    condition     = [for r in jsondecode(aws_iam_role_policy.event_task_launch.policy).Statement[0].Resource : r] == ["${aws_ecs_task_definition.task.arn_without_revision}:*"]
    error_message = "ecs:RunTask must be granted on this task definition's family with a :* revision suffix, built from arn_without_revision. A revision-suffixed ARN here stops working the moment the task definition changes, which is every deploy. Got ${jsonencode(jsondecode(aws_iam_role_policy.event_task_launch.policy).Statement[0].Resource)}."
  }

  assert {
    condition     = [for c in jsondecode(aws_iam_role_policy.event_task_launch.policy).Statement[0].Condition.ArnEquals["ecs:cluster"] : c] == [var.cluster]
    error_message = "ecs:RunTask must carry an ArnEquals condition on ecs:cluster naming the cluster this task's rules target — the same value main.tf gives the event target. Without it the execution role can start this task definition in any cluster in the account. Got ${jsonencode(jsondecode(aws_iam_role_policy.event_task_launch.policy).Statement[0].Condition)}."
  }

  assert {
    condition     = length(jsondecode(aws_iam_role_policy.event_task_launch.policy).Statement[1].Resource) == 2 && toset([for r in jsondecode(aws_iam_role_policy.event_task_launch.policy).Statement[1].Resource : r]) == toset([aws_iam_role.task.arn, aws_iam_role.task_execution.arn])
    error_message = "iam:PassRole must cover BOTH roles and nothing else: ECS is handed the task role and the execution role when it starts the task, and either one missing stops it. Got ${jsonencode(jsondecode(aws_iam_role_policy.event_task_launch.policy).Statement[1].Resource)}."
  }

  assert {
    condition     = [for c in jsondecode(aws_iam_role_policy.event_task_launch.policy).Statement[1].Condition.StringEquals["iam:PassedToService"] : c] == ["ecs-tasks.amazonaws.com"]
    error_message = "iam:PassRole must be conditioned on iam:PassedToService = ecs-tasks.amazonaws.com. Without that condition the execution role can hand either role to any AWS service that accepts one. Got ${jsonencode(jsondecode(aws_iam_role_policy.event_task_launch.policy).Statement[1].Condition)}."
  }
}

# ---------------------------------------------------------------------------
# 4. And nothing beyond that.
#
# This role is assumed by a service on the strength of an event pattern, so the
# grant it carries is worth reading as a whole and not statement by statement —
# which is how run 3 reads it, one statement at a time, where a widened resource
# in one statement leaves the others looking correct. Here the whole decoded
# document is flattened instead, so each assertion stands on its own: the
# actions granted, the number of statements granting them, and every resource
# any of them reaches. Two actions, two statements, three resources — starting
# this task in this cluster, and handing ECS the two roles it needs to run it.
# ---------------------------------------------------------------------------

run "the_launch_policy_grants_nothing_beyond_launching" {
  command = apply

  override_data {
    target = data.aws_ssm_parameters_by_path.task
    values = {
      names = ["/dev/acme/task/ingest/env"]
    }
  }

  override_resource {
    target = aws_iam_role.task
    values = {
      arn = "arn:aws:iam::000000000000:role/acme_ingest_task_dev"
    }
  }

  override_resource {
    target = aws_iam_role.task_execution
    values = {
      arn = "arn:aws:iam::000000000000:role/acme_scheduler_ingest_task_execution_dev"
    }
  }

  override_resource {
    target = aws_ecs_task_definition.task
    values = {
      arn_without_revision = "arn:aws:ecs:us-east-1:000000000000:task-definition/acme_event_task_ingest_dev"
    }
  }

  assert {
    condition     = toset(flatten([for s in jsondecode(aws_iam_role_policy.event_task_launch.policy).Statement : s.Action])) == toset(["ecs:RunTask", "iam:PassRole"])
    error_message = "The launch policy grants exactly ecs:RunTask and iam:PassRole. Anything else — ecs:StopTask over every task in the cluster, iam:PassRole on \"*\" — is a permission this task has no use for, on a role EventBridge assumes. Got ${jsonencode([for s in jsondecode(aws_iam_role_policy.event_task_launch.policy).Statement : s.Action])}."
  }

  assert {
    condition     = length(jsondecode(aws_iam_role_policy.event_task_launch.policy).Statement) == 2
    error_message = "Two statements: one to run the task, one to pass the roles. A third is a grant no part of this module asked for. Got ${jsonencode(jsondecode(aws_iam_role_policy.event_task_launch.policy).Statement)}."
  }

  # Every resource in the document, from every statement, flattened into one
  # set: this task definition's family and this task's own two roles. Run 3 pins
  # each statement's resources separately, so a widening inside one statement
  # trips it there; this is the form that holds on its own, whatever the
  # statements get reordered, split or added into.
  assert {
    condition     = toset(flatten([for s in jsondecode(aws_iam_role_policy.event_task_launch.policy).Statement : [for r in s.Resource : r]])) == toset(["${aws_ecs_task_definition.task.arn_without_revision}:*", aws_iam_role.task.arn, aws_iam_role.task_execution.arn])
    error_message = "Every resource the launch policy reaches must be either this task definition's family with a :* revision suffix, or one of this task's own two roles. A \"*\" here, another task's family, or somebody else's role turns a grant to start one task into a grant over workloads it has no part in — on a role EventBridge assumes whenever an event matches. Got ${jsonencode([for s in jsondecode(aws_iam_role_policy.event_task_launch.policy).Statement : s.Resource])}."
  }
}
