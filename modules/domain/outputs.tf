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

output "enable_custom_domain" {
  description = "Flag indicating custom domain is enabled (always true when domain module exists)"
  value       = true
}

output "domain_name" {
  description = "The zone/hostname this module actually created — env-prefixed only when add_env_domain_prefix is true"
  value       = local.domain_name
}

