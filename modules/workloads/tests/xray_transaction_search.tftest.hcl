# X-Ray Transaction Search (xray_transaction_search.tf, cloudwatch_access.tf).
#
# When the backend or any service has X-Ray on, the module creates one
# CloudWatch Logs resource policy that lets X-Ray write spans, one trace
# segment destination (CloudWatchLogs) and one indexing rule named Default at
# var.xray_indexing_percentage. Every task role with X-Ray on gets an XRaySpans
# statement. With X-Ray off nothing is created and no role gets XRaySpans.
#
#   1. Backend and one service on: resources, policy, rule and grants.
#      (backend_and_service_xray)
#   2. A service alone turns it on, the backend gets no XRaySpans.
#      (service_only_xray)
#   3. No X-Ray: no resources, no XRaySpans. (no_xray_creates_nothing)
#   4. The percentage reaches the rule. 0 and 100 are accepted, -1, 100.5 and
#      101 are refused by variable validation.
#
# Runs APPLY against the mocks: a log group's ARN is unknown during a plan. No
# AWS is reached. Account 000000000000 is synthetic.
#
# Run: terraform test  (from modules/workloads)

mock_provider "aws" {
  mock_data "aws_iam_policy_document" {
    defaults = {
      json = "{\"Version\":\"2012-10-17\",\"Statement\":[]}"
    }
  }

  mock_data "aws_vpc" {
    defaults = {
      cidr_block = "10.0.0.0/16"
    }
  }

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

  mock_resource "aws_iam_role" {
    defaults = {
      arn = "arn:aws:iam::000000000000:role/mock"
    }
  }
  mock_resource "aws_iam_policy" {
    defaults = {
      arn = "arn:aws:iam::000000000000:policy/mock"
    }
  }
  mock_resource "aws_cloudwatch_log_group" {
    defaults = {
      arn = "arn:aws:logs:us-east-1:000000000000:log-group:mock"
    }
  }
  mock_resource "aws_service_discovery_service" {
    defaults = {
      arn = "arn:aws:servicediscovery:us-east-1:000000000000:service/srv-mock"
    }
  }
  mock_resource "aws_lambda_function" {
    defaults = {
      arn = "arn:aws:lambda:us-east-1:000000000000:function:mock"
    }
  }
  mock_resource "aws_cloudwatch_event_rule" {
    defaults = {
      arn = "arn:aws:events:us-east-1:000000000000:rule/mock"
    }
  }
}

mock_provider "null" {}
mock_provider "random" {}

variables {
  # With autoscaling off, terraform_data.backend_desired_count runs the AWS CLI
  # in a local-exec. With it on, that resource has no instance.
  backend_autoscaling_enabled = true

  project          = "myapp"
  env              = "dev"
  vpc_id           = "vpc-00000000000000000"
  subnet_ids       = ["subnet-00000000000000001", "subnet-00000000000000002"]
  private_dns_name = "myapp.internal"
  api_domain       = "api.example.com"
  github_subjects  = ["repo:myapp/myapp:ref:refs/heads/main"]

  services = [
    { name = "orders" },
    { name = "traced", xray_enabled = true },
  ]
}

