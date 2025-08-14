output "zone_id" {
  description = "The ID of the Route53 hosted zone"
  value       = aws_route53_zone.root.zone_id
}

output "zone_arn" {
  description = "The ARN of the Route53 hosted zone"
  value       = aws_route53_zone.root.arn
}

output "name_servers" {
  description = "The nameservers for the Route53 hosted zone"
  value       = aws_route53_zone.root.name_servers
}

output "delegation_role_arn" {
  description = "The ARN of the IAM role for cross-account delegation"
  value       = aws_iam_role.dns_delegation.arn
}

output "delegation_role_name" {
  description = "The name of the IAM role for cross-account delegation"
  value       = aws_iam_role.dns_delegation.name
}

output "domain_name" {
  description = "The domain name"
  value       = var.domain_name
}