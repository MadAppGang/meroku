# SQS Queue
resource "aws_sqs_queue" "queue" {
  name = var.name
  tags = {
    Environment = var.env
  }
}

# IAM Policy for SQS access
data "aws_iam_policy_document" "sqs_policy" {
  statement {
    effect = "Allow"
    actions = [
      "sqs:SendMessage",
      "sqs:ReceiveMessage",
      "sqs:DeleteMessage",
      "sqs:GetQueueAttributes"
    ]
    resources = [
      aws_sqs_queue.queue.arn
    ]
  }
}

# Create IAM policy
resource "aws_iam_policy" "sqs_access_policy" {
  name        = "sqs-access-policy"
  path        = "/"
  description = "IAM policy for accessing SQS"
  policy      = data.aws_iam_policy_document.sqs_policy.json
}
