# The contract this module has to keep, in the order the contract matters.
#
#   1. Nothing already deployed gets renamed.   (legacy_names_are_untouched)
#   2. Nothing ever exceeds its AWS limit.      (extreme_inputs_stay_within_limits)
#   3. A required suffix always survives.       (fifo_suffix_survives_truncation)
#   4. Truncation never decides uniqueness.     (shared_prefixes_stay_distinct)
#
# Run: terraform test  (from modules/naming)

variables {
  project = "myapp"
  env     = "dev"

  requests = {
    # ---- limit 32 -------------------------------------------------------
    alb = {
      legacy = "myapp-alb-dev"
      parts  = ["alb"]
      limit  = 32
    }
    backend_tg = {
      legacy = "myapp-backend-tg-dev"
      parts  = ["backend"]
      limit  = 32
    }
    backend_tg_instance = {
      legacy = "myapp-backend-inst-tg-dev"
      parts  = ["backend", "inst"]
      limit  = 32
    }

    # ---- limit 63 -------------------------------------------------------
    s3_bucket = {
      legacy = "myapp-uploads-dev"
      parts  = ["uploads"]
      limit  = 63
    }
    backend_bucket = {
      legacy = "myapp-backend-dev-a1b2c"
      parts  = ["backend"]
      limit  = 63
      suffix = "-a1b2c"
    }
    postgres = {
      legacy = "myapp-postgres-dev"
      parts  = ["postgres"]
      limit  = 63
    }
    aurora = {
      legacy = "myapp-aurora-dev"
      parts  = ["aurora"]
      limit  = 63
    }
    aurora_instance = {
      legacy = "myapp-aurora-instance-dev"
      parts  = ["aurora", "instance"]
      limit  = 63
    }

    # ---- limit 64, "_" separator ----------------------------------------
    backend_task_role = {
      legacy    = "myapp_backend_task_dev"
      parts     = ["backend", "task"]
      limit     = 64
      separator = "_"
    }
    backend_task_exec_role = {
      legacy    = "myapp_backend_task_execution_dev"
      parts     = ["backend", "task", "execution"]
      limit     = 64
      separator = "_"
    }
    ecs_instance_role = {
      legacy    = "myapp_ecs_instance_dev"
      parts     = ["ecs", "instance"]
      limit     = 64
      separator = "_"
    }
    lambda_deploy_role = {
      legacy    = "myapp_lambda_deploy_iam_dev"
      parts     = ["lambda", "deploy", "iam"]
      limit     = 64
      separator = "_"
    }
    ci_lambda = {
      legacy    = "myapp_ci_lambda_dev"
      parts     = ["ci", "lambda"]
      limit     = 64
      separator = "_"
    }
    ci_ecr_rule = {
      legacy    = "myapp_ci_ecr_dev"
      parts     = ["ci", "ecr"]
      limit     = 64
      separator = "_"
    }
    ci_manual_global_rule = {
      legacy    = "myapp_ci_manual_global_dev"
      parts     = ["ci", "manual", "global"]
      limit     = 64
      separator = "_"
    }
    pgadmin_task_exec_role = {
      legacy    = "myapp_pgadmin_task_execution_dev"
      parts     = ["pgadmin", "task", "execution"]
      limit     = 64
      separator = "_"
    }

    # ---- limit 64, env in the MIDDLE of the legacy form ------------------
    # Form 2 would reorder these, which is exactly why legacy is passed: it
    # wins whenever it fits, so the reordering never reaches AWS.
    github_role = {
      legacy = "myapp-dev-github-actions-role"
      parts  = ["github", "actions", "role"]
      limit  = 64
    }
    appsync_role = {
      legacy = "myapp-dev-appsync-role"
      parts  = ["appsync", "role"]
      limit  = 64
    }
    schedule_group = {
      legacy = "myapp-schedule-group-dev-cleanup"
      parts  = ["schedule", "group", "cleanup"]
      limit  = 64
    }

    # ---- limit 80, required suffix --------------------------------------
    fifo_queue = {
      legacy = "myapp-dev-orders.fifo"
      parts  = ["orders"]
      limit  = 80
      suffix = ".fifo"
    }
  }
}

run "legacy_names_are_untouched" {
  command = apply

  assert {
    condition     = alltrue([for k, f in output.forms : f == 1])
    error_message = "Every request here passes a legacy name that fits, so every one must come back on form 1. Anything on form 2 or 3 is a rename, and a rename of a live AWS resource is a destroy-and-recreate: ${jsonencode({ for k, f in output.forms : k => f if f != 1 })}"
  }

  assert {
    condition     = output.names["backend_tg"] == "myapp-backend-tg-dev"
    error_message = "Legacy passthrough must be byte-identical, got ${output.names["backend_tg"]}"
  }

  assert {
    condition     = output.names["github_role"] == "myapp-dev-github-actions-role"
    error_message = "A legacy form with env in the middle must survive verbatim, got ${output.names["github_role"]}"
  }

  assert {
    condition     = length(output.collisions) == 0
    error_message = "Collisions: ${jsonencode(output.collisions)}"
  }
}

run "every_name_is_within_its_limit" {
  command = apply

  assert {
    condition     = alltrue([for k, n in output.names : length(n) <= var.requests[k].limit])
    error_message = "Over limit: ${jsonencode({ for k, n in output.names : k => "${length(n)}/${var.requests[k].limit}" if length(n) > var.requests[k].limit })}"
  }
}

