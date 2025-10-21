# ECR Cross-Account Access - Analysis & Implementation

## The Problem

When using `ecr_strategy: "cross_account"`, ECS tasks in the consuming account (e.g., prod) need to pull container images from another account's ECR (e.g., dev). This requires **TWO-WAY permissions**:

### Current State ✅ ❌

**In Dev Account (ECR Owner):**
✅ **Resource-Based Policy** - Already implemented in `modules/workloads/ecr.tf`:
```hcl
data "aws_iam_policy_document" "default_ecr_policy" {
  statement {
    sid = "External read ECR policy"
    principals {
      type        = "*"
      identifiers = ["*"]
    }
    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:BatchGetImage",
      "ecr:DescribeImages",
      "ecr:DescribeRepositories",
      "ecr:GetDownloadUrlForLayer"
    ]
    condition {
      test     = "StringEquals"
      variable = "aws:PrincipalOrgID"
      values   = [data.aws_organizations_organization.org.id]
    }
  }
}
```
✅ This allows any account in the same AWS Organization to pull images.

**In Prod Account (Image Consumer):**
❌ **Identity-Based Policy** - MISSING!

The ECS Task Execution Role currently has:
```hcl
resource "aws_iam_role_policy_attachment" "backend_task_execution" {
  role       = aws_iam_role.backend_task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}
```

`AmazonECSTaskExecutionRolePolicy` includes:
- `ecr:GetAuthorizationToken` (works across accounts)
- `ecr:BatchCheckLayerAvailability` (ONLY for same-account ECR)
- `ecr:GetDownloadUrlForLayer` (ONLY for same-account ECR)
- `ecr:BatchGetImage` (ONLY for same-account ECR)

## The Fix

When `ecr_strategy == "cross_account"`, we need to add an additional IAM policy to the task execution role that explicitly grants access to the cross-account ECR.

### Required Permissions

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ecr:GetAuthorizationToken"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "ecr:BatchCheckLayerAvailability",
        "ecr:GetDownloadUrlForLayer",
        "ecr:BatchGetImage"
      ],
      "Resource": [
        "arn:aws:ecr:{ecr_region}:{ecr_account_id}:repository/{project}_backend",
        "arn:aws:ecr:{ecr_region}:{ecr_account_id}:repository/{project}_service_*",
        "arn:aws:ecr:{ecr_region}:{ecr_account_id}:repository/{project}_task_*"
      ]
    }
  ]
}
```

Note:
- `ecr:GetAuthorizationToken` is account-level (Resource: "*")
- Other ECR actions need explicit repository ARNs from the source account

## Implementation Plan

### Files to Modify

1. **modules/workloads/variables.tf**
   - Already has `ecr_strategy` ✅
   - Add `ecr_account_id` and `ecr_account_region` variables (may already exist from before)

2. **modules/workloads/backend.tf**
   - Add conditional IAM policy for cross-account ECR access
   - Attach to `backend_task_execution` role when `ecr_strategy == "cross_account"`

3. **modules/ecs_service/iam.tf**
   - Add conditional IAM policy for services
   - Attach to service task execution roles

4. **modules/ecs_task/iam.tf**
   - Add conditional IAM policy for scheduled/event tasks

### Terraform Code to Add

```hcl
# In modules/workloads/backend.tf (after backend_task_execution role creation)

# Cross-account ECR access policy (only when using cross_account strategy)
data "aws_iam_policy_document" "cross_account_ecr_access" {
  count = var.ecr_strategy == "cross_account" ? 1 : 0

  # GetAuthorizationToken is account-level
  statement {
    effect = "Allow"
    actions = [
      "ecr:GetAuthorizationToken"
    ]
    resources = ["*"]
  }

  # Repository-specific permissions
  statement {
    effect = "Allow"
    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:GetDownloadUrlForLayer",
      "ecr:BatchGetImage",
      "ecr:DescribeImages",
      "ecr:DescribeRepositories"
    ]
    resources = [
      "arn:aws:ecr:${var.ecr_account_region}:${var.ecr_account_id}:repository/${var.project}_backend",
      "arn:aws:ecr:${var.ecr_account_region}:${var.ecr_account_id}:repository/${var.project}_service_*",
      "arn:aws:ecr:${var.ecr_account_region}:${var.ecr_account_id}:repository/${var.project}_task_*"
    ]
  }
}