# REQ-2, REQ-3, REQ-4 (default 1)
run "backend_and_service_xray" {
  command = apply

  variables {
    xray_enabled = true
  }

  # The CI Lambda build is a local provisioner; keep it out of the apply.
  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition = (
      length(aws_cloudwatch_log_resource_policy.xray_transaction_search) == 1 &&
      length(aws_xray_trace_segment_destination.transaction_search) == 1 &&
      length(aws_xray_indexing_rule.default) == 1
    )
    error_message = "With X-Ray on, exactly one log resource policy, one trace segment destination and one indexing rule are created."
  }

  assert {
    condition     = aws_cloudwatch_log_resource_policy.xray_transaction_search[0].policy_name == "myapp-dev-xray-transaction-search"
    error_message = "The log resource policy is named <project>-<env>-xray-transaction-search."
  }

  assert {
    condition     = length(jsondecode(aws_cloudwatch_log_resource_policy.xray_transaction_search[0].policy_document).Statement) == 1
    error_message = "The log resource policy has exactly one statement."
  }

  assert {
    condition = alltrue([
      for s in jsondecode(aws_cloudwatch_log_resource_policy.xray_transaction_search[0].policy_document).Statement :
      toset(flatten([s.Principal.Service])) == toset(["xray.amazonaws.com"]) &&
      toset(flatten([s.Action])) == toset(["logs:PutLogEvents"])
    ])
    error_message = "The log resource policy lets the xray.amazonaws.com service call logs:PutLogEvents only."
  }

  assert {
    condition = alltrue([
      for s in jsondecode(aws_cloudwatch_log_resource_policy.xray_transaction_search[0].policy_document).Statement :
      toset(flatten([s.Resource])) == toset([
        "arn:aws:logs:us-east-1:000000000000:log-group:aws/spans:*",
        "arn:aws:logs:us-east-1:000000000000:log-group:/aws/application-signals/data:*",
      ]) && length(flatten([s.Resource])) == 2
    ])
    error_message = "The log resource policy covers the aws/spans and /aws/application-signals/data log groups only."
  }

  assert {
    condition = alltrue([
      for s in jsondecode(aws_cloudwatch_log_resource_policy.xray_transaction_search[0].policy_document).Statement :
      toset(flatten([s.Condition.ArnLike["aws:SourceArn"]])) == toset(["arn:aws:xray:us-east-1:000000000000:*"]) &&
      toset(flatten([s.Condition.StringEquals["aws:SourceAccount"]])) == toset(["000000000000"])
    ])
    error_message = "The log resource policy is limited to X-Ray in this account and region (ArnLike aws:SourceArn, StringEquals aws:SourceAccount)."
  }

  assert {
    condition     = aws_xray_trace_segment_destination.transaction_search[0].destination == "CloudWatchLogs"
    error_message = "Trace segments go to CloudWatchLogs."
  }

  assert {
    condition     = aws_xray_indexing_rule.default[0].name == "Default"
    error_message = "The indexing rule is the account's Default rule."
  }

  assert {
    condition     = aws_xray_indexing_rule.default[0].rule[0].probabilistic[0].desired_sampling_percentage == 1
    error_message = "Without xray_indexing_percentage the Default rule indexes 1 percent."
  }

  # XRaySpans: exactly the three actions on "*", on the backend and on traced.
  assert {
    condition = length([
      for s in jsondecode(aws_iam_role_policy.backend_task_cloudwatch.policy).Statement : s
      if s.Sid == "XRaySpans" &&
      toset(flatten([s.Action])) == toset(["xray:PutSpans", "xray:PutSpansForIndexing", "xray:PutTraceSegments"]) &&
      length(flatten([s.Action])) == 3 &&
      toset(flatten([s.Resource])) == toset(["*"])
    ]) == 1
    error_message = "The backend with xray_enabled gets one XRaySpans statement: xray:PutSpans, xray:PutSpansForIndexing, xray:PutTraceSegments on *."
  }

  assert {
    condition = length([
      for s in jsondecode(aws_iam_role_policy.services_task_cloudwatch["traced"].policy).Statement : s
      if s.Sid == "XRaySpans" &&
      toset(flatten([s.Action])) == toset(["xray:PutSpans", "xray:PutSpansForIndexing", "xray:PutTraceSegments"]) &&
      length(flatten([s.Action])) == 3 &&
      toset(flatten([s.Resource])) == toset(["*"])
    ]) == 1
    error_message = "A service with xray_enabled gets one XRaySpans statement: xray:PutSpans, xray:PutSpansForIndexing, xray:PutTraceSegments on *."
  }

  assert {
    condition = !contains(
      [for s in jsondecode(aws_iam_role_policy.services_task_cloudwatch["orders"].policy).Statement : s.Sid],
      "XRaySpans",
    )
    error_message = "A service without xray_enabled gets no XRaySpans, even when the backend has X-Ray."
  }

  # Existing statements stay.
  assert {
    condition = contains(
      [for s in jsondecode(aws_iam_role_policy.services_task_cloudwatch["traced"].policy).Statement : s.Sid],
      "XRayWrite",
    )
    error_message = "The X-Ray service keeps its XRayWrite statement."
  }

  assert {
    condition = contains(
      [for s in jsondecode(aws_iam_role_policy.backend_task_cloudwatch.policy).Statement : s.Sid],
      "ContainerInsightsLogs",
    )
    error_message = "The X-Ray backend keeps its Container Insights statement."
  }
}

