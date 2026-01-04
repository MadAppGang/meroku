variable "domain" {
  description = "The email domain to verify with SES (e.g., mail.example.com)"
  type        = string
}

variable "env" {
  description = "Environment name"
  type        = string
}

variable "project" {
  description = "Project name"
  type        = string
}

variable "test_emails" {
  description = "List of test email addresses to verify in SES sandbox mode"
  type        = list(string)
  default     = []
}

variable "zone_id" {
  description = "Route53 hosted zone ID for DNS record creation"
  type        = string
}

# Email Deliverability Configuration
variable "enable_mail_from" {
  description = "Enable custom MAIL FROM domain for better email deliverability"
  type        = bool
  default     = true
}

variable "mail_from_subdomain" {
  description = "Subdomain prefix for custom MAIL FROM (e.g., 'bounce' creates bounce.mail.example.com)"
  type        = string
  default     = "bounce"
}

variable "dmarc_policy" {
  description = "DMARC policy: 'none' (monitor only), 'quarantine' (send to spam), 'reject' (block). Start with 'none' until DKIM/SPF are verified."
  type        = string
  default     = "none"

  validation {
    condition     = contains(["none", "quarantine", "reject"], var.dmarc_policy)
    error_message = "dmarc_policy must be one of: none, quarantine, reject"
  }
}

variable "dmarc_rua_email" {
  description = "Email address to receive DMARC aggregate reports. Defaults to dmarc-reports@{domain}"
  type        = string
  default     = ""
}
