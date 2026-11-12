# The bucket name, through the shared cascade. See modules/naming.
#
# S3 bucket names are global across every AWS account, so the account ID is part
# of the name. It is the suffix, which the cascade counts against the 63-character
# budget and never truncates: form 3 cuts "data" before it drops the account.
# A new resource, so there is no legacy form.

data "aws_caller_identity" "current" {}

module "naming" {
  source  = "../naming"
  project = var.project
  env     = var.env

  requests = {
    data_bucket = {
      parts  = ["data"]
      limit  = 63
      suffix = "-${data.aws_caller_identity.current.account_id}"
    }
  }
}
