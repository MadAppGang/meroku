# Compute pool capacity: the provider's ASG capacity wait is off, and
# terraform_data.pool_capacity (scripts/asg-capacity-wait.sh) waits instead.
# Plans only, so no provisioner runs; the script itself is tested by
# scripts/test/run.sh against a fake AWS CLI.
#
# Run: terraform test -filter=tests/pool_capacity.tftest.hcl  (from modules/workloads)

mock_provider "aws" {
  # Same defaults as tests/env_secret_collision.tftest.hcl, for the same reason:
  # the provider validates policy JSON, CIDRs and ARNs client-side. Account
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

  compute_pools = [
    { name = "general", instance_types = ["t3.small"] },
    { name = "burst", instance_types = ["t3.small"], min_size = 0 },
    { name = "big", instance_types = ["m6i.large"], min_size = 2, max_size = 4 },
  ]

  services = [
    { name = "bridge", runtime = "ec2", compute_pool = "general", network_access = "internal", docker_image = "public.ecr.aws/nginx/nginx:stable" },
  ]
}

# The provider's capacity wait is what failed the deploy: it ends the apply on
# the first failed scaling activity and taints the group. It must be off on
# every pool, the min_size 0 one included.
run "the_provider_capacity_wait_is_off" {
  command = plan

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition     = alltrue([for k, g in aws_autoscaling_group.pool : g.wait_for_capacity_timeout == "0"])
    error_message = "Every pool ASG must set wait_for_capacity_timeout = \"0\"; the provider's wait fails the apply on a temporary launch failure."
  }
}

# The script waits instead, for each pool that has a minimum to reach.
run "each_pool_with_a_minimum_gets_a_capacity_wait" {
  command = plan

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition     = sort(keys(terraform_data.pool_capacity)) == tolist(["big", "general"])
    error_message = "Pools with min_size > 0 must get a capacity wait and a min_size 0 pool none. Got: ${jsonencode(keys(terraform_data.pool_capacity))}"
  }

  assert {
    condition     = terraform_data.pool_capacity["big"].triggers_replace.min_size == 2
    error_message = "The wait must re-run when min_size changes, so min_size must be one of its triggers."
  }
}

run "no_pools_means_no_capacity_wait" {
  command = plan

  variables {
    compute_pools = []
    services      = [{ name = "web", docker_image = "public.ecr.aws/nginx/nginx:stable" }]
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition     = length(terraform_data.pool_capacity) == 0
    error_message = "An environment without pools must plan no capacity wait."
  }
}
