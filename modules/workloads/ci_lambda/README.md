# CI/CD Lambda - Automated ECS Deployment

This Lambda function provides automated deployment for ECS services in response to infrastructure events:
- **ECR Image Push** - Automatically deploys when new Docker images are pushed
- **S3 Env File Changes** - Redeploys services when environment files are updated
- **SSM Parameter Changes** - Triggers deployment when SSM parameters change
- **Manual Deployments** - Supports explicit production deployments via EventBridge

## Architecture

### V2 Architecture: Hybrid Approach

The V2 architecture uses a **two-phase** approach that eliminates fragile pattern-based ECS resource naming:

**Phase 1: Extract Service Identifier (Pattern-Based - Safe)**
```
ECR Repository: "myproject_service_api"
  → Extract service ID: "api" (using safe pattern - we control both sides)
```

**Phase 2: Lookup Actual ECS Resources (Direct - V2 Innovation)**
```
Service ID: "api"
  → Lookup in ECS_SERVICE_MAP["api"]
  → Get exact names: service_name="myproject_service_api_dev"
```

**Why This Works:**
- ✅ ECR repository names are **defined by us** in Terraform
- ✅ Pattern extraction ECR→serviceID is **safe** (we control naming)
- ✅ ServiceID→ECS resources uses **exact names** from Terraform (zero risk)
- ✅ The fragile pattern construction (V1 problem) is **eliminated**

**Environment Variables:**
```bash
ECS_CLUSTER_NAME = "myproject_cluster_dev"
ECS_SERVICE_MAP = '{"api": {"service_name": "myproject_service_api_dev", "task_family": "myproject_task_api_dev"}}'
S3_SERVICE_MAP = '{"api": [{"bucket": "myproject-env-dev", "key": "api/.env"}]}'
```

**Key Improvement Over V1:**
- ❌ V1: Constructed ECS names from patterns → risky pattern mismatch
- ✅ V2: Terraform tells Lambda exact ECS names → zero mismatch risk

## Project Structure

```
ci_lambda/
├── main.go                    # V2 entry point (uses direct naming)
├── go.mod, go.sum            # Dependencies
├── Makefile                  # Build automation
├── bootstrap                 # Compiled binary (17MB)
│
├── config/
│   └── config.go             # V2 config with direct resource names
│
├── services/
│   ├── ecs.go                # V2 ECS service client
│   └── slack.go              # Slack notifications
│
├── deployer/
│   └── deployer.go           # V2 deployment orchestration
│
├── handlers/
│   └── handler.go            # V2 event handlers (ECR, S3, SSM)
│
├── utils/
│   ├── logger.go             # Structured logging
│   └── service_name_extractor.go  # ECR repo parsing
│
└── cmd/
    └── integration_test.go   # Integration tests
```

## Build

The Lambda is built as a Go binary for AWS Lambda's provided.al2 runtime:

```bash
# Using Makefile (recommended)
make build

# Manual build
GOOS=linux GOARCH=amd64 go build -o bootstrap main.go
```

The compiled `bootstrap` binary is ready to be packaged by Terraform.

## Configuration

### Required Environment Variables

Set in `modules/workloads/lambda.tf`:

```hcl
# Core Configuration
PROJECT_NAME = "myproject"
PROJECT_ENV = "dev"
# Note: AWS_REGION is automatically set by Lambda runtime (reserved variable)

# ECS Resource Names (direct from Terraform)
ECS_CLUSTER_NAME = aws_ecs_cluster.main.name
ECS_SERVICE_MAP = local.ecs_service_map
S3_SERVICE_MAP = local.s3_to_service_map

# Slack Configuration
SLACK_WEBHOOK_URL = "https://hooks.slack.com/..."
ENABLE_SLACK_NOTIFICATIONS = "true"

# Deployment Configuration
DEPLOYMENT_TIMEOUT_SECONDS = "600"
MAX_DEPLOYMENT_RETRIES = "2"
DRY_RUN = "false"

# Feature Flags
ENABLE_ECR_MONITORING = "true"
ENABLE_SSM_MONITORING = "true"
ENABLE_S3_MONITORING = "true"
ENABLE_MANUAL_DEPLOY = "true"
```

### Event Monitoring

The Lambda listens for multiple event types via EventBridge:

1. **ECR Image Push**
   - Source: `aws.ecr`
   - Detail Type: `ECR Image Action`
   - Triggers: Automatic deployment when new image is pushed

2. **S3 Environment File Changes**
   - Source: `aws.s3`
   - Detail Type: `AWS API Call via CloudTrail`
   - Triggers: Deployment when `.env` files are updated

