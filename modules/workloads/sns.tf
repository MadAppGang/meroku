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
  # A device endpoint is created under the platform application, so
  # CreatePlatformEndpoint is checked against the application's ARN
  # (…:app/GCM/<app>). Publishing to a device and reading, updating or deleting
  # it are checked against the endpoint's own ARN (…:endpoint/GCM/<app>/<id>).
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid      = "CreateEndpoints"
        Effect   = "Allow"
        Action   = ["sns:CreatePlatformEndpoint"]
        Resource = aws_sns_platform_application.fcm_application[0].arn
      },
      {
        Sid    = "UseEndpoints"
        Effect = "Allow"
        Action = [
          "sns:Publish",
          "sns:GetEndpointAttributes",
          "sns:SetEndpointAttributes",
          "sns:DeleteEndpoint",
        ]
        Resource = "${replace(aws_sns_platform_application.fcm_application[0].arn, ":app/", ":endpoint/")}/*"
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