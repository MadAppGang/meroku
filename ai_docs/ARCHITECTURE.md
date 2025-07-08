# Infrastructure Architecture Documentation

This document provides a comprehensive overview of the AWS infrastructure architecture created by this Terraform repository, explaining how YAML configuration maps to AWS resources and how all components work together.

## Architecture Overview

The infrastructure creates a modern, scalable AWS architecture using:
- **Container Orchestration**: ECS Fargate for serverless containers
- **Networking**: VPC with public/private subnets, ALB, API Gateway
- **Database**: RDS PostgreSQL with optional pgAdmin
- **Authentication**: AWS Cognito for user management
- **Storage**: S3 for objects, EFS for shared files
- **Messaging**: SQS for queues, SNS for notifications
- **Email**: SES for transactional emails
- **Monitoring**: CloudWatch, X-Ray for distributed tracing

## Core Infrastructure Components

### 1. Container Platform (ECS)

The workloads module creates the core container infrastructure:

```yaml
# YAML Configuration
workload:
  backend_image_port: 8080
  backend_health_endpoint: /health
  xray_enabled: true
  backend_env_variables:
    - name: NODE_ENV
      value: production
```

**Creates:**
- **ECS Cluster** (`{project}_cluster_{env}`)
- **Backend Service**:
  - Fargate task definition with configurable CPU/memory
  - Auto-scaling policies
  - Health checks
  - CloudWatch logs (7-day retention)
  - X-Ray sidecar container (optional)
- **Service Discovery**: Private DNS namespace for internal communication
- **Security Groups**: Network isolation with VPC-only ingress

### 2. Additional Services

Each service in the `services` array creates a complete microservice stack:

```yaml
services:
  - name: worker
    cpu: 512
    memory: 1024
    container_port: 3000
    desired_count: 2
```

**Creates per service:**
- ECS Service and Task Definition
- ECR Repository (dev environment only)
- Security Group
- CloudWatch Log Group
- IAM Roles (execution and task)
- Service Discovery registration
- Optional ALB target group

### 3. Networking Architecture

#### API Gateway (Default)
- HTTP API for public access
- Security group-based access control (VPC Links deprecated for cost savings)
- Path-based routing: `/{service}/*` → service

#### Application Load Balancer (Optional)
```yaml
alb:
  enabled: true
workload:
  backend_alb_domain_name: api.example.com
```

**Creates:**
- Public-facing ALB in private subnets
- HTTPS listener with SSL/TLS termination
- HTTP to HTTPS redirect
- Target groups with health checks
- Route53 alias records

### 4. Domain & SSL Management

```yaml
domain:
  enabled: true
  domain_name: example.com
  api_domain_prefix: api
  add_domain_prefix: true  # false for prod
```

**Creates:**
- Route53 hosted zone (optional)
- ACM certificates:
  - Wildcard: `*.example.com`
  - API: `api.example.com` or `api.dev.example.com`
- DNS validation records
- Environment-based subdomain structure:
  - Dev: `dev.example.com`, `api.dev.example.com`
  - Prod: `example.com`, `api.example.com`

### 5. Database Layer

```yaml
postgres:
  enabled: true
  dbname: myapp
  username: postgres
  engine_version: "16"
  public_access: false
```

**Creates:**
- RDS PostgreSQL instance
- Auto-generated password in SSM Parameter Store:
  - `/{env}/{project}/postgres_password`
  - `/{env}/{project}/backend/pg_database_password`
- Security group (port 5432)
- Optional pgAdmin web interface

### 6. Authentication System

```yaml
cognito:
  enabled: true
  enable_web_client: true
  enable_user_pool_domain: true
  user_pool_domain_prefix: myapp-auth
```

**Creates:**
- Cognito User Pool with:
  - Email verification
  - Password policy (min 8 chars)
  - MFA optional
  - User groups: "test", "admin"
- App clients for web/dashboard
- Hosted UI for authentication
- IAM policy for backend user management

### 7. Storage Solutions

#### Object Storage (S3)
```yaml
buckets:
  - name: assets
    public: true
workload:
  bucket_postfix: prod123
  bucket_public: false
```

**Creates:**
- Backend bucket: `{project}-backend-{env}-{postfix}`
- Additional buckets: `{project}-{name}-{env}`
- Public access policies (when enabled)
- CORS configuration
- Versioning support

#### File Storage (EFS)
```yaml
efs:
  - name: uploads
    path: /uploads
workload:
  efs:
    - name: uploads
      mount_point: /app/uploads
```

**Creates:**
- EFS file system with encryption
- Mount targets in all private subnets
- Access points with POSIX permissions
- Security groups for NFS access
- Container mount configuration

### 8. Messaging & Notifications

#### Queue (SQS)
```yaml
sqs:
  enabled: true
  name: main-queue
```

**Creates:**
- Standard SQS queue
- IAM policy for queue operations
- Automatic integration with services

#### Push Notifications (SNS)
```yaml
workload:
  setup_fcnsns: true
```

**Creates:**
- SNS Platform Application for FCM
- SSM Parameter for GCM server key
- IAM policies for push operations

### 9. Email Service

```yaml
ses:
  enabled: true
  domain_name: mail.example.com
  test_emails:
    - admin@example.com
```

