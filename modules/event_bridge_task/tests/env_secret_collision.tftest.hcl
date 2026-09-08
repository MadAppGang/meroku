# The precondition on aws_ecs_task_definition.task — a name may not appear in
# both the container's `environment` and its `secrets`. ECS refuses that task
# definition:
#
#   ClientException: The secret name must be unique and not shared with any new
#   or existing environment variables set on the container, such as
#   'SCAN_CURSOR_STORE'.
#
# env_secret_check.tf carries the full account of how a project acquires one
# without anybody editing the name in the error. In short: the environment side
# is DECLARED in project/<env>.yaml, the secret side is DISCOVERED by reading
# every SSM parameter under the task's path, and neither mechanism knows the
# other exists. So `aws ssm put-parameter` is a way to break somebody else's
# next `terraform apply`, part-way through, naming a variable they have never
# touched.
#
# The contract, in the order the contract matters.
#
#   1. A clean configuration still applies, and the message still renders.
#      (a_clean_configuration_applies_and_the_message_renders)
#   2. A declared name that SSM also supplies fails the PLAN, and the message
#      that comes out of the real wiring renders — with the task's name, the
#      colliding name, and both mechanisms in it.
#      (a_declared_name_that_ssm_also_supplies_fails_the_plan)
#   3. A variable this module sets ITSELF collides just the same, and the reader
#      is sent to the file that sets it — this module's env.tf, not
#      modules/workloads'. It is the case with no YAML edit available, so a
#      wrong pointer here leaves the reader nowhere.
#      (a_variable_the_module_sets_itself_collides_and_names_this_modules_env_tf)
#   4. The parameter read is recursive, so a NESTED parameter's last segment is
#      the secret name. A check comparing anything but that last segment misses
#      it.
#      (a_nested_parameter_collides_on_its_last_segment)
#
# Rule 1's "and the message still renders" is the half that needs a test at all.
# `terraform validate` never evaluates a precondition's error_message, and
# Terraform builds that message BEFORE it tests the condition — so a message
# that cannot render kills a plan with nothing wrong with it, passes every other
# gate, and fails in a user's terminal. That is not hypothetical: the
# compute_pool message shipped exactly that way in v4.2.0 and broke every
# Fargate deploy for four releases
# (modules/compute_pool_check/tests/messages.tftest.hcl).
#
# The division of labour with ../../env_secret_check/tests/messages.tftest.hcl
# is the one modules/workloads/tests/env_secret_collision.tftest.hcl already
# has: that file owns the TEXT, byte for byte, against synthetic inputs, in a
# module with no provider. This file owns the WIRING — that the names this
# module hands over are the ones ECS will compare. Most of all it owns the
# upper-casing: an SSM parameter's last segment becomes a secret named
# `upper(...)`, so ".../scan_cursor_store" is the same name as the YAML variable
# SCAN_CURSOR_STORE, and a check comparing raw parameter paths would find
# nothing wrong with the configuration that fails.
#
# ---------------------------------------------------------------------------
# Every run here PLANS except the first, and unlike modules/workloads that is a
# convenience rather than a constraint.
#
# There, the SSM data sources `depends_on` the aws_ssm_parameter resources, so
# against an empty state the reads are deferred, the secret names are unknown,
# the condition is unknown — and Terraform POSTPONES an unknown precondition
# rather than failing it, which makes `expect_failures` unusable until an apply
# has put the parameters in state.
#
# data.aws_ssm_parameters_by_path.task (env.tf) has no depends_on. The read is
# not sequenced behind aws_ssm_parameter.task_env, so an override resolves at
# plan against an empty state and the condition is known on the very first plan.
# Run 1 still applies, because an apply is the only thing that proves the
# preconditions were evaluated on a configuration that is entirely correct — the
# case a message that cannot render destroys.
#
# `expect_failures` cannot be used in an apply run either way round: a condition
# that fails during the PLANNING stage of an apply is reported as a test failure
# regardless — "the apply operation could not be executed and so the overall test
# case will be marked as a failure".
# ---------------------------------------------------------------------------
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

  custom_env_vars = [
    { name = "INGEST_MODE", value = "batch" },
  ]
}

