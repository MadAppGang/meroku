output "subscriber_arn" {
  description = "ARN of the subscriber; \"\" on a tombstone."
  value       = try(awscc_eventsv2_subscriber.this[0].subscriber_arn, "")
}
