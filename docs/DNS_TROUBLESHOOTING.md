# DNS Troubleshooting

## Common Issues

### DNS Not Propagating

**Symptoms:** Old nameservers still returned after update

**Fix:**
```bash
# Check current NS records
dig NS example.com @8.8.8.8

# Wait for TTL (up to 48h for some registrars)
# Force check with different servers
dig NS example.com @1.1.1.1
```

### AWS Profile Not Found

**Symptoms:** "No AWS profile found for account ID"

**Fix:** The wizard creates profiles automatically. If it fails:
```bash
aws configure sso
# Profile name: projectname-prod
# SSO start URL: https://your-sso.awsapps.com/start
# Region: us-east-1
```

### Zone Validation Failed

**Symptoms:** "Cannot validate zone" error

**Fix:**
```bash
# Verify zone exists
aws route53 list-hosted-zones --profile prod

# Check zone ID matches dns.yaml
cat dns.yaml | grep zone_id

# If corrupted, delete dns.yaml and re-run setup
rm dns.yaml
./meroku  # Select DNS Setup
```

### Cross-Account Access Denied

**Symptoms:** Permission errors during subdomain creation

**Fix:**
```bash
# Verify delegation role exists
aws iam get-role --role-name dns-delegation --profile prod

# Check trust relationship includes target account
aws iam get-role --role-name dns-delegation --profile prod | grep AssumeRolePolicyDocument
```

### Subdomain Not Resolving

**Symptoms:** NXDOMAIN for dev.example.com

**Fix:**
```bash
# Check NS delegation records
dig NS dev.example.com

# Verify subdomain zone exists
aws route53 list-hosted-zones --profile dev

# Check delegation in root zone
aws route53 list-resource-record-sets \
  --hosted-zone-id ROOT_ZONE_ID \
  --query "ResourceRecordSets[?Name=='dev.example.com.']"
```

### Production Environment Missing

**Symptoms:** "Production environment not configured"

**Fix:** Enter AWS Account ID when prompted. System automatically:
1. Creates AWS profile if missing
2. Creates prod.yaml
3. Continues with DNS setup

## Debug Commands

```bash
# Test specific nameserver
dig @ns-123.awsdns-45.com example.com

# Trace DNS resolution path
dig +trace dev.example.com

# Check zone records
aws route53 list-resource-record-sets --hosted-zone-id Z123456

# Verify propagation globally
nslookup example.com 8.8.8.8  # Google
nslookup example.com 1.1.1.1  # Cloudflare
```

## Quick Fixes

| Issue | Command |
|-------|---------|
| Re-run DNS setup | `./meroku` → DNS Setup |
| Check DNS status | `./meroku dns status` |
| Validate configuration | `./meroku dns validate` |
| Remove subdomain | `./meroku dns remove dev.example.com` |
| Force Terraform refresh | `make infra-plan env=dev` |

## Complete Rollback

If you need to completely remove DNS setup:

```bash
# 1. Delete Route53 zones
aws route53 delete-hosted-zone --id ZONE_ID --profile prod

# 2. Restore original nameservers at registrar

# 3. Clean configuration
rm dns.yaml
sed -i '/domain:/,/^[^ ]/d' dev.yaml prod.yaml

# 4. Remove from Terraform
make infra-plan env=dev
make infra-apply env=dev
```