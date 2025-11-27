# ============================================================================
# CUSTOM PRE-MODULE EXAMPLE
# This module runs BEFORE the workloads module
# Use it to create resources that need to feed values INTO backend env vars
# ============================================================================

# Example 1: Create a KMS key for application secrets
resource "aws_kms_key" "app_secrets" {
  description             = "${var.project}-${var.env} application secrets"
  deletion_window_in_days = 7
  enable_key_rotation     = true

  tags = {
    Name        = "${var.project}-${var.env}-app-secrets"
    Project     = var.project
    Environment = var.env
    ManagedBy   = "custom-pre-module"
  }
}

resource "aws_kms_alias" "app_secrets" {
  name          = "alias/${var.project}-${var.env}-secrets"
  target_key_id = aws_kms_key.app_secrets.key_id
}

# Example 2: Create an SNS topic that backend needs to publish to
resource "aws_sns_topic" "notifications" {
  name = "${var.project}-${var.env}-notifications"

  tags = {
    Name        = "${var.project}-${var.env}-notifications"
    Project     = var.project
    Environment = var.env
    ManagedBy   = "custom-pre-module"
  }
}

# Example 3: Create a Secrets Manager secret
resource "aws_secretsmanager_secret" "api_keys" {
  name = "${var.project}-${var.env}-api-keys"

  tags = {
    Name        = "${var.project}-${var.env}-api-keys"
    Project     = var.project
    Environment = var.env
    ManagedBy   = "custom-pre-module"
  }
}
