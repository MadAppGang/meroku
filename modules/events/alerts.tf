# An alarm on each dead-letter queue, sent to the environment's alerts topic.
#
# The topic exists only when at least one workload consumes events, so an
# environment without consumers gets no change. Nothing subscribes to it: a
# project adds its own subscription (email, chat, a pager), reading the ARN from
# local.bridge.alerts_topic_arn.
#
# The topic is not encrypted. CloudWatch alarm actions cannot publish to a topic
# encrypted with the AWS-managed SNS key, and the messages carry only the alarm's
# name and state.

resource "aws_sns_topic" "alerts" {
  count = length(local.consumers) > 0 ? 1 : 0

  # SNS names cap at 256, so a literal template. app/events.go computes the same
  # name to refuse an extensions.sns_topics entry that would claim it.
  name = "${var.project}-${var.env}-alerts"
  tags = merge(local.tags, { Name = "${var.project}-${var.env}-alerts" })
}

locals {
  dead_letter_queues = merge(
    { for c in keys(local.consumers) : "${c}/dlq" => {
      consumer = c
      kind     = "application"
      name     = aws_sqs_queue.dlq[c].name
    } },
    { for c in keys(local.consumers) : "${c}/delivery-dlq" => {
      consumer = c
      kind     = "delivery"
      name     = aws_sqs_queue.delivery_dlq[c].name
    } },
  )
}

resource "aws_cloudwatch_metric_alarm" "dead_letters" {
  for_each = local.dead_letter_queues

  alarm_name        = "${var.project}-${var.env}-events-${replace(each.key, "/", "-")}"
  alarm_description = "Messages in the ${each.value.kind} dead-letter queue of ${each.value.consumer}'s events. See ai_docs/EVENTS.md for redriving them."

  namespace           = "AWS/SQS"
  metric_name         = "ApproximateNumberOfMessagesVisible"
  dimensions          = { QueueName = each.value.name }
  statistic           = "Maximum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 0
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "notBreaching"

  alarm_actions = [aws_sns_topic.alerts[0].arn]
  ok_actions    = [aws_sns_topic.alerts[0].arn]

  tags = local.tags
}
