output "zone_id" {
  value = local.zone_id
}

output "subdomains_certificate_arn" {
  value = aws_acm_certificate.subdomains.arn
}

output "api_certificate_arn" {
  value = aws_acm_certificate.api_domain.arn
}

output "api_domain_name" {
  value = aws_acm_certificate.api_domain.domain_name
}

