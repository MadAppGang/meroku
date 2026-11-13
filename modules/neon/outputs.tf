output "project_id" {
  description = "Id of the Neon project."
  value       = local.project_id
}

output "default_branch_id" {
  description = "Id of the project's default branch, where every database and role lives."
  value       = local.default_branch_id
}

output "host" {
  description = "Direct host of the default branch's read_write endpoint. The postgresql provider connects here."
  value       = local.host
}

output "pooler_host" {
  description = "Pooled (PgBouncer) host of the same endpoint. Every service DATABASE_URL uses it."
  value       = local.pooler_host
}
