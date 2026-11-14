# The plan-time warning for a declared secret with no value in SSM. The check
# lives in ./value_check, one instance per vault, so these runs plan that module
# on its own. SSM answers with the two parameters below.
mock_provider "aws" {
  mock_data "aws_ssm_parameters_by_path" {
    defaults = {
      names = [
        "/dev/myapp/vault/shared/database_url",
        "/dev/myapp/vault/shared/api_key"
      ]
    }
  }
}

variables {
  vault = "shared"
  path  = "/dev/myapp/vault/shared"
}

run "every_declared_secret_set_passes" {
  command = plan

  module {
    source = "./value_check"
  }

  variables {
    names = ["api_key", "database_url"]
  }
}

run "a_parameter_not_declared_is_not_a_warning" {
  command = plan

  module {
    source = "./value_check"
  }

  variables {
    names = ["database_url"]
  }
}

run "a_declared_secret_without_a_value_warns" {
  command = plan

  module {
    source = "./value_check"
  }

  variables {
    names = ["database_url", "stripe_key"]
  }

  # A check block's failure is a warning: the plan succeeds and Terraform
  # reports it. expect_failures makes this run fail if the check passes.
  expect_failures = [check.vault_values_set]
}
