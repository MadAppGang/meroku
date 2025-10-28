output "backend_ecr_repo_url" {
  value = join("", aws_ecr_repository.backend.*.repository_url)
}

output "ecr_cluster" {
  value = aws_ecs_cluster.main
}

output "backend_task_role_name" {
  value = aws_iam_role.backend_task.name
}

output "account_id" {
  value = data.aws_caller_identity.current.account_id
}

output "backend_cloud_map_arn" {
  description = "value of the backend service discovery ARN"
  value       = try(aws_service_discovery_service.backend[0].arn, "")
}

# ============================================================================
# Per-Service ECR Outputs (Schema v10)
# ============================================================================

output "service_ecr_repositories" {
  description = "Map of service ECR repositories (only for services with mode=create_ecr)"
  value = {
    for svc in local.services_needing_ecr :
    svc.name => {
      repository_url = aws_ecr_repository.services[svc.name].repository_url
      repository_arn = aws_ecr_repository.services[svc.name].arn
      registry_id    = aws_ecr_repository.services[svc.name].registry_id
    }
  }
}

output "service_ecr_url_map" {
  description = "Map of all service ECR URLs (resolved based on ecr_config mode)"
  value       = local.service_ecr_urls
}

