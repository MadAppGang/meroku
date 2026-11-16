# Per workload, the variables its container gets: every workload of
# var.workloads has an entry, empty when it is connected to no key. Sorted by key
# name. An HMAC key has no public key, so it gives only its ARN. The public key
# is a multi-line PEM; ECS passes an environment value as a JSON string, so its
# newlines reach the container unchanged.
output "environment" {
  description = "Environment variables for each workload, keyed by workload name: KMS_<NAME>_KEY_ARN for each connected key, and KMS_<NAME>_PUBLIC_KEY for each connected asymmetric key."
  value = { for w in keys(var.workloads) : w => flatten([
    for k in lookup(local.workload_keys, w, []) : concat(
      [{ name = "KMS_${upper(k)}_KEY_ARN", value = aws_kms_key.key[k].arn }],
      contains(local.hmac_keys, k) ? [] : [
        { name = "KMS_${upper(k)}_PUBLIC_KEY", value = module.public_keys.public_keys[k] },
      ],
    )
  ]) }
}

output "key_arns" {
  description = "The ARN of every key, retired ones included, keyed by key name."
  value       = { for k, key in aws_kms_key.key : k => key.arn }
}

output "aliases" {
  description = "The alias of every key, keyed by key name."
  value       = { for k, a in aws_kms_alias.key : k => a.name }
}

output "policies" {
  description = "The inline policy document written on each connected workload's task role, keyed by workload name."
  value       = { for w, p in aws_iam_role_policy.workload : w => p.policy }
}
