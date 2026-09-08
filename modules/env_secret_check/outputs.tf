output "valid" {
  description = "Per workload: may its task definition be registered? False when any name is in both `environment` and `secrets`."
  value       = local.valid
}

output "message" {
  description = "Per workload: the sentence a precondition shows when `valid` is false. Total, so a caller may interpolate it unconditionally."
  value       = local.message
}

output "collisions" {
  description = "Per workload: the colliding names, sorted. Empty when there are none. Exposed for tests and for anyone who wants the list without parsing the message."
  value       = local.collisions
}
