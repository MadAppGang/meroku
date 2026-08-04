output "backend_id" {
  description = "Identifier of the backend service."
  value       = local.backend_id
}

output "service_ids" {
  description = "Service name -> identifier."
  value       = local.service_ids
}

output "task_ids" {
  description = "Scheduled task name -> identifier."
  value       = local.task_ids
}

output "ecr_repo_ids" {
  description = "ECR repository name -> every identifier that deploys from it. Becomes ECR_REPO_MAP, and the repository allow-list in the ECR event pattern."
  value       = { for r in local.repos : r => sort([for p in local.repo_pairs : p.id if p.repo == r]) }
}

output "ssm_prefix_ids" {
  description = "SSM path prefix -> identifier. Becomes SSM_SERVICE_MAP."
  value       = { for p in local.ssm_pairs : p.prefix => p.id }
}

output "auto_deploy" {
  description = <<-EOT
    Identifier -> auto-deploy policy, covering the backend, every service and
    every scheduled task. lambda.tf writes it onto the target objects in
    ECS_SERVICE_MAP and SCHEDULED_TASK_MAP; the Lambda reads it and reports it.
    It is never used to drop an entry from a map.
  EOT
  value       = local.auto_deploy
}
