# HMAC keys: their usage, the MAC grants per access, no public key read, only
# the ARN variable, and a workload connected to a signing and an HMAC key.
#
# Applied against a mock provider. Every value here is synthetic. The key IDs
# are shaped like real ones: the provider validates aws_kms_public_key's key_id
# even against a mock.
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
  target = aws_kms_key.key["requests"]
  values = {
    arn    = "arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789005"
    key_id = "1234abcd-12ab-34cd-86ef-123456789005"
  }
}

override_resource {
  target = aws_kms_key.key["webhooks"]
  values = {
    arn    = "arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789006"
    key_id = "1234abcd-12ab-34cd-86ef-123456789006"
  }
}

variables {
  project = "myapp"
  env     = "dev"
  workloads = {
    backend = { task_role_name = "backend-task" }
    api     = { task_role_name = "api-task" }
    nightly = { task_role_name = "nightly-task" }
    mailer  = { task_role_name = "mailer-task" }
  }
  keys = {
    auth     = {}
    requests = { key_spec = "HMAC_256" }
    webhooks = { key_spec = "HMAC_512" }
  }
  connections = [
    { workload = "api", key = "requests", access = "sign" },
    { workload = "nightly", key = "requests", access = "verify" },
    { workload = "mailer", key = "requests", access = "both" },
    { workload = "backend", key = "auth", access = "sign" },
    { workload = "backend", key = "requests", access = "verify" },
    { workload = "backend", key = "webhooks", access = "both" },
  ]
}

run "hmac_key_usage" {
  command = apply

  assert {
    condition = (
      aws_kms_key.key["requests"].key_usage == "GENERATE_VERIFY_MAC" &&
      aws_kms_key.key["requests"].customer_master_key_spec == "HMAC_256" &&
      aws_kms_key.key["webhooks"].key_usage == "GENERATE_VERIFY_MAC" &&
      aws_kms_key.key["webhooks"].customer_master_key_spec == "HMAC_512"
    )
    error_message = "An HMAC_* key_spec must make a GENERATE_VERIFY_MAC key of that type."
  }

  assert {
    condition     = aws_kms_key.key["auth"].key_usage == "SIGN_VERIFY"
    error_message = "A signing key beside HMAC keys must stay SIGN_VERIFY."
  }

  assert {
    condition = (
      aws_kms_key.key["requests"].description == "myapp dev HMAC key requests (meroku)" &&
      aws_kms_key.key["auth"].description == "myapp dev signing key auth (meroku)"
    )
    error_message = "An HMAC key is described as an HMAC key, a signing key as a signing key."
  }

  assert {
    condition     = aws_kms_key.key["requests"].enable_key_rotation == false && aws_kms_key.key["requests"].deletion_window_in_days == 7
    error_message = "An HMAC key has no rotation and a 7-day deletion window."
  }

  assert {
    condition     = output.aliases["requests"] == "alias/myapp-dev-requests"
    error_message = "An HMAC key gets the alias alias/<project>-<env>-<name>."
  }
}

run "no_public_key_read_for_hmac_keys" {
  command = apply

  assert {
    condition     = keys(module.public_keys.public_keys) == ["auth"]
    error_message = "Only asymmetric keys may be passed to the public key read: reading an HMAC key's public key fails."
  }
}

run "mac_grants_per_access" {
  command = apply

  assert {
    condition     = output.policies["api"] == "{\"Statement\":[{\"Action\":[\"kms:GenerateMac\"],\"Effect\":\"Allow\",\"Resource\":[\"arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789005\"],\"Sid\":\"KmsGenerateMac\"},{\"Action\":[\"kms:DescribeKey\"],\"Effect\":\"Allow\",\"Resource\":[\"arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789005\"],\"Sid\":\"KmsDescribeMacKey\"}],\"Version\":\"2012-10-17\"}"
    error_message = "sign on an HMAC key grants kms:GenerateMac and kms:DescribeKey, and nothing else."
  }

  assert {
    condition = (
      toset(flatten([for st in jsondecode(output.policies["nightly"]).Statement : st.Action])) ==
      toset(["kms:VerifyMac", "kms:DescribeKey"])
    )
    error_message = "verify on an HMAC key grants kms:VerifyMac and kms:DescribeKey, and nothing else."
  }

  assert {
    condition = (
      [for st in jsondecode(output.policies["mailer"]).Statement : st.Sid] == ["KmsGenerateMac", "KmsVerifyMac", "KmsDescribeMacKey"] &&
      toset(flatten([for st in jsondecode(output.policies["mailer"]).Statement : st.Action])) ==
      toset(["kms:GenerateMac", "kms:VerifyMac", "kms:DescribeKey"])
    )
    error_message = "both on an HMAC key grants GenerateMac, VerifyMac and DescribeKey."
  }

  assert {
    condition = !anytrue([
      for w in ["api", "nightly", "mailer"] :
      strcontains(output.policies[w], "kms:GetPublicKey") || strcontains(output.policies[w], "kms:Sign") || strcontains(output.policies[w], "\"kms:Verify\"")
    ])
    error_message = "A workload connected only to HMAC keys gets no signing action and no kms:GetPublicKey."
  }
}

