# container_command overrides the image's CMD. Unset, the container definition
# carries no `command` key and the image's own CMD runs.
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
  task             = "consumer"
  vpc_id           = "vpc-00000000000000000"
  subnet_ids       = ["subnet-00000000000000001", "subnet-00000000000000002"]
  cluster          = "arn:aws:ecs:us-east-1:000000000000:cluster/myapp_cluster_dev"
  docker_image     = "example.com/myapp/task:synthetic"
  api_domain       = "api.example.com"
  private_dns_name = "myapp.internal"
  rules            = { orders = { sources = ["myapp.orders"], detail_types = ["order.created"] } }
}

run "container_command_sets_the_container_command" {
  command = plan

  variables {
    container_command = ["npm", "run", "consume"]
  }

  assert {
    condition     = jsondecode(aws_ecs_task_definition.task.container_definitions)[0].command == ["npm", "run", "consume"]
    error_message = "container_command must become the container definition's command."
  }
}

run "no_container_command_leaves_the_image_cmd" {
  command = plan

  assert {
    condition     = !contains(keys(jsondecode(aws_ecs_task_definition.task.container_definitions)[0]), "command")
    error_message = "Without container_command the container definition must carry no command, so the image's CMD runs."
  }
}
