output "alb_dns_name" {
  value = aws_lb.alb.dns_name
}

output "backend_ecr_repo_url" {
  value = join("", aws_ecr_repository.backend.*.repository_url)
}
