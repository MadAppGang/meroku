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

# IAM statements for the backend task role
# meroku merges them with workload.policy from the YAML into the backend's
# custom policy. The list is fixed and the actions are literal, so Terraform
# knows at plan that the policy exists; the resources may be created in the
# same apply.
output "backend_policies" {
  description = "IAM statements merged into the backend task role's custom policy"
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
