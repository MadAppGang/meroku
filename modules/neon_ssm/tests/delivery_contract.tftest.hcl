# REQ-15 / TEST-14: tokens are a service-keyed map, not a shared list.
mock_provider "aws" {
  mock_resource "aws_ssm_parameter" {
    defaults = {
      version = 7
    }
  }
}

variables {
  project  = "myapp"
  env      = "dev"
  services = ["backend", "reports"]
  urls = {
    backend = "postgresql://fake_backend:fake-password-do-not-use@ep-fake-pooler.example.invalid/app?sslmode=require"
    reports = "postgresql://fake_reports:fake-password-do-not-use@ep-fake-pooler.example.invalid/analytics?sslmode=require"
  }
}

run "each_service_gets_only_its_own_secure_database_url_and_token" {
  command = apply
  assert {
    condition     = join(",", keys(aws_ssm_parameter.database_url)) == "backend,reports"
    error_message = "Parameter identity is keyed by service, not by database or role."
  }
  assert {
    condition = alltrue([for svc, parameter in aws_ssm_parameter.database_url :
      parameter.name == "/dev/myapp/${svc}/database_url" &&
      parameter.type == "SecureString" &&
      nonsensitive(parameter.value) == nonsensitive(var.urls[svc])
    ])
    error_message = "Exactly one SecureString DATABASE_URL must be delivered under each service's existing SSM path."
  }
  assert {
    condition     = join(",", keys(output.dependency_tokens)) == "backend,reports" && output.dependency_tokens["backend"] == "/dev/myapp/backend/database_url@7" && output.dependency_tokens["reports"] == "/dev/myapp/reports/database_url@7"
    error_message = "Each service needs its own name@version token; a shared token would redeploy unrelated services."
  }
}

run "service_move_changes_url_not_parameter_path" {
  command = plan
  variables {
    urls = {
      backend = "postgresql://fake_backend_other:fake-password-do-not-use@ep-fake-pooler.example.invalid/other?sslmode=require"
      reports = "postgresql://fake_reports:fake-password-do-not-use@ep-fake-pooler.example.invalid/analytics?sslmode=require"
    }
  }
  assert {
    condition     = aws_ssm_parameter.database_url["backend"].name == "/dev/myapp/backend/database_url" && endswith(nonsensitive(aws_ssm_parameter.database_url["backend"].value), "/other?sslmode=require")
    error_message = "Moving a service to another database must update the existing parameter in place."
  }
}

run "no_services_writes_no_parameters_or_pg_variables" {
  command = plan
  variables {
    services = []
    urls     = {}
  }
  assert {
    condition     = length(aws_ssm_parameter.database_url) == 0 && length(output.dependency_tokens) == 0
    error_message = "No service connection means no SSM URL or dependency token."
  }
}
