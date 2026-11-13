# Backend and services use strict built-in policies and shared source vectors.
mock_provider "aws" {
  # These policy-document data sources support unrelated trust and ECR fixtures.
  # Policies asserted below must retain their nonempty read actions.
  mock_data "aws_iam_policy_document" {
    defaults = { json = "{\"Version\":\"2012-10-17\",\"Statement\":[]}" }
  }
  mock_data "aws_caller_identity" {
    defaults = { account_id = "000000000000" }
  }
  mock_data "aws_region" {
    defaults = { region = "us-east-1", name = "us-east-1" }
  }
  mock_data "aws_organizations_organization" {
    defaults = { id = "o-000000000000" }
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
  mock_data "aws_vpc" {
    defaults = { cidr_block = "10.0.0.0/16" }
  }
  mock_resource "aws_cloudwatch_log_group" {
    defaults = { arn = "arn:aws:logs:us-east-1:000000000000:log-group:mock" }
  }
  mock_resource "aws_service_discovery_service" {
    defaults = { arn = "arn:aws:servicediscovery:us-east-1:000000000000:service/srv-mock" }
  }
  mock_resource "aws_lambda_function" {
    defaults = { arn = "arn:aws:lambda:us-east-1:000000000000:function:mock" }
  }
  mock_resource "aws_cloudwatch_event_rule" {
    defaults = { arn = "arn:aws:events:us-east-1:000000000000:rule/mock" }
  }
}
mock_provider "null" {}
mock_provider "random" {}
override_resource {
  target = null_resource.build_ci_lambda
}

# Keep computed S3 identity consistent with the configured synthetic bucket.
# File scope supplies the same identity before any apply creates shared state.
override_resource {
  target = aws_s3_bucket.backend
  values = {
    id  = "myapp-backend-dev"
    arn = "arn:aws:s3:::myapp-backend-dev"
  }
}

variables {
  project          = one([for v in jsondecode(file("../../app/testdata/event_source_vectors.json")).vectors : v.project if v.kind == "backend"])
  env              = "dev"
  vpc_id           = "vpc-00000000000000000"
  subnet_ids       = ["subnet-00000000000000001", "subnet-00000000000000002"]
  private_dns_name = "myapp.internal"
  api_domain       = "api.example.com"
  github_subjects  = ["repo:myapp/app:ref:refs/heads/main"]
  docker_image     = "example.com/myapp/backend:synthetic"
  services = [
    for v in jsondecode(file("../../app/testdata/event_source_vectors.json")).vectors : {
      name         = v.workload
      docker_image = "example.com/myapp/service:synthetic"
      env_vars     = {}
    } if v.kind == "service"
  ]
}

run "workload_event_policies_are_strict_and_bus_and_sources_reach_containers" {
  # The own-path data reads are deferred until the configured /env parameters exist.
  command = apply

  variables {
    event_bus_name = "myapp-dev"
  }

  assert {
    condition = alltrue([
      for policy in concat([aws_iam_policy.backend_eventbridge.policy], [for p in aws_iam_policy.services_eventbridge : p.policy]) :
      toset(flatten([for st in jsondecode(policy).Statement : try(tolist(st.Action), [tostring(st.Action)])])) == toset([
        "events:DescribeEventBus", "events:DescribeRule", "events:ListTargetsByRule", "events:ListRules", "events:ListEventBuses"
      ]) &&
      toset(flatten([for st in jsondecode(policy).Statement : try(tolist(st.Resource), [tostring(st.Resource)])])) == toset([
        "arn:aws:events:us-east-1:000000000000:event-bus/default",
        "arn:aws:events:us-east-1:000000000000:event-bus/myapp-dev",
        "arn:aws:events:us-east-1:000000000000:rule/*"
      ])
    ])
    error_message = "Built-in EventBridge policies must preserve only the documented read actions on default, exact project bus and scoped rules: no PutEvents, infrastructure writes, events:* or newly widened resources."
  }

  assert {
    condition = (
      toset(keys(aws_iam_policy.services_eventbridge)) == toset([
        for v in jsondecode(file("../../app/testdata/event_source_vectors.json")).vectors : v.workload if v.kind == "service"
      ]) &&
      alltrue([
        for v in jsondecode(file("../../app/testdata/event_source_vectors.json")).vectors :
        one([
          for e in jsondecode(v.kind == "backend" ? aws_ecs_task_definition.backend.container_definitions : aws_ecs_task_definition.services[v.workload].container_definitions)[0].environment :
          e.value if e.name == "EVENT_SOURCE"
        ]) == v.want &&
        one([
          for e in jsondecode(v.kind == "backend" ? aws_ecs_task_definition.backend.container_definitions : aws_ecs_task_definition.services[v.workload].container_definitions)[0].environment :
          e.value if e.name == "EVENT_BUS_NAME"
        ]) == "myapp-dev"
        if contains(["backend", "service"], v.kind)
      ])
    )
    error_message = "Backend and every service vector, including a hyphenated service, must keep its exact EVENT_SOURCE and receive the input bus."
  }
}

run "bus_defaults_to_default" {
  # Applying with mocks resolves the backend's deferred discovery before decoding JSON.
  command = apply

  assert {
    condition     = one([for e in jsondecode(aws_ecs_task_definition.backend.container_definitions)[0].environment : e.value if e.name == "EVENT_BUS_NAME"]) == "default"
    error_message = "The backend must still receive default when no event_bus_name input is supplied."
  }
}

run "backend_bucket_keeps_its_existing_managed_grant" {
  command = apply

  # Compare against the configured bucket, whose computed identity is pinned
  # above; leave the policy document and attachment under test unmocked.
  assert {
    condition = (
      toset(flatten([for st in jsondecode(aws_iam_policy.full_access_to_backend_bucket.policy).Statement : try(tolist(st.Action), [tostring(st.Action)])])) == toset(["s3:*"]) &&
      toset(flatten([for st in jsondecode(aws_iam_policy.full_access_to_backend_bucket.policy).Statement : try(tolist(st.Resource), [tostring(st.Resource)])])) ==
      toset(["arn:aws:s3:::${aws_s3_bucket.backend.bucket}", "arn:aws:s3:::${aws_s3_bucket.backend.bucket}/*"]) &&
      alltrue([for st in jsondecode(aws_iam_policy.full_access_to_backend_bucket.policy).Statement : st.Effect == "Allow" && !contains(keys(st), "Condition")]) &&
      aws_iam_role_policy_attachment.backend_task_backend_bucket.role == aws_iam_role.backend_task.name &&
      aws_iam_role_policy_attachment.backend_task_backend_bucket.policy_arn == aws_iam_policy.full_access_to_backend_bucket.arn
    )
    error_message = "The backend must retain its existing managed s3:* grant on its own backend bucket and task role; the data module must not replace or narrow it."
  }
}
