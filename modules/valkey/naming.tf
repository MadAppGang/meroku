# The cache name, through the shared cascade. See modules/naming.
#
# CreateServerlessCache takes at most 40 characters of lower-case letters, digits
# and hyphens, so the name is built with "-" and then lower-cased with every "_"
# turned into "-": a project or env slug may hold either. A new resource, so there
# is no legacy form.

module "naming" {
  source  = "../naming"
  project = var.project
  env     = var.env

  requests = {
    cache = {
      parts = ["valkey"]
      limit = 40
    }
  }
}

locals {
  cache_name = lower(replace(module.naming.names["cache"], "_", "-"))
}
