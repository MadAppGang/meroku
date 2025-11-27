# ============================================================================
# OUTPUTS FOR CUSTOM PRE-MODULE
# These outputs are consumed by the workloads module
# ============================================================================

# Backend environment variables
# These are automatically merged with YAML-defined env vars
output "backend_env_vars" {
  description = "Environment variables to add to the backend container"
  value = [
    {
      name  = "KMS_KEY_ARN"
      value = aws_kms_key.app_secrets.arn
    },
    {
      name  = "KMS_KEY_ID"
      value = aws_kms_key.app_secrets.key_id
    },
    {
      name  = "SNS_NOTIFICATIONS_ARN"
      value = aws_sns_topic.notifications.arn
    },
    {
      name  = "SECRETS_MANAGER_ARN"
      value = aws_secretsmanager_secret.api_keys.arn
    }
  ]
}

# IAM policies for the backend task
# These grant the backend container permissions to use the resources
output "backend_policies" {
  description = "IAM policies to attach to the backend task role"
  value = [
    {
      actions = [
        "kms:Encrypt",
        "kms:Decrypt",
        "kms:GenerateDataKey"
      ]
      resources = [aws_kms_key.app_secrets.arn]
    },
    {
      actions = [
        "sns:Publish"
      ]
      resources = [aws_sns_topic.notifications.arn]
    },
    {
      actions = [
        "secretsmanager:GetSecretValue",
        "secretsmanager:DescribeSecret"
      ]
      resources = [aws_secretsmanager_secret.api_keys.arn]
    }
  ]
}

# Additional outputs for reference
output "kms_key_arn" {
  description = "ARN of the KMS key for application secrets"
  value       = aws_kms_key.app_secrets.arn
}

output "sns_topic_arn" {
  description = "ARN of the notifications SNS topic"
  value       = aws_sns_topic.notifications.arn
}
