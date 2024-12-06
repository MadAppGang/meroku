# modules/efs/variables.tf
variable "project" {
  type        = string
  description = "Project name"
}

variable "env" {
  type        = string
  description = "Environment name"
}

variable "vpc_id" {
  type        = string
  description = "VPC ID where to create EFS"
}

variable "private_subnets" {
  type        = list(string)
  description = "List of private subnet IDs"
}

variable "efs_configs" {
  type = list(object({
    name        = string
    uid         = optional(number, 1000)
    gid         = optional(number, 1000)
    permissions = optional(string, "755")
    path        = optional(string, "/")
  }))
  description = "List of EFS configurations to create"
}

# modules/efs/main.tf
