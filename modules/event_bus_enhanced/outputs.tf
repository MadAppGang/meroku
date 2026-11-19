output "bus_arn" {
  description = "ARN of the enhanced bus; \"\" when not enabled."
  value       = try(awscc_eventsv2_event_bus.project[0].event_bus_arn, "")
}

output "bus_name" {
  description = "Name of the enhanced bus, <project>-<env>; \"\" when not enabled."
  value       = try(awscc_eventsv2_event_bus.project[0].name, "")
}

output "share_arns" {
  description = "RAM resource share ARN per access value (publish, subscribe, full)."
  value       = { for access, s in aws_ram_resource_share.bus : access => s.arn }
}
