# Comprehensive DNS Setup TUI Implementation Plan

## Overview
Create a complete DNS management system within the meroku TUI application with guided setup, real-time propagation monitoring, and cross-account management.

## Phase 1: DNS Configuration Storage

### 1.1 Create DNS Configuration File Structure
**File: `dns.yaml`** (project root)
```yaml
dns_config:
  root_domain: app.com
  root_account:
    account_id: "123456789012"
    profile: "prod"
    region: "ap-southeast-2"
    zone_id: "Z1234567890ABC"
    delegation_role_arn: "arn:aws:iam::123456789012:role/dns-delegation"
  delegated_zones:
    - subdomain: dev.app.com
      account_id: "098765432109"
      profile: "dev"
      zone_id: "Z0987654321XYZ"
      ns_records: [ns1.aws.com, ns2.aws.com, ns3.aws.com, ns4.aws.com]
      status: "active"
    - subdomain: staging.app.com
      account_id: "111222333444"
      profile: "staging"
      zone_id: "Z111222333444ABC"
      ns_records: [ns1.aws.com, ns2.aws.com, ns3.aws.com, ns4.aws.com]
      status: "pending_propagation"
```

### 1.2 Update Environment Model
**File: `app/model.go`**
Add new structs:
```go
type DNSConfig struct {
    RootDomain   string              `yaml:"root_domain"`
    RootAccount  DNSRootAccount      `yaml:"root_account"`
    DelegatedZones []DelegatedZone   `yaml:"delegated_zones"`
}

type DNSRootAccount struct {
    AccountID        string `yaml:"account_id"`
    Profile          string `yaml:"profile"`
    Region           string `yaml:"region"`
    ZoneID           string `yaml:"zone_id"`
    DelegationRoleArn string `yaml:"delegation_role_arn"`
}

type DelegatedZone struct {
    Subdomain   string   `yaml:"subdomain"`
    AccountID   string   `yaml:"account_id"`
    Profile     string   `yaml:"profile"`
    ZoneID      string   `yaml:"zone_id"`
    NSRecords   []string `yaml:"ns_records"`
    Status      string   `yaml:"status"`
}
```

Update Domain struct:
```go
type Domain struct {
    Enabled          bool   `yaml:"enabled"`
    CreateDomainZone bool   `yaml:"create_domain_zone"`
    DomainName       string `yaml:"domain_name"`
    IsDNSRoot        bool   `yaml:"is_dns_root"`
    DNSRootAccountID string `yaml:"dns_root_account_id"`
    DelegationRoleArn string `yaml:"delegation_role_arn"`
}
```

## Phase 2: DNS TUI Implementation

### 2.1 Main Menu Integration
**File: `app/main_menu.go`**
Add new menu option:
- "Setup Custom Domain (DNS)" - launches DNS setup wizard

### 2.2 Create DNS Setup TUI
**File: `app/dns_setup_tui.go`**

#### Core Components:
1. **DNS Setup Wizard States**:
   - `StateCheckExisting` - Check for existing DNS configuration
   - `StateSelectRootAccount` - Choose which account hosts root domain
   - `StateCreateRootZone` - Create Route53 hosted zone in root account
   - `StateDisplayNameservers` - Show NS records for domain registrar
   - `StateWaitPropagation` - Monitor DNS propagation
   - `StateAddSubdomain` - Add subdomain delegations
   - `StateComplete` - Setup complete

2. **Key Functions**:
```go
type DNSSetupModel struct {
    state           DNSSetupState
    rootDomain      string
    selectedProfile string
    environments    []Environment
    nameservers     []string
    propagationStatus map[string]bool
    spinner         spinner.Model
    progress        progress.Model
}

func (m DNSSetupModel) checkDNSPropagation() tea.Cmd
func (m DNSSetupModel) createRootZone() tea.Cmd
func (m DNSSetupModel) createDelegationRole() tea.Cmd
func (m DNSSetupModel) addSubdomainDelegation() tea.Cmd
func (m DNSSetupModel) pollNameservers() tea.Cmd
```

### 2.3 DNS API Functions
**File: `app/api_dns.go`**

#### Core API Functions:
```go
// Create Route53 hosted zone
func createHostedZone(profile, domain string) (zoneID string, nameservers []string, error)

// Check DNS propagation using multiple DNS servers
func checkDNSPropagation(domain string, expectedNS []string) (map[string]bool, error)

// Create IAM role for cross-account delegation
func createDNSDelegationRole(profile string, trustedAccounts []string) (roleArn string, error)

// Assume role and create NS records in root account
func createNSRecordDelegation(rootProfile, childProfile, rootZoneID, subdomain string) error

// Query nameservers from different locations
func queryNameservers(domain string) ([]string, error)

// Get all AWS accounts from profiles
func getAllAWSAccounts() ([]AccountInfo, error)
```

### 2.4 TUI Flow Screens

