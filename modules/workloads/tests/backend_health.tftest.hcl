# The backend's optional container health check (backend_health_check) and
# deployment circuit breaker (backend_circuit_breaker).
#
# Both variables default to null, and null must leave the backend exactly as it
# is: no healthCheck key in the container JSON (a new key is a new task
# definition revision in every environment) and no deployment_circuit_breaker
# block on the service.
#
# The first run APPLIES (against mocks; no AWS is reached) and every run after
# it PLANS. container_definitions reads the backend's SSM parameters, and
# against an empty state that read is deferred to apply, so the JSON is unknown
# at plan. Once run 1 has put the parameters in the test state, later plans can
# read it. `override_during = plan` is not used because CI pins a Terraform that
# rejects it. Account 000000000000 is synthetic; this repository is public.
#
# Run: terraform test -filter=tests/backend_health.tftest.hcl  (from modules/workloads)

mock_provider "aws" {
  # The provider validates policy JSON, CIDRs and ARNs client-side.
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
}

# ---------------------------------------------------------------------------
# R1: neither variable set.
# ---------------------------------------------------------------------------
run "absent_variables_add_no_health_check_and_no_circuit_breaker" {
  command = apply

  # An overridden resource is never created, so its provisioner never runs.
  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition = length([
      for c in jsondecode(aws_ecs_task_definition.backend.container_definitions) : c
      if c.name == "acme_service_dev"
    ]) == 1
    error_message = "The backend container acme_service_dev must be in the task definition."
  }

  assert {
    condition = alltrue([
      for c in jsondecode(aws_ecs_task_definition.backend.container_definitions) :
      !contains(keys(c), "healthCheck") if c.name == "acme_service_dev"
    ])
    error_message = "With backend_health_check unset the backend container must carry no healthCheck key. Got ${aws_ecs_task_definition.backend.container_definitions}."
  }

  assert {
    condition     = length(aws_ecs_service.backend.deployment_circuit_breaker) == 0
    error_message = "With backend_circuit_breaker unset the backend service must have no deployment_circuit_breaker block. Got ${jsonencode(aws_ecs_service.backend.deployment_circuit_breaker)}."
  }

  assert {
    condition     = aws_service_discovery_service.backend[0].health_check_custom_config[0].failure_threshold == 1
    error_message = "The backend's service discovery failure_threshold must be 1."
  }
}

# ---------------------------------------------------------------------------
# R2: health check with every field given.
# ---------------------------------------------------------------------------
run "health_check_with_all_fields_reaches_the_backend_container" {
  command = plan

  variables {
    backend_health_check = {
      command      = ["CMD-SHELL", "curl -f http://localhost:8080/health/live || exit 1"]
      interval     = 15
      timeout      = 7
      retries      = 4
      start_period = 120
    }
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition = jsonencode([
      for c in jsondecode(aws_ecs_task_definition.backend.container_definitions) : c.healthCheck
      if c.name == "acme_service_dev"
      ]) == jsonencode([{
        command     = ["CMD-SHELL", "curl -f http://localhost:8080/health/live || exit 1"]
        interval    = 15
        timeout     = 7
        retries     = 4
        startPeriod = 120
    }])
    error_message = "The backend container healthCheck must carry the given command, interval, timeout, retries and startPeriod. Got ${aws_ecs_task_definition.backend.container_definitions}."
  }

  # The health check is a container concern; Cloud Map's custom health config
  # must not move.
  assert {
    condition     = aws_service_discovery_service.backend[0].health_check_custom_config[0].failure_threshold == 1
    error_message = "Setting backend_health_check must not change the service discovery failure_threshold (must stay 1)."
  }

  assert {
    condition     = length(aws_ecs_service.backend.deployment_circuit_breaker) == 0
    error_message = "Setting only backend_health_check must not add a deployment_circuit_breaker block."
  }
}

# ---------------------------------------------------------------------------
# R2: command only, so every numeric field takes its default.
# ---------------------------------------------------------------------------
run "health_check_with_command_only_uses_the_defaults" {
  command = plan

  variables {
    backend_health_check = {
      command = ["CMD", "/app/healthcheck"]
    }
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition = jsonencode([
      for c in jsondecode(aws_ecs_task_definition.backend.container_definitions) : c.healthCheck
      if c.name == "acme_service_dev"
      ]) == jsonencode([{
        command     = ["CMD", "/app/healthcheck"]
        interval    = 30
        timeout     = 5
        retries     = 3
        startPeriod = 60
    }])
    error_message = "A command-only health check must get interval 30, timeout 5, retries 3, startPeriod 60. Got ${aws_ecs_task_definition.backend.container_definitions}."
  }
}

# ---------------------------------------------------------------------------
# R2: start_period 0 is valid and must not be replaced by the default. The
# other fields sit on their lower bounds.
# ---------------------------------------------------------------------------
run "health_check_keeps_start_period_zero_and_accepts_lower_bounds" {
  command = plan

  variables {
    backend_health_check = {
      command      = ["CMD", "/app/healthcheck"]
      interval     = 5
      timeout      = 2
      retries      = 1
      start_period = 0
    }
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition = jsonencode([
      for c in jsondecode(aws_ecs_task_definition.backend.container_definitions) : c.healthCheck
      if c.name == "acme_service_dev"
      ]) == jsonencode([{
        command     = ["CMD", "/app/healthcheck"]
        interval    = 5
        timeout     = 2
        retries     = 1
        startPeriod = 0
    }])
    error_message = "start_period 0 must reach the container as startPeriod 0. Got ${aws_ecs_task_definition.backend.container_definitions}."
  }
}

