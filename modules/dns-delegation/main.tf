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

# Republish this zone's own apex NS records at the same TTL as the delegation
# above. See modules/domain/main.tf for the full reasoning; in short, Route53
# creates them at 172800 and a resolver caches the child's copy rather than the
# parent's referral, so the 300 above governs nothing on its own. Recreate the
# zone and every resolver that looked it up keeps querying a nameserver set that
# no longer hosts it — answering REFUSED, for up to two days.
#
# Values stay exactly as Route53 assigned them; only the TTL changes.
resource "aws_route53_record" "subdomain_apex_ns" {
  allow_overwrite = true

  zone_id = aws_route53_zone.subdomain.zone_id
  name    = var.subdomain
  type    = "NS"
  ttl     = 300

  records = aws_route53_zone.subdomain.name_servers
}