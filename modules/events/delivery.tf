# The role an enhanced-bus subscriber delivers through, one per consumer that
# has at least one subscriber. Subscriber delivery is role-based, so no queue
# policy is added for it (queue_policy.tf covers rules only).
#
# Only EventBridge in this account may assume the role. Its IAM path,
# /meroku-events/<project>/<env>/, is what the deployer's iam:PassRole is
# scoped to, so a subscriber can be given no role outside this environment.
#
# The policy lists every queue the consumer has, not only the ones its
# subscribers target now. Terraform updates the policy before any subscriber,
# so a policy that followed the subscribers would drop a queue while the old
# subscriber still delivers to it, during the overlap of a replacement. With
# every queue listed, a queue's grant goes only when the queue itself goes, and
# by then no subscriber targets it.

resource "aws_iam_role" "delivery" {
  for_each = local.delivery_consumers

  name = module.naming_roles.names["delivery_role/${each.key}"]
  path = "/meroku-events/${var.project}/${var.env}/"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "events.amazonaws.com" }
      Action    = "sts:AssumeRole"
      Condition = { StringEquals = { "aws:SourceAccount" = data.aws_caller_identity.current.account_id } }
    }]
  })

  tags = merge(local.tags, { Name = module.naming_roles.names["delivery_role/${each.key}"] })

  lifecycle {
    precondition {
      condition     = length(module.naming_subscribers.collisions) == 0
      error_message = "Two event subscribers resolved to the same AWS name: ${join(", ", module.naming_subscribers.collisions)}. Rename a workload or give a subscription a distinct name."
    }
    precondition {
      condition     = length(module.naming_roles.collisions) == 0
      error_message = "Two event delivery roles resolved to the same AWS name: ${join(", ", module.naming_roles.collisions)}. Rename a workload."
    }
  }
}

resource "aws_iam_role_policy" "delivery" {
  for_each = local.delivery_consumers

  name = "deliver"
  role = aws_iam_role.delivery[each.key].id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid    = "Deliver"
      Effect = "Allow"
      Action = "sqs:SendMessage"
      Resource = concat(
        [aws_sqs_queue.main[each.key].arn, aws_sqs_queue.delivery_dlq[each.key].arn],
        contains(local.fifo_consumers, each.key) ? [aws_sqs_queue.fifo_main[each.key].arn, aws_sqs_queue.fifo_delivery_dlq[each.key].arn] : [],
      )
    }]
  })
}
