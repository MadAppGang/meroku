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
# The floor stays at the repository's usual 1.2.6 rather than the 1.3.0 that
# modules/naming and modules/compute_pool_check carry: neither `optional()` nor
# anything else newer than 1.2 appears below, so there is no reason to raise it.
terraform {
  required_version = ">= 1.2.6"
}
