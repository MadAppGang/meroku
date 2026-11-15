# Optional bus, stable subscription identity and replay exclusion.
mock_provider "aws" {}
override_data {
  target = data.aws_region.current
  values = { region = "us-east-1", name = "us-east-1" }
}
override_data {
  target = data.aws_caller_identity.current
  values = { account_id = "000000000000" }
}

variables {
  project = "myapp"
  env     = "dev"
  workloads = {
    backend = { task_role_name = "backend-task", event_source = "myapp.backend" }
    orders  = { task_role_name = "orders-task", event_source = "myapp.orders" }
    worker  = { task_role_name = "worker-task", event_source = "myapp.worker" }
  }
}

run "disabled_bus_without_consumers_changes_nothing" {
  command = plan

  assert {
    condition = (
      length(aws_cloudwatch_event_bus.project) == 0 &&
      output.bus_name == "default" && output.bus_arn == "" &&
      length(aws_sns_topic.alerts) == 0 && output.alerts_topic_arn == "" &&
      length(aws_cloudwatch_metric_alarm.dead_letters) == 0 &&
      length(aws_sqs_queue.main) == 0 && length(aws_sqs_queue.dlq) == 0 &&
      length(aws_sqs_queue.delivery_dlq) == 0 && length(output.rule_names) == 0
    )
    error_message = "An environment without bus or consumers must create no bus, queues, alerts topic or alarms."
  }
}

run "enabled_bus_without_consumers_creates_only_the_bus" {
  command = plan

  variables {
    bus_enabled = true
  }

  assert {
    condition = (
      length(aws_cloudwatch_event_bus.project) == 1 &&
      aws_cloudwatch_event_bus.project[0].name == "myapp-dev" &&
      output.bus_name == "myapp-dev" &&
      length(aws_sns_topic.alerts) == 0 && output.alerts_topic_arn == "" &&
      length(aws_cloudwatch_metric_alarm.dead_letters) == 0
    )
    error_message = "Enabling the project bus alone must not create a consumer alerts topic."
  }
}

run "exact_and_prefix_patterns_exclude_replays" {
  command = plan

  variables {
    bus_enabled = true
    consumers = [
      {
        name          = "backend"
        subscriptions = [{ key = "orders", detail_type = "order.created", bus = "project" }]
      },
      {
        name               = "orders"
        visibility_timeout = 120
        max_receive_count  = 8
        subscriptions = [
          { key = "payments", detail_type = "payment.completed", source = "example.payments", bus = "project" },
          { key = "users-prefix-default", detail_type_prefix = "user.", bus = "default" }
        ]
      }
    ]
  }

  assert {
    condition = (
      output.rule_names == {
        "backend/orders"              = "myapp-events-backend-orders-dev"
        "orders/payments"             = "myapp-events-orders-payments-dev"
        "orders/users-prefix-default" = "myapp-events-orders-users-prefix-default-dev"
      } &&
      alltrue([for n in values(output.rule_names) : length(n) <= 64])
    )
    error_message = "Subscription identity must use consumer/key and bounded, stable rule names."
  }

  assert {
    condition = jsondecode(aws_cloudwatch_event_rule.subscription["backend/orders"].event_pattern) == {
      "detail-type" = ["order.created"]
      "replay-name" = [{ exists = false }]
    }
    error_message = "An exact subscription must keep its detail-type and exclude replays without inventing a source restriction."
  }

  assert {
    condition = jsondecode(aws_cloudwatch_event_rule.subscription["orders/payments"].event_pattern) == {
      "detail-type" = ["payment.completed"]
      source        = ["example.payments"]
      "replay-name" = [{ exists = false }]
    }
    error_message = "The optional source must be preserved alongside exact detail-type and replay exclusion."
  }

  assert {
    condition = jsondecode(aws_cloudwatch_event_rule.subscription["orders/users-prefix-default"].event_pattern) == {
      "detail-type" = [{ prefix = "user." }]
      "replay-name" = [{ exists = false }]
    }
    error_message = "A prefix subscription must use EventBridge's prefix matcher, not an exact string or wildcard."
  }

  assert {
    condition = (
      aws_cloudwatch_event_rule.subscription["backend/orders"].event_bus_name == "myapp-dev" &&
      aws_cloudwatch_event_rule.subscription["orders/payments"].event_bus_name == "myapp-dev" &&
      aws_cloudwatch_event_rule.subscription["orders/users-prefix-default"].event_bus_name == "default"
    )
    error_message = "Each rule must stay on the bus its subscription selected."
  }
}

