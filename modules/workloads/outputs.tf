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

