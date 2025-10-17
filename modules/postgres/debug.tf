# Temporary debug output to diagnose username issue
output "debug_username_raw" {
  value       = var.username
  description = "Raw username variable value"
}

output "debug_username_processed" {
  value       = local.db_username
  description = "Processed username local value"
}

output "debug_username_length" {
  value       = length(var.username)
  description = "Length of username variable"
}

output "debug_username_type" {
  value       = can(tostring(var.username)) ? "string" : "unknown"
  description = "Type of username variable"
}