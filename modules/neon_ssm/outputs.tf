output "dependency_tokens" {
  description = "Service name => \"<parameter name>@<version>\". modules/workloads gates each service's SSM read on its own token, so a changed DATABASE_URL reaches that service's task definition in the same apply, and no other's."
  value       = local.dependency_tokens
}
