output "zone_id" {
  description = "The ID of the subdomain Route53 hosted zone"
  value       = aws_route53_zone.subdomain.zone_id
}

output "zone_arn" {
  description = "The ARN of the subdomain Route53 hosted zone"
  value       = aws_route53_zone.subdomain.arn
}

output "name_servers" {
  description = "The nameservers for the subdomain Route53 hosted zone"
  value       = aws_route53_zone.subdomain.name_servers
}

output "subdomain" {
  description = "The subdomain name"
  value       = var.subdomain
}