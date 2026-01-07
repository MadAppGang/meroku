# Implementation Plan: Auto-Create DynamoDB State Lock Table

## Overview

Add automatic DynamoDB table creation to meroku, following the same pattern as S3 bucket auto-creation. The table will be created during AWS pre-flight checks, before Terraform needs it.

## Changes Required

### 1. New File: `app/dynamodb.go`

Create DynamoDB table management functions:
- `checkDynamoDBTableForEnv(env Env) error` - Check/create DynamoDB table
- `checkDynamoDBTableForEnvWithRetry(env Env, isRetry bool) error` - With SSO retry logic

DynamoDB table requirements:
- Table name: `{project}-terraform-locks-{env}` (convention matching S3 bucket naming)
- Hash key: `LockID` (String) - Required by Terraform
- Billing mode: PAY_PER_REQUEST (on-demand, most cost-effective)

### 2. Update: `app/aws_preflight.go`

Add Step 7 after S3 bucket check:
- Check if `state_lock_table` is configured
- If configured, check/create the DynamoDB table
- Follow same error handling pattern as S3

### 3. Update: `app/migrations.go`

Modify `migrateToV16` to:
- Auto-generate `state_lock_table` name as `{project}-terraform-locks-{env}`
- Only if user hasn't already specified a custom name
- Inform user of the auto-generated table name

### 4. Update go.mod (if needed)

Ensure DynamoDB SDK is available:
- `github.com/aws/aws-sdk-go-v2/service/dynamodb`

## File Structure

```
app/
├── dynamodb.go      # NEW - DynamoDB table management
├── aws_preflight.go # UPDATE - Add DynamoDB check
├── migrations.go    # UPDATE - Auto-set table name
├── s3.go           # REFERENCE - Similar pattern
└── model.go        # Already has StateLockTable field
```

## Implementation Order

1. Create `dynamodb.go` with table creation logic
2. Update `aws_preflight.go` to call the DynamoDB check
3. Update `migrateToV16` to auto-generate table name
4. Test with `go build` and `go test`

## Table Naming Convention

Following existing naming patterns in meroku:
- S3 bucket: `{project}-terraform-state-{env}`
- DynamoDB table: `{project}-terraform-locks-{env}`

Example:
- Project: `myapp`, Env: `dev`
- S3: `myapp-terraform-state-dev`
- DynamoDB: `myapp-terraform-locks-dev`

## Error Handling

Same pattern as S3:
1. Try to list/describe tables
2. If SSO token expired, retry with `aws sso login`
3. If table doesn't exist, create it
4. Return clear error messages with recovery steps