run "reordering_subscriptions_keeps_rule_identity" {
  command = plan

  variables {
    bus_enabled = true
    consumers = [
      {
        name          = "backend"
        subscriptions = [{ key = "orders", detail_type = "order.created", bus = "project" }]
      },
      {
        name               = "orders"
        visibility_timeout = 120
        max_receive_count  = 8
        subscriptions = [
          { key = "users-prefix-default", detail_type_prefix = "user.", bus = "default" },
          { key = "payments", detail_type = "payment.completed", source = "example.payments", bus = "project" }
        ]
      }
    ]
  }

  assert {
    condition = (
      output.rule_names == {
        "backend/orders"              = "myapp-events-backend-orders-dev"
        "orders/payments"             = "myapp-events-orders-payments-dev"
        "orders/users-prefix-default" = "myapp-events-orders-users-prefix-default-dev"
      } &&
      toset(keys(aws_cloudwatch_event_target.subscription)) == toset([
        "backend/orders", "orders/payments", "orders/users-prefix-default"
      ])
    )
    error_message = "Reversing subscription order must not rename rules, rekey targets, or tie identity to list indices."
  }
}

run "long_rule_names_fit_the_eventbridge_limit" {
  command = plan

  variables {
    project     = "myapp-with-a-very-long-identity-to-exercise-event-rule-name-budget"
    env         = "development-environment"
    bus_enabled = true
    consumers = [{
      name = "backend"
      subscriptions = [{
        key         = "order-created-subscription"
        detail_type = "order.created"
        bus         = "project"
      }]
    }]
  }

  assert {
    condition = (
      length(output.rule_names) == 1 &&
      alltrue([for name in values(output.rule_names) : length(name) <= 64 && can(regex("^[A-Za-z0-9_.-]+$", name))]) &&
      alltrue([for queue in concat(values(aws_sqs_queue.main), values(aws_sqs_queue.dlq), values(aws_sqs_queue.delivery_dlq)) : length(queue.name) <= 80])
    )
    error_message = "Long project and environment names must still produce valid EventBridge rules within 64 characters and queue names within 80."
  }
}

# The exact text of the patterns subscriptions without a wildcard or filters
# have always had. A byte changed here is an in-place update of every such rule.
run "existing_subscriptions_keep_their_pattern_text" {
  command = plan

  variables {
    bus_enabled = true
    consumers = [{
      name = "orders"
      subscriptions = [
        { key = "exact", detail_type = "order.created", bus = "project" },
        { key = "exact-source", detail_type = "payment.completed", source = "example.payments", bus = "project" },
        { key = "prefix", detail_type_prefix = "user.", bus = "default" },
        { key = "prefix-source", detail_type_prefix = "user.", source = "example.identity", bus = "default" }
      ]
    }]
  }

  assert {
    condition = (
      aws_cloudwatch_event_rule.subscription["orders/exact"].event_pattern == "{\"detail-type\":[\"order.created\"],\"replay-name\":[{\"exists\":false}]}" &&
      aws_cloudwatch_event_rule.subscription["orders/exact-source"].event_pattern == "{\"detail-type\":[\"payment.completed\"],\"replay-name\":[{\"exists\":false}],\"source\":[\"example.payments\"]}" &&
      aws_cloudwatch_event_rule.subscription["orders/prefix"].event_pattern == "{\"detail-type\":[{\"prefix\":\"user.\"}],\"replay-name\":[{\"exists\":false}]}" &&
      aws_cloudwatch_event_rule.subscription["orders/prefix-source"].event_pattern == "{\"detail-type\":[{\"prefix\":\"user.\"}],\"replay-name\":[{\"exists\":false}],\"source\":[\"example.identity\"]}"
    )
    error_message = "A subscription without a wildcard or filters must render the same pattern text as before, so upgrading updates no rule."
  }
}

run "wildcard_detail_type_uses_the_wildcard_operator" {
  command = plan

  variables {
    bus_enabled = true
    consumers = [{
      name = "orders"
      subscriptions = [
        { key = "job", detail_type = "job.*", source = "example.jobs", bus = "project" },
        { key = "middle", detail_type = "job.*.done", bus = "project" }
      ]
    }]
  }

  assert {
    condition     = aws_cloudwatch_event_rule.subscription["orders/job"].event_pattern == "{\"detail-type\":[{\"wildcard\":\"job.*\"}],\"replay-name\":[{\"exists\":false}],\"source\":[\"example.jobs\"]}"
    error_message = "A detail_type with * must become an array holding one {\"wildcard\": ...} object under detail-type."
  }

  assert {
    condition = jsondecode(aws_cloudwatch_event_rule.subscription["orders/middle"].event_pattern) == {
      "detail-type" = [{ wildcard = "job.*.done" }]
      "replay-name" = [{ exists = false }]
    }
    error_message = "A * in the middle of the detail-type is a wildcard too."
  }
}

