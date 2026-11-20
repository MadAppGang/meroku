# The schedule's dead-letter queue. EventBridge Scheduler sends it every
# launch it gives up on, after max_retry_attempts. Either meroku creates it
# (create_dlq) or the YAML names an existing queue (dlq_arn); the variable
# validation refuses both.
#
# dlq_enabled is decided from the two inputs, never from the queue's ARN, which
# is unknown until the queue exists: the count of the IAM grant and of the
# dead_letter_config block must be known at plan.
locals {
  dlq_enabled = var.create_dlq || var.dlq_arn != ""
  dlq_arn     = var.create_dlq ? one(aws_sqs_queue.dlq[*].arn) : var.dlq_arn
}

resource "aws_sqs_queue" "dlq" {
  count = var.create_dlq ? 1 : 0

  name                      = module.naming.names["dlq"]
  message_retention_seconds = 1209600 # 14 days, as the events DLQs
  sqs_managed_sse_enabled   = true

  tags = {
    Name        = module.naming.names["dlq"]
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}
