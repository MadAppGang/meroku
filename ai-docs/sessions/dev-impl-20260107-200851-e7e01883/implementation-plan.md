# Implementation Plan: Multi-Domain Support for AWS SES Module

**Date:** 2026-01-07
**Status:** Planning Phase
**Reference Pattern:** CloudFront module's conditional DNS record creation

---

## Executive Summary

This plan outlines the changes needed to add multiple domain support to the AWS SES module, similar to how CloudFront handles multiple domain aliases. The key enhancement is allowing each domain to optionally have its own Route53 zone for automatic DNS record creation, while providing manual DNS instructions when zone_id is not specified.

---

## 1. Data Structures

### 1.1 Go Model Changes (`app/model.go`)

**Current Structure:**
```go
type Ses struct {
	Enabled    bool     `yaml:"enabled"`
	DomainName string   `yaml:"domain_name"`
	TestEmails []string `yaml:"test_emails"`
}
```

**Proposed Structure:**
```go
type Ses struct {
	Enabled    bool        `yaml:"enabled"`
	// Legacy single domain support (for backward compatibility)
	DomainName string      `yaml:"domain_name,omitempty"`
	TestEmails []string    `yaml:"test_emails,omitempty"`

	// Multi-domain support (Schema v17)
	Domains    []SESDomain `yaml:"domains,omitempty"`

	// Global SES configuration (applies to all domains)
	EnableMailFrom      *bool  `yaml:"enable_mail_from,omitempty"`      // Default: true
	MailFromSubdomain   string `yaml:"mail_from_subdomain,omitempty"`   // Default: "bounce"
	DMARCPolicy         string `yaml:"dmarc_policy,omitempty"`          // Default: "none"
	DMARCRuaEmail       string `yaml:"dmarc_rua_email,omitempty"`       // Optional
}

type SESDomain struct {
	Domain            string   `yaml:"domain"`                        // Required: e.g., "mail.example.com"
	ZoneID            string   `yaml:"zone_id,omitempty"`             // Optional: Route53 zone ID
	TestEmails        []string `yaml:"test_emails,omitempty"`         // Domain-specific test emails

	// Per-domain overrides (optional)
	EnableMailFrom    *bool  `yaml:"enable_mail_from,omitempty"`    // Override global setting
	MailFromSubdomain string `yaml:"mail_from_subdomain,omitempty"` // Override global setting
	DMARCPolicy       string `yaml:"dmarc_policy,omitempty"`        // Override global setting
	DMARCRuaEmail     string `yaml:"dmarc_rua_email,omitempty"`     // Override global setting
}
```

**Design Rationale:**
- **Backward Compatibility:** Legacy `domain_name` field remains for existing configurations
- **Optional zone_id:** Matches CloudFront pattern; when missing, outputs DNS instructions
- **Per-domain overrides:** Allows fine-grained control while maintaining global defaults
- **Pointer for bool:** Distinguishes between "not set" and "explicitly false"

### 1.2 TypeScript Interface Changes (`web/src/types/yamlConfig.ts`)

**Current Interface:**
```typescript
interface SESConfig {
  enabled: boolean;
  domain_name?: string;
  test_emails?: string[];
}
```

**Proposed Interface:**
```typescript
interface SESConfig {
  enabled: boolean;
  // Legacy support
  domain_name?: string;
  test_emails?: string[];

  // Multi-domain support
  domains?: SESDomain[];

  // Global configuration
  enable_mail_from?: boolean;
  mail_from_subdomain?: string;
  dmarc_policy?: 'none' | 'quarantine' | 'reject';
  dmarc_rua_email?: string;
}

interface SESDomain {
  domain: string;                   // Required
  zone_id?: string;                 // Optional - for automatic DNS
  test_emails?: string[];           // Domain-specific test emails

  // Per-domain overrides
  enable_mail_from?: boolean;
  mail_from_subdomain?: string;
  dmarc_policy?: 'none' | 'quarantine' | 'reject';
  dmarc_rua_email?: string;
}
```

---

## 2. Terraform Module Changes

### 2.1 Variables (`modules/ses/variables.tf`)

**New Variables:**
```hcl
variable "domains" {
  description = <<-EOT
    List of email domains to configure with SES. Each domain can optionally have:
    - domain: The email domain (required)
    - zone_id: Route53 zone ID for automatic DNS record creation (optional)
    - test_emails: Domain-specific test email addresses (optional)
    - enable_mail_from: Enable custom MAIL FROM domain (optional, defaults to global)
    - mail_from_subdomain: Subdomain for MAIL FROM (optional, defaults to global)
    - dmarc_policy: DMARC policy for this domain (optional, defaults to global)
    - dmarc_rua_email: DMARC report email (optional, defaults to global)
  EOT
  type = list(object({
    domain              = string
    zone_id             = optional(string)
    test_emails         = optional(list(string), [])
    enable_mail_from    = optional(bool)
    mail_from_subdomain = optional(string)
    dmarc_policy        = optional(string)
    dmarc_rua_email     = optional(string)
  }))
  default = []
}

# Legacy support (deprecated)
variable "domain" {
  description = "[DEPRECATED] Use domains list instead. Single email domain to verify with SES"
  type        = string
  default     = ""
}

variable "zone_id" {
  description = "[DEPRECATED] Use zone_id in domains list. Route53 hosted zone ID for DNS record creation"
  type        = string
  default     = ""
}

# Global defaults
variable "global_enable_mail_from" {
  description = "Global default for enabling custom MAIL FROM domain"
  type        = bool
  default     = true
}

variable "global_mail_from_subdomain" {
  description = "Global default subdomain for custom MAIL FROM"
  type        = string
  default     = "bounce"
}

variable "global_dmarc_policy" {
  description = "Global default DMARC policy"
  type        = string
  default     = "none"

  validation {
    condition     = contains(["none", "quarantine", "reject"], var.global_dmarc_policy)
    error_message = "DMARC policy must be: none, quarantine, or reject"
  }
}

variable "global_dmarc_rua_email" {
  description = "Global default email for DMARC reports (defaults to dmarc-reports@{domain})"
  type        = string
  default     = ""
}

# Keep test_emails for backward compatibility
variable "test_emails" {
  description = "[DEPRECATED] Use test_emails in domains list. Legacy test email addresses"
  type        = list(string)
  default     = []
}
```

