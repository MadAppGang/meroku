# One vault's plan-time warning: every secret the YAML declares has a parameter
# in SSM. A check block warns and never fails a plan or an apply.
#
# The parameters are read by a plain data source, outside the check block. A
# data source nested in a check block is deferred to apply, so every plan would
# list its read and never report "No changes". Read here, it resolves at plan
# (its path is configuration), and a read error, such as a missing permission,
# fails the plan like any other data source.
#
# with_decryption is false, so a SecureString value comes back encrypted. The
# value of a String or StringList parameter has no encryption to skip: this read
# returns it in plaintext, and it appears in the plan. The Vaults tab always
# writes SecureString; a parameter written another way as String is exposed
# here. Not recursive, because a vault is flat.

data "aws_ssm_parameters_by_path" "vault" {
  path            = var.path
  recursive       = false
  with_decryption = false
}

locals {
  missing = sort(tolist(setsubtract(
    toset(var.names),
    toset([for p in data.aws_ssm_parameters_by_path.vault.names : reverse(split("/", p))[0]]),
  )))
}

check "vault_values_set" {
  assert {
    condition     = length(local.missing) == 0
    error_message = "Vault \"${var.vault}\" has no value yet for ${join(", ", local.missing)} (under ${var.path}/). Set the values in the web UI's Vaults tab. Until then, a new task of a workload connected to this vault cannot start."
  }
}
