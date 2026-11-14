variable "project" {
  type = string
}

variable "env" {
  type = string
}

variable "bus_enabled" {
  description = "Create the project bus <project>-<env>. Off, every entry is on the default bus."
  type        = bool
  default     = false
}

variable "workloads" {
  description = "Every workload of the environment, keyed by name, as the workload contract describes it. Only the task role name and the event source are read here. The backend is the entry keyed \"backend\"."
  type = map(object({
    task_role_name = string
    event_source   = string
  }))
}

variable "consumers" {
  description = <<-EOT
    The workloads that subscribe to events, each with its own queue. bus is
    "project" or "default", resolved by meroku. key is the subscription's stable
    identity: the rule's name and for_each key, so reordering entries changes
    nothing. Exactly one of detail_type and detail_type_prefix is set.
  EOT
  type = list(object({
    name               = string
    visibility_timeout = optional(number, 30)
    max_receive_count  = optional(number, 5)
    subscriptions = list(object({
      key                = string
      detail_type        = optional(string, "")
      detail_type_prefix = optional(string, "")
      source             = optional(string, "")
      bus                = string
    }))
  }))
  default = []

  validation {
    condition     = length(distinct([for c in var.consumers : c.name])) == length(var.consumers)
    error_message = "Each consumer may appear only once."
  }

  validation {
    condition     = alltrue([for c in var.consumers : length(c.subscriptions) > 0])
    error_message = "A consumer subscribes to at least one event."
  }

  validation {
    condition = alltrue(flatten([for c in var.consumers : [
      for s in c.subscriptions : can(regex("^[a-z0-9][a-z0-9-]{0,39}$", s.key))
    ]]))
    error_message = "A subscription key is 1 to 40 lower-case letters, digits and hyphens, starting with a letter or a digit."
  }

  validation {
    condition = alltrue([for c in var.consumers :
      length(distinct([for s in c.subscriptions : s.key])) == length(c.subscriptions)
    ])
    error_message = "Two subscriptions of one consumer have the same key; give one of them a distinct name."
  }

  validation {
    condition = alltrue(flatten([for c in var.consumers : [
      for s in c.subscriptions : (s.detail_type == "") != (s.detail_type_prefix == "")
    ]]))
    error_message = "A subscription sets exactly one of detail_type and detail_type_prefix."
  }

  validation {
    condition = alltrue(flatten([for c in var.consumers : [
      for s in c.subscriptions : contains(["default", "project"], s.bus) && (s.bus == "default" || var.bus_enabled)
    ]]))
    error_message = "A subscription's bus is \"default\", or \"project\" when bus_enabled is true."
  }

  validation {
    condition = alltrue([for c in var.consumers :
      c.visibility_timeout >= 0 && c.visibility_timeout <= 43200 && c.max_receive_count >= 1 && c.max_receive_count <= 1000
    ])
    error_message = "visibility_timeout is 0 to 43200 seconds and max_receive_count is 1 to 1000."
  }
}

variable "producers" {
  description = <<-EOT
    The workloads that may publish, each with its PutEvents grants. source is the
    workload's EVENT_SOURCE; it must equal the contract's event_source. An entry
    lists detail_types or sets any = true. own_source_only limits the entry to
    events whose source is the workload's own.
  EOT
  type = list(object({
    name   = string
    source = string
    entries = list(object({
      bus             = string
      detail_types    = optional(list(string), [])
      any             = optional(bool, false)
      own_source_only = optional(bool, true)
    }))
  }))
  default = []

  validation {
    condition     = length(distinct([for p in var.producers : p.name])) == length(var.producers)
    error_message = "Each producer may appear only once."
  }

  validation {
    condition = alltrue(flatten([for p in var.producers : [
      for e in p.entries : e.any != (length(e.detail_types) > 0)
    ]]))
    error_message = "A produces entry lists detail_types or sets any = true, not both and not neither."
  }

  validation {
    condition = alltrue(flatten([for p in var.producers : [
      for e in p.entries : contains(["default", "project"], e.bus) && (e.bus == "default" || var.bus_enabled)
    ]]))
    error_message = "A produces entry's bus is \"default\", or \"project\" when bus_enabled is true."
  }
}