### 2.2 Main Configuration (`modules/ses/main.tf`)

**Key Changes:**

1. **Migrate legacy to domains:**
```hcl
locals {
  # Merge legacy single domain with new domains list
  legacy_domain = var.domain != "" ? [{
    domain              = var.domain
    zone_id             = var.zone_id != "" ? var.zone_id : null
    test_emails         = var.test_emails
    enable_mail_from    = null  # Use global
    mail_from_subdomain = null
    dmarc_policy        = null
    dmarc_rua_email     = null
  }] : []

  all_domains = concat(local.legacy_domain, var.domains)

  # Create map of domains for easy lookup
  domains_map = { for d in local.all_domains : d.domain => d }
}
```

2. **Domain Identity (for_each pattern):**
```hcl
resource "aws_ses_domain_identity" "domain" {
  for_each = local.domains_map

  domain = each.value.domain
}
```

3. **Conditional DNS Records (CloudFront pattern):**
```hcl
# Domain verification TXT record (only if zone_id provided)
resource "aws_route53_record" "domain_amazonses_verification_record" {
  for_each = { for k, v in local.domains_map : k => v if v.zone_id != null }

  zone_id = each.value.zone_id
  name    = "_amazonses.${each.value.domain}"
  type    = "TXT"
  ttl     = "600"
  records = [aws_ses_domain_identity.domain[each.key].verification_token]
}

# DKIM Configuration
resource "aws_ses_domain_dkim" "dkim" {
  for_each = local.domains_map

  domain = aws_ses_domain_identity.domain[each.key].domain
}

# DKIM CNAME records (only if zone_id provided)
resource "aws_route53_record" "domain_amazonses_dkim_record" {
  for_each = merge([
    for domain_key, domain in local.domains_map : {
      for idx in range(3) : "${domain_key}-dkim-${idx}" => {
        domain_key = domain_key
        zone_id    = domain.zone_id
        domain     = domain.domain
        dkim_token = aws_ses_domain_dkim.dkim[domain_key].dkim_tokens[idx]
        idx        = idx
      } if domain.zone_id != null
    }
  ]...)

  zone_id = each.value.zone_id
  name    = "${each.value.dkim_token}._domainkey.${each.value.domain}"
  type    = "CNAME"
  ttl     = "3600"
  records = ["${each.value.dkim_token}.dkim.amazonses.com"]
}

# SPF Record (only if zone_id provided)
resource "aws_route53_record" "spf" {
  for_each = { for k, v in local.domains_map : k => v if v.zone_id != null }

  zone_id = each.value.zone_id
  name    = each.value.domain
  type    = "TXT"
  ttl     = "600"
  records = ["v=spf1 include:amazonses.com ~all"]
}

# Custom MAIL FROM Domain
locals {
  # Calculate per-domain mail_from settings
  domains_with_mail_from = {
    for k, v in local.domains_map : k => {
      domain              = v.domain
      zone_id             = v.zone_id
      enable_mail_from    = coalesce(v.enable_mail_from, var.global_enable_mail_from)
      mail_from_subdomain = coalesce(v.mail_from_subdomain, var.global_mail_from_subdomain)
      mail_from_domain    = "${coalesce(v.mail_from_subdomain, var.global_mail_from_subdomain)}.${v.domain}"
    }
  }

  enabled_mail_from = { for k, v in local.domains_with_mail_from : k => v if v.enable_mail_from }
}

resource "aws_ses_domain_mail_from" "mail_from" {
  for_each = local.enabled_mail_from

  domain           = aws_ses_domain_identity.domain[each.key].domain
  mail_from_domain = each.value.mail_from_domain
  behavior_on_mx_failure = "UseDefaultValue"
}

# MX record for custom MAIL FROM domain (only if zone_id provided)
resource "aws_route53_record" "mail_from_mx" {
  for_each = { for k, v in local.enabled_mail_from : k => v if v.zone_id != null }

  zone_id = each.value.zone_id
  name    = each.value.mail_from_domain
  type    = "MX"
  ttl     = "600"
  records = ["10 feedback-smtp.${data.aws_region.current.name}.amazonses.com"]
}

# MAIL FROM SPF record (only if zone_id provided)
resource "aws_route53_record" "mail_from_spf" {
  for_each = { for k, v in local.enabled_mail_from : k => v if v.zone_id != null }

  zone_id = each.value.zone_id
  name    = each.value.mail_from_domain
  type    = "TXT"
  ttl     = "600"
  records = ["v=spf1 include:amazonses.com ~all"]
}

# DMARC Record (only if zone_id provided)
locals {
  domains_with_dmarc = {
    for k, v in local.domains_map : k => {
      domain        = v.domain
      zone_id       = v.zone_id
      dmarc_policy  = coalesce(v.dmarc_policy, var.global_dmarc_policy)
      dmarc_rua_email = v.dmarc_rua_email != null ? v.dmarc_rua_email : (
        var.global_dmarc_rua_email != "" ? var.global_dmarc_rua_email : "dmarc-reports@${v.domain}"
      )
    }
  }
}

resource "aws_route53_record" "dmarc" {
  for_each = { for k, v in local.domains_with_dmarc : k => v if v.zone_id != null }

  zone_id = each.value.zone_id
  name    = "_dmarc.${each.value.domain}"
  type    = "TXT"
  ttl     = "300"
  records = ["v=DMARC1; p=${each.value.dmarc_policy}; pct=100; rua=mailto:${each.value.dmarc_rua_email}"]
}

# Test Email Identities (merge all test emails from all domains)
locals {
  all_test_emails = distinct(concat(
    var.test_emails,  # Legacy test emails
    flatten([for d in var.domains : d.test_emails])
  ))
}

resource "aws_ses_email_identity" "emails" {
  count = length(local.all_test_emails)
  email = local.all_test_emails[count.index]
}
```

### 2.3 Outputs (`modules/ses/outputs.tf`)

