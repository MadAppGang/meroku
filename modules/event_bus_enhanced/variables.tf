variable "project" {
  description = "Project name; the bus is <project>-<env>."
  type        = string
}

variable "env" {
  description = "Environment name."
  type        = string
}

variable "enabled" {
  description = "Create the bus. false destroys what state holds and creates nothing: the bus, its shares and the guard, whose destroy first revokes every other account's subscribers and event sources."
  type        = bool
  default     = true
}

variable "share" {
  description = <<-EOT
    Who else may use the bus, through AWS RAM. kind is organization (id ""), ou
    (an organizational unit id) or account (a 12-digit id); access is publish,
    subscribe or full. One RAM share per distinct access value.
  EOT
  type = list(object({
    kind   = string
    id     = string
    access = string
  }))
  default = []

  validation {
    condition     = alltrue([for s in var.share : contains(["organization", "ou", "account"], s.kind)])
    error_message = "Each share entry's kind is organization, ou or account."
  }
  validation {
    condition     = alltrue([for s in var.share : contains(["publish", "subscribe", "full"], s.access)])
    error_message = "Each share entry's access is publish, subscribe or full."
  }
  validation {
    condition     = alltrue([for s in var.share : s.kind != "account" || can(regex("^[0-9]{12}$", s.id))])
    error_message = "An account share entry's id is a 12-digit AWS account id."
  }
  validation {
    condition     = alltrue([for s in var.share : s.kind != "ou" || can(regex("^ou-[a-z0-9]{4,32}-[a-z0-9]{8,32}$", s.id))])
    error_message = "An ou share entry's id is an organizational unit id, such as ou-ab12-cd34ef56."
  }
  validation {
    condition     = alltrue([for s in var.share : s.kind != "organization" || s.id == ""])
    error_message = "An organization share entry has an empty id."
  }
  validation {
    condition     = length(distinct([for s in var.share : "${s.kind}/${s.id}"])) == length(var.share)
    error_message = "Each principal appears in share once; access full covers publish and subscribe."
  }
  validation {
    condition     = var.enabled || length(var.share) == 0
    error_message = "share must be empty when the bus is not enabled."
  }
}

variable "revoke" {
  description = <<-EOT
    The change step: revoke other accounts' subscribers when who may subscribe
    changes. granted is every principal that may subscribe now (account:<id>,
    ou:<id>, organization); keep_accounts, keep_org and keep_ous split it by
    kind; lost is what may no longer subscribe and has not been revoked yet.
    round numbers the revocation: a new round runs the sweep again even when
    granted and lost are unchanged. judged is the ARNs of other accounts'
    subscribers, listed at generate, that no account in keep_accounts owns:
    the ones this round's sweep decides on. The record keeps it once the
    sweep succeeded, so a subscriber still listed and in the record's judged
    is one a finished sweep kept.
  EOT
  type = object({
    enabled       = bool
    granted       = optional(list(string), [])
    keep_accounts = optional(list(string), [])
    keep_org      = optional(bool, false)
    keep_ous      = optional(list(string), [])
    lost          = optional(list(string), [])
    round         = optional(number, 0)
    judged        = optional(list(string), [])
  })
  default = { enabled = false }

  validation {
    condition     = alltrue([for a in var.revoke.keep_accounts : can(regex("^[0-9]{12}$", a))])
    error_message = "revoke.keep_accounts holds 12-digit AWS account ids."
  }
  validation {
    condition     = alltrue([for o in var.revoke.keep_ous : can(regex("^ou-[a-z0-9]{4,32}-[a-z0-9]{8,32}$", o))])
    error_message = "revoke.keep_ous holds organizational unit ids."
  }
  validation {
    condition = alltrue([for p in concat(var.revoke.granted, var.revoke.lost) :
      p == "organization" || can(regex("^account:[0-9]{12}$", p)) || can(regex("^ou:ou-[a-z0-9]{4,32}-[a-z0-9]{8,32}$", p))
    ])
    error_message = "revoke.granted and revoke.lost hold organization, account:<id> or ou:<id>."
  }
  validation {
    condition     = alltrue([for a in var.revoke.judged : can(regex("^arn:aws[a-z-]*:events:[a-z0-9-]+:[0-9]{12}:", a))])
    error_message = "revoke.judged holds EventBridge subscriber ARNs."
  }
}

variable "guard" {
  description = "Keep the teardown guard. Its destroy (leaving the enhanced bus, or terraform destroy) revokes every other account's subscribers and event sources before the bus is deleted. Set once the bus has ever been shared."
  type        = bool
  default     = false
}