# REQ-1, REQ-2, REQ-3
run "service_only_xray" {
  command = apply

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition = (
      length(aws_cloudwatch_log_resource_policy.xray_transaction_search) == 1 &&
      length(aws_xray_trace_segment_destination.transaction_search) == 1 &&
      length(aws_xray_indexing_rule.default) == 1
    )
    error_message = "A service with xray_enabled alone makes the env use X-Ray: one of each Transaction Search resource."
  }

  assert {
    condition = !contains(
      [for s in jsondecode(aws_iam_role_policy.backend_task_cloudwatch.policy).Statement : s.Sid],
      "XRaySpans",
    )
    error_message = "A backend without xray_enabled gets no XRaySpans."
  }

  assert {
    condition = contains(
      [for s in jsondecode(aws_iam_role_policy.services_task_cloudwatch["traced"].policy).Statement : s.Sid],
      "XRaySpans",
    )
    error_message = "A service with xray_enabled gets XRaySpans."
  }
}

# REQ-2, REQ-3
run "no_xray_creates_nothing" {
  command = apply

  variables {
    services = [
      { name = "orders" },
      { name = "billing", xray_enabled = false },
    ]
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition = (
      length(aws_cloudwatch_log_resource_policy.xray_transaction_search) == 0 &&
      length(aws_xray_trace_segment_destination.transaction_search) == 0 &&
      length(aws_xray_indexing_rule.default) == 0
    )
    error_message = "Without X-Ray none of the Transaction Search resources exist."
  }

  assert {
    condition = !anytrue([
      for p in concat(
        [aws_iam_role_policy.backend_task_cloudwatch.policy],
        [for v in values(aws_iam_role_policy.services_task_cloudwatch) : v.policy],
      ) : contains([for s in jsondecode(p).Statement : s.Sid], "XRaySpans")
    ])
    error_message = "Without X-Ray no task role gets XRaySpans."
  }
}

# REQ-2, REQ-4
run "custom_indexing_percentage" {
  command = apply

  variables {
    xray_enabled             = true
    xray_indexing_percentage = 50.5
  }

  override_resource {
    target = null_resource.build_ci_lambda
  }

  assert {
    condition     = aws_xray_indexing_rule.default[0].rule[0].probabilistic[0].desired_sampling_percentage == 50.5
    error_message = "The Default rule indexes xray_indexing_percentage percent."
  }
}

# REQ-4: the inclusive bounds are accepted.
run "indexing_percentage_zero_is_accepted" {
  command = plan

  variables {
    xray_enabled             = true
    xray_indexing_percentage = 0
  }

  assert {
    condition     = var.xray_indexing_percentage == 0
    error_message = "0 is a valid xray_indexing_percentage."
  }
}

run "indexing_percentage_hundred_is_accepted" {
  command = plan

  variables {
    xray_enabled             = true
    xray_indexing_percentage = 100
  }

  assert {
    condition     = var.xray_indexing_percentage == 100
    error_message = "100 is a valid xray_indexing_percentage."
  }
}

# REQ-4: outside 0..100 is refused.
run "indexing_percentage_negative_is_refused" {
  command = plan

  variables {
    xray_enabled             = true
    xray_indexing_percentage = -1
  }

  expect_failures = [var.xray_indexing_percentage]
}

