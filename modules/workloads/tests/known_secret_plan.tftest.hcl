# Known ENV collisions must fail before deferred own-path discovery.
mock_provider "aws" {
  mock_data "aws_iam_policy_document" {
    defaults = { json = "{\"Version\":\"2012-10-17\",\"Statement\":[]}" }
  }
  mock_data "aws_caller_identity" {
    defaults = { account_id = "000000000000" }
  }
  mock_data "aws_region" {
    defaults = { region = "us-east-1", name = "us-east-1" }
  }
  mock_data "aws_organizations_organization" {
    defaults = { id = "o-000000000000" }
  }
  mock_data "aws_ssm_parameters_by_path" {
    defaults = { names = [] }
  }
  mock_resource "aws_iam_role" {
    defaults = { arn = "arn:aws:iam::000000000000:role/mock" }
  }
  mock_resource "aws_iam_policy" {
    defaults = { arn = "arn:aws:iam::000000000000:policy/mock" }
  }
  mock_resource "aws_ecs_task_definition" {
    defaults = {
      arn                  = "arn:aws:ecs:us-east-1:000000000000:task-definition/mock:1"
      arn_without_revision = "arn:aws:ecs:us-east-1:000000000000:task-definition/mock"
    }
  }
  mock_data "aws_vpc" {
    defaults = { cidr_block = "10.0.0.0/16" }
  }
  mock_resource "aws_cloudwatch_log_group" {
    defaults = { arn = "arn:aws:logs:us-east-1:000000000000:log-group:mock" }
  }
  mock_resource "aws_service_discovery_service" {
    defaults = { arn = "arn:aws:servicediscovery:us-east-1:000000000000:service/srv-mock" }
  }
  mock_resource "aws_lambda_function" {
    defaults = { arn = "arn:aws:lambda:us-east-1:000000000000:function:mock" }
  }
  mock_resource "aws_cloudwatch_event_rule" {
    defaults = { arn = "arn:aws:events:us-east-1:000000000000:rule/mock" }
  }
}
mock_provider "null" {}
mock_provider "random" {}
override_resource {
  target = null_resource.build_ci_lambda
}

variables {
  project          = "myapp"
  env              = "dev"
  vpc_id           = "vpc-00000000000000000"
  subnet_ids       = ["subnet-00000000000000001", "subnet-00000000000000002"]
  private_dns_name = "myapp.internal"
  api_domain       = "api.example.com"
  github_subjects  = ["repo:myapp/app:ref:refs/heads/main"]
  docker_image     = "example.com/myapp/backend:synthetic"
  services         = [{ name = "api", docker_image = "example.com/myapp/api:synthetic", env_vars = {} }]
}

run "backend_vault_env_fails_before_deferred_discovery" {
  command = plan

  variables {
    extra_secrets = {
      backend = [{
        name      = "ENV"
        valueFrom = "arn:aws:ssm:us-east-1:000000000000:parameter/dev/myapp/vault/shared/env"
        origin    = "/dev/myapp/vault/shared/env"
      }]
    }
  }

  # This read remains deferred by the fresh backend's own /env resource.
  # The mock supplies its eventual value, not a plan-time override.
  expect_failures = [aws_ecs_task_definition.backend]
}

run "service_vault_env_fails_before_deferred_discovery" {
  command = plan

  variables {
    extra_secrets = {
      api = [{
        name      = "ENV"
        valueFrom = "arn:aws:ssm:us-east-1:000000000000:parameter/dev/myapp/vault/shared/env"
        origin    = "/dev/myapp/vault/shared/env"
      }]
    }
  }

  expect_failures = [aws_ecs_task_definition.services["api"]]
}
