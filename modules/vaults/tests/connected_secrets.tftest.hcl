# Vault discovery, observable secret mappings and least-privilege roles.
mock_provider "aws" {}

override_data {
  target = data.aws_region.current
  values = { region = "us-east-1", name = "us-east-1" }
}
override_data {
  target = data.aws_caller_identity.current
  values = { account_id = "000000000000" }
}
override_data {
  target = data.aws_partition.current
  values = { partition = "aws" }
}

variables {
  project = "myapp"
  env     = "dev"
  workloads = {
    backend = { task_role_name = "backend-task", execution_role_name = "backend-execution" }
    api     = { task_role_name = "api-task", execution_role_name = "api-execution" }
    worker  = { task_role_name = "worker-task", execution_role_name = "worker-execution" }
  }
  connections = [
    { workload = "api", vault = "payments", access = "read_write", prefix = "PAY" },
    { workload = "backend", vault = "shared" },
    { workload = "api", vault = "shared" }
  ]
}

run "vault_names_paths_and_origins_are_sorted" {
  command = plan

  override_data {
    target = data.aws_ssm_parameters_by_path.vault["shared"]
    values = {
      names = [
        "/dev/myapp/vault/shared/database_url",
        "/dev/myapp/vault/shared/api_key"
      ]
    }
  }
  override_data {
    target = data.aws_ssm_parameters_by_path.vault["payments"]
    values = {
      names = [
        "/dev/myapp/vault/payments/env",
        "/dev/myapp/vault/payments/database_url"
      ]
    }
  }

  assert {
    # A directory path may be represented with or without its final slash.
    condition     = { for name, path in output.vault_paths : name => trimsuffix(path, "/") } == { shared = "/dev/myapp/vault/shared", payments = "/dev/myapp/vault/payments" }
    error_message = "Every used vault must resolve to exactly its configured /env/project/vault/name directory."
  }

  assert {
    condition = (
      toset(keys(data.aws_ssm_parameters_by_path.vault)) == toset(["shared", "payments"]) &&
      alltrue([for d in data.aws_ssm_parameters_by_path.vault : d.recursive == false && d.with_decryption == false])
    )
    error_message = "Discover only connected vaults, nonrecursively and without decrypting values."
  }

  assert {
    condition = (
      [for secret in output.secrets["api"] : secret.name] == ["API_KEY", "DATABASE_URL", "PAY_DATABASE_URL", "PAY_ENV"] &&
      [for secret in output.secrets["backend"] : secret.name] == ["API_KEY", "DATABASE_URL"] &&
      length(output.secrets["worker"]) == 0
    )
    error_message = "Secret names must use the upper-cased last segment and optional prefix, sorted by final name; unconnected workloads receive none."
  }

  assert {
    condition = alltrue([
      for name, path in {
        API_KEY          = "/dev/myapp/vault/shared/api_key"
        DATABASE_URL     = "/dev/myapp/vault/shared/database_url"
        PAY_DATABASE_URL = "/dev/myapp/vault/payments/database_url"
        PAY_ENV          = "/dev/myapp/vault/payments/env"
      } :
      alltrue([
        for secret in output.secrets["api"] :
        (secret.valueFrom == path || secret.valueFrom == "arn:aws:ssm:us-east-1:000000000000:parameter${path}") &&
        strcontains(secret.origin, path)
        if secret.name == name
      ])
    ])
    error_message = "Every secret must reference the actual listed SSM parameter and retain its origin path for collision diagnostics."
  }

  assert {
    condition = alltrue([
      for b in output.secrets["backend"] :
      b.valueFrom == one([for a in output.secrets["api"] : a.valueFrom if a.name == b.name]) &&
      b.origin == one([for a in output.secrets["api"] : a.origin if a.name == b.name])
    ])
    error_message = "Two workloads reading the same vault must receive the same parameter reference and origin, not copies under their own paths."
  }
}

