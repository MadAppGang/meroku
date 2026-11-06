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
    Apex values longer than 255 characters are split into 255-character strings
    joined by the three-character delimiter `" "`.
    Note: test_emails are at global SES level (account-wide in AWS)
  EOT
  type = list(object({
    domain              = string
    zone_id             = optional(string)
    enable_mail_from    = optional(bool)
    mail_from_subdomain = optional(string)
    dmarc_policy        = optional(string)
    dmarc_rua_email     = optional(string)
    spf_record          = optional(string)
    spf_ttl             = optional(number)
    apex_txt_values     = optional(list(string))
  }))
  default = []

  # Conditions use `for ... if` filters rather than `||`: Terraform 1.9 does not
  # short-circuit `||`, so `x == null || f(x)` still evaluates f(null).

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
      ) : can(regex("^[\\x20-\\x7E]*$", x)) if x != null
    ])
    error_message = "spf_record and apex_txt_values may hold only printable ASCII characters (0x20-0x7E). Write any other byte as a DNS octal escape such as \\344."
  }

  validation {
    condition = alltrue([
      for x in concat(
        [for d in var.domains : d.spf_record if d.spf_record != null],
        flatten([for d in var.domains : d.apex_txt_values if d.apex_txt_values != null]),
      ) : !can(regex("\"", replace(x, "\" \"", ""))) && !can(regex("^\"|\"$", x)) if x != null
    ])
    error_message = "spf_record and apex_txt_values may contain a double quote only as part of the string delimiter \" \" between two parts of a value, never at its start or end."
  }

  validation {
    condition = alltrue(flatten([
      for x in concat(
        [for d in var.domains : d.spf_record if d.spf_record != null],
        flatten([for d in var.domains : d.apex_txt_values if d.apex_txt_values != null]),
        ) : [
        for segment in split("\" \"", x) : [
          for i in range(max(0, ceil(length(segment) / 255) - 1)) :
          !can(regex("\\\\[0-9]{0,2}$", substr(segment, i * 255, 255)))
        ]
      ] if x != null
    ]))
    error_message = "An apex TXT value over 255 characters would be split inside a backslash escape. Split it yourself with the delimiter \" \" at a point outside any escape."
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
