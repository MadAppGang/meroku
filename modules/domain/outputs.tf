output "zone_id" {
  value = local.zone_id
}

output "subdomains_certificate_arn" {
  value = aws_acm_certificate_validation.subdomains.certificate_arn
}

output "api_certificate_arn" {
  value = aws_acm_certificate_validation.api_domain.certificate_arn
}

output "api_domain_name" {
  value = aws_acm_certificate.api_domain.domain_name
}

