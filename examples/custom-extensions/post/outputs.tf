# ============================================================================
# OUTPUTS FOR CUSTOM POST-MODULE
# These outputs are available for reference in terraform state
# ============================================================================

output "dashboard_name" {
  description = "Name of the CloudWatch dashboard"
  value       = aws_cloudwatch_dashboard.main.dashboard_name
}

output "dashboard_url" {
  description = "URL to access the CloudWatch dashboard"
  value       = "https://${var.region}.console.aws.amazon.com/cloudwatch/home?region=${var.region}#dashboards:name=${aws_cloudwatch_dashboard.main.dashboard_name}"
}

output "latency_alarm_arn" {
  description = "ARN of the API latency alarm"
  value       = aws_cloudwatch_metric_alarm.api_high_latency.arn
}

output "error_alarm_arn" {
  description = "ARN of the API error alarm"
  value       = aws_cloudwatch_metric_alarm.api_errors.arn
}

output "custom_events_topic_arn" {
  description = "ARN of the custom events SNS topic"
  value       = aws_sns_topic.custom_events.arn
}
