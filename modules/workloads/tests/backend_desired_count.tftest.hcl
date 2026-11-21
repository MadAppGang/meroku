# Who owns the backend service's task count.
#
# With backend_autoscaling_enabled, Application Auto Scaling sets the live
# count, and backend_desired_count is only the size the service starts at: an
# apply must never plan the service's desired_count back to the configured
# value. With autoscaling off, terraform_data.backend_desired_count applies the
# configured count and is keyed on it, so a changed count still reaches the
# service.
#
# Runs share one state. A run that applies leaves its resources in that state
# and the next plan diffs against them. That is how these runs model a later
# apply against a service that already exists. No AWS is reached. Account
# 000000000000 is synthetic, and this repository is public.
#
# No run here applies with autoscaling off: that would create
# terraform_data.backend_desired_count, whose provisioner runs the AWS CLI.
# override_resource does not stop a provisioner, so no applying run anywhere in
# modules/workloads/tests uses the default shape (autoscaling off). That shape
# is covered by plan runs only.
#
# Whether toggling autoscaling replaces the service is not asserted here: a
# replaced service keeps its id under the mock provider, and a test cannot read
# a plan's actions. ignore_changes is static and desired_count is not ForceNew,
# so the toggle changes no attribute that would force a replace.
#
# Run: terraform test -filter=tests/backend_desired_count.tftest.hcl  (from modules/workloads)

mock_provider "aws" {
  # The provider validates policy JSON, CIDRs and ARNs client-side.
  mock_data "aws_iam_policy_document" {
    defaults = {
      json = "{\"Version\":\"2012-10-17\",\"Statement\":[]}"
    }
  }
  mock_data "aws_caller_identity" {
    defaults = { account_id = "000000000000" }
  }
  mock_data "aws_region" {
    defaults = { region = "us-east-1", name = "us-east-1" }
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

# override_resource does not stop a provisioner under terraform test, as
# modules/ses/tests/lifecycle/lifecycle.tftest.hcl says for terraform_data. In
# a run that applies, the local build in lambda.tf's ci_lambda_build_command
# really runs: the Go toolchain check, mkdir .build/<env>, go build and
# go run ./tools/mkzip. It makes no AWS call. A plan never runs provisioners.
override_resource {
  target = null_resource.build_ci_lambda
}

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
# Autoscaling on: the configured count is the starting size only.
# ---------------------------------------------------------------------------
run "autoscaling_on_service_runs_at_the_configured_count" {
  command = apply

  variables {
    backend_autoscaling_enabled = true
    backend_desired_count       = 2
  }

  assert {
    condition     = aws_ecs_service.backend.desired_count == 2
    error_message = "The backend service must be at backend_desired_count (2); got ${aws_ecs_service.backend.desired_count}."
  }
}

run "autoscaling_on_a_changed_count_does_not_reset_the_service" {
  command = plan

  variables {
    backend_autoscaling_enabled = true
    backend_desired_count       = 5
  }

  assert {
    condition     = aws_ecs_service.backend.desired_count == 2
    error_message = "With autoscaling on, Application Auto Scaling owns the live count: the plan must keep the service's existing desired_count (2), not set it to backend_desired_count (5). Got ${aws_ecs_service.backend.desired_count}."
  }
}

run "autoscaling_on_has_a_scalable_target_and_no_count_setter" {
  command = plan

  variables {
    backend_autoscaling_enabled = true
    backend_desired_count       = 2
  }

  assert {
    condition     = length(aws_appautoscaling_target.backend) == 1
    error_message = "With autoscaling on there must be one aws_appautoscaling_target.backend."
  }

  assert {
    condition     = length(terraform_data.backend_desired_count) == 0
    error_message = "With autoscaling on nothing may apply backend_desired_count to the service: terraform_data.backend_desired_count must have no instance."
  }
}

# ---------------------------------------------------------------------------
# Autoscaling off: the configured count is still applied.
# ---------------------------------------------------------------------------
run "autoscaling_off_applies_the_configured_count" {
  command = plan

  variables {
    backend_autoscaling_enabled = false
    backend_desired_count       = 3
  }

  assert {
    condition     = length(aws_appautoscaling_target.backend) == 0
    error_message = "With autoscaling off there must be no aws_appautoscaling_target.backend."
  }

  assert {
    condition     = length(terraform_data.backend_desired_count) == 1
    error_message = "With autoscaling off terraform_data.backend_desired_count must have one instance, which applies backend_desired_count to the service."
  }

  assert {
    condition     = terraform_data.backend_desired_count[0].triggers_replace.desired_count == 3
    error_message = "terraform_data.backend_desired_count must be keyed on backend_desired_count (3), so a changed count is applied."
  }
}
