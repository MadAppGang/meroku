# Architecture Document: Add DynamoDB State Locking to meroku

## 1. System Overview

### 1.1 Architecture Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│                         CURRENT STATE                             │
├──────────────────────────────────────────────────────────────────┤
│                                                                   │
│   ┌─────────────┐        ┌─────────────────────────────────────┐ │
│   │   meroku    │        │             AWS                      │ │
│   │   CLI/Web   │───────>│  ┌─────────────────────────────────┐│ │
│   └─────────────┘        │  │         S3 Bucket                ││ │
│         │                │  │  ┌───────────────────────────┐   ││ │
│         │ generates      │  │  │   terraform.tfstate       │   ││ │
│         ▼                │  │  │   (NO LOCKING!)           │   ││ │
│   ┌─────────────┐        │  │  └───────────────────────────┘   ││ │
│   │ env/main.hbs│        │  └─────────────────────────────────┘│ │
│   │  template   │        └─────────────────────────────────────┘ │
│   └─────────────┘                                                 │
│                                                                   │
└──────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────┐
│                         TARGET STATE                              │
├──────────────────────────────────────────────────────────────────┤
│                                                                   │
│   ┌─────────────┐        ┌─────────────────────────────────────┐ │
│   │   meroku    │        │             AWS                      │ │
│   │   CLI/Web   │───────>│                                      │ │
│   └─────────────┘        │  ┌─────────────┐  ┌───────────────┐ │ │
│         │                │  │  S3 Bucket  │  │   DynamoDB    │ │ │
│         │ generates      │  │             │  │   Table       │ │ │
│         ▼                │  │ ┌─────────┐ │  │               │ │ │
│   ┌─────────────┐        │  │ │ tfstate │ │  │ ┌───────────┐ │ │ │
│   │ env/main.hbs│        │  │ │  file   │ │  │ │  LockID   │ │ │ │
│   │  (updated)  │        │  │ └─────────┘ │  │ │ (hash key)│ │ │ │
│   └─────────────┘        │  └─────────────┘  │ └───────────┘ │ │ │
│                          │        │          │       ▲       │ │ │
│                          │        │          │       │       │ │ │
│                          │        └──────────┼───────┘       │ │ │
│                          │     lock acquired │               │ │ │
│                          │     before write  │               │ │ │
│                          │                   │               │ │ │
│                          └─────────────────────────────────────┘ │
│                                                                   │
└──────────────────────────────────────────────────────────────────┘
```

### 1.2 Component Descriptions

| Component | Purpose | Changes Required |
|-----------|---------|------------------|
| `env/main.hbs` | Terraform backend template | Add `dynamodb_table` configuration |
| `app/model.go` | YAML configuration model | Add `StateLockTable` field |
| `app/migrations.go` | YAML schema migrations | Add migration for new field |
| DynamoDB Table | State locking | Create new AWS resource |

### 1.3 Integration Points

1. **YAML → Template**: `state_lock_table` field flows to `dynamodb_table` in HCL
2. **Terraform → DynamoDB**: Terraform acquires lock before state operations
3. **S3 ↔ DynamoDB**: Work in concert (S3 stores, DynamoDB locks)

---

## 2. Data Design

### 2.1 DynamoDB Table Schema

```
Table Name: terraform-state-locks (or project-specific)

Primary Key:
  - Partition Key: LockID (String)

Attributes:
  - LockID: S3 bucket path used as unique identifier
  - Info: JSON containing lock metadata (terraform managed)

Billing: PAY_PER_REQUEST (on-demand)

Estimated Usage:
  - Reads: ~10-50/day per environment
  - Writes: ~5-20/day per environment
  - Cost: < $0.25/month
```

### 2.2 YAML Schema Addition

```yaml
# Schema Version 13 (or next available)
schema_version: 13
state_bucket: my-terraform-state
state_file: null
state_lock_table: terraform-state-locks  # NEW OPTIONAL FIELD

