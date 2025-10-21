# ✅ V2 Lambda Deployment Ready

## Summary

The Lambda function has been **completely refactored to V2 architecture** and is ready for deployment!

## What Was Completed

### ✅ 1. V2 Architecture Implementation
- **Direct Resource Naming** - No more pattern-based name construction
- **Terraform passes actual ECS names** to Lambda via environment variables
- **Zero risk of naming mismatches** - Lambda uses exact names Terraform created

### ✅ 2. Code Refactoring
**New Package Structure:**
```
ci_lambda/
├── main.go                     # V2 entry point with V2 architecture
├── config/config.go           # Loads actual ECS resource names from env vars
├── services/ecs.go            # ECS operations using direct name lookups
├── deployer/deployer.go       # Deployment orchestration (V2)
├── handlers/handler.go        # All event handlers in one file (V2)
└── utils/
    ├── logger.go              # Structured JSON logging
    └── service_name_extractor.go  # Still used for ECR repo parsing
```

**Old V1 files backed up to:**
```
ci_lambda/_backup_v1/
```

### ✅ 3. Terraform Changes

**File: `/Users/jack/mag/infrastructure/modules/workloads/lambda.tf`**

**New Environment Variables Added:**
```hcl
ECS_CLUSTER_NAME  = aws_ecs_cluster.main.name
ECS_SERVICE_MAP   = local.ecs_service_map
S3_SERVICE_MAP    = local.s3_to_service_map
```

**New Locals Created:**
```hcl
locals {
  // Maps service IDs to actual ECS resource names
  ecs_service_map = jsonencode(merge(
    {
      "" = {
        service_name = aws_ecs_service.backend.name
        task_family  = aws_ecs_task_definition.backend.family
      }
    },
    {
      for key, service in local.service_names : key => {
        service_name = aws_ecs_service.services[key].name
        task_family  = aws_ecs_task_definition.services[key].family
      }
    }
  ))

  // Maps services to S3 env files
  s3_to_service_map = jsonencode({
    for service_name, files in local.services_env_files_s3 : service_name => [
      for file in files : {
        bucket = "${var.project}-${file.bucket}-${var.env}"
        key    = file.key
      }
    ]
  })
}
```

**File: `/Users/jack/mag/infrastructure/modules/workloads/variables.tf`**

**Added Missing Variable:**
```hcl
variable "region" {
  type    = string
  default = "us-east-1"
}
```

### ✅ 4. Lambda Binary Built

**Location:** `/Users/jack/mag/infrastructure/modules/workloads/ci_lambda/bootstrap`
**Size:** 17MB
**Target:** Linux AMD64 (AWS Lambda)
**Status:** ✅ Build successful

## How to Deploy

### From Your Project Directory

```bash
# Navigate to your project
cd /Users/jack/merokudemo1

# Deploy with meroku
./meroku

# Or deploy with Terraform directly
cd env/dev
export AWS_PROFILE=merokutest-dev
terraform init
terraform apply
```

### What Happens During Deployment

1. **Terraform packages the Lambda**
   - Zips `/Users/jack/mag/infrastructure/modules/workloads/ci_lambda/bootstrap`
   - Creates `ci_lambda.zip`

2. **Terraform updates Lambda environment variables**
   ```
   ECS_CLUSTER_NAME  = "test1_cluster_dev"
   ECS_SERVICE_MAP   = {"": {...}, "api": {...}}
   S3_SERVICE_MAP    = {"api": [...]}
   AWS_REGION        = "ap-southeast-2"
   PROJECT_NAME      = "test1"
   PROJECT_ENV       = "dev"
   ```

3. **Lambda initializes with V2 architecture**
   - Loads actual ECS resource names from env vars
   - Validates configuration
   - Ready to deploy services using direct lookups

## Verification After Deployment

### 1. Check Lambda Logs

```bash
# View Lambda logs
aws logs tail /aws/lambda/ci_lambda_dev --follow --profile merokutest-dev

# Look for initialization message:
{
  "timestamp": "2025-01-21T...",
  "level": "info",
  "message": "Lambda function initializing (V2 Architecture)",
  "fields": {
    "architecture": "v2-direct-naming",
    "services_count": 2,
    "cluster": "test1_cluster_dev"
  }
}
```

### 2. Test Manual Deployment

```bash
# Trigger a test deployment
aws events put-events \
  --profile merokutest-dev \
  --entries '[{
    "Source": "action.deploy",
    "DetailType": "DEPLOY",
    "Detail": "{\"service\":\"\"}"
  }]'

# Check logs for deployment
aws logs tail /aws/lambda/ci_lambda_dev --follow --profile merokutest-dev
```

### 3. Test ECR Deployment

```bash
# Push image to ECR (backend)
docker push <account>.dkr.ecr.<region>.amazonaws.com/test1_backend:latest

# Lambda should auto-deploy backend service
# Check logs for:
{
  "level": "info",
  "message": "ECR-triggered deployment completed",
  "fields": {
    "service_id": "",
    "service_name": "test1_service_dev",
    "deployment_id": "ecs-svc-..."
  }
}
```

## Key Improvements in V2