resource "aws_iam_policy" "cross_account_ecr_access" {
  count = var.ecr_strategy == "cross_account" ? 1 : 0

  name        = "${var.project}_cross_account_ecr_access_${var.env}"
  description = "Allow pulling images from cross-account ECR"
  policy      = data.aws_iam_policy_document.cross_account_ecr_access[0].json

  tags = {
    Name        = "${var.project}_cross_account_ecr_access_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_role_policy_attachment" "backend_cross_account_ecr" {
  count = var.ecr_strategy == "cross_account" ? 1 : 0

  role       = aws_iam_role.backend_task_execution.name
  policy_arn = aws_iam_policy.cross_account_ecr_access[0].arn
}
```

### Variables Needed

```hcl
# In modules/workloads/variables.tf

variable "ecr_strategy" {
  description = "ECR repository strategy: 'local' or 'cross_account'"
  type        = string
  default     = "local"
}

variable "ecr_account_id" {
  description = "AWS account ID for cross-account ECR access"
  type        = string
  default     = ""
}

variable "ecr_account_region" {
  description = "AWS region for cross-account ECR access"
  type        = string
  default     = ""
}
```

### Validation

Add validation to ensure cross_account strategy has required fields:

```hcl
variable "ecr_strategy" {
  description = "ECR repository strategy: 'local' or 'cross_account'"
  type        = string
  default     = "local"

  validation {
    condition     = contains(["local", "cross_account"], var.ecr_strategy)
    error_message = "ecr_strategy must be either 'local' or 'cross_account'"
  }
}

# Add locals to validate cross_account configuration
locals {
  validate_cross_account = (
    var.ecr_strategy == "cross_account" &&
    (var.ecr_account_id == "" || var.ecr_account_region == "")
  ) ? tobool("ERROR: ecr_account_id and ecr_account_region are required when ecr_strategy is 'cross_account'") : true
}
```

## How It Works

### Scenario: Prod pulls from Dev ECR

**1. Dev Account (111111111111) - Creates ECR with `ecr_strategy: "local"`**
```yaml
# dev.yaml
env: dev
ecr_strategy: "local"
```

Terraform creates:
- ECR repository: `myproject_backend`
- ECR policy allowing AWS Org access

**2. Prod Account (222222222222) - Uses Dev's ECR with `ecr_strategy: "cross_account"`**
```yaml
# prod.yaml
env: prod
ecr_strategy: "cross_account"
ecr_account_id: "111111111111"
ecr_account_region: "us-east-1"
```

Terraform creates:
- NO ECR repository (cross_account mode)
- ECS Task Definition with image: `111111111111.dkr.ecr.us-east-1.amazonaws.com/myproject_backend:latest`
- IAM policy attached to task execution role:
  ```json
  {
    "Effect": "Allow",
    "Action": ["ecr:GetAuthorizationToken"],
    "Resource": "*"
  },
  {
    "Effect": "Allow",
    "Action": [
      "ecr:BatchCheckLayerAvailability",
      "ecr:GetDownloadUrlForLayer",
      "ecr:BatchGetImage"
    ],
    "Resource": "arn:aws:ecr:us-east-1:111111111111:repository/myproject_backend"
  }
  ```

**3. ECS Task Startup in Prod**
1. ECS service starts new task
2. Task execution role assumes permissions
3. Calls `ecr:GetAuthorizationToken` (allowed by new policy)
4. Calls `ecr:BatchGetImage` on `arn:aws:ecr:us-east-1:111111111111:repository/myproject_backend`
   - Allowed by prod's task execution role (identity-based) ✅
   - Allowed by dev's ECR policy (resource-based) ✅
5. Image pulled successfully
6. Container starts

## Testing Plan

### 1. Test Local Strategy (Baseline)
```bash
# dev.yaml
ecr_strategy: "local"

make infra-apply env=dev
# Verify ECR repository created
# Verify task can pull images
```

### 2. Test Cross-Account Strategy
```bash
# prod.yaml
ecr_strategy: "cross_account"
ecr_account_id: "111111111111"  # dev account
ecr_account_region: "us-east-1"

make infra-apply env=prod
# Verify NO ECR repository created
# Verify IAM policy created with cross-account permissions
# Verify task execution role has policy attached
```

### 3. Test Image Pull
```bash
# In dev: Push image
docker push 111111111111.dkr.ecr.us-east-1.amazonaws.com/myproject_backend:latest

# In prod: Deploy service
aws ecs update-service --cluster myproject-prod-cluster --service myproject-backend-prod --force-new-deployment

# Check task logs
aws ecs describe-tasks --cluster myproject-prod-cluster --tasks <task-id>
# Should show successful image pull, not "ImagePullBackOff"
```

## Edge Cases

### 1. Missing ecr_account_id/ecr_account_region
**Error:**
```
Error: ecr_account_id and ecr_account_region are required when ecr_strategy is 'cross_account'
```

**Solution:** Add validation in variables.tf

### 2. Accounts Not in Same AWS Organization
**Symptom:** Image pull fails with AccessDenied

**Solution:** Manually update ECR policy in dev account:
```json
{
  "Effect": "Allow",
  "Principal": {
    "AWS": "arn:aws:iam::222222222222:root"
  },
  "Action": [
    "ecr:GetDownloadUrlForLayer",
    "ecr:BatchGetImage",
    "ecr:BatchCheckLayerAvailability"
  ]
}
```

### 3. Wrong Account ID or Region
**Symptom:** Image not found

**Solution:** Verify `ecr_account_id` and `ecr_account_region` match dev account

## Security Considerations

1. **Least Privilege:** Policy only grants pull access, not push
2. **Repository Wildcards:** Uses `${project}_*` patterns to allow all project repositories
3. **Organization Boundary:** Dev ECR policy restricts to AWS Organization
4. **No Account Wildcards:** Never use `*` for account IDs

## Cost Impact

- No additional cost for cross-account ECR access
- IAM policies are free
- Cross-region data transfer charges apply if accounts are in different regions

## Alternative Approaches Considered

### 1. ECR Image Replication
**Pros:** No cross-account permissions needed
**Cons:** Duplicate storage costs, replication lag

### 2. Single Shared ECR Account
**Pros:** Centralized registry
**Cons:** Not isolated per environment

### 3. Docker Hub or Third-Party Registry
**Pros:** Works across any cloud
**Cons:** Additional costs, slower pulls from AWS

**Decision:** Cross-account ECR access is the best balance of security, cost, and simplicity.

## References

- [AWS ECR Cross-Account Access](https://docs.aws.amazon.com/AmazonECR/latest/userguide/repository-policies.html)
- [ECS Task Execution IAM Role](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task_execution_IAM_role.html)
- [AWS Organizations](https://docs.aws.amazon.com/organizations/latest/userguide/orgs_introduction.html)
