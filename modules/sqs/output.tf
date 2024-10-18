output "queue_url" {
  value = aws_sqs_queue.queue.url
}

output "sqs_access_policy_arn" {
  value = aws_iam_policy.sqs_access_policy.arn
}