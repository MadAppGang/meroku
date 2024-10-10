output "zone_id" {
  value = aws_route53_zone.domain.zone_id
}

output "subdomains_certificate_arn" {
  value = aws_acm_certificate.subdomains.arn
}

output "api_certificate_arn" {
  value = aws_acm_certificate.api_domain.arn
}



