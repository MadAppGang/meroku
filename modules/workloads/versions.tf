# Pin the AWS provider to the major every generated environment runs (env/main.hbs
# pins the same constraint). Without a pin, `terraform init -backend=false` in CI
# (.github/workflows/ci.yml) resolves the newest hashicorp/aws, which may be a
# major no environment runs, and CI then validates the wrong provider.
#
# The floor is 6.47.0: aws_xray_trace_segment_destination and
# aws_xray_indexing_rule (xray_transaction_search.tf) are new in that release.
# aws_ecs_capacity_provider's `managed_draining` needs provider >= 5.34.0,
# which every 6.x release satisfies.
#
# required_version: `optional()` in an object type constraint needs Terraform
# 1.3; `precondition` needs 1.2.
terraform {
  required_version = ">= 1.3.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.47"
    }
  }
}
