# YAML Configuration Validation Rules Catalog

**Last Updated:** 2025-01-24
**Purpose:** Complete catalog of all validation rules for Meroku infrastructure YAML configurations

---

## Table of Contents

1. [Overview](#overview)
2. [Validation Patterns Summary](#validation-patterns-summary)
3. [Complete Rules by Configuration Section](#complete-rules-by-configuration-section)
4. [Implementation Recommendations](#implementation-recommendations)
5. [Code Examples](#code-examples)

---

## Overview

This document catalogs all validation rules needed for Meroku's YAML configuration files (`dev.yaml`, `prod.yaml`, etc.) before they are processed by Terraform. The goal is to catch configuration errors early with clear, actionable error messages.

### Current State
- ✅ Basic ECR validation implemented in `app/validation.go`
- ❌ Most validation is missing or happens too late (in Terraform)
- ❌ No validation for cross-field dependencies
- ❌ Limited error messages

### Validation Complexity Required
- **Allowed Values (Enums)**: 20+ fields
- **Conditional Required**: 15+ rules
- **Cross-Field Dependencies**: 10+ rules
- **Format Validation (Regex)**: 12+ patterns
- **Cross-Entity Validation**: 3 critical rules
- **Range Validation**: 8+ numeric constraints
- **Uniqueness Checks**: 5 entity types
- **Complex Conditional Logic**: 6 scenarios

---

## Validation Patterns Summary

| Pattern Type | Count | Complexity | Example |
|--------------|-------|------------|---------|
| **Enum Values** | 20+ | Low | `ecr_strategy: "local" or "cross_account"` |
| **Required Fields** | 30+ | Low | `project`, `region`, `account_id` |
| **Conditional Required** | 15+ | Medium | `IF ecr_strategy == "cross_account" THEN ecr_account_id required` |
| **Format Validation** | 12+ | Medium | ECR URI regex, domain format, email format |
| **Cross-Field** | 10+ | High | `max_capacity >= min_capacity` |
| **Cross-Entity** | 3 | High | Source service must exist with correct ECR mode |
| **Range Constraints** | 8+ | Low | `port: 1-65535`, `account_id: 12 digits` |
| **Uniqueness** | 5 | Medium | Service names, bucket names must be unique |

---

## Complete Rules by Configuration Section

### 1. ENV-LEVEL CONFIGURATION

#### Required Fields
```yaml
project: string          # Non-empty
env: string             # Non-empty
region: string          # Valid AWS region format
account_id: string      # 12 digits when set
state_bucket: string    # Non-empty
state_file: string      # Optional, defaults to "state.tfstate"
```

#### Format Validation
| Field | Pattern | Example |
|-------|---------|---------|
| `region` | `^[a-z]{2}-[a-z]+-\d$` | `us-east-1`, `ap-southeast-2` |
| `account_id` | `^\d{12}$` | `123456789012` |
| `vpc_cidr` | Valid CIDR notation | `10.0.0.0/16` |

#### Allowed Values
```yaml
ecr_strategy: ["local", "cross_account"]  # Required
use_default_vpc: boolean
```

#### Conditional Validation: ECR Cross-Account
```
IF ecr_strategy == "cross_account" THEN:
  ✓ ecr_account_id: REQUIRED (12 digits)
  ✓ ecr_account_region: REQUIRED (valid AWS region)
```

---

### 2. ECR CONFIG (Services/Tasks/Event Processors)

**Location:** `services[].ecr_config`, `scheduled_tasks[].ecr_config`, `event_processor_tasks[].ecr_config`

#### Allowed Values
```yaml
mode: ["create_ecr", "manual_repo", "use_existing"]  # Defaults to "create_ecr"
```

#### Conditional: Mode = "manual_repo"
```
IF mode == "manual_repo" THEN:
  ✓ repository_uri: REQUIRED
  ✓ repository_uri: MUST match ECR URI format

ECR URI Pattern:
  ^\d{12}\.dkr\.ecr\.[a-z0-9-]+\.amazonaws\.com\/[a-zA-Z0-9_-]+$

Example:
  123456789012.dkr.ecr.us-east-1.amazonaws.com/my-repo
```

#### Conditional: Mode = "use_existing"
```
IF mode == "use_existing" THEN:
  ✓ source_service_name: REQUIRED, non-empty
  ✓ source_service_type: REQUIRED, one of:
    - "services"
    - "event_processor_tasks"
    - "scheduled_tasks"

  CROSS-ENTITY VALIDATION:
  ✓ Source service MUST exist in the specified collection
  ✓ Source service's ECR config mode MUST be "create_ecr"

Error Example:
  "service 'api': source service 'backend' not found in services"
  "service 'api': source service 'backend' must have ecr_config.mode='create_ecr', but has mode='manual_repo'"
```

#### Implementation Note
This is the MOST COMPLEX validation in the system because it requires:
1. Cross-entity lookup (find source service in different collections)
2. Nested validation (check source service's ECR config)
3. Circular dependency prevention

---

### 3. SERVICES

#### Required Fields
```yaml
name: string              # Unique across all services
container_port: int       # 1-65535
cpu: int                  # > 0
memory: int               # > 0
desired_count: int        # >= 0
```

#### Optional Fields with Defaults
```yaml
host_port: int            # Defaults to container_port
remote_access: boolean    # Defaults to false
xray_enabled: boolean     # Defaults to false
essential: boolean        # Defaults to true
docker_image: string      # Optional
container_command: array  # Optional
```

#### Format Validation
```yaml
env_vars: map[string]string        # Key-value pairs
env_variables: array               # Array of {name, value}
env_files_s3: array                # Array of {bucket, key}
```

#### Cross-Field Validation
```
IF host_port is specified THEN:
  ✓ host_port >= container_port
```

#### Uniqueness
```
Service names MUST be unique across:
  - services[]
  - scheduled_tasks[]
  - event_processor_tasks[]
```

---

### 4. SCHEDULED TASKS

#### Required Fields
```yaml
name: string              # Unique
schedule: string          # Valid EventBridge schedule expression
```

#### Schedule Format Validation
Must match ONE of:
```
rate(N minutes|hours|days)
  Examples: rate(1 minutes), rate(5 hours)

cron(minute hour day-of-month month day-of-week year)
  Examples: cron(0 12 * * ? *), cron(15 10 ? * MON-FRI *)
```

Pattern: `^(rate\(\d+ (minute|minutes|hour|hours|day|days)\)|cron\(.+\))$`

#### Optional Fields
```yaml
docker_image: string              # External Docker image
container_command: string         # Container command
ecr_config: ECRConfig            # See section #2
```

---

### 5. EVENT PROCESSOR TASKS

#### Required Fields
```yaml
name: string              # Unique
rule_name: string         # Non-empty
detail_types: array       # Non-empty array of strings
sources: array            # Non-empty array of strings
```

#### Optional Fields
```yaml
docker_image: string              # External Docker image
container_command: array          # Container command
ecr_config: ECRConfig            # See section #2
```

#### Array Validation
```
detail_types:
  ✓ Must be non-empty array
  ✓ Each element must be non-empty string

sources:
  ✓ Must be non-empty array
  ✓ Each element must be valid service name OR AWS service

Examples:
  - service1
  - service2
  - aws.ecs
  - aws.ec2
```

---

### 6. POSTGRES CONFIGURATION

#### Required When Enabled
```
IF postgres.enabled == true THEN:
  ✓ dbname: REQUIRED, non-empty
  ✓ username: REQUIRED, non-empty
  ✓ engine_version: REQUIRED, valid version format
```

#### Conditional: Aurora Serverless Mode
```
IF aurora == true THEN:
  ✓ min_capacity: REQUIRED, must be one of:
    [0.5, 1, 2, 4, 8, 16, 32, 64, 128, 192, 256, 384]
  ✓ max_capacity: REQUIRED, must be >= min_capacity
  ✓ max_capacity: must be from allowed values list

ELSE (RDS mode):
  ✓ instance_class: REQUIRED, valid DB instance class
    Examples: db.t3.micro, db.t3.small, db.m5.large
  ✓ allocated_storage: REQUIRED, must be > 0
  ✓ storage_type: optional, one of ["gp2", "gp3", "io1"]
```

#### Cross-Field Validation
```
max_capacity >= min_capacity
```

#### Engine Version Format
```
Valid formats:
  - "14" or "14.x"
  - "15" or "15.x"
  - "16" or "16.x"
```

---

### 7. DOMAIN CONFIGURATION

#### Required When Enabled
```
IF domain.enabled == true THEN:
  ✓ domain_name: REQUIRED, non-empty, valid FQDN
```

#### Conditional: DNS Delegation (Subdomain)
```
IF is_dns_root == false AND domain.enabled == true THEN:
  ✓ dns_root_account_id: REQUIRED (12 digits)
  ✓ delegation_role_arn: REQUIRED, valid ARN format
  ✓ root_zone_id: REQUIRED, non-empty (Route53 Zone ID)
```

#### Format Validation

**Domain Name:**
```
Pattern: ^([a-z0-9]+(-[a-z0-9]+)*\.)+[a-z]{2,}$
Examples:
  ✓ example.com
  ✓ api.example.com
  ✓ my-app.example.com
  ✗ https://example.com (no protocol)
  ✗ example.com/path (no path)
```

**Delegation Role ARN:**
```
Pattern: ^arn:aws:iam::\d{12}:role\/[a-zA-Z0-9+=,.@_-]+$
Example: arn:aws:iam::123456789012:role/DNSDelegationRole
```

**Zone ID:**
```
Pattern: ^Z[A-Z0-9]{10,}$
Example: Z1234567890ABC
```

---

### 8. COGNITO CONFIGURATION

#### Required When Enabled
```
IF cognito.enabled == true THEN:
  (No additional required fields)
```

#### Conditional: User Pool Domain
```
IF enable_user_pool_domain == true THEN:
  ✓ user_pool_domain_prefix: REQUIRED, non-empty
  ✓ user_pool_domain_prefix: lowercase alphanumeric + hyphens only

Pattern: ^[a-z0-9-]+$
Example: my-app-prod
```

#### Conditional: Dashboard Client
```
IF enable_dashboard_client == true THEN:
  ✓ dashboard_callback_urls: REQUIRED, non-empty array
  ✓ Each URL MUST start with "https://"

Examples:
  ✓ https://app.example.com/callback
  ✓ https://localhost:3000/callback (dev)
  ✗ http://example.com (not HTTPS)
```

#### Allowed Values
```yaml
auto_verified_attributes: array
  Each element must be: "email" OR "phone_number"
```

---

### 9. SES CONFIGURATION

#### Required When Enabled
```
IF ses.enabled == true THEN:
  ✓ domain_name: REQUIRED, valid domain format
  ✓ test_emails: optional, array of valid email addresses
```

#### Format Validation

**Email Address:**
```
Pattern: ^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$
Examples:
  ✓ user@example.com
  ✓ first.last+tag@company.co.uk
  ✗ invalid@domain (no TLD)
  ✗ @example.com (no local part)
```

---

### 10. WORKLOAD (Backend) CONFIGURATION

#### Scaling Validation
```
IF backend_autoscaling_enabled == true THEN:
  ✓ backend_autoscaling_min_capacity: REQUIRED, must be > 0
  ✓ backend_autoscaling_max_capacity: REQUIRED, must be >= min_capacity
  ✓ backend_desired_count: MUST be between min and max capacity

Cross-Field:
  min_capacity <= desired_count <= max_capacity
  min_capacity <= max_capacity
```

#### Allowed Values: CPU/Memory Combinations

**Valid CPU values:**
```yaml
backend_cpu: ["256", "512", "1024", "2048", "4096"]
```

**Valid Memory by CPU (all values in MB):**
```
CPU 256:
  Memory: [512, 1024, 2048]

CPU 512:
  Memory: [1024, 2048, 3072, 4096]

CPU 1024:
  Memory: [2048, 3072, 4096, 5120, 6144, 7168, 8192]

CPU 2048:
  Memory: [4096, 5120, 6144, 7168, 8192, 9216, 10240, 11264, 12288, 13312, 14336, 15360, 16384]

CPU 4096:
  Memory: [8192, 9216, 10240, ..., 30720]
```

**Validation:**
```
✓ backend_cpu must be in allowed list
✓ backend_memory must be valid for the specified CPU
```

#### Conditional: PgAdmin
```
IF install_pg_admin == true THEN:
  ✓ pg_admin_email: REQUIRED, valid email format
  ✓ postgres.enabled: MUST be true (cross-field dependency)

Error:
  "PgAdmin requires Postgres to be enabled"
```

#### Conditional: GitHub OIDC
```
IF enable_github_oidc == true THEN:
  ✓ github_oidc_subjects: REQUIRED, non-empty array
  ✓ Each subject MUST match pattern: ^repo:[^:]+\/[^:]+(:.*)?$

Examples:
  ✓ repo:MadAppGang/*
  ✓ repo:MadAppGang/project:ref:refs/heads/main
  ✗ invalid-format
```

---

### 11. AMPLIFY APPS

#### Required Fields
```yaml
name: string                    # Non-empty
github_repository: string       # Valid GitHub URL
branches: array                 # Non-empty array
```

#### GitHub Repository Format
```
Pattern: ^https://github\.com/[^/]+/[^/]+$
Examples:
  ✓ https://github.com/username/repo
  ✗ github.com/username/repo (missing https://)
  ✗ https://gitlab.com/username/repo (not GitHub)
```

#### Branch Validation
Each branch must have:
```yaml
name: string                    # Required, non-empty
stage: string                   # Optional, one of:
                                # ["PRODUCTION", "DEVELOPMENT", "BETA", "EXPERIMENTAL"]
enable_auto_build: boolean      # Optional
enable_pull_request_preview: boolean  # Optional
environment_variables: map      # Optional
```

#### Uniqueness
```
Amplify app names MUST be unique within the environment
```

---

### 12. ECR TRUSTED ACCOUNTS (Schema v8)

#### Array Element Requirements
Each trusted account must have:
```yaml
account_id: string              # Required, 12 digits
env: string                     # Required, non-empty
region: string                  # Required, valid AWS region
```

#### Conditional Validation
```
IF ecr_trusted_accounts is non-empty THEN:
  ✓ ecr_strategy: MUST be "local"

Reason: Can't trust other accounts if using cross-account ECR yourself

Error:
  "Cannot have ECR trusted accounts when ecr_strategy is 'cross_account'"
```

---

### 13. AWS SSO CONFIGURATION

#### Required Fields
```yaml
sso_start_url: string           # Must start with https://
sso_region: string              # Valid AWS region
account_id: string              # 12 digits
role_name: string               # Valid IAM role name
```

#### Format Validation

**SSO Start URL:**
```
Pattern: ^https://.*\.awsapps\.com/start.*$
Example: https://d-1234567890.awsapps.com/start
```

**Role Name:**
```
Pattern: ^[a-zA-Z0-9+=,.@_-]+$
Examples:
  ✓ AdministratorAccess
  ✓ PowerUserAccess
  ✓ CustomRole-123
  ✗ Invalid Role! (contains special chars)
```

**Output Format:**
```yaml
output: ["json", "yaml", "text", "table"]  # Optional
```

---

### 14. BUCKETS (S3) CONFIGURATION

#### Required Fields
```yaml
name: string                    # Unique, globally unique in S3
```

#### Format Validation

**Bucket Name:**
```
Requirements:
  - 3-63 characters
  - Lowercase letters, numbers, hyphens only
  - Must start and end with letter or number
  - No consecutive periods
  - Not IP address format

Pattern: ^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$

Examples:
  ✓ my-bucket-123
  ✓ app-data-prod
  ✗ MyBucket (uppercase)
  ✗ bucket_name (underscore)
  ✗ -bucket (starts with hyphen)
  ✗ bucket- (ends with hyphen)
```

#### Uniqueness
```
S3 bucket names MUST be globally unique across ALL AWS accounts
```

---

## Implementation Recommendations

### Library Comparison

| Feature | ozzo-validation | go-playground/validator | Zog |
|---------|----------------|------------------------|-----|
| **API Style** | Programmatic (like Zod) | Tag-based | Programmatic (like Zod) |
| **Conditional Rules** | ⭐⭐⭐⭐⭐ `validation.When()` | ⭐⭐⭐ Tag-based | ⭐⭐ Limited |
| **Cross-Field** | ⭐⭐⭐⭐⭐ Native support | ⭐⭐⭐⭐ Built-in tags | ⭐⭐⭐ Unknown |
| **Cross-Entity** | ⭐⭐⭐⭐⭐ Custom validation | ⭐⭐⭐ Custom validators | ⭐⭐ Unknown |
| **Readability** | ⭐⭐⭐⭐⭐ Very clear | ⭐⭐⭐ Tag syntax | ⭐⭐⭐⭐ Clear |
| **Flexibility** | ⭐⭐⭐⭐⭐ Extremely flexible | ⭐⭐⭐ Limited by tags | ⭐⭐⭐ Good |
| **Maturity** | ⭐⭐⭐⭐ Stable | ⭐⭐⭐⭐⭐ Most mature | ⭐⭐ New |
| **Community** | ⭐⭐⭐⭐ Good | ⭐⭐⭐⭐⭐ Largest | ⭐⭐ Growing |
| **Learning Curve** | ⭐⭐⭐⭐ Easy | ⭐⭐⭐ Medium | ⭐⭐⭐⭐ Easy |

### Recommendation: **ozzo-validation v4**

**Why:**
1. ✅ Best support for conditional validation (`validation.When()`)
2. ✅ Natural cross-field validation (access to full struct)
3. ✅ Easy to implement cross-entity validation (custom functions)
4. ✅ Code-based approach is more flexible than tags
5. ✅ Similar to Zod (familiar to TypeScript developers)
6. ✅ No struct tags needed (non-invasive)
7. ✅ Excellent error reporting

**Installation:**
```bash
go get github.com/go-ozzo/ozzo-validation/v4
```

---

## Code Examples

### Example 1: ECR Config Validation (Complex Conditional)

```go
package main

import (
    "fmt"
    "regexp"
    validation "github.com/go-ozzo/ozzo-validation/v4"
)

var ecrURIPattern = regexp.MustCompile(`^\d{12}\.dkr\.ecr\.[a-z0-9-]+\.amazonaws\.com\/[a-zA-Z0-9_-]+$`)

func (e *ECRConfig) Validate() error {
    return validation.ValidateStruct(e,
        // Mode must be one of allowed values (defaults to "create_ecr")
        validation.Field(&e.Mode,
            validation.In("create_ecr", "manual_repo", "use_existing")),

        // RepositoryURI required only when mode is "manual_repo"
        validation.Field(&e.RepositoryURI,
            validation.Required.When(e.Mode == "manual_repo").
                Error("repository_uri is required when mode is 'manual_repo'"),
            validation.Match(ecrURIPattern).When(e.Mode == "manual_repo").
                Error("repository_uri must be in format '<account-id>.dkr.ecr.<region>.amazonaws.com/<repo-name>'")),

        // SourceServiceName required only when mode is "use_existing"
        validation.Field(&e.SourceServiceName,
            validation.Required.When(e.Mode == "use_existing").
                Error("source_service_name is required when mode is 'use_existing'")),

        // SourceServiceType required only when mode is "use_existing"
        validation.Field(&e.SourceServiceType,
            validation.Required.When(e.Mode == "use_existing").
                Error("source_service_type is required when mode is 'use_existing'"),
            validation.In("services", "event_processor_tasks", "scheduled_tasks").
                When(e.Mode == "use_existing").
                Error("source_service_type must be 'services', 'event_processor_tasks', or 'scheduled_tasks'")),
    )
}
```

### Example 2: Cross-Entity Validation (ECR Source Service)

```go
// Custom validation function for cross-entity checks
func (s *Service) ValidateWithEnv(env *Env) error {
    // First validate the service itself
    if err := s.Validate(); err != nil {
        return err
    }

    // If ECR config uses existing service, validate cross-entity
    if s.ECRConfig != nil && s.ECRConfig.Mode == "use_existing" {
        if err := validateSourceServiceExists(
            s.ECRConfig.SourceServiceName,
            s.ECRConfig.SourceServiceType,
            s.Name,
            env,
        ); err != nil {
            return fmt.Errorf("service '%s': %w", s.Name, err)
        }
    }

    return nil
}

func validateSourceServiceExists(sourceName, sourceType, currentName string, env *Env) error {
    var sourceConfig *ECRConfig
    var found bool

    switch sourceType {
    case "services":
        for _, svc := range env.Services {
            if svc.Name == sourceName {
                found = true
                sourceConfig = svc.ECRConfig
                break
            }
        }
    case "event_processor_tasks":
        for _, task := range env.EventProcessorTasks {
            if task.Name == sourceName {
                found = true
                sourceConfig = task.ECRConfig
                break
            }
        }
    case "scheduled_tasks":
        for _, task := range env.ScheduledTasks {
            if task.Name == sourceName {
                found = true
                sourceConfig = task.ECRConfig
                break
            }
        }
    }

    if !found {
        return fmt.Errorf("source service '%s' not found in %s", sourceName, sourceType)
    }

    // Validate source service has create_ecr mode
    sourceMode := "create_ecr" // default
    if sourceConfig != nil && sourceConfig.Mode != "" {
        sourceMode = sourceConfig.Mode
    }

    if sourceMode != "create_ecr" {
        return fmt.Errorf("source service '%s' must have ecr_config.mode='create_ecr', but has mode='%s'",
            sourceName, sourceMode)
    }

    return nil
}
```

### Example 3: Postgres Aurora Validation (Cross-Field)

```go
func (p *Postgres) Validate() error {
    return validation.ValidateStruct(p,
        // When enabled, dbname and username are required
        validation.Field(&p.Dbname,
            validation.Required.When(p.Enabled).Error("dbname is required when Postgres is enabled")),
        validation.Field(&p.Username,
            validation.Required.When(p.Enabled).Error("username is required when Postgres is enabled")),
        validation.Field(&p.EngineVersion,
            validation.Required.When(p.Enabled).Error("engine_version is required when Postgres is enabled")),

        // Aurora-specific fields
        validation.Field(&p.MinCapacity,
            validation.Required.When(p.Enabled && p.Aurora).
                Error("min_capacity is required for Aurora"),
            validation.In(0.5, 1.0, 2.0, 4.0, 8.0, 16.0, 32.0, 64.0, 128.0, 192.0, 256.0, 384.0).
                When(p.Enabled && p.Aurora).
                Error("min_capacity must be one of: 0.5, 1, 2, 4, 8, 16, 32, 64, 128, 192, 256, 384")),
        validation.Field(&p.MaxCapacity,
            validation.Required.When(p.Enabled && p.Aurora).
                Error("max_capacity is required for Aurora"),
            validation.In(0.5, 1.0, 2.0, 4.0, 8.0, 16.0, 32.0, 64.0, 128.0, 192.0, 256.0, 384.0).
                When(p.Enabled && p.Aurora).
                Error("max_capacity must be one of: 0.5, 1, 2, 4, 8, 16, 32, 64, 128, 192, 256, 384"),
            validation.By(func(value interface{}) error {
                if p.Enabled && p.Aurora && p.MaxCapacity < p.MinCapacity {
                    return fmt.Errorf("max_capacity (%v) must be >= min_capacity (%v)", p.MaxCapacity, p.MinCapacity)
                }
                return nil
            })),

        // RDS-specific fields
        validation.Field(&p.InstanceClass,
            validation.Required.When(p.Enabled && !p.Aurora).
                Error("instance_class is required for RDS")),
        validation.Field(&p.AllocatedStorage,
            validation.Required.When(p.Enabled && !p.Aurora).
                Error("allocated_storage is required for RDS"),
            validation.Min(20).When(p.Enabled && !p.Aurora).
                Error("allocated_storage must be at least 20 GB")),
    )
}
```

### Example 4: Domain Configuration (Conditional Required)

```go
var domainPattern = regexp.MustCompile(`^([a-z0-9]+(-[a-z0-9]+)*\.)+[a-z]{2,}$`)
var arnPattern = regexp.MustCompile(`^arn:aws:iam::\d{12}:role\/[a-zA-Z0-9+=,.@_-]+$`)
var zoneIDPattern = regexp.MustCompile(`^Z[A-Z0-9]{10,}$`)

func (d *Domain) Validate() error {
    return validation.ValidateStruct(d,
        // Domain name required when enabled
        validation.Field(&d.DomainName,
            validation.Required.When(d.Enabled).Error("domain_name is required when domain is enabled"),
            validation.Match(domainPattern).When(d.Enabled).
                Error("domain_name must be a valid FQDN (no protocol, no path)")),

        // DNS delegation fields (for subdomain environments)
        validation.Field(&d.DNSRootAccountID,
            validation.Required.When(d.Enabled && !d.IsDNSRoot).
                Error("dns_root_account_id is required for subdomain delegation"),
            validation.Match(regexp.MustCompile(`^\d{12}$`)).When(d.Enabled && !d.IsDNSRoot).
                Error("dns_root_account_id must be 12 digits")),
        validation.Field(&d.DelegationRoleArn,
            validation.Required.When(d.Enabled && !d.IsDNSRoot).
                Error("delegation_role_arn is required for subdomain delegation"),
            validation.Match(arnPattern).When(d.Enabled && !d.IsDNSRoot).
                Error("delegation_role_arn must be a valid IAM role ARN")),
        validation.Field(&d.RootZoneID,
            validation.Required.When(d.Enabled && !d.IsDNSRoot).
                Error("root_zone_id is required for subdomain delegation"),
            validation.Match(zoneIDPattern).When(d.Enabled && !d.IsDNSRoot).
                Error("root_zone_id must be a valid Route53 Zone ID")),
    )
}
```

### Example 5: Workload Backend Scaling (Complex Cross-Field)

```go
func (w *Workload) Validate() error {
    return validation.ValidateStruct(w,
        // CPU must be valid value
        validation.Field(&w.BackendCPU,
            validation.In("256", "512", "1024", "2048", "4096").
                Error("backend_cpu must be one of: 256, 512, 1024, 2048, 4096")),

        // Memory must be valid for CPU
        validation.Field(&w.BackendMemory,
            validation.By(validateCPUMemoryCombination(w.BackendCPU, w.BackendMemory))),

        // Autoscaling validation
        validation.Field(&w.BackendAutoscalingMinCapacity,
            validation.Required.When(w.BackendAutoscalingEnabled).
                Error("backend_autoscaling_min_capacity is required when autoscaling is enabled"),
            validation.Min(1).When(w.BackendAutoscalingEnabled).
                Error("backend_autoscaling_min_capacity must be at least 1")),
        validation.Field(&w.BackendAutoscalingMaxCapacity,
            validation.Required.When(w.BackendAutoscalingEnabled).
                Error("backend_autoscaling_max_capacity is required when autoscaling is enabled"),
            validation.By(func(value interface{}) error {
                if w.BackendAutoscalingEnabled && w.BackendAutoscalingMaxCapacity < w.BackendAutoscalingMinCapacity {
                    return fmt.Errorf("max_capacity (%d) must be >= min_capacity (%d)",
                        w.BackendAutoscalingMaxCapacity, w.BackendAutoscalingMinCapacity)
                }
                return nil
            })),
        validation.Field(&w.BackendDesiredCount,
            validation.By(func(value interface{}) error {
                if w.BackendAutoscalingEnabled {
                    if w.BackendDesiredCount < w.BackendAutoscalingMinCapacity {
                        return fmt.Errorf("desired_count (%d) must be >= min_capacity (%d)",
                            w.BackendDesiredCount, w.BackendAutoscalingMinCapacity)
                    }
                    if w.BackendDesiredCount > w.BackendAutoscalingMaxCapacity {
                        return fmt.Errorf("desired_count (%d) must be <= max_capacity (%d)",
                            w.BackendDesiredCount, w.BackendAutoscalingMaxCapacity)
                    }
                }
                return nil
            })),

        // PgAdmin validation (cross-service dependency)
        validation.Field(&w.PgAdminEmail,
            validation.Required.When(w.InstallPgAdmin).Error("pg_admin_email is required when PgAdmin is enabled")),
    )
}

func validateCPUMemoryCombination(cpu, memory string) validation.RuleFunc {
    validCombos := map[string][]string{
        "256":  {"512", "1024", "2048"},
        "512":  {"1024", "2048", "3072", "4096"},
        "1024": {"2048", "3072", "4096", "5120", "6144", "7168", "8192"},
        "2048": {"4096", "5120", "6144", "7168", "8192", "9216", "10240",
                 "11264", "12288", "13312", "14336", "15360", "16384"},
        "4096": {"8192", "9216", "10240", "11264", "12288", "13312", "14336",
                 "15360", "16384", "17408", "18432", "19456", "20480", "21504",
                 "22528", "23552", "24576", "25600", "26624", "27648", "28672",
                 "29696", "30720"},
    }

    return func(value interface{}) error {
        validMemories, ok := validCombos[cpu]
        if !ok {
            return fmt.Errorf("invalid CPU value: %s", cpu)
        }

        for _, validMem := range validMemories {
            if memory == validMem {
                return nil
            }
        }

        return fmt.Errorf("memory '%s' is not valid for CPU '%s'. Valid values: %v",
            memory, cpu, validMemories)
    }
}
```

### Example 6: Environment-Level Validation

```go
func (e *Env) Validate() error {
    // Basic field validation
    if err := validation.ValidateStruct(e,
        validation.Field(&e.Project, validation.Required),
        validation.Field(&e.Env, validation.Required),
        validation.Field(&e.Region, validation.Required, validation.Match(regexp.MustCompile(`^[a-z]{2}-[a-z]+-\d$`))),
        validation.Field(&e.AccountID, validation.Required, validation.Match(regexp.MustCompile(`^\d{12}$`))),
        validation.Field(&e.StateBucket, validation.Required),

        // ECR strategy validation
        validation.Field(&e.ECRStrategy, validation.In("local", "cross_account")),
        validation.Field(&e.ECRAccountID,
            validation.Required.When(e.ECRStrategy == "cross_account").
                Error("ecr_account_id is required when ecr_strategy is 'cross_account'"),
            validation.Match(regexp.MustCompile(`^\d{12}$`)).When(e.ECRStrategy == "cross_account")),
        validation.Field(&e.ECRAccountRegion,
            validation.Required.When(e.ECRStrategy == "cross_account").
                Error("ecr_account_region is required when ecr_strategy is 'cross_account'")),

        // Nested struct validation
        validation.Field(&e.Domain),
        validation.Field(&e.Postgres),
        validation.Field(&e.Cognito),
        validation.Field(&e.Workload),
    ); err != nil {
        return err
    }

    // Validate all services with cross-entity checks
    for _, service := range e.Services {
        if err := service.ValidateWithEnv(e); err != nil {
            return err
        }
    }

    // Validate all scheduled tasks
    for _, task := range e.ScheduledTasks {
        if err := task.ValidateWithEnv(e); err != nil {
            return err
        }
    }

    // Validate all event processor tasks
    for _, task := range e.EventProcessorTasks {
        if err := task.ValidateWithEnv(e); err != nil {
            return err
        }
    }

    // Check service name uniqueness
    if err := validateUniqueNames(e); err != nil {
        return err
    }

    // Check PgAdmin dependency on Postgres
    if e.Workload.InstallPgAdmin && !e.Postgres.Enabled {
        return fmt.Errorf("PgAdmin requires Postgres to be enabled")
    }

    return nil
}

func validateUniqueNames(e *Env) error {
    names := make(map[string]bool)

    // Check services
    for _, svc := range e.Services {
        if names[svc.Name] {
            return fmt.Errorf("duplicate service name: '%s'", svc.Name)
        }
        names[svc.Name] = true
    }

    // Check scheduled tasks
    for _, task := range e.ScheduledTasks {
        if names[task.Name] {
            return fmt.Errorf("duplicate task name: '%s' (conflicts with service or another task)", task.Name)
        }
        names[task.Name] = true
    }

    // Check event processor tasks
    for _, task := range e.EventProcessorTasks {
        if names[task.Name] {
            return fmt.Errorf("duplicate task name: '%s' (conflicts with service or another task)", task.Name)
        }
        names[task.Name] = true
    }

    return nil
}
```

---

## Usage in Application

### Integration Point

Add validation call before Terraform generation:

```go
// In your main application flow
func generateTerraformFiles(envFile string) error {
    // Load YAML
    env, err := loadEnv(envFile)
    if err != nil {
        return fmt.Errorf("failed to load environment: %w", err)
    }

    // VALIDATE BEFORE PROCESSING
    if err := env.Validate(); err != nil {
        return fmt.Errorf("configuration validation failed:\n%w", err)
    }

    // Continue with Terraform generation
    return generateFromTemplate(env)
}
```

### Error Reporting

ozzo-validation provides detailed error messages:

```go
if err := env.Validate(); err != nil {
    // Error output example:
    // domain: (domain_name: domain_name is required when domain is enabled).
    // postgres: (max_capacity: max_capacity (0.5) must be >= min_capacity (1)).
    // services[0]: (ecr_config: (source_service_name: source service 'backend' not found in services)).

    fmt.Printf("Configuration validation failed:\n%v\n", err)
    return err
}
```

---

## Testing Strategy

### Unit Tests

```go
func TestECRConfigValidation(t *testing.T) {
    tests := []struct {
        name    string
        config  ECRConfig
        wantErr bool
        errMsg  string
    }{
        {
            name: "valid create_ecr mode",
            config: ECRConfig{Mode: "create_ecr"},
            wantErr: false,
        },
        {
            name: "manual_repo without repository_uri",
            config: ECRConfig{Mode: "manual_repo"},
            wantErr: true,
            errMsg: "repository_uri is required",
        },
        {
            name: "use_existing without source_service_name",
            config: ECRConfig{Mode: "use_existing"},
            wantErr: true,
            errMsg: "source_service_name is required",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.config.Validate()
            if (err != nil) != tt.wantErr {
                t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
            }
            if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
                t.Errorf("Error message = %v, want to contain %v", err.Error(), tt.errMsg)
            }
        })
    }
}
```

---

## Migration Plan

### Phase 1: Foundation (Week 1)
1. Add ozzo-validation dependency
2. Create validation methods for basic structs (ECRConfig, Service, Postgres)
3. Write unit tests

### Phase 2: Complex Validation (Week 2)
1. Implement cross-field validation (scaling, capacity)
2. Implement cross-entity validation (ECR source service)
3. Add uniqueness checks

### Phase 3: Integration (Week 3)
1. Integrate validation into main application flow
2. Add validation to TUI before Terraform generation
3. Add validation to web API endpoints
4. Improve error messages and formatting

### Phase 4: Complete Coverage (Week 4)
1. Add remaining validation rules (Amplify, Buckets, SES, etc.)
2. Write comprehensive test suite
3. Add validation documentation for users
4. Remove old validation code from `app/validation.go`

---

## Maintenance

### Adding New Validation Rules

When adding new configuration fields:

1. **Document the rule** in this catalog
2. **Implement validation** using ozzo-validation patterns
3. **Write tests** covering success and failure cases
4. **Update error messages** to be clear and actionable

### Common Patterns Reference

```go
// Required field
validation.Field(&field, validation.Required)

// Conditional required
validation.Field(&field, validation.Required.When(condition))

// Enum values
validation.Field(&field, validation.In("value1", "value2", "value3"))

// Pattern matching
validation.Field(&field, validation.Match(regexp.MustCompile(`pattern`)))

// Range
validation.Field(&field, validation.Min(1), validation.Max(100))

// Custom validation
validation.Field(&field, validation.By(customValidationFunc))

// Multiple rules with conditions
validation.Field(&field,
    validation.Required.When(condition1),
    validation.Match(pattern).When(condition2),
    validation.By(customFunc),
)
```

---

## References

- **ozzo-validation GitHub**: https://github.com/go-ozzo/ozzo-validation
- **ozzo-validation Documentation**: https://pkg.go.dev/github.com/go-ozzo/ozzo-validation/v4
- **AWS ECS Task Definitions**: https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task_definition_parameters.html
- **AWS Regions**: https://docs.aws.amazon.com/general/latest/gr/rande.html
- **S3 Bucket Naming Rules**: https://docs.aws.amazon.com/AmazonS3/latest/userguide/bucketnamingrules.html

---

**Document Status:** Initial version - ready for implementation
**Next Review:** After Phase 1 implementation
