# Architecture Diagram Node Mapping to YAML Configuration

This document maps each node in the architecture diagram to its corresponding YAML configuration settings, including enable/disable states.

## Node States
- **Enabled**: Node is active when corresponding YAML setting is configured
- **Disabled/Dimmed**: Node is greyed out when YAML setting is false or not configured
- **Required**: Always enabled (Backend Service)

## External Entry Points

### 1. Client App
**Status**: Always enabled (external to infrastructure)
**Description**: External client applications (web, mobile)
**YAML**: N/A - external component

### 2. Admin App  
**Status**: Enabled when Cognito dashboard client is configured
**YAML**:
```yaml
cognito:
  enabled: true
  enable_dashboard_client: true
  dashboard_callback_urls:
    - "https://admin.example.com/callback"
```

### 3. GitHub Actions
**Status**: Enabled when GitHub OIDC is configured
**YAML**:
```yaml
workload:
  enable_github_oidc: true
  github_oidc_subjects:
    - "repo:Owner/Repo:ref:refs/heads/main"
```

## Authentication Layer

### 4. Authentication System (AWS Cognito)
**Status**: Enabled/Disabled based on cognito.enabled
**YAML**:
```yaml
cognito:
  enabled: true  # false = dimmed node
  enable_web_client: true
  enable_user_pool_domain: true
  user_pool_domain_prefix: "myapp-auth"
  auto_verified_attributes: ["email"]
```
**Properties**:
- User Pool ID
- Client IDs
- Domain URL
- User Groups: "test", "admin"

## API Gateway Layer

### 5. Amazon API Gateway
**Status**: Always enabled (default ingress)
**YAML**: No explicit configuration needed - created by default
**Properties**:
- HTTP API
- VPC Links
- Path-based routing: `/{service}/*`
- JWT Authorization (if Cognito enabled)

### 6. AWS Amplify
**Status**: Disabled in current architecture
**YAML**: N/A - not implemented
**Properties**: Frontend distribution (when enabled)

## Load Balancing Layer

### 7. Amazon Route 53
**Status**: Enabled when domain.enabled is true
**YAML**:
```yaml
domain:
  enabled: true  # false = dimmed node
  domain_name: "example.com"
  create_domain_zone: true
  api_domain_prefix: "api"
  add_domain_prefix: true  # false for prod
```
**Properties**:
- Hosted Zone ID
- Domain records
- Certificate validation records

### 8. AWS WAF
**Status**: Disabled (not implemented)
**YAML**: N/A - future enhancement
**Properties**: Web Application Firewall rules

## Container Orchestration

### 9. Amazon ECS Cluster
**Status**: Always enabled (core component)
**YAML**:
```yaml
project: myapp
env: dev
```
**Properties**:
- Cluster Name: `{project}_cluster_{env}`
- Fargate launch type
- Container Insights enabled

## ECS Services

### 10. Backend Service (Required)
**Status**: Always enabled
**YAML**:
```yaml
workload:
  backend_image_port: 8080
  backend_health_endpoint: "/health"
  backend_remote_access: true
  backend_external_docker_image: ""  # empty = use ECR
  backend_container_command: []  # optional override
  backend_env_variables:
    - name: NODE_ENV
      value: production
```
**Properties**:
- Service Name: `{project}_backend_{env}`
- Task Definition
- Desired Count: 1
- CPU: 256 (512 with X-Ray)
- Memory: 512 (1024 with X-Ray)

### 11. Additional Services (Right of Backend)
**Status**: Enabled when services array has entries
**YAML**:
```yaml
services:
  - name: worker
    docker_image: ""  # optional override
    container_command: ["node", "worker.js"]
    container_port: 3000
    cpu: 512
    memory: 1024
    desired_count: 2
    remote_access: true
    xray_enabled: false
    env_vars:
      - name: WORKER_TYPE
        value: background
```
**Properties per service**:
- Service Name: `{project}_{name}_{env}`
- ECR Repository (dev only)
- Security Group
- CloudWatch Logs

### 12. Scheduled Tasks (Below Services)
**Status**: Enabled when scheduled_tasks array has entries
**YAML**:
```yaml
scheduled_tasks:
  - name: daily-cleanup
    schedule: "rate(1 day)"
    docker_image: ""  # optional, uses backend if empty
    container_command: '["node", "scripts/cleanup.js"]'
    allow_public_access: false
```
**Properties per task**:
- EventBridge Schedule Rule
- ECS Task Definition
- IAM Execution Role

### 13. Event Processor Tasks (Below Services)
**Status**: Enabled when event_processor_tasks array has entries
**YAML**:
```yaml
event_processor_tasks:
  - name: order-processor
    rule_name: order-events
    detail_types: ["Order Created"]
    sources: ["com.myapp.orders"]
    docker_image: ""  # optional
    container_command: '["node", "process.js"]'
    allow_public_access: false
```
**Properties per task**:
- EventBridge Rule
- Pattern matching
- ECS Task trigger

## Container Registry