run "health_check_accepts_upper_bounds" {
  command = plan

  variables {
    backend_health_check = {
      command      = ["CMD", "/app/healthcheck"]
      interval     = 300
      timeout      = 120
      retries      = 10
      start_period = 300
    }
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition = jsonencode([
      for c in jsondecode(aws_ecs_task_definition.backend.container_definitions) : [c.healthCheck.interval, c.healthCheck.timeout, c.healthCheck.retries, c.healthCheck.startPeriod]
      if c.name == "acme_service_dev"
    ]) == jsonencode([[300, 120, 10, 300]])
    error_message = "The upper bounds 300/120/10/300 must be accepted and reach the container."
  }
}

# ---------------------------------------------------------------------------
# R2: the module refuses out-of-range values and bad command lists.
# ---------------------------------------------------------------------------
run "refuses_interval_below_5" {
  command = plan
  variables {
    backend_health_check = { command = ["CMD", "/app/healthcheck"], interval = 4 }
  }
  expect_failures = [var.backend_health_check]
}

run "refuses_interval_above_300" {
  command = plan
  variables {
    backend_health_check = { command = ["CMD", "/app/healthcheck"], interval = 301 }
  }
  expect_failures = [var.backend_health_check]
}

run "refuses_timeout_below_2" {
  command = plan
  variables {
    backend_health_check = { command = ["CMD", "/app/healthcheck"], timeout = 1 }
  }
  expect_failures = [var.backend_health_check]
}

run "refuses_timeout_above_120" {
  command = plan
  variables {
    backend_health_check = { command = ["CMD", "/app/healthcheck"], timeout = 121 }
  }
  expect_failures = [var.backend_health_check]
}

run "refuses_retries_below_1" {
  command = plan
  variables {
    backend_health_check = { command = ["CMD", "/app/healthcheck"], retries = 0 }
  }
  expect_failures = [var.backend_health_check]
}

run "refuses_retries_above_10" {
  command = plan
  variables {
    backend_health_check = { command = ["CMD", "/app/healthcheck"], retries = 11 }
  }
  expect_failures = [var.backend_health_check]
}

run "refuses_negative_start_period" {
  command = plan
  variables {
    backend_health_check = { command = ["CMD", "/app/healthcheck"], start_period = -1 }
  }
  expect_failures = [var.backend_health_check]
}

run "refuses_start_period_above_300" {
  command = plan
  variables {
    backend_health_check = { command = ["CMD", "/app/healthcheck"], start_period = 301 }
  }
  expect_failures = [var.backend_health_check]
}

run "refuses_empty_command_list" {
  command = plan
  variables {
    backend_health_check = { command = [] }
  }
  expect_failures = [var.backend_health_check]
}

run "refuses_command_not_starting_with_cmd_or_cmd_shell" {
  command = plan
  variables {
    backend_health_check = { command = ["NONE", "/app/healthcheck"] }
  }
  expect_failures = [var.backend_health_check]
}

run "refuses_cmd_shell_without_a_string" {
  command = plan
  variables {
    backend_health_check = { command = ["CMD-SHELL"] }
  }
  expect_failures = [var.backend_health_check]
}

run "refuses_cmd_shell_with_two_strings" {
  command = plan
  variables {
    backend_health_check = { command = ["CMD-SHELL", "curl -f http://localhost:8080/", "exit 1"] }
  }
  expect_failures = [var.backend_health_check]
}

run "refuses_cmd_alone" {
  command = plan
  variables {
    backend_health_check = { command = ["CMD"] }
  }
  expect_failures = [var.backend_health_check]
}

# ---------------------------------------------------------------------------
# R3: circuit breaker.
# ---------------------------------------------------------------------------
run "circuit_breaker_with_given_values" {
  command = plan

  variables {
    backend_circuit_breaker = {
      enable   = false
      rollback = true
    }
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition     = length(aws_ecs_service.backend.deployment_circuit_breaker) == 1
    error_message = "backend_circuit_breaker set must give exactly one deployment_circuit_breaker block."
  }

  assert {
    condition     = jsonencode([for b in aws_ecs_service.backend.deployment_circuit_breaker : [b.enable, b.rollback]]) == jsonencode([[false, true]])
    error_message = "enable = false must be kept; got ${jsonencode(aws_ecs_service.backend.deployment_circuit_breaker)}."
  }

  assert {
    condition = alltrue([
      for c in jsondecode(aws_ecs_task_definition.backend.container_definitions) :
      !contains(keys(c), "healthCheck") if c.name == "acme_service_dev"
    ])
    error_message = "Setting only backend_circuit_breaker must not add a healthCheck to the backend container."
  }
}

run "circuit_breaker_rollback_false" {
  command = plan

  variables {
    backend_circuit_breaker = {
      enable   = true
      rollback = false
    }
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition     = jsonencode([for b in aws_ecs_service.backend.deployment_circuit_breaker : [b.enable, b.rollback]]) == jsonencode([[true, false]])
    error_message = "rollback = false must be kept; got ${jsonencode(aws_ecs_service.backend.deployment_circuit_breaker)}."
  }
}

run "empty_circuit_breaker_gets_both_defaults" {
  command = plan

  variables {
    backend_circuit_breaker = {}
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition     = jsonencode([for b in aws_ecs_service.backend.deployment_circuit_breaker : [b.enable, b.rollback]]) == jsonencode([[true, true]])
    error_message = "backend_circuit_breaker = {} must give one block with enable = true, rollback = true; got ${jsonencode(aws_ecs_service.backend.deployment_circuit_breaker)}."
  }
}
