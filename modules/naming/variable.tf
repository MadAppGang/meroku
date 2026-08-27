variable "project" {
  description = "Project slug, as it appears in the environment YAML."
  type        = string
}

variable "env" {
  description = "Environment slug (dev, staging, prod)."
  type        = string
}

variable "requests" {
  description = <<-EOT
    One entry per AWS name this caller builds.

      legacy    The exact string this resource is named TODAY. Pass it for any
                resource that may already exist in someone's AWS account; leave
                it out for a brand-new one. When it fits the limit it is
                returned untouched, which is what keeps this module from
                renaming live infrastructure.

      parts     The identity of the thing, most significant first, WITHOUT
                project or env — those are added back by the module. This is
                what survives when the decorated form does not fit.

      limit     The AWS cap for this resource type. See naming.md for the table.

      separator "-" or "_". AWS is inconsistent about which it accepts; match
                whatever `legacy` uses so forms 1 and 2 read alike.

      suffix    Required trailing text that must survive truncation, e.g.
                ".fifo" on a FIFO queue. Counted against the budget, never cut.
  EOT

  type = map(object({
    legacy    = optional(string, "")
    parts     = list(string)
    limit     = number
    separator = optional(string, "-")
    suffix    = optional(string, "")
  }))

  validation {
    condition     = alltrue([for k, r in var.requests : r.limit >= 16])
    error_message = "Every request needs a limit of at least 16 characters; form 3 reserves 9 for the digest and separator alone."
  }

  validation {
    condition     = alltrue([for k, r in var.requests : contains(["-", "_"], r.separator)])
    error_message = "separator must be \"-\" or \"_\"."
  }

  validation {
    condition     = alltrue([for k, r in var.requests : length(r.parts) > 0])
    error_message = "parts must not be empty; it is the identity that survives truncation."
  }
}