**Creates:**
- SES domain identity
- Email authentication:
  - DKIM (3 CNAME records)
  - SPF (TXT record)
  - DMARC policy
- Test email verifications

### 10. Scheduled & Event-Driven Tasks

#### Scheduled Tasks (Cron)
```yaml
scheduled_tasks:
  - name: daily-cleanup
    schedule: "rate(1 day)"
    container_command: '["node", "cleanup.js"]'
```

**Creates:**
- EventBridge Scheduler rule
- ECS task definition
- IAM execution role
- Optional public IP assignment

#### Event-Driven Tasks
```yaml
event_processor_tasks:
  - name: order-processor
    rule_name: order-events
    detail_types: ["Order Created"]
    sources: ["com.myapp.orders"]
```

**Creates:**
- EventBridge rule with pattern matching
- ECS task triggered by events
- Dead letter queue for failures

## Security Architecture

### Network Security
- **VPC Isolation**: Default VPC with public/private subnets
- **Security Groups**: Least-privilege access rules
- **Private Endpoints**: Services communicate internally via Service Discovery

### Identity & Access Management
- **Task Roles**: Service-specific AWS permissions
- **Execution Roles**: Container runtime permissions
- **OIDC**: GitHub Actions passwordless deployments
- **Custom Policies**: Additional permissions via YAML

```yaml
workload:
  policy:
    - actions: ["s3:GetObject", "s3:PutObject"]
      resources: ["arn:aws:s3:::my-bucket/*"]
```

### Secrets Management
- **SSM Parameter Store**: Database passwords, API keys
- **Environment Files**: S3-stored configuration
- **Container Secrets**: Injected at runtime

## Deployment Architecture

### Container Image Flow
1. **Development**:
   - Local ECR repositories created
   - Images pushed to: `{account}.dkr.ecr.{region}.amazonaws.com/{project}_backend`

2. **Production**:
   - Cross-account ECR access
   - Images from: `{ecr_account_id}.dkr.ecr.{ecr_account_region}.amazonaws.com/{project}_backend`

### CI/CD Integration
- **GitHub OIDC**: Passwordless AWS access
- **EventBridge**: Watches ECR for new images
- **Lambda Deployer**: Updates ECS services
- **Slack Notifications**: Deployment status

### Environment Configuration
- **SSM Parameters**: `/{env}/{project}/backend/*`
- **Environment Variables**: Direct injection
- **S3 Config Files**: Loaded at container start

## Monitoring & Observability

### Logging
- **CloudWatch Logs**: All container output
- **Log Groups**: Per service organization
- **Retention**: 7 days default

### Tracing
- **X-Ray**: Distributed request tracing
- **Service Map**: Visual service dependencies
- **Performance Insights**: Latency analysis

### Metrics
- **CloudWatch Metrics**: CPU, memory, request counts
- **Custom Metrics**: Application-specific data
- **Alarms**: Configurable thresholds

## Scaling & High Availability

### Auto-Scaling
- **ECS Services**: Min/max task counts
- **Target Tracking**: CPU/memory based
- **Schedule-Based**: Time-based scaling

### High Availability
- **Multi-AZ Deployment**: Services spread across zones
- **Load Balancing**: ALB/API Gateway distribution
- **Database**: RDS Multi-AZ option
- **Self-Healing**: Automatic task replacement

## Cost Optimization

### Resource Efficiency
- **Fargate**: Pay-per-use containers
- **Spot Instances**: Optional for non-critical workloads
- **Right-Sizing**: Configurable CPU/memory

### Storage Optimization
- **S3 Lifecycle**: Object expiration policies
- **EFS Infrequent Access**: Automatic tiering
- **CloudWatch Logs**: Configurable retention

## Disaster Recovery

### Backup Strategy
- **RDS**: Automated backups (7-day retention)
- **S3**: Optional versioning
- **Infrastructure**: Terraform state in S3

### Recovery Procedures
- **Infrastructure**: `terraform apply` recreates all resources
- **Data**: RDS point-in-time recovery
- **Configuration**: Version-controlled YAML

## Common Patterns

### Service Communication
```
Internet → API Gateway/ALB → ECS Service → RDS/S3/SQS
                            ↓
                    Service Discovery
                            ↓
                    Internal Services
```

### Authentication Flow
```
User → Cognito → JWT Token → API Gateway → Backend Service
                                               ↓
                                          Validate Token
```

### Deployment Flow
```
GitHub → Build → ECR → EventBridge → Lambda → ECS Update
                                        ↓
                                  Slack Notification
```

## Terraform Outputs

The infrastructure provides these outputs for integration:
- `backend_ecr_repo_url`: ECR repository for backend images
- `account_id`: AWS account ID
- `region`: AWS region
- `backend_task_role_name`: IAM role for backend tasks
- `backend_cloud_map_arn`: Service discovery namespace

## Best Practices Implemented

1. **Infrastructure as Code**: Everything defined in Terraform
2. **Environment Parity**: Dev/prod use same architecture
3. **Immutable Infrastructure**: Containers rebuilt on changes
4. **Secrets Management**: No hardcoded credentials
5. **Monitoring First**: Comprehensive observability
6. **Security by Default**: Least privilege access
7. **Cost Awareness**: Resource tagging and optimization
8. **Disaster Recovery**: Automated backups and recovery

This architecture provides a production-ready, scalable, and secure foundation for modern cloud applications.