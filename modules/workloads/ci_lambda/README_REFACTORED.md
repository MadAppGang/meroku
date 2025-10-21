# CI/CD Lambda Function - Refactored Architecture

## Overview

This Lambda function provides automated ECS service deployment triggered by various AWS events:

- **ECR Image Pushes** - Automatically redeploys services when new Docker images are pushed
- **SSM Parameter Changes** - Redeploys services when their configuration parameters change
- **S3 Env File Changes** - Redeploys services when their environment files are modified
- **ECS Deployment Events** - Sends Slack notifications for deployment status
- **Manual Deployments** - Supports manual deployment triggers via EventBridge

## Architecture

### Package Structure

```
ci_lambda/
├── main.go              # Application entry point and initialization
├── config/              # Configuration management
│   └── config.go       # Environment variable loading and validation
├── handlers/            # Event handlers
│   ├── handler.go      # Main event router
│   ├── ecr.go         # ECR image push events
│   ├── ecs.go         # ECS deployment status events
│   ├── ssm.go         # SSM parameter change events
│   ├── s3.go          # S3 object change events
│   └── deploy.go      # Manual deployment events
├── services/           # AWS service clients
│   ├── ecs.go        # ECS operations (deployments, task definitions)
│   └── slack.go      # Slack notifications
├── deployer/           # Deployment orchestration
│   └── deployer.go   # Deployment logic with retries and notifications
└── utils/              # Utilities
    ├── logger.go               # Structured logging for CloudWatch
    └── service_name_extractor.go  # Service name parsing
```

### Event Flow

```
EventBridge Event
        ↓
Main Handler (handlers/handler.go)
        ↓
Event-Specific Handler (handlers/*.go)
        ↓
Deployer (deployer/deployer.go)
        ↓
    ┌─────────┬────────────┐
    ↓         ↓            ↓
ECS Service  Retry Logic  Slack Service
    ↓                      ↓
UpdateService API    Send Notifications
```

### Design Principles

1. **Separation of Concerns** - Clear boundaries between configuration, event handling, deployment logic, and AWS service interactions
2. **Dependency Injection** - All components receive their dependencies through constructors
3. **Testability** - Interfaces for all external services (ECS, Slack, HTTP)
4. **Structured Logging** - JSON-formatted logs with contextual fields for CloudWatch Insights
5. **Retry Logic** - Configurable retry behavior with exponential backoff
6. **Feature Flags** - Enable/disable specific monitoring capabilities via environment variables
7. **Dry Run Mode** - Test deployment logic without actual ECS updates

## Environment Variables

### Core Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `PROJECT_NAME` | Yes | - | Project identifier used in resource naming |
| `PROJECT_ENV` | Yes | - | Environment (dev, staging, prod) |
| `AWS_REGION` | No | `us-east-1` | AWS region for ECS operations |

### Logging Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `LOG_LEVEL` | `info` | Logging verbosity: `debug`, `info`, `warn`, `error` |

### Naming Patterns

Customize how AWS resources are named. Use placeholders:
- `{project}` - Project name
- `{env}` - Environment
- `{name}` - Service name (empty for backend service)

| Variable | Default | Description |
|----------|---------|-------------|
| `CLUSTER_NAME_PATTERN` | `{project}_cluster_{env}` | ECS cluster naming |
| `SERVICE_NAME_PATTERN` | `{project}_service_{name}_{env}` | ECS service naming |
| `TASK_FAMILY_PATTERN` | `{project}_service_{name}_{env}` | Task definition family naming |
| `BACKEND_SERVICE_NAME` | `` | Name for backend service (empty = default) |

### Slack Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SLACK_WEBHOOK_URL` | - | Slack webhook URL for notifications |
| `ENABLE_SLACK_NOTIFICATIONS` | `true` | Enable/disable Slack notifications |

### S3 Service Configuration

| Variable | Required | Description |
|----------|----------|-------------|
| `SERVICE_CONFIG` | Yes | JSON mapping of services to S3 env files |

**Format:**
```json
{
  "api": [
    {"bucket": "env-bucket", "key": "api/.env"}
  ],
  "worker": [
    {"bucket": "env-bucket", "key": "worker/.env"}
  ]
}
```

### Deployment Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DEPLOYMENT_TIMEOUT_SECONDS` | `600` | Maximum time to wait for deployment |
| `MAX_DEPLOYMENT_RETRIES` | `2` | Number of retry attempts for failed deployments |
| `DRY_RUN` | `false` | Test mode - logs actions without deploying |

### Feature Flags

