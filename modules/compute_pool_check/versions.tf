# No provider: this module compares strings and creates nothing, exactly like
# modules/naming. That is the whole point of it. modules/workloads reads eight
# remote data sources, so it can never be planned in CI without credentials,
# and `terraform validate` does not evaluate a precondition's error_message at
# all. Neither gate can see a message that fails to render. This module can be
# planned by `terraform test` with no provider and no network, so the message
# is checked where checking it is possible.
#
# The floor is 1.3.0 rather than the repository's usual 1.2.6 because
# `variable "workloads"` uses `optional()` inside an object type constraint,
# which 1.2 parses but does not honour.
terraform {
  required_version = ">= 1.3.0"
}
