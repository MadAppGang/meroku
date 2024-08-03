locals {
  domain_name = "${var.env == "prod" ? "api." : format("%s.", var.env)}${var.domain}"
}

resource "aws_route53_zone" "domain" {
  name = local.domain_name
}


resource "aws_acm_certificate" "domain" {
  domain_name       = "*.${local.domain_name}"
  validation_method = "DNS"
  lifecycle {
    create_before_destroy = true
  }
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
  zone_id         = aws_route53_zone.domain.zone_id
}

resource "aws_acm_certificate_validation" "domain" {
  certificate_arn         = aws_acm_certificate.domain.arn
  validation_record_fqdns = [for record in aws_route53_record.domain : record.fqdn]
}

