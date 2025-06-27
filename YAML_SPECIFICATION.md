# YAML Configuration Specification

This document provides a complete specification for the YAML configuration files used in the infrastructure repository.

## Overview

The YAML configuration files (e.g., `dev.yaml`, `prod.yaml`) define environment-specific settings that are processed by Gomplate to generate Terraform configurations. These files control all aspects of the AWS infrastructure deployment.

## Complete Configuration Schema

```yaml
# ===================================
# CORE SETTINGS (Required)
# ===================================

project: <string>              # Required: Project name (alphanumeric + dash, e.g., "my-app")
env: <string>                  # Required: Environment name (e.g., "dev", "staging", "prod")
is_prod: <boolean>             # Required: Whether this is a production environment
region: <string>               # Required: AWS region (e.g., "us-east-1", "eu-central-1")
state_bucket: <string>         # Required: S3 bucket for Terraform state storage
state_file: <string>           # Required: Terraform state file name (default: "state.tfstate")

# Optional: ECR configuration for cross-account/region image pulls
ecr_account_id: <string>       # AWS account ID where ECR repositories are located
ecr_account_region: <string>   # AWS region where ECR repositories are located

# ===================================
# WORKLOAD CONFIGURATION
# ===================================

workload:
  # Basic backend configuration
  backend_health_endpoint: <string>          # Health check endpoint (default: "/health/live")
  backend_external_docker_image: <string>    # External Docker image URL (leave empty to use ECR)
  backend_container_command: <list>          # Container command override
    - <string>
  backend_image_port: <number>               # Container port (default: 8080)
  backend_remote_access: <boolean>           # Enable ECS Exec for debugging (default: true)

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
  add_domain_prefix: <boolean>               # Add environment prefix to domain (e.g., "dev.example.com")

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
    container_command: <string>              # Command in JSON array format: '["cmd", "arg1"]'
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
    container_command: <list>                # Command override
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

9. **GitHub OIDC**: Subjects must match your GitHub repository structure exactly.

10. **Schedule Expressions**: Use UTC timezone for cron expressions.
