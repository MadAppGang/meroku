output "subnet_ids" {
  value = data.aws_subnets.all.ids
}


output "alb_dns_name" {
  value = module.workloads.alb_dns_name
}

output "backend_ecr_repo_url" {
  value = module.workloads.backend_ecr_repo_url
}

