# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a comprehensive Terraform Infrastructure as Code (IaC) repository that provides a reusable, modular AWS infrastructure setup. It includes:

- Terraform modules for AWS services (20+ modules including ECS, RDS, ALB, Cognito, Lambda, etc.)
- Go CLI application ("meroku") for interactive infrastructure management using Bubble Tea TUI framework
- React+TypeScript web frontend for visual infrastructure management
- Handlebars-based templating system (using Raymond Go package) for environment configuration
- GitHub Actions CI/CD workflows with OIDC authentication

## Template and Configuration Management Strategy

### Dual Approach: Migrations + Default Helper

We use **both** migrations and the `default` Handlebars helper for managing configuration values:

#### When to Use Migrations

**Use migrations for core infrastructure fields that should always be explicit in YAML:**

```go
// migrations.go - Example v12
booleanDefaults := map[string]interface{}{
    "multi_az":                             false,
    "storage_encrypted":                    true,
    "iam_database_authentication_enabled":  false,
}
```

**Template:**
```handlebars
multi_az = {{postgres.multi_az}}
storage_encrypted = {{postgres.storage_encrypted}}
```

**Benefits:**
- YAML is self-documenting (all values visible)
- Single source of truth for defaults (migration code)
- Type safety at load time
- Fields always exist when template renders

**Use for:**
- Core infrastructure booleans (security, HA, encryption)
- Required fields that should never be missing
- Fields where explicit values improve clarity

#### When to Use Default Helper

**Use `default` helper for optional fields with sensible fallbacks:**

```handlebars
instance_class = "{{default postgres.instance_class "db.t4g.micro"}}"
allocated_storage = {{default postgres.allocated_storage 20}}
backend_cpu = "{{default workload.backend_cpu "256"}}"
```

**Benefits:**
- Defaults visible in template (documentation)
- Less YAML clutter for optional fields
- Flexibility to override when needed
- Works correctly with `false`, `0`, `0.0` (fixed in our implementation)

**Use for:**
- Optional configuration with sensible defaults
- Fields that vary by use case
- Non-critical settings

#### Default Helper Behavior

The `default` helper in `app/raymond.go` has been fixed to handle all types correctly:

- `nil` (field missing from YAML) → returns default
- `false` → returns `false` (valid value)
- `0` → returns `0` (valid value)
- `0.0` → returns `0.0` (valid value)
- `""` (empty string) → returns default
- `[]` (empty array) → returns default

**Example:**
```yaml
# YAML
postgres:
  min_capacity: 0  # Explicitly set to 0 (valid for Aurora pausing)

# Template
min_capacity = {{default postgres.min_capacity 0.5}}

# Result: 0 (uses explicit value, not default)
```

#### When to Use Exists Helper

**Use `exists` helper when you need to distinguish "value is 0/false" from "value is missing":**

```handlebars
{{!-- Aurora capacity: 0 is valid (pause when idle), distinguish from missing --}}
{{#if (exists postgres.min_capacity)}}
  min_capacity = {{postgres.min_capacity}}
{{/if}}
```

**Why `exists` vs `{{#if}}`:**
- `{{#if value}}` checks **truthiness** → 0, false, "", [] are all falsy
- `{{#if (exists value)}}` checks **presence** → only nil is false

**Use cases:**
- Aurora Serverless v2 capacity where 0 means "pause when idle"
- Numeric fields where 0 is a valid configuration
- Boolean fields where false needs different handling than missing

**Pattern comparison:**

```handlebars
{{!-- OLD PATTERN (complex, hard to read) --}}
{{#if (or min_capacity (eq min_capacity 0))}}
  min_capacity = {{min_capacity}}
{{/if}}

{{!-- NEW PATTERN (simple, clear intent) --}}
{{#if (exists min_capacity)}}
  min_capacity = {{min_capacity}}
{{/if}}
```

**Other available helpers:**
- `or` - Logical OR: `{{#if (or a b)}}` (checks truthiness)
- `eq` - Equality: `{{#if (eq value 0)}}` (compares values)
- `default` - Fallback values: `{{default value 10}}`

**Tests:** All helpers have comprehensive test coverage in `app/raymond_test.go`

## Important Architecture Decisions

### VPC Configuration

**Default VPC Strategy**: New projects use **custom VPCs with 2 public subnets** for ultimate simplicity and cost optimization.

Key decisions:
- **Custom VPC by default** (`use_default_vpc: false`) - Better isolation and control
- **Hardcoded to 2 AZs** - Covers 99% of use cases, minimum for HA
- **Public subnets only** - All resources are internet-accessible via Internet Gateway
- **NO private subnets** - Removed from codebase (keeps architecture simple)
- **NO NAT Gateway** - Removed from codebase (not needed, saves ~$32/month)
- **NO AZ count option** - Removed from codebase (hardcoded to 2)