run "indexing_percentage_above_hundred_is_refused" {
  command = plan

  variables {
    xray_enabled             = true
    xray_indexing_percentage = 100.5
  }

  expect_failures = [var.xray_indexing_percentage]
}

run "indexing_percentage_101_is_refused" {
  command = plan

  variables {
    xray_enabled             = true
    xray_indexing_percentage = 101
  }

  expect_failures = [var.xray_indexing_percentage]
}

# Use fresh mock state so earlier task definitions cannot supply a case's containers.
# REQ-1, REQ-2, REQ-3
run "xray_spans_alone_turns_on_transaction_search" {
  command   = apply
  state_key = "xray_spans_alone_turns_on_transaction_search"

  override_resource {
    target = null_resource.build_ci_lambda
  }

  variables {
    backend_xray_spans       = true
    xray_indexing_percentage = 10
    services = [
      { name = "orders" },
      { name = "otlp", xray_spans = true }
    ]
  }

  assert {
    condition = (
      length(aws_cloudwatch_log_resource_policy.xray_transaction_search) == 1 &&
      length(aws_xray_trace_segment_destination.transaction_search) == 1 &&
      length(aws_xray_indexing_rule.default) == 1
    )
    error_message = "Expected 1 of each Transaction Search resource (logs policy, segment destination and indexing rule)."
  }

  assert {
    condition     = one(aws_xray_indexing_rule.default[*].rule[0].probabilistic[0].desired_sampling_percentage) == 10
    error_message = "The Default indexing rule must use 10 percent."
  }

  assert {
    condition = sort(flatten([
      for name, policy in merge(
        { backend = aws_iam_role_policy.backend_task_cloudwatch.policy },
        { for name, p in aws_iam_role_policy.services_task_cloudwatch : name => p.policy }
        ) : [
        for s in jsondecode(policy).Statement : name if s.Sid == "XRaySpans"
      ]
    ])) == sort(["backend", "otlp"])
    error_message = "Only backend and otlp should have XRaySpans, exactly once each."
  }

  assert {
    condition = alltrue(flatten([
      for defs in concat(
        [aws_ecs_task_definition.backend.container_definitions],
        [for svc in aws_ecs_task_definition.services : svc.container_definitions]
        ) : [
        for c in jsondecode(defs) : !strcontains(c.name, "adot-collector") && !strcontains(c.name, "xray-daemon")
      ]
    ]))
    error_message = "Without xray_enabled, neither the backend nor a named service should have an X-Ray sidecar."
  }

  assert {
    condition = alltrue(flatten([
      for policy in [
        aws_iam_role_policy.backend_task_cloudwatch.policy,
        aws_iam_role_policy.services_task_cloudwatch["otlp"].policy
        ] : [
        for s in jsondecode(policy).Statement :
        s.Effect == "Allow" && toset(flatten([s.Resource])) == toset(["*"]) &&
        toset(flatten([s.Action])) == toset(["xray:PutSpans", "xray:PutSpansForIndexing", "xray:PutTraceSegments"])
        if s.Sid == "XRaySpans"
      ]
    ]))
    error_message = "Each selected role must be allowed exactly the three span actions on *."
  }

  assert {
    condition = (
      aws_cloudwatch_log_resource_policy.xray_transaction_search[0].policy_name == "myapp-dev-xray-transaction-search" &&
      aws_xray_indexing_rule.default[0].name == "Default" &&
      aws_xray_trace_segment_destination.transaction_search[0].destination == "CloudWatchLogs"
    )
    error_message = "Sidecar-free ownership must configure the same policy, Default rule and CloudWatchLogs destination."
  }

  assert {
    condition = (
      length(aws_cloudwatch_log_group.adot_collector) == 0 &&
      !contains([for s in jsondecode(aws_iam_role_policy.backend_task_cloudwatch.policy).Statement : s.Sid], "ContainerInsightsLogs") &&
      !contains([for s in jsondecode(aws_iam_role_policy.services_task_cloudwatch["otlp"].policy).Statement : s.Sid], "XRayWrite")
    )
    error_message = "Span senders should not need collector log groups or sidecar-only task role permissions."
  }
}

