# Public split checks and diagnostics, without providers.

run "known_collisions_name_both_parameter_origins" {
  command = plan

  variables {
    workloads = {
      reserved = {
        subject         = "Service \"api\""
        ssm_path        = "/dev/myapp/api"
        yaml_field      = "env_vars"
        yaml_file       = "project/dev.yaml"
        defaults_source = "modules/workloads/env_services.tf"
        environment     = ["AWS_REGION"]
        secrets         = ["ENV", "ENV"]
        known_secrets = [
          { name = "ENV", origin = "/dev/myapp/api/env" },
          { name = "ENV", origin = "/dev/myapp/vault/shared/env" }
        ]
      }
      vault_pair = {
        subject         = "Service \"api\""
        ssm_path        = "/dev/myapp/api"
        yaml_field      = "env_vars"
        yaml_file       = "project/dev.yaml"
        defaults_source = "modules/workloads/env_services.tf"
        environment     = ["AWS_REGION"]
        secrets         = ["ENV", "DATABASE_URL", "DATABASE_URL"]
        known_secrets = [
          { name = "ENV", origin = "/dev/myapp/api/env" },
          { name = "DATABASE_URL", origin = "/dev/myapp/vault/shared/database_url" },
          { name = "DATABASE_URL", origin = "/dev/myapp/vault/payments/database_url" }
        ]
      }
    }
  }

  assert {
    condition = (
      output.valid_known["reserved"] == false &&
      strcontains(output.message_known["reserved"], "ENV is reserved") &&
      strcontains(output.message_known["reserved"], "/dev/myapp/api/env") &&
      strcontains(output.message_known["reserved"], "/dev/myapp/vault/shared/env")
    )
    error_message = "A vault exposing ENV must fail the known check with the ENV-specific explanation and both parameter paths."
  }

  assert {
    condition = (
      output.valid_known["vault_pair"] == false &&
      strcontains(output.message_known["vault_pair"], "DATABASE_URL") &&
      strcontains(output.message_known["vault_pair"], "/dev/myapp/vault/shared/database_url") &&
      strcontains(output.message_known["vault_pair"], "/dev/myapp/vault/payments/database_url")
    )
    error_message = "Two known vault secrets with one final name must be refused, naming both origins."
  }
}

run "prefixed_env_is_not_reserved" {
  command = plan

  variables {
    workloads = {
      api = {
        subject         = "Service \"api\""
        ssm_path        = "/dev/myapp/api"
        yaml_field      = "env_vars"
        yaml_file       = "project/dev.yaml"
        defaults_source = "modules/workloads/env_services.tf"
        environment     = ["AWS_REGION"]
        secrets         = ["ENV", "PAY_ENV"]
        known_secrets = [
          { name = "ENV", origin = "/dev/myapp/api/env" },
          { name = "PAY_ENV", origin = "/dev/myapp/vault/payments/env" }
        ]
      }
    }
  }

  assert {
    condition     = output.valid_known["api"] && output.valid_discovered["api"]
    error_message = "Only the exact final name ENV is reserved; PAY_ENV must coexist with the workload's own ENV."
  }
}

run "extra_names_cannot_override_but_yaml_defaults_can" {
  command = plan

  variables {
    workloads = {
      extra_duplicate = {
        subject         = "Service \"api\""
        ssm_path        = "/dev/myapp/api"
        yaml_field      = "env_vars"
        yaml_file       = "project/dev.yaml"
        defaults_source = "modules/workloads/env_services.tf"
        environment     = ["AWS_REGION", "VALKEY_URL", "VALKEY_URL"]
        secrets         = ["ENV"]
        extra_env_names = ["VALKEY_URL"]
        known_secrets   = [{ name = "ENV", origin = "/dev/myapp/api/env" }]
      }
      default_override = {
        subject         = "Service \"api\""
        ssm_path        = "/dev/myapp/api"
        yaml_field      = "env_vars"
        yaml_file       = "project/dev.yaml"
        defaults_source = "modules/workloads/env_services.tf"
        environment     = ["AWS_REGION", "SQS_QUEUE_URL", "SQS_QUEUE_URL"]
        secrets         = ["ENV"]
        known_secrets   = [{ name = "ENV", origin = "/dev/myapp/api/env" }]
      }
    }
  }

  assert {
    condition = (
      output.valid_known["extra_duplicate"] == false &&
      strcontains(output.message_known["extra_duplicate"], "VALKEY_URL") &&
      strcontains(output.message_known["extra_duplicate"], "env_vars") &&
      output.valid_known["default_override"]
    )
    error_message = "Only a duplicated capability extra is forbidden; an ordinary default followed by its YAML override must remain valid."
  }
}

run "discovered_overlaps_name_both_origins" {
  command = plan

  variables {
    workloads = {
      vault_overlap = {
        subject         = "Service \"api\""
        ssm_path        = "/dev/myapp/api"
        yaml_field      = "env_vars"
        yaml_file       = "project/dev.yaml"
        defaults_source = "modules/workloads/env_services.tf"
        environment     = ["AWS_REGION"]
        secrets         = ["ENV", "DATABASE_URL", "DATABASE_URL"]
        known_secrets = [
          { name = "ENV", origin = "/dev/myapp/api/env" },
          { name = "DATABASE_URL", origin = "/dev/myapp/vault/shared/database_url" }
        ]
        discovered_secrets = [{ name = "DATABASE_URL", origin = "/dev/myapp/api/database_url" }]
      }
      yaml_overlap = {
        subject            = "Service \"api\""
        ssm_path           = "/dev/myapp/api"
        yaml_field         = "env_vars"
        yaml_file          = "project/dev.yaml"
        defaults_source    = "modules/workloads/env_services.tf"
        environment        = ["AWS_REGION", "API_KEY"]
        secrets            = ["ENV", "API_KEY"]
        known_secrets      = [{ name = "ENV", origin = "/dev/myapp/api/env" }]
        discovered_secrets = [{ name = "API_KEY", origin = "/dev/myapp/api/api_key" }]
      }
    }
  }

  assert {
    condition = (
      output.valid_known["vault_overlap"] &&
      output.valid_discovered["vault_overlap"] == false &&
      strcontains(output.message_discovered["vault_overlap"], "DATABASE_URL") &&
      strcontains(output.message_discovered["vault_overlap"], "/dev/myapp/api/database_url") &&
      strcontains(output.message_discovered["vault_overlap"], "/dev/myapp/vault/shared/database_url")
    )
    error_message = "Own-path discovery colliding with a known vault must fail only the discovered check and name both parameter origins."
  }

  assert {
    condition = (
      output.valid_known["yaml_overlap"] &&
      output.valid_discovered["yaml_overlap"] == false &&
      strcontains(output.message_discovered["yaml_overlap"], "API_KEY") &&
      strcontains(output.message_discovered["yaml_overlap"], "/dev/myapp/api/api_key") &&
      strcontains(output.message_discovered["yaml_overlap"], "env_vars") &&
      strcontains(output.message_discovered["yaml_overlap"], "project/dev.yaml")
    )
    error_message = "Own-path discovery colliding with YAML must identify the actual parameter, field and YAML file."
  }
}
