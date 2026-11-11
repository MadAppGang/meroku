output "projects" {
  description = "Runner name => CodeBuild project name."
  value       = { for name, project in aws_codebuild_project.runner : name => project.name }
}

# `$${{` renders a literal `${{`: the label is pasted into a workflow, where
# GitHub expands it, not Terraform.
output "runs_on_labels" {
  description = "Runner name => the exact `runs-on` value a workflow job uses to run on that runner."
  value = {
    for name, project in local.project_names :
    name => "codebuild-${project}-$${{ github.run_id }}-$${{ github.run_attempt }}"
  }
}

output "service_role_arn" {
  description = "ARN of the IAM role the runner builds run as."
  value       = aws_iam_role.runner.arn
}

output "security_group_id" {
  description = "Security group of the runners when vpc_access is true, otherwise null."
  value       = var.vpc_access ? aws_security_group.runner[0].id : null
}
