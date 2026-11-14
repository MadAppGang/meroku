output "bus_name" {
  description = "The bus a workload's EVENT_BUS_NAME names: the project bus when it is on, otherwise \"default\"."
  value       = var.bus_enabled ? aws_cloudwatch_event_bus.project[0].name : "default"
}

output "bus_arn" {
  description = "ARN of the project bus; \"\" when it is off."
  value       = var.bus_enabled ? aws_cloudwatch_event_bus.project[0].arn : ""
}

output "queue_urls" {
  description = "Each consumer's queue URL, keyed by workload name."
  value       = { for c, q in aws_sqs_queue.main : c => q.url }
}

output "alerts_topic_arn" {
  description = "ARN of the topic the dead-letter alarms publish to; \"\" when no workload consumes events."
  value       = length(aws_sns_topic.alerts) > 0 ? aws_sns_topic.alerts[0].arn : ""
}

# Per workload, the variables its container gets: EVENT_QUEUE_URL for a
# consumer, nothing for any other workload.
output "environment" {
  description = "Environment variables for each workload, keyed by workload name."
  value = { for w in keys(var.workloads) : w => contains(keys(local.consumers), w) ? [
    { name = "EVENT_QUEUE_URL", value = aws_sqs_queue.main[w].url },
  ] : [] }
}

output "rule_names" {
  description = "Each subscription's rule name, keyed \"<consumer>/<key>\"."
  value       = { for k, r in aws_cloudwatch_event_rule.subscription : k => r.name }
}

output "policies" {
  description = "The inline policy document written on each consuming or producing workload's task role, keyed by workload name."
  value       = { for w, p in aws_iam_role_policy.workload : w => p.policy }
}
