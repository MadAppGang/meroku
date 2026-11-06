variable "env" {
  description = "Environment name"
  type        = string
}

variable "project" {
  description = "Project name"
  type        = string
}

# =============================================================================
# Multi-Domain Configuration (Schema v17+)
# =============================================================================

variable "domains" {
  description = <<-EOT
    List of email domains to configure with SES. Each domain can optionally have:
    - domain: The email domain (required)
    - zone_id: Route53 zone ID for automatic DNS record creation (optional)
    - enable_mail_from: Enable custom MAIL FROM domain (optional, defaults to global)
    - mail_from_subdomain: Subdomain for MAIL FROM (optional, defaults to global)
    - dmarc_policy: DMARC policy for this domain (optional, defaults to global)
    - dmarc_rua_email: DMARC report email (optional, defaults to global)
    - spf_record: Full value of the apex SPF TXT record (optional, defaults to
      "v=spf1 include:amazonses.com ~all")
    - spf_ttl: TTL of the apex TXT record set (optional, defaults to 600)
    - apex_txt_values: Extra TXT values written into the same apex record set
      as the SPF value, with the same TTL (optional)
    - txt_records: Extra TXT record sets at names below the domain, each with
      a name relative to the domain (e.g. "google._domainkey"), its values and
      an optional TTL (default 600) (optional)
    - zone_origin: "created" (default) when meroku created the zone, so the
      listed values are the whole record set; "existing" when it did not, so
      meroku adds its values to the live set and removes only those (optional)
    TXT values longer than 255 characters are split into 255-character strings
    joined by the three-character delimiter `" "`.
    Note: test_emails are at global SES level (account-wide in AWS)
  EOT
  type = list(object({
    domain              = string
    zone_id             = optional(string)
    zone_origin         = optional(string)
    enable_mail_from    = optional(bool)
    mail_from_subdomain = optional(string)
    dmarc_policy        = optional(string)
    dmarc_rua_email     = optional(string)
    spf_record          = optional(string)
    spf_ttl             = optional(number)
    apex_txt_values     = optional(list(string))
    txt_records = optional(list(object({
      name   = string
      values = list(string)
      ttl    = optional(number)
    })))
  }))
  default = []

  # Conditions use `for ... if` filters rather than `||`: Terraform 1.9 does not
  # short-circuit `||`, so `x == null || f(x)` still evaluates f(null).

  validation {
    condition = alltrue([
      for d in var.domains : contains(["created", "existing"], d.zone_origin) if d.zone_origin != null
    ])
    error_message = "zone_origin must be \"created\" or \"existing\"."
  }

  validation {
    condition = alltrue(flatten([
      for d in var.domains : [
        for r in d.txt_records : can(regex("^[a-z0-9_]([a-z0-9_-]{0,61}[a-z0-9_])?(\\.[a-z0-9_]([a-z0-9_-]{0,61}[a-z0-9_])?)*$", r.name))
      ] if d.txt_records != null
    ]))
    error_message = "txt_records[].name must be a DNS name relative to the domain, such as \"google._domainkey\": lower-case letters, digits, hyphens and underscores, with dots between the parts, no dot at the end, not empty and not @."
  }

  validation {
    condition = alltrue(flatten([
      for d in var.domains : [
        for r in d.txt_records : r.name != lower(d.domain) && !endswith(r.name, ".${lower(d.domain)}")
      ] if d.txt_records != null
    ]))
    error_message = "txt_records[].name is relative to the domain: write \"google._domainkey\", not \"google._domainkey.example.com\"."
  }

  validation {
    condition = alltrue([
      for d in var.domains : length(distinct([for r in d.txt_records : r.name])) == length(d.txt_records) if d.txt_records != null
    ])
    error_message = "txt_records names must be unique within a domain: put every value for one name in one entry."
  }

  validation {
    condition = alltrue(flatten([
      for d in var.domains : [for r in d.txt_records : length("${r.name}.${d.domain}") <= 253] if d.txt_records != null
    ]))
    error_message = "txt_records[].name plus the domain must be 253 characters or fewer."
  }

  validation {
    condition = alltrue(flatten([
      for d in var.domains : [for r in d.txt_records : length(r.values) > 0] if d.txt_records != null
    ]))
    error_message = "txt_records[].values must hold at least one value."
  }

  validation {
    condition = alltrue(flatten([
      for d in var.domains : [
        for r in d.txt_records : [
          for x in r.values : try(replace(x, "\" \"", "") != "", false)
        ]
      ] if d.txt_records != null
    ]))
    error_message = "txt_records[].values entries must not be null, empty, or only the delimiter \" \"."
  }

  validation {
    condition = alltrue(flatten([
      for d in var.domains : [
        for r in d.txt_records : [
          for x in r.values : !can(regex("(?i)^v=spf1( |$)", replace(x, "\" \"", ""))) if x != null
        ]
      ] if d.txt_records != null
    ]))
    error_message = "txt_records must not contain an SPF record: meroku writes SPF only at the domain itself, through spf_record."
  }

  validation {
    condition = alltrue(flatten([
      for d in var.domains : [for r in d.txt_records : r.ttl >= 0 && floor(r.ttl) == r.ttl if r.ttl != null] if d.txt_records != null
    ]))
    error_message = "txt_records[].ttl must be a whole number of seconds, 0 or more."
  }

  validation {
    condition = alltrue([
      for d in var.domains : can(regex("^v=spf1( |$)", d.spf_record)) if d.spf_record != null
    ])
    error_message = "spf_record must be an SPF record: it begins with \"v=spf1\" followed by a space or the end of the value."
  }

  validation {
    condition = alltrue([
      for d in var.domains : d.spf_ttl >= 0 && floor(d.spf_ttl) == d.spf_ttl if d.spf_ttl != null
    ])
    error_message = "spf_ttl must be a whole number of seconds, 0 or more."
  }

  validation {
    condition = alltrue([
      for x in flatten([for d in var.domains : d.apex_txt_values if d.apex_txt_values != null]) :
      try(replace(x, "\" \"", "") != "", false)
    ])
    error_message = "apex_txt_values entries must not be null, empty, or only the delimiter \" \"."
  }

  validation {
    condition = alltrue([
      for x in flatten([for d in var.domains : d.apex_txt_values if d.apex_txt_values != null]) :
      !can(regex("(?i)^v=spf1( |$)", replace(x, "\" \"", ""))) if x != null
    ])
    error_message = "apex_txt_values must not contain an SPF record: set spf_record instead. Two SPF records at one name make SPF evaluate to PermError."
  }

  validation {
    condition = alltrue([
      for x in concat(
        [for d in var.domains : d.spf_record if d.spf_record != null],
        flatten([for d in var.domains : d.apex_txt_values if d.apex_txt_values != null]),
        flatten([for d in var.domains : [for r in d.txt_records : r.values] if d.txt_records != null]),
      ) : can(regex("^[\\x20-\\x7E]*$", x)) if x != null
    ])
    error_message = "spf_record, apex_txt_values and txt_records values may hold only printable ASCII characters (0x20-0x7E). Write any other byte as a DNS octal escape such as \\344."
  }

  validation {
    condition = alltrue([
      for x in concat(
        [for d in var.domains : d.spf_record if d.spf_record != null],
        flatten([for d in var.domains : d.apex_txt_values if d.apex_txt_values != null]),
        flatten([for d in var.domains : [for r in d.txt_records : r.values] if d.txt_records != null]),
      ) : !can(regex("\"", replace(x, "\" \"", ""))) && !can(regex("^\"|\"$", x)) if x != null
    ])
    error_message = "spf_record, apex_txt_values and txt_records values may contain a double quote only as part of the string delimiter \" \" between two parts of a value, never at its start or end."
  }

  validation {
    condition = alltrue(flatten([
      for x in concat(
        [for d in var.domains : d.spf_record if d.spf_record != null],
        flatten([for d in var.domains : d.apex_txt_values if d.apex_txt_values != null]),
        flatten([for d in var.domains : [for r in d.txt_records : r.values] if d.txt_records != null]),
        ) : [
        for segment in split("\" \"", x) : [
          for i in range(max(0, ceil(length(segment) / 255) - 1)) :
          !can(regex("\\\\[0-9]{0,2}$", substr(segment, i * 255, 255)))
        ]
      ] if x != null
    ]))
    error_message = "A TXT value over 255 characters would be split inside a backslash escape. Split it yourself with the delimiter \" \" at a point outside any escape."
  }
}

