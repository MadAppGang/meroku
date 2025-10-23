variable "amplify_apps" {
  description = "List of Amplify app configurations"
  type = list(object({
    name                  = string
    github_repository     = string
    environment_variables = optional(map(string), {})
    branches             = list(object({
      name                          = string
      stage                        = optional(string, "DEVELOPMENT")
      enable_auto_build            = optional(bool, true)
      enable_pull_request_preview  = optional(bool, false)
      environment_variables        = optional(map(string), {})
      custom_subdomains            = optional(list(string), [])
    }))
    subdomain_prefix     = optional(string)       # NEW: Auto-constructs domain
    custom_domain        = optional(string)       # For manual override (edge cases)
  }))
  default = []
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
  description = "Whether to add environment prefix to domains (from domain.add_env_domain_prefix)"
  type        = bool
  default     = true
}