run "filters_nest_under_detail" {
  command = plan

  variables {
    bus_enabled = true
    consumers = [{
      name = "orders"
      subscriptions = [
        {
          key         = "single"
          detail_type = "job.completed"
          bus         = "project"
          filters     = [{ field = "detail.status", values = ["done", "failed"] }]
        },
        {
          key         = "nested"
          detail_type = "job.completed"
          bus         = "project"
          filters     = [{ field = "detail.order.type", values = ["retail"] }]
        },
        {
          key         = "two"
          detail_type = "job.*"
          source      = "example.jobs"
          bus         = "project"
          filters = [
            { field = "detail.status", values = ["done", "failed"] },
            { field = "detail.region", values = ["eu"] }
          ]
        },
        {
          key                = "shared-parent"
          detail_type_prefix = "job."
          bus                = "default"
          filters = [
            { field = "detail.order.type", values = ["retail"] },
            { field = "detail.status", values = ["done"] },
            { field = "detail.order.id", values = ["o-1", "o-2"] },
            { field = "detail.order-x.a", values = ["x"] },
            { field = "detail.order.meta.tier", values = ["gold"] }
          ]
        }
      ]
    }]
  }

  assert {
    condition     = aws_cloudwatch_event_rule.subscription["orders/single"].event_pattern == "{\"detail\":{\"status\":[\"done\",\"failed\"]},\"detail-type\":[\"job.completed\"],\"replay-name\":[{\"exists\":false}]}"
    error_message = "One filter must add detail.<field> with its values."
  }

  assert {
    condition = jsondecode(aws_cloudwatch_event_rule.subscription["orders/nested"].event_pattern) == {
      detail        = { order = { type = ["retail"] } }
      "detail-type" = ["job.completed"]
      "replay-name" = [{ exists = false }]
    }
    error_message = "A dotted field path must become nested objects under detail."
  }

  assert {
    condition     = aws_cloudwatch_event_rule.subscription["orders/two"].event_pattern == "{\"detail\":{\"region\":[\"eu\"],\"status\":[\"done\",\"failed\"]},\"detail-type\":[{\"wildcard\":\"job.*\"}],\"replay-name\":[{\"exists\":false}],\"source\":[\"example.jobs\"]}"
    error_message = "Two filters must share one detail object, beside the wildcard detail-type and the source."
  }

  assert {
    condition = jsondecode(aws_cloudwatch_event_rule.subscription["orders/shared-parent"].event_pattern) == {
      detail = {
        order = {
          id   = ["o-1", "o-2"]
          meta = { tier = ["gold"] }
          type = ["retail"]
        }
        "order-x" = { a = ["x"] }
        status    = ["done"]
      }
      "detail-type" = [{ prefix = "job." }]
      "replay-name" = [{ exists = false }]
    }
    error_message = "Filters with a common parent must merge into one nested object."
  }
}

run "filter_field_outside_detail_is_refused" {
  command = plan

  variables {
    consumers = [{
      name          = "orders"
      subscriptions = [{ key = "a", detail_type = "job.done", bus = "default", filters = [{ field = "status", values = ["done"] }] }]
    }]
  }

  expect_failures = [var.consumers]
}

run "filter_without_values_is_refused" {
  command = plan

  variables {
    consumers = [{
      name          = "orders"
      subscriptions = [{ key = "a", detail_type = "job.done", bus = "default", filters = [{ field = "detail.status", values = [] }] }]
    }]
  }

  expect_failures = [var.consumers]
}

run "filter_field_inside_another_is_refused" {
  command = plan

  variables {
    consumers = [{
      name = "orders"
      subscriptions = [{
        key         = "a"
        detail_type = "job.done"
        bus         = "default"
        filters     = [{ field = "detail.order", values = ["x"] }, { field = "detail.order.type", values = ["y"] }]
      }]
    }]
  }

  expect_failures = [var.consumers]
}

run "wildcard_in_prefix_is_refused" {
  command = plan

  variables {
    consumers = [{
      name          = "orders"
      subscriptions = [{ key = "a", detail_type_prefix = "job.*", bus = "default" }]
    }]
  }

  expect_failures = [var.consumers]
}
