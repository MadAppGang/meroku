variable "env" {
  description = "meroku environment name."
  type        = string
}

variable "project_id" {
  description = "Neon project id (module.neon.project_id)."
  type        = string
}

variable "branch_id" {
  description = "Default branch id (module.neon.default_branch_id)."
  type        = string
}

variable "database" {
  description = "The database this connection serves."
  type        = string
}

variable "admin_role" {
  description = "The Neon owner role (neon.owner_role). It owns the objects, issues every grant and is the FOR ROLE of every default privilege."
  type        = string
}

variable "pooler_host" {
  description = "Pooled host written into each service's DATABASE_URL (module.neon.pooler_host)."
  type        = string
}

variable "revoke_public" {
  description = "True when meroku created the database: PUBLIC loses its default CONNECT and TEMPORARY on it."
  type        = bool
}

variable "services" {
  description = "Service roles in this database, keyed by service name (the SSM path segment). schemas is also the role's search_path."
  type = map(object({
    schemas          = list(string)
    access           = string
    role_name        = string
    password_version = number
  }))

  validation {
    condition     = alltrue([for s in var.services : contains(["readonly", "readwrite"], s.access)])
    error_message = "services[*].access must be \"readonly\" or \"readwrite\"."
  }

  validation {
    condition = alltrue([for s in var.services :
    length(s.role_name) <= 63 && can(regex("^[a-z_][a-z0-9_]*$", s.role_name))])
    error_message = "services[*].role_name must be at most 63 bytes of lower-case letters, digits and underscores, and must not start with a digit."
  }
}
