# What a task role's containers may do in CloudWatch, one inline policy per
# role, in place of the AWS-managed CloudWatchFullAccess.
#
# CloudWatchFullAccess also carries events:*, sns:* and more. IAM allows add up,
# so with it attached no events.produces scoping (modules/events) could stop a
# workload from calling PutEvents on any bus. What a container needs here is
# narrower: publish custom metrics, and write to its own log group (a logging
# library that ships logs itself rather than through stdout).
#
# The ADOT collector the backend runs with xray_enabled has two EMF exporters
# (otel-task-metrics-config.yaml): one writes ECS Container Insights metrics to
# /aws/ecs/containerinsights/<cluster>/performance, the other writes the
# metrics the app sends it over OTLP or StatsD to /aws/ecs/application/metrics.
# Each creates its log group and streams. A service runs the X-Ray daemon
# instead of the collector, and the daemon sends traces with the task role's
# credentials.
#
# The execution roles keep their attachments: ECS assumes them to pull images,
# resolve secrets and write the awslogs streams, and a container never sees
# their credentials.
#
# Inline rather than managed: a role holds at most 10 managed policies, and the
# backend's task role already carries most of them.

locals {
  cloudwatch_metrics_statement = {
    Sid      = "PutMetrics"
    Effect   = "Allow"
    Action   = ["cloudwatch:PutMetricData"]
    Resource = "*"
  }

  cloudwatch_own_log_actions = [
    "logs:CreateLogStream",
    "logs:PutLogEvents",
    "logs:DescribeLogStreams",
  ]

  container_insights_log_group_arn = "arn:aws:logs:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:log-group:/aws/ecs/containerinsights/*"

  container_insights_statement = {
    Sid    = "ContainerInsightsLogs"
    Effect = "Allow"
    Action = [
      "logs:CreateLogGroup",
      "logs:CreateLogStream",
      "logs:PutLogEvents",
      "logs:DescribeLogStreams",
      "logs:DescribeLogGroups",
    ]
    Resource = [
      local.container_insights_log_group_arn,
      "${local.container_insights_log_group_arn}:*",
    ]
  }

  application_metrics_log_group_arn = "arn:aws:logs:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:log-group:/aws/ecs/application/metrics"

  application_metrics_statement = {
    Sid    = "ApplicationMetricsLogs"
    Effect = "Allow"
    Action = [
      "logs:CreateLogGroup",
      "logs:CreateLogStream",
      "logs:PutLogEvents",
      "logs:DescribeLogStreams",
    ]
    Resource = [
      local.application_metrics_log_group_arn,
      "${local.application_metrics_log_group_arn}:*",
    ]
  }

  xray_write_statement = {
    Sid    = "XRayWrite"
    Effect = "Allow"
    Action = [
      "xray:PutTraceSegments",
      "xray:PutTelemetryRecords",
      "xray:GetSamplingRules",
      "xray:GetSamplingTargets",
      "xray:GetSamplingStatisticSummaries",
    ]
    Resource = "*"
  }
}

resource "aws_iam_role_policy" "backend_task_cloudwatch" {
  name = "cloudwatch"
  role = aws_iam_role.backend_task.name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [
        local.cloudwatch_metrics_statement,
        {
          Sid      = "OwnLogs"
          Effect   = "Allow"
          Action   = local.cloudwatch_own_log_actions
          Resource = [aws_cloudwatch_log_group.backend.arn, "${aws_cloudwatch_log_group.backend.arn}:*"]
        },
      ],
      # X-Ray writes come from AWSXrayWriteOnlyAccess (backend.tf).
      var.xray_enabled ? [local.container_insights_statement, local.application_metrics_statement] : [],
    )
  })
}

resource "aws_iam_role_policy" "services_task_cloudwatch" {
  for_each = local.service_names

  name = "cloudwatch"
  role = aws_iam_role.services_task[each.key].name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = concat(
      [
        local.cloudwatch_metrics_statement,
        {
          Sid      = "OwnLogs"
          Effect   = "Allow"
          Action   = local.cloudwatch_own_log_actions
          Resource = [aws_cloudwatch_log_group.services[each.key].arn, "${aws_cloudwatch_log_group.services[each.key].arn}:*"]
        },
      ],
      each.value.xray_enabled ? [local.xray_write_statement] : [],
    )
  })
}

resource "aws_iam_role_policy" "pgadmin_task_cloudwatch" {
  count = var.pgadmin_enabled ? 1 : 0

  name = "cloudwatch"
  role = aws_iam_role.pgadmin_task[0].name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      local.cloudwatch_metrics_statement,
      {
        Sid      = "OwnLogs"
        Effect   = "Allow"
        Action   = local.cloudwatch_own_log_actions
        Resource = [aws_cloudwatch_log_group.pgadmin[0].arn, "${aws_cloudwatch_log_group.pgadmin[0].arn}:*"]
      },
    ]
  })
}
