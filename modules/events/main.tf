# Events: the project bus, a queue per consuming workload, the rules that feed
# it, and the dead-letter alarms. See ai_docs/EVENTS.md.
#
# Each consumer has three queues:
#   - the main queue, which the workload reads (EVENT_QUEUE_URL). Its running
#     copies share it, so each event is handled once;
#   - the application DLQ, where SQS moves a message after max_receive_count
#     failed receives;
#   - the delivery DLQ, where EventBridge puts an event it could not deliver to
#     the main queue.
#
# All three use SSE-SQS, the encryption SQS manages itself; no KMS key.

data "aws_region" "current" {}
data "aws_caller_identity" "current" {}

locals {
  tags = {
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }

  consumers = { for c in var.consumers : c.name => c }

  # One entry per subscription, keyed "<consumer>/<key>". The key is stable under
  # reordering, so moving an entry in the YAML changes no rule.
  subscriptions = merge([for c in var.consumers : {
    for s in c.subscriptions : "${c.name}/${s.key}" => merge(s, { consumer = c.name })
  }]...)

  default_bus_arn = "arn:aws:events:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:event-bus/default"

  bus_names = {
    default = "default"
    project = var.bus_enabled ? aws_cloudwatch_event_bus.project[0].name : ""
  }
  bus_arns = {
    default = local.default_bus_arn
    project = var.bus_enabled ? aws_cloudwatch_event_bus.project[0].arn : ""
  }

  # Fixed queue settings: 4 days of retention on the main queue, 14 on the DLQs,
  # 20-second long polling.
  queue_retention_seconds = 345600
  dlq_retention_seconds   = 1209600
  receive_wait_seconds    = 20
}

resource "aws_cloudwatch_event_bus" "project" {
  count = var.bus_enabled ? 1 : 0

  name = "${var.project}-${var.env}"
  tags = merge(local.tags, { Name = "${var.project}-${var.env}" })
}

resource "aws_sqs_queue" "dlq" {
  for_each = local.consumers

  name                      = module.naming.names["dlq/${each.key}"]
  message_retention_seconds = local.dlq_retention_seconds
  sqs_managed_sse_enabled   = true

  tags = merge(local.tags, { Name = module.naming.names["dlq/${each.key}"] })
}

resource "aws_sqs_queue" "delivery_dlq" {
  for_each = local.consumers

  name                      = module.naming.names["delivery_dlq/${each.key}"]
  message_retention_seconds = local.dlq_retention_seconds
  sqs_managed_sse_enabled   = true

  tags = merge(local.tags, { Name = module.naming.names["delivery_dlq/${each.key}"] })
}

resource "aws_sqs_queue" "main" {
  for_each = local.consumers

  name                       = module.naming.names["queue/${each.key}"]
  visibility_timeout_seconds = each.value.visibility_timeout
  message_retention_seconds  = local.queue_retention_seconds
  receive_wait_time_seconds  = local.receive_wait_seconds
  sqs_managed_sse_enabled    = true

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.dlq[each.key].arn
    maxReceiveCount     = each.value.max_receive_count
  })

  tags = merge(local.tags, { Name = module.naming.names["queue/${each.key}"] })
}

resource "aws_cloudwatch_event_rule" "subscription" {
  for_each = local.subscriptions

  name           = module.naming.names["rule/${each.key}"]
  event_bus_name = local.bus_names[each.value.bus]

  # The detail-type, exact or by prefix, an optional source, and never a
  # replayed event: a replay would deliver every archived event to the queue
  # again. jsondecode lets the two detail-type shapes share one expression.
  event_pattern = jsonencode(merge(
    {
      "detail-type" = jsondecode(each.value.detail_type != "" ? jsonencode([each.value.detail_type]) : jsonencode([{ prefix = each.value.detail_type_prefix }]))
      "replay-name" = [{ exists = false }]
    },
    each.value.source != "" ? { source = [each.value.source] } : {},
  ))

  tags = merge(local.tags, { Name = module.naming.names["rule/${each.key}"] })

  lifecycle {
    precondition {
      condition     = length(module.naming.collisions) == 0
      error_message = "Two event queues or rules resolved to the same AWS name: ${join(", ", module.naming.collisions)}. Rename a workload or give a subscription a distinct name."
    }
  }
}

resource "aws_cloudwatch_event_target" "subscription" {
  for_each = local.subscriptions

  rule           = aws_cloudwatch_event_rule.subscription[each.key].name
  event_bus_name = aws_cloudwatch_event_rule.subscription[each.key].event_bus_name
  target_id      = "queue"
  arn            = aws_sqs_queue.main[each.value.consumer].arn

  # 24 hours (the AWS default age) and 185 attempts: an outage shorter than a day
  # delays events instead of moving them to the delivery DLQ. Errors EventBridge
  # cannot retry go to the DLQ at once.
  retry_policy {
    maximum_event_age_in_seconds = 86400
    maximum_retry_attempts       = 185
  }

  dead_letter_config {
    arn = aws_sqs_queue.delivery_dlq[each.value.consumer].arn
  }
}
