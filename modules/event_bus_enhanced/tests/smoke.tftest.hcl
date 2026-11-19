# Smoke test: a shared bus with the revoke step and the guard plans under mock
# providers. No AWS calls, provisioners do not run. Fixtures are synthetic.
#
# Run: terraform -chdir=modules/event_bus_enhanced test

mock_provider "awscc" {}

mock_provider "aws" {
  mock_data "aws_region" {
    defaults = {
      region = "ap-southeast-2"
    }
  }
  mock_data "aws_caller_identity" {
    defaults = {
      account_id = "000000000000"
    }
  }
}

run "shared_bus_plans" {
  command = plan

  variables {
    project = "myapp"
    env     = "dev"
    share   = [{ kind = "account", id = "000000000001", access = "subscribe" }]
    revoke = {
      enabled       = true
      granted       = ["account:000000000001"]
      keep_accounts = ["000000000001"]
    }
    guard = true
  }

  assert {
    condition     = length(awscc_eventsv2_event_bus.project) == 1 && length(terraform_data.revoke) == 1 && length(terraform_data.revoke_record) == 1 && length(terraform_data.bus_guard) == 1
    error_message = "A shared, enabled bus plans the bus, the revoke step, its record and the guard."
  }

  assert {
    condition     = terraform_data.revoke[0].triggers_replace.round == "0" && terraform_data.revoke_record[0].input.round == 0
    error_message = "round defaults to 0 and reaches the sweep's triggers and the record."
  }

  assert {
    condition     = length(terraform_data.revoke[0].input.judged) == 0 && length(terraform_data.revoke_record[0].input.judged) == 0
    error_message = "judged defaults to empty and reaches the sweep and the record."
  }
}
