resource "aws_sns_platform_application" "fcm_application" {
  count               = (var.setup_FCM_SNS) ? 1 : 0
  name                = "${var.project}-fcm-${var.env}"
  platform            = "GCM"
  platform_credential = nonsensitive(aws_ssm_parameter.gcm_server_key[0].value)
}

resource "aws_ssm_parameter" "gcm_server_key" {
  count = (var.setup_FCM_SNS) ? 1 : 0
  name  = "/${var.env}/${var.project}/backend/gcm-server-key"
  type  = "SecureString"
  value = " "
  // if we manually change the value, don't rewrite it
  lifecycle {
    ignore_changes = [
      value,
    ]
  }

  tags = {
    Name        = "/${var.env}/${var.project}/backend/gcm-server-key"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}


resource "aws_iam_role_policy_attachment" "backend_task_sns_fcm_policies" {
  count      = (var.setup_FCM_SNS) ? 1 : 0
  role       = aws_iam_role.backend_task.name
  policy_arn = aws_iam_policy.backend_fcm_policies[0].arn
}

resource "aws_iam_policy" "backend_fcm_policies" {
  # Account-global; was hardcoded while the tags already carried project/env.
  name  = "ManageEndpointsAndPublishFirebaseCloudMessages_${var.project}_${var.env}"
  count = (var.setup_FCM_SNS) ? 1 : 0
  # SNS checks every one of these actions against the platform application's
  # ARN (…:app/GCM/<app>): creating a device endpoint, and publishing to,
  # reading, updating or deleting one under it. A grant on the endpoint ARNs
  # (…:endpoint/GCM/<app>/*) authorises none of them.
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "ManageEndpointsAndPublish"
        Effect = "Allow"
        Action = [
          "sns:CreatePlatformEndpoint",
          "sns:Publish",
          "sns:GetEndpointAttributes",
          "sns:SetEndpointAttributes",
          "sns:DeleteEndpoint",
        ]
        Resource = aws_sns_platform_application.fcm_application[0].arn
      },
    ]
  })
  tags = {
    Name        = "ManageEndpointsAndPublishFirebaseCloudMessages"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

# Topics whose ARN the backend receives in its environment
# (extensions.sns_topics[].add_to_backend_env). The backend may publish to
# exactly these; any other SNS access is declared in the workload's `policy`.
resource "aws_iam_role_policy" "backend_task_sns_publish" {
  count = length(var.backend_sns_publish_topic_arns) > 0 ? 1 : 0

  name = "sns-publish"
  role = aws_iam_role.backend_task.name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid      = "PublishExtensionTopics"
      Effect   = "Allow"
      Action   = ["sns:Publish"]
      Resource = var.backend_sns_publish_topic_arns
    }]
  })
}