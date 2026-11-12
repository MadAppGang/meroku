# REQ-20 / TEST-16. Offline; no configured real provider.
mock_provider "neon" {
  override_during = plan
  mock_resource "neon_project" {
    defaults = {
      id                   = "fake-project-000000"
      default_branch_id    = "br-fake-main"
      database_host        = "ep-fake.example.invalid"
      database_host_pooler = "ep-fake-pooler.example.invalid"
    }
  }
}

variables {
  mode       = "create"
  owner_role = "neondb_owner"
  project = {
    name       = "fake-project"
    org_id     = "org-fake-000000"
    region_id  = "aws-us-east-1"
    pg_version = 17
  }
  databases = {
    app     = { create = true }
    archive = { create = true }
    neondb  = { create = false }
  }
}

run "create_manages_only_requested_databases_and_uses_project_default_branch" {
  # Plan-only: never put prevent_destroy data into test state for teardown.
  command = plan

  assert {
    condition     = length(neon_project.this) == 1 && join(",", sort(keys(neon_database.created))) == "app,archive"
    error_message = "Create mode must manage one project and only the create=true databases."
  }
  assert {
    condition     = output.project_id == "fake-project-000000" && output.default_branch_id == "br-fake-main"
    error_message = "Project and branch ids must come from the created project, not a configured branch."
  }
  assert {
    condition     = output.host == "ep-fake.example.invalid" && output.pooler_host == "ep-fake-pooler.example.invalid"
    error_message = "The direct and pooled host outputs must remain distinct and match the provider."
  }
  assert {
    condition = alltrue([for name, db in neon_database.created :
      db.name == name && db.project_id == output.project_id &&
      db.branch_id == output.default_branch_id && db.owner_name == "neondb_owner"
    ])
    error_message = "Each requested database belongs to the owner on the project's default branch."
  }
}

run "unknown_mode_is_refused" {
  command = plan
  variables {
    mode = "adopt"
  }
  expect_failures = [var.mode]
}
