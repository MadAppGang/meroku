# The precondition on aws_ecs_task_definition.{backend,services} — a name may
# not appear in both the container's `environment` and its `secrets`. ECS
# refuses that task definition:
#
#   ClientException: The secret name must be unique and not shared with any new
#   or existing environment variables set on the container, such as
#   'SCAN_CURSOR_STORE'.
#
# env_secret_check.tf carries the full account of how a project acquires one
# without anybody editing the name in the error. In short: the environment side
# is DECLARED in project/<env>.yaml, the secret side is DISCOVERED by reading
# every SSM parameter under the workload's path, and neither mechanism knows the
# other exists. So `aws ssm put-parameter` is a way to break somebody else's
# next `terraform apply`, part-way through, naming a variable they have never
# touched.
#
# The contract, in the order the contract matters.
#
#   1. A clean configuration still applies, and every message still renders.
#      (a_clean_configuration_applies_and_every_message_renders)
#   2. A collision fails the PLAN, and the message that comes out of the real
#      wiring renders — with the service's name, the colliding name, and both
#      mechanisms in it.
#      (a_declared_name_that_ssm_also_supplies_fails_the_plan)
#   3. The backend is covered by the same rule and gets its own message.
#      (the_backend_is_covered_too)
#   4. The comparison is per workload. One service's variable does not collide
#      with another service's parameter.
#      (a_name_shared_between_two_services_does_not_collide)
#
# Rule 1's "and every message renders" is the half that needs a test at all.
# `terraform validate` never evaluates a precondition's error_message, and
# Terraform builds that message BEFORE it tests the condition and for every
# instance — so a message that cannot render kills a plan with nothing wrong
# with it, passes every other gate, and fails in a user's terminal. That is not
# hypothetical: the compute_pool message shipped exactly that way in v4.2.0 and
# broke every Fargate deploy for four releases
# (modules/compute_pool_check/tests/messages.tftest.hcl).
#
# The division of labour with ../../env_secret_check/tests/messages.tftest.hcl:
# that file owns the TEXT, byte for byte, against synthetic inputs, in a module
# with no provider. This file owns the WIRING — that the names modules/workloads
# hands over are the ones ECS will compare, which is the part no provider-less
# module can know. Most of all it owns the upper-casing: an SSM parameter's last
# segment becomes a secret named `upper(...)`, so ".../scan_cursor_store" is the
# same name as the YAML variable SCAN_CURSOR_STORE, and a check comparing raw
# parameter paths would find nothing wrong with the configuration that fails.
#
# ---------------------------------------------------------------------------
# Why the first run APPLIES and every run after it PLANS. This is structural,
# not stylistic, and getting it backwards silently guts the file.
#
# The SSM data sources `depends_on` the aws_ssm_parameter resources
# (env_services.tf, env.tf). Against an empty state those parameters are pending
# creation, so Terraform defers the reads, the secret names are unknown at plan,
# the condition is unknown — and Terraform POSTPONES an unknown precondition
# rather than failing it. A `command = plan` run there reports "Missing expected
# failure" no matter how broken the configuration is.
#
# Once the parameters exist in state the reads happen at plan and the condition
# is known, which is both what makes `expect_failures` work here and what a real
# environment looks like from its second apply onwards — the exact situation the
# guard is for: a live project, someone runs put-parameter, the next plan breaks.
# Run 1 is what puts them there.
#
# It cannot be an apply run either way round: `expect_failures` on a condition
# that fails during the PLANNING stage of an apply run is reported as a test
# failure regardless — "the apply operation could not be executed and so the
# overall test case will be marked as a failure".
#
# The usual ordering advice therefore does not apply here (see
# tests/revision_notification.tftest.hcl, which puts its cheap plan runs first
# because an ERROR skips every run after it). Run 1 has to be first. It is also
# the only run that applies, which keeps the surface for such an error small.
#
# `override_during = plan` would let the overrides resolve earlier and make all
# of this moot. It is deliberately not used: CI pins Terraform 1.9.8
# (.github/workflows/ci.yml), which rejects the argument outright and would fail
# to parse this file in the one place it must run.
# ---------------------------------------------------------------------------
#
# Run: terraform test  (from modules/workloads)