#### Screen 1: Welcome & Check Existing
```
╭─────────────────────────────────────────╮
│        DNS Custom Domain Setup          │
├─────────────────────────────────────────┤
│                                         │
│  Welcome to DNS Setup Wizard!          │
│                                         │
│  This wizard will help you:            │
│  • Set up a root domain                │
│  • Configure subdomain delegations     │
│  • Monitor DNS propagation             │
│                                         │
│  Checking for existing configuration...│
│  ⟳ Loading...                          │
│                                         │
│  [Continue]  [Cancel]                  │
╰─────────────────────────────────────────╯
```

#### Screen 2: Select Root Account
```
╭─────────────────────────────────────────╮
│      Select DNS Root Account           │
├─────────────────────────────────────────┤
│                                         │
│  Domain: app.com                       │
│                                         │
│  Which AWS account should host the     │
│  root DNS zone?                        │
│                                         │
│  ◉ prod (123456789012) - Recommended   │
│  ○ dev (098765432109)                  │
│  ○ staging (111222333444)              │
│  ○ Create dedicated DNS account        │
│                                         │
│  The root account will:                │
│  • Own the main domain zone            │
│  • Delegate subdomains to other envs   │
│                                         │
│  [Next]  [Back]  [Cancel]              │
╰─────────────────────────────────────────╯
```

#### Screen 3: Creating Root Zone
```
╭─────────────────────────────────────────╮
│       Creating Root DNS Zone           │
├─────────────────────────────────────────┤
│                                         │
│  Creating zone for: app.com            │
│  Account: prod (123456789012)          │
│                                         │
│  Progress:                              │
│  ✓ Creating hosted zone                │
│  ✓ Retrieving nameservers              │
│  ⟳ Creating delegation IAM role        │
│  ○ Saving configuration                │
│                                         │
│  Nameservers retrieved:                │
│  • ns-1234.awsdns-12.org              │
│  • ns-5678.awsdns-34.co.uk            │
│  • ns-9012.awsdns-56.com              │
│  • ns-3456.awsdns-78.net              │
│                                         │
│  ⟳ Processing...                       │
╰─────────────────────────────────────────╯
```

#### Screen 4: Domain Registrar Instructions
```
╭─────────────────────────────────────────╮
│    Update Domain Registrar             │
├─────────────────────────────────────────┤
│                                         │
│  ⚠️  Action Required!                   │
│                                         │
│  Update your domain registrar with     │
│  these nameservers:                    │
│                                         │
│  ┌─────────────────────────────────┐   │
│  │ ns-1234.awsdns-12.org          │   │
│  │ ns-5678.awsdns-34.co.uk        │   │
│  │ ns-9012.awsdns-56.com          │   │
│  │ ns-3456.awsdns-78.net          │   │
│  └─────────────────────────────────┘   │
│                                         │
│  [📋 Copy to Clipboard]                │
│                                         │
│  Common Registrars:                    │
│  • GoDaddy: DNS > Nameservers          │
│  • Namecheap: Domain > Nameservers     │
│  • Cloudflare: DNS > Nameservers       │
│                                         │
│  [I've Updated Registrar]  [Skip]      │
╰─────────────────────────────────────────╯
```

#### Screen 5: DNS Propagation Monitor
```
╭─────────────────────────────────────────╮
│     Monitoring DNS Propagation         │
├─────────────────────────────────────────┤
│                                         │
│  Checking DNS propagation for app.com  │
│                                         │
│  DNS Servers Status:                   │
│  ✓ Google DNS (8.8.8.8)               │
│  ✓ Cloudflare (1.1.1.1)               │
│  ⟳ Quad9 (9.9.9.9)                    │
│  ✗ OpenDNS (208.67.222.222)           │
│  ⟳ Local ISP                          │
│                                         │
│  Propagation: ⟳ Checking...            │
│                                         │
│  ⏱ Elapsed: 3m 42s                     │
│  📊 Success Rate: 3/5 servers          │
│                                         │
│  💡 DNS propagation typically takes    │
│     5-30 minutes globally              │
│                                         │
│  [Refresh]  [Continue Anyway]  [Wait]  │
╰─────────────────────────────────────────╯
```

#### Screen 6: Add Subdomain Delegations
```
╭─────────────────────────────────────────╮
│    Configure Subdomain Delegations     │
├─────────────────────────────────────────┤
│                                         │
│  Root Zone: app.com (prod account)     │
│                                         │
│  Select environments to delegate:      │
│                                         │
│  ☑ dev.app.com → dev account          │
│  ☑ staging.app.com → staging account  │
│  ☐ demo.app.com → demo account        │
│                                         │
│  Selected delegations will:            │
│  • Create hosted zones in target       │
│  • Add NS records to root zone         │
│  • Configure cross-account access      │
│                                         │
│  Progress:                              │
│  ✓ dev.app.com configured              │
│  ⟳ staging.app.com in progress         │
│                                         │
│  [Add Delegations]  [Skip]  [Back]     │
╰─────────────────────────────────────────╯
```

