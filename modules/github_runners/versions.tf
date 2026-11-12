# Keep the provider constraint equal to env/main.hbs, so a standalone init here
# validates against the provider major every environment runs.
#
# 1.9 is the floor because the vpc_id and subnet_ids validations refer to
# var.vpc_access, and cross-variable validation arrived in 1.9.
terraform {
  required_version = ">= 1.9.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
    github = {
      source  = "integrations/github"
      version = "~> 6.0"
    }
  }
}