# =============================================================================
# Legacy Single Domain Configuration (Deprecated - for backward compatibility)
# =============================================================================

variable "domain" {
  description = "[DEPRECATED] Use domains list instead. Single email domain to verify with SES"
  type        = string
  default     = ""
}

variable "zone_id" {
  description = "[DEPRECATED] Use zone_id in domains list. Route53 hosted zone ID for DNS record creation"
  type        = string
  default     = ""
}

variable "zone_origin" {
  description = "Zone origin of the legacy single domain: \"created\" when meroku created the zone, \"existing\" when it did not (see domains[].zone_origin)."
  type        = string
  default     = "created"

  validation {
    condition     = contains(["created", "existing"], var.zone_origin)
    error_message = "zone_origin must be \"created\" or \"existing\"."
  }
}

variable "test_emails" {
  description = "Test email addresses for SES sandbox mode (account-wide, not per-domain)"
  type        = list(string)
  default     = []
}

# =============================================================================
# Global Email Deliverability Configuration
# =============================================================================

variable "global_enable_mail_from" {
  description = "Global default for enabling custom MAIL FROM domain"
  type        = bool
  default     = true
}

variable "global_mail_from_subdomain" {
  description = "Global default subdomain for custom MAIL FROM"
  type        = string
  default     = "bounce"
}

variable "global_dmarc_policy" {
  description = "Global default DMARC policy"
  type        = string
  default     = "none"

  validation {
    condition     = contains(["none", "quarantine", "reject"], var.global_dmarc_policy)
    error_message = "DMARC policy must be: none, quarantine, or reject"
  }
}

variable "global_dmarc_rua_email" {
  description = "Global default email for DMARC reports (defaults to dmarc-reports@{domain})"
  type        = string
  default     = ""
}

# =============================================================================
# Legacy Email Deliverability Configuration (Deprecated)
# =============================================================================

variable "enable_mail_from" {
  description = "[DEPRECATED] Use global_enable_mail_from instead. Enable custom MAIL FROM domain for better email deliverability"
  type        = bool
  default     = true
}

variable "mail_from_subdomain" {
  description = "[DEPRECATED] Use global_mail_from_subdomain instead. Subdomain prefix for custom MAIL FROM (e.g., 'bounce' creates bounce.mail.example.com)"
  type        = string
  default     = "bounce"
}

variable "dmarc_policy" {
  description = "[DEPRECATED] Use global_dmarc_policy instead. DMARC policy: 'none' (monitor only), 'quarantine' (send to spam), 'reject' (block). Start with 'none' until DKIM/SPF are verified."
  type        = string
  default     = "none"

  validation {
    condition     = contains(["none", "quarantine", "reject"], var.dmarc_policy)
    error_message = "dmarc_policy must be one of: none, quarantine, reject"
  }
}

variable "dmarc_rua_email" {
  description = "[DEPRECATED] Use global_dmarc_rua_email instead. Email address to receive DMARC aggregate reports. Defaults to dmarc-reports@{domain}"
  type        = string
  default     = ""
}