run "mixed_workload" {
  command = apply

  assert {
    condition = jsondecode(output.policies["backend"]).Statement == [
      {
        Sid      = "KmsSign"
        Effect   = "Allow"
        Action   = ["kms:Sign"]
        Resource = ["arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789001"]
      },
      {
        Sid      = "KmsGenerateMac"
        Effect   = "Allow"
        Action   = ["kms:GenerateMac"]
        Resource = ["arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789006"]
      },
      {
        Sid    = "KmsVerifyMac"
        Effect = "Allow"
        Action = ["kms:VerifyMac"]
        Resource = [
          "arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789005",
          "arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789006",
        ]
      },
      {
        Sid      = "KmsPublicKey"
        Effect   = "Allow"
        Action   = ["kms:GetPublicKey", "kms:DescribeKey"]
        Resource = ["arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789001"]
      },
      {
        Sid    = "KmsDescribeMacKey"
        Effect = "Allow"
        Action = ["kms:DescribeKey"]
        Resource = [
          "arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789005",
          "arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789006",
        ]
      },
    ]
    error_message = "A workload on a signing and HMAC keys gets signing actions on the signing key, MAC actions on the HMAC keys, GetPublicKey only on the signing key, and DescribeKey on every key."
  }

  assert {
    condition = [for e in output.environment["backend"] : e.name] == [
      "KMS_AUTH_KEY_ARN", "KMS_AUTH_PUBLIC_KEY", "KMS_REQUESTS_KEY_ARN", "KMS_WEBHOOKS_KEY_ARN",
    ]
    error_message = "A mixed workload gets the public key of its signing key only, and the ARN of every key."
  }
}

run "hmac_key_gives_only_its_arn" {
  command = apply

  assert {
    condition = output.environment["api"] == [
      { name = "KMS_REQUESTS_KEY_ARN", value = "arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789005" },
    ]
    error_message = "A workload connected to an HMAC key gets KMS_<NAME>_KEY_ARN and no public key variable."
  }
}

# A key created as signing keeps its type and usage when key_spec turns it into
# an HMAC key: no replacement, the plan warns, and its grants and variables stay
# those of a signing key.
run "switch_before" {
  command = apply

  variables {
    keys        = { auth = {} }
    connections = [{ workload = "api", key = "auth", access = "sign" }]
  }
}

run "switch_from_signing_to_hmac_is_ignored_and_warned" {
  command = apply

  variables {
    keys        = { auth = { key_spec = "HMAC_256" } }
    connections = [{ workload = "api", key = "auth", access = "sign" }]
  }

  expect_failures = [check.key_spec_unchanged]

  assert {
    condition = (
      aws_kms_key.key["auth"].customer_master_key_spec == "ECC_NIST_P256" &&
      aws_kms_key.key["auth"].key_usage == "SIGN_VERIFY"
    )
    error_message = "A switch to an HMAC key_spec must not replace the key: it stays ECC_NIST_P256 and SIGN_VERIFY."
  }

  assert {
    condition = (
      [for st in jsondecode(output.policies["api"]).Statement : st.Sid] == ["KmsSign", "KmsPublicKey"] &&
      [for e in output.environment["api"] : e.name] == ["KMS_AUTH_KEY_ARN", "KMS_AUTH_PUBLIC_KEY"]
    )
    error_message = "Grants and variables follow the key's real type, not the key_spec the check warns about."
  }
}
