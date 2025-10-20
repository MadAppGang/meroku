# AWS Pre-Flight Checks - Self-Recovery System

## Overview

The AWS Pre-Flight Check system provides **automatic validation and recovery** before any Terraform operations. This prevents cryptic Terraform errors by catching and fixing AWS configuration issues early.

## Problem Solved

**Before:** Users would get confusing errors like:
```
Error: error configuring S3 Backend: no valid credential sources for S3 Backend found.
Error: : internal error
status code: 500, request id: ...
```

**After:** Clear, actionable error messages with automatic recovery:
```
🔍 Running AWS pre-flight checks...
✅ AWS_PROFILE set to: meroku2
⚠️  SSO token expired for profile: meroku2
🔄 Refreshing SSO token for profile: meroku2
✅ SSO token refreshed successfully
✅ AWS credentials valid - Account: 123456789, User: arn:aws:...
🪣  Checking S3 state bucket: my-terraform-state
✅ Bucket my-terraform-state already exists
✅ All AWS pre-flight checks passed!
```

## Architecture

### Pre-Flight Check Sequence

```
┌─────────────────────────────────────────┐
│    User runs deployment/plan/init       │
└──────────────┬──────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────┐
│  Step 1: Validate AWS_PROFILE            │
│  • Check if AWS_PROFILE env var is set   │
│  • Auto-set from YAML config if missing  │
│  • Error with recovery steps if not set  │
└──────────────┬───────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────┐
│  Step 2: Validate AWS Credentials        │
│  • Call STS GetCallerIdentity API        │
│  • Detect SSO token expiration           │
│  • Auto-refresh SSO token if expired     │
│  • Retry validation after refresh        │
│  • Error with recovery steps if failed   │
└──────────────┬───────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────┐
│  Step 3: Check/Create S3 Bucket          │
│  • List all S3 buckets                   │
│  • Check if state bucket exists          │
│  • Auto-create if missing (with region)  │
│  • Handle SSO refresh if needed          │
│  • Error with recovery steps if failed   │
└──────────────┬───────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────┐
│  ✅ All checks passed                    │
│  Proceed with Terraform operation        │
└──────────────────────────────────────────┘
```

## Implementation

### Core Function

**File:** `app/aws_preflight.go`

```go
// AWSPreflightCheck performs comprehensive AWS setup validation
func AWSPreflightCheck(env Env) error {
    // Step 1: Validate AWS_PROFILE
    // Step 2: Validate AWS Credentials (with SSO auto-refresh)
    // Step 3: Check/Create S3 Bucket
    return nil // if all checks pass
}
```

### Integration Points

1. **`terraformInitIfNeeded()`** - Before terraform init
2. **`runCommandToDeploy()`** - Before deployment
3. Future: Before terraform plan/destroy

### Auto-Recovery Features

#### 1. AWS_PROFILE Auto-Setting
```go
if awsProfile == "" && env.AWSProfile != "" {
    os.Setenv("AWS_PROFILE", env.AWSProfile)
}
```

#### 2. SSO Token Auto-Refresh
```go
if strings.Contains(err.Error(), "SSO") || strings.Contains(err.Error(), "expired") {
    refreshSSOToken(awsProfile)
    // Retry operation after refresh
}
```

#### 3. S3 Bucket Auto-Creation
```go
if !bucketExists {
    client.CreateBucket(ctx, &s3.CreateBucketInput{
        Bucket: aws.String(env.StateBucket),
        // With proper region configuration
    })
}
```

## Error Messages

### Design Principles

1. **Clear problem statement** - What went wrong
2. **Actionable recovery steps** - How to fix it
3. **Specific commands** - Exact commands to run
4. **Context-aware** - Include actual values (profile name, bucket name, etc.)

### Example Error Message

```
❌ AWS credentials validation failed: ExpiredToken: token expired

Recovery steps:
1. Check if your AWS profile exists: aws configure list-profiles
2. For SSO: Run 'aws sso login --profile meroku2'
3. For IAM keys: Run 'aws configure --profile meroku2'
4. Verify credentials: aws sts get-caller-identity --profile meroku2
```

## Success Flow Example

```
🚀 Starting deployment for environment: dev

🔍 Running AWS pre-flight checks...
✅ AWS_PROFILE set to: meroku2
✅ AWS credentials valid - Account: 134726540963, User: arn:aws:sts::134726540963:assumed-role/...
🪣  Checking S3 state bucket: sate-bucket-meroku-dev-0wl2w
✅ Bucket sate-bucket-meroku-dev-0wl2w already exists
✅ All AWS pre-flight checks passed!

🚀 Preparing environment: dev
✅ Terraform already initialized.
Running terraform plan...
```

## Failure Flow Example

```
🚀 Starting deployment for environment: dev

🔍 Running AWS pre-flight checks...
✅ AWS_PROFILE set to: meroku2
⚠️  SSO token expired for profile: meroku2
🔄 Refreshing SSO token for profile: meroku2
✅ SSO token refreshed successfully
🔄 Retrying AWS credential validation...
✅ AWS credentials valid - Account: 134726540963, User: arn:aws:sts::134726540963:assumed-role/...
🪣  Checking S3 state bucket: sate-bucket-meroku-dev-0wl2w
✅ Bucket sate-bucket-meroku-dev-0wl2w already exists
✅ All AWS pre-flight checks passed!
```

## Benefits

### For Users

1. **No more cryptic Terraform errors** - Issues caught early with clear messages
2. **Automatic recovery** - SSO refresh, bucket creation happen automatically
3. **Faster debugging** - Exact commands to fix issues
4. **Better UX** - Clear progress indicators

### For Developers

1. **Reduced support burden** - Users can self-recover
2. **Better error tracking** - Know exactly where failures occur
3. **Easier debugging** - Pre-flight checks log all steps
4. **Future extensibility** - Easy to add more checks

## Future Enhancements

### Potential Additional Checks

1. **IAM Permissions Validation**
   - Check if user has required permissions for Terraform operations
   - S3, EC2, RDS, IAM, etc.

2. **Region Validation**
   - Verify region is valid AWS region
   - Check if region supports required services

3. **VPC Quota Checks**
   - Verify VPC limits not exceeded
   - Check available IP space

4. **Service Quota Validation**
   - Check ECS, RDS, Lambda quotas
   - Warn before hitting limits

5. **State Lock Detection**
   - Check if Terraform state is locked
   - Provide commands to break lock if needed

6. **Network Connectivity**
   - Verify AWS API endpoints are reachable
   - Check for proxy/VPN issues

## Testing

### Manual Testing

```bash
# Test with expired SSO token
unset AWS_SESSION_TOKEN
./meroku  # Select deployment

# Test with missing profile
unset AWS_PROFILE
./meroku  # Select deployment

# Test with non-existent bucket
# (modify dev.yaml to use non-existent bucket name)
./meroku  # Select deployment
```

### Expected Behavior

1. **Expired SSO** - Auto-refresh with `aws sso login`
2. **Missing Profile** - Error with recovery steps
3. **Missing Bucket** - Auto-create with proper region config

## Maintenance

### Adding New Checks

1. Add check function in `aws_preflight.go`
2. Call from `AWSPreflightCheck()`
3. Provide clear error messages with recovery steps
4. Update this documentation

### Updating Error Messages

- Keep messages concise but actionable
- Include specific values (profile names, regions, etc.)
- Provide exact commands to run
- Test with real users for clarity
