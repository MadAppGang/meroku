# No provider: this module computes strings and creates nothing. It therefore
# carries no `required_providers` block at all, which also means adding it to a
# module costs no extra provider resolution during `terraform init`.
#
# The floor is 1.3.0 rather than the repository's usual 1.2.6 because
# `variable "requests"` uses `optional()` with defaults inside an object type
# constraint, which 1.2 parses but does not honour.
terraform {
  required_version = ">= 1.3.0"
}
