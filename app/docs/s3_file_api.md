# S3 File Management API

## Overview

The S3 File Management API provides endpoints to create, read, update, and delete files in S3 buckets associated with your infrastructure project. This is particularly useful for managing environment files that are loaded by ECS services.

## Bucket Naming Convention

All bucket names are automatically prefixed with the pattern: `${project}-${bucket}-${env}`

For example:
- If your project is "instagram", environment is "dev", and you specify bucket "config"
- The actual S3 bucket name will be: `instagram-config-dev`

## API Endpoints

### 1. List Project Buckets

Lists all S3 buckets associated with the current project and environment.

**Endpoint:** `GET /api/s3/buckets?env=<env>`

**Parameters:**
- `env` (required): Environment name (e.g., "dev", "prod")

**Response:**
```json
[
  {
    "name": "config",
    "fullName": "instagram-config-dev"
  },
  {
    "name": "assets",
    "fullName": "instagram-assets-dev"
  }
]
```

### 2. Get File Content

Retrieves the content of a file from an S3 bucket.

**Endpoint:** `GET /api/s3/file?env=<env>&bucket=<bucket>&key=<key>`

**Parameters:**
- `env` (required): Environment name
- `bucket` (required): Bucket name (without prefix)
- `key` (required): File path/key in the bucket

**Response:**
```json
{
  "bucket": "config",
  "key": "backend/.env",
  "content": "DATABASE_URL=postgresql://localhost:5432/db\nAPI_KEY=secret"
}
```

### 3. Create/Update File

Creates a new file or updates an existing file in an S3 bucket.

**Endpoint:** `PUT /api/s3/file?env=<env>`

**Request Body:**
```json
{
  "bucket": "config",
  "key": "backend/.env",
  "content": "DATABASE_URL=postgresql://localhost:5432/db\nAPI_KEY=secret"
}
```

**Response:**
```json
{
  "message": "file uploaded successfully",
  "bucket": "config",
  "key": "backend/.env"
}
```

### 4. Delete File

Deletes a file from an S3 bucket.

**Endpoint:** `DELETE /api/s3/file?env=<env>&bucket=<bucket>&key=<key>`

**Parameters:**
- `env` (required): Environment name
- `bucket` (required): Bucket name (without prefix)
- `key` (required): File path/key in the bucket

**Response:**
```json
{
  "message": "file deleted successfully",
  "bucket": "config",
  "key": "backend/.env"
}
```

### 5. List Files

Lists files and folders in an S3 bucket with optional prefix filtering.

**Endpoint:** `GET /api/s3/files?env=<env>&bucket=<bucket>&prefix=<prefix>`

**Parameters:**
- `env` (required): Environment name
- `bucket` (required): Bucket name (without prefix)
- `prefix` (optional): Prefix to filter files (e.g., "backend/")

**Response:**
```json
{
  "files": [
    {
      "bucket": "config",
      "key": "backend/.env",
      "size": 256,
      "lastModified": "2024-01-15T10:30:00Z",
      "etag": "\"d41d8cd98f00b204e9800998ecf8427e\""
    }
  ],
  "folders": [
    "backend/",
    "worker/"
  ]
}
```

## Usage Examples

### Managing Environment Files for ECS Services

1. **Create an environment file for the backend service:**
```bash
curl -X PUT "http://localhost:8080/api/s3/file?env=dev" \
  -H "Content-Type: application/json" \
  -d '{
    "bucket": "config",
    "key": "backend/.env",
    "content": "DATABASE_URL=postgresql://user:pass@db:5432/myapp\nREDIS_URL=redis://redis:6379"
  }'
```

2. **Update environment variables without redeploying:**
```bash
# Update the file
curl -X PUT "http://localhost:8080/api/s3/file?env=dev" \
  -H "Content-Type: application/json" \
  -d '{
    "bucket": "config",
    "key": "backend/.env",
    "content": "DATABASE_URL=postgresql://user:pass@db:5432/myapp\nREDIS_URL=redis://redis:6379\nNEW_VAR=new_value"
  }'

# Force ECS to reload the service
aws ecs update-service --cluster myapp_cluster_dev --service backend --force-new-deployment
```

3. **Check current environment configuration:**
```bash
curl -X GET "http://localhost:8080/api/s3/file?env=dev&bucket=config&key=backend/.env"
```

### Managing Multiple Service Configurations

1. **List all configuration files:**
```bash
curl -X GET "http://localhost:8080/api/s3/files?env=dev&bucket=config"
```

2. **Create service-specific configurations:**
```bash
# Worker service config
curl -X PUT "http://localhost:8080/api/s3/file?env=dev" \
  -H "Content-Type: application/json" \
  -d '{
    "bucket": "config",
    "key": "worker/.env",
    "content": "WORKER_THREADS=4\nQUEUE_NAME=default"
  }'

# API service config
curl -X PUT "http://localhost:8080/api/s3/file?env=dev" \
  -H "Content-Type: application/json" \
  -d '{
    "bucket": "config",
    "key": "api/.env",
    "content": "PORT=3000\nRATE_LIMIT=100"
  }'
```

## Integration with ECS Services

To use these S3 files with your ECS services, configure your YAML as follows:

```yaml
workload:
  env_files_s3:
    - bucket: config
      key: backend/.env

services:
  - name: worker
    env_files_s3:
      - bucket: config
        key: worker/.env
```

## Security Considerations

1. **Access Control**: The API uses the AWS profile selected when starting the meroku app
2. **Bucket Permissions**: Ensure the IAM role has appropriate S3 permissions
3. **Sensitive Data**: For highly sensitive values, consider using SSM Parameter Store instead
4. **Audit Trail**: S3 access is logged via CloudTrail for security auditing

## Error Handling

Common error responses:

- `404 Not Found`: Bucket or file doesn't exist
- `400 Bad Request`: Missing required parameters
- `500 Internal Server Error`: AWS API errors or connectivity issues

## Best Practices

1. **File Organization**: Use folders to organize configurations (e.g., `backend/`, `worker/`)
2. **Version Control**: Consider backing up configuration files before updates
3. **Environment Separation**: Keep separate buckets for different environments
4. **File Format**: Use standard `.env` format for environment files
5. **Comments**: Include comments in env files to document variables