terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
      configuration_aliases = [aws.root]
    }
  }
}

resource "aws_route53_zone" "subdomain" {
  name    = var.subdomain
  comment = "Delegated zone for ${var.subdomain}"

  tags = merge(
    var.tags,
    {
      Name        = var.subdomain
      Environment = var.environment
      Purpose     = "DNS Delegated Zone"
      RootZone    = var.root_domain
    }
  )
}

resource "aws_route53_record" "delegation_ns" {
  provider = aws.root
  
  zone_id = var.root_zone_id
  name    = var.subdomain
  type    = "NS"
  ttl     = 300

  records = aws_route53_zone.subdomain.name_servers
}