Configuration options (minimal):
- `use_default_vpc`: `true` (use AWS default VPC) or `false` (create custom VPC)
- `vpc_cidr`: Optional CIDR block for custom VPC (defaults to "10.0.0.0/16")

This architecture is the simplest possible while maintaining high availability:
- **2 Availability Zones** - Minimum for HA, covers regional failures
- **Public subnets only** - Direct internet access, no NAT overhead
- **Security groups** - Proper access control without network complexity
- **Cost-effective** - No NAT Gateway (~$32/month saved), no VPC endpoints (~$27/month saved)

Sufficient for most use cases where:
- ECS tasks need direct internet access
- RDS can use security groups for access control
- No strict requirement for private subnet isolation

**Why 2 AZs is hardcoded:**
- 2 AZs is the minimum for high availability
- Handles single AZ failure (most common outage scenario)
- 3 AZs adds cost with marginal benefit for 99% of applications
- Keeps configuration simple - one less thing to think about
- Power users can modify the VPC module directly if needed

**Migration Note**:
- Existing projects migrating from before schema v6 will keep `use_default_vpc: true` for backward compatibility
- The migration automatically removes deprecated fields: `az_count`, `create_private_subnets`, `enable_nat_gateway`

### VPC Endpoints (Deprecated)

**Note**: VPC endpoints are NO LONGER USED in this infrastructure due to cost considerations (~$27/month per interface endpoint). Instead, we rely on:

- Security groups for access control
- Internet Gateway for outbound connectivity
- Service-to-service communication through the VPC

All VPC endpoint code in `modules/workloads/ecs_endpoints.tf` is commented out and should remain so.

### API Gateway vs ALB

The infrastructure supports two ingress patterns:

- **Default (enable_alb: false)**: API Gateway → ECS Services
- **Alternative (enable_alb: true)**: ALB → ECS Services

Note: Currently, both resources are created regardless of the setting, but only one is used for traffic routing.

## AI Agent

This infrastructure includes an **autonomous AI agent** that can investigate and fix deployment errors automatically using the ReAct pattern (Reasoning + Acting).

### When to Use

1. **Automatic**: After terraform apply/destroy failures, you'll be prompted to run the agent
2. **Manual**: Select "🤖 AI Agent - Troubleshoot Issues" from the main menu

### How It Works

The agent uses an iterative debugging approach:
- **Think**: Analyzes the situation and decides what to do next
- **Act**: Executes commands (AWS CLI, file edits, terraform)
- **Observe**: Reviews results and adapts strategy
- **Repeat**: Continues until problem is solved or max iterations reached

### Documentation

- [AI Agent Architecture](./ai_docs/AI_AGENT_ARCHITECTURE.md) - Technical design and implementation
- [AI Agent User Guide](./ai_docs/AI_AGENT_USER_GUIDE.md) - How to use the agent effectively

### Requirements

```bash
export ANTHROPIC_API_KEY=your_key_here
```

Get your API key from: https://console.anthropic.com/settings/keys

## Memories

- Always keep all AI-related documentation, created by AI or intended to be consumed by AI, in the @ai_docs/ folder

## Common Commands

### Infrastructure Management

```bash
# Initialize Terraform (run after creating env/*.tf files)
make infra-init env=dev

# Update Terraform modules
make infra-update env=dev

# Plan infrastructure changes
make infra-plan env=dev

# Apply infrastructure changes
make infra-apply env=dev

# Destroy infrastructure
make infra-destroy env=dev

# Show current infrastructure state
make infra-show env=dev

# Generate Terraform files from YAML config
make infra-gen-dev    # For dev environment
make infra-gen-prod   # For prod environment

# Import existing AWS resources
make infra-import env=dev
```

### Development Commands

```bash
# Run the TUI application
make tui

# Run the web frontend
make web

# Build the CLI
make build

# Run tests
make test

# Generate code from templates
make generate

# Test terraform plan diff viewer (for debugging)
./meroku --renderdiff terraform-plan.json
```

## Project Structure

```
infrastructure/
├── modules/          # Terraform modules for AWS services
├── env/             # Environment-specific Terraform configurations
├── project/         # YAML configuration files (dev.yaml, prod.yaml)
├── templates/       # Handlebars templates for Terraform generation
├── app/            # Go CLI application (meroku)
├── web/            # React+TypeScript frontend
└── scripts/        # Utility scripts
```

## Key Configuration Files

