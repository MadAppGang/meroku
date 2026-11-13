# First-plan secrets and the actual task registration guards.
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
}

variables {
  project          = "myapp"
  env              = "dev"
  task             = "nightly"
  vpc_id           = "vpc-00000000000000000"
  subnet_ids       = ["subnet-00000000000000001", "subnet-00000000000000002"]
  cluster          = "arn:aws:ecs:us-east-1:000000000000:cluster/myapp_cluster_dev"
  docker_image     = "example.com/myapp/task:synthetic"
  api_domain       = "api.example.com"
  private_dns_name = "myapp.internal"
}

run "empty_discovery_still_supplies_env" {
  command = plan

  override_data {
    target = data.aws_ssm_parameters_by_path.task
    values = { names = [] }
  }

  assert {
    condition = (
      [for secret in jsondecode(aws_ecs_task_definition.task.container_definitions)[0].secrets : secret.name] == ["ENV"] &&
      contains([
        "/dev/myapp/task/nightly/env",
        "arn:aws:ssm:us-east-1:000000000000:parameter/dev/myapp/task/nightly/env"
      ], one(jsondecode(aws_ecs_task_definition.task.container_definitions)[0].secrets).valueFrom)
    )
    error_message = "A fresh task must have exactly one ENV secret pointing at its own /env, even when discovery returns no names."
  }
}

run "discovered_env_is_not_duplicated" {
  command = plan

  override_data {
    target = data.aws_ssm_parameters_by_path.task
    values = { names = ["/dev/myapp/task/nightly/database_password", "/dev/myapp/task/nightly/env"] }
  }

  assert {
    condition     = [for secret in jsondecode(aws_ecs_task_definition.task.container_definitions)[0].secrets : secret.name] == ["DATABASE_PASSWORD", "ENV"]
    error_message = "Appending the configured /env must not duplicate a discovered ENV or reorder the existing discovered list."
  }
}

run "vault_env_is_rejected_on_a_fresh_task" {
  command = plan

  variables {
    extra_secrets = [{
      name      = "ENV"
      valueFrom = "arn:aws:ssm:us-east-1:000000000000:parameter/dev/myapp/vault/shared/env"
      origin    = "/dev/myapp/vault/shared/env"
    }]
  }

  override_data {
    target = data.aws_ssm_parameters_by_path.task
    values = { names = [] }
  }

  expect_failures = [aws_ecs_task_definition.task]
}

run "prefixed_vault_env_is_allowed" {
  command = plan

  variables {
    extra_secrets = [{
      name      = "PAY_ENV"
      valueFrom = "arn:aws:ssm:us-east-1:000000000000:parameter/dev/myapp/vault/payments/env"
      origin    = "/dev/myapp/vault/payments/env"
    }]
  }

  assert {
    condition = (
      [for secret in jsondecode(aws_ecs_task_definition.task.container_definitions)[0].secrets : secret.name] == ["ENV", "PAY_ENV"] &&
      one([for secret in jsondecode(aws_ecs_task_definition.task.container_definitions)[0].secrets : secret.valueFrom if secret.name == "PAY_ENV"]) ==
      "arn:aws:ssm:us-east-1:000000000000:parameter/dev/myapp/vault/payments/env" &&
      alltrue([
        for secret in jsondecode(aws_ecs_task_definition.task.container_definitions)[0].secrets :
        toset(keys(secret)) == toset(["name", "valueFrom"])
      ])
    )
    error_message = "PAY_ENV must be accepted beside ENV and origin metadata must be removed before ECS receives the secret definition."
  }
}

run "duplicate_extra_environment_fails_the_plan" {
  command = plan

  variables {
    custom_env_vars = [{ name = "VALKEY_URL", value = "rediss://yaml.example.com:6379" }]
    extra_env       = [{ name = "VALKEY_URL", value = "rediss://cache.example.com:6379" }]
  }

  expect_failures = [aws_ecs_task_definition.task]
}

run "yaml_override_of_an_ordinary_default_is_allowed" {
  command = plan

  variables {
    sqs_queue_url   = "https://sqs.us-east-1.amazonaws.com/000000000000/default"
    custom_env_vars = [{ name = "SQS_QUEUE_URL", value = "https://sqs.us-east-1.amazonaws.com/000000000000/custom" }]
  }

  assert {
    condition = [
      for e in jsondecode(aws_ecs_task_definition.task.container_definitions)[0].environment : e.value if e.name == "SQS_QUEUE_URL"
      ] == [
      "https://sqs.us-east-1.amazonaws.com/000000000000/default",
      "https://sqs.us-east-1.amazonaws.com/000000000000/custom"
    ]
    error_message = "An ordinary default/YAML duplicate must remain allowed and the YAML override must follow the default."
  }
}