# REQ-1, REQ-2, REQ-3
run "service_xray_spans_alone" {
  command   = apply
  state_key = "service_xray_spans_alone"

  override_resource {
    target = null_resource.build_ci_lambda
  }

  variables {
    services = [
      { name = "orders" },
      { name = "otlp", xray_spans = true }
    ]
  }

  assert {
    condition = (
      length(aws_cloudwatch_log_resource_policy.xray_transaction_search) == 1 &&
      length(aws_xray_trace_segment_destination.transaction_search) == 1 &&
      length(aws_xray_indexing_rule.default) == 1
    )
    error_message = "Expected 1 of each Transaction Search resource (logs policy, segment destination and indexing rule)."
  }

  assert {
    condition     = one(aws_xray_indexing_rule.default[*].rule[0].probabilistic[0].desired_sampling_percentage) == 1
    error_message = "The Default indexing rule must use 1 percent."
  }

  assert {
    condition = sort(flatten([
      for name, policy in merge(
        { backend = aws_iam_role_policy.backend_task_cloudwatch.policy },
        { for name, p in aws_iam_role_policy.services_task_cloudwatch : name => p.policy }
        ) : [
        for s in jsondecode(policy).Statement : name if s.Sid == "XRaySpans"
      ]
    ])) == sort(["otlp"])
    error_message = "Only otlp should have XRaySpans, exactly once each."
  }

  assert {
    condition = alltrue(flatten([
      for defs in concat(
        [aws_ecs_task_definition.backend.container_definitions],
        [for svc in aws_ecs_task_definition.services : svc.container_definitions]
        ) : [
        for c in jsondecode(defs) : !strcontains(c.name, "adot-collector") && !strcontains(c.name, "xray-daemon")
      ]
    ]))
    error_message = "Without xray_enabled, neither the backend nor a named service should have an X-Ray sidecar."
  }
}

# REQ-1, REQ-2, REQ-3
run "backend_xray_spans_without_services" {
  command   = apply
  state_key = "backend_xray_spans_without_services"

  override_resource {
    target = null_resource.build_ci_lambda
  }

  variables {
    backend_xray_spans = true
    services           = []
  }

  assert {
    condition = (
      length(aws_cloudwatch_log_resource_policy.xray_transaction_search) == 1 &&
      length(aws_xray_trace_segment_destination.transaction_search) == 1 &&
      length(aws_xray_indexing_rule.default) == 1
    )
    error_message = "Expected 1 of each Transaction Search resource (logs policy, segment destination and indexing rule)."
  }

  assert {
    condition     = one(aws_xray_indexing_rule.default[*].rule[0].probabilistic[0].desired_sampling_percentage) == 1
    error_message = "The Default indexing rule must use 1 percent."
  }

  assert {
    condition = sort(flatten([
      for name, policy in merge(
        { backend = aws_iam_role_policy.backend_task_cloudwatch.policy },
        { for name, p in aws_iam_role_policy.services_task_cloudwatch : name => p.policy }
        ) : [
        for s in jsondecode(policy).Statement : name if s.Sid == "XRaySpans"
      ]
    ])) == sort(["backend"])
    error_message = "Only backend should have XRaySpans, exactly once each."
  }

  assert {
    condition = alltrue(flatten([
      for defs in concat(
        [aws_ecs_task_definition.backend.container_definitions],
        [for svc in aws_ecs_task_definition.services : svc.container_definitions]
        ) : [
        for c in jsondecode(defs) : !strcontains(c.name, "adot-collector") && !strcontains(c.name, "xray-daemon")
      ]
    ]))
    error_message = "Without xray_enabled, neither the backend nor a named service should have an X-Ray sidecar."
  }
}

