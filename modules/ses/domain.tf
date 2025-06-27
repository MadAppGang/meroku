resource "aws_acm_certificate" "email_domain" {
  domain_name       = "${var.domain}"
  validation_method = "DNS"
  lifecycle {
    create_before_destroy = true
  }

  tags = {
    Name        = "email-cert-${var.env}"
    Environment = var.env
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

