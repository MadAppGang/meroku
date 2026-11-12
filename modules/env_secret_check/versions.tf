# No provider: this module compares two lists of strings and creates nothing,
# exactly like modules/naming and modules/compute_pool_check. That is the whole
# point of it. modules/workloads reads eight remote data sources, so it can
# never be planned in CI without credentials, and `terraform validate` does not
# evaluate a precondition's error_message at all. Neither gate can see a message
# that fails to render. This module can be planned by `terraform test` with no
# provider and no network, so the message is checked where checking it is
# possible — see tests/messages.tftest.hcl for what that blind spot cost once
# already (v4.2.0, four releases).
#
# variables.tf uses optional() object attributes, which need 1.3. The floor
# matches env/main.hbs, the root every caller of this module is planned from,
# so it asks no environment for a newer Terraform.
terraform {
  required_version = ">= 1.10.0"
}
