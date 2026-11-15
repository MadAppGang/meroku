# Keys, aliases, grants per access and the variables each workload gets.
#
# Applied against a mock provider: the key ARNs and the public keys are computed
# values, so they are known after apply. Every value here is synthetic. The key
# IDs are shaped like real ones (version nibble 1-5, variant nibble 8-b): the
# provider validates aws_kms_public_key's key_id even against a mock.
mock_provider "aws" {
  mock_data "aws_kms_public_key" {
    defaults = {
      public_key_pem = "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEfakeKeyDoNotUseAAAAAAAAAAAAAA\nAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==\n-----END PUBLIC KEY-----\n"
    }
  }
}

override_resource {
  target = aws_kms_key.key["auth"]
  values = {
    arn    = "arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789001"
    key_id = "1234abcd-12ab-34cd-86ef-123456789001"
  }
}

override_resource {
  target = aws_kms_key.key["webhooks"]
  values = {
    arn    = "arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789002"
    key_id = "1234abcd-12ab-34cd-86ef-123456789002"
  }
}

override_resource {
  target = aws_kms_key.key["legacy"]
  values = {
    arn    = "arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789003"
    key_id = "1234abcd-12ab-34cd-86ef-123456789003"
  }
}

variables {
  project = "myapp"
  env     = "dev"
  workloads = {
    backend = { task_role_name = "backend-task" }
    api     = { task_role_name = "api-task" }
    nightly = { task_role_name = "nightly-task" }
    idle    = { task_role_name = "idle-task" }
  }
  keys = {
    auth     = {}
    webhooks = { key_spec = "RSA_2048" }
    legacy   = { key_spec = "ECC_NIST_P384" }
  }
  connections = [
    { workload = "api", key = "webhooks", access = "verify" },
    { workload = "api", key = "auth", access = "sign" },
    { workload = "backend", key = "auth", access = "both" },
    { workload = "nightly", key = "auth", access = "verify" }
  ]
}

run "keys_and_aliases" {
  command = apply

  assert {
    condition     = toset(keys(aws_kms_key.key)) == toset(["auth", "webhooks", "legacy"])
    error_message = "Every key of var.keys, the retired one included, must be a KMS key."
  }

  assert {
    condition = alltrue([
      for k in aws_kms_key.key :
      k.key_usage == "SIGN_VERIFY" && k.deletion_window_in_days == 7 && k.enable_key_rotation == false
    ])
    error_message = "Every key must be SIGN_VERIFY, without rotation, with a 7-day deletion window."
  }

  assert {
    condition = (
      aws_kms_key.key["auth"].customer_master_key_spec == "ECC_NIST_P256" &&
      aws_kms_key.key["webhooks"].customer_master_key_spec == "RSA_2048" &&
      aws_kms_key.key["legacy"].customer_master_key_spec == "ECC_NIST_P384"
    )
    error_message = "A key without key_spec must be ECC_NIST_P256; an explicit key_spec must be kept."
  }

  assert {
    condition = (
      strcontains(aws_kms_key.key["auth"].description, "myapp") &&
      strcontains(aws_kms_key.key["auth"].description, "dev") &&
      strcontains(aws_kms_key.key["auth"].description, "auth") &&
      aws_kms_key.key["auth"].tags["Project"] == "myapp" &&
      aws_kms_key.key["auth"].tags["Environment"] == "dev"
    )
    error_message = "A key's description and tags must name its project, env and key."
  }

  assert {
    condition = output.aliases == {
      auth     = "alias/myapp-dev-auth"
      webhooks = "alias/myapp-dev-webhooks"
      legacy   = "alias/myapp-dev-legacy"
    }
    error_message = "Every key must have the alias alias/<project>-<env>-<name>."
  }

  assert {
    condition     = aws_kms_alias.key["webhooks"].target_key_id == "1234abcd-12ab-34cd-86ef-123456789002"
    error_message = "An alias must point at its own key."
  }

  assert {
    condition = output.key_arns == {
      auth     = "arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789001"
      webhooks = "arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789002"
      legacy   = "arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789003"
    }
    error_message = "key_arns must list every key's ARN by name."
  }
}

