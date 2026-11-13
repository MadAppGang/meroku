# Built-in IAM is read-only and container bus/source follow their contracts.
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
}

variables {
  project          = one([for v in jsondecode(file("../../app/testdata/event_source_vectors.json")).vectors : v.project if v.kind == "event_task"])
  task             = one([for v in jsondecode(file("../../app/testdata/event_source_vectors.json")).vectors : v.workload if v.kind == "event_task"])
  env              = "dev"
  vpc_id           = "vpc-00000000000000000"
  subnet_ids       = ["subnet-00000000000000001", "subnet-00000000000000002"]
  cluster          = "arn:aws:ecs:us-east-1:000000000000:cluster/myapp_cluster_dev"
  docker_image     = "example.com/myapp/task:synthetic"
  api_domain       = "api.example.com"
  private_dns_name = "myapp.internal"
  rules            = { orders = { sources = ["myapp.orders"], detail_types = ["order.created"] } }
}

run "task_event_policy_is_strict_and_bus_and_source_reach_the_container" {
  command = plan

  variables {
    event_bus_name = "myapp-dev"
  }

  assert {
    condition = alltrue([
      for policy in [aws_iam_policy.eventbridge_access.policy] :
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
      one([for e in jsondecode(aws_ecs_task_definition.task.container_definitions)[0].environment : e.value if e.name == "EVENT_BUS_NAME"]) == "myapp-dev" &&
      one([for e in jsondecode(aws_ecs_task_definition.task.container_definitions)[0].environment : e.value if e.name == "EVENT_SOURCE"]) ==
      one([for v in jsondecode(file("../../app/testdata/event_source_vectors.json")).vectors : v.want if v.kind == "event_task"])
    )
    error_message = "EVENT_BUS_NAME must follow the input bus and EVENT_SOURCE must equal the shared source vector for this task kind."
  }
}

run "bus_defaults_to_default" {
  command = plan

  assert {
    condition     = one([for e in jsondecode(aws_ecs_task_definition.task.container_definitions)[0].environment : e.value if e.name == "EVENT_BUS_NAME"]) == "default"
    error_message = "Without an event_bus_name input the container must still receive default."
  }
}

run "rules_and_targets_use_their_resolved_bus" {
  command = plan

  variables {
    event_bus_name = "myapp-dev"
    rules = {
      inherited = { sources = ["myapp.orders"], detail_types = ["order.created"] }
      explicit  = { sources = ["aws.s3"], detail_types = ["Object Created"], event_bus_name = "default" }
    }
  }

  assert {
    condition = (
      toset(keys(aws_cloudwatch_event_rule.rule)) == toset(["inherited", "explicit"]) &&
      aws_cloudwatch_event_rule.rule["inherited"].event_bus_name == "myapp-dev" &&
      aws_cloudwatch_event_target.target["inherited"].event_bus_name == "myapp-dev" &&
      aws_cloudwatch_event_rule.rule["explicit"].event_bus_name == "default" &&
      aws_cloudwatch_event_target.target["explicit"].event_bus_name == "default" &&
      alltrue([for k, target in aws_cloudwatch_event_target.target : target.rule == aws_cloudwatch_event_rule.rule[k].name])
    )
    error_message = "Both rule and target must use the inherited project bus or explicit default bus, without cross-bus rule references."
  }
}

run "legacy_rule_inherits_the_input_bus" {
  command = plan

  variables {
    event_bus_name = "myapp-dev"
    rules          = {}
    rule_name      = "legacy"
    sources        = ["myapp.orders"]
    detail_types   = ["order.created"]
  }

  assert {
    condition = (
      length(aws_cloudwatch_event_rule.rule) == 1 &&
      length(aws_cloudwatch_event_target.target) == 1 &&
      alltrue([for rule in aws_cloudwatch_event_rule.rule : rule.event_bus_name == "myapp-dev"]) &&
      alltrue([for target in aws_cloudwatch_event_target.target : target.event_bus_name == "myapp-dev"])
    )
    error_message = "The legacy single-rule form must use event_bus_name for both the rule and target."
  }
}