### 1. **Zero Naming Risk**
```
❌ V1: Lambda constructs "test1_service_api_dev" (might be wrong)
✅ V2: Terraform tells Lambda "test1_service_api_dev" (always correct)
```

### 2. **Better Logging**
```
❌ V1: fmt.Printf("deploying service %s", name)
✅ V2: Structured JSON logs queryable in CloudWatch Insights
```

### 3. **Retry Logic**
```
❌ V1: Deploy once, fail = done
✅ V2: Configurable retries with exponential backoff
```

### 4. **Feature Flags**
```
✅ V2: Enable/disable monitoring per event type
ENABLE_ECR_MONITORING = "true"
ENABLE_SSM_MONITORING = "true"
ENABLE_S3_MONITORING  = "true"
```

### 5. **Dry Run Mode**
```
✅ V2: Test deployments without actual ECS updates
DRY_RUN = "true"
```

## Environment Variables Reference

### Required (Terraform Provides)
- `PROJECT_NAME` - Project identifier
- `PROJECT_ENV` - Environment (dev/staging/prod)
- `ECS_CLUSTER_NAME` - Actual ECS cluster name
- `ECS_SERVICE_MAP` - Map of service IDs to actual ECS names
- `AWS_REGION` - AWS region

### Optional (Configurable)
- `LOG_LEVEL` - Default: "info" (debug/info/warn/error)
- `SLACK_WEBHOOK_URL` - Slack notifications
- `ENABLE_SLACK_NOTIFICATIONS` - Default: "true"
- `DEPLOYMENT_TIMEOUT_SECONDS` - Default: "600"
- `MAX_DEPLOYMENT_RETRIES` - Default: "2"
- `DRY_RUN` - Default: "false"
- `ENABLE_ECR_MONITORING` - Default: "true"
- `ENABLE_SSM_MONITORING` - Default: "true"
- `ENABLE_S3_MONITORING` - Default: "true"
- `ENABLE_MANUAL_DEPLOY` - Default: "true"

## Files Modified

### Infrastructure Module
1. ✅ `/Users/jack/mag/infrastructure/modules/workloads/lambda.tf` - Added V2 env vars
2. ✅ `/Users/jack/mag/infrastructure/modules/workloads/variables.tf` - Added region variable

### Lambda Code (All New V2 Files)
1. ✅ `main.go` - V2 entry point
2. ✅ `config/config.go` - Direct resource name loading
3. ✅ `services/ecs.go` - ECS operations with direct lookups
4. ✅ `services/slack.go` - Slack notifications (unchanged)
5. ✅ `deployer/deployer.go` - Deployment orchestration
6. ✅ `handlers/handler.go` - All event handlers (V2)
7. ✅ `utils/logger.go` - Structured logging
8. ✅ `bootstrap` - Compiled Lambda binary (17MB)

### Old Files Backed Up
- All old V1 files moved to `_backup_v1/` directory

## Documentation Created

1. ✅ `V2_ARCHITECTURE.md` - Complete V2 architecture explanation
2. ✅ `INTEGRATION_TESTING.md` - Integration testing guide
3. ✅ `README_REFACTORED.md` - Full refactoring documentation
4. ✅ `REFACTORING_SUMMARY.md` - Migration summary
5. ✅ `NAMING_VERIFICATION.md` - Naming pattern verification
6. ✅ `DEPLOYMENT_READY.md` - This file

## Next Steps

1. **Deploy to dev environment**
   ```bash
   cd /Users/jack/merokudemo1
   ./meroku
   # Select deploy option
   ```

2. **Verify deployment in CloudWatch logs**
   - Look for V2 initialization message
   - Check structured JSON logs

3. **Test ECR auto-deployment**
   - Push Docker image
   - Verify Lambda triggers deployment

4. **Test manual deployment**
   - Use EventBridge manual trigger
   - Verify deployment works

5. **Deploy to staging/prod** (when ready)
   - Same process as dev
   - Monitor logs carefully

## Rollback Plan (If Needed)

If issues arise, rollback is simple:

```bash
cd /Users/jack/mag/infrastructure/modules/workloads/ci_lambda

# 1. Restore old main.go
mv _backup_v1/main_old_v1.go main.go

# 2. Restore old files
mv _backup_v1/*.go .
mv _backup_v1/config_v1_old.go config/config.go
mv _backup_v1/ecs_v1_old.go services/ecs.go
mv _backup_v1/deployer_v1_old.go deployer/deployer.go

# 3. Rebuild
GOOS=linux GOARCH=amd64 go build -o bootstrap main.go

# 4. Redeploy from project
cd /Users/jack/merokudemo1
./meroku
```

## Success Criteria

✅ Lambda deploys without errors
✅ CloudWatch shows V2 initialization logs
✅ ECR push triggers deployment
✅ Manual deployment works
✅ Structured JSON logs appear in CloudWatch
✅ Deployment retries work on transient failures

## Questions?

See comprehensive documentation:
- `V2_ARCHITECTURE.md` - Why V2 is better
- `INTEGRATION_TESTING.md` - How to test
- `README_REFACTORED.md` - Complete usage guide

---

**Status: ✅ READY FOR DEPLOYMENT**

Deploy from `/Users/jack/merokudemo1` with meroku!