# The everyday configuration: nothing shared between the two lists.
#
# The message assertions are the point of it. Terraform builds a precondition's
# error_message before it tests the condition, so this string is constructed on
# every plan of every healthy project. A message that cannot render — a null
# interpolated into it, an index into a map with no such key — fails HERE, with
# nothing wrong with the configuration, which is precisely how the compute_pool
# message broke four consecutive releases.
run "a_clean_configuration_applies_and_the_message_renders" {
  command = apply

  # /env is the parameter env.tf creates for every task, so it is always in this
  # list and always becomes the secret ENV. It is stated in every run below
  # because it is real — and because it is a name that can collide by accident:
  # a task that puts ENV in its environment_variables hits this same error with
  # nothing exotic in its configuration at all.
  override_data {
    target = data.aws_ssm_parameters_by_path.task
    values = {
      names = ["/dev/acme/task/ingest/env", "/dev/acme/task/ingest/database_password"]
    }
  }

  # The rendered `environment` array, in order, as it reaches the container
  # definition. Order is load-bearing: it goes through jsonencode into
  # container_definitions, which is ForceNew, so a reordering registers a new
  # task-definition revision in every environment for no change at all. Every
  # name below is also a name the collision check must be comparing — the SSM
  # parameters overridden above deliberately avoid all of them, which is what
  # makes this run clean.
  assert {
    condition = [for e in jsondecode(aws_ecs_task_definition.task.container_definitions)[0].environment : e.name] == [
      "AWS_REGION",
      "SQS_QUEUE_URL",
      "EVENT_BUS_NAME",
      "EVENT_SOURCE",
      "API_DOMAIN",
      "PRIVATE_DNS_NAMESPACE",
      "BACKEND_INTERNAL_URL",
      "INGEST_MODE",
    ]
    error_message = "The container's environment list changed shape or order. The first seven are local.default_env_vars (env.tf) — set for every task whether the user asked or not, and therefore collidable without any YAML edit — and the last is custom_env_vars. Got ${jsonencode([for e in jsondecode(aws_ecs_task_definition.task.container_definitions)[0].environment : e.name])}"
  }

  # And the secrets, to name the other half of what the check compares. "ENV" is
  # the /env parameter env.tf creates for every task; the upper-casing of
  # "database_password" is the transformation the whole check turns on.
  assert {
    condition     = [for s in jsondecode(aws_ecs_task_definition.task.container_definitions)[0].secrets : s.name] == ["ENV", "DATABASE_PASSWORD"]
    error_message = "The secret names are the upper-cased last segments of the SSM parameter paths. Got ${jsonencode([for s in jsondecode(aws_ecs_task_definition.task.container_definitions)[0].secrets : s.name])}"
  }

  assert {
    condition     = length(module.task_env_secret_check.collisions["task"]) == 0
    error_message = "Nothing here shares a name between environment and secrets. Got ${jsonencode(module.task_env_secret_check.collisions["task"])}."
  }

  assert {
    condition     = length(module.task_env_secret_check.message["task"]) > 0
    error_message = "The message must render even with no collision — Terraform builds error_message before it tests the condition, so a message that needs a non-empty collision list kills a plan that is entirely correct."
  }

  # The specific way the empty case goes wrong: join(", ", []) is "", which
  # renders "would set  both as a plain environment variable". Never shown,
  # because the condition passes — which is exactly what lets it reach a release.
  assert {
    condition     = !strcontains(module.task_env_secret_check.message["task"], "would set  both")
    error_message = "The message trails off into an empty name list: ${module.task_env_secret_check.message["task"]}"
  }
}

# The reported failure, in this module's shape.
#
# SCAN_CURSOR_STORE is declared in the task's environment_variables. Somebody
# then creates the SSM parameter /dev/acme/task/ingest/scan_cursor_store — note
# the LOWER CASE, which is what the console and every put-parameter example
# encourage — and env.tf turns its last segment into the secret
# SCAN_CURSOR_STORE. Two mechanisms, one name, and the next apply anybody runs
# dies at RegisterTaskDefinition.
#
# The lower-case parameter is the whole point of the fixture. It is what proves
# the check compares the names ECS compares — the upper-cased ones — rather than
# the parameter paths. A check written against the raw paths passes this
# configuration and lets the deploy fail anyway.
run "a_declared_name_that_ssm_also_supplies_fails_the_plan" {
  command = plan

  variables {
    custom_env_vars = [
      { name = "SCAN_CURSOR_STORE", value = "dynamo" },
    ]
  }

  override_data {
    target = data.aws_ssm_parameters_by_path.task
    values = {
      names = ["/dev/acme/task/ingest/env", "/dev/acme/task/ingest/scan_cursor_store"]
    }
  }

  # THE assertion. Not on the EventBridge target and not on a module output: on
  # the resource whose RegisterTaskDefinition call AWS rejects, which is the
  # address Terraform prints next to the message.
  expect_failures = [
    aws_ecs_task_definition.task,
  ]

  # And the message that goes with it, read through the real wiring — the half
  # `terraform validate` cannot see. Reading it at all proves it rendered; every
  # value in the sentence comes from this module, and any one of them arriving
  # null fails this run instead of a user's deploy. Assertions are still
  # evaluated in a run whose expected failure occurred.
  assert {
    condition     = module.task_env_secret_check.collisions["task"] == tolist(["SCAN_CURSOR_STORE"])
    error_message = "The lower-case SSM parameter /dev/acme/task/ingest/scan_cursor_store becomes the secret SCAN_CURSOR_STORE, which is the name in the task's environment_variables. Exactly that one name must be reported. Got ${jsonencode(module.task_env_secret_check.collisions["task"])} — an empty list means the check is comparing raw parameter paths instead of the names ECS compares."
  }

  assert {
    condition     = strcontains(module.task_env_secret_check.message["task"], "Event task \"ingest\" would set SCAN_CURSOR_STORE both as a plain environment variable and as a secret.")
    error_message = "The message must name the TASK and the COLLIDING VARIABLE in its first sentence; whoever reads it is looking at an error about a variable they did not touch. Got: ${module.task_env_secret_check.message["task"]}"
  }

  # Both mechanisms named, with somewhere to go for each — and with THIS
  # module's paths. The SSM prefix carries an extra "task" segment a service's
  # does not, and the YAML field is environment_variables rather than env_vars;
  # a reader handed a service's spelling of either has nowhere to make the edit.
  assert {
    condition     = strcontains(module.task_env_secret_check.message["task"], "/dev/acme/task/ingest") && strcontains(module.task_env_secret_check.message["task"], "environment_variables in project/dev.yaml")
    error_message = "The message must name both sides with this module's own path and field. Got: ${module.task_env_secret_check.message["task"]}"
  }
}

