# Every AWS name this module builds, in one place. See modules/naming.
#
# S3 bucket names are capped at 63 characters and are global, not per-account,
# so the digest in form 3 is doing more work here than elsewhere: it is what
# keeps two projects that both want "uploads" from claiming the same bucket.
module "naming" {
  source  = "../naming"
  project = var.project
  env     = var.env

  requests = { for name, bucket in local.bucket_names : name => {
    legacy = "${var.project}-${name}-${var.env}"
    parts  = [name]
    limit  = 63
  } }
}