mock_provider "aws" {
  # Identical to tests/revision_notification.tftest.hcl — see the long comment
  # there for why each of these exists. Briefly: a generated mock string is not
  # a policy document, a CIDR block or an ARN, and the AWS provider validates
  # all three CLIENT-side, before anything would reach a network. Account
  # 000000000000 is synthetic; this repository is public.
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

variables {
  project          = "acme"
  env              = "dev"
  vpc_id           = "vpc-00000000000000000"
  subnet_ids       = ["subnet-00000000000000001", "subnet-00000000000000002"]
  private_dns_name = "acme.internal"
  api_domain       = "api.example.com"
  github_subjects  = ["repo:acme/acme:ref:refs/heads/main"]

  # The clean set, and the same two services in every run — the for_each keys
  # have to stay put across runs that share one state. Only env_vars,
  # backend_env and the overridden parameter lists vary below.
  #
  # docker_image is set on both so the container definition does not depend on a
  # mocked ECR repository URL. Nothing here is about ECR.
  services = [
    { name = "orders", docker_image = "public.ecr.aws/nginx/nginx:stable", env_vars = { ORDERS_MODE = "batch" } },
    { name = "billing", docker_image = "public.ecr.aws/nginx/nginx:stable", env_vars = { LEDGER_MODE = "batch" } },
  ]
}

# The everyday configuration, and the run that gives every later one a state to
# plan against.
#
# The message assertions are the point of it. Terraform builds a precondition's
# error_message before it tests the condition and for EVERY instance, so these
# strings are constructed on every plan of every healthy project. A message that
# cannot render — a null interpolated into it, an index into a map with no such
# key — fails HERE, with nothing wrong with the configuration, which is
# precisely how the compute_pool message broke four consecutive releases.
run "a_clean_configuration_applies_and_every_message_renders" {
  command = apply

  # An override, not a mock, and the difference is the point: an overridden
  # resource is never created, so its PROVISIONER never runs.
  # null_resource.build_ci_lambda shells out to `go build` (lambda.tf), which
  # has no business on the critical path of a test about a set intersection.
  override_resource {
    target = null_resource.build_ci_lambda
  }

  # /env is the parameter env_services.tf and env.tf create for every workload,
  # so it is always in this list and always becomes the secret ENV. It is stated
  # in every run below because it is real — and because it is a name that can
  # collide by accident: a project that puts ENV in its env_vars hits this same
  # error with nothing exotic in its configuration at all.
  override_data {
    target = data.aws_ssm_parameters_by_path.services["orders"]
    values = {
      names = ["/dev/acme/orders/env", "/dev/acme/orders/database_password"]
    }
  }

  override_data {
    target = data.aws_ssm_parameters_by_path.services["billing"]
    values = {
      names = ["/dev/acme/billing/env"]
    }
  }

  override_data {
    target = data.aws_ssm_parameters_by_path.backend
    values = {
      names = ["/dev/acme/backend/env", "/dev/acme/backend/database_password"]
    }
  }

  # The apply completing at all is most of this run's value: the preconditions
  # were evaluated for all three workloads and none of them fired.
  assert {
    condition     = length(aws_ecs_task_definition.services) == 2
    error_message = "Both services' task definitions must still be registered. Got ${length(aws_ecs_task_definition.services)}."
  }

  # The rendered `environment` array, in order, as it reaches the container
  # definition. Two separate things depend on this list and both are why it is
  # pinned here rather than left to the check module's own tests:
  #
  #   * ZERO DIFF. The expression that builds it moved out of services.tf into
  #     local.services_container_env (env_services.tf) so that the precondition
  #     compares the list the resource carries rather than a second copy of it.
  #     jsonencode sorts an object's keys but never a list's elements, so a
  #     reordering here re-renders container_definitions, which is ForceNew —
  #     one unrequested task-definition revision in every environment, for no
  #     change. This assertion is what says the move was a move.
  #   * The check reads the SAME list. Every name below is a name the collision
  #     check must be comparing; the SSM parameters overridden above deliberately
  #     avoid all of them, which is what makes this run clean.
  assert {
    condition = [for e in jsondecode(aws_ecs_task_definition.services["orders"].container_definitions)[0].environment : e.name] == [
      "PG_DATABASE_HOST",
      "PG_DATABASE_USERNAME",
      "PG_DATABASE_NAME",
      "AWS_REGION",
      "URL",
      "SQS_QUEUE_URL",
      "AWS_QUEUE_URL",
      "EVENT_BUS_NAME",
      "API_DOMAIN",
      "PRIVATE_DNS_NAMESPACE",
      "BACKEND_INTERNAL_URL",
      "ORDERS_MODE",
      "EVENT_SOURCE",
      "SERVICE_INTERNAL_URL",
      "SERVICE_NAME",
    ]
    error_message = "The container's environment list changed shape or order. Order is load-bearing: it goes through jsonencode into container_definitions, which is ForceNew, so a reordering registers a new task-definition revision in every environment for no change at all. Got ${jsonencode([for e in jsondecode(aws_ecs_task_definition.services["orders"].container_definitions)[0].environment : e.name])}"
  }

  # And the secrets, to name the other half of what the check compares. "ENV" is
  # the /env parameter env_services.tf creates for every service; the upper-casing
  # of "database_password" is the transformation the whole check turns on.
  assert {
    condition     = [for s in jsondecode(aws_ecs_task_definition.services["orders"].container_definitions)[0].secrets : s.name] == ["ENV", "DATABASE_PASSWORD"]
    error_message = "The secret names are the upper-cased last segments of the SSM parameter paths. Got ${jsonencode([for s in jsondecode(aws_ecs_task_definition.services["orders"].container_definitions)[0].secrets : s.name])}"
  }

  assert {
    condition     = alltrue([for k, c in module.service_env_secret_check.collisions : length(c) == 0]) && length(module.backend_env_secret_check.collisions["backend"]) == 0
    error_message = "Nothing here shares a name between environment and secrets. Got services ${jsonencode(module.service_env_secret_check.collisions)}, backend ${jsonencode(module.backend_env_secret_check.collisions["backend"])}."
  }

  assert {
    condition     = alltrue([for k, m in module.service_env_secret_check.message : length(m) > 0]) && length(module.backend_env_secret_check.message["backend"]) > 0
    error_message = "Every workload must render a message even with no collision — Terraform builds error_message before it tests the condition, so a message that needs a non-empty collision list kills a plan that is entirely correct. Got: ${jsonencode(module.service_env_secret_check.message)}"
  }

  # The specific way the empty case goes wrong: join(", ", []) is "", which
  # renders "would set  both as a plain environment variable". Never shown,
  # because the condition passes — which is exactly what lets it reach a release.
  assert {
    condition     = alltrue([for k, m in module.service_env_secret_check.message : !strcontains(m, "would set  both")])
    error_message = "A message trails off into an empty name list: ${jsonencode({ for k, m in module.service_env_secret_check.message : k => m if strcontains(m, "would set  both") })}"
  }
}

# The reported failure, reproduced on the state run 1 left behind — which is to
# say on a live environment, which is where it happens.
#
# SCAN_CURSOR_STORE is declared in the service's env_vars. Somebody then creates
# the SSM parameter /dev/acme/orders/scan_cursor_store — note the LOWER CASE,
# which is what the console and every put-parameter example encourage — and
# env_services.tf turns its last segment into the secret SCAN_CURSOR_STORE. Two
# mechanisms, one name, and the next apply anybody runs dies at
# RegisterTaskDefinition.
#
# The lower-case parameter is the whole point of the fixture. It is what proves
# the check compares the names ECS compares — the upper-cased ones — rather than
# the parameter paths. A check written against the raw paths passes this
# configuration and lets the deploy fail anyway.
run "a_declared_name_that_ssm_also_supplies_fails_the_plan" {
  command = plan

  variables {
    services = [
      { name = "orders", docker_image = "public.ecr.aws/nginx/nginx:stable", env_vars = { SCAN_CURSOR_STORE = "dynamo" } },
      { name = "billing", docker_image = "public.ecr.aws/nginx/nginx:stable", env_vars = { LEDGER_MODE = "batch" } },
    ]
  }

  override_data {
    target = data.aws_ssm_parameters_by_path.services["orders"]
    values = {
      names = ["/dev/acme/orders/env", "/dev/acme/orders/scan_cursor_store"]
    }
  }

  override_data {
    target = data.aws_ssm_parameters_by_path.services["billing"]
    values = {
      names = ["/dev/acme/billing/env"]
    }
  }

  override_data {
    target = data.aws_ssm_parameters_by_path.backend
    values = {
      names = ["/dev/acme/backend/env"]
    }
  }

  # THE assertion. Not on the ECS service and not on a module output: on the
  # resource whose RegisterTaskDefinition call AWS rejects, which is the address
  # Terraform prints next to the message.
  expect_failures = [
    aws_ecs_task_definition.services["orders"],
  ]

  # And the message that goes with it, read through the real wiring — the half
  # `terraform validate` cannot see. Reading it at all proves it rendered; every
  # value in the sentence comes from modules/workloads, and any one of them
  # arriving null fails this run instead of a user's deploy. Assertions are
  # still evaluated in a run whose expected failure occurred.
  assert {
    condition     = module.service_env_secret_check.collisions["orders"] == tolist(["SCAN_CURSOR_STORE"])
    error_message = "The lower-case SSM parameter /dev/acme/orders/scan_cursor_store becomes the secret SCAN_CURSOR_STORE, which is the name in the service's env_vars. Exactly that one name must be reported. Got ${jsonencode(module.service_env_secret_check.collisions["orders"])} — an empty list means the check is comparing raw parameter paths instead of the names ECS compares."
  }

  assert {
    condition     = strcontains(module.service_env_secret_check.message["orders"], "Service \"orders\" would set SCAN_CURSOR_STORE both as a plain environment variable and as a secret.")
    error_message = "The message must name the SERVICE and the COLLIDING VARIABLE in its first sentence; whoever reads it is looking at an error about a variable they did not touch. Got: ${module.service_env_secret_check.message["orders"]}"
  }

  # Both mechanisms named, with somewhere to go for each. Without the SSM path
  # the reader has no way to find the half of the collision that is in no config
  # file.
  assert {
    condition     = strcontains(module.service_env_secret_check.message["orders"], "/dev/acme/orders") && strcontains(module.service_env_secret_check.message["orders"], "env_vars in project/dev.yaml")
    error_message = "The message must name both sides: the SSM path the secrets are discovered under, and the YAML field the declared variables come from. Got: ${module.service_env_secret_check.message["orders"]}"
  }

  # The other service in the same plan is fine and must be seen to be fine. A
  # guard that failed the whole environment on one service's collision would
  # satisfy expect_failures above without this.
  assert {
    condition     = length(module.service_env_secret_check.collisions["billing"]) == 0
    error_message = "\"billing\" shares no name between its environment and its secrets and must be clean, got ${jsonencode(module.service_env_secret_check.collisions["billing"])}."
  }
}

# The backend: a separate resource, in a separate file, reading separate locals
# (env.tf, backend.tf), sharing nothing with the services path but the check
# module. The environment side here is var.backend_env, which env/main.hbs
# renders from backend_env_variables.
run "the_backend_is_covered_too" {
  command = plan

  variables {
    backend_env = [
      { name = "STRIPE_SECRET_KEY", value = "sk_test_not_a_real_key" },
    ]
  }

  override_data {
    target = data.aws_ssm_parameters_by_path.backend
    values = {
      names = ["/dev/acme/backend/env", "/dev/acme/backend/stripe_secret_key"]
    }
  }

  # Both services stay clean, so the backend has to fail on its own account.
  override_data {
    target = data.aws_ssm_parameters_by_path.services["orders"]
    values = {
      names = ["/dev/acme/orders/env"]
    }
  }

  override_data {
    target = data.aws_ssm_parameters_by_path.services["billing"]
    values = {
      names = ["/dev/acme/billing/env"]
    }
  }

  expect_failures = [
    aws_ecs_task_definition.backend,
  ]

  assert {
    condition     = module.backend_env_secret_check.collisions["backend"] == tolist(["STRIPE_SECRET_KEY"])
    error_message = "The backend's own collision must be reported. Got ${jsonencode(module.backend_env_secret_check.collisions["backend"])}."
  }

  # The backend's sentence differs from a service's in both halves: it opens
  # "The backend" rather than "Service \"x\"", and it sends the reader to
  # backend_env_variables rather than to env_vars.
  assert {
    condition     = strcontains(module.backend_env_secret_check.message["backend"], "The backend would set STRIPE_SECRET_KEY both as a plain environment variable and as a secret.")
    error_message = "The backend message must open with the backend and name the colliding variable. Got: ${module.backend_env_secret_check.message["backend"]}"
  }

  assert {
    condition     = strcontains(module.backend_env_secret_check.message["backend"], "/dev/acme/backend") && strcontains(module.backend_env_secret_check.message["backend"], "backend_env_variables in project/dev.yaml")
    error_message = "The backend message must name the backend's SSM path and backend_env_variables — a reader sent to a service's env_vars has nowhere to make the edit. Got: ${module.backend_env_secret_check.message["backend"]}"
  }

  # No service is dragged down with it.
  assert {
    condition     = alltrue([for k, c in module.service_env_secret_check.collisions : length(c) == 0])
    error_message = "The backend's collision must not be attributed to any service. Got ${jsonencode(module.service_env_secret_check.collisions)}."
  }
}

# The near miss. The same NAME, on two different services, and no collision:
#
#   orders  declares SCAN_CURSOR_STORE in env_vars, and has no SSM parameter of
#           that name under its own path.
#   billing has the parameter and does not declare the variable.
#
# Each service's secrets come from its own path — data.aws_ssm_parameters_by_path
# is per service — so the two containers never share an environment and both must
# plan. Without this run the guard could be written against the union of every
# service's parameters, pass every assertion above, and start refusing
# configurations that deploy cleanly today.
run "a_name_shared_between_two_services_does_not_collide" {
  command = plan

  variables {
    services = [
      { name = "orders", docker_image = "public.ecr.aws/nginx/nginx:stable", env_vars = { SCAN_CURSOR_STORE = "dynamo" } },
      { name = "billing", docker_image = "public.ecr.aws/nginx/nginx:stable", env_vars = { LEDGER_MODE = "batch" } },
    ]
  }

  override_data {
    target = data.aws_ssm_parameters_by_path.services["orders"]
    values = {
      names = ["/dev/acme/orders/env"]
    }
  }

  override_data {
    target = data.aws_ssm_parameters_by_path.services["billing"]
    values = {
      names = ["/dev/acme/billing/env", "/dev/acme/billing/scan_cursor_store"]
    }
  }

  override_data {
    target = data.aws_ssm_parameters_by_path.backend
    values = {
      names = ["/dev/acme/backend/env"]
    }
  }

  assert {
    condition     = alltrue([for k, c in module.service_env_secret_check.collisions : length(c) == 0])
    error_message = "orders DECLARES SCAN_CURSOR_STORE and billing SUPPLIES it, but from its own SSM path — the two containers never share an environment, so neither collides. Got ${jsonencode(module.service_env_secret_check.collisions)}."
  }

  assert {
    condition     = length(module.backend_env_secret_check.collisions["backend"]) == 0
    error_message = "The backend must be clean here too. Got ${jsonencode(module.backend_env_secret_check.collisions["backend"])}."
  }
}
