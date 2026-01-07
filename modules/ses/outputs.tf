# =============================================================================
# SES Module Outputs (Schema v17: Multi-Domain Support)
# =============================================================================

# =============================================================================
# Multi-Domain Outputs
# =============================================================================

output "domain_identities" {
  description = "Map of domain names to their SES identity ARNs"
  value = {
    for k, v in aws_ses_domain_identity.domain : k => v.arn
  }
}

output "domains" {
  description = "List of configured email domains"
  value       = keys(local.domains_map)
}

output "dkim_tokens" {
  description = "Map of domains to their DKIM tokens"
  value = {
    for k, v in aws_ses_domain_dkim.dkim : k => v.dkim_tokens
  }
}

output "verification_tokens" {
  description = "Map of domains to their verification tokens"
  value = {
    for k, v in aws_ses_domain_identity.domain : k => v.verification_token
  }
}

# =============================================================================
# Manual DNS Instructions (for domains without zone_id)
# =============================================================================

output "manual_dns_instructions" {
  description = "DNS records to create manually for domains without zone_id"
  value = {
    for domain_key, domain in local.domains_map : domain_key => {
      domain = domain.domain
      records = domain.zone_id == null ? concat(
        [
          {
            name  = "_amazonses.${domain.domain}"
            type  = "TXT"
            value = aws_ses_domain_identity.domain[domain_key].verification_token
            note  = "Domain verification record"
          },
          {
            name  = domain.domain
            type  = "TXT"
            value = "v=spf1 include:amazonses.com ~all"
            note  = "SPF authorization record"
          },
          {
            name  = "${aws_ses_domain_dkim.dkim[domain_key].dkim_tokens[0]}._domainkey.${domain.domain}"
            type  = "CNAME"
            value = "${aws_ses_domain_dkim.dkim[domain_key].dkim_tokens[0]}.dkim.amazonses.com"
            note  = "DKIM record 1/3"
          },
          {
            name  = "${aws_ses_domain_dkim.dkim[domain_key].dkim_tokens[1]}._domainkey.${domain.domain}"
            type  = "CNAME"
            value = "${aws_ses_domain_dkim.dkim[domain_key].dkim_tokens[1]}.dkim.amazonses.com"
            note  = "DKIM record 2/3"
          },
          {
            name  = "${aws_ses_domain_dkim.dkim[domain_key].dkim_tokens[2]}._domainkey.${domain.domain}"
            type  = "CNAME"
            value = "${aws_ses_domain_dkim.dkim[domain_key].dkim_tokens[2]}.dkim.amazonses.com"
            note  = "DKIM record 3/3"
          },
          {
            name  = "_dmarc.${domain.domain}"
            type  = "TXT"
            value = "v=DMARC1; p=${local.domains_with_dmarc[domain_key].dmarc_policy}; pct=100; rua=mailto:${local.domains_with_dmarc[domain_key].dmarc_rua_email}"
            note  = "DMARC policy record"
          }
        ],
        # Add MAIL FROM records if enabled for this domain
        lookup(local.enabled_mail_from, domain_key, null) != null ? [
          {
            name  = local.enabled_mail_from[domain_key].mail_from_domain
            type  = "MX"
            value = "10 feedback-smtp.${data.aws_region.current.name}.amazonses.com"
            note  = "Custom MAIL FROM MX record"
          },
          {
            name  = local.enabled_mail_from[domain_key].mail_from_domain
            type  = "TXT"
            value = "v=spf1 include:amazonses.com ~all"
            note  = "Custom MAIL FROM SPF record"
          }
        ] : []
      ) : []
    } if domain.zone_id == null
  }
}

# =============================================================================
# Deliverability Summary
# =============================================================================

output "deliverability_summary" {
  description = "Summary of email deliverability configuration for all domains"
  value = {
    for k, v in local.domains_map : k => {
      domain           = v.domain
      zone_id_provided = v.zone_id != null
      auto_dns         = v.zone_id != null ? "Enabled" : "Manual setup required"
      dkim_enabled     = true
      spf_enabled      = true
      mail_from_domain = lookup(local.enabled_mail_from, k, null) != null ? local.enabled_mail_from[k].mail_from_domain : "amazonses.com (default)"
      dmarc_policy     = local.domains_with_dmarc[k].dmarc_policy
      dmarc_rua_email  = local.domains_with_dmarc[k].dmarc_rua_email
    }
  }
}

# =============================================================================
# Legacy Outputs (for backward compatibility)
# =============================================================================

output "domain_identity_arn" {
  description = "[DEPRECATED] ARN of the first SES domain identity (for backward compatibility)"
  value       = length(aws_ses_domain_identity.domain) > 0 ? values(aws_ses_domain_identity.domain)[0].arn : null
}

output "domain" {
  description = "[DEPRECATED] The first email domain configured in SES (for backward compatibility)"
  value       = length(local.domains_map) > 0 ? keys(local.domains_map)[0] : null
}

output "mail_from_domain" {
  description = "[DEPRECATED] The custom MAIL FROM domain for the first domain (for backward compatibility)"
  value       = length(local.enabled_mail_from) > 0 ? values(local.enabled_mail_from)[0].mail_from_domain : null
}

# Legacy single output (kept for compatibility)
output "dkim_tokens_legacy" {
  description = "[DEPRECATED] DKIM tokens for the first domain (for backward compatibility)"
  value       = length(aws_ses_domain_dkim.dkim) > 0 ? values(aws_ses_domain_dkim.dkim)[0].dkim_tokens : []
}

output "verification_token_legacy" {
  description = "[DEPRECATED] Domain verification token for the first domain (for backward compatibility)"
  value       = length(aws_ses_domain_identity.domain) > 0 ? values(aws_ses_domain_identity.domain)[0].verification_token : null
}

# DNS Records Created (updated for multi-domain)
output "dns_records_created" {
  description = "Summary of DNS records created by this module (only for domains with zone_id)"
  value = {
    for domain_key, domain in local.domains_map : domain_key => domain.zone_id != null ? concat(
      [
        {
          name = "_amazonses.${domain.domain}"
          type = "TXT"
          note = "Domain verification"
        },
        {
          name = domain.domain
          type = "TXT"
          note = "SPF record"
        },
        {
          name = "_dmarc.${domain.domain}"
          type = "TXT"
          note = "DMARC policy: ${local.domains_with_dmarc[domain_key].dmarc_policy}"
        }
      ],
      [
        for i in range(3) : {
          name = "${aws_ses_domain_dkim.dkim[domain_key].dkim_tokens[i]}._domainkey.${domain.domain}"
          type = "CNAME"
          note = "DKIM record ${i + 1}/3"
        }
      ],
      lookup(local.enabled_mail_from, domain_key, null) != null ? [
        {
          name = local.enabled_mail_from[domain_key].mail_from_domain
          type = "MX"
          note = "Custom MAIL FROM"
        },
        {
          name = local.enabled_mail_from[domain_key].mail_from_domain
          type = "TXT"
          note = "MAIL FROM SPF"
        }
      ] : []
    ) : [] if domain.zone_id != null
  }
}
