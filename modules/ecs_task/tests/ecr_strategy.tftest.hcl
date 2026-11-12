# The task's ECR repository follows ecr_strategy, whatever the environment is called.

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

  mock_data "aws_organizations_organization" {
    defaults = {
      id = "o-000000000000"
    }
  }

  mock_data "aws_caller_identity" {
    defaults = {
      account_id = "000000000000"
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

  # aws_scheduler_schedule reads task_definition_arn off this resource, and the
  # scheduler API takes an ARN without a revision — hence both attributes here,
  # not just `arn` as in modules/event_bridge_task, which has no scheduler.
  mock_resource "aws_ecs_task_definition" {
    defaults = {
      arn                  = "arn:aws:ecs:us-east-1:000000000000:task-definition/mock:1"
      arn_without_revision = "arn:aws:ecs:us-east-1:000000000000:task-definition/mock"
    }
  }
}


variables {
  project    = "acme"
  env        = "dev"
  task       = "cleanup"
  vpc_id     = "vpc-00000000000000000"
  subnet_ids = ["subnet-00000000000000001", "subnet-00000000000000002"]
  cluster    = "arn:aws:ecs:us-east-1:000000000000:cluster/acme_cluster_dev"

  docker_image = ""

  api_domain       = "api.example.com"
  private_dns_name = "acme_dev.private"

}

run "local_strategy_creates_the_repository_under_any_name" {
  command = plan

  variables {
    env          = "sandbox"
    ecr_strategy = "local"
  }

  override_data {
    target = data.aws_ssm_parameters_by_path.task
    values = {
      names = ["/sandbox/acme/task/cleanup/env"]
    }
  }

  assert {
    condition     = length(aws_ecr_repository.task) == 1
    error_message = "ecr_strategy = local must create the repository in an environment not named dev."
  }
}

run "cross_account_strategy_creates_no_repository_and_uses_the_url" {
  command = plan

  variables {
    env          = "dev"
    ecr_strategy = "cross_account"
    ecr_url      = "000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_task_cleanup"
  }

  override_data {
    target = data.aws_ssm_parameters_by_path.task
    values = {
      names = ["/dev/acme/task/cleanup/env"]
    }
  }

  assert {
    condition     = length(aws_ecr_repository.task) == 0
    error_message = "ecr_strategy = cross_account must not create a repository, even in dev."
  }

  assert {
    condition     = local.docker_image == "000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_task_cleanup:latest"
    error_message = "The task must pull from var.ecr_url."
  }
}

run "trusted_accounts_get_their_own_policy_document" {
  command = plan

  variables {
    ecr_strategy         = "local"
    ecr_trusted_accounts = ["111111111111"]
  }

  override_data {
    target = data.aws_ssm_parameters_by_path.task
    values = {
      names = ["/dev/acme/task/cleanup/env"]
    }
  }

  assert {
    condition     = length(data.aws_iam_policy_document.trusted_ecr_policy) == 1
    error_message = "A trusted account must add the pull policy."
  }
}
