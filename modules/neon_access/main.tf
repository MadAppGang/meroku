# Service access to one Neon database: a LOGIN role per service, the schemas
# the services list, and the grants that let each role read or write them.
#
# The configured postgresql provider connects to this database as the Neon
# owner role (var.admin_role). That role owns every table, sequence and schema
# here and issues every grant. A service role owns nothing: it has no CREATE on
# any schema, so it is dropped with a plain DROP ROLE once its grants are gone.

locals {
  # Every distinct schema a service lists, other than public. A missing one is
  # created owned by the connecting owner role; an existing one is left as is.
  created_schemas = toset(distinct(flatten([
    for s in var.services : [for schema in s.schemas : schema if schema != "public"]
  ])))

  # One entry per (service, listed schema).
  service_schemas = merge([
    for svc, s in var.services : {
      for schema in distinct(s.schemas) : "${svc}|${schema}" => {
        svc    = svc
        schema = schema
        access = s.access
      }
    }
  ]...)

  # Data access only, no DDL. Sequence USAGE and UPDATE let a readwrite role
  # call nextval and setval on serial and identity columns.
  table_privileges = {
    readonly  = ["SELECT"]
    readwrite = ["SELECT", "INSERT", "UPDATE", "DELETE"]
  }
  sequence_privileges = {
    readonly  = ["SELECT"]
    readwrite = ["USAGE", "SELECT", "UPDATE"]
  }

  # Grants on existing objects, keyed "<svc>|database" and
  # "<svc>|<schema>|schema|table|sequence". objects = [] means every object
  # of that type in the schema.
  grants = merge(
    {
      for svc, s in var.services : "${svc}|database" => {
        svc         = svc
        object_type = "database"
        schema      = null
        privileges  = ["CONNECT"]
      }
    },
    {
      for key, ss in local.service_schemas : "${key}|schema" => {
        svc         = ss.svc
        object_type = "schema"
        schema      = ss.schema
        privileges  = ["USAGE"]
      }
    },
    {
      for key, ss in local.service_schemas : "${key}|table" => {
        svc         = ss.svc
        object_type = "table"
        schema      = ss.schema
        privileges  = local.table_privileges[ss.access]
      }
    },
    {
      for key, ss in local.service_schemas : "${key}|sequence" => {
        svc         = ss.svc
        object_type = "sequence"
        schema      = ss.schema
        privileges  = local.sequence_privileges[ss.access]
      }
    },
  )

  # The same table and sequence privileges for objects the owner role creates
  # later, keyed "<svc>|<schema>|table|default" and "<svc>|<schema>|sequence|default".
  default_privileges = merge(
    {
      for key, ss in local.service_schemas : "${key}|table|default" => {
        svc         = ss.svc
        object_type = "table"
        schema      = ss.schema
        privileges  = local.table_privileges[ss.access]
      }
    },
    {
      for key, ss in local.service_schemas : "${key}|sequence|default" => {
        svc         = ss.svc
        object_type = "sequence"
        schema      = ss.schema
        privileges  = local.sequence_privileges[ss.access]
      }
    },
  )

  role_names = { for svc, role in postgresql_role.svc : svc => role.name }

  # The pooled host for every service. No options parameter: the role's
  # search_path applies through the pooler.
  database_urls = {
    for svc, role in postgresql_role.svc :
    svc => "postgresql://${role.name}:${urlencode(random_password.svc[svc].result)}@${var.pooler_host}/${var.database}?sslmode=require"
  }
}

# A new password_version replaces the password: the role gets it and the
# service URL changes, while the role keeps its name and address.
resource "random_password" "svc" {
  for_each = var.services

  length  = 32
  special = false

  keepers = {
    version = tostring(each.value.password_version)
  }
}

resource "postgresql_role" "svc" {
  for_each = var.services

  name  = each.value.role_name
  login = true

  # Write-only: the password never enters this resource's state. A change of
  # password_wo_version is what makes the provider send it again.
  password_wo         = random_password.svc[each.key].result
  password_wo_version = tostring(each.value.password_version)

  # Always set, so a change back to ["public"] also updates the role.
  search_path = each.value.schemas

  # The role owns nothing, so there is nothing to reassign before DROP ROLE.
  skip_reassign_owned = true
}

# On a database meroku created, PUBLIC loses its default CONNECT and TEMPORARY.
# The owner role keeps access as the database owner; each service role gets
# CONNECT below. A reused database is never changed here.
resource "postgresql_grant" "public_database" {
  count = var.revoke_public ? 1 : 0

  database    = var.database
  role        = "public"
  object_type = "database"
  privileges  = []
}

# Schemas are never dropped: a schema that leaves config is removed from state
# before the plan. An existing schema keeps its owner.
resource "postgresql_schema" "this" {
  for_each = local.created_schemas

  database      = var.database
  name          = each.key
  if_not_exists = true
  drop_cascade  = false

  lifecycle {
    ignore_changes = [owner]
  }
}

resource "postgresql_grant" "svc" {
  for_each = local.grants

  database    = var.database
  role        = postgresql_role.svc[each.value.svc].name
  object_type = each.value.object_type
  schema      = each.value.schema
  objects     = each.value.object_type == "database" || each.value.object_type == "schema" ? null : []
  privileges  = each.value.privileges

  depends_on = [postgresql_schema.this]
}

resource "postgresql_default_privileges" "svc" {
  for_each = local.default_privileges

  database    = var.database
  owner       = var.admin_role
  role        = postgresql_role.svc[each.value.svc].name
  object_type = each.value.object_type
  schema      = each.value.schema
  privileges  = each.value.privileges

  depends_on = [postgresql_schema.this]
}

# What this connection is, for the tooling that reads state: which project,
# branch and database, which admin role connects, and the role of each
# service. No secrets.
resource "terraform_data" "connection" {
  input = {
    key        = var.database
    project_id = var.project_id
    branch_id  = var.branch_id
    database   = var.database
    admin_role = var.admin_role
    roles      = local.role_names
  }
}
