# ============================================================================
# CloudFront Module Outputs
# ============================================================================

output "distribution_id" {
  description = "CloudFront distribution ID"
  value       = aws_cloudfront_distribution.main.id
}

output "distribution_arn" {
  description = "CloudFront distribution ARN"
  value       = aws_cloudfront_distribution.main.arn
}

output "distribution_domain_name" {
  description = "CloudFront distribution domain name (e.g., d1234567890.cloudfront.net)"
  value       = aws_cloudfront_distribution.main.domain_name
}

output "distribution_hosted_zone_id" {
  description = "CloudFront hosted zone ID for Route53 alias records"
  value       = aws_cloudfront_distribution.main.hosted_zone_id
}

output "distribution_status" {
  description = "Current status of the CloudFront distribution"
  value       = aws_cloudfront_distribution.main.status
}

output "domain_aliases" {
  description = "List of domain aliases configured for the distribution"
  value       = var.domain_aliases
}

output "origin_access_control_ids" {
  description = "Map of origin names to their Origin Access Control IDs (for S3 origins)"
  value = {
    for k, v in aws_cloudfront_origin_access_control.s3 : k => v.id
  }
}

output "etag" {
  description = "Current version of the distribution's information"
  value       = aws_cloudfront_distribution.main.etag
}
