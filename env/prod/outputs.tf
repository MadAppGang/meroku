output "subnet_ids" {
  value = data.aws_subnets.all.ids
}

output "alb_dns_name" {
  value = module.workloads.chubby_alb_dns_name
}

output "chubby_backend_ecr_repo_url" {
  value = module.workloads.chubby_backend_ecr_repo_url
}