run "vault_grants_separate_execution_reads_from_task_writes" {
  command = plan

  override_data {
    target = data.aws_ssm_parameters_by_path.vault["shared"]
    values = {
      names = [
        "/dev/myapp/vault/shared/database_url",
        "/dev/myapp/vault/shared/api_key"
      ]
    }
  }
  override_data {
    target = data.aws_ssm_parameters_by_path.vault["payments"]
    values = {
      names = [
        "/dev/myapp/vault/payments/env",
        "/dev/myapp/vault/payments/database_url"
      ]
    }
  }

  assert {
    condition     = toset([for p in aws_iam_role_policy.execution_read : p.role]) == toset(["api-execution", "backend-execution"])
    error_message = "Vault GetParameters belongs only to connected execution roles."
  }

  assert {
    condition = alltrue([
      for p in aws_iam_role_policy.execution_read :
      toset(flatten([for st in jsondecode(p.policy).Statement : try(tolist(st.Action), [tostring(st.Action)])])) == toset(["ssm:GetParameters"]) &&
      alltrue([for st in jsondecode(p.policy).Statement : st.Effect == "Allow"]) &&
      toset(flatten([for st in jsondecode(p.policy).Statement : try(tolist(st.Resource), [tostring(st.Resource)])])) == (
        p.role == "api-execution" ?
        toset(["arn:aws:ssm:us-east-1:000000000000:parameter/dev/myapp/vault/shared/*", "arn:aws:ssm:us-east-1:000000000000:parameter/dev/myapp/vault/payments/*"]) :
        toset(["arn:aws:ssm:us-east-1:000000000000:parameter/dev/myapp/vault/shared/*"])
      )
    ])
    error_message = "Each execution role must read only its connected vault paths through GetParameters."
  }

  assert {
    condition = (
      length(aws_iam_role_policy.task_read_write) == 1 &&
      one(values(aws_iam_role_policy.task_read_write)).role == "api-task"
    )
    error_message = "Only the read_write connection may create a task-role grant; read connections must not."
  }

  assert {
    condition = (
      alltrue([for st in jsondecode(one(values(aws_iam_role_policy.task_read_write)).policy).Statement : st.Effect == "Allow"]) &&
      length(setsubtract(
        toset(["ssm:PutParameter", "ssm:GetParameter", "ssm:GetParametersByPath"]),
        toset(flatten([for st in jsondecode(one(values(aws_iam_role_policy.task_read_write)).policy).Statement : try(tolist(st.Action), [tostring(st.Action)])]))
      )) == 0 &&
      length(setsubtract(
        toset(flatten([for st in jsondecode(one(values(aws_iam_role_policy.task_read_write)).policy).Statement : try(tolist(st.Action), [tostring(st.Action)])])),
        toset(["ssm:PutParameter", "ssm:GetParameter", "ssm:GetParameters", "ssm:GetParametersByPath"])
      )) == 0 &&
      alltrue(flatten([
        for st in jsondecode(one(values(aws_iam_role_policy.task_read_write)).policy).Statement : [
          for action in try(tolist(st.Action), [tostring(st.Action)]) :
          action == "ssm:GetParametersByPath" ?
          (
            length(try(tolist(st.Resource), [tostring(st.Resource)])) > 0 &&
            length(setsubtract(
              toset(try(tolist(st.Resource), [tostring(st.Resource)])),
              toset([
                "arn:aws:ssm:us-east-1:000000000000:parameter/dev/myapp/vault/payments",
                "arn:aws:ssm:us-east-1:000000000000:parameter/dev/myapp/vault/payments/*"
              ])
            )) == 0
          ) :
          toset(try(tolist(st.Resource), [tostring(st.Resource)])) ==
          toset(["arn:aws:ssm:us-east-1:000000000000:parameter/dev/myapp/vault/payments/*"])
        ]
      ])) &&
      anytrue([
        for st in jsondecode(one(values(aws_iam_role_policy.task_read_write)).policy).Statement :
        contains(try(tolist(st.Action), [tostring(st.Action)]), "ssm:GetParametersByPath") &&
        contains(try(tolist(st.Resource), [tostring(st.Resource)]), "arn:aws:ssm:us-east-1:000000000000:parameter/dev/myapp/vault/payments")
      ])
    )
    error_message = "The task may Put/Get/GetByPath only under its read_write vault, with no Delete or wildcard action."
  }
}

run "no_connections_create_no_secrets_or_grants" {
  command = plan

  variables {
    connections = []
  }

  assert {
    condition = (
      length(data.aws_ssm_parameters_by_path.vault) == 0 &&
      length(aws_iam_role_policy.execution_read) == 0 &&
      length(aws_iam_role_policy.task_read_write) == 0 &&
      alltrue([for w, secrets in output.secrets : length(secrets) == 0]) &&
      toset(keys(output.secrets)) == toset(["backend", "api", "worker"])
    )
    error_message = "Without connections there must be no vault lookup or grant, but each workload still has an empty secrets output."
  }
}
