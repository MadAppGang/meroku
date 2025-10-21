# YAML Configuration Specification

This document provides a complete specification for the YAML configuration files used in the infrastructure repository.

## Overview

The YAML configuration files (e.g., `dev.yaml`, `prod.yaml`) define environment-specific settings that are processed by Handlebars (using the Raymond Go package) to generate Terraform configurations. These files control all aspects of the AWS infrastructure deployment.

The configuration is processed through the `env/main.hbs` Handlebars template to generate Terraform configurations in the `env/` directory.

## Complete Configuration Schema

```yaml
# ===================================
# CORE SETTINGS (Required)
# ===================================

project: <string>              # Required: Project name (alphanumeric + dash, e.g., "my-app")
env: <string>                  # Required: Environment name (e.g., "dev", "staging", "prod")
is_prod: <boolean>             # Required: Whether this is a production environment (affects domain behavior)
region: <string>               # Required: AWS region (e.g., "us-east-1", "eu-central-1")
state_bucket: <string>         # Required: S3 bucket for Terraform state storage
state_file: <string>           # Required: Terraform state file name (default: "state.tfstate")

# ===================================
# ECR CONFIGURATION
# ===================================
# ECR repository strategy (Schema v7)
ecr_strategy: <string>         # Required: "local" (create ECR in this account) or "cross_account" (pull from another account)
                               # Default: "local"

# Cross-account ECR configuration (only needed when ecr_strategy is "cross_account")
ecr_account_id: <string>       # AWS account ID where ECR repositories are located
ecr_account_region: <string>   # AWS region where ECR repositories are located

# ===================================
# WORKLOAD CONFIGURATION
# ===================================

workload:
  # Basic backend configuration
  backend_health_endpoint: <string>          # Health check endpoint (default: "/health/live")
  backend_external_docker_image: <string>    # External Docker image URL (leave empty to use ECR)
  backend_container_command: <list>          # Container command override (uses backend Dockerfile CMD if not set)
    - <string>
  backend_image_port: <number>               # Container port (default: 8080)
  backend_remote_access: <boolean>           # Enable ECS Exec for debugging (default: true)
  
  # Backend scaling configuration
  backend_cpu: <number>                      # CPU units for backend (256, 512, 1024, 2048, 4096) (default: 256)
  backend_memory: <number>                   # Memory in MB for backend (default: 512)
  backend_desired_count: <number>            # Number of backend instances (default: 1)
  
  # Backend autoscaling configuration
  backend_autoscaling_enabled: <boolean>     # Enable autoscaling for backend service (default: false)
  backend_autoscaling_min_capacity: <number> # Minimum number of tasks (default: 1)
  backend_autoscaling_max_capacity: <number> # Maximum number of tasks (default: 10)
  backend_autoscaling_target_cpu: <number>   # Target CPU utilization % (default: 70)
  backend_autoscaling_target_memory: <number> # Target memory utilization % (default: 80)

  # S3 bucket configuration
  bucket_postfix: <string>                   # S3 bucket name suffix (alphanumeric + dash, max 30 chars)
  bucket_public: <boolean>                   # Make backend S3 bucket publicly accessible (default: true)

  # Environment configuration
  backend_env_variables: <list>              # Environment variables for backend
    - name: <string>
      value: <string>
  env_files_s3: <list>                       # S3-stored environment files
    - bucket: <string>                       # S3 bucket name
      key: <string>                          # S3 object key (e.g., "config/app.env")

  # Monitoring and observability
  xray_enabled: <boolean>                    # Enable AWS X-Ray distributed tracing

  # Notifications
  setup_fcnsns: <boolean>                    # Setup Firebase Cloud Messaging/SNS for push notifications
  slack_webhook: <string>                    # Slack webhook URL for deployment notifications

  # CI/CD configuration
  enable_github_oidc: <boolean>              # Enable GitHub OIDC for passwordless CI/CD
  github_oidc_subjects: <list>               # GitHub repos/branches allowed to assume roles
    - <string>                               # Format: "repo:Owner/Repo:ref:refs/heads/branch" or "repo:Owner/*"

  # Database admin tools
  install_pg_admin: <boolean>                # Install pgAdmin web interface
  pg_admin_email: <string>                   # pgAdmin admin email address

  # IAM policies
  policy: <list>                             # Additional IAM policies for backend task
    - actions: <list>                        # IAM actions (e.g., ["s3:GetObject", "s3:PutObject"])
        - <string>
      resources: <list>                      # IAM resources (e.g., ["arn:aws:s3:::my-bucket/*"])
        - <string>

  # EFS mounts
  efs: <list>                                # EFS volumes to mount in backend
    - name: <string>                         # EFS config name (must match EFS definition)
      mount_point: <string>                  # Container mount path (e.g., "/data")

  # ALB configuration (requires alb.enabled)
  backend_alb_domain_name: <string>          # Custom domain for ALB (e.g., "api.example.com")

# ===================================
# DOMAIN CONFIGURATION
# ===================================

domain:
  enabled: <boolean>                         # Enable Route53 domain management
  create_domain_zone: <boolean>              # Create new hosted zone (false = use existing)
  domain_name: <string>                      # Base domain name (e.g., "example.com")
  api_domain_prefix: <string>                # API subdomain prefix (e.g., "api" → "api.example.com")
  add_domain_prefix: <boolean>               # Add environment prefix to domain (auto-false for prod env)

# ===================================
# DATABASE CONFIGURATION
# ===================================

postgres:
  enabled: <boolean>                         # Enable RDS PostgreSQL database
  dbname: <string>                           # Database name
  username: <string>                         # Master username (default: "postgres")
  public_access: <boolean>                   # Allow public internet access
  engine_version: <string>                   # PostgreSQL version (e.g., "14", "15", "16.x")

# ===================================
# AUTHENTICATION CONFIGURATION
# ===================================

cognito:
  enabled: <boolean>                         # Enable AWS Cognito user pool
  enable_web_client: <boolean>               # Create web application client
  enable_dashboard_client: <boolean>         # Create dashboard/admin client
  dashboard_callback_urls: <list>            # OAuth callback URLs for dashboard
    - <string>                               # e.g., "https://dashboard.example.com/callback"
  enable_user_pool_domain: <boolean>         # Enable Cognito hosted UI
  user_pool_domain_prefix: <string>          # Cognito domain prefix (globally unique)
  backend_confirm_signup: <boolean>          # Allow backend to confirm user signups
  auto_verified_attributes: <list>           # Attributes auto-verified on signup
    - <string>                               # Options: "email", "phone_number"

# ===================================
# EMAIL SERVICE CONFIGURATION
# ===================================

ses:
  enabled: <boolean>                         # Enable AWS SES for sending emails
  domain_name: <string>                      # Custom domain for sending (uses main domain if empty)
  test_emails: <list>                        # Email addresses for testing
    - <string>

# ===================================
# MESSAGE QUEUE CONFIGURATION
# ===================================

sqs:
  enabled: <boolean>                         # Enable SQS message queue
  name: <string>                             # Queue name

# ===================================
# FILE STORAGE CONFIGURATION
# ===================================

# Elastic File System (EFS) volumes
efs: <list>
  - name: <string>                           # EFS configuration name
    path: <string>                           # Root directory path in EFS

# Additional S3 buckets
buckets: <list>
  - name: <string>                           # Bucket name suffix
    public: <boolean>                        # Make bucket publicly accessible

# ===================================
# LOAD BALANCER CONFIGURATION
# ===================================

alb:
  enabled: <boolean>                         # Enable Application Load Balancer

# ===================================
# SCHEDULED TASKS (CRON JOBS)
# ===================================

scheduled_tasks: <list>
  - name: <string>                           # Task name (alphanumeric + dash)
    schedule: <string>                       # Schedule expression (see examples below)
    docker_image: <string>                   # Docker image (optional, uses backend if empty)
    container_command: <string>              # Command in JSON array format: '["cmd", "arg1"]' (uses image default if not set)
    allow_public_access: <boolean>           # Assign public IP to task

# Schedule expression examples:
# - "rate(5 minutes)"      - Every 5 minutes
# - "rate(1 hour)"         - Every hour
# - "rate(7 days)"         - Every 7 days
# - "cron(0 12 * * ? *)"   - Daily at 12:00 UTC
# - "cron(0 0 ? * SUN *)"  - Every Sunday at midnight

# ===================================
# EVENT-DRIVEN TASKS
# ===================================

event_processor_tasks: <list>
  - name: <string>                           # Task name
    rule_name: <string>                      # EventBridge rule name
    detail_types: <list>                     # Event detail types to match
      - <string>                             # e.g., "Order Created", "User Registered"
    sources: <list>                          # Event sources to match
      - <string>                             # e.g., "com.myapp.orders"
    docker_image: <string>                   # Docker image (optional)
    container_command: <string>              # Command in JSON array format
    allow_public_access: <boolean>           # Assign public IP to task

# ===================================
# GRAPHQL API CONFIGURATION
# ===================================

pubsub_appsync:
  enabled: <boolean>                         # Enable AWS AppSync GraphQL API
  schema: <boolean>                          # Use custom schema file
  auth_lambda: <boolean>                     # Use Lambda authorizer
  resolvers: <boolean>                       # Use custom VTL resolvers

# ===================================
# ADDITIONAL SERVICES
# ===================================

services: <list>
  - name: <string>                           # Service name
    # Container configuration
    docker_image: <string>                   # Docker image override
    container_command: <list>                # Command override (uses image CMD if not set)
      - <string>
    container_port: <number>                 # Container port (default: 3000)
    host_port: <number>                      # Host port (default: 3000)

    # Resource allocation
    cpu: <number>                            # CPU units (256, 512, 1024, etc.)
    memory: <number>                         # Memory in MB (512, 1024, 2048, etc.)
    desired_count: <number>                  # Number of running tasks (default: 1)

    # Features
    remote_access: <boolean>                 # Enable ECS Exec for debugging
    xray_enabled: <boolean>                  # Enable X-Ray tracing
    essential: <boolean>                     # Mark as essential container

    # Environment configuration
    env_vars: <list>                         # Environment variables
      - name: <string>
        value: <string>
    env_files_s3: <list>                     # S3-stored environment files
      - bucket: <string>
        key: <string>
```

