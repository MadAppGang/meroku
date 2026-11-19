output "bus_name" {
  description = "The bus a workload's EVENT_BUS_NAME names: the project bus when it is on, otherwise \"default\"."
  value       = var.bus_enabled ? local.bus_names.project : "default"
}

output "bus_arn" {
  description = "ARN of the project bus, classic or enhanced; \"\" when it is off."
  value       = local.bus_arns.project
}

output "queue_urls" {
  description = "Each consumer's queue URL, keyed by workload name."
  value       = { for c, q in aws_sqs_queue.main : c => q.url }
}

output "fifo_queue_urls" {
  description = "Each consumer's FIFO queue URL, keyed by workload name; {} when none."
  value       = { for c, q in aws_sqs_queue.fifo_main : c => q.url }
}

output "alerts_topic_arn" {
  description = "ARN of the topic the dead-letter alarms publish to; \"\" when no workload consumes events."
  value       = length(aws_sns_topic.alerts) > 0 ? aws_sns_topic.alerts[0].arn : ""
}

# Per workload, the variables its container gets: EVENT_QUEUE_URL and
# EVENT_DLQ_URL (its application DLQ) for a consumer, EVENT_FIFO_QUEUE_URL and
# EVENT_FIFO_DLQ_URL for a consumer with a FIFO queue set, and EVENT_BUS_ARN for
# every workload on an enhanced bus. On classic the last two terms are empty.
output "environment" {
  description = "Environment variables for each workload, keyed by workload name."
  value = { for w in keys(var.workloads) : w => concat(
    contains(keys(local.consumers), w) ? [
      { name = "EVENT_QUEUE_URL", value = aws_sqs_queue.main[w].url },
      { name = "EVENT_DLQ_URL", value = aws_sqs_queue.dlq[w].url },
    ] : [],
    contains(local.fifo_consumers, w) ? [
      { name = "EVENT_FIFO_QUEUE_URL", value = aws_sqs_queue.fifo_main[w].url },
      { name = "EVENT_FIFO_DLQ_URL", value = aws_sqs_queue.fifo_dlq[w].url },
    ] : [],
    local.enhanced ? [{ name = "EVENT_BUS_ARN", value = var.enhanced_bus_arn }] : [],
  ) }
}

output "rule_names" {
  description = "Each rule-delivered subscription's rule name, keyed \"<consumer>/<key>\"."
  value       = { for k, r in aws_cloudwatch_event_rule.subscription : k => r.name }
}

output "policies" {
  description = "The inline policy document written on each consuming or producing workload's task role, keyed by workload name."
  value       = { for w, p in aws_iam_role_policy.workload : w => p.policy }
}

# What the caller needs to create each enhanced-bus subscriber. A subscriber
# must not exist before its role may send, hence the depends_on.
output "subscribers" {
  description = "Each standard enhanced-bus subscription, keyed \"<consumer>/<key>\": what modules/event_subscriber needs. {} on classic."
  value = { for k, s in local.subscriber_subscriptions : k => {
    name            = module.naming_subscribers.names["subscriber/${k}"]
    type            = "UNORDERED"
    target_arn      = aws_sqs_queue.main[s.consumer].arn
    dead_letter_arn = aws_sqs_queue.delivery_dlq[s.consumer].arn
    role_arn        = aws_iam_role.delivery[s.consumer].arn
    pattern         = local.subscriber_patterns[k]
  } }
  depends_on = [aws_iam_role_policy.delivery]
}

output "fifo_subscribers" {
  description = "Each consumer's FIFO subscriber, keyed by consumer: one per consumer with ordered subscriptions. {} otherwise."
  value = { for c in local.ordered_consumers : c => {
    name            = module.naming_subscribers.names["fifo_subscriber/${c}"]
    type            = "FIFO"
    target_arn      = aws_sqs_queue.fifo_main[c].arn
    dead_letter_arn = aws_sqs_queue.fifo_delivery_dlq[c].arn
    role_arn        = aws_iam_role.delivery[c].arn
    pattern         = local.fifo_patterns[c]
  } }
  depends_on = [aws_iam_role_policy.delivery]
}
