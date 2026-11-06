# Pin the AWS provider to the major every generated environment runs.
#
# Without a pin, `terraform init -backend=false` in CI (.github/workflows/ci.yml)
# and in any standalone validation of this module resolves the newest
# hashicorp/aws, which may be a major no environment runs, and CI then validates
# the wrong provider. Keep this constraint equal to env/main.hbs.
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
