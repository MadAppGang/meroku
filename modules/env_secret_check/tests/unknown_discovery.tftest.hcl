# A known result must not inherit unknown own-path discovery.

run "known_env_collision_is_decided_before_discovery" {
  command = plan

  module {
    source = "./tests/fixtures/unknown_discovery"
  }

  variables {
    vault_secret_name = "ENV"
  }

  assert {
    condition = (
      output.valid_known["fresh"] == false &&
      strcontains(output.message_known["fresh"], "ENV is reserved") &&
      strcontains(output.message_known["fresh"], "/dev/myapp/api/env") &&
      strcontains(output.message_known["fresh"], "/dev/myapp/vault/payments/env")
    )
    error_message = "ENV must be rejected at plan even while own-path discovery is unknown, with both origins already available."
  }
}

run "known_prefixed_env_is_valid_before_discovery" {
  command = plan

  module {
    source = "./tests/fixtures/unknown_discovery"
  }

  variables {
    vault_secret_name = "PAY_ENV"
  }

  assert {
    condition     = output.valid_known["fresh"] && length(output.message_known["fresh"]) > 0
    error_message = "PAY_ENV must be accepted by the known check at plan, without waiting for unknown discovery or failing message rendering."
  }
}