# REQ-1, REQ-2
run "xray_spans_with_transaction_search_false" {
  command   = apply
  state_key = "xray_spans_with_transaction_search_false"

  override_resource {
    target = null_resource.build_ci_lambda
  }

  variables {
    backend_xray_spans      = true
    xray_transaction_search = false
    services                = [{ name = "otlp", xray_spans = true }]
  }

  assert {
    condition = (
      length(aws_cloudwatch_log_resource_policy.xray_transaction_search) == 0 &&
      length(aws_xray_trace_segment_destination.transaction_search) == 0 &&
      length(aws_xray_indexing_rule.default) == 0
    )
    error_message = "Expected 0 of each Transaction Search resource (logs policy, segment destination and indexing rule)."
  }

  assert {
    condition = sort(flatten([
      for name, policy in merge(
        { backend = aws_iam_role_policy.backend_task_cloudwatch.policy },
        { for name, p in aws_iam_role_policy.services_task_cloudwatch : name => p.policy }
        ) : [
        for s in jsondecode(policy).Statement : name if s.Sid == "XRaySpans"
      ]
    ])) == sort(["backend", "otlp"])
    error_message = "Only backend and otlp should have XRaySpans, exactly once each."
  }

  assert {
    condition = alltrue(flatten([
      for defs in concat(
        [aws_ecs_task_definition.backend.container_definitions],
        [for svc in aws_ecs_task_definition.services : svc.container_definitions]
        ) : [
        for c in jsondecode(defs) : !strcontains(c.name, "adot-collector") && !strcontains(c.name, "xray-daemon")
      ]
    ]))
    error_message = "Without xray_enabled, neither the backend nor a named service should have an X-Ray sidecar."
  }
}

# REQ-1, REQ-2, REQ-3
run "transaction_search_true_alone" {
  command   = apply
  state_key = "transaction_search_true_alone"

  override_resource {
    target = null_resource.build_ci_lambda
  }

  variables {
    xray_transaction_search = true
    services                = [{ name = "api" }]
  }

  assert {
    condition = (
      length(aws_cloudwatch_log_resource_policy.xray_transaction_search) == 1 &&
      length(aws_xray_trace_segment_destination.transaction_search) == 1 &&
      length(aws_xray_indexing_rule.default) == 1
    )
    error_message = "Expected 1 of each Transaction Search resource (logs policy, segment destination and indexing rule)."
  }

  assert {
    condition     = one(aws_xray_indexing_rule.default[*].rule[0].probabilistic[0].desired_sampling_percentage) == 1
    error_message = "The Default indexing rule must use 1 percent."
  }

  assert {
    condition = sort(flatten([
      for name, policy in merge(
        { backend = aws_iam_role_policy.backend_task_cloudwatch.policy },
        { for name, p in aws_iam_role_policy.services_task_cloudwatch : name => p.policy }
        ) : [
        for s in jsondecode(policy).Statement : name if s.Sid == "XRaySpans"
      ]
    ])) == sort([])
    error_message = "No task role should have XRaySpans."
  }

  assert {
    condition = alltrue(flatten([
      for defs in concat(
        [aws_ecs_task_definition.backend.container_definitions],
        [for svc in aws_ecs_task_definition.services : svc.container_definitions]
        ) : [
        for c in jsondecode(defs) : !strcontains(c.name, "adot-collector") && !strcontains(c.name, "xray-daemon")
      ]
    ]))
    error_message = "Without xray_enabled, neither the backend nor a named service should have an X-Ray sidecar."
  }
}

