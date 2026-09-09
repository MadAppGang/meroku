# Every input is a plain string, and that is a design constraint rather than an
# accident of extraction.
#
# The two values this document is built from that the caller reads off AWS —
# the account ID and the region — arrive here as VARIABLES. In modules/workloads
# they come from `data.aws_caller_identity.current` and `data.aws_region.current`
# (modules/workloads/main.tf:14-15). Reproducing those data sources here would
# make this module unplannable without credentials and a network, which is
# precisely the property that made the policy untestable in the first place.
# Keeping them as inputs is what lets `terraform test` render the real JSON on a
# laptop and in CI with no AWS account at all.
#
# Do not add a data source to this module. If a future statement needs another
# AWS-derived value, pass it in.

variable "project" {
  description = "Project slug, as it appears in the environment YAML. Everything this policy grants is scoped by it."
  type        = string

  validation {
    condition     = length(var.project) > 0
    error_message = "project must not be empty; it is the only thing separating this project's ECR namespace from a neighbouring project's in a shared account."
  }
}

variable "env" {
  description = "Environment slug (dev, staging, prod). Used by the iam:PassRole scope, which names roles suffixed with it."
  type        = string

  validation {
    condition     = length(var.env) > 0
    error_message = "env must not be empty; the iam:PassRole patterns end in it, and an empty value would widen them across every environment in the account."
  }
}

variable "aws_account_id" {
  description = "The account this role is created in — data.aws_caller_identity.current.account_id at the caller. THIS account, never var.ecr_account_id."
  type        = string

  validation {
    condition     = length(var.aws_account_id) > 0
    error_message = "aws_account_id must not be empty. \"arn:aws:ecr:us-east-1::repository/x\" is a syntactically valid ARN that matches nothing at all, so an empty value produces a grant that looks present in the plan and denies at the call."
  }
}

variable "region" {
  description = "The region this role's repositories live in — data.aws_region.current.name at the caller. THIS region, never var.ecr_account_region."
  type        = string

  validation {
    condition     = length(var.region) > 0
    error_message = "region must not be empty. \"arn:aws:ecr::123456789012:repository/x\" is a syntactically valid ARN that matches nothing at all, so an empty value silently denies every push in every region."
  }
}

variable "ecr_strategy" {
  description = "ECR repository strategy: 'local' to create ECR in this account, 'cross_account' to pull from another account. Mirrors modules/workloads/variables.tf:187."
  type        = string
  default     = "local"

  validation {
    condition     = contains(["local", "cross_account"], var.ecr_strategy)
    error_message = "ecr_strategy must be either 'local' or 'cross_account'"
  }
}

variable "ecr_account_id" {
  description = "AWS account ID of the SOURCE registry under ecr_strategy = 'cross_account'. Empty otherwise, and empty is handled: see the guard on local.github_ecr_source_repository_arns."
  type        = string
  default     = ""
}

variable "ecr_account_region" {
  description = "AWS region of the SOURCE registry under ecr_strategy = 'cross_account'. Empty otherwise, and guarded for the same reason as ecr_account_id."
  type        = string
  default     = ""
}
