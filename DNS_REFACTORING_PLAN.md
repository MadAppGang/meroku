# DNS Management Refactoring - Step-by-Step Implementation Plan

## Implementation Instructions

⚠️ **IMPORTANT**: This implementation must be done step-by-step with validation after each step.

**Process:**
1. Complete each step fully
2. Test the implementation
3. Manually validate it works as expected
4. Only proceed to next step after confirmation
5. If issues found, fix before continuing

**Validation Command**: After each step, tell Claude: "Step X validated, proceed to Step Y"

---

## Current Understanding

### Existing Domain Pattern (DO NOT BREAK)
- `domain.domain_name` is ALWAYS the root domain (e.g., "example.com") for ALL environments
- Environment prefixes are added by the domain module based on `add_env_domain_prefix` flag
- prod: `example.com` (add_env_domain_prefix = false)
- dev: `dev.example.com` (add_env_domain_prefix = true)
- staging: `staging.example.com` (add_env_domain_prefix = true)

---

## Step 1: Update Domain Model Structure

### Files to modify:
- `app/model.go`

### Changes:
Add new DNS-related fields to Domain struct WITHOUT changing existing fields:

```go
type Domain struct {
    // EXISTING FIELDS - DON'T TOUCH
    Enabled            bool   `yaml:"enabled"`
    DomainName         string `yaml:"domain_name"`  // Keep as-is - always root
    CreateDomainZone   bool   `yaml:"create_domain_zone"`
    APIDomainPrefix    string `yaml:"api_domain_prefix,omitempty"`
    AddEnvDomainPrefix bool   `yaml:"add_env_domain_prefix,omitempty"`
    
    // NEW DNS MANAGEMENT FIELDS
    ZoneID         string `yaml:"zone_id,omitempty"`         // For existing zones
    RootZoneID     string `yaml:"root_zone_id,omitempty"`    // For subdomain delegation
    RootAccountID  string `yaml:"root_account_id,omitempty"` // For cross-account access
}
```

### Validation:
1. Run: `go build ./app`
2. Verify compilation succeeds
3. Check existing YAML files still parse correctly

**✅ Checkpoint**: Confirm model compiles and existing configs still load

---

## Step 2: Create DNS Configuration Management

### Files to create:
- `app/dns_config.go` (already exists - update it)

### Changes:
Ensure dns.yaml structure matches our needs:

```go
type DNSConfig struct {
    RootDomain     string           `yaml:"root_domain"`
    RootAccount    DNSRootAccount   `yaml:"root_account"`
    DelegatedZones []DelegatedZone  `yaml:"delegated_zones"`
}

type DNSRootAccount struct {
    AccountID string `yaml:"account_id"`
    ZoneID    string `yaml:"zone_id"`
    // Note: No delegation role ARN stored - constructed as needed
}

type DelegatedZone struct {
    Subdomain  string   `yaml:"subdomain"`
    AccountID  string   `yaml:"account_id"`
    ZoneID     string   `yaml:"zone_id"`
    NSRecords  []string `yaml:"ns_records"`
    Status     string   `yaml:"status"`
}
```

### Validation:
1. Test loading/saving dns.yaml
2. Verify functions work: `loadDNSConfig()`, `saveDNSConfig()`

**✅ Checkpoint**: DNS config file operations work correctly

---

## Step 3: Update DNS Setup TUI - Part 1 (Root Zone Creation)

### Files to modify:
- `app/dns_setup_tui.go`

### Changes:
1. Force production account selection for root zone
2. Create root zone in production account
3. Save to dns.yaml
4. Create/update prod.yaml with zone info

### Key Functions to Update:
```go
func (m DNSSetupModel) createRootZone() tea.Cmd {
    // Must use prod profile
    // Create zone
    // Create delegation IAM role (fixed name: "dns-delegation")
    // Save to dns.yaml
}

func ensureProductionEnvironment(rootDomain, zoneID, accountID string) error {
    // Create prod.yaml if missing
    // Set domain_name = rootDomain
    // Set zone_id = zoneID
    // Set create_domain_zone = false
    // Set add_env_domain_prefix = false
}
```

### Validation:
1. Run DNS setup from menu
2. Verify it forces prod account selection
3. Check root zone created in AWS
4. Verify prod.yaml created/updated correctly
5. Confirm dns.yaml has root zone info

**✅ Checkpoint**: Root zone creation works, prod.yaml configured

---

## Step 4: Update DNS Setup TUI - Part 2 (Environment Propagation)

### Files to modify:
- `app/dns_setup_tui.go`

### Changes:
Propagate root zone info to all environment files:

```go
func propagateRootZoneInfo(dnsConfig *DNSConfig) error {
    envFiles := []string{"dev.yaml", "staging.yaml"}
    
    for _, envFile := range envFiles {
        path := fmt.Sprintf("project/%s", envFile)
        
        // Create if doesn't exist
        env := loadOrCreateEnvironment(path)
        
        // CRITICAL: Keep domain_name as root domain!
        env.Domain.DomainName = dnsConfig.RootDomain
        env.Domain.Enabled = true
        env.Domain.CreateDomainZone = true  // Will create subdomain
        env.Domain.AddEnvDomainPrefix = true  // This makes it dev.example.com
        
        // Add DNS info
        env.Domain.RootZoneID = dnsConfig.RootAccount.ZoneID
        env.Domain.RootAccountID = dnsConfig.RootAccount.AccountID
        
        saveEnvironment(path, env)
    }
}
```

