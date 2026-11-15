# Adding or deleting a key must leave every other key's public key known at
# plan. An unknown public key makes every container definition that names it
# unknown, so ECS replaces the task definition of each connected workload
# although nothing in it changed.
#
# Each plan run compares the PEM it plans with the one the previous apply
# stored. A PEM read during apply is unknown at plan, and an assert on an
# unknown value fails the run. Every value here is synthetic.
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
  target = aws_kms_key.key["extra"]
  values = {
    arn    = "arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-86ef-123456789004"
    key_id = "1234abcd-12ab-34cd-86ef-123456789004"
  }
}

variables {
  project = "myapp"
  env     = "dev"
  workloads = {
    backend = { task_role_name = "backend-task" }
    api     = { task_role_name = "api-task" }
  }
  keys        = { auth = {} }
  connections = [{ workload = "backend", key = "auth", access = "both" }]
}

run "one_key" {
  command = apply
}

run "adding_a_key_keeps_the_others_known" {
  command = plan

  variables {
    keys = { auth = {}, extra = {} }
    connections = [
      { workload = "backend", key = "auth", access = "both" },
      { workload = "api", key = "extra", access = "sign" },
    ]
  }

  assert {
    condition     = output.environment["backend"] == run.one_key.environment["backend"]
    error_message = "Adding a key must leave the variables of a workload connected only to another key unchanged and known at plan."
  }
}

run "two_keys" {
  command = apply

  variables {
    keys = { auth = {}, extra = {} }
    connections = [
      { workload = "backend", key = "auth", access = "both" },
      { workload = "api", key = "extra", access = "sign" },
    ]
  }
}

run "deleting_a_key_keeps_the_others_known" {
  command = plan

  assert {
    condition     = output.environment["backend"] == run.two_keys.environment["backend"]
    error_message = "Deleting a key must leave the variables of a workload connected only to another key unchanged and known at plan."
  }
}