# Alternative: Auto-generate table name
# state_lock_table: auto  # Creates: {project}-terraform-locks-{env}
```

### 2.3 Data Flow

```
User runs `terraform apply`:
  1. Terraform reads backend config from generated .tf file
  2. Terraform attempts to acquire lock in DynamoDB:
     PUT item with LockID = "bucket/key"
     Condition: attribute_not_exists(LockID)
  3. If lock acquired:
     - Read state from S3
     - Perform operations
     - Write state to S3
     - Release lock (DELETE item)
  4. If lock NOT acquired:
     - Display "state is locked by ..." error
     - Wait or fail based on -lock-timeout
```

---

## 3. Technical Specifications

### 3.1 Template Changes (`env/main.hbs`)

```handlebars
terraform {
  backend "s3" {
    bucket = "{{ state_bucket }}"
    key    = "{{ state_file }}"
    region = "{{ region }}"
    {{#if state_lock_table}}
    dynamodb_table = "{{ state_lock_table }}"
    encrypt        = true
    {{/if}}
  }
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
  required_version = ">= 1.2.6"
}
```

### 3.2 Model Changes (`app/model.go`)

```go
type Env struct {
    SchemaVersion   int    `yaml:"schema_version,omitempty"`
    Project         string `yaml:"project"`
    Env             string `yaml:"env"`
    Region          string `yaml:"region"`
    StateBucket     string `yaml:"state_bucket"`
    StateFile       string `yaml:"state_file"`
    StateLockTable  string `yaml:"state_lock_table,omitempty"` // NEW
    // ... existing fields
}
```

### 3.3 Migration (`app/migrations.go`)

```go
// Migration from schema v12 to v13: Add state lock table support
func migrateV12ToV13(data map[string]interface{}) error {
    // State lock table is optional, no default value needed
    // Users can add state_lock_table: terraform-state-locks when ready

    data["schema_version"] = 13
    return nil
}
```

### 3.4 DynamoDB Table Terraform (Bootstrap)

```hcl
# bootstrap/dynamodb.tf - Run once per AWS account
resource "aws_dynamodb_table" "terraform_locks" {
  name         = "terraform-state-locks"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "LockID"

  attribute {
    name = "LockID"
    type = "S"
  }

  tags = {
    Name      = "Terraform State Locks"
    ManagedBy = "meroku"
    Purpose   = "Terraform state locking"
  }
}
```

### 3.5 IAM Permission Requirements

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "TerraformStateLocking",
      "Effect": "Allow",
      "Action": [
        "dynamodb:GetItem",
        "dynamodb:PutItem",
        "dynamodb:DeleteItem"
      ],
      "Resource": "arn:aws:dynamodb:*:*:table/terraform-state-locks"
    }
  ]
}
```

---

## 4. Security Design

### 4.1 Threat Model

| Threat | Mitigation |
|--------|------------|
| Unauthorized lock manipulation | IAM policies restrict access |
| State file exposure | S3 encryption (enabled with locking) |
| Denial of service (lock never released) | DynamoDB TTL (optional), force-unlock command |
| Cross-account access | Scoped IAM roles per environment |

### 4.2 Security Considerations

1. **Encryption**: Enable `encrypt = true` in backend config (added with locking)
2. **Access Control**: DynamoDB table should only be accessible by terraform roles
3. **Audit Trail**: DynamoDB has CloudTrail integration for audit logging
4. **Lock Timeout**: Terraform default is infinite; consider `-lock-timeout=5m` in CI/CD

---

## 5. Implementation Plan

### 5.1 Phases and Steps

#### Phase 1: Preparation (No AWS Changes)

1. Update `app/model.go` to add `StateLockTable` field
2. Update `env/main.hbs` template with conditional DynamoDB block
3. Add migration v12→v13 (or appropriate version)
4. Update tests in `app/migrations_test.go`
5. Update documentation

#### Phase 2: Bootstrap (One-Time Per Account)

1. Create DynamoDB table manually or via bootstrap terraform:
   ```bash
   aws dynamodb create-table \
     --table-name terraform-state-locks \
     --attribute-definitions AttributeName=LockID,AttributeType=S \
     --key-schema AttributeName=LockID,KeyType=HASH \
     --billing-mode PAY_PER_REQUEST \
     --region <your-region>
   ```

2. Update IAM roles (GitHub Actions, developer roles) with DynamoDB permissions

#### Phase 3: Migration (Per Environment)

1. Add `state_lock_table: terraform-state-locks` to environment YAML
2. Run `./meroku migrate <env>.yaml`
3. Run `make infra-gen-<env>` to regenerate terraform files
4. Run `terraform init -reconfigure` to update backend configuration
5. Verify with `terraform plan` (should show no changes)
6. Test locking: `terraform apply` in two terminals (second should be blocked)

### 5.2 Rollback Plan

If issues occur:

1. Remove `state_lock_table` from YAML
2. Regenerate terraform files
3. Run `terraform init -reconfigure` (backend config will change)
4. DynamoDB table can remain (unused but harmless)

### 5.3 Validation Checklist

- [ ] `terraform init` succeeds with new backend config
- [ ] `terraform plan` shows no changes (state unchanged)
- [ ] Concurrent `terraform apply` blocks correctly
- [ ] Force unlock works: `terraform force-unlock <lock-id>`
- [ ] CI/CD pipeline works with locking enabled
- [ ] IAM permissions are correct in all environments

---

## 6. Testing Strategy

### 6.1 Unit Tests

```go
// app/migrations_test.go
func TestMigrateV12ToV13(t *testing.T) {
    input := `
schema_version: 12
project: test
state_bucket: my-bucket
`
    result, err := migrate(input)
    assert.NoError(t, err)
    assert.Equal(t, 13, result["schema_version"])
    // state_lock_table is optional, should not be auto-added
}
```

### 6.2 Integration Tests

1. **Template Generation Test**:
   - Input: YAML with `state_lock_table`
   - Output: Generated .tf file contains `dynamodb_table`

2. **Backward Compatibility Test**:
   - Input: YAML without `state_lock_table`
   - Output: Generated .tf file has NO `dynamodb_table` line

### 6.3 Manual Testing

1. Create test environment with locking enabled
2. Run two concurrent `terraform apply` commands
3. Verify second command waits or errors appropriately
4. Verify lock is released after first command completes

---

## 7. Cost Analysis

| Resource | Monthly Cost | Notes |
|----------|--------------|-------|
| DynamoDB Table | $0.00 | No storage charge for small tables |
| DynamoDB Operations | ~$0.25 | PAY_PER_REQUEST pricing |
| **Total** | **~$0.25/month** | Negligible |

---

## 8. Appendix

### A. Alternative: Terraform 1.10 Native S3 Locking

If upgrading to Terraform 1.10+, the template can be simplified:

```handlebars
terraform {
  backend "s3" {
    bucket       = "{{ state_bucket }}"
    key          = "{{ state_file }}"
    region       = "{{ region }}"
    {{#if use_s3_native_lock}}
    use_lockfile = true
    {{else if state_lock_table}}
    dynamodb_table = "{{ state_lock_table }}"
    {{/if}}
    encrypt      = true
  }
}
```

### B. CLI Command Suggestion (Future Enhancement)

```bash
# Create DynamoDB table for locking
./meroku setup-state-locking --table-name terraform-state-locks --region us-east-1

# Migrate existing environment to use locking
./meroku enable-locking --env dev --table terraform-state-locks
```

### C. AWS CLI Quick Commands

```bash
# Create table
aws dynamodb create-table \
  --table-name terraform-state-locks \
  --attribute-definitions AttributeName=LockID,AttributeType=S \
  --key-schema AttributeName=LockID,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST

# List locks (for debugging)
aws dynamodb scan --table-name terraform-state-locks

# Force release a stuck lock
aws dynamodb delete-item \
  --table-name terraform-state-locks \
  --key '{"LockID":{"S":"my-bucket/path/to/state"}}'
```
