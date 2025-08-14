resource "aws_route53_zone" "root" {
  name    = var.domain_name
  comment = "Root DNS zone for ${var.domain_name}"

  tags = {
    Name        = var.domain_name
    Environment = "root"
    Purpose     = "DNS Root Zone"
  }
}

resource "aws_iam_role" "dns_delegation" {
  name               = "${replace(var.domain_name, ".", "-")}-dns-delegation"
  description        = "Role for cross-account DNS delegation for ${var.domain_name}"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          AWS = var.trusted_account_ids
        }
        Action = "sts:AssumeRole"
      }
    ]
  })

  tags = {
    Name        = "${var.domain_name}-dns-delegation"
    Purpose     = "DNS Cross-Account Delegation"
  }
}

resource "aws_iam_role_policy" "dns_delegation" {
  name = "route53-delegation-policy"
  role = aws_iam_role.dns_delegation.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "route53:GetHostedZone",
          "route53:ListHostedZones",
          "route53:ListResourceRecordSets",
          "route53:ChangeResourceRecordSets",
          "route53:GetChange"
        ]
        Resource = [
          "arn:aws:route53:::hostedzone/${aws_route53_zone.root.zone_id}",
          "arn:aws:route53:::change/*"
        ]
      }
    ]
  })
}

resource "aws_route53_record" "root_ns" {
  count = var.create_ns_records ? 1 : 0

  zone_id = aws_route53_zone.root.zone_id
  name    = var.domain_name
  type    = "NS"
  ttl     = 172800

  records = aws_route53_zone.root.name_servers
}