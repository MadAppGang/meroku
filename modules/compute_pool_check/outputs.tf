output "valid" {
  description = "Per workload: may it be created? False only for a runtime \"ec2\" workload naming a pool that is missing or disabled."
  value       = local.valid
}

output "message" {
  description = "Per workload: the sentence a precondition shows when `valid` is false. Total, so a caller may interpolate it unconditionally."
  value       = local.message
}
