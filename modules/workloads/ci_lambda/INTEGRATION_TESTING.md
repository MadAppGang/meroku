# Integration Testing Guide

## Overview

The Lambda function now includes comprehensive integration testing to verify that all configured services exist in ECS and can be deployed.

## Why Integration Testing?

The new architecture passes **actual ECS resource names** from Terraform instead of constructing them from patterns. This means:

✅ **No naming mismatches** - We use exact names Terraform created
✅ **Verifiable configuration** - Test that all services actually exist
✅ **Faster debugging** - Catch configuration errors before deployment
✅ **Confidence in production** - Know your Lambda will work before you need it

## New Architecture

### Before (Pattern-Based)
```
Lambda constructs names from patterns:
- Pattern: {project}_service_{name}_{env}
- Lambda guesses: "myproject_service_api_dev"
- Risk: Pattern mismatch = deployment failure
```

### After (Direct Lookup)
```
Terraform tells Lambda exact names:
- Terraform: "myproject_service_api_dev"
- Lambda uses: "myproject_service_api_dev"
- Risk: Zero - Lambda knows exact names
```

## Configuration Structure

### Terraform Side (`lambda.tf`)

Three new environment variables passed to Lambda:

**1. ECS_CLUSTER_NAME** - Actual cluster name
```hcl
ECS_CLUSTER_NAME = aws_ecs_cluster.main.name
# Example: "myproject_cluster_dev"
```

**2. ECS_SERVICE_MAP** - Maps service IDs to ECS resources
```json
{
  "": {
    "service_name": "myproject_service_dev",
    "task_family": "myproject_service_dev"
  },
  "api": {
    "service_name": "myproject_service_api_dev",
    "task_family": "myproject_service_api_dev"
  },
  "worker": {
    "service_name": "myproject_service_worker_dev",
    "task_family": "myproject_service_worker_dev"
  }
}
```

**3. S3_SERVICE_MAP** - Maps services to their S3 env files
```json
{
  "api": [
    {
      "bucket": "myproject-env-dev",
      "key": "api/.env"
    }
  ],
  "worker": [
    {
      "bucket": "myproject-env-dev",
      "key": "worker/.env"
    }
  ]
}
```

### Lambda Side (`config/config_v2.go`)

Simple lookups - no pattern construction:

```go
// Get actual service name
serviceName, err := config.GetServiceName("api")
// Returns: "myproject_service_api_dev"

// Get actual cluster name
clusterName := config.GetClusterName()
// Returns: "myproject_cluster_dev"

// Find services affected by S3 file
services := config.GetServicesForS3File("myproject-env-dev", "api/.env")
// Returns: ["api"]
```

## Running Integration Tests

### Prerequisites

1. **Deployed Infrastructure** - ECS cluster and services must exist
2. **AWS Credentials** - Configured for the target environment
3. **Environment Variables** - Same as Lambda function uses

### Local Testing

```bash
# 1. Export required environment variables
export PROJECT_NAME="myproject"
export PROJECT_ENV="dev"
export AWS_REGION="us-east-1"
export ECS_CLUSTER_NAME="myproject_cluster_dev"
export ECS_SERVICE_MAP='{
  "": {"service_name": "myproject_service_dev", "task_family": "myproject_service_dev"},
  "api": {"service_name": "myproject_service_api_dev", "task_family": "myproject_service_api_dev"}
}'
export S3_SERVICE_MAP='{
  "api": [{"bucket": "myproject-env-dev", "key": "api/.env"}]
}'

# 2. Run integration test
cd modules/workloads/ci_lambda
make integration-test
```

### Expected Output

```
🧪 Running Lambda Integration Tests...

📋 Step 1: Loading configuration from environment variables
✅ Configuration loaded successfully
   Project: myproject
   Environment: dev
   Cluster: myproject_cluster_dev
   Services configured: 2

🔌 Step 2: Connecting to AWS ECS
✅ Connected to ECS in region us-east-1

📝 Step 3: Listing all configured services
Found 2 services in configuration:
   • (backend)
   {
     "cluster_name": "myproject_cluster_dev",
     "identifier": "",
     "service_name": "myproject_service_dev",
     "task_family": "myproject_service_dev",
     "s3_files": []
   }
   • api
   {
     "cluster_name": "myproject_cluster_dev",
     "identifier": "api",
     "service_name": "myproject_service_api_dev",
     "task_family": "myproject_service_api_dev",
     "s3_files": [
       {
         "bucket": "myproject-env-dev",
         "key": "api/.env"
       }
     ]
   }

✅ Step 4: Verifying all services exist in ECS
   Checking service: (backend) → myproject_service_dev... ✅ OK
   Checking service: api → myproject_service_api_dev... ✅ OK

📊 Test Summary
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Total services:    2
Verified:          2
Failed:            0

✅ ALL INTEGRATION TESTS PASSED!

The Lambda function is ready to deploy services:
   ✓ backend (myproject_service_dev)
   ✓ api (myproject_service_api_dev)

📦 S3 File Mappings:
   api:
      - s3://myproject-env-dev/api/.env
```

