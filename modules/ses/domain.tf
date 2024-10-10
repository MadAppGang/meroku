resource "aws_acm_certificate" "email_domain" {
  domain_name       = "${var.domain}"
  validation_method = "DNS"
  lifecycle {
    create_before_destroy = true
  }
}

