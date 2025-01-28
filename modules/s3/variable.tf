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
    name       = string
    public     = optional(bool, false) # Added public option, defaults to false
    versioning = optional(bool, true)  # Added versioning option, defaults to true
    cors_rules = optional(list(object({
      allowed_headers = optional(list(string), ["*"])
      allowed_methods = optional(list(string), ["GET", "PUT", "POST", "DELETE", "HEAD"])
      allowed_origins = optional(list(string), ["*"])
      expose_headers  = optional(list(string), ["ETag"])
      max_age_seconds = optional(number, 3600)
      })), [{ # Default CORS rule that accepts everything
      allowed_headers = ["*"]
      allowed_methods = ["GET", "PUT", "POST", "DELETE", "HEAD"]
      allowed_origins = ["*"]
      expose_headers  = ["ETag"]
      max_age_seconds = 3600
    }])
  }))
  description = "List of bucket configurations to create"
}