## Examples

### Minimal Development Configuration

```yaml
project: myapp
env: dev
is_prod: false
region: us-east-1
state_bucket: myapp-terraform-state-dev
state_file: state.tfstate

workload:
  bucket_postfix: dev123
  backend_image_port: 3000
```

### Production Configuration with All Features

```yaml
project: myapp
env: prod
is_prod: true
region: us-east-1
state_bucket: myapp-terraform-state-prod
state_file: state.tfstate

workload:
  backend_health_endpoint: "/health"
  backend_image_port: 8080
  bucket_postfix: prod456
  bucket_public: false
  xray_enabled: true
  
  # Backend scaling for production
  backend_cpu: 1024
  backend_memory: 2048
  backend_desired_count: 2
  backend_autoscaling_enabled: true
  backend_autoscaling_min_capacity: 2
  backend_autoscaling_max_capacity: 20
  backend_autoscaling_target_cpu: 70
  backend_autoscaling_target_memory: 80
  
  backend_env_variables:
    - name: NODE_ENV
      value: production
    - name: LOG_LEVEL
      value: info
  slack_webhook: "https://hooks.slack.com/services/XXX/YYY/ZZZ"
  enable_github_oidc: true
  github_oidc_subjects:
    - "repo:MyOrg/myapp:ref:refs/heads/main"
  policy:
    - actions:
        - "s3:GetObject"
        - "s3:PutObject"
      resources:
        - "arn:aws:s3:::myapp-uploads-prod/*"

domain:
  enabled: true
  create_domain_zone: false
  domain_name: myapp.com
  api_domain_prefix: api

postgres:
  enabled: true
  dbname: myapp
  username: dbadmin
  public_access: false
  engine_version: "15"

cognito:
  enabled: true
  enable_web_client: true
  enable_dashboard_client: true
  dashboard_callback_urls:
    - "https://admin.myapp.com/callback"
  enable_user_pool_domain: true
  user_pool_domain_prefix: myapp-auth
  auto_verified_attributes:
    - email

ses:
  enabled: true
  test_emails:
    - admin@myapp.com

sqs:
  enabled: true
  name: main-queue

scheduled_tasks:
  - name: daily-cleanup
    schedule: "rate(1 day)"
    container_command: '["node", "scripts/cleanup.js"]'
    allow_public_access: false

services:
  - name: worker
    container_command: ["node", "worker.js"]
    cpu: 512
    memory: 1024
    desired_count: 2
    env_vars:
      - name: WORKER_TYPE
        value: background
```