# The case with no YAML edit available, and the reason `defaults_source` is
# threaded through the check module at all.
#
# AWS_REGION is set by env.tf for every task, whether the user asked for it or
# not. It appears in project/<env>.yaml nowhere, so "remove it from
# environment_variables" is not an option — deleting the SSM parameter is the
# only remedy there is, and the reader can only satisfy themselves of that by
# reading the file that sets it. That file is modules/event_bridge_task/env.tf.
# Before this wiring existed the sentence named modules/workloads/env_services.tf
# outright, which sets no variable on this container at all.
run "a_variable_the_module_sets_itself_collides_and_names_this_modules_env_tf" {
  command = plan

  variables {
    custom_env_vars = []
  }

  override_data {
    target = data.aws_ssm_parameters_by_path.task
    values = {
      names = ["/dev/acme/task/ingest/env", "/dev/acme/task/ingest/aws_region"]
    }
  }

  expect_failures = [
    aws_ecs_task_definition.task,
  ]

  assert {
    condition     = module.task_env_secret_check.collisions["task"] == tolist(["AWS_REGION"])
    error_message = "AWS_REGION is in local.default_env_vars and is on every one of these containers, so a parameter named .../aws_region collides with a task whose environment_variables are empty. Got ${jsonencode(module.task_env_secret_check.collisions["task"])}."
  }

  assert {
    condition     = strcontains(module.task_env_secret_check.message["task"], "the rest of the list in modules/event_bridge_task/env.tf")
    error_message = "The remedy paragraph must send the reader to the file that sets the variable they cannot edit. Got: ${module.task_env_secret_check.message["task"]}"
  }

  assert {
    condition     = !strcontains(module.task_env_secret_check.message["task"], "modules/workloads")
    error_message = "An event task's reader has no modules/workloads/env_services.tf; sending them there at the one moment they have no other option is worse than saying nothing. Got: ${module.task_env_secret_check.message["task"]}"
  }
}

# data.aws_ssm_parameters_by_path.task sets `recursive = true` (env.tf), so a
# parameter nested below the task's prefix is read too — and its secret name is
# its LAST segment, not its path relative to the prefix. /…/ingest/db/api_key
# becomes API_KEY, and collides with a declared API_KEY exactly as a top-level
# parameter would.
#
# Without this run a check that compared, say, the path below the prefix
# ("db/api_key") would pass every other assertion in this file and still let the
# apply die.
run "a_nested_parameter_collides_on_its_last_segment" {
  command = plan

  variables {
    custom_env_vars = [
      { name = "API_KEY", value = "not-a-real-key" },
    ]
  }

  override_data {
    target = data.aws_ssm_parameters_by_path.task
    values = {
      names = ["/dev/acme/task/ingest/env", "/dev/acme/task/ingest/db/api_key"]
    }
  }

  expect_failures = [
    aws_ecs_task_definition.task,
  ]

  assert {
    condition     = module.task_env_secret_check.collisions["task"] == tolist(["API_KEY"])
    error_message = "The read is recursive, so /dev/acme/task/ingest/db/api_key is one of this container's secrets and its name is API_KEY. Got ${jsonencode(module.task_env_secret_check.collisions["task"])}."
  }
}
