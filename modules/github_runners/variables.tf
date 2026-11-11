variable "project" {
  description = "Project slug, as it appears in the environment YAML."
  type        = string
}

variable "env" {
  description = "Environment slug (dev, staging, prod)."
  type        = string
}

variable "token_secret" {
  description = <<-EOT
    Name (or ARN) of the Secrets Manager secret holding the GitHub token. It is
    created outside Terraform, so the token never passes through state. Its
    format is in ai_docs/GITHUB_RUNNERS.md.
  EOT
  type        = string

  validation {
    condition     = length(trimspace(var.token_secret)) > 0
    error_message = "token_secret must name the Secrets Manager secret that holds the GitHub token."
  }
}

variable "vpc_access" {
  description = <<-EOT
    Run builds inside the VPC. CodeBuild gives a build in a VPC no public IP,
    so subnet_ids must be private subnets behind a NAT Gateway.
  EOT
  type        = bool
  default     = false
}

variable "vpc_id" {
  description = "VPC for the runners' security group. Used only when vpc_access is true."
  type        = string
  default     = ""

  validation {
    condition     = !var.vpc_access || var.vpc_id != ""
    error_message = "vpc_access is true, so vpc_id must be set."
  }
}

variable "subnet_ids" {
  description = "Private subnets with a NAT route. Used only when vpc_access is true."
  type        = list(string)
  default     = []

  validation {
    condition     = !var.vpc_access || length(var.subnet_ids) > 0
    error_message = "vpc_access is true, so subnet_ids must list the private subnets behind the NAT Gateway."
  }
}

variable "runners" {
  description = <<-EOT
    One CodeBuild project per entry. Exactly one of repository (owner/repo) and
    organization is set. secrets entries set exactly one of ssm (a Parameter
    Store name or ARN) and secrets_manager (name-or-arn[:json-key]).
  EOT

  type = list(object({
    name            = string
    repository      = optional(string, "")
    organization    = optional(string, "")
    compute_type    = optional(string, "BUILD_GENERAL1_SMALL")
    image           = optional(string, "aws/codebuild/amazonlinux-x86_64-standard:5.0")
    docker          = optional(bool, false)
    timeout_minutes = optional(number, 60)
    env_variables = optional(list(object({
      name  = string
      value = string
    })), [])
    secrets = optional(list(object({
      name            = string
      ssm             = optional(string, "")
      secrets_manager = optional(string, "")
    })), [])
  }))

  validation {
    condition     = length(var.runners) > 0
    error_message = "runners must list at least one runner."
  }

  validation {
    condition     = alltrue([for r in var.runners : can(regex("^[a-z0-9]([a-z0-9-]*[a-z0-9])?$", r.name))])
    error_message = "Every runner name must be lower-case letters, digits and hyphens, starting and ending with a letter or digit."
  }

  validation {
    condition     = length(distinct([for r in var.runners : r.name])) == length(var.runners)
    error_message = "Runner names must be unique."
  }

  validation {
    condition     = alltrue([for r in var.runners : (r.repository == "") != (r.organization == "")])
    error_message = "Every runner sets exactly one of repository or organization."
  }

  validation {
    condition = alltrue([for r in var.runners : contains([
      "BUILD_GENERAL1_SMALL", "BUILD_GENERAL1_MEDIUM", "BUILD_GENERAL1_LARGE",
      "BUILD_GENERAL1_XLARGE", "BUILD_GENERAL1_2XLARGE",
    ], r.compute_type)])
    error_message = "compute_type must be one of BUILD_GENERAL1_SMALL, BUILD_GENERAL1_MEDIUM, BUILD_GENERAL1_LARGE, BUILD_GENERAL1_XLARGE, BUILD_GENERAL1_2XLARGE."
  }

  validation {
    condition     = alltrue(flatten([for r in var.runners : [for s in r.secrets : (s.ssm == "") != (s.secrets_manager == "")]]))
    error_message = "Every secret sets exactly one of ssm or secrets_manager."
  }
}
