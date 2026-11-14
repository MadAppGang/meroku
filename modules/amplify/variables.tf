variable "amplify_apps" {
  description = "List of Amplify app configurations"
  type = list(object({
    name                  = string
    github_repository     = string
    environment_variables = optional(map(string), {})
    branches = list(object({
      name                        = string
      stage                       = optional(string, "DEVELOPMENT")
      enable_auto_build           = optional(bool, true)
      enable_pull_request_preview = optional(bool, false)
      environment_variables       = optional(map(string), {})
      custom_subdomains           = optional(list(string), [])
    }))
    subdomain_prefix = optional(string)     # NEW: Auto-constructs domain
    custom_domain    = optional(string)     # For manual override (edge cases)
    spa_mode         = optional(bool, true) # Per-app SPA routing control
    # Rewrites and redirects, applied in order before the SPA rule. A rule has
    # either a fixed target, or a workload whose URL comes from workload_urls
    # with target_path appended.
    custom_rules = optional(list(object({
      source      = string
      target      = optional(string, "")
      workload    = optional(string, "")
      target_path = optional(string, "")
      status      = optional(string, "200")
    })), [])
  }))
  default = []
}

variable "workload_urls" {
  description = "Base URL of each workload (backend or a service) that a custom rule names, without a trailing slash"
  type        = map(string)
  default     = {}
}

variable "project" {
  description = "Project name"
  type        = string
}

variable "env" {
  description = "Environment name"
  type        = string
}

variable "base_domain" {
  description = "Base domain name from domain configuration (e.g., example.com)"
  type        = string
  default     = ""
}

variable "add_env_domain_prefix" {
  description = "Whether to add environment prefix to domains (domain.add_env_domain_prefix; always true for a delegated subdomain, domain.root_zone_id)"
  type        = bool
  default     = true
}

variable "zone_id" {
  description = "Route53 hosted zone ID for creating DNS records for Amplify domain verification"
  type        = string
  default     = ""
}

variable "manage_dns_records" {
  description = <<-EOT
    Whether Terraform should manage Route53 DNS records for Amplify domains.
    Set to FALSE (default) when Route53 is in the SAME AWS account - Amplify auto-creates records.
    Set to TRUE only when Route53 is in a DIFFERENT account or managed externally.
  EOT
  type        = bool
  default     = false
}

