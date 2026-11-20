# The schedule's dead-letter queue: one meroku creates (create_dlq), one the
# YAML names by ARN (dlq_arn), or none. Whichever it is, the scheduler role may
# send to that queue and to nothing else.
mock_provider "aws" {
  mock_data "aws_iam_policy_document" {
    defaults = { json = "{\"Version\":\"2012-10-17\",\"Statement\":[]}" }
  }
  mock_data "aws_caller_identity" {
    defaults = { account_id = "000000000000" }
  }
  mock_data "aws_region" {
    defaults = { region = "us-east-1", name = "us-east-1" }
  }
  mock_data "aws_ssm_parameters_by_path" {
    defaults = { names = [] }
  }
  mock_resource "aws_iam_role" {
    defaults = { arn = "arn:aws:iam::000000000000:role/mock" }
  }
  mock_resource "aws_iam_policy" {
    defaults = { arn = "arn:aws:iam::000000000000:policy/mock" }
  }
  mock_resource "aws_ecs_task_definition" {
    defaults = {
      arn                  = "arn:aws:ecs:us-east-1:000000000000:task-definition/mock:1"
      arn_without_revision = "arn:aws:ecs:us-east-1:000000000000:task-definition/mock"
    }
  }
  mock_resource "aws_sqs_queue" {
    defaults = { arn = "arn:aws:sqs:us-east-1:000000000000:myapp-task-cleanup-dlq-dev" }
  }
}

variables {
  project          = "myapp"
  env              = "dev"
  task             = "cleanup"
  vpc_id           = "vpc-00000000000000000"
  subnet_ids       = ["subnet-00000000000000001", "subnet-00000000000000002"]
  cluster          = "arn:aws:ecs:us-east-1:000000000000:cluster/myapp_cluster_dev"
  docker_image     = "example.com/myapp/task:synthetic"
  api_domain       = "api.example.com"
  private_dns_name = "myapp.internal"
}

run "create_dlq_makes_the_queue_and_wires_it" {
  command = apply

  variables {
    create_dlq = true
  }

  assert {
    condition     = length(aws_sqs_queue.dlq) == 1 && aws_sqs_queue.dlq[0].name == "myapp-task-cleanup-dlq-dev"
    error_message = "create_dlq must create one queue named <project>-task-<task>-dlq-<env>."
  }

  assert {
    condition     = aws_sqs_queue.dlq[0].sqs_managed_sse_enabled && aws_sqs_queue.dlq[0].message_retention_seconds == 1209600
    error_message = "The DLQ must be encrypted and keep messages for 14 days, as the events DLQs do."
  }

  assert {
    condition     = one(aws_scheduler_schedule.scheduler.target[0].dead_letter_config).arn == aws_sqs_queue.dlq[0].arn
    error_message = "The schedule's dead-letter target must be the queue meroku created."
  }

  assert {
    condition     = jsondecode(one(aws_iam_role_policy.scheduler_dlq_send).policy).Statement[0].Resource == [aws_sqs_queue.dlq[0].arn]
    error_message = "The scheduler role may send to the created queue and nothing else."
  }
}

run "dlq_arn_still_wires_an_existing_queue" {
  command = plan

  variables {
    dlq_arn = "arn:aws:sqs:us-east-1:000000000000:existing-dlq"
  }

  assert {
    condition     = length(aws_sqs_queue.dlq) == 0
    error_message = "dlq_arn names an existing queue; meroku must not create one."
  }

  assert {
    condition     = one(aws_scheduler_schedule.scheduler.target[0].dead_letter_config).arn == "arn:aws:sqs:us-east-1:000000000000:existing-dlq"
    error_message = "dlq_arn must stay the schedule's dead-letter target."
  }

  assert {
    condition     = jsondecode(one(aws_iam_role_policy.scheduler_dlq_send).policy).Statement[0].Resource == ["arn:aws:sqs:us-east-1:000000000000:existing-dlq"]
    error_message = "The scheduler role may send to the named queue and nothing else."
  }
}

run "no_dlq_creates_nothing_and_grants_nothing" {
  command = plan

  assert {
    condition = (
      length(aws_sqs_queue.dlq) == 0 &&
      length(aws_scheduler_schedule.scheduler.target[0].dead_letter_config) == 0 &&
      length(aws_iam_role_policy.scheduler_dlq_send) == 0
    )
    error_message = "Without a DLQ there is no queue, no dead-letter target and no sqs:SendMessage."
  }
}

run "create_dlq_and_dlq_arn_together_are_refused" {
  command = plan

  variables {
    create_dlq = true
    dlq_arn    = "arn:aws:sqs:us-east-1:000000000000:existing-dlq"
  }

  expect_failures = [var.dlq_arn]
}
