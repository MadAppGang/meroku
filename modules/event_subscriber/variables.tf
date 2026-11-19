variable "project" {
  description = "Project name; tags the subscriber and checks its name."
  type        = string
}

variable "env" {
  description = "Environment name."
  type        = string
}

variable "enabled" {
  description = "false makes this a tombstone: what state holds is deleted and nothing is created."
  type        = bool
  default     = true
}

variable "kind" {
  description = "standard (an UNORDERED subscriber on the consumer's standard queue) or fifo (a FIFO subscriber on its FIFO queue)."
  type        = string

  validation {
    condition     = contains(["standard", "fifo"], var.kind)
    error_message = "kind is standard or fifo."
  }
}

variable "bus_arn" {
  description = "ARN of the enhanced bus (module.event_bus_enhanced.bus_arn)."
  type        = string
  default     = ""

  validation {
    condition     = !var.enabled || var.bus_arn != ""
    error_message = "bus_arn is required unless the subscriber is a tombstone."
  }
}

variable "subscription" {
  description = "One entry of modules/events' subscribers or fifo_subscribers output. null only on a tombstone."
  type = object({
    name            = string
    type            = string
    target_arn      = string
    dead_letter_arn = string
    role_arn        = string
    pattern         = string
  })
  default = null

  validation {
    condition     = !var.enabled || var.subscription != null
    error_message = "subscription is required unless the subscriber is a tombstone."
  }
}
