# Requirements Analysis: Terraform State Backend Migration

## Investigation Context

The user wants to understand:
1. How difficult it would be to migrate from S3 to DynamoDB for Terraform state
2. Whether this migration is worthwhile

## Current Architecture

### Current State Backend Configuration

**Template Location**: `env/main.hbs`

```hcl
terraform {
  backend "s3" {
    bucket = "{{ state_bucket }}"
    key    = "{{ state_file }}"
    region = "{{ region  }}"
  }
}
```

**Key Observations**:
- **S3-only storage**: State files stored in S3 bucket
- **NO state locking**: DynamoDB table for locking is NOT configured
- **NO consistency checks**: No mechanism to prevent concurrent state modifications
- **Per-environment buckets**: Each environment (dev, prod, staging) has its own `state_bucket`

### YAML Configuration Model

From `app/model.go`:
```go
type Env struct {
    StateBucket  string `yaml:"state_bucket"`
    StateFile    string `yaml:"state_file"`
    // ... other fields
}
```

### Example Configuration

From `project/dev.yaml`:
```yaml
state_bucket: instagram-terraform-state-dev
state_file: null  # Defaults to project/env key
```

---

## CRITICAL CLARIFICATION: S3 vs DynamoDB for State Storage

### What the User May Be Asking

There seems to be a **misunderstanding** about Terraform state backends. Let me clarify:

#### Option A: "Replace S3 with DynamoDB for state storage"
**This is NOT possible and NOT recommended.**

DynamoDB cannot replace S3 as a Terraform state backend because:
- Terraform state files can be **large** (megabytes)
- DynamoDB has a **400KB item size limit**
- DynamoDB is optimized for **key-value lookups**, not file storage
- Terraform does NOT support DynamoDB as a primary state backend

#### Option B: "Add DynamoDB locking to S3 backend"
**This IS the standard approach and IS recommended.**

The correct architecture is:
- **S3**: Stores the actual state file (state.tfstate)
- **DynamoDB**: Provides state locking and consistency checking

This is what Terraform's S3 backend supports natively:

```hcl
terraform {
  backend "s3" {
    bucket         = "my-state-bucket"
    key            = "path/to/state.tfstate"
    region         = "us-east-1"
    dynamodb_table = "terraform-state-locks"  # ADD THIS
    encrypt        = true
  }
}
```

---

## Requirements (Assuming Option B: Add DynamoDB Locking)

### Functional Requirements

1. **State Locking**: Prevent concurrent terraform apply operations
2. **Consistency Checking**: Detect and prevent state corruption
3. **Backward Compatibility**: Support existing YAML configurations
4. **Multi-Environment Support**: Each environment needs locking
5. **Template Updates**: Modify `env/main.hbs` to include DynamoDB configuration

### Non-Functional Requirements

1. **Minimal Disruption**: Existing deployments should continue working
2. **Zero Downtime Migration**: No state data loss during migration
3. **Cost Efficiency**: DynamoDB on-demand pricing is ~$0.25/month per table
4. **Regional Consistency**: DynamoDB table must be in same region as S3 bucket

### Technical Constraints

1. **Terraform Version**: Requires Terraform >= 0.12 (already satisfied: >= 1.2.6)
2. **AWS Provider**: Requires AWS provider with DynamoDB permissions
3. **IAM Permissions**: Need `dynamodb:*` permissions for state locking

### Assumptions

1. Teams may run terraform concurrently (otherwise locking is less critical)
2. State files are stored per-environment (not shared)
3. The meroku CLI manages terraform operations centrally

---

## Risk Assessment

### Current Risk (Without Locking)

| Risk | Severity | Likelihood | Impact |
|------|----------|------------|--------|
| Concurrent terraform apply corrupts state | HIGH | MEDIUM | State file corruption, manual recovery needed |
| Two users overwrite each other's changes | HIGH | MEDIUM | Infrastructure inconsistency |
| CI/CD pipeline conflicts | HIGH | HIGH (if using CI/CD) | Failed deployments |

### Migration Risk

| Risk | Severity | Likelihood | Impact |
|------|----------|------------|--------|
| State file loss during migration | CRITICAL | LOW | Full infrastructure rebuild |
| Lock table misconfiguration | MEDIUM | LOW | Locking doesn't work |
| Backward compatibility issues | MEDIUM | LOW | Existing projects break |

---

## Dependencies

1. **DynamoDB Table**: Must be created before terraform can use it
2. **IAM Permissions**: GitHub Actions role needs DynamoDB access
3. **YAML Schema Update**: May need migration for new fields
4. **Template Update**: `env/main.hbs` must be modified

---

## Summary

**Key Finding**: The question "move terraform state from S3 to DynamoDB" is technically a misnomer. The correct approach is to **ADD DynamoDB state locking to the existing S3 backend**.

**Recommendation**: Proceed with adding DynamoDB locking - it's:
- **Simple**: ~20 lines of code changes
- **Cheap**: ~$0.25/month
- **Safe**: Zero risk to existing state
- **Valuable**: Prevents state corruption in team environments