**New Outputs:**
```hcl
output "domain_identities" {
  description = "Map of domain names to their SES identity ARNs"
  value = {
    for k, v in aws_ses_domain_identity.domain : k => v.arn
  }
}

output "domains" {
  description = "List of configured email domains"
  value       = keys(local.domains_map)
}

output "dkim_tokens" {
  description = "Map of domains to their DKIM tokens"
  value = {
    for k, v in aws_ses_domain_dkim.dkim : k => v.dkim_tokens
  }
}

output "verification_tokens" {
  description = "Map of domains to their verification tokens"
  value = {
    for k, v in aws_ses_domain_identity.domain : k => v.verification_token
  }
}

# Manual DNS instructions for domains without zone_id
output "manual_dns_instructions" {
  description = "DNS records to create manually for domains without zone_id"
  value = {
    for domain_key, domain in local.domains_map : domain_key => {
      domain = domain.domain
      records = domain.zone_id == null ? [
        {
          name  = "_amazonses.${domain.domain}"
          type  = "TXT"
          value = aws_ses_domain_identity.domain[domain_key].verification_token
          note  = "Domain verification record"
        },
        {
          name  = domain.domain
          type  = "TXT"
          value = "v=spf1 include:amazonses.com ~all"
          note  = "SPF authorization record"
        },
        # DKIM records
        {
          name  = "${aws_ses_domain_dkim.dkim[domain_key].dkim_tokens[0]}._domainkey.${domain.domain}"
          type  = "CNAME"
          value = "${aws_ses_domain_dkim.dkim[domain_key].dkim_tokens[0]}.dkim.amazonses.com"
          note  = "DKIM record 1/3"
        },
        {
          name  = "${aws_ses_domain_dkim.dkim[domain_key].dkim_tokens[1]}._domainkey.${domain.domain}"
          type  = "CNAME"
          value = "${aws_ses_domain_dkim.dkim[domain_key].dkim_tokens[1]}.dkim.amazonses.com"
          note  = "DKIM record 2/3"
        },
        {
          name  = "${aws_ses_domain_dkim.dkim[domain_key].dkim_tokens[2]}._domainkey.${domain.domain}"
          type  = "CNAME"
          value = "${aws_ses_domain_dkim.dkim[domain_key].dkim_tokens[2]}.dkim.amazonses.com"
          note  = "DKIM record 3/3"
        },
        # DMARC record
        {
          name  = "_dmarc.${domain.domain}"
          type  = "TXT"
          value = "v=DMARC1; p=${local.domains_with_dmarc[domain_key].dmarc_policy}; pct=100; rua=mailto:${local.domains_with_dmarc[domain_key].dmarc_rua_email}"
          note  = "DMARC policy record"
        },
        # MAIL FROM records (if enabled)
        lookup(local.enabled_mail_from, domain_key, null) != null ? {
          name  = local.enabled_mail_from[domain_key].mail_from_domain
          type  = "MX"
          value = "10 feedback-smtp.${data.aws_region.current.name}.amazonses.com"
          note  = "Custom MAIL FROM MX record"
        } : null,
        lookup(local.enabled_mail_from, domain_key, null) != null ? {
          name  = local.enabled_mail_from[domain_key].mail_from_domain
          type  = "TXT"
          value = "v=spf1 include:amazonses.com ~all"
          note  = "Custom MAIL FROM SPF record"
        } : null,
      ] : []
    } if domain.zone_id == null
  }
}

output "deliverability_summary" {
  description = "Summary of email deliverability configuration for all domains"
  value = {
    for k, v in local.domains_map : k => {
      domain           = v.domain
      zone_id_provided = v.zone_id != null
      auto_dns         = v.zone_id != null ? "Enabled" : "Manual setup required"
      dkim_enabled     = true
      spf_enabled      = true
      mail_from_domain = lookup(local.enabled_mail_from, k, null) != null ? local.enabled_mail_from[k].mail_from_domain : "amazonses.com (default)"
      dmarc_policy     = local.domains_with_dmarc[k].dmarc_policy
      dmarc_rua_email  = local.domains_with_dmarc[k].dmarc_rua_email
    }
  }
}

# Legacy outputs (for backward compatibility)
output "domain_identity_arn" {
  description = "[DEPRECATED] ARN of the first SES domain identity (for backward compatibility)"
  value       = length(aws_ses_domain_identity.domain) > 0 ? values(aws_ses_domain_identity.domain)[0].arn : null
}

output "domain" {
  description = "[DEPRECATED] The first email domain configured in SES (for backward compatibility)"
  value       = length(local.domains_map) > 0 ? keys(local.domains_map)[0] : null
}

output "mail_from_domain" {
  description = "[DEPRECATED] The custom MAIL FROM domain for the first domain (for backward compatibility)"
  value       = length(local.enabled_mail_from) > 0 ? values(local.enabled_mail_from)[0].mail_from_domain : null
}
```

---

## 3. Template Changes (`env/main.hbs`)

**Current Template:**
```handlebars
{{#if ses.enabled}}
module "ses" {
  source = "{{modules}}/ses"
  project = "{{project}}"
  env     = "{{env}}"
  {{#if ses.domain_name}}
  domain = "{{ses.domain_name}}"
  {{else}}
    {{#compare env "==" "prod"}}
  domain = "mail.{{ domain.domain_name }}"
    {{else}}
  domain = "mail.{{ env }}.{{ domain.domain_name }}"
    {{/compare}}
  {{/if}}
  test_emails = {{{array ses.test_emails}}}
  {{#if domain.enabled}}
  zone_id = module.domain.zone_id
  {{/if}}

  # Email deliverability configuration
  enable_mail_from    = {{default ses.enable_mail_from true}}
  mail_from_subdomain = "{{default ses.mail_from_subdomain "bounce"}}"
  dmarc_policy        = "{{default ses.dmarc_policy "none"}}"
  {{#if ses.dmarc_rua_email}}
  dmarc_rua_email     = "{{ses.dmarc_rua_email}}"
  {{/if}}
}
{{/if}}
```

