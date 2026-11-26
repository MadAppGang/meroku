# X-Ray Transaction Search for every environment that sends spans to X-Ray.
#
# Services that send OpenTelemetry traces over OTLP post them to
# https://xray.<region>.amazonaws.com/v1/traces. That endpoint accepts spans
# only while Transaction Search is on in the account and region: the trace
# segment destination is CloudWatch Logs, and X-Ray may write to the aws/spans
# and /aws/application-signals/data log groups. Without it the endpoint rejects
# every span, whatever the task role allows.
#
# What turns it on (local.transaction_search_on):
#
#   - xray_enabled on the backend or a service (the sidecar). Always: it has
#     turned Transaction Search on since it was added, and
#     var.xray_transaction_search = false does not override it.
#   - xray_spans on the backend or a service (the grant without a sidecar),
#     unless var.xray_transaction_search is false.
#   - var.xray_transaction_search = true, with no workload asking: a task
#     granted spans through its own policy, for example.
#
# var.xray_transaction_search = false is for an environment whose only X-Ray
# use is xray_spans, when a sibling environment, the console or another stack
# already owns Transaction Search in the account and region. This environment
# then creates none of the three resources, its log resource policy included,
# so the owner must keep a policy of its own. app/xray_transaction_search.go
# holds the same rule as envUsesTransactionSearch.
#
# Both X-Ray settings are one per account and region, not per environment:
#
#   - aws_xray_trace_segment_destination and aws_xray_indexing_rule "Default"
#     already exist in every account. Creating them updates the existing
#     setting; deleting them changes nothing in AWS. Removing X-Ray from an
#     environment, setting var.xray_transaction_search = false, or destroying
#     the environment therefore leaves Transaction Search on.
#     Turning it off is a console or CLI change
#     (`aws xray update-trace-segment-destination --destination XRay`).
#   - Two environments in one account and region both hold these resources.
#     That is harmless while they agree; with different indexing percentages
#     each apply overwrites the other's, which `meroku generate` warns about
#     (xray_indexing_conflict).
#
# The log resource policy is named per environment, so destroying one
# environment never removes the policy a sibling's Transaction Search relies
# on. A log resource policy name may be 5120 characters, far above the cutoff
# at which names go through modules/naming.
#
# Once the destination is CloudWatch Logs, spans are stored in aws/spans and
# billed as CloudWatch Logs ingestion. Indexing up to 1% of spans as trace
# summaries is free; var.xray_indexing_percentage sets the share.
#
# aws_xray_trace_segment_destination and aws_xray_indexing_rule need
# hashicorp/aws 6.47.0 (versions.tf).

locals {
  # The sidecar: the ADOT collector on the backend, the X-Ray daemon on a service.
  xray_in_use = var.xray_enabled || anytrue([for s in var.services : s.xray_enabled])
  # The OTLP span grant without a sidecar.
  xray_spans_in_use = var.backend_xray_spans || anytrue([for s in var.services : s.xray_spans])

  transaction_search_on = local.xray_in_use || (
    var.xray_transaction_search != null ? var.xray_transaction_search : local.xray_spans_in_use
  )
}

resource "aws_cloudwatch_log_resource_policy" "xray_transaction_search" {
  count = local.transaction_search_on ? 1 : 0

  policy_name = "${var.project}-${var.env}-xray-transaction-search"
  policy_document = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "TransactionSearchXRayAccess"
        Effect    = "Allow"
        Principal = { Service = "xray.amazonaws.com" }
        Action    = "logs:PutLogEvents"
        Resource = [
          "arn:aws:logs:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:log-group:aws/spans:*",
          "arn:aws:logs:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:log-group:/aws/application-signals/data:*",
        ]
        Condition = {
          ArnLike = {
            "aws:SourceArn" = "arn:aws:xray:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:*"
          }
          StringEquals = {
            "aws:SourceAccount" = data.aws_caller_identity.current.account_id
          }
        }
      },
    ]
  })
}

resource "aws_xray_trace_segment_destination" "transaction_search" {
  count = local.transaction_search_on ? 1 : 0

  destination = "CloudWatchLogs"

  # X-Ray checks at this call that it may write to the log groups.
  depends_on = [aws_cloudwatch_log_resource_policy.xray_transaction_search]
}

resource "aws_xray_indexing_rule" "default" {
  count = local.transaction_search_on ? 1 : 0

  name = "Default"

  rule {
    probabilistic {
      desired_sampling_percentage = var.xray_indexing_percentage
    }
  }

  depends_on = [aws_xray_trace_segment_destination.transaction_search]
}
