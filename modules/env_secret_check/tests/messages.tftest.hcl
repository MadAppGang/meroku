# The contract, in the order the contract matters.
#
#   1. The message renders for EVERY workload, the clean ones included.
#      (a_clean_workload_still_renders_a_message)
#   2. A name in both lists is a collision; a name in one is not.
#      (a_collision_is_invalid / a_clean_workload_still_renders_a_message)
#   3. The message names the WORKLOAD and the COLLIDING NAME, and says what to
#      do about it. Whoever reads it did not edit that name.
#      (the_message_names_the_workload_the_name_and_the_two_mechanisms)
#   4. Case is not folded here. The SSM side arrives already upper-cased, so
#      folding again would invent a collision ECS does not have.
#      (case_is_not_folded_a_second_time)
#
# Rule 1 is why this module exists at all rather than two inline expressions in
# modules/workloads. That module reads eight remote data sources, so
# `terraform plan` on it cannot run in CI without AWS credentials, and
# `terraform validate` never evaluates a precondition's error_message — so
# between v4.2.0 and v4.4.1 a message that interpolated a null shipped four
# times and broke every Fargate deploy. This module has no provider, so
# `terraform test` plans it with no credentials and no network, and a message
# that cannot render fails here instead of in a user's terminal.
#
# modules/workloads/tests/ does now show that `terraform test` CAN plan that
# module under mock_provider, and env_secret_collision.tftest.hcl there covers
# the wiring — which names reach this module, and that the upper-casing on the
# SSM side happens before they do. It does not replace this file: mock_provider
# needs the AWS provider's schema and runs its client-side validation, so a
# failure there surfaces wherever the plan walk happens to stop, while a message
# that cannot render fails here on the line that renders it.
#
# Run: terraform test  (from modules/env_secret_check)

variables {
  workloads = {
    # Clean. The list is empty, which is the case that renders on every plan of
    # every healthy project and therefore the case most likely to crash unseen.
    clean = {
      subject     = "Service \"web\""
      ssm_path    = "/dev/acme/web"
      yaml_field  = "env_vars"
      yaml_file   = "project/dev.yaml"
      environment = ["AWS_REGION", "SERVICE_NAME"]
      secrets     = ["ENV", "DATABASE_PASSWORD"]
    }

    # The reported failure: SCAN_CURSOR_STORE declared in YAML, and an SSM
    # parameter under the service's path whose upper-cased last segment is the
    # same string.
    colliding = {
      subject     = "Service \"orders\""
      ssm_path    = "/dev/acme/orders"
      yaml_field  = "env_vars"
      yaml_file   = "project/dev.yaml"
      environment = ["AWS_REGION", "SCAN_CURSOR_STORE", "SERVICE_NAME"]
      secrets     = ["ENV", "SCAN_CURSOR_STORE"]
    }
  }
}

# THE regression test, and it asserts almost nothing about the text because the
# assertion is not the point: rendering the message for a workload with NOTHING
# WRONG WITH IT is. Terraform builds error_message before it tests the
# condition, and for every instance — so a message that needs a non-empty
# collision list kills a plan that is entirely correct. A bare
# `join(", ", local.collisions[k])` here is enough to make the sentence
# nonsense; something that could not render at all would fail this run before
# any assert is reached.
run "a_clean_workload_still_renders_a_message" {
  command = apply

  assert {
    condition     = length(output.message["clean"]) > 0
    error_message = "A workload with no collision must still render a message. This is the v4.2.0 regression shape: Terraform renders error_message before it tests the condition, so a message that needs a collision kills a plan that is entirely correct."
  }

  assert {
    condition     = output.valid["clean"]
    error_message = "\"web\" shares no name between environment and secrets and must be valid, got invalid."
  }

  assert {
    condition     = length(output.collisions["clean"]) == 0
    error_message = "A clean workload must report no collisions, got ${jsonencode(output.collisions["clean"])}."
  }

  # The specific way the empty case goes wrong. `join(", ", [])` is "", which
  # renders "Service \"web\" would set  both as a plain environment variable"
  # — a double space where the subject of the sentence should be. It is never
  # SHOWN, because the condition passes, which is exactly what makes it the kind
  # of defect that survives to a release.
  assert {
    condition     = !strcontains(output.message["clean"], "would set  both")
    error_message = "The message trails off into an empty name list: ${output.message["clean"]}"
  }
}

run "a_collision_is_invalid" {
  command = apply

  assert {
    condition     = output.valid["colliding"] == false
    error_message = "SCAN_CURSOR_STORE is in both environment and secrets, so the workload must be invalid. ECS rejects the RegisterTaskDefinition call outright; a plan that lets it through fails mid-apply."
  }

  assert {
    condition     = output.collisions["colliding"] == tolist(["SCAN_CURSOR_STORE"])
    error_message = "The colliding name must be reported exactly once, got ${jsonencode(output.collisions["colliding"])}."
  }
}

