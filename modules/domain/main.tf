data "aws_route53_zone" "domain" {
  domain       = "*.${var.env == "prod" ? "" : format("%s.", var.env)}${var.domain}"
  private_zone = false
}


resource "aws_acm_certificate" "domain" {
  domain            = "*.${var.env == "prod" ? "" : format("%s.", var.env)}${var.domain}"
  validation_method = "DNS"
}



resource "aws_route53_record" "domain" {
  for_each = {
    for dvo in aws_acm_certificate.domain.domain_validation_options : dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  }

  allow_overwrite = true
  name            = each.value.name
  records         = [each.value.record]
  ttl             = 60
  type            = each.value.type
  zone_id         = data.aws_route53_zone.domain.zone_id
}

resource "aws_acm_certificate_validation" "domain" {
  certificate_arn         = aws_acm_certificate.domain.arn
  validation_record_fqdns = [for record in aws_route53_record.domain : record.fqdn]
}