### Validation:
1. After DNS setup completes, check dev.yaml and staging.yaml
2. Verify domain_name is root domain (e.g., "example.com")
3. Confirm root_zone_id and root_account_id are populated
4. Ensure add_env_domain_prefix is true for non-prod

**✅ Checkpoint**: All environment files have correct DNS configuration

---

## Step 5: Create DNS Delegation Terraform Module

### Files to create:
- `modules/dns-delegation/main.tf`
- `modules/dns-delegation/variables.tf`
- `modules/dns-delegation/outputs.tf`

### Module Implementation:
```hcl
# main.tf
resource "aws_route53_zone" "subdomain" {
  name = var.subdomain
}

provider "aws" {
  alias = "root"
  assume_role {
    role_arn = "arn:aws:iam::${var.root_account_id}:role/dns-delegation"
  }
}

resource "aws_route53_record" "ns_delegation" {
  provider = aws.root
  zone_id  = var.root_zone_id
  name     = var.subdomain
  type     = "NS"
  ttl      = 300
  records  = aws_route53_zone.subdomain.name_servers
}

output "zone_id" {
  value = aws_route53_zone.subdomain.zone_id
}
```

### Validation:
1. Run terraform init in a test environment
2. Verify module syntax is correct

**✅ Checkpoint**: DNS delegation module created and validates

---

## Step 6: Update Handlebars Template

### Files to modify:
- `env/main.hbs`

### Changes:
Update the domain module section to handle DNS zones properly:

```handlebars
{{#if domain.enabled}}
  {{#compare env "==" "prod"}}
    # PROD: Use existing root zone
    data "aws_route53_zone" "main" {
      zone_id = "{{ domain.zone_id }}"
    }
    
    module "domain" {
      source = "{{ modules }}/domain"
      # ... existing config ...
    }
  {{else}}
    {{#if domain.root_zone_id}}
      # NON-PROD: Create subdomain with delegation
      module "dns_delegation" {
        source = "{{ modules }}/dns-delegation"
        subdomain = "{{ env }}.{{ domain.domain_name }}"
        root_zone_id = "{{ domain.root_zone_id }}"
        root_account_id = "{{ domain.root_account_id }}"
      }
    {{/if}}
    
    module "domain" {
      source = "{{ modules }}/domain"
      # ... existing config ...
    }
  {{/compare}}
{{/if}}
```

### Validation:
1. Run `make infra-gen-dev`
2. Check generated Terraform files
3. Verify DNS delegation module is included for dev
4. Confirm prod uses data source for zone

**✅ Checkpoint**: Terraform generation includes DNS management

---

## Step 7: Test End-to-End Flow

### Test Sequence:
1. Clean start - remove any existing dns.yaml
2. Run DNS setup from menu
3. Enter test domain
4. Select prod account
5. Wait for completion

### Expected Results:
- Root zone created in prod account
- IAM role "dns-delegation" created
- prod.yaml has zone_id
- dev.yaml has root_zone_id and root_account_id
- dns.yaml contains root zone info

### Terraform Test:
1. Run `make infra-gen-dev`
2. Run `make infra-plan env=dev`
3. Verify plan shows:
   - Subdomain zone creation
   - NS record delegation

**✅ Checkpoint**: Complete flow works end-to-end

---

## Step 8: Add DNS Status Command

### Files to modify:
- `app/cmd_dns.go`

### Implementation:
```go
func cmdDNSStatus() error {
    // Load dns.yaml
    // Show root zone info
    // List configured environments
    // Check DNS propagation
}
```

### Validation:
1. Run `./meroku` and select DNS status
2. Verify shows current configuration
3. Check propagation status works

**✅ Checkpoint**: DNS status command provides useful information

---

## Step 9: Documentation Update

### Files to update:
- `CLAUDE.md` - Add DNS management section
- `README.md` - Document DNS setup process

### Content to Add:
- DNS setup workflow
- Environment configuration pattern
- Troubleshooting guide

**✅ Checkpoint**: Documentation complete

---

## Step 10: Final Integration Test

### Complete Test:
1. Start fresh (clean environment)
2. Set up new project with DNS
3. Deploy to dev environment
4. Verify DNS records created
5. Test subdomain access

### Success Criteria:
- DNS zones created correctly
- Delegation works
- Services accessible via configured domains
- No breaking changes to existing functionality

**✅ Final Checkpoint**: System fully operational

---

## Rollback Plan

If issues occur at any step:
1. Revert code changes for that step
2. Clean up any created AWS resources
3. Restore original configuration files
4. Document the issue
5. Adjust plan before retry

---

## Notes

- Always backup existing configs before changes
- Test in dev account first
- Monitor AWS costs (Route53 zones)
- Keep delegation role name consistent ("dns-delegation")
- Preserve existing domain.domain_name behavior (always root)