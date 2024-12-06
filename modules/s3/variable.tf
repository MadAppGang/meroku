# modules/efs/variables.tf
variable "project" {
  type        = string
  description = "Project name"
}

variable "env" {
  type        = string
  description = "Environment name"
}

variable "buckets" {
  type = list(object({
    name        = string
    public      = optional(bool, false)  # Added public option, defaults to false
    versioning  = optional(bool, true)   # Added versioning option, defaults to true
  }))
  description = "List of bucket configurations to create"
}

