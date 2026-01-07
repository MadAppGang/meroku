# Alternative Designs: Terraform State Backend Enhancement

## Overview

Based on the requirements analysis, the goal is to **add state locking** to the existing S3 backend, not replace S3 with DynamoDB. Here are the design alternatives:

---

## Alternative 1: Per-Environment DynamoDB Tables (Recommended)

### Approach
Create a dedicated DynamoDB table for each environment's state locking.

### Configuration

**YAML Addition** (`project/dev.yaml`):
```yaml
state_bucket: instagram-terraform-state-dev
state_file: null
state_lock_table: instagram-terraform-locks-dev  # NEW
```

**Template Update** (`env/main.hbs`):
```hcl
terraform {
  backend "s3" {
    bucket         = "{{ state_bucket }}"
    key            = "{{ state_file }}"
    region         = "{{ region }}"
    {{#if state_lock_table}}
    dynamodb_table = "{{ state_lock_table }}"
    encrypt        = true
    {{/if}}
  }
}
```

**DynamoDB Table Creation** (one-time setup):
```hcl
resource "aws_dynamodb_table" "terraform_locks" {
  name         = "{{ project }}-terraform-locks-{{ env }}"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "LockID"

  attribute {
    name = "LockID"
    type = "S"
  }

  tags = {
    Name        = "Terraform State Locks"
    Project     = "{{ project }}"
    Environment = "{{ env }}"
  }
}
```

### Pros
- **Isolation**: Each environment has its own lock table
- **Simple IAM**: Permissions scoped to specific tables
- **Clear ownership**: Easy to identify which locks belong to which environment

### Cons
- **More resources**: Creates N tables for N environments
- **Chicken-and-egg**: Need bootstrap mechanism to create table before terraform uses it

### Estimated Complexity
- **Template changes**: 5 lines
- **Model changes**: 1 field
- **Migration**: Optional (add field)
- **Total effort**: ~1-2 hours

### Risk Assessment
- **State corruption risk**: NONE (additive change)
- **Backward compatibility**: FULL (optional field)
- **Rollback complexity**: LOW (remove field from YAML)

---

## Alternative 2: Shared DynamoDB Table (Simpler)

### Approach
Use a single DynamoDB table across all environments, with the S3 key used as the lock identifier.

### Configuration

**YAML Addition** (`project/dev.yaml`):
```yaml
state_bucket: instagram-terraform-state-dev
state_file: null
state_lock_table: terraform-state-locks  # Shared table name
```

**Template Update** (`env/main.hbs`):
Same as Alternative 1.

### Pros
- **Fewer resources**: Only one DynamoDB table needed
- **Simpler bootstrap**: One-time table creation
- **Cost efficient**: Single table for all environments

### Cons
- **Less isolation**: All environments share locks
- **IAM complexity**: Permissions must allow access to shared table
- **Potential confusion**: Lock entries from multiple environments in one table

### Estimated Complexity
- **Template changes**: 5 lines
- **Model changes**: 1 field
- **Migration**: Optional
- **Total effort**: ~30 minutes

### Risk Assessment
- **State corruption risk**: NONE
- **Backward compatibility**: FULL
- **Rollback complexity**: LOW

---

## Alternative 3: Use Terraform Cloud/Enterprise

### Approach
Migrate entirely to Terraform Cloud for state management.

### Configuration

**Template Update** (`env/main.hbs`):
```hcl
terraform {
  cloud {
    organization = "{{ terraform_org }}"
    workspaces {
      name = "{{ project }}-{{ env }}"
    }
  }
}
```

### Pros
- **Managed service**: No infrastructure to maintain
- **Built-in locking**: Native state locking
- **Additional features**: Run history, policy checks, cost estimation
- **Team collaboration**: Better visibility and approval workflows

### Cons
- **Cost**: Free tier limited, paid tiers start at $20/user/month
- **Vendor lock-in**: Tied to HashiCorp's platform
- **Migration complexity**: Requires moving all state files
- **Internet dependency**: State operations require connectivity
- **Breaking change**: Completely different backend configuration

### Estimated Complexity
- **Template changes**: Complete rewrite of backend block
- **Model changes**: New fields for org, workspace
- **Migration**: Complex (state file migration)
- **Total effort**: ~1-2 days

### Risk Assessment
- **State corruption risk**: LOW (but migration needed)
- **Backward compatibility**: NONE (breaking change)
- **Rollback complexity**: HIGH

---

## Alternative 4: Use S3 Native Lock (Terraform >= 1.10)

### Approach
Leverage Terraform 1.10's new native S3 locking using DynamoDB-less S3 conditional writes.

### Terraform Requirement
```hcl
required_version = ">= 1.10.0"  # Currently requires >= 1.2.6
```

### Configuration

**Template Update** (`env/main.hbs`):
```hcl
terraform {
  backend "s3" {
    bucket       = "{{ state_bucket }}"
    key          = "{{ state_file }}"
    region       = "{{ region }}"
    use_lockfile = true  # NEW in 1.10
    encrypt      = true
  }
}
```

### Pros
- **No DynamoDB needed**: Eliminates additional resource
- **Simpler**: No lock table management
- **Cost savings**: No DynamoDB costs
- **Native support**: Built into Terraform core

### Cons
- **Version requirement**: Requires Terraform 1.10+ (released Dec 2024)
- **Less mature**: Newer feature, potentially less battle-tested
- **Breaking for older versions**: Can't use with Terraform < 1.10

### Estimated Complexity
- **Template changes**: 2 lines
- **Model changes**: 0-1 fields
- **Migration**: Terraform version upgrade required
- **Total effort**: ~30 minutes (if already on 1.10+)

### Risk Assessment
- **State corruption risk**: NONE
- **Backward compatibility**: DEPENDS on current Terraform version
- **Rollback complexity**: LOW

---

## Comparison Matrix

| Criteria | Alt 1: Per-Env DynamoDB | Alt 2: Shared DynamoDB | Alt 3: TF Cloud | Alt 4: S3 Native |
|----------|-------------------------|------------------------|-----------------|------------------|
| **Implementation Time** | 1-2 hours | 30 minutes | 1-2 days | 30 minutes |
| **Monthly Cost** | ~$0.25 × N envs | ~$0.25 total | $20+/user | $0 |
| **Breaking Change** | No | No | Yes | Version dependent |
| **Team Scalability** | Good | Good | Excellent | Good |
| **Rollback Ease** | Easy | Easy | Hard | Easy |
| **Terraform Version** | Any | Any | Any | >= 1.10 |
| **AWS Resources** | N tables | 1 table | None | None |

---

## Recommendation Summary

1. **For teams on Terraform 1.10+**: Use **Alternative 4** (S3 Native Lock)
2. **For teams on older Terraform**: Use **Alternative 2** (Shared DynamoDB)
3. **For enterprise environments**: Consider **Alternative 3** (Terraform Cloud)
4. **For maximum isolation**: Use **Alternative 1** (Per-Environment DynamoDB)