**Proposed Template:**
```handlebars
{{#if ses.enabled}}
module "ses" {
  source = "{{modules}}/ses"
  project = "{{project}}"
  env     = "{{env}}"

  {{#if ses.domains}}
  # Multi-domain configuration
  domains = [
    {{#each ses.domains}}
    {
      domain = "{{this.domain}}"
      {{#if this.zone_id}}
      zone_id = "{{this.zone_id}}"
      {{else}}
        {{#if ../domain.enabled}}
      zone_id = module.domain.zone_id  # Use main domain's zone if available
        {{/if}}
      {{/if}}
      {{#if this.test_emails}}
      test_emails = {{{array this.test_emails}}}
      {{/if}}
      {{#if (exists this.enable_mail_from)}}
      enable_mail_from = {{this.enable_mail_from}}
      {{/if}}
      {{#if this.mail_from_subdomain}}
      mail_from_subdomain = "{{this.mail_from_subdomain}}"
      {{/if}}
      {{#if this.dmarc_policy}}
      dmarc_policy = "{{this.dmarc_policy}}"
      {{/if}}
      {{#if this.dmarc_rua_email}}
      dmarc_rua_email = "{{this.dmarc_rua_email}}"
      {{/if}}
    }{{#unless @last}},{{/unless}}
    {{/each}}
  ]
  {{else}}
  # Legacy single domain configuration (backward compatibility)
  {{#if ses.domain_name}}
  domain = "{{ses.domain_name}}"
  {{else}}
    {{#compare env "==" "prod"}}
  domain = "mail.{{ domain.domain_name }}"
    {{else}}
  domain = "mail.{{ env }}.{{ domain.domain_name }}"
    {{/compare}}
  {{/if}}
  {{#if domain.enabled}}
  zone_id = module.domain.zone_id
  {{/if}}
  test_emails = {{{array ses.test_emails}}}
  {{/if}}

  # Global configuration
  global_enable_mail_from    = {{default ses.enable_mail_from true}}
  global_mail_from_subdomain = "{{default ses.mail_from_subdomain "bounce"}}"
  global_dmarc_policy        = "{{default ses.dmarc_policy "none"}}"
  {{#if ses.dmarc_rua_email}}
  global_dmarc_rua_email     = "{{ses.dmarc_rua_email}}"
  {{/if}}
}
{{/if}}
```

---

## 4. Web UI Changes

### 4.1 Component Structure (`web/src/components/SESNodeProperties.tsx`)

**New UI Sections:**

1. **Domain Management Section:**
   - List of configured domains (similar to ECR trusted accounts)
   - Add/Remove domain buttons
   - Each domain shows:
     - Domain name input
     - Zone ID input (optional) with info tooltip
     - Indicator: "Auto DNS" (green) or "Manual DNS Required" (yellow)
     - Test emails for this domain
     - Advanced settings (collapsed by default):
       - Per-domain MAIL FROM override
       - Per-domain DMARC override

2. **Global Settings Section:**
   - Default MAIL FROM configuration
   - Default DMARC policy
   - Default DMARC reporting email

3. **DNS Status Indicators:**
   ```typescript
   const getDNSStatus = (domain: SESDomain) => {
     if (domain.zone_id) {
       return {
         status: 'auto',
         color: 'green',
         icon: CheckCircle,
         text: 'Automatic DNS'
       };
     }
     return {
       status: 'manual',
       color: 'yellow',
       icon: AlertCircle,
       text: 'Manual DNS Setup Required'
     };
   };
   ```

4. **Manual DNS Instructions Card:**
   - Shows when any domain lacks zone_id
   - Displays required DNS records in copyable format
   - Expandable per-domain sections

**Component Pseudo-Code:**
```typescript
export function SESNodeProperties({ config, onConfigChange }: SESNodePropertiesProps) {
  const [newDomain, setNewDomain] = useState<SESDomain>({
    domain: '',
    zone_id: undefined,
    test_emails: []
  });

  const sesConfig = config.ses || { enabled: false, domains: [] };
  const domains = sesConfig.domains || [];

  const handleAddDomain = () => {
    if (newDomain.domain) {
      onConfigChange({
        ses: {
          ...sesConfig,
          domains: [...domains, newDomain]
        }
      });
      setNewDomain({ domain: '', zone_id: undefined, test_emails: [] });
    }
  };

  const handleRemoveDomain = (index: number) => {
    onConfigChange({
      ses: {
        ...sesConfig,
        domains: domains.filter((_, i) => i !== index)
      }
    });
  };

  const handleUpdateDomain = (index: number, updated: SESDomain) => {
    const newDomains = [...domains];
    newDomains[index] = updated;
    onConfigChange({
      ses: {
        ...sesConfig,
        domains: newDomains
      }
    });
  };

  return (
    <div className="space-y-6">
      {/* Enable/Disable SES Card */}
      <Card>...</Card>

      {sesConfig.enabled && (
        <>
          {/* Global Settings Card */}
          <Card>
            <CardHeader>
              <CardTitle>Global Email Settings</CardTitle>
              <CardDescription>
                Default settings applied to all domains (can be overridden per domain)
              </CardDescription>
            </CardHeader>
            <CardContent>
              {/* MAIL FROM, DMARC policy, DMARC email inputs */}
            </CardContent>
          </Card>

          {/* Domain Management Card */}
          <Card>
            <CardHeader>
              <CardTitle>Email Domains</CardTitle>
              <CardDescription>
                Configure multiple domains for sending emails via SES
              </CardDescription>
            </CardHeader>
            <CardContent>
              {/* Add new domain */}
              <div className="space-y-4">
                <div className="grid grid-cols-2 gap-4">
                  <Input
                    placeholder="mail.example.com"
                    value={newDomain.domain}
                    onChange={(e) => setNewDomain({...newDomain, domain: e.target.value})}
                  />
                  <Input
                    placeholder="Zone ID (optional)"
                    value={newDomain.zone_id || ''}
                    onChange={(e) => setNewDomain({...newDomain, zone_id: e.target.value || undefined})}
                  />
                </div>
                <Button onClick={handleAddDomain}>
                  <Plus className="w-4 h-4 mr-2" />
                  Add Domain
                </Button>
              </div>

              {/* Domain list */}
              {domains.map((domain, index) => (
                <DomainCard
                  key={index}
                  domain={domain}
                  onUpdate={(updated) => handleUpdateDomain(index, updated)}
                  onRemove={() => handleRemoveDomain(index)}
                />
              ))}
            </CardContent>
          </Card>

          {/* Manual DNS Instructions (if any domain lacks zone_id) */}
          {domains.some(d => !d.zone_id) && (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <AlertCircle className="w-5 h-5 text-yellow-500" />
                  Manual DNS Setup Required
                </CardTitle>
                <CardDescription>
                  Create these DNS records in your domain provider for domains without zone_id
                </CardDescription>
              </CardHeader>
              <CardContent>
                {domains.filter(d => !d.zone_id).map((domain, index) => (
                  <ManualDNSInstructions key={index} domain={domain} />
                ))}
              </CardContent>
            </Card>
          )}

          {/* Resources Created Card */}
          <Card>...</Card>
        </>
      )}
    </div>
  );
}
```

