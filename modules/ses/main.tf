resource "aws_ses_domain_identity" "domain" {
  domain = "${var.env}.${var.domain}"
}

resource "aws_ses_domain_dkim" "dkim" {
  domain = aws_ses_domain_identity.domain.domain
}


resource "aws_route53_record" "domain_amazonses_verification_record" {
  zone_id = var.zone_id
  name    = "_amazonses.${var.env}.${var.domain}"
  type    = "TXT"
  ttl     = "600"
  records = [aws_ses_domain_identity.domain.verification_token]
}

resource "aws_route53_record" "domain_amazonses_dkim_record" {
  count   = 3
  zone_id = var.zone_id
  name    = "${element(aws_ses_domain_dkim.dkim.dkim_tokens, count.index)}._domainkey.${var.domain}"
  type    = "CNAME"
  ttl     = "3600"
  records = ["${element(aws_ses_domain_dkim.dkim.dkim_tokens, count.index)}.dkim.amazonses.com"]
}

resource "aws_ses_email_identity" "emails" {
  count = length(var.test_emails)
  email = element(var.test_emails, count.index)
}