- `project/dev.yaml` - Development environment configuration
- `project/prod.yaml` - Production environment configuration
- `env/dev/*.tf` - Generated Terraform files for dev (DO NOT EDIT MANUALLY)
- `env/prod/*.tf` - Generated Terraform files for prod (DO NOT EDIT MANUALLY)

## Working with the Codebase

1. **Making Infrastructure Changes**: Edit YAML files in `project/`, then run `make infra-gen-{env}`
2. **Adding New Services**: Update the `services` array in YAML configuration
3. **Modifying Terraform Modules**: Edit files in `modules/` directory
4. **Updating Templates**: Modify Handlebars templates in `templates/`

## Testing Guidelines

- Always run `make infra-plan env={env}` before applying changes
- Test infrastructure changes in dev environment first
- Use `make test` to run Go tests
- Frontend tests: `cd web && npm test`

## Terraform Plan Viewer

The meroku CLI includes an advanced Terraform plan viewer with the following features:

- Visual tree view of resources organized by provider and service
- Detailed attribute diff display with proper formatting
- Support for resource replacements (shows both delete and create phases)
- Scrollable detail views for reviewing all changes
- Color-coded changes (green for create, yellow for update, red for delete)

To test the plan viewer with a JSON file:

```bash
./meroku --renderdiff path/to/terraform-plan.json
```

The viewer properly handles:

- Replace operations by showing both delete and create as separate items
- Complex nested attributes and arrays
- Long strings and multi-line values
- Null values and empty collections

## DNS Management

The infrastructure includes a comprehensive DNS management system that handles cross-account zone delegation:

### DNS Setup Workflow

1. **Initial Setup**: Run `./meroku` and select "DNS Setup" from the menu
2. **Root Zone Creation**: The wizard creates the root zone in the production account
3. **Automatic Configuration**: Environment files (dev.yaml, prod.yaml, staging.yaml) are automatically updated
4. **Subdomain Delegation**: Non-production environments get delegated subdomains (dev.example.com, staging.example.com)

### DNS Commands

```bash
# Run interactive DNS setup wizard
./meroku
# Then select "DNS Setup" from menu

# Check DNS configuration status
./meroku dns status

# Validate DNS propagation
./meroku dns validate

# Remove subdomain delegation
./meroku dns remove <subdomain>
```

### DNS Architecture

- **Root Zone**: Created in production account with delegation IAM role
- **Subdomain Zones**: Created in respective environment accounts
- **Cross-Account Access**: Uses IAM role assumption for NS record management
- **Automatic Delegation**: NS records are automatically created in root zone

### Configuration Files

- `dns.yaml`: Stores root zone information and delegated zones
- `project/*.yaml`: Environment files contain zone IDs and delegation info
- Domain names ALWAYS use root domain (e.g., "example.com") in all environments
- Environment prefixes are added automatically based on `add_env_domain_prefix` flag

### Documentation

For comprehensive DNS management details, refer to:
- [DNS Architecture Design](./docs/DNS_ARCHITECTURE.md) - System design and architecture documentation
- [DNS Management Instructions](./DNS_MANAGEMENT_INSTRUCTIONS.md) - Step-by-step operational guide

## Cross-Account ECR Configuration

The infrastructure supports flexible ECR strategies for container image management with intelligent cross-account access.

### ECR Strategies

1. **Local Strategy** (`ecr_strategy: local`):
   - Creates ECR repositories in the same AWS account
   - Each environment manages its own container registry
   - Best for isolated environments or single-account setups

2. **Cross-Account Strategy** (`ecr_strategy: cross_account`):
   - Pulls container images from another AWS account's ECR
   - Ideal for dev→staging→prod promotion pipelines
   - Reduces image duplication and build costs

### Automated Cross-Account Setup

The web UI provides an intelligent dropdown for configuring cross-account ECR access:

1. **Automatic Discovery**: Lists all environments with local ECR repositories
2. **Deployment Status**: Shows whether trust policies are deployed to AWS
3. **Bidirectional Updates**: Automatically updates both source and target YAML files
4. **Trust Policy Management**: Creates ECR repository policies for cross-account pull access

### Configuration Workflow

#### Via Web UI (Recommended)

1. Navigate to the ECR configuration node in the web interface
2. Select "Use Cross-Account ECR" mode
3. Choose a source environment from the dropdown
4. The system will automatically:
   - Update the target environment to use cross-account mode
   - Add the target to the source's trusted accounts list
   - Create timestamped backups of both YAML files
5. Follow the displayed next steps to deploy

#### Manual Configuration

In `project/staging.yaml` (target environment):
```yaml
schema_version: 8
ecr_strategy: cross_account
ecr_account_id: "123456789012"  # Source account ID
ecr_account_region: us-east-1   # Source region
```

