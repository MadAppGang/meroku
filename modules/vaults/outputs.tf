# Per workload, the secrets its container gets from its vaults: every workload
# of var.workloads has an entry, empty when it is connected to none. `origin`
# names the vault and parameter, for the collision messages in
# modules/env_secret_check; the owning modules drop it before the container
# definition.
#
# The value reads the read grant's name, so whatever consumes it is ordered
# after the grant. The name is a configured value, never .id, so the secrets stay
# known at plan.
output "secrets" {
  description = "Vault secrets for each workload, keyed by workload name: name, valueFrom and origin, sorted by name."
  value = { for w in keys(var.workloads) : w => [
    for s in lookup(local.workload_secrets_sorted, w, []) : s if lookup(local.execution_grant_names, w, "") != ""
  ] }
}

output "vault_paths" {
  description = "The SSM path of each vault some workload is connected to, keyed by vault name."
  value       = local.vault_path
}
