variable "mode" {
  description = "create: meroku creates the Neon project. reuse: an existing project, read as data."
  type        = string

  validation {
    condition     = contains(["create", "reuse"], var.mode)
    error_message = "mode must be \"create\" or \"reuse\"."
  }
}

variable "project_id" {
  description = "Id of the existing Neon project. Reuse mode only."
  type        = string
  default     = ""

  validation {
    condition     = var.mode != "reuse" || var.project_id != ""
    error_message = "project_id is required when mode is \"reuse\"."
  }
}

variable "project" {
  description = "The project meroku creates. Create mode only."
  type = object({
    name       = string
    org_id     = string
    region_id  = string
    pg_version = number
  })
  default = null

  validation {
    condition     = var.mode != "create" || var.project != null
    error_message = "project is required when mode is \"create\"."
  }
}

variable "owner_role" {
  description = "The project's owner role. It owns every database meroku creates and every table, sequence and schema in them."
  type        = string

  validation {
    condition     = var.owner_role != ""
    error_message = "owner_role must not be empty."
  }
}

variable "databases" {
  description = "Databases on the project's default branch, keyed by name. create = true: meroku creates it; false: it exists and is read as data."
  type = map(object({
    create = bool
  }))
}