#### Screen 7: Completion Summary
```
╭─────────────────────────────────────────╮
│      DNS Setup Complete! 🎉            │
├─────────────────────────────────────────┤
│                                         │
│  ✓ Root zone created: app.com          │
│  ✓ DNS propagation verified            │
│  ✓ Delegations configured:             │
│    • dev.app.com                       │
│    • staging.app.com                   │
│                                         │
│  Configuration saved to:                │
│  .meroku-dns.yaml                      │
│                                         │
│  Next Steps:                            │
│  1. Run 'make infra-plan' for each env │
│  2. Apply Terraform changes            │
│  3. Deploy your applications           │
│                                         │
│  DNS Status Dashboard:                  │
│  Run: ./meroku dns status              │
│                                         │
│  [View Config]  [Done]                 │
╰─────────────────────────────────────────╯
```

## Phase 3: Terraform Module Updates

### 3.1 DNS Root Module
**File: `modules/dns-root/main.tf`**
- Create Route53 hosted zone for root domain
- Create IAM role for cross-account delegations
- Output zone ID, nameservers, and role ARN

### 3.2 DNS Delegation Module
**File: `modules/dns-delegation/main.tf`**
- Create subdomain hosted zone
- Use AWS provider with assume_role to root account
- Create NS records in root zone pointing to subdomain
- Handle both creation and updates

### 3.3 Conditional Module Loading
**File: `templates/domain.tf.tmpl`**
```hcl
{{#if domain.is_dns_root}}
module "dns_root" {
  source = "{{modules}}/dns-root"
  domain_name = "{{domain.domain_name}}"
  trusted_account_ids = {{domain.trusted_accounts}}
}
{{else}}
module "dns_delegation" {
  source = "{{modules}}/dns-delegation"
  root_zone_id = "{{domain.root_zone_id}}"
  subdomain = "{{domain.subdomain}}"
  delegation_role_arn = "{{domain.delegation_role_arn}}"
}
{{/if}}
```

## Phase 4: Additional Commands

### 4.1 DNS Status Command
**File: `app/cmd_dns.go`**
```bash
./meroku dns status
```
Shows:
- Current DNS configuration
- Propagation status for all zones
- Health checks for delegations
- TTL information

### 4.2 DNS Validate Command
```bash
./meroku dns validate
```
- Checks all NS records are correct
- Verifies cross-account permissions
- Tests DNS resolution

### 4.3 DNS Remove Command
```bash
./meroku dns remove dev.app.com
```
- Removes subdomain delegation
- Updates configuration file
- Cleans up IAM permissions

## Phase 5: Web UI Updates

### 5.1 DNS Status Component
**File: `web/src/components/DNSHierarchy.tsx`**
- Visual tree showing root and delegated zones
- Real-time propagation status
- Interactive zone management

### 5.2 Route53 Component Updates
**File: `web/src/components/Route53DNSRecords.tsx`**
- Show delegation relationships
- Display root account indicator
- Add delegation management buttons

## Phase 6: Error Handling & Recovery

### 6.1 Common Error Scenarios
- Root zone already exists
- Permission denied for cross-account
- DNS propagation timeout
- Conflicting NS records

### 6.2 Recovery Mechanisms
- Rollback partial configurations
- Force refresh DNS cache
- Manual NS record override
- Configuration reset option

## Phase 7: Testing & Validation

### 7.1 Unit Tests
- DNS API functions
- Configuration file parsing
- Cross-account role assumption

### 7.2 Integration Tests
- End-to-end DNS setup flow
- Multi-account scenarios
- Propagation monitoring

## Implementation Priority

1. **High Priority** (Week 1):
   - DNS configuration model and storage
   - Basic TUI wizard flow
   - Root zone creation
   - NS record display

2. **Medium Priority** (Week 2):
   - DNS propagation monitoring
   - Cross-account delegations
   - IAM role management
   - Configuration persistence

3. **Low Priority** (Week 3):
   - Web UI components
   - Advanced error handling
   - DNS health monitoring
   - Automated testing

## File Structure Summary

```
app/
├── dns_setup_tui.go      # Main DNS setup TUI
├── dns_api.go            # DNS API functions
├── dns_config.go         # Configuration management
├── dns_propagation.go    # Propagation monitoring
├── cmd_dns.go           # DNS CLI commands
└── model.go             # Updated with DNS structs

modules/
├── dns-root/            # Root zone module
└── dns-delegation/      # Delegation module

templates/
└── domain.tf.tmpl       # Updated domain template

web/src/components/
├── DNSHierarchy.tsx     # DNS visual hierarchy
└── Route53DNSRecords.tsx # Updated with delegation info
```

## Configuration Files

1. `.meroku-dns.yaml` - Project-level DNS configuration
2. `dev.yaml`, `prod.yaml` - Environment configs with DNS settings
3. `.gitignore` - Exclude `.meroku-dns.yaml` if contains sensitive data

This comprehensive plan provides a complete DNS management solution integrated into your existing meroku TUI application, with guided setup, real-time monitoring, and cross-account management capabilities.