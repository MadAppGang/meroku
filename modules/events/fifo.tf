# The FIFO queue set of a consumer with ordered subscriptions on an enhanced
# bus, beside its standard set (which it always keeps):
#   - the FIFO main queue, which the workload reads (EVENT_FIFO_QUEUE_URL). The
#     consumer's FIFO subscriber delivers every ordered subscription to it, in
#     order within each message group;
#   - the FIFO application DLQ, where SQS moves a message after
#     max_receive_count failed receives (EVENT_FIFO_DLQ_URL);
#   - the FIFO delivery DLQ, where the subscriber puts what it could not
#     deliver.
#
# A consumer with queue.fifo_retired keeps the set while no subscriber
# delivers to it, so that dropping the last ordered subscription and deleting
# the queues are two deploys.
#
# fifo_queue is a literal: a queue cannot change between standard and FIFO in
# place, and a FIFO queue is a separate address. The subscriber sets an explicit
# MessageDeduplicationId, which takes precedence; content-based deduplication
# is the fallback for sends without one, such as the delivery DLQ's records.

resource "aws_sqs_queue" "fifo_dlq" {
  for_each = local.fifo_consumers

  name                        = module.naming.names["fifo_dlq/${each.key}"]
  fifo_queue                  = true
  content_based_deduplication = true
  deduplication_scope         = "messageGroup"
  fifo_throughput_limit       = "perMessageGroupId"
  message_retention_seconds   = local.dlq_retention_seconds
  sqs_managed_sse_enabled     = true

  tags = merge(local.tags, { Name = module.naming.names["fifo_dlq/${each.key}"] })
}

resource "aws_sqs_queue" "fifo_delivery_dlq" {
  for_each = local.fifo_consumers

  name                        = module.naming.names["fifo_delivery_dlq/${each.key}"]
  fifo_queue                  = true
  content_based_deduplication = true
  deduplication_scope         = "messageGroup"
  fifo_throughput_limit       = "perMessageGroupId"
  message_retention_seconds   = local.dlq_retention_seconds
  sqs_managed_sse_enabled     = true

  tags = merge(local.tags, { Name = module.naming.names["fifo_delivery_dlq/${each.key}"] })
}

resource "aws_sqs_queue" "fifo_main" {
  for_each = local.fifo_consumers

  name                        = module.naming.names["fifo_queue/${each.key}"]
  fifo_queue                  = true
  content_based_deduplication = true
  deduplication_scope         = "messageGroup"
  fifo_throughput_limit       = "perMessageGroupId"
  visibility_timeout_seconds  = local.consumers[each.key].visibility_timeout
  message_retention_seconds   = local.queue_retention_seconds
  receive_wait_time_seconds   = local.receive_wait_seconds
  sqs_managed_sse_enabled     = true

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.fifo_dlq[each.key].arn
    maxReceiveCount     = local.consumers[each.key].max_receive_count
  })

  tags = merge(local.tags, { Name = module.naming.names["fifo_queue/${each.key}"] })
}
