# Custom Extensions Guide

Extend your meroku infrastructure with custom Terraform code while maintaining full integration with core modules.

## Table of Contents

- [Quickstart](#quickstart)
- [Extension Types](#extension-types)
- [Step-by-Step Tutorials](#step-by-step-tutorials)
- [YAML Extensions Reference](#yaml-extensions-reference)
- [Real-World Examples](#real-world-examples)
- [Bridge Variables Reference](#bridge-variables-reference)
- [Best Practices](#best-practices)

---

## Quickstart

### 5-Minute Setup: Add SNS Webhook

**Goal**: Create an SNS topic and subscribe your backend API to receive notifications.

**Step 1**: Add to your `dev.yaml`:

```yaml
extensions:
  sns_topics:
    - name: order_events
      add_to_backend_env: AWS_SNS_ORDER_TOPIC_ARN
      webhooks:
        - path: /webhooks/sns/orders
```

**Step 2**: Deploy:

```bash
meroku deploy
```

**That's it!** Your backend now has:
- Environment variable `AWS_SNS_ORDER_TOPIC_ARN` with the topic ARN
- SNS subscription delivering messages to `https://api.yourdomain.com/webhooks/sns/orders`

---

## Extension Types

| Type | Use Case | Bidirectional | Complexity |
|------|----------|---------------|------------|
| **YAML Extensions** | Common patterns (SNS, SQS, Lambda) | Yes | Low |
| **Custom Post-Module** | Resources that consume core outputs | One-way | Medium |
| **Custom Pre-Module** | Resources that feed into core | One-way | Medium |
| **Raw Terraform** | Simple additions | No | Low |

### When to Use Each

```
Need to create SNS/SQS/Lambda?
  └─> Use YAML Extensions

Need to reference API endpoint, ECS cluster, etc.?
  └─> Use Custom Post-Module

Need to pass values TO backend env vars?
  └─> Use YAML Extensions (preferred) or Custom Pre-Module

Just need to add a simple resource?
  └─> Use Raw Terraform files
```

---

## Step-by-Step Tutorials

### Tutorial 1: SNS Topic with Backend Integration

**Scenario**: You need an SNS topic for order events. Your backend should:
1. Know the topic ARN (to publish events)
2. Receive webhook notifications (when events occur)

**Step 1**: Edit your environment YAML:

```yaml
# dev.yaml
extensions:
  sns_topics:
    - name: order_events
      display_name: "Order Events"
      add_to_backend_env: AWS_SNS_ORDER_TOPIC_ARN
      webhooks:
        - path: /api/webhooks/orders
          raw_message_delivery: true
```

**Step 2**: In your backend code, use the environment variable:

```go
// Go example
topicArn := os.Getenv("AWS_SNS_ORDER_TOPIC_ARN")
snsClient.Publish(&sns.PublishInput{
    TopicArn: &topicArn,
    Message:  &message,
})
```

**Step 3**: Handle incoming webhooks:

```go
// Handler at /api/webhooks/orders
func HandleOrderWebhook(w http.ResponseWriter, r *http.Request) {
    body, _ := io.ReadAll(r.Body)

    // Parse SNS message
    var msg SNSMessage
    json.Unmarshal(body, &msg)

    // Process the order event
    processOrder(msg.Message)

    w.WriteHeader(http.StatusOK)
}
```

**Step 4**: Deploy:

```bash
meroku deploy
```

---

### Tutorial 2: SQS Queue with Dead Letter Queue

**Scenario**: Process async jobs with automatic retry and dead-letter queue.

**Step 1**: Add to YAML:

```yaml
extensions:
  sqs_queues:
    - name: job_processor
      add_to_backend_env: AWS_SQS_JOBS_URL
      visibility_timeout: 300  # 5 minutes for long jobs
      dlq_enabled: true
      dlq_max_receive: 3       # Move to DLQ after 3 failures
```

**Step 2**: Backend code to send messages:

```go
queueUrl := os.Getenv("AWS_SQS_JOBS_URL")
sqsClient.SendMessage(&sqs.SendMessageInput{
    QueueUrl:    &queueUrl,
    MessageBody: &jobPayload,
})
```

**Step 3**: Add IAM policy for SQS access in YAML:

```yaml
workload:
  backend_policies:
    - actions:
        - sqs:SendMessage
        - sqs:ReceiveMessage
        - sqs:DeleteMessage
      resources:
        - "arn:aws:sqs:*:*:myproject-*-job_processor"
```

---

### Tutorial 3: Custom Post-Module (Advanced)

**Scenario**: Create a custom CloudWatch dashboard that monitors your infrastructure.

**Step 1**: Create the custom module structure:

```bash
mkdir -p custom/post
```

**Step 2**: Create `custom/post/main.tf`:

```hcl
# custom/post/main.tf

resource "aws_cloudwatch_dashboard" "main" {
  dashboard_name = "${var.project}-${var.env}-overview"

  dashboard_body = jsonencode({
    widgets = [
      {
        type   = "metric"
        x      = 0
        y      = 0
        width  = 12
        height = 6
        properties = {
          title   = "API Gateway Requests"
          region  = var.region
          metrics = [
            ["AWS/ApiGateway", "Count", "ApiId", var.api_gateway_id]
          ]
          period = 300
          stat   = "Sum"
        }
      },
      {
        type   = "metric"
        x      = 12
        y      = 0
        width  = 12
        height = 6
        properties = {
          title   = "ECS CPU Utilization"
          region  = var.region
          metrics = [
            ["AWS/ECS", "CPUUtilization", "ClusterName", var.ecs_cluster_name]
          ]
          period = 300
          stat   = "Average"
        }
      }
    ]
  })
}

# CloudWatch alarm for high API latency
resource "aws_cloudwatch_metric_alarm" "api_latency" {
  alarm_name          = "${var.project}-${var.env}-api-high-latency"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 2
  metric_name         = "Latency"
  namespace           = "AWS/ApiGateway"
  period              = 300
  statistic           = "Average"
  threshold           = 1000  # 1 second
  alarm_description   = "API latency is above 1 second"

  dimensions = {
    ApiId = var.api_gateway_id
  }
}
```

**Step 3**: Create `custom/post/variables.tf`:

```hcl
# custom/post/variables.tf

# Context variables (automatically passed by meroku)
variable "project" { type = string }
variable "env" { type = string }
variable "region" { type = string }
variable "account_id" { type = string }
variable "vpc_id" { type = string }
variable "subnet_ids" { type = list(string) }

# Core module outputs (automatically passed by meroku)
variable "api_endpoint" { type = string }
variable "api_gateway_id" { type = string }
variable "ecs_cluster_arn" { type = string }
variable "ecs_cluster_name" { type = string }
variable "backend_ecr_repo_url" { type = string }
variable "backend_task_role" { type = string }

# Optional outputs (may be empty if module disabled)
variable "domain_zone_id" {
  type    = string
  default = ""
}
variable "db_endpoint" {
  type    = string
  default = ""
}
```

**Step 4**: Create `custom/post/outputs.tf`:

```hcl
# custom/post/outputs.tf

output "dashboard_url" {
  value = "https://${var.region}.console.aws.amazon.com/cloudwatch/home?region=${var.region}#dashboards:name=${aws_cloudwatch_dashboard.main.dashboard_name}"
}
```

**Step 5**: Deploy:

```bash
meroku deploy
```

---

### Tutorial 4: Custom Pre-Module (Feed Values to Core)

**Scenario**: Create a KMS key and use it to encrypt backend secrets.

**Step 1**: Create the pre-module:

```bash
mkdir -p custom/pre
```

**Step 2**: Create `custom/pre/main.tf`:

```hcl
# custom/pre/main.tf

resource "aws_kms_key" "app_secrets" {
  description             = "${var.project}-${var.env} application secrets"
  deletion_window_in_days = 7
  enable_key_rotation     = true

  tags = {
    Project     = var.project
    Environment = var.env
  }
}

resource "aws_kms_alias" "app_secrets" {
  name          = "alias/${var.project}-${var.env}-secrets"
  target_key_id = aws_kms_key.app_secrets.key_id
}
```

**Step 3**: Create `custom/pre/variables.tf`:

```hcl
# custom/pre/variables.tf

variable "project" { type = string }
variable "env" { type = string }
variable "region" { type = string }
variable "account_id" { type = string }
variable "vpc_id" { type = string }
variable "subnet_ids" { type = list(string) }
```

**Step 4**: Create `custom/pre/outputs.tf`:

```hcl
# custom/pre/outputs.tf

# These values are merged into backend env vars
output "backend_env_vars" {
  value = [
    {
      name  = "KMS_KEY_ARN"
      value = aws_kms_key.app_secrets.arn
    },
    {
      name  = "KMS_KEY_ID"
      value = aws_kms_key.app_secrets.key_id
    }
  ]
}

# Add IAM policies for the backend task
output "backend_policies" {
  value = [
    {
      actions = [
        "kms:Encrypt",
        "kms:Decrypt",
        "kms:GenerateDataKey"
      ]
      resources = [aws_kms_key.app_secrets.arn]
    }
  ]
}
```

---

### Tutorial 5: Raw Terraform Files (Simple)

**Scenario**: Add a simple S3 bucket notification to existing infrastructure.

**Step 1**: Create the directory:

```bash
mkdir -p custom/terraform/_shared
```

**Step 2**: Create `custom/terraform/_shared/s3-notification.tf`:

```hcl
# This file is appended to the generated main.tf
# It can reference any module outputs directly

resource "aws_s3_bucket_notification" "media_bucket" {
  bucket = module.s3.buckets["media"].id

  topic {
    topic_arn     = aws_sns_topic.ext_media_events.arn
    events        = ["s3:ObjectCreated:*"]
    filter_prefix = "uploads/"
  }
}
```

**Note**: Raw terraform files have access to all module outputs but cannot feed values back into core modules.

---

## YAML Extensions Reference

### SNS Topics

```yaml
extensions:
  sns_topics:
    - name: string                    # Required: Suffix for topic name
      display_name: string            # Optional: Human-readable name
      add_to_backend_env: string      # Optional: Export ARN as env var
      fifo: bool                      # Optional: FIFO topic (default: false)
      content_based_dedup: bool       # Optional: For FIFO (default: false)
      kms_key_id: string              # Optional: KMS key for encryption
      tags: map                       # Optional: Additional tags
      webhooks:                       # Optional: HTTP(S) subscriptions
        - path: string                # Required: Backend endpoint path
          raw_message_delivery: bool  # Optional: Raw JSON (default: false)
          filter_policy: map          # Optional: Message filtering
```

**Generated Resources**:
- `aws_sns_topic.ext_{name}`
- `aws_sns_topic_subscription.ext_{name}_webhook_{index}` (for each webhook)

### SQS Queues

```yaml
extensions:
  sqs_queues:
    - name: string                    # Required: Suffix for queue name
      add_to_backend_env: string      # Optional: Export URL as env var
      add_arn_to_backend_env: string  # Optional: Export ARN as env var
      fifo: bool                      # Optional: FIFO queue (default: false)
      visibility_timeout: int         # Optional: Seconds (default: 30)
      message_retention: int          # Optional: Seconds (default: 345600)
      max_message_size: int           # Optional: Bytes (default: 262144)
      delay_seconds: int              # Optional: Delivery delay (default: 0)
      receive_wait_time: int          # Optional: Long polling (default: 0)
      dlq_enabled: bool               # Optional: Create DLQ (default: true)
      dlq_max_receive: int            # Optional: Before DLQ (default: 3)
      dlq_retention: int              # Optional: DLQ retention (default: 1209600)
      kms_key_id: string              # Optional: KMS encryption
      sns_subscriptions:              # Optional: Subscribe to SNS topics
        - topic: string               # Name of SNS topic (from extensions)
          raw_message_delivery: bool  # Optional (default: false)
          filter_policy: map          # Optional: Message filtering
```

**Generated Resources**:
- `aws_sqs_queue.ext_{name}`
- `aws_sqs_queue.ext_{name}_dlq` (if dlq_enabled)
- `aws_sqs_queue_redrive_policy.ext_{name}` (if dlq_enabled)
- `aws_sns_topic_subscription.ext_{name}_sns_{topic}` (for each sns_subscription)

### Lambda Functions

```yaml
extensions:
  lambda_functions:
    - name: string                    # Required: Function name
      runtime: string                 # Required: nodejs20.x, python3.12, etc.
      handler: string                 # Required: Handler path
      source_dir: string              # Required: Path to source code
      add_to_backend_env: string      # Optional: Export ARN as env var
      memory: int                     # Optional: MB (default: 128)
      timeout: int                    # Optional: Seconds (default: 30)
      environment: map                # Optional: Function env vars
      vpc_enabled: bool               # Optional: Run in VPC (default: false)
      layers: list                    # Optional: Lambda layer ARNs
      reserved_concurrency: int       # Optional: Reserved concurrency
      triggers:                       # Optional: Event triggers
        - type: sns                   # SNS trigger
          topic: string               # Topic name from extensions
        - type: sqs                   # SQS trigger
          queue: string               # Queue name from extensions
          batch_size: int             # Optional (default: 10)
        - type: s3                    # S3 trigger
          bucket: string              # Bucket name
          events: list                # S3 events
          prefix: string              # Optional key prefix
          suffix: string              # Optional key suffix
        - type: schedule              # CloudWatch Events schedule
          expression: string          # rate() or cron()
        - type: api_gateway           # API Gateway route
          method: string              # GET, POST, etc.
          path: string                # Route path
```

### EventBridge Rules

```yaml
extensions:
  eventbridge_rules:
    - name: string                    # Required: Rule name
      description: string             # Optional: Description
      schedule_expression: string     # Optional: rate() or cron()
      event_pattern: map              # Optional: Event pattern JSON
      enabled: bool                   # Optional: (default: true)
      targets:                        # Required: At least one target
        - type: lambda|ecs_task|sqs|sns
          name: string                # Reference to extension by name
          input: string               # Optional: Static input JSON
          input_path: string          # Optional: JSONPath to extract
          input_transformer:          # Optional: Transform input
            input_paths: map
            input_template: string
```

---

## Real-World Examples

### Example 1: SES Email Tracking (Complete Setup)

Track email delivery, bounces, and complaints:

```yaml
# dev.yaml
ses:
  enabled: true
  domain_name: mail.dev.example.com

extensions:
  sns_topics:
    # Delivery notifications
    - name: ses_delivery
      add_to_backend_env: AWS_SNS_SES_DELIVERY_ARN
      webhooks:
        - path: /api/webhooks/email/delivery
          raw_message_delivery: true

    # Bounce notifications
    - name: ses_bounce
      add_to_backend_env: AWS_SNS_SES_BOUNCE_ARN
      webhooks:
        - path: /api/webhooks/email/bounce
          raw_message_delivery: true

    # Complaint notifications
    - name: ses_complaint
      add_to_backend_env: AWS_SNS_SES_COMPLAINT_ARN
      webhooks:
        - path: /api/webhooks/email/complaint
          raw_message_delivery: true
```

Backend handler:

```go
func HandleEmailWebhook(w http.ResponseWriter, r *http.Request) {
    var notification SESNotification
    json.NewDecoder(r.Body).Decode(&notification)

    switch notification.NotificationType {
    case "Delivery":
        markEmailDelivered(notification.Mail.MessageID)
    case "Bounce":
        handleBounce(notification.Bounce)
    case "Complaint":
        handleComplaint(notification.Complaint)
    }

    w.WriteHeader(http.StatusOK)
}
```

### Example 2: Async Job Processing with SQS

Process jobs asynchronously with retry logic:

```yaml
extensions:
  sqs_queues:
    - name: jobs_high_priority
      add_to_backend_env: AWS_SQS_HIGH_PRIORITY_URL
      visibility_timeout: 300
      dlq_enabled: true
      dlq_max_receive: 3

    - name: jobs_low_priority
      add_to_backend_env: AWS_SQS_LOW_PRIORITY_URL
      visibility_timeout: 900  # 15 min for slow jobs
      dlq_enabled: true
      dlq_max_receive: 5

workload:
  backend_policies:
    - actions:
        - sqs:*
      resources:
        - "arn:aws:sqs:*:*:myproject-*"
```

### Example 3: Real-Time Notifications with SNS + WebSocket

Publish events to SNS, deliver to connected clients:

```yaml
extensions:
  sns_topics:
    - name: realtime_events
      add_to_backend_env: AWS_SNS_REALTIME_ARN
      webhooks:
        - path: /api/internal/realtime
          raw_message_delivery: true
          filter_policy:
            event_type:
              - order_created
              - order_updated
              - payment_received

pubsub_appsync:
  enabled: true
```

### Example 4: Image Processing Pipeline

S3 upload triggers Lambda, results stored back:

```yaml
buckets:
  - name: media
    public: false

extensions:
  sns_topics:
    - name: image_processed
      add_to_backend_env: AWS_SNS_IMAGE_PROCESSED_ARN
      webhooks:
        - path: /api/webhooks/image/processed

  lambda_functions:
    - name: image_processor
      runtime: python3.12
      handler: handler.process
      source_dir: custom/lambda/image-processor
      memory: 1024
      timeout: 60
      environment:
        OUTPUT_BUCKET: "${module.s3.buckets.media.id}"
        NOTIFICATION_TOPIC: "${aws_sns_topic.ext_image_processed.arn}"
      triggers:
        - type: s3
          bucket: media
          events:
            - s3:ObjectCreated:*
          prefix: uploads/
          suffix: .jpg
```

### Example 5: Scheduled Data Sync

Periodic data synchronization:

```yaml
extensions:
  eventbridge_rules:
    - name: daily_sync
      schedule_expression: "cron(0 2 * * ? *)"  # 2 AM UTC daily
      targets:
        - type: ecs_task
          name: data_sync
          input: |
            {"job": "full_sync", "source": "external_api"}

scheduled_tasks:
  - name: data_sync
    schedule: "rate(0 minutes)"  # Triggered by EventBridge, not rate
    docker_image: ""  # Uses backend image
    container_command: '["./app", "sync"]'
```

### Example 6: Multi-Environment Event Bus

Share events across environments:

```yaml
# prod.yaml
extensions:
  sns_topics:
    - name: domain_events
      add_to_backend_env: AWS_SNS_DOMAIN_EVENTS_ARN
      # Cross-account subscription (dev listens to prod events)

  eventbridge_rules:
    - name: forward_to_analytics
      event_pattern:
        source:
          - myapp.orders
        detail-type:
          - OrderCreated
          - OrderCompleted
      targets:
        - type: sns
          name: domain_events
```

### Example 7: API Rate Limiting with SQS

Queue requests during high load:

```yaml
extensions:
  sqs_queues:
    - name: api_overflow
      add_to_backend_env: AWS_SQS_OVERFLOW_URL
      visibility_timeout: 30
      dlq_enabled: true

  sns_topics:
    - name: high_load_alert
      webhooks:
        - path: /api/internal/alerts/high-load
```

### Example 8: Audit Log Pipeline

Capture and process audit events:

```yaml
extensions:
  sqs_queues:
    - name: audit_events
      add_to_backend_env: AWS_SQS_AUDIT_URL
      message_retention: 1209600  # 14 days
      fifo: true
      content_based_dedup: true

  lambda_functions:
    - name: audit_processor
      runtime: nodejs20.x
      handler: index.handler
      source_dir: custom/lambda/audit
      triggers:
        - type: sqs
          queue: audit_events
          batch_size: 10
```

---

## Bridge Variables Reference

When using raw terraform files or custom post-module, these variables are available:

| Variable | Description | Example |
|----------|-------------|---------|
| `var.project` | Project name | `"myproject"` |
| `var.env` | Environment name | `"dev"` |
| `var.region` | AWS region | `"us-east-1"` |
| `var.account_id` | AWS account ID | `"123456789012"` |
| `var.vpc_id` | VPC ID | `"vpc-abc123"` |
| `var.subnet_ids` | Subnet IDs | `["subnet-a", "subnet-b"]` |
| `var.api_endpoint` | API Gateway endpoint | `"https://abc.execute-api..."` |
| `var.api_gateway_id` | API Gateway ID | `"abc123xyz"` |
| `var.ecs_cluster_arn` | ECS cluster ARN | `"arn:aws:ecs:..."` |
| `var.ecs_cluster_name` | ECS cluster name | `"myproject-dev"` |
| `var.backend_ecr_repo_url` | Backend ECR URL | `"123.dkr.ecr..."` |
| `var.backend_task_role` | Backend task role name | `"myproject-dev-backend"` |
| `var.domain_zone_id` | Route53 zone ID (if domain enabled) | `"Z1234567890"` |
| `var.api_domain` | API custom domain (if enabled) | `"api.dev.example.com"` |
| `var.db_endpoint` | Database endpoint (if postgres enabled) | `"mydb.cluster-xxx.rds..."` |

---

## Best Practices

### 1. Use YAML Extensions for Common Patterns

Prefer YAML extensions over custom modules when possible:

```yaml
# Good: YAML extension
extensions:
  sns_topics:
    - name: events
      add_to_backend_env: EVENTS_TOPIC_ARN

# Avoid: Custom module for simple SNS
# custom/pre/main.tf with SNS resource
```

### 2. Keep Custom Modules Focused

Each custom module should do one thing:

```
custom/
├── post/
│   └── monitoring/      # Just monitoring resources
└── pre/
    └── encryption/      # Just KMS/encryption setup
```

### 3. Use Consistent Naming

Follow the `{project}-{env}-{resource}` pattern:

```hcl
resource "aws_sns_topic" "events" {
  name = "${var.project}-${var.env}-events"
}
```

### 4. Document Your Extensions

Add comments explaining why custom extensions exist:

```yaml
extensions:
  # Required for email deliverability tracking
  # See: https://docs.aws.amazon.com/ses/latest/dg/monitor-sending-activity.html
  sns_topics:
    - name: ses_events
```

### 5. Test in Dev First

Always deploy extensions to dev before prod:

```bash
# Test in dev
meroku deploy  # with dev.yaml selected

# Verify working
aws sns list-topics

# Then deploy to prod
```

### 6. Version Control Custom Code

Keep `custom/` directory in git:

```gitignore
# .gitignore
env/*/
!custom/
```

---

## Troubleshooting

### Extension Not Applied

1. Check YAML syntax:
   ```bash
   cat dev.yaml | python -c "import yaml,sys; yaml.safe_load(sys.stdin)"
   ```

2. Regenerate and check main.tf:
   ```bash
   meroku generate dev
   cat env/dev/main.tf | grep -A5 "ext_"
   ```

### Custom Module Not Found

Ensure correct directory structure:
```bash
ls -la custom/post/main.tf
ls -la custom/pre/main.tf
```

### Variable Not Available

Check the bridge variables reference. Some variables are only available when certain modules are enabled (e.g., `db_endpoint` requires `postgres.enabled: true`).

### Circular Dependencies

If you see terraform errors about cycles, ensure:
- Pre-modules don't reference core module outputs
- Post-modules don't try to feed values back to core

---

## Migration from Manual Terraform

If you have existing custom Terraform:

1. Move files to `custom/terraform/_shared/`:
   ```bash
   mkdir -p custom/terraform/_shared
   mv my-custom.tf custom/terraform/_shared/
   ```

2. Update resource references to use module outputs:
   ```hcl
   # Before
   resource "aws_sns_topic_subscription" "x" {
     endpoint = "https://api.example.com/webhook"
   }

   # After
   resource "aws_sns_topic_subscription" "x" {
     endpoint = "${module.workloads.api_gateway_endpoint}/webhook"
   }
   ```

3. Consider migrating to YAML extensions for cleaner config.
