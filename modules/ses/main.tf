# =============================================================================
# AWS SES Email Domain Configuration
# =============================================================================
# This module sets up SES with proper email deliverability:
# - Domain identity verification
# - DKIM signing (3 CNAME records)
# - SPF authorization (TXT record)
# - Custom MAIL FROM domain (MX + TXT records)
# - DMARC policy (configurable)
#
# Schema v17: Multi-domain support with optional zone_id per domain
# =============================================================================

data "aws_region" "current" {}

# =============================================================================
# Domain Consolidation (Legacy + New Format)
# =============================================================================

locals {
  # Merge legacy single domain with new domains list for backward compatibility
  legacy_domain = var.domain != "" ? [{
    domain              = var.domain
    zone_id             = var.zone_id != "" ? var.zone_id : null
    enable_mail_from    = var.enable_mail_from ? true : null # Convert to nullable
    mail_from_subdomain = var.mail_from_subdomain != "bounce" ? var.mail_from_subdomain : null
    dmarc_policy        = var.dmarc_policy != "none" ? var.dmarc_policy : null
    dmarc_rua_email     = var.dmarc_rua_email != "" ? var.dmarc_rua_email : null
  }] : []

  all_domains = concat(local.legacy_domain, var.domains)

  # Create map of domains for easy lookup (key = domain name)
  domains_map = { for d in local.all_domains : d.domain => d }
}

# =============================================================================
# SES Domain Identity
# =============================================================================

resource "aws_ses_domain_identity" "domain" {
  for_each = local.domains_map

  domain = each.value.domain
}

# Domain verification TXT record (only if zone_id provided)
resource "aws_route53_record" "domain_amazonses_verification_record" {
  for_each = { for k, v in local.domains_map : k => v if v.zone_id != null }

  zone_id         = each.value.zone_id
  name            = "_amazonses.${each.value.domain}"
  type            = "TXT"
  ttl             = "600"
  records         = [aws_ses_domain_identity.domain[each.key].verification_token]
  allow_overwrite = true
}

# =============================================================================
# DKIM Configuration (Email Signing)
# =============================================================================

resource "aws_ses_domain_dkim" "dkim" {
  for_each = local.domains_map

  domain = aws_ses_domain_identity.domain[each.key].domain
}

# DKIM CNAME records (only if zone_id provided)
# Creates 3 records per domain using flattened map
resource "aws_route53_record" "domain_amazonses_dkim_record" {
  for_each = merge([
    for domain_key, domain in local.domains_map : {
      for idx in range(3) : "${domain_key}-dkim-${idx}" => {
        domain_key = domain_key
        zone_id    = domain.zone_id
        domain     = domain.domain
        dkim_token = aws_ses_domain_dkim.dkim[domain_key].dkim_tokens[idx]
        idx        = idx
      } if domain.zone_id != null
    }
  ]...)

  zone_id = each.value.zone_id
  name    = "${each.value.dkim_token}._domainkey.${each.value.domain}"
  type    = "CNAME"
  ttl     = "3600"
  records = ["${each.value.dkim_token}.dkim.amazonses.com"]
}

# =============================================================================
# SPF Record (Sender Authorization)
# =============================================================================

# SPF tells receiving mail servers that Amazon SES is authorized to send
# email on behalf of your domain (only if zone_id provided)
# Note: allow_overwrite handles cases where TXT record already exists
resource "aws_route53_record" "spf" {
  for_each = { for k, v in local.domains_map : k => v if v.zone_id != null }

  zone_id         = each.value.zone_id
  name            = each.value.domain
  type            = "TXT"
  ttl             = "600"
  records         = ["v=spf1 include:amazonses.com ~all"]
  allow_overwrite = true
}

# =============================================================================
# Custom MAIL FROM Domain (Envelope Sender)
# =============================================================================

# By default, SES uses amazonses.com as the MAIL FROM domain.
# A custom MAIL FROM domain improves deliverability by:
# 1. Ensuring SPF alignment for DMARC
# 2. Making bounce addresses use your domain
# 3. Looking more professional in email headers

locals {
  # Calculate per-domain mail_from settings with proper fallbacks
  domains_with_mail_from = {
    for k, v in local.domains_map : k => {
      domain              = v.domain
      zone_id             = v.zone_id
      enable_mail_from    = coalesce(v.enable_mail_from, var.global_enable_mail_from)
      mail_from_subdomain = coalesce(v.mail_from_subdomain, var.global_mail_from_subdomain)
      mail_from_domain    = "${coalesce(v.mail_from_subdomain, var.global_mail_from_subdomain)}.${v.domain}"
    }
  }

  # Filter to only domains with mail_from enabled
  enabled_mail_from = { for k, v in local.domains_with_mail_from : k => v if v.enable_mail_from }
}

resource "aws_ses_domain_mail_from" "mail_from" {
  for_each = local.enabled_mail_from

  domain           = aws_ses_domain_identity.domain[each.key].domain
  mail_from_domain = each.value.mail_from_domain

  # If MX record lookup fails, use default SES behavior
  behavior_on_mx_failure = "UseDefaultValue"
}

# MX record for custom MAIL FROM domain (only if zone_id provided)
# Points to the regional SES SMTP endpoint
resource "aws_route53_record" "mail_from_mx" {
  for_each = { for k, v in local.enabled_mail_from : k => v if v.zone_id != null }

  zone_id = each.value.zone_id
  name    = each.value.mail_from_domain
  type    = "MX"
  ttl     = "600"
  records = ["10 feedback-smtp.${data.aws_region.current.name}.amazonses.com"]
}

# SPF record for custom MAIL FROM domain (only if zone_id provided)
resource "aws_route53_record" "mail_from_spf" {
  for_each = { for k, v in local.enabled_mail_from : k => v if v.zone_id != null }

  zone_id         = each.value.zone_id
  name            = each.value.mail_from_domain
  type            = "TXT"
  ttl             = "600"
  records         = ["v=spf1 include:amazonses.com ~all"]
  allow_overwrite = true
}

# =============================================================================
# DMARC Record (Policy for Failed Authentication)
# =============================================================================

# DMARC tells receiving servers what to do when emails fail SPF/DKIM checks:
# - none: Just report, don't take action (use during initial setup)
# - quarantine: Send to spam folder
# - reject: Block the email entirely

locals {
  domains_with_dmarc = {
    for k, v in local.domains_map : k => {
      domain       = v.domain
      zone_id      = v.zone_id
      dmarc_policy = coalesce(v.dmarc_policy, var.global_dmarc_policy)
      dmarc_rua_email = v.dmarc_rua_email != null ? v.dmarc_rua_email : (
        var.global_dmarc_rua_email != "" ? var.global_dmarc_rua_email : "dmarc-reports@${v.domain}"
      )
    }
  }
}

resource "aws_route53_record" "dmarc" {
  for_each = { for k, v in local.domains_with_dmarc : k => v if v.zone_id != null }

  zone_id         = each.value.zone_id
  name            = "_dmarc.${each.value.domain}"
  type            = "TXT"
  ttl             = "300"
  records         = ["v=DMARC1; p=${each.value.dmarc_policy}; pct=100; rua=mailto:${each.value.dmarc_rua_email}"]
  allow_overwrite = true
}

# =============================================================================
# Test Email Identities (for SES Sandbox mode)
# Account-wide - not per-domain (Schema v18+)
# =============================================================================

resource "aws_ses_email_identity" "emails" {
  count = length(var.test_emails)
  email = var.test_emails[count.index]
}
