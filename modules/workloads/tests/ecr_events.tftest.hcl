# Connecting image-push events between environments that share a registry.

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

run "a_trusted_account_with_the_flag_gets_a_forwarding_rule" {
  command = apply

  override_resource {
    target = null_resource.build_ci_lambda
  }

  override_data {
    target = data.aws_caller_identity.current
    values = {
      account_id = "000000000000"
    }
  }

  override_data {
    target = data.aws_region.current
    values = {
      region = "us-east-1"
    }
  }

  variables {
    ecr_trusted_accounts = [
      { account_id = "111111111111", env = "sandbox", region = "us-east-1", forward_image_events = true },
      { account_id = "111111111111", env = "qa", region = "us-east-1", forward_image_events = true },
      { account_id = "222222222222", env = "prod", region = "us-east-1" },
    ]
  }

  assert {
    condition     = keys(aws_cloudwatch_event_rule.ecr_forward) == ["111111111111-us-east-1"]
    error_message = "One rule per flagged account and region, none for the unflagged account."
  }

  assert {
    condition     = aws_cloudwatch_event_target.ecr_forward["111111111111-us-east-1"].arn == "arn:aws:events:us-east-1:111111111111:event-bus/default"
    error_message = "The target must be the destination's default event bus."
  }
}

run "a_destination_on_the_same_bus_needs_no_forwarding" {
  command = apply

  override_resource {
    target = null_resource.build_ci_lambda
  }

  override_data {
    target = data.aws_caller_identity.current
    values = {
      account_id = "000000000000"
    }
  }

  override_data {
    target = data.aws_region.current
    values = {
      region = "us-east-1"
    }
  }

  variables {
    ecr_trusted_accounts = [
      { account_id = "000000000000", env = "sandbox", region = "us-east-1", forward_image_events = true },
    ]
  }

  assert {
    condition     = length(aws_cloudwatch_event_rule.ecr_forward) == 0 && length(aws_iam_role.ecr_forward) == 0
    error_message = "The same account and region already share the bus."
  }
}

run "a_connected_environment_accepts_events_and_maps_the_repositories" {
  command = apply

  override_resource {
    target = null_resource.build_ci_lambda
  }

  override_data {
    target = data.aws_caller_identity.current
    values = {
      account_id = "000000000000"
    }
  }

  override_data {
    target = data.aws_region.current
    values = {
      region = "us-east-1"
    }
  }

  variables {
    env                  = "sandbox"
    ecr_strategy         = "cross_account"
    ecr_account_id       = "111111111111"
    ecr_account_region   = "us-east-1"
    ecr_url              = "111111111111.dkr.ecr.us-east-1.amazonaws.com/acme_backend"
    ecr_image_events     = true
    scheduled_task_names = ["cleanup"]
  }

  assert {
    condition     = aws_cloudwatch_event_permission.ecr_source[0].principal == "111111111111"
    error_message = "The source account must be allowed to put events on this bus."
  }

  assert {
    condition     = contains(keys(module.ci_identifiers.ecr_repo_ids), "acme_backend") && contains(keys(module.ci_identifiers.ecr_repo_ids), "acme_task_cleanup")
    error_message = "The source registry's repositories must be mapped to this environment's targets."
  }

  assert {
    condition     = length(aws_cloudwatch_event_rule.ci_ecr_push) == 1
    error_message = "The ordinary push rule must exist to receive the forwarded events."
  }
}

run "without_the_flag_a_cross_account_environment_hears_nothing" {
  command = apply

  override_resource {
    target = null_resource.build_ci_lambda
  }

  override_data {
    target = data.aws_caller_identity.current
    values = {
      account_id = "000000000000"
    }
  }

  override_data {
    target = data.aws_region.current
    values = {
      region = "us-east-1"
    }
  }

  variables {
    env                = "sandbox"
    ecr_strategy       = "cross_account"
    ecr_account_id     = "111111111111"
    ecr_account_region = "us-east-1"
    ecr_url            = "111111111111.dkr.ecr.us-east-1.amazonaws.com/acme_backend"
  }

  assert {
    condition     = length(aws_cloudwatch_event_permission.ecr_source) == 0 && length(aws_cloudwatch_event_rule.ci_ecr_push) == 0
    error_message = "With ecr_image_events off nothing is connected."
  }
}

run "cross_account_services_pull_from_the_source_registry" {
  command = apply

  override_resource {
    target = null_resource.build_ci_lambda
  }

  variables {
    env                = "sandbox"
    ecr_strategy       = "cross_account"
    ecr_account_id     = "111111111111"
    ecr_account_region = "us-east-1"
    ecr_url            = "111111111111.dkr.ecr.us-east-1.amazonaws.com/acme_backend"
    ecr_image_events   = true
    services = [
      { name = "api", ecr_config = { mode = "create_ecr" } },
      { name = "worker" },
      { name = "legacy", ecr_config = { mode = "manual_repo", repository_uri = "registry.example.com/team/legacy:v1" } },
    ]
  }

  assert {
    condition     = length(aws_ecr_repository.services) == 0
    error_message = "A cross_account environment must not create service repositories."
  }

  assert {
    condition = (
      local.service_ecr_urls["api"] == "111111111111.dkr.ecr.us-east-1.amazonaws.com/acme_service_api" &&
      local.service_ecr_urls["worker"] == "111111111111.dkr.ecr.us-east-1.amazonaws.com/acme_service_worker" &&
      local.service_ecr_urls["legacy"] == "registry.example.com/team/legacy:v1"
    )
    error_message = "Services must pull from the source registry, and manual_repo must keep its URI."
  }

  assert {
    condition     = contains(keys(module.ci_identifiers.ecr_repo_ids), "acme_service_api")
    error_message = "A connected environment must map the shared service repository."
  }
}
