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

# The env-resolved domain: "<env>.<zone>" or "<zone>", depending on add_env_domain_prefix.
# Consumers must use this instead of re-deriving the prefix themselves.
output "domain_name" {
  description = "Env-resolved domain name the zone and wildcard certificate are issued for"
  value       = local.domain_name
}

output "enable_custom_domain" {
  description = "Flag indicating custom domain is enabled (always true when domain module exists)"
  value       = true
}