# REQ-1, REQ-2
run "transaction_search_false_alone" {
  command   = apply
  state_key = "transaction_search_false_alone"

  override_resource {
    target = null_resource.build_ci_lambda
  }

  variables {
    xray_transaction_search = false
    services                = []
  }

  assert {
    condition = (
      length(aws_cloudwatch_log_resource_policy.xray_transaction_search) == 0 &&
      length(aws_xray_trace_segment_destination.transaction_search) == 0 &&
      length(aws_xray_indexing_rule.default) == 0
    )
    error_message = "Expected 0 of each Transaction Search resource (logs policy, segment destination and indexing rule)."
  }

  assert {
    condition = sort(flatten([
      for name, policy in merge(
        { backend = aws_iam_role_policy.backend_task_cloudwatch.policy },
        { for name, p in aws_iam_role_policy.services_task_cloudwatch : name => p.policy }
        ) : [
        for s in jsondecode(policy).Statement : name if s.Sid == "XRaySpans"
      ]
    ])) == sort([])
    error_message = "No task role should have XRaySpans."
  }

  assert {
    condition = alltrue(flatten([
      for defs in concat(
        [aws_ecs_task_definition.backend.container_definitions],
        [for svc in aws_ecs_task_definition.services : svc.container_definitions]
        ) : [
        for c in jsondecode(defs) : !strcontains(c.name, "adot-collector") && !strcontains(c.name, "xray-daemon")
      ]
    ]))
    error_message = "Without xray_enabled, neither the backend nor a named service should have an X-Ray sidecar."
  }
}

# REQ-1, REQ-2, REQ-3, REQ-5
run "xray_enabled_wins_over_false" {
  command   = apply
  state_key = "xray_enabled_wins_over_false"

  override_resource {
    target = null_resource.build_ci_lambda
  }

  variables {
    xray_enabled            = true
    xray_transaction_search = false
    services                = []
  }

  assert {
    condition = (
      length(aws_cloudwatch_log_resource_policy.xray_transaction_search) == 1 &&
      length(aws_xray_trace_segment_destination.transaction_search) == 1 &&
      length(aws_xray_indexing_rule.default) == 1
    )
    error_message = "Expected 1 of each Transaction Search resource (logs policy, segment destination and indexing rule)."
  }

  assert {
    condition     = one(aws_xray_indexing_rule.default[*].rule[0].probabilistic[0].desired_sampling_percentage) == 1
    error_message = "The Default indexing rule must use 1 percent."
  }

  assert {
    condition = sort(flatten([
      for name, policy in merge(
        { backend = aws_iam_role_policy.backend_task_cloudwatch.policy },
        { for name, p in aws_iam_role_policy.services_task_cloudwatch : name => p.policy }
        ) : [
        for s in jsondecode(policy).Statement : name if s.Sid == "XRaySpans"
      ]
    ])) == sort(["backend"])
    error_message = "Only backend should have XRaySpans, exactly once each."
  }
}

# REQ-1, REQ-2, REQ-3, REQ-5
run "transaction_search_true_with_xray_enabled" {
  command   = apply
  state_key = "transaction_search_true_with_xray_enabled"

  override_resource {
    target = null_resource.build_ci_lambda
  }

  variables {
    xray_enabled            = true
    xray_transaction_search = true
    services                = []
  }

  assert {
    condition = (
      length(aws_cloudwatch_log_resource_policy.xray_transaction_search) == 1 &&
      length(aws_xray_trace_segment_destination.transaction_search) == 1 &&
      length(aws_xray_indexing_rule.default) == 1
    )
    error_message = "Expected 1 of each Transaction Search resource (logs policy, segment destination and indexing rule)."
  }

  assert {
    condition     = one(aws_xray_indexing_rule.default[*].rule[0].probabilistic[0].desired_sampling_percentage) == 1
    error_message = "The Default indexing rule must use 1 percent."
  }

  assert {
    condition = sort(flatten([
      for name, policy in merge(
        { backend = aws_iam_role_policy.backend_task_cloudwatch.policy },
        { for name, p in aws_iam_role_policy.services_task_cloudwatch : name => p.policy }
        ) : [
        for s in jsondecode(policy).Statement : name if s.Sid == "XRaySpans"
      ]
    ])) == sort(["backend"])
    error_message = "Only backend should have XRaySpans, exactly once each."
  }
}