### 14. Amazon ECR
**Status**: Always enabled in dev, cross-account in prod
**YAML**:
```yaml
# Cross-account configuration
ecr_account_id: "123456789012"
ecr_account_region: "us-east-1"
```
**Properties**:
- Repository: `{project}_backend`
- Lifecycle policies
- Cross-account permissions

### 15. Amazon Aurora
**Status**: Disabled (RDS PostgreSQL used instead)
**YAML**: N/A
**Alternative**: See RDS PostgreSQL configuration

## Storage Layer

### 16. Amazon S3
**Status**: Always enabled (backend bucket required)
**YAML**:
```yaml
workload:
  bucket_postfix: "prod123"  # required
  bucket_public: false

# Additional buckets
buckets:
  - name: assets
    public: true
```
**Properties**:
- Backend bucket: `{project}-backend-{env}-{postfix}`
- Additional buckets: `{project}-{name}-{env}`
- CORS configuration
- Public access settings

### 17. Amazon EventBridge
**Status**: Always enabled for deployments
**YAML**: Automatically configured
**Properties**:
- ECR image push events
- Custom event bus (optional)
- Failed event DLQ

### 18. Amazon SNS
**Status**: Enabled when setup_fcnsns is true
**YAML**:
```yaml
workload:
  setup_fcnsns: true  # false = dimmed node
```
**Properties**:
- Platform Application (FCM/GCM)
- GCM Server Key in Parameter Store
- Push notification endpoints

### 19. Amazon SES
**Status**: Enabled when ses.enabled is true
**YAML**:
```yaml
ses:
  enabled: true  # false = dimmed node
  domain_name: "mail.example.com"
  test_emails:
    - "admin@example.com"
```
**Properties**:
- Domain identity
- DKIM records
- Verified emails

## Monitoring & Observability

### 20. Amazon CloudWatch
**Status**: Always enabled
**YAML**: Automatically configured
**Properties**:
- Log Groups per service
- 7-day retention
- Metrics and alarms

### 21. AWS X-Ray
**Status**: Enabled when xray_enabled is true
**YAML**:
```yaml
workload:
  xray_enabled: true  # false = dimmed node
```
**Properties**:
- Service map
- Trace sampling
- Sidecar container

### 22. AWS Secrets Manager
**Status**: Disabled (SSM Parameter Store used instead)
**YAML**: N/A
**Alternative**: SSM Parameter Store for secrets

### 23. Alarm Rules
**Status**: Disabled (not implemented)
**YAML**: N/A - future enhancement
**Properties**: CloudWatch alarms

## Additional Components (Not Shown)

### RDS PostgreSQL
**Status**: Enabled when postgres.enabled is true
**YAML**:
```yaml
postgres:
  enabled: true
  dbname: "myapp"
  username: "postgres"
  public_access: false
  engine_version: "16"

workload:
  install_pg_admin: true
  pg_admin_email: "admin@example.com"
```

### SQS
**Status**: Enabled when sqs.enabled is true
**YAML**:
```yaml
sqs:
  enabled: true
  name: "main-queue"
```

### EFS
**Status**: Enabled when efs array has entries
**YAML**:
```yaml
efs:
  - name: uploads
    path: /uploads

workload:
  efs:
    - name: uploads
      mount_point: /app/uploads
```

### ALB
**Status**: Enabled when alb.enabled is true
**YAML**:
```yaml
alb:
  enabled: true

workload:
  backend_alb_domain_name: "api.example.com"
```

## Service Discovery & Networking

### Service Mesh / Cloud Map
**Status**: Always enabled for internal communication
**Properties**:
- Private DNS namespace: `{project}_{env}.private`
- Service registry for all ECS services
- Internal service names: `{service}.{namespace}`

## Visual State Rules

1. **Always Enabled (Never Dimmed)**:
   - Client App (external)
   - API Gateway (default ingress)
   - ECS Cluster
   - Backend Service
   - ECR
   - S3 (backend bucket required)
   - EventBridge
   - CloudWatch
   - Service Discovery

2. **Conditionally Enabled/Dimmed**:
   - Admin App: `cognito.enable_dashboard_client`
   - GitHub Actions: `workload.enable_github_oidc`
   - Cognito: `cognito.enabled`
   - Route 53: `domain.enabled`
   - Additional Services: `services[]` array
   - Scheduled Tasks: `scheduled_tasks[]` array
   - Event Tasks: `event_processor_tasks[]` array
   - SNS: `workload.setup_fcnsns`
   - SES: `ses.enabled`
   - X-Ray: `workload.xray_enabled`
   - PostgreSQL: `postgres.enabled`
   - SQS: `sqs.enabled`
   - EFS: `efs[]` array
   - ALB: `alb.enabled`

3. **Always Disabled (Not Implemented)**:
   - Amplify
   - WAF
   - Aurora (PostgreSQL used instead)
   - Secrets Manager (Parameter Store used)
   - Alarm Rules

## Connection Types

- **Solid Lines**: Primary data flow
- **Dashed Lines**: Configuration or optional connections
- **Purple Connectors**: Service-to-service communication
- **API Calls**: Through API Gateway or internal Service Discovery