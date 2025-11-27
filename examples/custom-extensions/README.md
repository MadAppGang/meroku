# Custom Extensions Examples

This directory contains example templates for extending meroku infrastructure.

## Quick Start

Copy the directory structure you need to your project:

```bash
# For custom pre-module (feed values INTO backend)
cp -r examples/custom-extensions/pre your-project/custom/pre

# For custom post-module (use core module outputs)
cp -r examples/custom-extensions/post your-project/custom/post

# For raw terraform files
cp -r examples/custom-extensions/terraform your-project/custom/terraform
```

## Directory Structure

```
custom-extensions/
├── pre/                    # Custom pre-module example
│   ├── main.tf             # Resource definitions
│   ├── variables.tf        # Input variables (auto-passed by meroku)
│   └── outputs.tf          # Outputs (feed into workloads module)
├── post/                   # Custom post-module example
│   ├── main.tf             # Resource definitions
│   ├── variables.tf        # Input variables (includes core outputs)
│   └── outputs.tf          # Outputs for reference
└── terraform/              # Raw terraform files
    └── _shared/            # Applied to all environments
        └── sns-webhook-example.tf
```

## Extension Types

### 1. YAML Extensions (Recommended for common patterns)

Add to your environment YAML:

```yaml
extensions:
  sns_topics:
    - name: order_events
      add_to_backend_env: AWS_SNS_ORDER_TOPIC_ARN
      webhooks:
        - path: /webhooks/sns/orders

  sqs_queues:
    - name: job_processor
      add_to_backend_env: AWS_SQS_JOBS_URL
      dlq_enabled: true
```

### 2. Custom Pre-Module (Feed values to backend)

Use when you need custom resources that provide values to the backend:

- KMS keys
- Secrets Manager secrets
- SNS topics (for publishing)
- Custom IAM roles

Copy `pre/` to `your-project/custom/pre/` and modify.

### 3. Custom Post-Module (Use core outputs)

Use when you need resources that consume core module outputs:

- CloudWatch dashboards
- Monitoring alarms
- SNS subscriptions to API endpoint
- Additional security groups

Copy `post/` to `your-project/custom/post/` and modify.

### 4. Raw Terraform Files (Simple additions)

Use for simple resources that don't need bidirectional integration:

Copy files to `your-project/custom/terraform/_shared/` for all environments
or `your-project/custom/terraform/{env}/` for specific environments.

## Available Variables

### Pre-Module Variables

```hcl
var.project    # Project name
var.env        # Environment (dev, staging, prod)
var.region     # AWS region
var.account_id # AWS account ID
var.vpc_id     # VPC ID
var.subnet_ids # List of subnet IDs
```

### Post-Module Variables

All pre-module variables plus:

```hcl
var.api_endpoint         # API Gateway endpoint with stage
var.api_gateway_id       # API Gateway ID
var.ecs_cluster_arn      # ECS cluster ARN
var.ecs_cluster_name     # ECS cluster name
var.backend_ecr_repo_url # Backend ECR repository URL
var.backend_task_role    # Backend task IAM role name
var.domain_zone_id       # Route53 zone ID (if domain enabled)
var.api_domain           # Custom API domain (if domain enabled)
var.db_endpoint          # Database endpoint (if postgres enabled)
```

### Bridge File (for raw terraform)

```hcl
local.bridge.project
local.bridge.env
local.bridge.api_endpoint
# ... see terraform/_shared/sns-webhook-example.tf for full list
```

## Examples

### SNS with Backend Integration

```yaml
# dev.yaml
extensions:
  sns_topics:
    - name: ses_bounce
      add_to_backend_env: AWS_SNS_BOUNCE_ARN
      webhooks:
        - path: /api/webhooks/email/bounce
          raw_message_delivery: true
```

### SQS with Dead Letter Queue

```yaml
extensions:
  sqs_queues:
    - name: async_jobs
      add_to_backend_env: AWS_SQS_JOBS_URL
      visibility_timeout: 300
      dlq_enabled: true
      dlq_max_receive: 3
```

### Custom CloudWatch Dashboard

```bash
cp -r examples/custom-extensions/post your-project/custom/
# Edit custom/post/main.tf to customize the dashboard
```

## Deployment

After setting up extensions:

```bash
meroku deploy
```

Meroku will automatically:
1. Generate terraform including your extensions
2. Create bridge file for reference
3. Merge extension env vars with backend env vars
4. Deploy everything together
