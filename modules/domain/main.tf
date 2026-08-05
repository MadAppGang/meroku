resource "aws_route53_zone" "domain" {
  count         = var.create_domain_zone ? 1 : 0
  name          = local.domain_name
  force_destroy = var.force_destroy

  tags = {
    Name        = "zone-${var.env}"
    Environment = var.env
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

data "aws_route53_zone" "domain" {
  count = var.create_domain_zone ? 0 : 1
  name  = local.domain_name
}

# Republish the zone's own apex NS records at a short TTL.
#
# Route53 creates every hosted zone with these records at a TTL of 172800 — two
# days. A resolver caches the NS set from the child's authoritative answer, not
# from the parent's referral, so that TTL and not the 300s on the delegation
# record in the parent is what governs how long a resolver keeps using a given
# nameserver set.
#
# That matters because Route53 assigns a fresh random nameserver set to each new
# hosted zone. Delete this zone and create it again — which any destroy/apply
# cycle in a dev environment does — and every resolver that looked the name up
# beforehand keeps querying the old set for up to two days. Those servers no
# longer host the zone, so they answer REFUSED and the name goes SERVFAIL rather
# than merely resolving to something stale. Certificate validation asks through
# exactly such a resolver.
#
# Observed on a recreated dev zone: two public resolvers were still querying an
# entirely different nameserver set hours later, while resolvers with a cold
# cache saw the new delegation within seconds.
#
# The values must stay exactly as Route53 assigned them; only the TTL changes,
# which is why this reads name_servers straight back out of the zone. 300 matches
# the delegation record written into the parent, so both halves of the referral
# now expire together.
resource "aws_route53_record" "apex_ns" {
  count = var.create_domain_zone ? 1 : 0

  # The RRset already exists — it was created with the zone — so this is an
  # overwrite rather than an addition.
  allow_overwrite = true

  zone_id = aws_route53_zone.domain[0].zone_id
  name    = local.domain_name
  type    = "NS"
  ttl     = 300
  records = aws_route53_zone.domain[0].name_servers
}

locals {
  zone_id         = var.create_domain_zone ? aws_route53_zone.domain[0].zone_id : data.aws_route53_zone.domain[0].zone_id
  domain_name     = var.add_env_domain_prefix ? "${var.env}.${var.domain_zone}" : var.domain_zone
  api_domain_name = var.api_domain_prefix == "" ? local.domain_name : "${var.api_domain_prefix}.${local.domain_name}"
}

resource "aws_acm_certificate" "subdomains" {
  domain_name               = local.domain_name
  subject_alternative_names = ["*.${local.domain_name}"]
  validation_method         = "DNS"
  lifecycle {
    create_before_destroy = true
  }

  tags = {
    Name        = "wildcard-cert-${var.env}"
    Environment = var.env
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_acm_certificate" "api_domain" {
  domain_name       = local.api_domain_name
  validation_method = "DNS"
  lifecycle {
    create_before_destroy = true
  }

  tags = {
    Name        = "api-cert-${var.env}"
    Environment = var.env
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}


resource "aws_route53_record" "api_domain" {
  for_each = {
    for dvo in aws_acm_certificate.api_domain.domain_validation_options : dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  }

  allow_overwrite = true
  name            = each.value.name
  records         = [each.value.record]
  ttl             = 60
  type            = each.value.type
  zone_id         = local.zone_id
}

resource "aws_acm_certificate_validation" "api_domain" {
  certificate_arn         = aws_acm_certificate.api_domain.arn
  validation_record_fqdns = [for record in aws_route53_record.api_domain : record.fqdn]

  # ACM can only validate once the CNAME below resolves on the public internet,
  # which requires this zone to be delegated from its parent. If delegation is
  # missing, the default 75m timeout parks the entire apply with no explanation
  # (every consumer of the cert ARN waits on this resource). Fail legibly instead.
  timeouts {
    # ACM's own DNS validation is documented as taking up to 30 minutes, and it
    # backs off after a failed check — so a certificate created moments after the
    # delegation lands can sit pending well past the half hour before ACM looks
    # again. 20m was too tight and failed a deploy whose DNS was provably
    # correct: the validation record resolved from the root, with no cache, to
    # exactly the value ACM had asked for.
    #
    # Fast failure is no longer this timeout's job. app/dns_preflight.go catches
    # a genuinely undelegated zone in about two seconds, before the apply starts,
    # so all this bound has to do is stop a stuck apply running for the 75-minute
    # provider default.
    create = "45m"
  }
}


resource "aws_route53_record" "subdomains" {
  for_each = {
    for dvo in aws_acm_certificate.subdomains.domain_validation_options : dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  }

  allow_overwrite = true
  name            = each.value.name
  records         = [each.value.record]
  ttl             = 60
  type            = each.value.type
  zone_id         = local.zone_id
}

resource "aws_acm_certificate_validation" "subdomains" {
  certificate_arn         = aws_acm_certificate.subdomains.arn
  validation_record_fqdns = [for record in aws_route53_record.subdomains : record.fqdn]

  # See the api_domain validation above — same delegation dependency.
  timeouts {
    # ACM's own DNS validation is documented as taking up to 30 minutes, and it
    # backs off after a failed check — so a certificate created moments after the
    # delegation lands can sit pending well past the half hour before ACM looks
    # again. 20m was too tight and failed a deploy whose DNS was provably
    # correct: the validation record resolved from the root, with no cache, to
    # exactly the value ACM had asked for.
    #
    # Fast failure is no longer this timeout's job. app/dns_preflight.go catches
    # a genuinely undelegated zone in about two seconds, before the apply starts,
    # so all this bound has to do is stop a stuck apply running for the 75-minute
    # provider default.
    create = "45m"
  }
}