### 4.2 New Sub-Components

**DomainCard Component:**
```typescript
interface DomainCardProps {
  domain: SESDomain;
  onUpdate: (domain: SESDomain) => void;
  onRemove: () => void;
}

function DomainCard({ domain, onUpdate, onRemove }: DomainCardProps) {
  const [showAdvanced, setShowAdvanced] = useState(false);
  const dnsStatus = getDNSStatus(domain);

  return (
    <div className="border rounded-lg p-4 space-y-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Globe className="w-5 h-5" />
          <div>
            <h4 className="font-medium">{domain.domain}</h4>
            <div className="flex items-center gap-2 mt-1">
              <dnsStatus.icon className={`w-4 h-4 text-${dnsStatus.color}-500`} />
              <span className="text-xs text-gray-400">{dnsStatus.text}</span>
            </div>
          </div>
        </div>
        <Button variant="destructive" size="sm" onClick={onRemove}>
          <X className="w-4 h-4" />
        </Button>
      </div>

      <div className="space-y-2">
        <Label>Zone ID (Optional)</Label>
        <Input
          value={domain.zone_id || ''}
          onChange={(e) => onUpdate({...domain, zone_id: e.target.value || undefined})}
          placeholder="Leave empty for manual DNS setup"
        />
        <p className="text-xs text-gray-500">
          {domain.zone_id
            ? "DNS records will be created automatically"
            : "You'll need to create DNS records manually"}
        </p>
      </div>

      {/* Test emails for this domain */}
      <TestEmailsSection
        emails={domain.test_emails || []}
        onChange={(emails) => onUpdate({...domain, test_emails: emails})}
      />

      {/* Advanced settings */}
      <Collapsible open={showAdvanced} onOpenChange={setShowAdvanced}>
        <CollapsibleTrigger>
          <Button variant="ghost" size="sm">
            Advanced Settings
          </Button>
        </CollapsibleTrigger>
        <CollapsibleContent>
          {/* Per-domain overrides */}
        </CollapsibleContent>
      </Collapsible>
    </div>
  );
}
```

**ManualDNSInstructions Component:**
```typescript
function ManualDNSInstructions({ domain }: { domain: SESDomain }) {
  const dnsRecords = generateManualDNSRecords(domain);

  return (
    <div className="space-y-3 border-l-4 border-yellow-500 pl-4">
      <h4 className="font-medium">{domain.domain}</h4>
      <p className="text-sm text-gray-400">
        After applying Terraform, copy these DNS records to your domain provider:
      </p>

      {dnsRecords.map((record, index) => (
        <div key={index} className="bg-gray-800 rounded p-3 font-mono text-xs">
          <div className="flex justify-between items-center mb-2">
            <span className="text-gray-400">{record.note}</span>
            <CopyButton text={`${record.name} ${record.type} ${record.value}`} />
          </div>
          <div className="space-y-1">
            <div><span className="text-gray-500">Name:</span> {record.name}</div>
            <div><span className="text-gray-500">Type:</span> {record.type}</div>
            <div><span className="text-gray-500">Value:</span> {record.value}</div>
          </div>
        </div>
      ))}

      <Alert>
        <Info className="h-4 w-4" />
        <AlertDescription>
          After creating these DNS records, it may take up to 72 hours for verification to complete.
          You can check status in the AWS SES Console.
        </AlertDescription>
      </Alert>
    </div>
  );
}
```

---

## 5. YAML Schema Migration

### 5.1 Migration Version 17 (`app/migrations.go`)

**Add to AllMigrations:**
```go
{
  Version:     17,
  Description: "Add multi-domain support to SES configuration",
  Apply:       migrateToV17,
}
```

**Migration Function:**
```go
func migrateToV17(data map[string]interface{}) error {
  ses, ok := data["ses"].(map[interface{}]interface{})
  if !ok || ses == nil {
    return nil // No SES config, nothing to migrate
  }

  // Check if already migrated (has domains array)
  if _, hasNewFormat := ses["domains"]; hasNewFormat {
    return nil
  }

  // Check if legacy format exists
  domainName, hasDomain := ses["domain_name"]
  if !hasDomain {
    return nil // No domain configured
  }

  // Extract zone_id if it exists (from domain module)
  var zoneID interface{} = nil
  if domain, ok := data["domain"].(map[interface{}]interface{}); ok {
    if enabled, ok := domain["enabled"].(bool); ok && enabled {
      // Note: zone_id will be populated at template render time via module.domain.zone_id
      // For migration, we just note that it should use the domain zone
      zoneID = "__use_domain_zone__" // Marker for template
    }
  }

  // Migrate to new format
  // Keep old format for backward compatibility
  // Add new domains array with single domain
  ses["domains"] = []interface{}{
    map[interface{}]interface{}{
      "domain":  domainName,
      "zone_id": zoneID,
    },
  }

  // Migrate test_emails to domain-specific
  if testEmails, ok := ses["test_emails"]; ok {
    if domains, ok := ses["domains"].([]interface{}); ok && len(domains) > 0 {
      if domain, ok := domains[0].(map[interface{}]interface{}); ok {
        domain["test_emails"] = testEmails
      }
    }
  }

  // Extract global settings (if they exist)
  globalSettings := map[string]interface{}{
    "enable_mail_from":    true,
    "mail_from_subdomain": "bounce",
    "dmarc_policy":        "none",
  }

  for key, defaultValue := range globalSettings {
    if value, ok := ses[key]; ok {
      // Keep the value, it becomes the global default
      continue
    } else {
      ses[key] = defaultValue
    }
  }

  return nil
}
```

### 5.2 Backward Compatibility Strategy

**Three-phase approach:**

1. **Phase 1: Dual Support (Current + Next Release)**
   - Both old and new formats supported
   - Templates handle both patterns
   - Migration auto-converts on load
   - Old format still saved (for rollback)

2. **Phase 2: Deprecation Warnings (Future Release)**
   - CLI shows warnings for old format
   - Web UI shows migration prompt
   - Documentation updated

