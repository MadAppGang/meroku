resource "aws_sns_platform_application" "fcm_application" {
  count               = (var.setup_FCM_SNS) ? 1 : 0
  name                = "${var.project}-fcm-${var.env}"
  platform            = "GCM"
  platform_credential = nonsensitive(data.aws_ssm_parameter.gcm_server_key.value)
}

resource "aws_ssm_parameter" "gcm_server_key" {
  count = (var.setup_FCM_SNS) ? 1 : 0
  name  = "/${var.env}/${var.project}/backend/gcm-server-key"
  value = " "
  // if we manually change the value, don't rewrite it
  lifecycle {
    ignore_changes = [
      value,
    ]
  }
}


resource "aws_iam_role_policy_attachment" "backend_task_sns_fcm_policies" {
  count = (var.setup_FCM_SNS) ? 1 : 0
  role       = aws_iam_role.backend_task.name
  policy_arn = aws_iam_policy.backend_fcm_policies.arn
}

resource "aws_iam_policy" "backend_fcm_policies" {
  name   = "ManageEndpointsAndPublishFirebaseCloudMessages"
  count = (var.setup_FCM_SNS) ? 1 : 0
  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "sns:Publish",
        "sns:DeleteEndpoint",
        "sns:CreatePlatformEndpoint",
        "sns:GetEndpointAttributes",
				"sns:SetEndpointAttributes"
      ],
      "Resource": [
        "${aws_sns_platform_application.fcm_application.arn}"
      ]
    }
   ]
}
EOF
  tags = {
    terraform = "true"
    env       = var.env
  }
}