# This module had no versions.tf until EC2 compute pools landed, which meant
# `terraform init -backend=false` in CI (.github/workflows/ci.yml:112-118)
# resolved the newest hashicorp/aws — 6.x — while every generated environment
# pins `~> 5.0` (env/main.hbs:9-14). CI was validating a provider no environment
# actually runs.
#
# That stops being cosmetic here:
#   * aws_ecs_capacity_provider's `managed_draining` needs provider >= 5.34.0
#     (PR #35421, milestone v5.34.0). An environment on an older lock file fails
#     `terraform plan` with `Unsupported argument` while CI stays green.
#   * The whole launch_type / capacity_provider_strategy ForceNew analysis this
#     feature is shaped around — including `force_new_deployment` — describes
#     5.x behaviour. Validating under 6.x exercises the wrong version.
#
# The ceiling is deliberate and is not a bump: `>= 5.34.0, < 6.0.0` is a subset
# of the `~> 5.0` the rest of the repository already pins.
#
# required_version matches env/main.hbs:15. `precondition` needs >= 1.2.
terraform {
  required_version = ">= 1.2.6"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.34.0, < 6.0.0"
    }
  }
}
