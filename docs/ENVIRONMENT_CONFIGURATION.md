# Environment Configuration Guide

This comprehensive guide explains all available configuration fields in environment YAML files (`dev.yaml`, `prod.yaml`, `staging.yaml`, etc.). These files define your infrastructure as code and are used by the meroku CLI to generate Terraform configurations.

## Table of Contents

- [Core Settings](#core-settings)
- [Workload Configuration](#workload-configuration)
- [Domain Configuration](#domain-configuration)
- [Database Configuration (PostgreSQL)](#database-configuration-postgresql)
- [Authentication (Cognito)](#authentication-cognito)
- [Email Service (SES)](#email-service-ses)
- [Message Queue (SQS)](#message-queue-sqs)
- [Load Balancer (ALB)](#load-balancer-alb)
- [Services](#services)
- [Scheduled Tasks](#scheduled-tasks)
- [Event Processor Tasks](#event-processor-tasks)
- [File Storage (EFS)](#file-storage-efs)
- [S3 Buckets](#s3-buckets)
- [Frontend Deployment (Amplify)](#frontend-deployment-amplify)
- [GraphQL API (AppSync)](#graphql-api-appsync)
- [Complete Example](#complete-example)

---

## Core Settings

These fields define the basic infrastructure parameters for your environment.

### `project`
- **Type**: String (required)
- **Description**: Your project name. Used as a prefix for all AWS resources.
- **Example**: `"instagram"`, `"myapp"`, `"acme-corp"`
- **Notes**: Use lowercase, no spaces. This appears in resource names like `myapp-backend-dev`.

### `env`
- **Type**: String (required)
- **Description**: Environment name (dev, staging, prod, etc.)
- **Example**: `"dev"`, `"staging"`, `"prod"`
- **Notes**: Determines which configuration file is loaded and resource naming.

### `is_prod`
- **Type**: Boolean
- **Description**: Flag indicating if this is a production environment
- **Default**: `false`
- **Example**: `true`
- **Notes**: Affects backup retention, deletion protection, and other production safeguards.

### `region`
- **Type**: String (required)
- **Description**: AWS region where resources will be deployed
- **Example**: `"us-east-1"`, `"eu-central-1"`, `"ap-southeast-2"`
- **Notes**: Choose a region close to your users for better latency.

### `account_id`
- **Type**: String
- **Description**: AWS account ID (12-digit number)
- **Example**: `"123456789012"`
- **Notes**: Automatically populated when using meroku. Used for cross-account operations.

### `aws_profile`
- **Type**: String
- **Description**: AWS CLI profile name to use for this environment
- **Example**: `"myproject-dev"`, `"production-account"`
- **Notes**: Must match a profile in your `~/.aws/credentials` file.

### `state_bucket`
- **Type**: String (required)
- **Description**: S3 bucket name for storing Terraform state
- **Example**: `"myapp-terraform-state-dev"`
- **Notes**: Must be globally unique. Create this bucket before running `terraform init`.

### `state_file`
- **Type**: String
- **Description**: Name of the Terraform state file within the bucket
- **Default**: `"state.tfstate"`
- **Example**: `"terraform.tfstate"`

---

## Workload Configuration

The `workload` section configures your main backend application and related infrastructure.

### Backend Application

#### `backend_health_endpoint`
- **Type**: String
- **Description**: Custom health check path for the backend service
- **Default**: `"/health/live"`
- **Example**: `"/api/health"`, `"/healthz"`
- **Notes**: Your application must return HTTP 200 on this endpoint to pass health checks.

#### `backend_external_docker_image`
- **Type**: String
- **Description**: Full Docker image URI if using an image from outside your ECR
- **Example**: `"nginx:latest"`, `"public.ecr.aws/docker/library/redis:7"`
- **Notes**: Leave empty to use the project's ECR repository.

#### `backend_container_command`
- **Type**: Array of strings or string
- **Description**: Override the container's default command
- **Example**: `["node", "server.js"]` or `'["migrate", "&&", "start"]'`
- **Notes**: Useful for running migrations or custom startup scripts.

#### `backend_image_port`
- **Type**: Integer
- **Description**: Port your backend application listens on inside the container
- **Default**: `8080`
- **Example**: `3000`, `8080`, `9000`
- **Notes**: Must match the port your application binds to.

#### `backend_alb_domain_name`
- **Type**: String
- **Description**: Custom domain name for the Application Load Balancer
- **Example**: `"api.example.com"`, `"alb.myapp.com"`
- **Notes**: Requires DNS configuration. A Route53 A record will be created.

### Scaling Configuration

#### `backend_desired_count`
- **Type**: Integer
- **Description**: Initial number of backend tasks to run
- **Default**: `1`
- **Example**: `2`, `3`, `5`
- **Notes**: Higher values provide better availability but increase costs.

#### `backend_cpu`
- **Type**: String
- **Description**: CPU units for the backend task (1 vCPU = 1024 units)
- **Default**: `"256"`
- **Example**: `"256"`, `"512"`, `"1024"`, `"2048"`, `"4096"`
- **Notes**: Must be compatible with `backend_memory` (see AWS Fargate task sizing).

#### `backend_memory`
- **Type**: String
- **Description**: Memory for the backend task in MB
- **Default**: `"512"`
- **Example**: `"512"`, `"1024"`, `"2048"`, `"4096"`
- **Notes**: Must be compatible with `backend_cpu` (see AWS Fargate task sizing).

#### `backend_autoscaling_enabled`
- **Type**: Boolean
- **Description**: Enable automatic scaling based on resource utilization
- **Default**: `false`
- **Example**: `true`
- **Notes**: Recommended for production to handle traffic spikes.

#### `backend_autoscaling_min_capacity`
- **Type**: Integer
- **Description**: Minimum number of tasks when autoscaling is enabled
- **Default**: `1`
- **Example**: `2`, `3`
- **Notes**: Ensures minimum availability even during low traffic.

#### `backend_autoscaling_max_capacity`
- **Type**: Integer
- **Description**: Maximum number of tasks when autoscaling is enabled
- **Default**: `10`
- **Example**: `5`, `10`, `20`
- **Notes**: Prevents runaway scaling and unexpected costs.

### Storage

#### `bucket_postfix`
- **Type**: String
- **Description**: Random suffix added to bucket names for uniqueness
- **Example**: `"jdlks"`, `"a7b2c"`
- **Notes**: Auto-generated if not specified. Keeps bucket names globally unique.

#### `bucket_public`
- **Type**: Boolean
- **Description**: Make the default backend bucket publicly accessible
- **Default**: `false`
- **Example**: `true`
- **Notes**: Only set to `true` if hosting public static assets. Keep `false` for security.

### Environment Variables

#### `backend_env_variables`
- **Type**: Map of key-value strings
- **Description**: Environment variables passed to the backend container
- **Example**:
  ```yaml
  backend_env_variables:
    NODE_ENV: production
    LOG_LEVEL: debug
    API_TIMEOUT: "30000"
  ```
- **Notes**: For secrets, use AWS Systems Manager Parameter Store instead.

#### `env_files_s3`
- **Type**: Array of objects
- **Description**: Load environment variables from S3 files
- **Example**:
  ```yaml
  env_files_s3:
    - bucket: myapp-secrets-dev
      key: backend.env
    - bucket: shared-configs
      key: common.env
  ```
- **Notes**: Files should be in `KEY=VALUE` format, one per line.

### IAM Policies

#### `policies`
- **Type**: Array of strings
- **Description**: AWS managed policy ARNs to attach to backend task role
- **Example**:
  ```yaml
  policies:
    - arn:aws:iam::aws:policy/AmazonS3ReadOnlyAccess
    - arn:aws:iam::aws:policy/CloudWatchLogsFullAccess
  ```

#### `backend_policies`
- **Type**: Array of policy objects
- **Description**: Custom inline IAM policies for the backend
- **Example**:
  ```yaml
  backend_policies:
    - actions:
        - s3:GetObject
        - s3:PutObject
      resources:
        - arn:aws:s3:::my-bucket/*
    - actions:
        - dynamodb:GetItem
        - dynamodb:PutItem
      resources:
        - arn:aws:dynamodb:us-east-1:123456789012:table/MyTable
  ```

### Integrations

#### `setup_fcnsns`
- **Type**: Boolean
- **Description**: Set up Firebase Cloud Messaging (FCM) with SNS for push notifications
- **Default**: `false`
- **Example**: `true`
- **Notes**: Requires additional FCM configuration.

#### `xray_enabled`
- **Type**: Boolean
- **Description**: Enable AWS X-Ray distributed tracing for the backend
- **Default**: `false`
- **Example**: `true`
- **Notes**: Useful for debugging and performance monitoring.

#### `slack_webhook`
- **Type**: String
- **Description**: Slack webhook URL for deployment notifications
- **Example**: `"https://hooks.slack.com/services/T00/B00/XXX"`
- **Notes**: Sends notifications on deployments and infrastructure changes.

### GitHub OIDC (for CI/CD)

#### `enable_github_oidc`
- **Type**: Boolean
- **Description**: Enable GitHub Actions OIDC authentication
- **Default**: `false`
- **Example**: `true`
- **Notes**: Allows GitHub Actions to deploy without long-lived credentials.

#### `github_oidc_subjects`
- **Type**: Array of strings
- **Description**: GitHub repository subjects allowed to assume the deployment role
- **Example**:
  ```yaml
  github_oidc_subjects:
    - repo:MadAppGang/myapp:ref:refs/heads/main
    - repo:MadAppGang/myapp:environment:production
    - repo:MadAppGang/*
  ```
- **Notes**: Use specific refs for security. See [GitHub OIDC docs](https://docs.github.com/en/actions/deployment/security-hardening-your-deployments/about-security-hardening-with-openid-connect).

### Database Tools

#### `install_pg_admin`
- **Type**: Boolean
- **Description**: Deploy pgAdmin web interface for database management
- **Default**: `false`
- **Example**: `true`
- **Notes**: Creates a pgAdmin container accessible through the ALB.

#### `pg_admin_email`
- **Type**: String
- **Description**: Email for pgAdmin login (when `install_pg_admin` is true)
- **Example**: `"admin@example.com"`
- **Notes**: Password is auto-generated and stored in AWS Secrets Manager.

---

## Domain Configuration

The `domain` section manages DNS and domain settings.

### `enabled`
- **Type**: Boolean
- **Description**: Enable domain/DNS configuration
- **Default**: `false`
- **Example**: `true`
- **Notes**: Required for custom domains. Disabling uses AWS-generated URLs.

### `create_domain_zone`
- **Type**: Boolean
- **Description**: Create a new Route53 hosted zone for this domain
- **Default**: `false`
- **Example**: `true`
- **Notes**: Set to `false` if using an existing zone or external DNS.

### `domain_name`
- **Type**: String
- **Description**: Your domain name (always use root domain)
- **Example**: `"example.com"`, `"myapp.io"`
- **Notes**: Always specify the root domain. Environment prefixes are added automatically.

### `add_env_domain_prefix`
- **Type**: Boolean
- **Description**: Prefix domain with environment name
- **Default**: `false`
- **Example**: `true`
- **Notes**: `true` creates `dev.example.com`, `false` uses `example.com`.

### `api_domain_prefix`
- **Type**: String
- **Description**: Custom prefix for API endpoints
- **Default**: `"api"`
- **Example**: `"api"`, `"backend"`, `"services"`
- **Notes**: Creates URLs like `api.example.com` or `api.dev.example.com`.

### DNS Delegation (Advanced)

#### `is_dns_root`
- **Type**: Boolean
- **Description**: Mark this environment as the DNS root (manages the main zone)
- **Default**: `false`
- **Example**: `true`
- **Notes**: Usually `true` for production only.

#### `dns_root_account_id`
- **Type**: String
- **Description**: AWS account ID where the root DNS zone is hosted
- **Example**: `"123456789012"`
- **Notes**: Used for cross-account subdomain delegation.

#### `delegation_role_arn`
- **Type**: String
- **Description**: IAM role ARN for cross-account DNS delegation
- **Example**: `"arn:aws:iam::123456789012:role/DNSDelegationRole"`
- **Notes**: Auto-configured by meroku DNS setup wizard.

#### `zone_id`
- **Type**: String
- **Description**: Route53 hosted zone ID for this environment
- **Example**: `"Z1234567890ABC"`
- **Notes**: Auto-populated when using meroku.

#### `root_zone_id`
- **Type**: String
- **Description**: Route53 zone ID of the root domain (for delegated subdomains)
- **Example**: `"Z0987654321XYZ"`
- **Notes**: Used in non-production environments with subdomain delegation.

---

## Database Configuration (PostgreSQL)

The `postgres` section configures RDS Aurora PostgreSQL databases.

### `enabled`
- **Type**: Boolean
- **Description**: Enable PostgreSQL database provisioning
- **Default**: `false`
- **Example**: `true`
- **Notes**: Creates an Aurora PostgreSQL cluster.

### `dbname`
- **Type**: String
- **Description**: Initial database name
- **Example**: `"myapp"`, `"production"`
- **Notes**: Additional databases can be created later via SQL.

### `username`
- **Type**: String
- **Description**: Master username for the database
- **Example**: `"dbadmin"`, `"postgres"`
- **Notes**: Password is auto-generated and stored in AWS Secrets Manager.

### `public_access`
- **Type**: Boolean
- **Description**: Allow public internet access to the database
- **Default**: `false`
- **Example**: `true`
- **Notes**: Only enable for development. Keep `false` for production security.

### `engine_version`
- **Type**: String
- **Description**: PostgreSQL engine version
- **Default**: `"16.x"`
- **Example**: `"14.x"`, `"15.x"`, `"16.x"`
- **Notes**: Use the latest stable version. Minor versions auto-update.

### Aurora Serverless v2

#### `aurora`
- **Type**: Boolean
- **Description**: Use Aurora Serverless v2 instead of provisioned instances
- **Default**: `false`
- **Example**: `true`
- **Notes**: Serverless auto-scales and can reduce to zero during inactivity.

#### `min_capacity`
- **Type**: Float
- **Description**: Minimum Aurora Capacity Units (ACUs) for serverless
- **Default**: `0.5`
- **Example**: `0.5`, `1`, `2`
- **Notes**: 0.5 ACU = ~1GB RAM. Minimum billable capacity.

#### `max_capacity`
- **Type**: Float
- **Description**: Maximum Aurora Capacity Units (ACUs) for serverless
- **Default**: `1`
- **Example**: `1`, `2`, `16`
- **Notes**: Prevents runaway scaling costs. 1 ACU = ~2GB RAM.

---

## Authentication (Cognito)

The `cognito` section configures AWS Cognito user pools for authentication.

### `enabled`
- **Type**: Boolean
- **Description**: Enable Cognito user pool creation
- **Default**: `false`
- **Example**: `true`
- **Notes**: Provides user registration, login, and token management.

### `auto_verified_attributes`
- **Type**: Array of strings
- **Description**: User attributes that are automatically verified
- **Example**: `["email"]`, `["phone_number"]`, `["email", "phone_number"]`
- **Notes**: Empty array `[]` skips verification. Recommended: `["email"]`.

### `enable_web_client`
- **Type**: Boolean
- **Description**: Create an app client for web applications
- **Default**: `false`
- **Example**: `true`
- **Notes**: Generates a client ID for frontend authentication.

### `enable_dashboard_client`
- **Type**: Boolean
- **Description**: Create a separate app client for admin dashboards
- **Default**: `false`
- **Example**: `true`
- **Notes**: Useful for separating user and admin authentication flows.

### `dashboard_callback_ur_ls`
- **Type**: Array of strings
- **Description**: OAuth callback URLs for the dashboard client
- **Example**:
  ```yaml
  dashboard_callback_urls:
    - https://admin.example.com/callback
    - http://localhost:3000/callback
  ```
- **Notes**: Required when `enable_dashboard_client` is `true`.

### `enable_user_pool_domain`
- **Type**: Boolean
- **Description**: Create a Cognito hosted UI domain
- **Default**: `false`
- **Example**: `true`
- **Notes**: Provides a ready-to-use login/signup UI.

### `user_pool_domain_prefix`
- **Type**: String
- **Description**: Prefix for the Cognito hosted domain
- **Example**: `"myapp-auth"`, `"mycompany-login"`
- **Notes**: Creates domain like `myapp-auth.auth.us-east-1.amazoncognito.com`.

### `backend_confirm_signup`
- **Type**: Boolean
- **Description**: Let backend confirm signups instead of email/SMS
- **Default**: `false`
- **Example**: `true`
- **Notes**: Requires custom logic in your backend application.

---

## Email Service (SES)

The `ses` section configures Amazon Simple Email Service.

### `enabled`
- **Type**: Boolean
- **Description**: Enable SES for sending emails
- **Default**: `false`
- **Example**: `true`
- **Notes**: Starts in sandbox mode. Request production access from AWS.

### `domain_name`
- **Type**: String
- **Description**: Custom domain for sending emails (optional)
- **Example**: `"mail.example.com"`, `"noreply.myapp.com"`
- **Notes**: Leave empty to use the default domain. Requires DNS verification.

### `test_emails`
- **Type**: Array of strings
- **Description**: Email addresses verified for testing in SES sandbox
- **Example**:
  ```yaml
  test_emails:
    - developer@example.com
    - qa@example.com
  ```
- **Notes**: In sandbox mode, you can only send to verified addresses.

---

## Message Queue (SQS)

The `sqs` section configures Amazon Simple Queue Service.

### `enabled`
- **Type**: Boolean
- **Description**: Enable SQS queue creation
- **Default**: `false`
- **Example**: `true`
- **Notes**: Creates a standard SQS queue with dead-letter queue.

### `name`
- **Type**: String
- **Description**: Queue name suffix
- **Example**: `"main"`, `"default-queue"`, `"jobs"`
- **Notes**: Full name becomes `{project}-{name}-{env}`.

---

## Load Balancer (ALB)

The `alb` section configures Application Load Balancer.

### `enabled`
- **Type**: Boolean
- **Description**: Create an Application Load Balancer
- **Default**: `false`
- **Example**: `true`
- **Notes**: Alternative to API Gateway. Better for WebSockets and gRPC.

---

## Services

The `services` section defines additional ECS services beyond the main backend.

### Service Fields

#### `name`
- **Type**: String (required)
- **Description**: Service name (used in resource naming)
- **Example**: `"worker"`, `"cron"`, `"websocket"`

#### `docker_image`
- **Type**: String
- **Description**: Custom Docker image for this service
- **Example**: `"myregistry/worker:latest"`
- **Notes**: Defaults to project ECR if not specified.

#### `container_command`
- **Type**: Array of strings
- **Description**: Command to run in the container
- **Example**: `["node", "worker.js"]`, `["python", "consumer.py"]`

#### `container_port`
- **Type**: Integer
- **Description**: Port the service listens on
- **Example**: `3000`, `8080`, `9000`

#### `host_port`
- **Type**: Integer
- **Description**: Port exposed on the host
- **Example**: `3000`, `8080`
- **Notes**: Usually matches `container_port`.

#### `cpu`
- **Type**: Integer
- **Description**: CPU units for this service
- **Example**: `256`, `512`, `1024`

#### `memory`
- **Type**: Integer
- **Description**: Memory in MB for this service
- **Example**: `512`, `1024`, `2048`

#### `desired_count`
- **Type**: Integer
- **Description**: Number of tasks to run
- **Example**: `1`, `2`, `3`

#### `remote_access`
- **Type**: Boolean
- **Description**: Enable ECS Exec for remote shell access
- **Default**: `false`
- **Example**: `true`

#### `xray_enabled`
- **Type**: Boolean
- **Description**: Enable X-Ray tracing for this service
- **Default**: `false`
- **Example**: `true`

#### `essential`
- **Type**: Boolean
- **Description**: Task fails if this container stops
- **Default**: `true`
- **Example**: `false`

#### `env_vars`
- **Type**: Map of strings
- **Description**: Environment variables for the service
- **Example**:
  ```yaml
  env_vars:
    WORKER_THREADS: "4"
    QUEUE_NAME: "jobs"
  ```

#### `env_variables`
- **Type**: Array of name/value objects
- **Description**: Alternative format for environment variables
- **Example**:
  ```yaml
  env_variables:
    - name: API_KEY
      value: "secret123"
  ```

#### `env_files_s3`
- **Type**: Array of bucket/key objects
- **Description**: Load environment from S3 files
- **Example**:
  ```yaml
  env_files_s3:
    - bucket: secrets-bucket
      key: worker.env
  ```

### Complete Service Example

```yaml
services:
  - name: worker
    container_port: 3000
    host_port: 3000
    cpu: 512
    memory: 1024
    desired_count: 2
    remote_access: true
    xray_enabled: false
    container_command: ["node", "worker.js"]
    env_vars:
      WORKER_THREADS: "4"
      REDIS_URL: "redis://cache:6379"
    env_files_s3:
      - bucket: myapp-secrets
        key: worker.env
```

---

## Scheduled Tasks

The `scheduled_tasks` section defines cron-like tasks running on ECS.

### Task Fields

#### `name`
- **Type**: String (required)
- **Description**: Task name
- **Example**: `"daily-backup"`, `"cleanup"`, `"report"`

#### `schedule`
- **Type**: String (required)
- **Description**: CloudWatch Events schedule expression
- **Example**: `"rate(1 hour)"`, `"rate(1 day)"`, `"cron(0 2 * * ? *)"`
- **Notes**: Use `rate()` for intervals, `cron()` for specific times.

#### `docker_image`
- **Type**: String
- **Description**: Custom Docker image for this task
- **Example**: `"myregistry/backup:latest"`
- **Notes**: Defaults to project ECR.

#### `container_command`
- **Type**: String or array
- **Description**: Command to run
- **Example**: `'["backup", "--full"]'` or `"npm run backup"`

#### `allow_public_access`
- **Type**: Boolean
- **Description**: Allow task to access the internet
- **Default**: `false`
- **Example**: `true`

### Example

```yaml
scheduled_tasks:
  - name: daily-report
    schedule: "cron(0 8 * * ? *)"  # 8 AM UTC daily
    docker_image: ""  # Use default ECR
    container_command: '["node", "scripts/report.js"]'
    allow_public_access: true
  - name: hourly-cleanup
    schedule: "rate(1 hour)"
    container_command: '["cleanup"]'
    allow_public_access: false
```

---

## Event Processor Tasks

The `event_processor_tasks` section defines tasks triggered by EventBridge events.

### Task Fields

#### `name`
- **Type**: String (required)
- **Description**: Task name
- **Example**: `"deployment-handler"`, `"notification-sender"`

#### `rule_name`
- **Type**: String (required)
- **Description**: EventBridge rule name
- **Example**: `"handle-deployments"`, `"process-orders"`

#### `sources`
- **Type**: Array of strings
- **Description**: Event sources to listen to
- **Example**: `["aws.ecs"]`, `["custom-app"]`, `["github"]`

#### `detail_types`
- **Type**: Array of strings
- **Description**: Event detail types to match
- **Example**:
  ```yaml
  detail_types:
    - SERVICE_DEPLOYMENT_COMPLETED
    - SERVICE_DEPLOYMENT_FAILED
  ```

#### `docker_image`
- **Type**: String
- **Description**: Custom Docker image
- **Example**: `"processor:latest"`

#### `container_command`
- **Type**: Array of strings
- **Description**: Command to execute
- **Example**: `["node", "handlers/deploy.js"]`

#### `allow_public_access`
- **Type**: Boolean
- **Description**: Allow internet access
- **Default**: `false`
- **Example**: `true`

### Example

```yaml
event_processor_tasks:
  - name: deployment-notifier
    rule_name: notify_on_deploy
    sources:
      - aws.ecs
    detail_types:
      - ECS Task State Change
      - ECS Service Action
    container_command: ["node", "notify.js"]
    allow_public_access: true
```

---

## File Storage (EFS)

The `efs` section defines Elastic File System volumes.

### EFS Fields

#### `name`
- **Type**: String (required)
- **Description**: EFS volume name
- **Example**: `"uploads"`, `"static"`, `"shared-data"`
- **Notes**: Creates EFS with name `{project}-{name}-{env}`.

### Example

```yaml
efs:
  - name: uploads    # For user-uploaded files
  - name: static     # For static assets
  - name: cache      # For application cache
```

---

## S3 Buckets

The `buckets` section defines additional S3 buckets beyond the default backend bucket.

### Bucket Fields

#### `name`
- **Type**: String (required)
- **Description**: Bucket name suffix
- **Example**: `"uploads"`, `"backups"`, `"exports"`
- **Notes**: Full name becomes `{project}-{name}-{env}`.

#### `public`
- **Type**: Boolean
- **Description**: Allow public read access
- **Default**: `false`
- **Example**: `true`
- **Notes**: Only enable for public assets. Keep private for user data.

#### `versioning`
- **Type**: Boolean (pointer, can be null)
- **Description**: Enable S3 versioning
- **Default**: `true`
- **Example**: `false`
- **Notes**: Versioning helps prevent accidental deletions.

#### `cors_rules`
- **Type**: Array of CORS rule objects
- **Description**: CORS configuration for browser access
- **Example**:
  ```yaml
  cors_rules:
    - allowed_headers: ["*"]
      allowed_methods: ["GET", "PUT", "POST"]
      allowed_origins: ["https://example.com"]
      expose_headers: ["ETag"]
      max_age_seconds: 3600
  ```

### Complete Bucket Example

```yaml
buckets:
  - name: uploads
    public: false
    versioning: true
    cors_rules:
      - allowed_headers: ["*"]
        allowed_methods: ["GET", "PUT", "POST", "DELETE"]
        allowed_origins:
          - "https://app.example.com"
          - "https://www.example.com"
        expose_headers: ["ETag", "x-amz-version-id"]
        max_age_seconds: 3600

  - name: public-assets
    public: true
    versioning: false
    cors_rules:
      - allowed_headers: ["*"]
        allowed_methods: ["GET"]
        allowed_origins: ["*"]
        expose_headers: ["ETag"]
        max_age_seconds: 86400
```

---

## Frontend Deployment (Amplify)

The `amplify_apps` section configures AWS Amplify for frontend hosting.

### App Fields

#### `name`
- **Type**: String (required)
- **Description**: Amplify app name
- **Example**: `"web"`, `"admin-dashboard"`, `"marketing-site"`

#### `github_repository`
- **Type**: String (required)
- **Description**: GitHub repository URL
- **Example**: `"https://github.com/username/repo"`

#### `custom_domain`
- **Type**: String
- **Description**: Custom domain for the app
- **Example**: `"example.com"`, `"app.example.com"`

#### `enable_root_domain`
- **Type**: Boolean
- **Description**: Enable the root domain (not just www)
- **Default**: `false`
- **Example**: `true`
- **Notes**: Redirects `www.example.com` to `example.com`.

#### `sub_domains`
- **Type**: Array of strings
- **Description**: Additional subdomains
- **Example**: `["www", "app", "beta"]`

### Branch Configuration (Recommended)

#### `branches`
- **Type**: Array of branch objects
- **Description**: Multiple branch deployments with individual configs
- **Example**:
  ```yaml
  branches:
    - name: main
      stage: PRODUCTION
      enable_auto_build: true
      enable_pull_request_preview: true
      environment_variables:
        REACT_APP_API_URL: https://api.example.com
        REACT_APP_ENV: production

    - name: develop
      stage: DEVELOPMENT
      enable_auto_build: true
      enable_pull_request_preview: true
      environment_variables:
        REACT_APP_API_URL: https://api-dev.example.com
        REACT_APP_ENV: development
  ```

### Legacy Single Branch (Still Supported)

#### `branch_name`
- **Type**: String
- **Description**: Single branch to deploy (legacy format)
- **Example**: `"main"`, `"master"`, `"production"`

#### `enable_pr_preview`
- **Type**: Boolean
- **Description**: Enable pull request previews (legacy)
- **Default**: `false`
- **Example**: `true`

#### `environment_variables`
- **Type**: Map of strings
- **Description**: App-level environment variables (legacy)
- **Example**:
  ```yaml
  environment_variables:
    REACT_APP_API_URL: https://api.example.com
  ```

### Complete Amplify Example

```yaml
amplify_apps:
  - name: main-web
    github_repository: https://github.com/mycompany/frontend
    custom_domain: example.com
    sub_domains:
      - www
      - app
    enable_root_domain: true

    branches:
      - name: main
        stage: PRODUCTION
        enable_auto_build: true
        enable_pull_request_preview: false
        environment_variables:
          REACT_APP_API_URL: https://api.example.com
          REACT_APP_COGNITO_REGION: us-east-1
          REACT_APP_COGNITO_USER_POOL_ID: ${cognito_user_pool_id}
          REACT_APP_COGNITO_CLIENT_ID: ${cognito_web_client_id}
          REACT_APP_ENV: production

      - name: develop
        stage: DEVELOPMENT
        enable_auto_build: true
        enable_pull_request_preview: true
        environment_variables:
          REACT_APP_API_URL: https://api-dev.example.com
          REACT_APP_ENV: development

  - name: admin-dashboard
    github_repository: https://github.com/mycompany/admin
    branch_name: main  # Legacy single branch format
    custom_domain: admin.example.com
    enable_root_domain: true
    environment_variables:
      VUE_APP_API_URL: https://api.example.com
```

**Notes:**
- Use `${variable_name}` to reference Terraform outputs
- Branch-specific vars override app-level vars
- PR previews create temporary URLs for testing

---

## GraphQL API (AppSync)

The `pubsub_appsync` section configures AWS AppSync for GraphQL APIs.

### `enabled`
- **Type**: Boolean
- **Description**: Enable AppSync GraphQL API
- **Default**: `false`
- **Example**: `true`

### `schema`
- **Type**: Boolean
- **Description**: Deploy GraphQL schema
- **Default**: `false`
- **Example**: `true`
- **Notes**: Schema file should be at `modules/appsync/schema.graphql`.

### `auth_lambda`
- **Type**: Boolean
- **Description**: Use Lambda for AppSync authorization
- **Default**: `false`
- **Example**: `true`
- **Notes**: Lambda function should be at `modules/appsync/auth_lambda/`.

### `resolvers`
- **Type**: Boolean
- **Description**: Deploy VTL resolvers
- **Default**: `false`
- **Example**: `true`
- **Notes**: Resolvers defined in `modules/appsync/vtl_templates.yaml`.

### Example

```yaml
pubsub_appsync:
  enabled: true
  schema: true
  auth_lambda: true
  resolvers: true
```

---

## Complete Example

Here's a comprehensive example combining all sections:

```yaml
# Core settings
project: myapp
env: dev
is_prod: false
region: us-east-1
account_id: "123456789012"
aws_profile: myapp-dev
state_bucket: myapp-terraform-state-dev
state_file: state.tfstate

# Workload
workload:
  backend_health_endpoint: /health/live
  backend_image_port: 8080
  backend_alb_domain_name: api-dev.example.com
  bucket_postfix: x7k2m
  bucket_public: false

  # Scaling
  backend_desired_count: 2
  backend_cpu: "512"
  backend_memory: "1024"
  backend_autoscaling_enabled: true
  backend_autoscaling_min_capacity: 1
  backend_autoscaling_max_capacity: 10

  # Environment
  backend_env_variables:
    NODE_ENV: development
    LOG_LEVEL: debug

  # Integrations
  xray_enabled: true
  enable_github_oidc: true
  github_oidc_subjects:
    - repo:mycompany/myapp:ref:refs/heads/main

  slack_webhook: https://hooks.slack.com/services/XXX

  # Tools
  install_pg_admin: true
  pg_admin_email: admin@example.com

# Domain
domain:
  enabled: true
  create_domain_zone: false
  domain_name: example.com
  add_env_domain_prefix: true  # Creates dev.example.com
  api_domain_prefix: api       # Creates api.dev.example.com

# Database
postgres:
  enabled: true
  dbname: myapp
  username: dbadmin
  public_access: false
  engine_version: "16.x"
  aurora: true
  min_capacity: 0.5
  max_capacity: 2

# Authentication
cognito:
  enabled: true
  auto_verified_attributes:
    - email
  enable_web_client: true
  enable_dashboard_client: true
  dashboard_callback_urls:
    - https://admin.dev.example.com/callback
  enable_user_pool_domain: true
  user_pool_domain_prefix: myapp-dev-auth
  backend_confirm_signup: false

# Email
ses:
  enabled: true
  domain_name: mail.example.com
  test_emails:
    - developer@example.com

# Queue
sqs:
  enabled: true
  name: main

# Load Balancer
alb:
  enabled: true

# Additional services
services:
  - name: worker
    container_port: 3000
    cpu: 256
    memory: 512
    desired_count: 1
    remote_access: true
    container_command: ["node", "worker.js"]
    env_vars:
      WORKER_THREADS: "4"

# Scheduled tasks
scheduled_tasks:
  - name: daily-cleanup
    schedule: "cron(0 2 * * ? *)"
    container_command: '["cleanup"]'
    allow_public_access: false

# Event processors
event_processor_tasks:
  - name: deploy-notifier
    rule_name: notify_deploys
    sources:
      - aws.ecs
    detail_types:
      - ECS Task State Change
    container_command: ["node", "notify.js"]
    allow_public_access: true

# File storage
efs:
  - name: uploads
  - name: static

# S3 buckets
buckets:
  - name: uploads
    public: false
    versioning: true
    cors_rules:
      - allowed_methods: ["GET", "PUT", "POST"]
        allowed_origins: ["https://dev.example.com"]
        max_age_seconds: 3600

# Frontend
amplify_apps:
  - name: web
    github_repository: https://github.com/mycompany/frontend
    custom_domain: dev.example.com
    enable_root_domain: true
    branches:
      - name: develop
        stage: DEVELOPMENT
        enable_auto_build: true
        enable_pull_request_preview: true
        environment_variables:
          REACT_APP_API_URL: https://api.dev.example.com
          REACT_APP_ENV: development

# GraphQL
pubsub_appsync:
  enabled: false
  schema: false
  auth_lambda: false
  resolvers: false
```

---

## Best Practices

1. **Start Small**: Begin with minimal configuration and add features as needed
2. **Use Comments**: YAML supports comments (`#`) - document your choices
3. **Environment Parity**: Keep dev/staging/prod configs similar, vary only scaling and domains
4. **Secret Management**: Never put secrets in YAML - use Parameter Store or Secrets Manager
5. **Version Control**: Commit all environment files to track infrastructure changes
6. **Test Changes**: Always run `terraform plan` before `apply`
7. **Resource Naming**: Use consistent, descriptive names for all resources
8. **Enable Autoscaling**: For production, always enable autoscaling with reasonable limits
9. **Monitoring**: Enable X-Ray and CloudWatch for production environments
10. **Backup Strategy**: Enable database backups and S3 versioning for production

---

## Getting Help

- **Field Validation**: The meroku CLI validates fields when generating Terraform
- **Examples**: Check `receipts/examples/` directory for more examples
- **Issues**: Report problems at [GitHub Issues](https://github.com/MadAppGang/infrastructure/issues)
- **Documentation**: See main [README.md](../README.md) for overview

---

**Last Updated**: Based on meroku v3.5.14
