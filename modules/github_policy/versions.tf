# This module declares the AWS provider and still never reaches the network.
# `aws_iam_policy_document` is a CLIENT-SIDE renderer: the provider walks the
# statement blocks and emits JSON locally, with no API call and no credentials.
# That is the property the whole extraction rests on — a `terraform test` here
# can plan the real document and read the real string, which is impossible from
# modules/workloads/tests/ (see main.tf's header for why).
#
# The version constraint is modules/workloads/versions.tf's, character for
# character, and deliberately so. CI inits this directory separately from its
# consumer, so an unpinned child would resolve the newest hashicorp/aws, and
# every assertion here could describe a provider no generated environment runs
# (env/main.hbs pins the same `~> 6.0`).
#
# required_version: nothing here needs a construct newer than Terraform 1.2.
terraform {
  required_version = ">= 1.2.6"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}
