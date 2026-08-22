# Pin the AWS provider to the major every generated environment actually runs.
#
# Without this, `terraform init -backend=false` — in CI
# (.github/workflows/ci.yml) and in any standalone validation of this module —
# resolves the newest hashicorp/aws, currently 6.x, while every generated
# environment pins `~> 5.0` (env/main.hbs). CI then validates a provider no
# environment runs.
#
# That is not cosmetic. It is how `data.aws_region.current.name` came to look
# deprecated across this repository: `.name` is deprecated in 6.x and correct
# in 5.x, and 5.x has no `.region` to move to — `try()` cannot bridge the two,
# because an attribute the provider schema does not define fails at validate
# time rather than at evaluation. Pinning the major is what makes the warning
# go away honestly, instead of rewriting working 5.x code to match a provider
# nothing here uses.
#
# modules/workloads carries a narrower floor (>= 5.34.0) for a reason specific
# to EC2 capacity providers; this is the plain repository-wide constraint, and
# `~> 5.0` matches env/main.hbs exactly.
terraform {
  required_version = ">= 1.2.6"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}
