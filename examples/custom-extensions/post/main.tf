# ============================================================================
# CUSTOM POST-MODULE EXAMPLE
# This module runs AFTER core modules
# Use it to create resources that CONSUME core module outputs
# ============================================================================

# Example 1: CloudWatch Dashboard monitoring your infrastructure
resource "aws_cloudwatch_dashboard" "main" {
  dashboard_name = "${var.project}-${var.env}-overview"

  dashboard_body = jsonencode({
    widgets = [
      {
        type   = "metric"
        x      = 0
        y      = 0
        width  = 12
        height = 6
        properties = {
          title  = "API Gateway Requests"
          region = var.region
          metrics = [
            ["AWS/ApiGateway", "Count", "ApiId", var.api_gateway_id]
          ]
          period = 300
          stat   = "Sum"
        }
      },
      {
        type   = "metric"
        x      = 12
        y      = 0
        width  = 12
        height = 6
        properties = {
          title  = "ECS CPU Utilization"
          region = var.region
          metrics = [
            ["AWS/ECS", "CPUUtilization", "ClusterName", var.ecs_cluster_name]
          ]
          period = 300
          stat   = "Average"
        }
      },
      {
        type   = "metric"
        x      = 0
        y      = 6
        width  = 12
        height = 6
        properties = {
          title  = "API Gateway Latency"
          region = var.region
          metrics = [
            ["AWS/ApiGateway", "Latency", "ApiId", var.api_gateway_id]
          ]
          period = 300
          stat   = "Average"
        }
      },
      {
        type   = "metric"
        x      = 12
        y      = 6
        width  = 12
        height = 6
        properties = {
          title  = "ECS Memory Utilization"
          region = var.region
          metrics = [
            ["AWS/ECS", "MemoryUtilization", "ClusterName", var.ecs_cluster_name]
          ]
          period = 300
          stat   = "Average"
        }
      }
    ]
  })
}

# Example 2: CloudWatch alarm for high API latency
resource "aws_cloudwatch_metric_alarm" "api_high_latency" {
  alarm_name          = "${var.project}-${var.env}-api-high-latency"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 2
  metric_name         = "Latency"
  namespace           = "AWS/ApiGateway"
  period              = 300
  statistic           = "Average"
  threshold           = 1000 # 1 second
  alarm_description   = "API latency is above 1 second"

  dimensions = {
    ApiId = var.api_gateway_id
  }

  tags = {
    Name        = "${var.project}-${var.env}-api-high-latency"
    Project     = var.project
    Environment = var.env
    ManagedBy   = "custom-post-module"
  }
}

# Example 3: CloudWatch alarm for high error rate
resource "aws_cloudwatch_metric_alarm" "api_errors" {
  alarm_name          = "${var.project}-${var.env}-api-errors"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 2
  metric_name         = "5XXError"
  namespace           = "AWS/ApiGateway"
  period              = 300
  statistic           = "Sum"
  threshold           = 10
  alarm_description   = "API 5XX errors exceed threshold"

  dimensions = {
    ApiId = var.api_gateway_id
  }

  tags = {
    Name        = "${var.project}-${var.env}-api-errors"
    Project     = var.project
    Environment = var.env
    ManagedBy   = "custom-post-module"
  }
}

# Example 4: SNS topic subscription using the API endpoint
# (Alternative to YAML extensions for complex scenarios)
resource "aws_sns_topic" "custom_events" {
  name = "${var.project}-${var.env}-custom-events"

  tags = {
    Project     = var.project
    Environment = var.env
    ManagedBy   = "custom-post-module"
  }
}

resource "aws_sns_topic_subscription" "custom_webhook" {
  topic_arn              = aws_sns_topic.custom_events.arn
  protocol               = "https"
  endpoint               = "${var.api_endpoint}/webhooks/custom-events"
  endpoint_auto_confirms = true
}