3. **Phase 3: Legacy Removal (Far Future)**
   - Remove old format support
   - Clean up template conditionals

**Example YAML Formats:**

**Legacy (still works):**
```yaml
ses:
  enabled: true
  domain_name: "mail.example.com"
  test_emails:
    - "test@example.com"
  enable_mail_from: true
  mail_from_subdomain: "bounce"
  dmarc_policy: "none"
```

**New Multi-Domain:**
```yaml
ses:
  enabled: true

  # Global defaults
  enable_mail_from: true
  mail_from_subdomain: "bounce"
  dmarc_policy: "none"
  dmarc_rua_email: "dmarc@example.com"

  # Multiple domains
  domains:
    - domain: "mail.example.com"
      zone_id: "Z1234567890ABC"  # Auto DNS
      test_emails:
        - "test1@example.com"

    - domain: "alerts.example.com"
      zone_id: "Z1234567890ABC"  # Same zone
      test_emails:
        - "test2@example.com"

    - domain: "external-domain.com"
      # No zone_id - manual DNS setup required
      test_emails:
        - "test3@external-domain.com"
      # Override DMARC for this domain
      dmarc_policy: "quarantine"
      dmarc_rua_email: "reports@external-domain.com"
```

---

## 6. Testing Strategy

### 6.1 Unit Tests

**Go Tests (`app/migrations_test.go`):**
```go
func TestMigrateToV17_LegacySingleDomain(t *testing.T) {
  data := map[string]interface{}{
    "schema_version": 16,
    "ses": map[interface{}]interface{}{
      "enabled":     true,
      "domain_name": "mail.example.com",
      "test_emails": []interface{}{"test@example.com"},
    },
    "domain": map[interface{}]interface{}{
      "enabled": true,
    },
  }

  err := migrateToV17(data)
  require.NoError(t, err)

  ses := data["ses"].(map[interface{}]interface{})
  domains := ses["domains"].([]interface{})

  assert.Len(t, domains, 1)

  domain := domains[0].(map[interface{}]interface{})
  assert.Equal(t, "mail.example.com", domain["domain"])
  assert.NotNil(t, domain["zone_id"])

  testEmails := domain["test_emails"].([]interface{})
  assert.Len(t, testEmails, 1)
  assert.Equal(t, "test@example.com", testEmails[0])
}

func TestMigrateToV17_NoSESConfig(t *testing.T) {
  data := map[string]interface{}{
    "schema_version": 16,
  }

  err := migrateToV17(data)
  require.NoError(t, err)

  _, hasSES := data["ses"]
  assert.False(t, hasSES)
}

func TestMigrateToV17_AlreadyMigrated(t *testing.T) {
  data := map[string]interface{}{
    "schema_version": 17,
    "ses": map[interface{}]interface{}{
      "enabled": true,
      "domains": []interface{}{
        map[interface{}]interface{}{
          "domain": "mail.example.com",
        },
      },
    },
  }

  err := migrateToV17(data)
  require.NoError(t, err)

  ses := data["ses"].(map[interface{}]interface{})
  domains := ses["domains"].([]interface{})
  assert.Len(t, domains, 1)
}
```

**Terraform Module Tests (`modules/ses/tests/`):**
```hcl
# Test 1: Multi-domain with mixed zone_id
run "multi_domain_mixed_zones" {
  command = plan

  variables {
    domains = [
      {
        domain  = "mail1.example.com"
        zone_id = "Z123"
      },
      {
        domain = "mail2.example.com"
        # No zone_id - should skip DNS records
      }
    ]
  }

  assert {
    condition = length(aws_route53_record.domain_amazonses_verification_record) == 1
    error_message = "Should only create DNS records for domains with zone_id"
  }

  assert {
    condition = length(aws_ses_domain_identity.domain) == 2
    error_message = "Should create SES identities for both domains"
  }
}

# Test 2: Legacy single domain
run "legacy_single_domain" {
  command = plan

  variables {
    domain  = "mail.example.com"
    zone_id = "Z123"
  }

  assert {
    condition = length(aws_ses_domain_identity.domain) == 1
    error_message = "Should create SES identity for legacy domain"
  }
}

# Test 3: Manual DNS output verification
run "manual_dns_output" {
  command = plan

  variables {
    domains = [
      {
        domain = "external.com"
        # No zone_id
      }
    ]
  }

  assert {
    condition = length(output.manual_dns_instructions["external.com"].records) > 0
    error_message = "Should output manual DNS instructions"
  }
}
```

### 6.2 Integration Tests

**End-to-End Test Scenarios:**

1. **Scenario 1: Fresh Multi-Domain Setup**
   - Create new YAML with multiple domains
   - Generate Terraform
   - Validate all resources created
   - Check DNS records for domains with zone_id
   - Verify manual instructions for domains without zone_id

2. **Scenario 2: Migration from Legacy**
   - Start with v16 YAML (single domain)
   - Run migration
   - Validate YAML updated to v17
   - Generate Terraform
   - Verify backward compatibility

3. **Scenario 3: Web UI Workflow**
   - Add domain via web UI
   - Toggle zone_id
   - Verify DNS status indicator updates
   - Save and validate YAML

4. **Scenario 4: Template Rendering**
   - Test template with legacy format
   - Test template with new format
   - Test template with mixed (should fail gracefully)

### 6.3 Manual Verification Checklist

**Before Deployment:**
- [ ] YAML schema validates both old and new formats
- [ ] Migration runs without errors on sample configs
- [ ] Terraform plan shows correct resources
- [ ] DNS records created only when zone_id provided
- [ ] Manual DNS instructions appear in outputs
- [ ] Web UI displays domains correctly
- [ ] Web UI shows correct DNS status indicators

**After Deployment:**
- [ ] Existing SES configurations continue to work
- [ ] New multi-domain configurations work
- [ ] DNS records verify in Route53
- [ ] SES domain verification succeeds
- [ ] DKIM tokens generated correctly
- [ ] Test emails send successfully

---

## 7. Documentation Updates

### 7.1 User Documentation

**Update Files:**
- `CLAUDE.md` - Add SES multi-domain section
- `docs/SES_MULTI_DOMAIN.md` - New comprehensive guide