| Variable | Default | Description |
|----------|---------|-------------|
| `ENABLE_ECR_MONITORING` | `true` | Auto-deploy on ECR image pushes |
| `ENABLE_SSM_MONITORING` | `true` | Auto-deploy on SSM parameter changes |
| `ENABLE_S3_MONITORING` | `true` | Auto-deploy on S3 env file changes |
| `ENABLE_MANUAL_DEPLOY` | `true` | Allow manual deployment triggers |

## Event Handlers

### 1. ECR Image Push Events (`aws.ecr`)

**Trigger:** New Docker image pushed to ECR repository

**Event Pattern:**
```json
{
  "source": ["aws.ecr"],
  "detail-type": ["ECR Image Action"],
  "detail": {
    "action-type": ["PUSH"],
    "result": ["SUCCESS"]
  }
}
```

**Behavior:**
1. Extracts service name from repository name
2. Triggers deployment with latest task definition
3. Sends Slack notification on completion

**Repository Naming:**
- Backend service: `{project}_backend`
- Other services: `{project}_service_{name}`

### 2. SSM Parameter Changes (`aws.ssm`)

**Trigger:** SSM Parameter Store parameter updated

**Event Pattern:**
```json
{
  "source": ["aws.ssm"],
  "detail-type": ["Parameter Store Change"]
}
```

**Parameter Path Format:** `/{env}/{project}/{service}/PARAMETER_NAME`

**Example:** `/dev/myproject/api/DATABASE_URL`

**Behavior:**
1. Parses service name from parameter path
2. Triggers deployment to reload environment variables
3. Skips parameters that don't match expected format

### 3. S3 Object Changes (`aws.s3`)

**Trigger:** S3 object modified (via CloudTrail events)

**Event Pattern:**
```json
{
  "source": ["aws.s3"],
  "detail-type": ["AWS API Call via CloudTrail"],
  "detail": {
    "eventName": ["PutObject", "DeleteObject"]
  }
}
```

**Behavior:**
1. Identifies services using the modified S3 file (from `SERVICE_CONFIG`)
2. Deploys all affected services simultaneously
3. Aggregates results and reports failures

### 4. ECS Deployment Status (`aws.ecs`)

**Trigger:** ECS deployment state changes

**Event Names:**
- `SERVICE_DEPLOYMENT_IN_PROGRESS`
- `SERVICE_DEPLOYMENT_COMPLETED` ✅
- `SERVICE_DEPLOYMENT_FAILED` ❌
- `SERVICE_TASK_START_IMPAIRED` ⚠️
- `SERVICE_STEADY_STATE` (ignored - too noisy)

**Behavior:**
- Sends Slack notifications based on deployment status
- No deployment action - observation only

### 5. Manual Deployments (`action.production`, `action.deploy`)

**Trigger:** Custom EventBridge event

**Event Format:**
```json
{
  "service": "api",
  "task_definition": "optional-task-def-arn",
  "reason": "Hotfix deployment"
}
```

**CLI Example:**
```bash
aws events put-events --entries \
  'Source=action.production,DetailType=DEPLOY,Detail="{\"service\":\"api\",\"reason\":\"Hotfix\"}",EventBusName=default'
```

**Behavior:**
1. Validates service field is present
2. Deploys specified service
3. Uses provided task definition or latest if not specified

## Deployment Logic

### Deployment Flow

1. **Normalize Service Name** - Handle backend service special case
2. **Find Latest Task Definition** - Query ECS for most recent task def
3. **Dry Run Check** - Skip actual deployment if `DRY_RUN=true`
4. **Update Service** - Call ECS UpdateService with `ForceNewDeployment`
5. **Retry on Failure** - Exponential backoff retry logic
6. **Send Notifications** - Slack notifications for success/failure

### Retry Behavior

```
Attempt 1: Immediate
Attempt 2: Wait 5 seconds
Attempt 3: Wait 10 seconds
```

Max retries controlled by `MAX_DEPLOYMENT_RETRIES` environment variable.

## Logging

### Log Format

All logs are JSON-formatted for CloudWatch Insights:

```json
{
  "timestamp": "2025-01-21T10:30:45Z",
  "level": "info",
  "message": "Deployment completed successfully",
  "project_name": "myproject",
  "environment": "prod",
  "fields": {
    "service": "api",
    "deployment_id": "ecs-deploy-123",
    "task_definition": "myproject_service_api_prod:42"
  }
}
```

### CloudWatch Insights Queries

**Find all deployments for a service:**
```
fields @timestamp, message, fields.service, fields.deployment_id
| filter fields.service = "api"
| filter message like /deployment/
| sort @timestamp desc
```

