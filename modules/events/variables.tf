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

variable "bus_type" {
  description = "classic: the project bus is aws_cloudwatch_event_bus. enhanced: the bus is created by modules/event_bus_enhanced and passed in as enhanced_bus_arn; project-bus subscriptions are delivered by subscribers, not rules."
  type        = string
  default     = "classic"

  validation {
    condition     = contains(["classic", "enhanced"], var.bus_type)
    error_message = "bus_type is classic or enhanced."
  }
}

variable "enhanced_bus_arn" {
  description = "The enhanced bus's ARN (event-busv2/<name>/<id>); \"\" on classic."
  type        = string
  default     = ""

  validation {
    condition     = !(var.bus_enabled && var.bus_type == "enhanced") || var.enhanced_bus_arn != ""
    error_message = "An enhanced project bus needs its ARN in enhanced_bus_arn."
  }
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
    nothing. Exactly one of detail_type and detail_type_prefix is set; a * in
    detail_type is EventBridge's wildcard. filters match fields of the event's
    detail: field is a path such as detail.order.type, values an exact-match
    list (any of).

    On an enhanced bus a project-bus subscription may be ordered: it is then
    delivered to the consumer's FIFO queue by its one FIFO subscriber. fifo is
    true for a consumer with an ordered subscription or with fifo_retired, which
    keeps a FIFO queue set that no subscriber delivers to any more.
  EOT
  type = list(object({
    name               = string
    visibility_timeout = optional(number, 30)
    max_receive_count  = optional(number, 5)
    fifo               = optional(bool, false)
    fifo_retired       = optional(bool, false)
    subscriptions = optional(list(object({
      key                = string
      detail_type        = optional(string, "")
      detail_type_prefix = optional(string, "")
      source             = optional(string, "")
      bus                = string
      ordered            = optional(bool, false)
      filters = optional(list(object({
        field  = string
        values = list(string)
      })), [])
    })), [])
  }))
  default = []

  validation {
    condition     = length(distinct([for c in var.consumers : c.name])) == length(var.consumers)
    error_message = "Each consumer may appear only once."
  }

  validation {
    condition     = alltrue([for c in var.consumers : length(c.subscriptions) > 0 || c.fifo_retired])
    error_message = "A consumer subscribes to at least one event, or keeps a retired ordered queue (fifo_retired)."
  }

  validation {
    condition = alltrue(flatten([for c in var.consumers : [
      for s in c.subscriptions : !s.ordered || (s.bus == "project" && var.bus_enabled && var.bus_type == "enhanced")
    ]]))
    error_message = "Only a subscription on an enhanced project bus may be ordered."
  }

  # An enhanced bus's subscribers have no wildcard operator: a single * at the
  # end becomes a prefix and a single * at the start a suffix. Anything else
  # with a * cannot be expressed there.
  validation {
    condition = alltrue(flatten([for c in var.consumers : [
      for s in c.subscriptions : !(var.bus_enabled && var.bus_type == "enhanced" && s.bus == "project") || !strcontains(s.detail_type, "*") || (
        length(s.detail_type) > 1 && length(split("*", s.detail_type)) == 2 && (startswith(s.detail_type, "*") || endswith(s.detail_type, "*"))
      )
    ]]))
    error_message = "On an enhanced project bus a detail_type holds at most one *, at its start or its end, beside other text."
  }

  validation {
    condition = alltrue([for c in var.consumers :
      !c.fifo_retired || !anytrue([for s in c.subscriptions : s.ordered])
    ])
    error_message = "A consumer with fifo_retired has no ordered subscription."
  }

  validation {
    condition = alltrue([for c in var.consumers :
      c.fifo == (anytrue([for s in c.subscriptions : s.ordered]) || c.fifo_retired)
    ])
    error_message = "fifo is true exactly when the consumer has an ordered subscription or fifo_retired."
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
      for s in c.subscriptions : !strcontains(s.detail_type_prefix, "*")
    ]]))
    error_message = "detail_type_prefix cannot contain *; use detail_type with a wildcard instead."
  }

  validation {
    condition = alltrue(flatten([for c in var.consumers : [
      for s in c.subscriptions : length(s.filters) <= 5 && alltrue([
        for f in s.filters : can(regex("^detail(\\.[A-Za-z0-9_-]+)+$", f.field)) &&
        length(f.values) >= 1 && length(f.values) <= 10 && alltrue([for v in f.values : v != ""])
      ])
    ]]))
    error_message = "A subscription has at most 5 filters; a filter's field is a path under detail (detail.status, detail.order.type) and its values 1 to 10 non-empty strings."
  }

  validation {
    condition = alltrue(flatten([for c in var.consumers : [
      for s in c.subscriptions : length(distinct([for f in s.filters : f.field])) == length(s.filters) && alltrue(flatten([
        for a in s.filters : [for b in s.filters : !startswith(b.field, "${a.field}.")]
      ]))
    ]]))
    error_message = "A subscription's filters each match a different field, and no field lies inside another (detail.order and detail.order.type together)."
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
