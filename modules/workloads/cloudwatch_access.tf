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
# X-Ray writes: the backend's task role carries AWSXrayWriteOnlyAccess
# (backend.tf) and a service's with xray_enabled gets the XRayWrite statement
# below. Neither allows xray:PutSpans, which the OTLP endpoint
# (https://xray.<region>.amazonaws.com/v1/traces) requires, so every role with
# xray_enabled or xray_spans gets XRaySpans. xray_spans is that grant alone,
# for an app that signs its OTLP requests itself and runs no sidecar. The
# endpoint accepts spans only once Transaction Search is on
# (xray_transaction_search.tf).
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

  # The calls a service makes when it sends OTLP traces to the X-Ray
  # endpoint. Granted on "*", as XRayWrite is. This is the one definition of
  # the OTLP span grant: any role that sends spans references it rather than
  # copying the actions, and should feed local.xray_spans_in_use
  # (xray_transaction_search.tf) so Transaction Search follows it.
  xray_spans_statement = {
    Sid    = "XRaySpans"
    Effect = "Allow"
    Action = [
      "xray:PutSpans",
      "xray:PutSpansForIndexing",
      "xray:PutTraceSegments",
    ]
    Resource = "*"
  }

  # Who gets xray_spans_statement: a workload with the sidecar, or with the
  # grant alone. One condition, so one statement even when both are set.
  backend_gets_xray_spans = var.xray_enabled || var.backend_xray_spans
  service_gets_xray_spans = { for s in var.services : s.name => s.xray_enabled || s.xray_spans }
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
      # Segment and telemetry writes come from AWSXrayWriteOnlyAccess
      # (backend.tf); OTLP spans need XRaySpans.
      var.xray_enabled ? [local.container_insights_statement, local.application_metrics_statement] : [],
      local.backend_gets_xray_spans ? [local.xray_spans_statement] : [],
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
      local.service_gets_xray_spans[each.key] ? [local.xray_spans_statement] : [],
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