3. **SSM Parameter Changes**
   - Source: `aws.ssm`
   - Detail Type: `Parameter Store Change`
   - Triggers: Deployment when SSM parameters are modified

4. **Manual Production Deployments**
   - Source: `action.production`
   - Detail Type: `DEPLOY`
   - Triggers: Explicit production deployments only

## Deployment Workflow

### Automatic Deployments (Dev)

Every ECR push or S3/SSM change automatically triggers deployment:

```
ECR Push → EventBridge → Lambda → ECS UpdateService → Slack Notification
```

### Manual Production Deployments

Send explicit deployment event via AWS CLI:

```bash
aws events put-events --entries \
  'Source=action.production,
   DetailType=DEPLOY,
   Detail="{\"service\":\"api\"}",
   EventBusName=default'
```

## Deployment Process

1. **Event Reception** - Lambda receives EventBridge event
2. **Service Identification** - Extract service name from event
3. **Resource Lookup** - Get exact ECS service name from config
4. **Task Definition** - Fetch latest task definition
5. **Service Update** - Trigger ECS deployment
6. **Wait for Stability** - Monitor deployment status (max 10 minutes)
7. **Slack Notification** - Send success/failure notification

## Logging

Structured JSON logging with configurable levels:

```go
logger.Info("Deployment started", map[string]interface{}{
    "service": "api",
    "cluster": "myproject_cluster_dev",
    "task_definition": "myproject_task_api_dev:42",
})
```

**Log Levels:** `debug`, `info`, `warn`, `error`

Set via `LOG_LEVEL` environment variable.

## Testing

### Unit Tests
```bash
go test ./...
```

### Integration Tests
```bash
go test -v ./cmd/integration_test.go
```

Integration tests verify:
- Config loading from environment
- ECS service name resolution
- S3 bucket mapping
- Event handler routing

## IAM Permissions

Required IAM permissions (configured in `lambda.tf`):

```json
{
  "Effect": "Allow",
  "Action": [
    "ecs:DescribeTaskDefinition",
    "ecs:ListTaskDefinitions",
    "ecs:UpdateService",
    "iam:PassRole"
  ],
  "Resource": "*"
}
```

Plus standard Lambda execution role for CloudWatch Logs.

## Slack Notifications

The Lambda sends deployment notifications to Slack:

**Success:**
```
✅ Deployment Successful
Service: api
Environment: dev
Task: myproject_task_api_dev:42
```

**Failure:**
```
❌ Deployment Failed
Service: api
Error: Service deployment timeout
```

**Templates:**
- `slack.message.success.json.tmpl`
- `slack.message.error.json.tmpl`
- `slack.message.info.json.tmpl`

## Troubleshooting

### Service Not Found
- Check `ECS_SERVICE_MAP` contains correct service name
- Verify service exists in AWS ECS console
- Check CloudWatch Logs for exact name being used

### Deployment Timeout
- Increase `DEPLOYMENT_TIMEOUT_SECONDS` (default: 600)
- Check ECS service health in AWS console
- Review task definition for issues

### No Slack Notifications
- Verify `SLACK_WEBHOOK_URL` is set
- Check `ENABLE_SLACK_NOTIFICATIONS = "true"`
- Test webhook URL manually

## Monitoring

**CloudWatch Logs:**
- Log Group: `/aws/lambda/ci_lambda_{env}`
- Retention: 30 days (configurable in Terraform)

**Metrics:**
- Lambda invocations
- Lambda errors
- Lambda duration
- ECS deployment status

## Development

### Adding New Event Handler

1. Update event pattern in `lambda.tf`:
```hcl
event_pattern = jsonencode({
  source = ["aws.newservice"]
  detail-type = ["New Event Type"]
})
```

2. Add handler in `handlers/handler.go`:
```go
func (h *EventHandlerV2) HandleNewEvent(ctx context.Context, event Event) error {
    // Implementation
}
```

3. Update routing in `main.go`

### Modifying Deployment Behavior

Edit `deployer/deployer.go` to customize:
- Timeout values
- Retry logic
- Deployment strategies
- Health checks

## Migration Notes

This codebase uses **V2 Architecture** with direct resource naming. All V1 pattern-based code has been removed.

**Key Changes:**
- ✅ Direct resource names from Terraform (no pattern construction)
- ✅ Simplified config package (`config/config.go`)
- ✅ Modular package structure (services, deployer, handlers)
- ✅ Comprehensive integration tests
- ❌ Removed backward compatibility code
- ❌ Removed pattern-based naming logic

## References

- AWS Lambda Go SDK: https://github.com/aws/aws-lambda-go
- ECS UpdateService: https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_UpdateService.html
- EventBridge Events: https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-events.html