**Content to Cover:**
```markdown
# SES Multi-Domain Configuration

## Overview
Configure multiple email domains with AWS SES for sending transactional emails.

## Configuration Options

### Single Domain (Legacy)
yaml
ses:
  enabled: true
  domain_name: "mail.example.com"
  test_emails:
    - "test@example.com"


### Multiple Domains
yaml
ses:
  enabled: true

  # Global defaults
  enable_mail_from: true
  mail_from_subdomain: "bounce"
  dmarc_policy: "none"

  domains:
    - domain: "mail.example.com"
      zone_id: "Z1234567890ABC"  # Automatic DNS
      test_emails:
        - "test@example.com"

    - domain: "external.com"
      # No zone_id - manual DNS setup
      test_emails:
        - "test@external.com"


## DNS Setup

### Automatic DNS (zone_id provided)
DNS records are automatically created in Route53.

### Manual DNS (zone_id not provided)
Run terraform apply and check the manual_dns_instructions output:

bash
terraform output manual_dns_instructions


Copy the DNS records to your domain provider.

## Per-Domain Overrides
yaml
domains:
  - domain: "strict.example.com"
    zone_id: "Z123"
    dmarc_policy: "reject"  # Override global setting
    mail_from_subdomain: "noreply"  # Custom MAIL FROM


## Migration from Single Domain
Existing configurations automatically migrate to the new format while maintaining backward compatibility.
```

### 7.2 Developer Documentation

**Add to `ai_docs/MIGRATIONS.md`:**
```markdown
## Schema Version 17: SES Multi-Domain Support

### Changes
- Added domains array to SES configuration
- Made zone_id optional per domain
- Added global email settings
- Added per-domain overrides

### Migration
Legacy single domain configs automatically convert to domains array with one entry.

### Backward Compatibility
Both old and new formats supported during transition period.
```

### 7.3 API Documentation

**Add to API docs (if applicable):**
```markdown
## SES Configuration Endpoints

### GET /api/ses/domains
Returns list of configured SES domains with verification status.

Response:
json
{
  "domains": [
    {
      "domain": "mail.example.com",
      "verified": true,
      "zone_id_provided": true,
      "auto_dns": true,
      "dkim_verified": true
    }
  ]
}


### POST /api/ses/domains
Add a new SES domain.

Request:
json
{
  "domain": "new.example.com",
  "zone_id": "Z123",  // optional
  "test_emails": ["test@new.example.com"]
}

```

---

## 8. Implementation Phases

### Phase 1: Core Infrastructure (Week 1)
**Deliverables:**
- [ ] Update Go model (`Ses` struct, `SESDomain` struct)
- [ ] Create migration v17
- [ ] Update Terraform module variables
- [ ] Implement for_each pattern in main.tf
- [ ] Add manual DNS instructions output
- [ ] Write unit tests for migration
- [ ] Test Terraform module in isolation

**Validation:**
- All existing tests pass
- Migration tested on sample configs
- Terraform plan generates correctly

### Phase 2: Template Integration (Week 1-2)
**Deliverables:**
- [ ] Update `env/main.hbs` template
- [ ] Add handlebars helper if needed (for domain iteration)
- [ ] Test template rendering with legacy format
- [ ] Test template rendering with new format
- [ ] Validate generated Terraform

**Validation:**
- Template handles both formats
- Generated Terraform matches expectations
- No breaking changes to existing configs

### Phase 3: Web UI (Week 2)
**Deliverables:**
- [ ] Update TypeScript interfaces
- [ ] Implement domain management UI
- [ ] Add DNS status indicators
- [ ] Create manual DNS instructions component
- [ ] Add global settings section
- [ ] Test UI with various configurations

**Validation:**
- UI displays domains correctly
- Add/remove domain works
- DNS status indicators accurate
- YAML saves correctly

### Phase 4: Documentation & Testing (Week 2-3)
**Deliverables:**
- [ ] Write user documentation
- [ ] Update CLAUDE.md
- [ ] Create integration tests
- [ ] Manual end-to-end testing
- [ ] Update migration documentation

**Validation:**
- All documentation complete
- Integration tests pass
- Manual testing checklist complete

### Phase 5: Release & Monitor (Week 3)
**Deliverables:**
- [ ] Tag release (schema v17)
- [ ] Update CHANGELOG
- [ ] Monitor for issues
- [ ] Gather user feedback

**Validation:**
- No critical bugs reported
- Users can migrate successfully
- Performance acceptable

---

## 9. Risk Assessment

### High Risk Items

1. **Backward Compatibility Breaking**
   - **Risk:** Existing SES configs fail after update
   - **Mitigation:** Dual format support, migration auto-applies, extensive testing
   - **Contingency:** Revert capability, clear upgrade path documentation

2. **Template Rendering Complexity**
   - **Risk:** Handlebars template becomes too complex
   - **Mitigation:** Keep clear separation of legacy vs new, add helper functions
   - **Contingency:** Simplify template, move logic to Go generation phase

3. **Migration Data Loss**
   - **Risk:** Migration corrupts existing YAML configs
   - **Mitigation:** Create backups before migration, test extensively
   - **Contingency:** Restore from backup, fix migration logic

### Medium Risk Items

1. **for_each Complexity in Terraform**
   - **Risk:** Nested for_each for DKIM records becomes hard to maintain
   - **Mitigation:** Use clear local variables, comprehensive comments
   - **Contingency:** Simplify to separate resources if needed

2. **Web UI State Management**
   - **Risk:** Complex domain list management leads to bugs
   - **Mitigation:** Use proper React patterns, test thoroughly
   - **Contingency:** Simplify UI, focus on read-only display first

### Low Risk Items

1. **Output Verbosity**
   - **Risk:** Manual DNS instructions output is too verbose
   - **Mitigation:** Format clearly, provide copy buttons
   - **Contingency:** Move to separate output file

---

## 10. Success Criteria

### Must Have (P0)
- [ ] Backward compatibility maintained (legacy configs work)
- [ ] Migration runs successfully on all test configs
- [ ] Multiple domains can be configured
- [ ] Optional zone_id works correctly
- [ ] Manual DNS instructions generated
- [ ] Terraform applies without errors
- [ ] No data loss during migration

### Should Have (P1)
- [ ] Web UI supports domain management
- [ ] DNS status indicators work
- [ ] Per-domain overrides functional
- [ ] Comprehensive documentation
- [ ] Integration tests pass