# The text a user reads at the moment their deploy stops, and the only thing
# telling them what to do. The opening line is pinned byte for byte because
# every value that could arrive null or empty is interpolated into it; the rest
# is checked by fragment, because the remedy is prose that will be reworded and
# a byte-exact copy of four paragraphs is a test nobody would maintain.
run "the_message_names_the_workload_the_name_and_the_two_mechanisms" {
  command = apply

  assert {
    condition     = split("\n", output.message["colliding"])[0] == "Service \"orders\" would set SCAN_CURSOR_STORE both as a plain environment variable and as a secret. ECS refuses to register a task definition where one name appears in both, so this does not fail here for tidiness — it fails mid-apply, at RegisterTaskDefinition, with \"ClientException: The secret name must be unique and not shared with any new or existing environment variables set on the container\"."
    error_message = "Opening line changed: ${split("\n", output.message["colliding"])[0]}"
  }

  # The two mechanisms, named. Whoever hits this did not edit SCAN_CURSOR_STORE
  # — someone else created an SSM parameter — so a message that only says "fix
  # your config" sends them to the wrong file.
  assert {
    condition     = strcontains(output.message["colliding"], "/dev/acme/orders")
    error_message = "The message must name the SSM path the secrets are discovered under; it is the half of the collision that is in no config file. Got: ${output.message["colliding"]}"
  }

  assert {
    condition     = strcontains(output.message["colliding"], "env_vars in project/dev.yaml")
    error_message = "The message must name the YAML field and file the declared variables come from. Got: ${output.message["colliding"]}"
  }

  assert {
    condition     = strcontains(output.message["colliding"], "UPPER-CASED")
    error_message = "The message must say that an SSM parameter's last segment is upper-cased, or a reader who created \"scan_cursor_store\" will not connect it to SCAN_CURSOR_STORE. Got: ${output.message["colliding"]}"
  }
}

# Two names at once, and the sort. setintersection returns a set, whose
# iteration order is Terraform's; without the sort the message reorders itself
# between runs and no byte-exact assertion on it can hold.
run "multiple_collisions_are_all_listed_in_sorted_order" {
  command = apply

  variables {
    workloads = {
      many = {
        subject     = "The backend"
        ssm_path    = "/dev/acme/backend"
        yaml_field  = "backend_env_variables"
        yaml_file   = "project/dev.yaml"
        environment = ["ZULU", "ALPHA", "AWS_REGION", "MIKE"]
        secrets     = ["MIKE", "ZULU", "ALPHA", "ENV"]
      }
    }
  }

  assert {
    condition     = output.collisions["many"] == tolist(["ALPHA", "MIKE", "ZULU"])
    error_message = "All three colliding names must be reported, sorted. Got ${jsonencode(output.collisions["many"])}."
  }

  assert {
    condition     = strcontains(output.message["many"], "The backend would set ALPHA, MIKE, ZULU both")
    error_message = "The message must list every colliding name, not just the first. Got: ${output.message["many"]}"
  }
}

# The deliberate limit of the check, stated as a test so nobody "fixes" it.
#
# The upper-casing lives upstream, in modules/workloads: an SSM parameter
# ".../scan_cursor_store" reaches this module already named SCAN_CURSOR_STORE.
# Folding case again HERE would catch nothing extra and would invent a collision
# AWS does not have — ECS compares the two lists by exact string equality, so a
# container may carry the environment variable "scan_cursor_store" and the
# secret "SCAN_CURSOR_STORE" side by side, and refusing to plan that would block
# a configuration that applies cleanly today.
run "case_is_not_folded_a_second_time" {
  command = apply

  variables {
    workloads = {
      mixed = {
        subject     = "Service \"orders\""
        ssm_path    = "/dev/acme/orders"
        yaml_field  = "env_vars"
        yaml_file   = "project/dev.yaml"
        environment = ["scan_cursor_store"]
        secrets     = ["SCAN_CURSOR_STORE"]
      }
    }
  }

  assert {
    condition     = output.valid["mixed"]
    error_message = "\"scan_cursor_store\" and \"SCAN_CURSOR_STORE\" are two different names to ECS and it accepts both on one container. Folding case here would refuse a configuration AWS applies. The case that DOES need catching — a lower-case SSM parameter against an upper-case YAML variable — is caught because modules/workloads upper-cases the parameter's last segment before the name ever reaches this module; see modules/workloads/tests/env_secret_collision.tftest.hcl."
  }
}
