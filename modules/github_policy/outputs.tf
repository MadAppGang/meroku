output "json" {
  description = <<-EOT
    The rendered IAM policy, exactly as aws_iam_role_policy.github_access
    receives it. This is the contract of the module: the string the deploy role
    actually carries, and therefore the only artifact a test can hold the
    security requirement against.
  EOT
  value       = data.aws_iam_policy_document.github.json
}

output "ecr_repository_arns" {
  description = <<-EOT
    Every ECR resource the repository-scoped statement grants: this project's
    three name prefixes in this account, plus the same three in the source
    account under ecr_strategy = "cross_account". Constant size by design — it
    does not grow with the service or scheduled-task count.

    Exposed alongside `json` because it is the input to the property, and a
    failure that names the offending ARN reads better than one that names a
    substring of a JSON blob.
  EOT
  value       = local.github_ecr_repository_arns
}

output "ecr_scoped_actions" {
  description = <<-EOT
    The ECR actions granted at repository scope. ecr:GetAuthorizationToken is
    deliberately not among them — AWS evaluates it at the registry, so pairing
    it with repository ARNs denies it and breaks `docker login` in every
    generated workflow. It has its own statement on "*" in `json`.
  EOT
  value       = local.github_ecr_scoped_actions
}
