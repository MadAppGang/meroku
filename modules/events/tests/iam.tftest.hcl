# The inline policy modules/events puts on each consumer's and producer's task
# role, planned on the Terraform version CI pins (.github/workflows/ci.yml).
#
# A workload that subscribes and does not produce is the common consumer. Its
# policy's precondition compares a producer's source with the workload's
# EVENT_SOURCE, and must not index local.producers for a workload that is not
# in it: Terraform before 1.12 evaluates both sides of `||`, so an index guarded
# only by `!contains(...) ||` fails with "Invalid index" for every consumer-only
# workload (consumer_only_workload_plans).
#
# Run: terraform test  (from modules/events)

mock_provider "aws" {
  mock_data "aws_region" {
    defaults = {
      region = "us-east-1"
    }
  }

  mock_data "aws_caller_identity" {
    defaults = {
      account_id = "000000000000"
    }
  }

  # ARNs are unknown until apply; these are the values the apply-mode run reads.
  # Account 000000000000 is synthetic; this repository is public.
  mock_resource "aws_cloudwatch_event_bus" {
    defaults = {
      arn = "arn:aws:events:us-east-1:000000000000:event-bus/acme-dev"
    }
  }

  mock_resource "aws_sqs_queue" {
    defaults = {
      arn = "arn:aws:sqs:us-east-1:000000000000:mock"
    }
  }

  mock_resource "aws_sns_topic" {
    defaults = {
      arn = "arn:aws:sns:us-east-1:000000000000:mock"
    }
  }

  mock_resource "aws_cloudwatch_event_rule" {
    defaults = {
      arn = "arn:aws:events:us-east-1:000000000000:rule/acme-dev/mock"
    }
  }
}

variables {
  project     = "acme"
  env         = "dev"
  bus_enabled = true

  workloads = {
    backend  = { task_role_name = "acme_backend_task_dev", event_source = "acme.backend" }
    api      = { task_role_name = "acme_service_api_task_dev", event_source = "acme.api" }
    listener = { task_role_name = "acme_service_listener_task_dev", event_source = "acme.listener" }
  }

  producers = [{
    name    = "api"
    source  = "acme.api"
    entries = [{ bus = "project", detail_types = ["order.created"] }]
  }]

  consumers = [{
    name = "listener"
    subscriptions = [{
      key         = "order-created"
      detail_type = "order.created"
      bus         = "project"
    }]
  }]
}

run "consumer_only_workload_plans" {
  command = plan

  assert {
    condition     = toset(keys(aws_iam_role_policy.workload)) == toset(["api", "listener"])
    error_message = "Want one events policy for the producer api and one for the consumer-only listener."
  }

  assert {
    condition     = aws_iam_role_policy.workload["listener"].role == "acme_service_listener_task_dev"
    error_message = "The consumer's policy belongs on its own task role."
  }
}

# Apply against the mocks: the project bus ARN is unknown at plan. No AWS is
# reached; the apply and its teardown are mocked.
run "producer_policy_is_scoped_puts" {
  command = apply

  assert {
    condition = alltrue([
      for s in jsondecode(aws_iam_role_policy.workload["api"].policy).Statement :
      s.Action == "events:PutEvents" && s.Resource == "arn:aws:events:us-east-1:000000000000:event-bus/acme-dev"
    ])
    error_message = "The producer's policy grants only PutEvents, on the project bus."
  }

  assert {
    condition     = jsondecode(aws_iam_role_policy.workload["api"].policy).Statement[0].Condition["ForAllValues:StringEquals"]["events:detail-type"] == ["order.created"]
    error_message = "PutEvents is limited to the produces entry's detail types."
  }
}

run "drifted_producer_source_is_refused" {
  command = plan

  variables {
    producers = [{
      name    = "api"
      source  = "acme.other"
      entries = [{ bus = "project", detail_types = ["order.created"] }]
    }]
  }

  expect_failures = [aws_iam_role_policy.workload["api"]]
}
