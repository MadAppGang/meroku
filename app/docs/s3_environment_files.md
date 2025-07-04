# S3 Environment Files for ECS Services

## Overview

The infrastructure supports loading environment variables from S3 files for ECS services. This feature allows you to manage environment configurations centrally and securely without rebuilding Docker images or redeploying services.

## Configuration

### YAML Configuration

Add the `env_files_s3` configuration under the `workload` section in your environment YAML file (e.g., `dev.yaml`, `prod.yaml`):

```yaml
workload:
  env_files_s3:
    - bucket: config         # S3 bucket name (without project/env prefix)
      key: backend/.env      # S3 object key/path
    - bucket: secrets        # You can load from multiple files
      key: api/secrets.env
```

### Important Notes

1. **Bucket Naming**: The actual S3 bucket name will be automatically prefixed with `${project}-${bucket}-${env}`. For example:
   - YAML config: `bucket: config`
   - Actual bucket: `instagram-config-dev` (for project=instagram, env=dev)

2. **File Format**: S3 files should contain environment variables in standard `.env` format:
   ```
   DATABASE_URL=postgresql://user:pass@host:5432/db
   API_KEY=your-secret-key
   DEBUG=true
   ```

3. **Multiple Files**: You can load from multiple S3 files. Variables are loaded in order, with later files overriding earlier ones.

## How It Works

### 1. Infrastructure Setup

When you run `terraform apply`, the infrastructure:

1. Creates IAM policies granting the ECS task execution role access to the specified S3 files
2. Automatically creates empty S3 files if they don't exist
3. Configures the ECS task definition with `environmentFiles` parameter

### 2. ECS Task Execution

When ECS starts a container:

1. The ECS agent downloads the specified S3 files
2. Parses the environment variables from the files
3. Injects them into the container environment
4. Starts the container with all variables available

### 3. Variable Priority

Environment variables are loaded in this order (later overrides earlier):
1. Variables defined in the Docker image
2. Variables from `backend_env_variables` in YAML config
3. Variables from S3 environment files (in the order specified)
4. Variables from SSM Parameter Store (if configured)

## Example Usage

### 1. Basic Configuration

```yaml
# dev.yaml
project: myapp
env: dev

workload:
  env_files_s3:
    - bucket: config
      key: backend/common.env
    - bucket: config
      key: backend/dev.env
```

### 2. Create S3 Files

After running `terraform apply`, upload your environment files to S3:

```bash
# Create a local .env file
cat > backend.env << EOF
DATABASE_URL=postgresql://user:pass@localhost:5432/myapp
REDIS_URL=redis://localhost:6379
API_KEY=development-key
EOF

# Upload to S3
aws s3 cp backend.env s3://myapp-config-dev/backend/dev.env
```

### 3. Service-Specific Environment Files

Services can also have their own environment files:

```yaml
services:
  - name: worker
    env_files_s3:
      - bucket: config
        key: worker/config.env
```

## Managing Environment Files

### Update Variables Without Redeployment

To update environment variables:

1. Update the S3 file:
   ```bash
   aws s3 cp updated.env s3://myapp-config-dev/backend/dev.env
   ```

2. Force a new deployment:
   ```bash
   aws ecs update-service --cluster myapp_cluster_dev --service backend --force-new-deployment
   ```

### View Current Environment File

```bash
aws s3 cp s3://myapp-config-dev/backend/dev.env -
```

### Security Best Practices

1. **Use SecureString SSM Parameters** for highly sensitive values instead of S3 files
2. **Restrict S3 bucket access** using bucket policies
3. **Enable S3 bucket encryption** at rest
4. **Use separate files** for different environments (dev, staging, prod)
5. **Audit access** using CloudTrail

## Troubleshooting

### Common Issues

1. **Container fails to start**: Check ECS task logs for S3 access errors
2. **Variables not loading**: Ensure the S3 file exists and has correct format
3. **Permission denied**: Verify the ECS task execution role has S3 access

### Debug Commands

```bash
# Check if file exists
aws s3 ls s3://myapp-config-dev/backend/dev.env

# View ECS task definition
aws ecs describe-task-definition --task-definition backend

# Check IAM role permissions
aws iam get-role-policy --role-name backend-task-execution --policy-name backend-s3-env
```

## Integration with SSM Parameters

You can use both S3 environment files and SSM parameters together. SSM parameters take precedence over S3 environment files:

```yaml
workload:
  # Load base configuration from S3
  env_files_s3:
    - bucket: config
      key: backend/base.env
  
  # Override specific values with SSM
  backend_env_variables:
    DATABASE_PASSWORD: /myapp/dev/db_password  # SSM parameter reference
```

This approach provides flexibility to:
- Store non-sensitive configuration in S3 files
- Store secrets in SSM Parameter Store
- Override any S3 value with SSM when needed