In `project/dev.yaml` (source environment):
```yaml
schema_version: 8
ecr_strategy: local
ecr_trusted_accounts:
  - account_id: "987654321098"  # Target account ID
    env: staging
    region: us-east-1
```

### Deployment Status Indicators

The web UI shows real-time deployment status:

- **Warning** (Yellow): Trust policy configured but not deployed to AWS
  - Action: Run `make infra-apply env=<source>`
- **Success** (Green): Trust policy deployed and cross-account access ready
- **Info** (Blue): Configuration changes will update both YAML files

### ECR Trust Policies

When `ecr_trusted_accounts` is configured, Terraform creates repository policies allowing pull access:

```hcl
# Automatically generated in modules/workloads/ecr.tf
resource "aws_ecr_repository_policy" "backend_trusted" {
  # Allows same-account full access
  # Allows cross-account pull-only access for trusted accounts
}
```

### CI/CD with Cross-Account ECR

The recommended deployment pattern uses EventBridge for event-driven deployments:

1. **Dev environment**: Builds and pushes images to local ECR
2. **EventBridge event**: Triggers deployment pipeline
3. **Staging/Prod**: Pulls images from dev account's ECR
4. **Deployment**: ECS services use cross-account images

See [CI/CD EventBridge Pattern](./docs/CI_CD_EVENTBRIDGE_PATTERN.md) for detailed implementation.

### Security Considerations

- Trust policies grant **pull-only access** (no push permissions)
- Access is scoped to specific AWS accounts and regions
- IAM permissions follow least-privilege principle
- All configuration changes create timestamped backups

### Troubleshooting

**Issue**: Cross-account ECR pull fails
- Verify trust policy is deployed: Check web UI deployment status
- Confirm account IDs match: Compare source and target YAML files
- Check ECS task role: Ensure it has ECR pull permissions

**Issue**: No ECR sources in dropdown
- Verify at least one environment has `ecr_strategy: local`
- Ensure environments are configured with `account_id` and `region`
- Check that YAML files are in the project directory

## YAML Schema Migration System

The infrastructure includes an automatic migration system for YAML configuration files to ensure backward compatibility as the schema evolves.

### Migration Features

- **Automatic Migration**: YAML files are automatically migrated when loaded
- **Backup Creation**: Creates timestamped backups before making changes
- **Version Tracking**: Tracks schema version in `schema_version` field
- **Safe Migrations**: Only adds new fields, never modifies existing data

### Migration Commands

```bash
# Migrate all YAML files in project directory
./meroku migrate all

# Migrate a specific file
./meroku migrate dev.yaml

# Show migration help and current version
./meroku migrate
```

### Current Schema Version

**Version 8** - Includes:
- Aurora Serverless v2 support (v2)
- DNS management fields (v3)
- Backend scaling configuration (v4)
- Account ID and AWS profile tracking (v5)
- Custom VPC configuration (v6)
- ECR strategy configuration (v7)
- ECR trusted accounts for cross-account access (v8)

### How It Works

When you load a YAML file, the system:
1. Detects the current schema version
2. Creates a timestamped backup if migration needed
3. Applies all necessary migrations sequentially
4. Updates the `schema_version` field
5. Saves the migrated file

Example backup: `dev.yaml.backup_20251015_211246`

### Documentation

For detailed migration information, refer to:
- [YAML Schema Migrations](./ai_docs/MIGRATIONS.md) - Complete migration system documentation

## Security Considerations

- Never commit secrets or credentials
- Use AWS SSM Parameter Store for sensitive values
- Security groups control service access (no VPC endpoints needed)
- Enable encryption at rest for all data stores
- Use IAM roles for service authentication

## Cost Optimization

- VPC endpoints are disabled to save costs
- Use appropriate instance sizes for ECS tasks
- Enable auto-scaling where appropriate
- Monitor CloudWatch costs (logs retention is set to 30 days)
- when we bump version number,we need to create tag and change version.txt file content
-

# Codebase Navigation Rule

**ALWAYS use CodebaseDetective subagent when finding or locating code.**

When you need to:

- Find any function, class, or implementation
- Locate endpoints, configs, or specific logic
- Understand code flow or dependencies

Do this:

1. Activate Detective mode: `[Detective Mode: Finding X]`
2. Index: `index_codebase` (if MCP available)
3. Search: `search_code with query: "semantic description"`
4. Fallback to grep/find only if MCP unavailable

Never manually browse files randomly. Always use Detective for systematic code navigation.
- or app is located in app folder and web interface it serve in web app.