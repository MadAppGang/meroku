variable "amplify_apps" {
  description = "List of Amplify app configurations"
  type = list(object({
    name                  = string
    github_repository     = string
    branches             = list(object({
      name                          = string
      stage                        = optional(string, "DEVELOPMENT")
      enable_auto_build            = optional(bool, true)
      enable_pull_request_preview  = optional(bool, false)
      environment_variables        = optional(map(string), {})
      custom_subdomains            = optional(list(string), [])
    }))
    custom_domain        = optional(string)
    enable_root_domain   = optional(bool, false)
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