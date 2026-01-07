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
    - test_emails: Domain-specific test email addresses (optional)
    - enable_mail_from: Enable custom MAIL FROM domain (optional, defaults to global)
    - mail_from_subdomain: Subdomain for MAIL FROM (optional, defaults to global)
    - dmarc_policy: DMARC policy for this domain (optional, defaults to global)
    - dmarc_rua_email: DMARC report email (optional, defaults to global)
  EOT
  type = list(object({
    domain              = string
    zone_id             = optional(string)
    test_emails         = optional(list(string), [])
    enable_mail_from    = optional(bool)
    mail_from_subdomain = optional(string)
    dmarc_policy        = optional(string)
    dmarc_rua_email     = optional(string)
  }))
  default = []
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
  description = "[DEPRECATED] Use test_emails in domains list. Legacy test email addresses"
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