run "grants_per_access" {
  command = apply

  assert {
    condition     = toset([for p in aws_iam_role_policy.workload : p.role]) == toset(["api-task", "backend-task", "nightly-task"])
    error_message = "Only connected workloads get a policy, on their task role."
  }

  assert {
    condition     = aws_iam_role_policy.workload["api"].name == "myapp_kms_keys_api_dev"
    error_message = "The policy is named <project>_kms_keys_<workload>_<env>."
  }

  # sign: kms:Sign on auth; verify: kms:Verify on webhooks; both read actions on both keys.
  assert {
    condition = jsondecode(output.policies["api"]).Statement == [
      {
        Sid      = "KmsSign"
        Effect   = "Allow"
        Action   = ["kms:Sign"]
        Resource = ["arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789001"]
      },
      {
        Sid      = "KmsVerify"
        Effect   = "Allow"
        Action   = ["kms:Verify"]
        Resource = ["arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789002"]
      },
      {
        Sid    = "KmsPublicKey"
        Effect = "Allow"
        Action = ["kms:GetPublicKey", "kms:DescribeKey"]
        Resource = [
          "arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789001",
          "arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789002",
        ]
      },
    ]
    error_message = "sign must grant kms:Sign and verify kms:Verify on their own keys only, and both GetPublicKey and DescribeKey."
  }

  assert {
    condition = (
      toset(flatten([for st in jsondecode(output.policies["backend"]).Statement : st.Action])) ==
      toset(["kms:Sign", "kms:Verify", "kms:GetPublicKey", "kms:DescribeKey"]) &&
      alltrue([
        for st in jsondecode(output.policies["backend"]).Statement :
        st.Resource == ["arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789001"]
      ])
    )
    error_message = "both must grant Sign, Verify, GetPublicKey and DescribeKey on the key."
  }

  assert {
    condition = (
      toset(flatten([for st in jsondecode(output.policies["nightly"]).Statement : st.Action])) ==
      toset(["kms:Verify", "kms:GetPublicKey", "kms:DescribeKey"])
    )
    error_message = "verify must not grant kms:Sign."
  }

  assert {
    condition = !anytrue([
      for p in values(output.policies) :
      strcontains(p, "1234abcd-12ab-34cd-86ef-123456789003")
    ])
    error_message = "A retired key, with no connections, must appear in no policy."
  }
}

run "environment_per_workload" {
  command = apply

  assert {
    condition     = toset(keys(output.environment)) == toset(["backend", "api", "nightly", "idle"])
    error_message = "Every workload must have an entry."
  }

  assert {
    condition     = output.environment["idle"] == []
    error_message = "An unconnected workload gets no variables."
  }

  assert {
    condition = [for e in output.environment["api"] : e.name] == [
      "KMS_AUTH_KEY_ARN", "KMS_AUTH_PUBLIC_KEY", "KMS_WEBHOOKS_KEY_ARN", "KMS_WEBHOOKS_PUBLIC_KEY",
    ]
    error_message = "A connected workload gets KMS_<NAME>_KEY_ARN and KMS_<NAME>_PUBLIC_KEY per key, sorted by key name."
  }

  assert {
    condition = (
      one([for e in output.environment["nightly"] : e.value if e.name == "KMS_AUTH_KEY_ARN"]) ==
      "arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789001"
    )
    error_message = "KMS_<NAME>_KEY_ARN must be the key's ARN."
  }

  assert {
    condition = (
      one([for e in output.environment["backend"] : e.value if e.name == "KMS_AUTH_PUBLIC_KEY"]) ==
      "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEfakeKeyDoNotUseAAAAAAAAAAAAAA\nAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==\n-----END PUBLIC KEY-----\n"
    )
    error_message = "KMS_<NAME>_PUBLIC_KEY must be the key's PEM, newlines included."
  }

  assert {
    condition = !anytrue(flatten([
      for w, env in output.environment : [for e in env : strcontains(e.name, "LEGACY")]
    ]))
    error_message = "A retired key must give no workload a variable."
  }
}

run "no_keys_renders_nothing" {
  command = apply

  variables {
    keys        = {}
    connections = []
  }

  assert {
    condition = (
      length(aws_kms_key.key) == 0 &&
      length(aws_iam_role_policy.workload) == 0 &&
      output.key_arns == {} &&
      alltrue([for w, env in output.environment : length(env) == 0])
    )
    error_message = "An environment without keys creates no key, no grant and no variable."
  }
}

run "connection_to_unknown_key_is_refused" {
  command = plan

  variables {
    connections = [{ workload = "api", key = "missing", access = "sign" }]
  }

  expect_failures = [var.connections]
}

run "bad_access_is_refused" {
  command = plan

  variables {
    connections = [{ workload = "api", key = "auth", access = "encrypt" }]
  }

  expect_failures = [var.connections]
}

run "bad_key_spec_is_refused" {
  command = plan

  variables {
    keys        = { auth = { key_spec = "SYMMETRIC_DEFAULT" } }
    connections = []
  }

  expect_failures = [var.keys]
}

# A key that exists keeps its type when key_spec changes: no replacement, and
# the plan warns. The first run creates auth as ECC_NIST_P256; the second sets
# RSA_2048 against that state.
run "key_spec_before_change" {
  command = apply

  assert {
    condition     = aws_kms_key.key["auth"].customer_master_key_spec == "ECC_NIST_P256"
    error_message = "auth must be created as ECC_NIST_P256."
  }
}

run "changed_key_spec_is_ignored_and_warned" {
  command = apply

  variables {
    keys = {
      auth     = { key_spec = "RSA_2048" }
      webhooks = { key_spec = "RSA_2048" }
      legacy   = { key_spec = "ECC_NIST_P384" }
    }
  }

  expect_failures = [check.key_spec_unchanged]

  assert {
    condition     = aws_kms_key.key["auth"].customer_master_key_spec == "ECC_NIST_P256"
    error_message = "A changed key_spec must not replace an existing key: auth must stay ECC_NIST_P256."
  }

  assert {
    condition     = output.key_arns["auth"] == "arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789001"
    error_message = "auth must keep its ARN."
  }
}