## Important Notes

1. **Project Naming**: Use only lowercase letters, numbers, and hyphens. Must start with a letter.

2. **Environment Naming**: Common values are "dev", "staging", "prod". This affects resource naming.

3. **State Management**: Each environment must have a unique state bucket or state file to prevent conflicts.

4. **Container Commands**: Must be valid JSON arrays when specified as strings.

5. **IAM Policies**: Follow the principle of least privilege. Only grant necessary permissions.

6. **Secrets**: Never put secrets in YAML files. Use AWS Secrets Manager or Parameter Store.

7. **Domain Setup**: Requires valid DNS configuration. Certificate validation may take up to 30 minutes.

8. **Resource Limits**:

   - CPU: Must be 256, 512, 1024, 2048, or 4096
   - Memory: Must be compatible with CPU (see AWS Fargate limits)
   - Valid CPU/Memory combinations for Fargate:
     - 256 CPU: 512, 1024, 2048 MB
     - 512 CPU: 1024-4096 MB (in 1024 MB increments)
     - 1024 CPU: 2048-8192 MB (in 1024 MB increments)
     - 2048 CPU: 4096-16384 MB (in 1024 MB increments)
     - 4096 CPU: 8192-30720 MB (in 1024 MB increments)
   - When X-Ray is enabled, ensure total task resources accommodate both containers

