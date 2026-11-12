# Every capped AWS name this module builds, in one place. See modules/naming.
#
# The service role is the only name here with a tight cap (IAM roles, 64). The
# CodeBuild project (255), log groups (512), security group (255) and inline
# policy (128) stay literal templates, per the cutoff in CLAUDE.md.
module "naming" {
  source  = "../naming"
  project = var.project
  env     = var.env

  requests = {
    service_role = {
      parts     = ["gh", "runners"]
      limit     = 64
      separator = "_"
    }
  }
}
