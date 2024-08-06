resource "aws_ses_domain_identity" "domain" {
  domain = local.domain_name 
}

resource "aws_ses_domain_dkim" "dkim" {
  domain = aws_ses_domain_identity.domain.domain
}


resource "aws_route53_record" "domain_amazonses_verification_record" {
  zone_id = local.zone_id
  name    = "_amazonses.${local.domain_name}"
  type    = "TXT"
  ttl     = "600"
  records = [aws_ses_domain_identity.domain.verification_token]
}

resource "aws_route53_record" "domain_amazonses_dkim_record" {
  count   = 3
  zone_id = local.zone_id
  name    = "${element(aws_ses_domain_dkim.dkim.dkim_tokens, count.index)}._domainkey"
  type    = "CNAME"
  ttl     = "3600"
  records = ["${element(aws_ses_domain_dkim.dkim.dkim_tokens, count.index)}.dkim.amazonses.com"]
}

resource "aws_ses_email_identity" "emails" {
  count = length(var.test_emails)
  email = element(var.test_emails, count.index)
}

resource "aws_route53_record" "dmarc" {
  zone_id = local.zone_id
  name    = "_dmarc.${local.domain_name}"
  type    = "TXT"
  ttl     = "300"
  records = ["v=DMARC1; p=quarantine; pct=100; rua=mailto:dmarc-reports@${local.domain_name}"]
}
