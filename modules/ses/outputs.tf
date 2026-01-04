# =============================================================================
# SES Module Outputs
# =============================================================================

output "domain_identity_arn" {
  description = "ARN of the SES domain identity"
  value       = aws_ses_domain_identity.domain.arn
}

output "domain" {
  description = "The email domain configured in SES"
  value       = var.domain
}

output "mail_from_domain" {
  description = "The custom MAIL FROM domain (if enabled)"
  value       = var.enable_mail_from ? local.mail_from_domain : null
}

output "dkim_tokens" {
  description = "DKIM tokens for the domain"
  value       = aws_ses_domain_dkim.dkim.dkim_tokens
}

output "verification_token" {
  description = "Domain verification token"
  value       = aws_ses_domain_identity.domain.verification_token
}

# Email Deliverability Status Summary
output "deliverability_summary" {
  description = "Summary of email deliverability configuration"
  value = {
    domain           = var.domain
    dkim_enabled     = true
    spf_enabled      = true
    mail_from_domain = var.enable_mail_from ? local.mail_from_domain : "amazonses.com (default)"
    dmarc_policy     = var.dmarc_policy
    dmarc_rua_email  = local.dmarc_rua_email
  }
}

# DNS Records Created (for verification)
output "dns_records_created" {
  description = "List of DNS records created by this module"
  value = concat(
    [
      {
        name  = "_amazonses.${var.domain}"
        type  = "TXT"
        value = "Domain verification"
      },
      {
        name  = var.domain
        type  = "TXT"
        value = "SPF record"
      },
      {
        name  = "_dmarc.${var.domain}"
        type  = "TXT"
        value = "DMARC policy: ${var.dmarc_policy}"
      }
    ],
    [
      for i in range(3) : {
        name  = "${aws_ses_domain_dkim.dkim.dkim_tokens[i]}._domainkey.${var.domain}"
        type  = "CNAME"
        value = "DKIM record ${i + 1}/3"
      }
    ],
    var.enable_mail_from ? [
      {
        name  = local.mail_from_domain
        type  = "MX"
        value = "Custom MAIL FROM"
      },
      {
        name  = local.mail_from_domain
        type  = "TXT"
        value = "MAIL FROM SPF"
      }
    ] : []
  )
}
