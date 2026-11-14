# The full consumer delivery contract.
mock_provider "aws" {}
override_data {
  target = data.aws_region.current
  values = { region = "us-east-1", name = "us-east-1" }
}
override_data {
  target = data.aws_caller_identity.current
  values = { account_id = "000000000000" }
}

override_resource {
  target = aws_cloudwatch_event_bus.project[0]
  values = { arn = "arn:aws:events:us-east-1:000000000000:event-bus/myapp-dev" }
}
override_resource {
  target = aws_sns_topic.alerts[0]
  values = { arn = "arn:aws:sns:us-east-1:000000000000:myapp-dev-alerts" }
}
override_resource {
  target = aws_sqs_queue.main["backend"]
  values = {
    arn = "arn:aws:sqs:us-east-1:000000000000:myapp-events-backend-dev"
    url = "https://sqs.us-east-1.amazonaws.com/000000000000/myapp-events-backend-dev"
    id  = "https://sqs.us-east-1.amazonaws.com/000000000000/myapp-events-backend-dev"
  }
}
override_resource {
  target = aws_sqs_queue.dlq["backend"]
  values = {
    arn = "arn:aws:sqs:us-east-1:000000000000:myapp-events-backend-dlq-dev"
    url = "https://sqs.us-east-1.amazonaws.com/000000000000/myapp-events-backend-dlq-dev"
    id  = "https://sqs.us-east-1.amazonaws.com/000000000000/myapp-events-backend-dlq-dev"
  }
}
override_resource {
  target = aws_sqs_queue.delivery_dlq["backend"]
  values = {
    arn = "arn:aws:sqs:us-east-1:000000000000:myapp-events-backend-delivery-dlq-dev"
    url = "https://sqs.us-east-1.amazonaws.com/000000000000/myapp-events-backend-delivery-dlq-dev"
    id  = "https://sqs.us-east-1.amazonaws.com/000000000000/myapp-events-backend-delivery-dlq-dev"
  }
}
override_resource {
  target = aws_sqs_queue.main["orders"]
  values = {
    arn = "arn:aws:sqs:us-east-1:000000000000:myapp-events-orders-dev"
    url = "https://sqs.us-east-1.amazonaws.com/000000000000/myapp-events-orders-dev"
    id  = "https://sqs.us-east-1.amazonaws.com/000000000000/myapp-events-orders-dev"
  }
}
override_resource {
  target = aws_sqs_queue.dlq["orders"]
  values = {
    arn = "arn:aws:sqs:us-east-1:000000000000:myapp-events-orders-dlq-dev"
    url = "https://sqs.us-east-1.amazonaws.com/000000000000/myapp-events-orders-dlq-dev"
    id  = "https://sqs.us-east-1.amazonaws.com/000000000000/myapp-events-orders-dlq-dev"
  }
}
override_resource {
  target = aws_sqs_queue.delivery_dlq["orders"]
  values = {
    arn = "arn:aws:sqs:us-east-1:000000000000:myapp-events-orders-delivery-dlq-dev"
    url = "https://sqs.us-east-1.amazonaws.com/000000000000/myapp-events-orders-delivery-dlq-dev"
    id  = "https://sqs.us-east-1.amazonaws.com/000000000000/myapp-events-orders-delivery-dlq-dev"
  }
}
override_resource {
  target = aws_cloudwatch_event_rule.subscription["backend/orders"]
  values = { arn = "arn:aws:events:us-east-1:000000000000:rule/myapp-dev/myapp-events-backend-orders-dev" }
}
override_resource {
  target = aws_cloudwatch_event_rule.subscription["orders/payments"]
  values = { arn = "arn:aws:events:us-east-1:000000000000:rule/myapp-dev/myapp-events-orders-payments-dev" }
}
override_resource {
  target = aws_cloudwatch_event_rule.subscription["orders/users-prefix-default"]
  values = { arn = "arn:aws:events:us-east-1:000000000000:rule/myapp-events-orders-users-prefix-default-dev" }
}

