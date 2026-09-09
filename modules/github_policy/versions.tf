# This module declares the AWS provider and still never reaches the network.
# `aws_iam_policy_document` is a CLIENT-SIDE renderer: the provider walks the
# statement blocks and emits JSON locally, with no API call and no credentials.
# That is the property the whole extraction rests on — a `terraform test` here
# can plan the real document and read the real string, which is impossible from
# modules/workloads/tests/ (see main.tf's header for why).
#
# The version window is modules/workloads/versions.tf's window, character for
# character, and deliberately so. CI inits this directory separately from its
# consumer, so an unpinned child would resolve the newest hashicorp/aws — 6.x —
# and every assertion here would describe a provider no generated environment
# runs (env/main.hbs:9-14 pins `~> 5.0`). Nothing below needs anything newer
# than 5.0; the floor is inherited so the two directories cannot drift.
#
# required_version stays at the repository's usual 1.2.6: nothing here uses
# `optional()` or any other 1.3 construct, so there is no reason to raise it.
terraform {
  required_version = ">= 1.2.6"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.34.0, < 6.0.0"
    }
  }
}
