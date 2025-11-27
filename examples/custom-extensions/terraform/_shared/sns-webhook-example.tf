# ============================================================================
# RAW TERRAFORM EXAMPLE
# This file is appended to the generated main.tf
# Use for simple resources that don't need bidirectional integration
# ============================================================================

# Example: Subscribe an existing SNS topic to your backend webhook
# This is useful when you have an SNS topic created outside of meroku
# (e.g., from another AWS service like SES, CloudWatch, etc.)
#
# resource "aws_sns_topic_subscription" "external_events" {
#   topic_arn              = "arn:aws:sns:us-east-1:123456789012:external-events"
#   protocol               = "https"
#   endpoint               = "${module.workloads.api_gateway_endpoint}/webhooks/external"
#   endpoint_auto_confirms = true
# }

# Example: Add a Route53 health check for your API
# Useful for monitoring API availability from outside AWS
#
# resource "aws_route53_health_check" "api" {
#   fqdn              = local.bridge.api_domain  # Uses bridge file
#   port              = 443
#   type              = "HTTPS"
#   resource_path     = "/health/live"
#   failure_threshold = 3
#   request_interval  = 30
#
#   tags = {
#     Name        = "${local.bridge.project}-${local.bridge.env}-api-health"
#     Project     = local.bridge.project
#     Environment = local.bridge.env
#     ManagedBy   = "custom-terraform"
#   }
# }

# Example: Create an S3 bucket notification to SNS
# Useful when you want to trigger events on S3 uploads
#
# resource "aws_s3_bucket_notification" "uploads" {
#   bucket = module.s3.buckets["media"].id
#
#   topic {
#     topic_arn     = aws_sns_topic.ext_media_events.arn
#     events        = ["s3:ObjectCreated:*"]
#     filter_prefix = "uploads/"
#   }
# }

# ============================================================================
# AVAILABLE REFERENCES
# ============================================================================
#
# From bridge file (local.bridge.*):
#   - local.bridge.project
#   - local.bridge.env
#   - local.bridge.region
#   - local.bridge.account_id
#   - local.bridge.vpc_id
#   - local.bridge.subnet_ids
#   - local.bridge.api_endpoint
#   - local.bridge.api_gateway_id
#   - local.bridge.ecs_cluster_arn
#   - local.bridge.ecs_cluster_name
#   - local.bridge.backend_task_role
#   - local.bridge.domain_zone_id (if domain enabled)
#   - local.bridge.api_domain (if domain enabled)
#   - local.bridge.db_endpoint (if postgres enabled)
#
# From modules:
#   - module.workloads.api_gateway_endpoint
#   - module.workloads.ecr_cluster.arn
#   - module.workloads.backend_task_role_name
#   - module.domain.zone_id (if enabled)
#   - module.postgres.endpoint (if enabled)
#   - module.s3.buckets["name"].id (if buckets defined)
#
# From YAML extensions:
#   - aws_sns_topic.ext_{name}.arn
#   - aws_sqs_queue.ext_{name}.url
#   - aws_sqs_queue.ext_{name}.arn