# The case that started all of this: a service name long enough to blow the
# 32-character target-group budget. It must drop the decoration, not the name.
run "long_service_keeps_its_name" {
  command = apply

  variables {
    project = "myapp"
    env     = "dev"
    requests = {
      service_tg = {
        # 42 characters: "-service-" and "-tg-" spend 13 of the 32 before the
        # project or the service name get a say.
        legacy = "myapp-service-orders-sync-connector-tg-dev"
        parts  = ["orders-sync-connector"]
        limit  = 32
      }
    }
  }

  assert {
    condition     = output.names["service_tg"] == "myapp-orders-sync-connector-dev"
    error_message = "Expected the decoration to be dropped and the service name kept whole, got ${output.names["service_tg"]}"
  }

  assert {
    condition     = length(output.names["service_tg"]) <= 32
    error_message = "Over limit: ${output.names["service_tg"]}"
  }

  assert {
    condition     = output.forms["service_tg"] == 2
    error_message = "Expected form 2, got ${output.forms["service_tg"]}"
  }
}

run "extreme_inputs_stay_within_limits" {
  command = apply

  variables {
    project = "an-extremely-long-project-name"
    env     = "production"
    requests = {
      tg = {
        parts = ["payments-reconciliation-service"]
        limit = 32
      }
      role = {
        parts     = ["scheduler", "payments-reconciliation", "task", "execution"]
        limit     = 64
        separator = "_"
      }
      bucket = {
        parts = ["backend"]
        limit = 63
      }
    }
  }

  assert {
    condition     = alltrue([for k, n in output.names : length(n) <= var.requests[k].limit])
    error_message = "Over limit: ${jsonencode({ for k, n in output.names : k => length(n) })}"
  }

  assert {
    condition     = startswith(output.names["tg"], "payments-reconciliation")
    error_message = "The identity must lead the name even when truncated, got ${output.names["tg"]}"
  }

  assert {
    condition     = strcontains(output.names["role"], "_")
    error_message = "The requested separator must be honoured in form 3, got ${output.names["role"]}"
  }
}

run "fifo_suffix_survives_truncation" {
  command = apply

  variables {
    project = "an-extremely-long-project-name"
    env     = "production"
    requests = {
      q = {
        parts  = ["orders-reconciliation-batch-processor-with-a-very-long-name"]
        limit  = 80
        suffix = ".fifo"
      }
      tiny = {
        parts  = ["orders-reconciliation-batch-processor"]
        limit  = 32
        suffix = ".fifo"
      }
    }
  }

  assert {
    condition     = alltrue([for k, n in output.names : endswith(n, ".fifo")])
    error_message = "A FIFO queue that loses its .fifo suffix is rejected by AWS: ${jsonencode(output.names)}"
  }

  assert {
    condition     = alltrue([for k, n in output.names : length(n) <= var.requests[k].limit])
    error_message = "Suffix must be counted against the budget, not added past it: ${jsonencode({ for k, n in output.names : k => length(n) })}"
  }
}

# Truncation must never be what decides uniqueness. These three share a 30+
# character prefix and differ only past the point where the head is cut.
run "shared_prefixes_stay_distinct" {
  command = apply

  variables {
    project = "myapp"
    env     = "dev"
    requests = {
      a = { parts = ["payments-reconciliation-batch-alpha"], limit = 32 }
      b = { parts = ["payments-reconciliation-batch-bravo"], limit = 32 }
      c = { parts = ["payments-reconciliation-batch-charlie"], limit = 32 }
    }
  }

  assert {
    condition     = length(output.collisions) == 0
    error_message = "Truncated heads collided; the digest is supposed to prevent this: ${jsonencode(output.names)}"
  }

  assert {
    condition     = length(distinct(values(output.names))) == 3
    error_message = "Expected 3 distinct names, got ${jsonencode(output.names)}"
  }
}

# Two environments sharing one AWS account must not collide either. The
# identity is long enough here that both envs land on form 3, where env has
# been collapsed into the digest and is the ONLY thing separating them — so
# these two runs compare the exact case the digest exists for.
#
# Compared run-to-run rather than against a hardcoded digest: a literal would
# have to be recomputed by hand whenever the inputs change, which is how it
# silently stops testing anything.
run "same_identity_in_dev" {
  command = apply

  variables {
    project = "a-project-name-long-enough-to-force-form-3"
    env     = "dev"
    requests = {
      tg = { parts = ["orders-sync-connector"], limit = 32 }
    }
  }

  assert {
    condition     = output.forms["tg"] == 3
    error_message = "This case is only meaningful on form 3, got form ${output.forms["tg"]}"
  }
}

run "same_identity_in_staging_gets_a_different_name" {
  command = apply

  variables {
    project = "a-project-name-long-enough-to-force-form-3"
    env     = "staging"
    requests = {
      tg = { parts = ["orders-sync-connector"], limit = 32 }
    }
  }

  assert {
    condition     = output.names["tg"] != run.same_identity_in_dev.names["tg"]
    error_message = "dev and staging both produced ${output.names["tg"]}; the digest must cover env or two environments sharing one AWS account collide"
  }

  assert {
    condition     = length(output.names["tg"]) <= 32
    error_message = "Over limit: ${output.names["tg"]}"
  }
}