# REQ-5
run "legacy_sidecar_policy_order" {
  command   = apply
  state_key = "legacy_sidecar_policy_order"

  override_resource {
    target = null_resource.build_ci_lambda
  }

  variables {
    xray_enabled = true
  }

  assert {
    condition = (
      length(aws_cloudwatch_log_resource_policy.xray_transaction_search) == 1 &&
      length(aws_xray_trace_segment_destination.transaction_search) == 1 &&
      length(aws_xray_indexing_rule.default) == 1
    )
    error_message = "Expected 1 of each Transaction Search resource (logs policy, segment destination and indexing rule)."
  }

  assert {
    condition     = one(aws_xray_indexing_rule.default[*].rule[0].probabilistic[0].desired_sampling_percentage) == 1
    error_message = "The Default indexing rule must use 1 percent."
  }

  assert {
    condition = sort(flatten([
      for name, policy in merge(
        { backend = aws_iam_role_policy.backend_task_cloudwatch.policy },
        { for name, p in aws_iam_role_policy.services_task_cloudwatch : name => p.policy }
        ) : [
        for s in jsondecode(policy).Statement : name if s.Sid == "XRaySpans"
      ]
    ])) == sort(["backend", "traced"])
    error_message = "Only backend and traced should have XRaySpans, exactly once each."
  }

  assert {
    condition = (
      [for s in jsondecode(aws_iam_role_policy.backend_task_cloudwatch.policy).Statement : s.Sid] ==
      ["PutMetrics", "OwnLogs", "ContainerInsightsLogs", "ApplicationMetricsLogs", "XRaySpans"] &&
      [for s in jsondecode(aws_iam_role_policy.services_task_cloudwatch["traced"].policy).Statement : s.Sid] ==
      ["PutMetrics", "OwnLogs", "XRayWrite", "XRaySpans"]
    )
    error_message = "Existing sidecar roles must retain their statement order and exactly one XRaySpans grant."
  }
}

# REQ-2, REQ-5
run "xray_enabled_and_spans_one_statement" {
  command   = apply
  state_key = "xray_enabled_and_spans_one_statement"

  override_resource {
    target = null_resource.build_ci_lambda
  }

  variables {
    xray_enabled       = true
    backend_xray_spans = true
    services           = [{ name = "both", xray_enabled = true, xray_spans = true }]
  }

  assert {
    condition = (
      length(aws_cloudwatch_log_resource_policy.xray_transaction_search) == 1 &&
      length(aws_xray_trace_segment_destination.transaction_search) == 1 &&
      length(aws_xray_indexing_rule.default) == 1
    )
    error_message = "Expected 1 of each Transaction Search resource (logs policy, segment destination and indexing rule)."
  }

  assert {
    condition     = one(aws_xray_indexing_rule.default[*].rule[0].probabilistic[0].desired_sampling_percentage) == 1
    error_message = "The Default indexing rule must use 1 percent."
  }

  assert {
    condition = sort(flatten([
      for name, policy in merge(
        { backend = aws_iam_role_policy.backend_task_cloudwatch.policy },
        { for name, p in aws_iam_role_policy.services_task_cloudwatch : name => p.policy }
        ) : [
        for s in jsondecode(policy).Statement : name if s.Sid == "XRaySpans"
      ]
    ])) == sort(["backend", "both"])
    error_message = "Only backend and both should have XRaySpans, exactly once each."
  }

  assert {
    condition = (
      [for s in jsondecode(aws_iam_role_policy.backend_task_cloudwatch.policy).Statement : s.Sid] ==
      ["PutMetrics", "OwnLogs", "ContainerInsightsLogs", "ApplicationMetricsLogs", "XRaySpans"] &&
      [for s in jsondecode(aws_iam_role_policy.services_task_cloudwatch["both"].policy).Statement : s.Sid] ==
      ["PutMetrics", "OwnLogs", "XRayWrite", "XRaySpans"]
    )
    error_message = "Existing sidecar roles must retain their statement order and exactly one XRaySpans grant."
  }
}
