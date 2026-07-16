output "ecr_repo_url" {
  description = "Repository URL this task's image comes from (per-task ECR repo in dev, var.ecr_url otherwise). Used by ecr_config mode \"use_existing\" consumers."
  value       = local.ecr_image
}
