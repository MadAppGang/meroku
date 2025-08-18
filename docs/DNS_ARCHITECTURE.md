# DNS Architecture

## Hierarchy

```
Production Account (Root Zone)
├── example.com (A/CNAME records)
├── dev.example.com (NS delegation → Dev Account)
├── staging.example.com (NS delegation → Staging Account)
└── api.example.com (Production services)
```

## Core Principles

1. **Root zone ALWAYS in production** (enforced)
2. **Single domain name** across all configs
3. **Automatic prefix addition** via `add_env_domain_prefix`
4. **Cross-account IAM delegation** for subdomain management

## Configuration Schema

### dns.yaml
```yaml
root_domain: example.com
root_account:
  account_id: "123456789012"
  zone_id: "Z1234567890ABC"
  delegation_role_arn: "arn:aws:iam::123456789012:role/dns-delegation"
delegated_zones:
  - subdomain: dev.example.com
    account_id: "234567890123"
    zone_id: "Z2345678901DEF"
    ns_records: [ns-123.awsdns-12.com, ...]
```

### Production (prod.yaml)
```yaml
domain:
  enabled: true
  domain_name: example.com      # Root domain
  zone_id: Z1234567890ABC       # Set by wizard
  add_env_domain_prefix: false  # No prefix
```

### Non-Production (dev.yaml)
```yaml
domain:
  enabled: true
  domain_name: example.com         # Same root domain
  add_env_domain_prefix: true     # Adds 'dev.' prefix
  root_zone_id: Z1234567890ABC    # For delegation
  root_account_id: "123456789012"
```

## Terraform Integration

### Production
```hcl
module "domain" {
  source = "../modules/domain"
  domain_zone = "example.com"
  zone_id = "Z1234567890ABC"
  add_env_domain_prefix = false
}
```

### Non-Production
```hcl
provider "aws" {
  alias = "root"
  assume_role {
    role_arn = "arn:aws:iam::123456789012:role/dns-delegation"
  }
}

module "dns_delegation" {
  source = "../modules/dns-delegation"
  providers = { aws.root = aws.root }
  subdomain = "dev.example.com"
  root_zone_id = "Z1234567890ABC"
}
```

## IAM Delegation Role

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {
      "AWS": ["arn:aws:iam::234567890123:root"]
    },
    "Action": "sts:AssumeRole"
  }]
}
```

Permissions: Route53 ChangeResourceRecordSets on root zone only.

## Cost

- Route53 Zone: $0.50/month
- Queries: $0.40/million
- Total typical: ~$2-3/month for 3 environments