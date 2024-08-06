locals {
  domain_name = "${var.env == "prod" ? "email." : format("email.%s.", var.env)}${var.domain}"
  zone_id  = var.env == "prod" ? aws_route53_zone.domain.zone_id : var.zone_id
}

resource "aws_route53_zone" "domain" {
  name = local.domain_name
}


resource "aws_acm_certificate" "email_domain" {
  domain_name       = "${local.domain_name}"
  validation_method = "DNS"
  lifecycle {
    create_before_destroy = true
  }
}


