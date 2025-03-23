resource "aws_route53_zone" "domain" {
  count = var.create_domain_zone ? 1 : 0
  name  = local.domain_name
}

data "aws_route53_zone" "domain" {
  count = var.create_domain_zone ? 0 : 1
  name  = local.domain_name
}

locals {
  zone_id = var.create_domain_zone ? aws_route53_zone.domain[0].zone_id : data.aws_route53_zone.domain[0].zone_id
  domain_name = var.add_env_domain_prefix ? "${var.env}.${var.domain_zone}" : var.domain_zone
}

resource "aws_acm_certificate" "subdomains" {
  domain_name       = "*.${local.domain_name}"
  validation_method = "DNS"
  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_acm_certificate" "api_domain" {
  domain_name       = var.api_domain_prefix == "" ? local.domain_name : "${var.api_domain_prefix}.${local.domain_name}"
  validation_method = "DNS"
  lifecycle {
    create_before_destroy = true
  }
}


resource "aws_route53_record" "api_domain" {
  for_each = {
    for dvo in aws_acm_certificate.api_domain.domain_validation_options : dvo.domain_name => {
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
  zone_id         = local.zone_id
}

resource "aws_acm_certificate_validation" "api_domain" {
  certificate_arn         = aws_acm_certificate.api_domain.arn
  validation_record_fqdns = [for record in aws_route53_record.api_domain : record.fqdn]
}


resource "aws_route53_record" "subdomains" {
  for_each = {
    for dvo in aws_acm_certificate.subdomains.domain_validation_options : dvo.domain_name => {
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
  zone_id         = local.zone_id
}

resource "aws_acm_certificate_validation" "subdomains" {
  certificate_arn         = aws_acm_certificate.subdomains.arn
  validation_record_fqdns = [for record in aws_route53_record.subdomains : record.fqdn]
}