9. **GitHub OIDC**: Subjects must match your GitHub repository structure exactly.

10. **Schedule Expressions**: Use UTC timezone for cron expressions.

11. **Backend Autoscaling**: 
    - When enabled, the backend service will scale based on CPU and memory utilization
    - Scaling policies use target tracking with configurable thresholds
    - Scale-out is faster (60s cooldown) than scale-in (300s cooldown) to handle traffic spikes
    - If ALB is enabled, an additional request-based scaling policy is created
    - Ensure min/max capacity aligns with your application's requirements and budget

## Template Processing

### Helper Functions

The Handlebars template (processed by Raymond) uses several helper functions to process YAML configuration:

1. **`{{default value fallback}}`** - Provides default values when not specified
2. **`{{array list}}`** - Converts YAML lists to Terraform array format
3. **`{{envArray list}}`** - Converts environment variable lists to Terraform format
4. **`{{compare value operator value}}`** - Conditional logic (e.g., `{{compare env "==" "prod"}}`)
5. **`{{#if condition}}`** - Conditional blocks
6. **`{{#each list}}`** - Iteration over lists
7. **`{{@root.property}}`** - Access root context in loops
8. **`{{len list}}`** - Get length of lists to check if empty

### Module Dependencies

Based on the template, the following Terraform modules are conditionally loaded:

1. **domain** - Loaded when `domain.enabled` is true
2. **postgres** - Loaded when `postgres.enabled` is true
3. **sqs** - Loaded when `sqs.enabled` is true
4. **efs** - Loaded when `efs` list has items
5. **s3** - Loaded when `buckets` list has items
6. **alb** - Loaded when `alb.enabled` is true
7. **cognito** - Loaded when `cognito.enabled` is true
8. **ses** - Loaded when `ses.enabled` is true
9. **appsync** - Loaded when `pubsub_appsync.enabled` is true
10. **workloads** - Always loaded (core module)

### Special Template Behaviors

#### ECR Repository Strategy (Schema v7)

**Local Strategy** (`ecr_strategy: "local"`):
- Creates ECR repositories in the current AWS account
- Repositories are created for backend, services, and tasks
- Repository naming: `{project}_backend`, `{project}_service_{name}`, `{project}_task_{name}`
- Use this when you want each environment to have its own container registry

**Cross-Account Strategy** (`ecr_strategy: "cross_account"`):
- Pulls container images from another AWS account's ECR
- Requires `ecr_account_id` and `ecr_account_region` to be set
- No ECR repositories are created in this environment
- Use this when multiple environments share a single ECR (e.g., dev ECR used by staging and prod)

When `ecr_strategy` is "cross_account" and both `ecr_account_id` and `ecr_account_region` are set, ECR URLs are automatically generated:

- Backend: `{ecr_account_id}.dkr.ecr.{ecr_account_region}.amazonaws.com/{project}_backend`
- Scheduled tasks: `{ecr_account_id}.dkr.ecr.{ecr_account_region}.amazonaws.com/{project}_task_{name}`
- Event tasks: `{ecr_account_id}.dkr.ecr.{ecr_account_region}.amazonaws.com/{project}_task_{name}`

**Migration from Schema v6:**
- If `env == "dev"`: Sets `ecr_strategy: "local"`
- If `ecr_account_id` is set: Sets `ecr_strategy: "cross_account"`
- Otherwise: Sets `ecr_strategy: "local"`

#### Domain Prefix Handling

- Production environments (`env == "prod"`) automatically set `add_env_domain_prefix` to false
- Other environments default to true unless explicitly set
- This prevents production from having environment prefixes in domain names

#### Email Domain Default

- If `ses.domain_name` is not specified, defaults to `mail.{domain.domain_name}`
- Requires `domain.enabled` to be true for zone ID access

#### AppSync Custom Modules

When enabled, AppSync looks for custom files relative to the `custom_modules` path:

- Schema: `{custom_modules}/appsync/schema.graphql`
- Resolvers: `{custom_modules}/appsync/vtl_templates.yaml`
- Auth Lambda: `{custom_modules}/appsync/auth_lambda`

### Terraform Outputs

The generated Terraform configuration provides these outputs:

- `backend_ecr_repo_url` - ECR repository URL for the backend service
- `account_id` - AWS account ID
- `region` - AWS region
- `backend_task_role_name` - IAM role name for backend ECS tasks
- `backend_cloud_map_arn` - Service discovery ARN for the backend service
