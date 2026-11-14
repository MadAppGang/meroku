# One vault's plan-time warning: every secret the YAML declares has a parameter
# in SSM. A check block warns and never fails a plan or an apply, and an error
# reading its own data source (no permission, for example) is a warning too.
#
# The read lists names only: with_decryption is false, and a value never
# reaches the plan. Not recursive, because a vault is flat.

check "vault_values_set" {
  data "aws_ssm_parameters_by_path" "vault" {
    path            = var.path
    recursive       = false
    with_decryption = false
  }

  assert {
    condition = length(setsubtract(
      toset(var.names),
      toset([for p in data.aws_ssm_parameters_by_path.vault.names : reverse(split("/", p))[0]]),
    )) == 0
    error_message = "Vault \"${var.vault}\" has no value yet for ${join(", ", sort(tolist(setsubtract(toset(var.names), toset([for p in data.aws_ssm_parameters_by_path.vault.names : reverse(split("/", p))[0]])))))} (under ${var.path}/). Set the values in the web UI's Vaults tab. Until then, a new task of a workload connected to this vault cannot start."
  }
}