variables {
  project     = "myapp"
  env         = "dev"
  bus_enabled = true
  workloads = {
    backend = { task_role_name = "backend-task", event_source = "myapp.backend" }
    orders  = { task_role_name = "orders-task", event_source = "myapp.orders" }
    worker  = { task_role_name = "worker-task", event_source = "myapp.worker" }
  }
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

run "consumers_get_encrypted_queues_retries_policies_and_alarms" {
  # ARNs, URLs and redrive/policy links are computed attributes under Terraform 1.10.
  command = apply

  assert {
    condition = (
      toset(keys(aws_sqs_queue.main)) == toset(["backend", "orders"]) &&
      toset(keys(aws_sqs_queue.dlq)) == toset(["backend", "orders"]) &&
      toset(keys(aws_sqs_queue.delivery_dlq)) == toset(["backend", "orders"]) &&
      alltrue([for q in concat(values(aws_sqs_queue.main), values(aws_sqs_queue.dlq), values(aws_sqs_queue.delivery_dlq)) : q.sqs_managed_sse_enabled && (q.kms_master_key_id == null || q.kms_master_key_id == "")])
    )
    error_message = "Each consumer, backend included, needs exactly three SSE-SQS encrypted queues."
  }

  assert {
    condition = (
      alltrue([for q in aws_sqs_queue.main : q.message_retention_seconds == 345600 && q.receive_wait_time_seconds == 20]) &&
      alltrue([for q in concat(values(aws_sqs_queue.dlq), values(aws_sqs_queue.delivery_dlq)) : q.message_retention_seconds == 1209600]) &&
      aws_sqs_queue.main["backend"].visibility_timeout_seconds == 30 &&
      aws_sqs_queue.main["orders"].visibility_timeout_seconds == 120 &&
      jsondecode(aws_sqs_queue.main["backend"].redrive_policy).maxReceiveCount == 5 &&
      jsondecode(aws_sqs_queue.main["orders"].redrive_policy).maxReceiveCount == 8
    )
    error_message = "Queues must retain main messages four days and DLQ messages fourteen days, poll twenty seconds, and preserve default/explicit visibility and redrive limits."
  }

  assert {
    condition = alltrue([
      for c in ["backend", "orders"] :
      jsondecode(aws_sqs_queue.main[c].redrive_policy).deadLetterTargetArn == aws_sqs_queue.dlq[c].arn
    ])
    error_message = "Application redrive must use the app DLQ, not the EventBridge delivery DLQ."
  }

  assert {
    condition = toset(keys(aws_cloudwatch_event_target.subscription)) == toset(["backend/orders", "orders/payments", "orders/users-prefix-default"]) && alltrue([
      for key, target in aws_cloudwatch_event_target.subscription :
      target.arn == aws_sqs_queue.main[split("/", key)[0]].arn &&
      target.rule == aws_cloudwatch_event_rule.subscription[key].name &&
      target.event_bus_name == aws_cloudwatch_event_rule.subscription[key].event_bus_name &&
      one(target.retry_policy).maximum_retry_attempts == 185 &&
      one(target.retry_policy).maximum_event_age_in_seconds == 86400 &&
      one(target.dead_letter_config).arn == aws_sqs_queue.delivery_dlq[split("/", key)[0]].arn
    ])
    error_message = "Every target must deliver to its consumer's main queue on the correct bus, retry 185/86400, and send delivery failures to its separate delivery DLQ."
  }

  assert {
    condition = (
      toset(keys(aws_sqs_queue_policy.main)) == toset(["backend", "orders"]) &&
      toset(keys(aws_sqs_queue_policy.delivery_dlq)) == toset(["backend", "orders"]) &&
      alltrue([for c in ["backend", "orders"] :
        aws_sqs_queue_policy.main[c].queue_url == aws_sqs_queue.main[c].url &&
        aws_sqs_queue_policy.delivery_dlq[c].queue_url == aws_sqs_queue.delivery_dlq[c].url
      ])
    )
    error_message = "Both the main queue and the delivery DLQ require their own queue policy."
  }

  assert {
    condition = (
      alltrue([
        for st in jsondecode(aws_sqs_queue_policy.main["backend"].policy).Statement :
        st.Effect == "Allow" &&
        toset(try(tolist(st.Action), [tostring(st.Action)])) == toset(["sqs:SendMessage"]) &&
        toset(try(tolist(st.Resource), [tostring(st.Resource)])) == toset(["arn:aws:sqs:us-east-1:000000000000:myapp-events-backend-dev"]) &&
        toset(try(tolist(st.Principal.Service), [tostring(st.Principal.Service)])) == toset(["events.amazonaws.com"])
      ]) &&
      toset(flatten([
        for st in jsondecode(aws_sqs_queue_policy.main["backend"].policy).Statement :
        try(tolist(st.Condition.ArnEquals["aws:SourceArn"]),
          [tostring(st.Condition.ArnEquals["aws:SourceArn"])],
          tolist(st.Condition.ArnEquals["AWS:SourceArn"]),
          [tostring(st.Condition.ArnEquals["AWS:SourceArn"])],
          tolist(st.Condition.StringEquals["aws:SourceArn"]),
        [tostring(st.Condition.StringEquals["aws:SourceArn"])], [])
      ])) == toset(["arn:aws:events:us-east-1:000000000000:rule/myapp-dev/myapp-events-backend-orders-dev"])
    )
    error_message = "backend's main queue policy must allow only EventBridge SendMessage to this queue from exactly this consumer's rule ARNs."
  }

  assert {
    condition = (
      alltrue([
        for st in jsondecode(aws_sqs_queue_policy.delivery_dlq["backend"].policy).Statement :
        st.Effect == "Allow" &&
        toset(try(tolist(st.Action), [tostring(st.Action)])) == toset(["sqs:SendMessage"]) &&
        toset(try(tolist(st.Resource), [tostring(st.Resource)])) == toset(["arn:aws:sqs:us-east-1:000000000000:myapp-events-backend-delivery-dlq-dev"]) &&
        toset(try(tolist(st.Principal.Service), [tostring(st.Principal.Service)])) == toset(["events.amazonaws.com"])
      ]) &&
      toset(flatten([
        for st in jsondecode(aws_sqs_queue_policy.delivery_dlq["backend"].policy).Statement :
        try(tolist(st.Condition.ArnEquals["aws:SourceArn"]),
          [tostring(st.Condition.ArnEquals["aws:SourceArn"])],
          tolist(st.Condition.ArnEquals["AWS:SourceArn"]),
          [tostring(st.Condition.ArnEquals["AWS:SourceArn"])],
          tolist(st.Condition.StringEquals["aws:SourceArn"]),
        [tostring(st.Condition.StringEquals["aws:SourceArn"])], [])
      ])) == toset(["arn:aws:events:us-east-1:000000000000:rule/myapp-dev/myapp-events-backend-orders-dev"])
    )
    error_message = "backend's delivery_dlq queue policy must allow only EventBridge SendMessage to this queue from exactly this consumer's rule ARNs."
  }

  assert {
    condition = (
      alltrue([
        for st in jsondecode(aws_sqs_queue_policy.main["orders"].policy).Statement :
        st.Effect == "Allow" &&
        toset(try(tolist(st.Action), [tostring(st.Action)])) == toset(["sqs:SendMessage"]) &&
        toset(try(tolist(st.Resource), [tostring(st.Resource)])) == toset(["arn:aws:sqs:us-east-1:000000000000:myapp-events-orders-dev"]) &&
        toset(try(tolist(st.Principal.Service), [tostring(st.Principal.Service)])) == toset(["events.amazonaws.com"])
      ]) &&
      toset(flatten([
        for st in jsondecode(aws_sqs_queue_policy.main["orders"].policy).Statement :
        try(tolist(st.Condition.ArnEquals["aws:SourceArn"]),
          [tostring(st.Condition.ArnEquals["aws:SourceArn"])],
          tolist(st.Condition.ArnEquals["AWS:SourceArn"]),
          [tostring(st.Condition.ArnEquals["AWS:SourceArn"])],
          tolist(st.Condition.StringEquals["aws:SourceArn"]),
        [tostring(st.Condition.StringEquals["aws:SourceArn"])], [])
      ])) == toset(["arn:aws:events:us-east-1:000000000000:rule/myapp-dev/myapp-events-orders-payments-dev", "arn:aws:events:us-east-1:000000000000:rule/myapp-events-orders-users-prefix-default-dev"])
    )
    error_message = "orders's main queue policy must allow only EventBridge SendMessage to this queue from exactly this consumer's rule ARNs."
  }

  assert {
    condition = (
      alltrue([
        for st in jsondecode(aws_sqs_queue_policy.delivery_dlq["orders"].policy).Statement :
        st.Effect == "Allow" &&
        toset(try(tolist(st.Action), [tostring(st.Action)])) == toset(["sqs:SendMessage"]) &&
        toset(try(tolist(st.Resource), [tostring(st.Resource)])) == toset(["arn:aws:sqs:us-east-1:000000000000:myapp-events-orders-delivery-dlq-dev"]) &&
        toset(try(tolist(st.Principal.Service), [tostring(st.Principal.Service)])) == toset(["events.amazonaws.com"])
      ]) &&
      toset(flatten([
        for st in jsondecode(aws_sqs_queue_policy.delivery_dlq["orders"].policy).Statement :
        try(tolist(st.Condition.ArnEquals["aws:SourceArn"]),
          [tostring(st.Condition.ArnEquals["aws:SourceArn"])],
          tolist(st.Condition.ArnEquals["AWS:SourceArn"]),
          [tostring(st.Condition.ArnEquals["AWS:SourceArn"])],
          tolist(st.Condition.StringEquals["aws:SourceArn"]),
        [tostring(st.Condition.StringEquals["aws:SourceArn"])], [])
      ])) == toset(["arn:aws:events:us-east-1:000000000000:rule/myapp-dev/myapp-events-orders-payments-dev", "arn:aws:events:us-east-1:000000000000:rule/myapp-events-orders-users-prefix-default-dev"])
    )
    error_message = "orders's delivery_dlq queue policy must allow only EventBridge SendMessage to this queue from exactly this consumer's rule ARNs."
  }

  assert {
    condition = (
      toset(keys(output.policies)) == toset(["backend", "orders"]) &&
      alltrue([
        for c, policy in output.policies :
        toset(flatten([for st in jsondecode(policy).Statement : try(tolist(st.Action), [tostring(st.Action)])])) ==
        toset(["sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:ChangeMessageVisibility", "sqs:GetQueueAttributes", "sqs:GetQueueUrl"]) &&
        toset(flatten([for st in jsondecode(policy).Statement : try(tolist(st.Resource), [tostring(st.Resource)])])) == toset([aws_sqs_queue.main[c].arn]) &&
        aws_iam_role_policy.workload[c].role == (c == "backend" ? "backend-task" : "orders-task")
      ])
    )
    error_message = "Consumer IAM must grant exactly queue consumption on its main queue to its task role, never queue or bus creation."
  }

  assert {
    condition = (
      length(aws_sns_topic.alerts) == 1 &&
      aws_sns_topic.alerts[0].name == "myapp-dev-alerts" &&
      (aws_sns_topic.alerts[0].kms_master_key_id == null || aws_sns_topic.alerts[0].kms_master_key_id == "") &&
      length(aws_cloudwatch_metric_alarm.dead_letters) == 4 &&
      toset([for a in aws_cloudwatch_metric_alarm.dead_letters : a.dimensions["QueueName"]]) ==
      toset([for q in concat(values(aws_sqs_queue.dlq), values(aws_sqs_queue.delivery_dlq)) : q.name]) &&
      alltrue([
        for a in aws_cloudwatch_metric_alarm.dead_letters :
        a.namespace == "AWS/SQS" &&
        a.metric_name == "ApproximateNumberOfMessagesVisible" &&
        a.statistic == "Maximum" && a.comparison_operator == "GreaterThanThreshold" &&
        a.threshold == 0 && a.period == 300 && a.treat_missing_data == "notBreaching" &&
        toset(a.alarm_actions) == toset([aws_sns_topic.alerts[0].arn]) &&
        toset(a.ok_actions) == toset([aws_sns_topic.alerts[0].arn])
      ])
    )
    error_message = "Each of the four DLQs must have one visible-message alarm and alarm/OK actions to the shared alerts topic."
  }

  assert {
    condition = (
      output.alerts_topic_arn == "arn:aws:sns:us-east-1:000000000000:myapp-dev-alerts" &&
      output.queue_urls == { backend = aws_sqs_queue.main["backend"].url, orders = aws_sqs_queue.main["orders"].url } &&
      jsonencode(output.environment["backend"]) == jsonencode([{ name = "EVENT_QUEUE_URL", value = aws_sqs_queue.main["backend"].url }]) &&
      jsonencode(output.environment["orders"]) == jsonencode([{ name = "EVENT_QUEUE_URL", value = aws_sqs_queue.main["orders"].url }]) &&
      length(output.environment["worker"]) == 0
    )
    error_message = "Only consumers receive EVENT_QUEUE_URL; queue and alerts outputs must reference the resources actually created."
  }
}
