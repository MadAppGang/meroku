variable "project" {
  type = string
}

variable "env" {
  type = string
}

variable "workloads" {
  description = "Every workload of the environment, keyed by name, as the workload contract describes it. Only the two role names are read here."
  type = map(object({
    task_role_name      = string
    execution_role_name = string
  }))
}

variable "vaults" {
  description = "Every vault of the environment, keyed by name, with the secret names the YAML declares for it. Each name is the last segment of the parameter /<env>/<project>/vault/<vault>/<name>."
  type        = map(list(string))
  default     = {}

  validation {
    condition     = alltrue([for v, names in var.vaults : can(regex("^[a-z0-9]([a-z0-9_-]{0,30}[a-z0-9])?$", v))])
    error_message = "A vault name is 1 to 32 characters of lower-case letters, digits, hyphens and underscores, and starts and ends with a letter or a digit."
  }

  validation {
    condition     = alltrue(flatten([for v, names in var.vaults : [for n in names : can(regex("^[a-z][a-z0-9_]{0,62}$", n))]]))
    error_message = "A vault secret name is 1 to 63 lower-case letters, digits and underscores, starting with a letter."
  }

  validation {
    condition     = alltrue([for v, names in var.vaults : length(distinct(names)) == length(names)])
    error_message = "A vault lists each secret name once."
  }
}

variable "connections" {
  description = "One entry per workload and vault it is connected to. `access` is read or read_write; `prefix` is put before each secret name, joined with an underscore."
  type = list(object({
    workload = string
    vault    = string
    access   = optional(string, "read")
    prefix   = optional(string, "")
  }))
  default = []

  validation {
    condition     = alltrue([for c in var.connections : contains(["read", "read_write"], c.access)])
    error_message = "A vault connection's access is read or read_write."
  }

  validation {
    condition     = alltrue([for c in var.connections : c.prefix == "" || can(regex("^[A-Z][A-Z0-9_]*$", c.prefix))])
    error_message = "A vault connection's prefix is upper-case letters, digits and underscores, starting with a letter."
  }

  validation {
    condition     = alltrue([for c in var.connections : contains(keys(var.vaults), c.vault)])
    error_message = "Every vault connection names a vault of var.vaults."
  }

  validation {
    condition     = alltrue([for c in var.connections : contains(keys(var.workloads), c.workload)])
    error_message = "Every vault connection names a workload of var.workloads."
  }

  validation {
    condition     = length(distinct([for c in var.connections : "${c.workload}/${c.vault}"])) == length(var.connections)
    error_message = "A workload may be connected to each vault only once."
  }
}
