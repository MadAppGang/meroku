output "alb_dns_name" {
  value = aws_lb.alb.dns_name
}

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
