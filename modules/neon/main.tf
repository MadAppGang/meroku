# A Neon project and the databases on its default branch.
#
# Neon data is never deleted from here. The project and every database this
# module creates carry a literal prevent_destroy, so any plan that would destroy
# or replace them fails. To stop managing one, remove its address from state;
# the data stays in Neon.
#
# Only the project's default branch is used. It is read, never created or
# changed.

locals {
  create_project = var.mode == "create"

  # Databases with create = false already exist and are used as they are. The
  # provider has no database data source; their existence is checked against
  # the Neon API before Terraform runs.
  created_databases = toset([for name, db in var.databases : name if db.create])

  # Reuse mode: the branch Neon marks primary is the default branch, and its
  # read_write endpoint is the host every connection goes to.
  reuse_default_branch_id = local.create_project ? null : one([
    for b in data.neon_branches.all[0].branches : b.id if b.primary
  ])
  reuse_host = local.create_project ? null : one([
    for e in data.neon_branch_endpoints.default[0].endpoints : e.host if e.type == "read_write"
  ])

  project_id        = local.create_project ? neon_project.this[0].id : var.project_id
  default_branch_id = local.create_project ? neon_project.this[0].default_branch_id : local.reuse_default_branch_id
  host              = local.create_project ? neon_project.this[0].database_host : local.reuse_host

  # The pooled (PgBouncer) host is the endpoint host with "-pooler" after its
  # first label: ep-x-123.region.aws.neon.tech -> ep-x-123-pooler.region.aws.neon.tech.
  pooler_host = local.create_project ? neon_project.this[0].database_host_pooler : replace(local.reuse_host, "/^([^.]+)\\./", "$${1}-pooler.")
}

resource "neon_project" "this" {
  count = local.create_project ? 1 : 0

  name       = var.project.name
  org_id     = var.project.org_id
  region_id  = var.project.region_id
  pg_version = var.project.pg_version

  branch {
    name      = "main"
    role_name = var.owner_role
  }

  lifecycle {
    prevent_destroy = true
  }
}

data "neon_branches" "all" {
  count = local.create_project ? 0 : 1

  project_id = var.project_id

  lifecycle {
    postcondition {
      condition     = length([for b in self.branches : b.id if b.primary]) == 1
      error_message = "Neon project ${var.project_id} must have exactly one default (primary) branch."
    }
  }
}

data "neon_branch_endpoints" "default" {
  count = local.create_project ? 0 : 1

  project_id = var.project_id
  branch_id  = local.reuse_default_branch_id

  lifecycle {
    postcondition {
      condition     = length([for e in self.endpoints : e.id if e.type == "read_write"]) == 1
      error_message = "The default branch of Neon project ${var.project_id} must have exactly one read_write endpoint."
    }
  }
}

resource "neon_database" "created" {
  for_each = local.created_databases

  project_id = local.project_id
  branch_id  = local.default_branch_id
  name       = each.key
  owner_name = var.owner_role

  lifecycle {
    prevent_destroy = true
  }
}
