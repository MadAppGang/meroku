# REQ-11, REQ-12, REQ-14 / TEST-12.
# Grant/default-privilege resource labels are absent from the supplied contract;
# exact privilege-set assertions require those labels or public grant outputs.
mock_provider "postgresql" {}
mock_provider "random" {
  mock_resource "random_password" {
    defaults = {
      result = "fake-generated-password-do-not-use"
    }
  }
}

variables {
  env           = "dev"
  project_id    = "fake-project-000000"
  branch_id     = "br-fake-main"
  database      = "app"
  admin_role    = "neondb_owner"
  pooler_host   = "ep-fake-pooler.example.invalid"
  revoke_public = true
  services = {
    backend = {
      schemas          = ["app", "public"]
      access           = "readwrite"
      role_name        = "myapp_dev_backend_app_f0819dbc"
      password_version = 1
    }
    reports = {
      schemas          = ["public"]
      access           = "readonly"
      role_name        = "fake_reports_app_00000000"
      password_version = 1
    }
  }
}

run "roles_plan_with_schema_list_not_one_quoted_identifier" {
  command = plan
  assert {
    condition     = join(",", postgresql_role.svc["backend"].search_path) == "app,public" && join(",", postgresql_role.svc["reports"].search_path) == "public"
    error_message = "search_path must be the ordered list of schemas on the role."
  }
  # An omitted optional superuser attribute is null in a mock plan; it does
  # not request SUPERUSER. Explicit true must still fail this assertion.
  assert {
    condition = alltrue([for svc, role in postgresql_role.svc :
      role.name == var.services[svc].role_name &&
      role.login && !coalesce(role.superuser, false) && role.skip_reassign_owned &&
      role.password == null

    ])
    error_message = "Service roles must be normal LOGIN roles, without a stored password argument and owning nothing."
  }
  # REQ-14: the version is a non-secret trigger, not the write-only password.
  # Normalize its scalar representation; HCL equality does not coerce types.
  assert {
    condition = alltrue([for svc, role in postgresql_role.svc :
      tonumber(role.password_wo_version) == var.services[svc].password_version
    ])
    error_message = "Each role must use its service password-version trigger."
  }
  assert {
    condition = alltrue([for svc, pw in random_password.svc :
      pw.length == 32 && !pw.special && tonumber(pw.keepers.version) == var.services[svc].password_version
    ])
    error_message = "Only an explicit password-version change should rotate the 32-character password."
  }
  assert {
    condition     = join(",", keys(postgresql_schema.this)) == "app" && postgresql_schema.this["app"].if_not_exists && !postgresql_schema.this["app"].drop_cascade
    error_message = "Only non-public schemas are created if missing, without cascading deletion."
  }
  assert {
    condition     = length(postgresql_grant.public_database) == 1 && postgresql_grant.public_database[0].role == "public" && postgresql_grant.public_database[0].object_type == "database" && length(postgresql_grant.public_database[0].privileges) == 0
    error_message = "Created databases must revoke PUBLIC's default database privileges."
  }
}

run "urls_are_pooled_ssl_only_and_do_not_encode_search_path" {
  command = apply
  assert {
    condition     = join(",", keys(output.database_urls)) == "backend,reports" && output.role_names["backend"] == "myapp_dev_backend_app_f0819dbc"
    error_message = "There must be one role and one URL per connected service."
  }
  assert {
    condition = alltrue([for svc, url in output.database_urls :
      startswith(url, "postgresql://${var.services[svc].role_name}:") &&
      endswith(url, "@ep-fake-pooler.example.invalid/app?sslmode=require") &&
      !strcontains(url, "options=") && !strcontains(url, "search_path")
    ])
    error_message = "Service URLs use the pooled host and sslmode=require; schemas live only on the role."
  }
}

run "access_schema_and_password_version_updates_preserve_role_identity" {
  command = plan
  variables {
    services = {
      backend = {
        schemas          = ["public"]
        access           = "readonly"
        role_name        = "myapp_dev_backend_app_f0819dbc"
        password_version = 2
      }
      reports = {
        schemas          = ["public"]
        access           = "readwrite"
        role_name        = "fake_reports_app_00000000"
        password_version = 1
      }
    }
  }
  # The schema must be released by meroku before this plan in production.
  # Keep the schema in state from the prior run, but do not apply its removal.
  assert {
    condition     = join(",", keys(postgresql_role.svc)) == "backend,reports" && postgresql_role.svc["backend"].name == run.urls_are_pooled_ssl_only_and_do_not_encode_search_path.role_names["backend"]
    error_message = "Access/rotation updates must not rename the role or change its service key."
  }
  assert {
    condition     = join(",", postgresql_role.svc["backend"].search_path) == "public"
    error_message = "Returning search_path to public must explicitly update the role schema list."
  }
  assert {
    condition     = tonumber(postgresql_role.svc["backend"].password_wo_version) == 2 && tonumber(random_password.svc["backend"].keepers.version) == 2
    error_message = "An explicit rotation must update both the password generator and role version trigger."
  }
}

run "reused_database_preserves_public_privileges" {
  command = plan
  variables {
    revoke_public = false
  }
  assert {
    condition     = length(postgresql_grant.public_database) == 0
    error_message = "Meroku must not revoke PUBLIC on reused databases."
  }
}

run "uppercase_role_name_is_refused" {
  command = plan
  variables {
    services = {
      backend = {
        schemas          = ["public"]
        access           = "readwrite"
        role_name        = "InvalidUpperCase"
        password_version = 1
      }
    }
  }
  expect_failures = [var.services]
}

run "overlong_role_name_is_refused" {
  command = plan
  variables {
    services = {
      backend = {
        schemas          = ["public"]
        access           = "readonly"
        role_name        = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
        password_version = 1
      }
    }
  }
  expect_failures = [var.services]
}

run "owner_access_level_is_refused" {
  command = plan
  variables {
    services = {
      backend = {
        schemas          = ["public"]
        access           = "owner"
        role_name        = "fake_backend"
        password_version = 1
      }
    }
  }
  expect_failures = [var.services]
}