### Failure Example

If a service doesn't exist:

```
✅ Step 4: Verifying all services exist in ECS
   Checking service: (backend) → myproject_service_dev... ✅ OK
   Checking service: api → myproject_service_api_dev... ❌ FAILED
      Error: service not found: myproject_service_api_dev

📊 Test Summary
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Total services:    2
Verified:          1
Failed:            1

❌ INTEGRATION TEST FAILED
Failed services:
   • api

Please check:
  1. ECS_SERVICE_MAP environment variable is correct
  2. All services have been deployed to ECS
  3. AWS credentials and region are correct
```

## Testing in CI/CD

### GitHub Actions Example

```yaml
name: Test Lambda

on: [push]

jobs:
  integration-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2

      - uses: actions/setup-go@v2
        with:
          go-version: '1.21'

      - name: Configure AWS Credentials
        uses: aws-actions/configure-aws-credentials@v1
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: us-east-1

      - name: Get ECS configuration from Terraform
        run: |
          cd env/dev
          terraform init
          echo "ECS_CLUSTER_NAME=$(terraform output -raw ecs_cluster_name)" >> $GITHUB_ENV
          echo "ECS_SERVICE_MAP=$(terraform output -json ecs_service_map)" >> $GITHUB_ENV

      - name: Run Integration Tests
        env:
          PROJECT_NAME: ${{ secrets.PROJECT_NAME }}
          PROJECT_ENV: dev
        run: |
          cd modules/workloads/ci_lambda
          make integration-test
```

## Troubleshooting

### Test Fails: "Configuration validation failed"

**Problem**: Missing required environment variables

**Solution**:
```bash
# Check all required vars are set
echo $PROJECT_NAME
echo $PROJECT_ENV
echo $ECS_CLUSTER_NAME
echo $ECS_SERVICE_MAP
```

### Test Fails: "Service not found in ECS"

**Problem**: Service exists in config but not deployed to ECS

**Solution**:
```bash
# Verify service exists in ECS
aws ecs describe-services \
  --cluster myproject_cluster_dev \
  --services myproject_service_api_dev

# If not found, deploy infrastructure
cd env/dev
terraform apply
```

### Test Fails: "Failed to parse ECS_SERVICE_MAP"

**Problem**: Invalid JSON in environment variable

**Solution**:
```bash
# Validate JSON
echo $ECS_SERVICE_MAP | jq .

# Common issues:
# - Missing quotes around keys
# - Trailing commas
# - Unescaped quotes
```

### Test Fails: "Cannot connect to ECS"

**Problem**: AWS credentials or permissions

**Solution**:
```bash
# Check credentials
aws sts get-caller-identity

# Test ECS access
aws ecs list-clusters

# Required IAM permissions:
# - ecs:DescribeServices
# - ecs:ListTaskDefinitions
```

## Manual Verification

If you don't want to run the full integration test, manually verify services:

```bash
# 1. List all ECS services in cluster
aws ecs list-services --cluster myproject_cluster_dev

# 2. Describe a specific service
aws ecs describe-services \
  --cluster myproject_cluster_dev \
  --services myproject_service_api_dev \
  | jq '.services[0] | {serviceName, status, runningCount, desiredCount}'

# 3. List task definitions for a service
aws ecs list-task-definitions \
  --family-prefix myproject_service_api_dev \
  --sort DESC \
  --max-results 5
```

## Benefits of This Approach

### 1. Eliminates Pattern Errors
**Before**: Lambda might construct "myproject-service-api-dev" but Terraform created "myproject_service_api_dev"
**After**: Lambda uses exact name from Terraform

### 2. Testable Before Deployment
**Before**: Only know if names work when Lambda runs in production
**After**: Integration test catches errors immediately

### 3. Self-Documenting
**Before**: Need to read code to understand naming convention
**After**: JSON config shows exact names

### 4. Easy to Debug
**Before**: "Deployment failed" - why? Wrong pattern? Typo? Service doesn't exist?
**After**: Integration test tells you exactly what's wrong

### 5. Supports Non-Standard Naming
**Before**: Locked into specific naming pattern
**After**: Any naming convention works - Terraform provides exact names

## Next Steps

After integration tests pass:

1. **Deploy Lambda**: `terraform apply`
2. **Test ECR Event**: Push Docker image
3. **Test Manual Deploy**: Trigger via EventBridge
4. **Monitor Logs**: Watch CloudWatch for JSON-structured logs

## Related Documentation

- [README_REFACTORED.md](./README_REFACTORED.md) - Full Lambda architecture
- [REFACTORING_SUMMARY.md](./REFACTORING_SUMMARY.md) - Migration guide
- [NAMING_VERIFICATION.md](./NAMING_VERIFICATION.md) - Naming pattern details
