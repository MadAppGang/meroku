output "role_names" {
  description = "Service name => Postgres role name."
  value       = local.role_names
}

output "database_urls" {
  description = "Service name => postgresql:// URL on the pooled host, with sslmode=require."
  value       = local.database_urls
  sensitive   = true
}
