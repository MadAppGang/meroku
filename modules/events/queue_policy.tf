# Who may send to a consumer's queues: EventBridge, and only for that consumer's
# own rules. The main queue receives the events; the delivery DLQ receives what
# EventBridge could not deliver, sent by the same rules. The application DLQ has
# no policy: SQS moves messages into it itself.

locals {
  consumer_rule_arns = { for c in keys(local.consumers) : c => [
    for k, s in local.subscriptions : aws_cloudwatch_event_rule.subscription[k].arn if s.consumer == c
  ] }
}

resource "aws_sqs_queue_policy" "main" {
  for_each = local.consumers

  queue_url = aws_sqs_queue.main[each.key].id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid       = "EventBridgeRules"
      Effect    = "Allow"
      Principal = { Service = "events.amazonaws.com" }
      Action    = "sqs:SendMessage"
      Resource  = aws_sqs_queue.main[each.key].arn
      Condition = { ArnEquals = { "aws:SourceArn" = local.consumer_rule_arns[each.key] } }
    }]
  })
}

resource "aws_sqs_queue_policy" "delivery_dlq" {
  for_each = local.consumers

  queue_url = aws_sqs_queue.delivery_dlq[each.key].id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid       = "EventBridgeDeadLetters"
      Effect    = "Allow"
      Principal = { Service = "events.amazonaws.com" }
      Action    = "sqs:SendMessage"
      Resource  = aws_sqs_queue.delivery_dlq[each.key].arn
      Condition = { ArnEquals = { "aws:SourceArn" = local.consumer_rule_arns[each.key] } }
    }]
  })
}