**Find failed deployments:**
```
fields @timestamp, message, fields.service, fields.error
| filter level = "error"
| filter message like /deployment failed/
| sort @timestamp desc
```

**Track retry attempts:**
```
fields @timestamp, fields.service, fields.attempt, fields.max_retries
| filter message like /retry/
| sort @timestamp desc
```

## Slack Notifications

### Notification Types

1. **Success** ✅ - Green checkmark, deployment completed
2. **Error** ❌ - Red X, deployment failed
3. **Info** ℹ️ - Blue info icon, deployment started or in progress

### Message Format

Slack messages include:
- Environment (dev/staging/prod)
- Service name
- Deployment status
- Deployment ID
- Task definition (for successful deployments)
- Failure reason (for errors)

## Testing

### Dry Run Mode

Enable dry run to test without actual deployments:

```bash
# In Terraform
environment {
  variables = {
    DRY_RUN = "true"
  }
}
```

Lambda will:
- Process all events normally
- Log what it would deploy
- Skip actual ECS UpdateService calls
- Send Slack notifications (if enabled)

### Manual Testing

**Test ECR deployment:**
```bash
# Push image to ECR
docker push 123456789012.dkr.ecr.us-east-1.amazonaws.com/myproject_service_api:latest

# Lambda will auto-trigger
```

**Test SSM deployment:**
```bash
aws ssm put-parameter \
  --name "/dev/myproject/api/DATABASE_URL" \
  --value "postgresql://..." \
  --type SecureString \
  --overwrite
```

**Test manual deployment:**
```bash
aws events put-events --entries \
  'Source=action.production,DetailType=DEPLOY,Detail="{\"service\":\"api\"}",EventBusName=default'
```

## Troubleshooting

### Common Issues

**1. Deployment fails: "no task definitions found"**

Check:
- Task definition family name matches pattern
- At least one task definition exists for the service
- `TASK_FAMILY_PATTERN` environment variable is correct

**2. Slack notifications not sent**

Check:
- `ENABLE_SLACK_NOTIFICATIONS` is `true`
- `SLACK_WEBHOOK_URL` is set correctly
- Webhook URL is not expired
- Check Lambda logs for HTTP errors

**3. Service name extraction fails**

Check:
- ECR repository naming follows convention
- SSM parameter path follows format `/{env}/{project}/{service}/param`
- `PROJECT_NAME` matches your naming

**4. Multiple deployments from S3 changes**

Check:
- `SERVICE_CONFIG` JSON is valid
- Multiple services may legitimately use the same env file

### Debug Logging

Enable debug logging for verbose output:

```bash
LOG_LEVEL = "debug"
```

Debug logs include:
- Full event payloads
- Task definition selection logic
- HTTP request/response details
- Retry attempt details

## Building and Deploying

### Build the Lambda

```bash
cd modules/workloads/ci_lambda
GOOS=linux GOARCH=amd64 go build -o bootstrap main.go
```

### Dependencies

The Lambda uses Go modules. Key dependencies:
- `github.com/aws/aws-lambda-go` - Lambda runtime
- `github.com/aws/aws-sdk-go` - AWS service clients

### Deploy with Terraform

```bash
# Generate environment files
make infra-gen-dev

# Initialize and apply
make infra-init env=dev
make infra-apply env=dev
```

## Migration from Old Code

The refactored code maintains backward compatibility with existing deployments.

### Key Changes

1. **New package structure** - Better code organization
2. **Enhanced environment variables** - More configuration options
3. **Structured logging** - JSON logs instead of fmt.Printf
4. **Retry logic** - Automatic retries for failed deployments
5. **Feature flags** - Enable/disable specific monitors
6. **Dry run mode** - Test without deploying

### Migration Steps

1. Update Lambda code (replace `bootstrap` binary)
2. Update Terraform environment variables (backward compatible)
3. Test in dev environment first
4. Monitor CloudWatch logs for structured JSON logs
5. Gradually enable new features (retries, dry run)

### Backward Compatibility

- All existing environment variables still work
- New variables have sensible defaults
- Naming patterns default to existing conventions
- Event handling behavior unchanged

## Future Enhancements

Potential improvements:

1. **Deployment Strategies** - Blue/green, canary deployments
2. **Health Checks** - Wait for service health before declaring success
3. **Rollback Logic** - Automatic rollback on deployment failure
4. **Metrics** - CloudWatch metrics for deployment success rate
5. **Multi-Region** - Support deployments across multiple regions
6. **Approval Workflows** - Manual approval for prod deployments
7. **Custom Webhooks** - Support additional notification channels
8. **Deployment Pipelines** - Coordinate deployments across services

## License

Managed by Meroku - Infrastructure as Code
