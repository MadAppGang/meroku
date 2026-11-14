# One vault's plan-time warning: every secret the YAML declares has a parameter
# in SSM. A check block warns and never fails a plan or an apply, and an error
# reading its own data source (no permission, for example) is a warning too.
#
# with_decryption is false, so a SecureString value comes back encrypted. The
# value of a String or StringList parameter has no encryption to skip: this read
# returns it in plaintext, and it appears in the plan. The Vaults tab always
# writes SecureString; a parameter written another way as String is exposed
# here. Not recursive, because a vault is flat.

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
