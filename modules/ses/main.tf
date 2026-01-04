# =============================================================================
# AWS SES Email Domain Configuration
# =============================================================================
# This module sets up SES with proper email deliverability:
# - Domain identity verification
# - DKIM signing (3 CNAME records)
# - SPF authorization (TXT record)
# - Custom MAIL FROM domain (MX + TXT records)
# - DMARC policy (configurable)
# =============================================================================

data "aws_region" "current" {}

# -----------------------------------------------------------------------------
# SES Domain Identity
# -----------------------------------------------------------------------------
resource "aws_ses_domain_identity" "domain" {
  domain = var.domain
}

# Domain verification TXT record
resource "aws_route53_record" "domain_amazonses_verification_record" {
  zone_id = var.zone_id
  name    = "_amazonses.${var.domain}"
  type    = "TXT"
  ttl     = "600"
  records = [aws_ses_domain_identity.domain.verification_token]
}

# -----------------------------------------------------------------------------
# DKIM Configuration (Email Signing)
# -----------------------------------------------------------------------------
resource "aws_ses_domain_dkim" "dkim" {
  domain = aws_ses_domain_identity.domain.domain
}

# DKIM CNAME records (3 required)
resource "aws_route53_record" "domain_amazonses_dkim_record" {
  count   = 3
  zone_id = var.zone_id
  name    = "${element(aws_ses_domain_dkim.dkim.dkim_tokens, count.index)}._domainkey.${var.domain}"
  type    = "CNAME"
  ttl     = "3600"
  records = ["${element(aws_ses_domain_dkim.dkim.dkim_tokens, count.index)}.dkim.amazonses.com"]
}

# -----------------------------------------------------------------------------
# SPF Record (Sender Authorization)
# -----------------------------------------------------------------------------
# SPF tells receiving mail servers that Amazon SES is authorized to send
# email on behalf of your domain
resource "aws_route53_record" "spf" {
  zone_id = var.zone_id
  name    = var.domain
  type    = "TXT"
  ttl     = "600"
  records = ["v=spf1 include:amazonses.com ~all"]
}

# -----------------------------------------------------------------------------
# Custom MAIL FROM Domain (Envelope Sender)
# -----------------------------------------------------------------------------
# By default, SES uses amazonses.com as the MAIL FROM domain.
# A custom MAIL FROM domain improves deliverability by:
# 1. Ensuring SPF alignment for DMARC
# 2. Making bounce addresses use your domain
# 3. Looking more professional in email headers

locals {
  mail_from_domain = "${var.mail_from_subdomain}.${var.domain}"
}

resource "aws_ses_domain_mail_from" "mail_from" {
  count = var.enable_mail_from ? 1 : 0

  domain           = aws_ses_domain_identity.domain.domain
  mail_from_domain = local.mail_from_domain

  # If MX record lookup fails, use default SES behavior
  behavior_on_mx_failure = "UseDefaultValue"
}

# MX record for custom MAIL FROM domain
# Points to the regional SES SMTP endpoint
resource "aws_route53_record" "mail_from_mx" {
  count = var.enable_mail_from ? 1 : 0

  zone_id = var.zone_id
  name    = local.mail_from_domain
  type    = "MX"
  ttl     = "600"
  records = ["10 feedback-smtp.${data.aws_region.current.name}.amazonses.com"]
}

# SPF record for custom MAIL FROM domain
resource "aws_route53_record" "mail_from_spf" {
  count = var.enable_mail_from ? 1 : 0

  zone_id = var.zone_id
  name    = local.mail_from_domain
  type    = "TXT"
  ttl     = "600"
  records = ["v=spf1 include:amazonses.com ~all"]
}

# -----------------------------------------------------------------------------
# DMARC Record (Policy for Failed Authentication)
# -----------------------------------------------------------------------------
# DMARC tells receiving servers what to do when emails fail SPF/DKIM checks:
# - none: Just report, don't take action (use during initial setup)
# - quarantine: Send to spam folder
# - reject: Block the email entirely

locals {
  dmarc_rua_email = var.dmarc_rua_email != "" ? var.dmarc_rua_email : "dmarc-reports@${var.domain}"
}

resource "aws_route53_record" "dmarc" {
  zone_id = var.zone_id
  name    = "_dmarc.${var.domain}"
  type    = "TXT"
  ttl     = "300"
  records = ["v=DMARC1; p=${var.dmarc_policy}; pct=100; rua=mailto:${local.dmarc_rua_email}"]
}

# -----------------------------------------------------------------------------
# Test Email Identities (for SES Sandbox mode)
# -----------------------------------------------------------------------------
resource "aws_ses_email_identity" "emails" {
  count = length(var.test_emails)
  email = element(var.test_emails, count.index)
}