### Nice to Have (P2)
- [ ] DNS verification status in CLI
- [ ] Automatic zone_id detection from domain module
- [ ] Bulk domain import
- [ ] Export DNS records to file

---

## 11. Open Questions

1. **Should we auto-detect zone_id from domain module?**
   - Current plan: Template conditionally sets zone_id if domain module enabled
   - Alternative: Explicit required in config
   - **Decision needed:** Prefer explicit or implicit?

2. **How to handle DKIM record for_each complexity?**
   - Current plan: Nested merge with flattened keys
   - Alternative: Separate resource per DKIM index
   - **Decision needed:** Which is more maintainable?

3. **Should test_emails be global or per-domain only?**
   - Current plan: Both (global + per-domain, merged)
   - Alternative: Per-domain only
   - **Decision needed:** Prefer flexibility or simplicity?

4. **Migration timing for deprecation warnings?**
   - Current plan: v17 = support both, v18+ = warnings, v20 = remove
   - Alternative: Faster deprecation
   - **Decision needed:** Timeline for full migration?

---

## 12. Next Steps

1. **Immediate (This Week):**
   - Review this plan with team
   - Get approval on data structures
   - Resolve open questions
   - Set up feature branch

2. **Short Term (Weeks 1-2):**
   - Implement Phase 1 & 2
   - Get code review
   - Test with sample configs

3. **Medium Term (Weeks 2-3):**
   - Implement Phase 3 & 4
   - Documentation complete
   - Integration testing

4. **Long Term (Week 3+):**
   - Release v17
   - Monitor adoption
   - Gather feedback
   - Plan deprecation timeline

---

## Appendix A: Reference Examples

### CloudFront Conditional DNS Pattern
```hcl
# From modules/cloudfront/main.tf:222-248
resource "aws_route53_record" "cloudfront" {
  for_each = var.create_dns_records ? toset(var.domain_aliases) : toset([])

  zone_id = var.zone_id
  name    = each.value
  type    = "A"

  alias {
    name                   = aws_cloudfront_distribution.main.domain_name
    zone_id                = aws_cloudfront_distribution.main.hosted_zone_id
    evaluate_target_health = false
  }
}
```

**Key Takeaway:** Use `for_each` with conditional expression to create resources only when criteria met.

### Current SES Implementation
```hcl
# From modules/ses/main.tf:38-45
resource "aws_route53_record" "domain_amazonses_dkim_record" {
  count   = 3
  zone_id = var.zone_id
  name    = "${element(aws_ses_domain_dkim.dkim.dkim_tokens, count.index)}._domainkey.${var.domain}"
  type    = "CNAME"
  ttl     = "3600"
  records = ["${element(aws_ses_domain_dkim.dkim.dkim_tokens, count.index)}.dkim.amazonses.com"]
}
```

**Key Takeaway:** Current uses `count` for iteration. Need to switch to `for_each` for multi-domain support.

---

## Appendix B: Sample Configurations

### Minimal Configuration
```yaml
schema_version: 17
ses:
  enabled: true
  domains:
    - domain: "mail.example.com"
      zone_id: "Z1234567890ABC"
```

### Full-Featured Configuration
```yaml
schema_version: 17
ses:
  enabled: true

  # Global settings
  enable_mail_from: true
  mail_from_subdomain: "bounce"
  dmarc_policy: "quarantine"
  dmarc_rua_email: "dmarc@example.com"

  # Multiple domains
  domains:
    # Primary domain with auto DNS
    - domain: "mail.example.com"
      zone_id: "Z1234567890ABC"
      test_emails:
        - "test1@example.com"
        - "test2@example.com"

    # Secondary domain with custom DMARC
    - domain: "alerts.example.com"
      zone_id: "Z1234567890ABC"
      dmarc_policy: "reject"
      test_emails:
        - "alerts-test@example.com"

    # External domain - manual DNS
    - domain: "partner.otherdomain.com"
      # No zone_id - requires manual DNS setup
      test_emails:
        - "partner-test@otherdomain.com"
      dmarc_rua_email: "reports@otherdomain.com"
```

### Legacy Configuration (Still Works)
```yaml
schema_version: 16  # Auto-upgrades to 17
ses:
  enabled: true
  domain_name: "mail.example.com"
  test_emails:
    - "test@example.com"
```

---

## Appendix C: Manual DNS Record Format

**Output from `terraform output manual_dns_instructions`:**
```json
{
  "partner.otherdomain.com": {
    "domain": "partner.otherdomain.com",
    "records": [
      {
        "name": "_amazonses.partner.otherdomain.com",
        "type": "TXT",
        "value": "AbCdEfGhIjKlMnOpQrStUvWxYz1234567890",
        "note": "Domain verification record"
      },
      {
        "name": "partner.otherdomain.com",
        "type": "TXT",
        "value": "v=spf1 include:amazonses.com ~all",
        "note": "SPF authorization record"
      },
      {
        "name": "abc123._domainkey.partner.otherdomain.com",
        "type": "CNAME",
        "value": "abc123.dkim.amazonses.com",
        "note": "DKIM record 1/3"
      },
      {
        "name": "def456._domainkey.partner.otherdomain.com",
        "type": "CNAME",
        "value": "def456.dkim.amazonses.com",
        "note": "DKIM record 2/3"
      },
      {
        "name": "ghi789._domainkey.partner.otherdomain.com",
        "type": "CNAME",
        "value": "ghi789.dkim.amazonses.com",
        "note": "DKIM record 3/3"
      },
      {
        "name": "_dmarc.partner.otherdomain.com",
        "type": "TXT",
        "value": "v=DMARC1; p=none; pct=100; rua=mailto:reports@otherdomain.com",
        "note": "DMARC policy record"
      },
      {
        "name": "bounce.partner.otherdomain.com",
        "type": "MX",
        "value": "10 feedback-smtp.us-east-1.amazonses.com",
        "note": "Custom MAIL FROM MX record"
      },
      {
        "name": "bounce.partner.otherdomain.com",
        "type": "TXT",
        "value": "v=spf1 include:amazonses.com ~all",
        "note": "Custom MAIL FROM SPF record"
      }
    ]
  }
}
```

---

**End of Implementation Plan**
