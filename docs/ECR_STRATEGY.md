# ECR Strategy Guide

This guide explains how to configure Amazon Elastic Container Registry (ECR) repositories for your infrastructure across multiple environments.

## Overview

The infrastructure supports two ECR strategies:

1. **Local Strategy** (`ecr_strategy: "local"`) - Create ECR repositories in the current environment
2. **Cross-Account Strategy** (`ecr_strategy: "cross_account"`) - Pull images from another environment's ECR

## Configuration

### Local ECR Strategy

Create ECR repositories in the current AWS account/environment.

```yaml
env: dev
ecr_strategy: "local"
```

**Use Cases:**
- Development environment (build and push images here)
- Isolated production environment (separate from dev)
- Any environment that needs its own container registry

**What Gets Created:**
- ECR repository for backend: `{project}_backend`
- ECR repositories for services: `{project}_service_{name}`
- ECR repositories for tasks: `{project}_task_{name}`

**Advantages:**
- Full control over repository lifecycle
- No cross-account dependencies
- Simpler IAM permissions

**Disadvantages:**
- Need to build/push images to each environment separately
- Or manually replicate images between registries

### Cross-Account ECR Strategy

Pull container images from another environment's ECR.

```yaml
env: prod
ecr_strategy: "cross_account"
ecr_account_id: "123456789012"  # Dev account ID
ecr_account_region: "us-east-1"  # Dev region
```

**Use Cases:**
- Production pulling from dev ECR (build once, deploy everywhere)
- Staging pulling from dev ECR (test same image as dev)
- Any non-dev environment using dev's images

**Advantages:**
- Build once in dev, deploy everywhere
- Same image tested in dev is deployed to prod
- Simplified CI/CD pipeline

**Disadvantages:**
- Production depends on dev account
- Need cross-account IAM permissions
- Dev account owns the "source of truth"

## Common Setup Patterns

### Pattern 1: Centralized Dev ECR (Recommended for Most Teams)

**Best for:** Small to medium teams, simple deployment workflows

```yaml
# dev.yaml
env: dev
ecr_strategy: "local"  # Dev creates ECR

# staging.yaml
env: staging
ecr_strategy: "cross_account"
ecr_account_id: "111111111111"  # Dev account
ecr_account_region: "us-east-1"

# prod.yaml
env: prod
ecr_strategy: "cross_account"
ecr_account_id: "111111111111"  # Dev account
ecr_account_region: "us-east-1"
```

**Deployment Flow:**
```
GitHub Actions → Build → Push to Dev ECR
                                ↓
                    Dev/Staging/Prod all pull from Dev ECR
```

### Pattern 2: Isolated Production ECR

**Best for:** Teams with strict security requirements, regulated industries

```yaml
# dev.yaml
env: dev
ecr_strategy: "local"  # Dev creates ECR

# staging.yaml
env: staging
ecr_strategy: "cross_account"
ecr_account_id: "111111111111"  # Dev account
ecr_account_region: "us-east-1"

# prod.yaml
env: prod
ecr_strategy: "local"  # Prod creates its own ECR
```

**Deployment Flow:**
```
Dev/Staging:
  GitHub Actions → Build → Push to Dev ECR → Deploy

Production:
  GitHub Actions → Build → Push to Prod ECR → Deploy
```

### Pattern 3: Per-Environment ECR (Maximum Isolation)

**Best for:** Large teams, multiple regions, strict compliance

```yaml
# dev.yaml
env: dev
ecr_strategy: "local"

# staging.yaml
env: staging
ecr_strategy: "local"

# prod.yaml
env: prod
ecr_strategy: "local"
```

**Deployment Flow:**
```
Each environment:
  GitHub Actions → Build → Push to {env} ECR → Deploy
```

## Migration Guide

### Migrating from v6 to v7

The migration system automatically sets `ecr_strategy` based on your existing configuration:

**Automatic Migration Rules:**
1. If `env == "dev"` → Sets `ecr_strategy: "local"`
2. If `ecr_account_id` is set → Sets `ecr_strategy: "cross_account"`
3. Otherwise → Sets `ecr_strategy: "local"`

**Run Migration:**
```bash
./meroku migrate dev.yaml
# or
./meroku migrate all
```

### Manual Configuration

Edit your YAML files to explicitly set the strategy:

```yaml
# Option 1: Local ECR
ecr_strategy: "local"

# Option 2: Cross-Account ECR
ecr_strategy: "cross_account"
ecr_account_id: "123456789012"
ecr_account_region: "us-east-1"
```

## Cross-Account Permissions

When using `ecr_strategy: "cross_account"`, the source ECR must allow cross-organization access.

### Automatic Setup (Already Configured)

The infrastructure automatically creates an ECR policy that allows cross-organization access:

```hcl
# modules/workloads/ecr.tf
data "aws_iam_policy_document" "default_ecr_policy" {
  statement {
    sid = "External read ECR policy"
    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:BatchGetImage",
      "ecr:DescribeImages",
      "ecr:DescribeRepositories",
      "ecr:GetDownloadUrlForLayer"
    ]
    condition {
      test     = "StringEquals"
      variable = "aws:PrincipalOrgID"
      values   = [data.aws_organizations_organization.org.id]
    }
  }
}
```

This allows any AWS account **within the same AWS Organization** to pull images.

### Manual Setup (If Not Using AWS Organizations)

If your accounts are NOT in the same AWS Organization, add a manual ECR policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": [
          "arn:aws:iam::222222222222:root",  // Staging account
          "arn:aws:iam::333333333333:root"   // Prod account
        ]
      },
      "Action": [
        "ecr:GetDownloadUrlForLayer",
        "ecr:BatchGetImage",
        "ecr:BatchCheckLayerAvailability"
      ]
    }
  ]
}
```

## Image Tagging Strategy

### Recommended Tags

Tag images with multiple tags for flexibility:

```bash
# Latest tag (always points to newest)
docker tag myapp:latest 123456789012.dkr.ecr.us-east-1.amazonaws.com/myproject_backend:latest

# Git commit SHA (specific version)
docker tag myapp:latest 123456789012.dkr.ecr.us-east-1.amazonaws.com/myproject_backend:git-abc123f

# Environment promotion tag
docker tag myapp:latest 123456789012.dkr.ecr.us-east-1.amazonaws.com/myproject_backend:env-prod-20250122

# Semantic version
docker tag myapp:latest 123456789012.dkr.ecr.us-east-1.amazonaws.com/myproject_backend:v1.2.3
```

### Tag Conventions

- `latest` - Latest development build (auto-deployed to dev)
- `git-{sha}` - Specific commit (e.g., `git-abc123f`)
- `env-{env}-{timestamp}` - Environment promotions (e.g., `env-prod-20250122`)
- `v{version}` - Semantic versions (e.g., `v1.2.3`)
- `stable` - Production-ready tag

## CI/CD Integration

### GitHub Actions Example (Centralized Dev ECR)

```yaml
name: Deploy

on:
  push:
    branches: [main]

jobs:
  build-and-deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      # Build and push to Dev ECR
      - name: Build and Push to Dev ECR
        env:
          AWS_REGION: us-east-1
        run: |
          # Authenticate to Dev ECR
          aws ecr get-login-password --region $AWS_REGION | \
            docker login --username AWS --password-stdin 111111111111.dkr.ecr.$AWS_REGION.amazonaws.com

          # Build
          docker build -t myproject_backend .

          # Tag
          COMMIT_SHA=$(git rev-parse --short HEAD)
          docker tag myproject_backend:latest 111111111111.dkr.ecr.$AWS_REGION.amazonaws.com/myproject_backend:latest
          docker tag myproject_backend:latest 111111111111.dkr.ecr.$AWS_REGION.amazonaws.com/myproject_backend:git-$COMMIT_SHA

          # Push
          docker push 111111111111.dkr.ecr.$AWS_REGION.amazonaws.com/myproject_backend:latest
          docker push 111111111111.dkr.ecr.$AWS_REGION.amazonaws.com/myproject_backend:git-$COMMIT_SHA

      # Deploy to Dev (automatic)
      - name: Deploy to Dev
        run: |
          # Deployment happens automatically via CI/CD Lambda

      # Deploy to Prod (manual via EventBridge)
      - name: Trigger Prod Deployment
        if: github.event.inputs.deploy_to_prod == 'true'
        run: |
          aws events put-events --entries \
            'Source=action.production,DetailType=DEPLOY,Detail="{\"service\":\"backend\",\"tag\":\"git-'$COMMIT_SHA'\"}"'
```

## Deployment Workflows

### Automatic Dev/Staging Deployments

```
GitHub Push → Build → Push to Dev ECR → EventBridge → Lambda → ECS Deploy (Dev)
                                                                     ↓
                                                            ECS Deploy (Staging)
```

### Manual Production Deployments

```
User Action → GitHub Actions → EventBridge Message → CI/CD Lambda → ECS Deploy (Prod)
```

**Trigger Manual Prod Deploy:**
```bash
aws events put-events --entries \
  'Source=action.production,DetailType=DEPLOY,Detail="{\"service\":\"backend\",\"tag\":\"git-abc123f\"}"'
```

## Troubleshooting

### Issue: "No ECR repository found"

**Cause:** Environment has `ecr_strategy: "cross_account"` but `ecr_account_id` is not set.

**Solution:**
```yaml
ecr_strategy: "cross_account"
ecr_account_id: "123456789012"
ecr_account_region: "us-east-1"
```

### Issue: "Access Denied" when pulling images

**Cause:** Cross-account ECR policy not configured.

**Solution:** Ensure accounts are in the same AWS Organization, or add manual ECR policy (see "Cross-Account Permissions" above).

### Issue: "Image not found" in cross-account setup

**Cause:** Image hasn't been pushed to dev ECR, or wrong tag.

**Solution:**
1. Verify image exists in dev ECR: `aws ecr describe-images --repository-name myproject_backend`
2. Check ECS task definition is using correct image URL
3. Verify image tag matches what was pushed

### Issue: Migration not detecting strategy correctly

**Cause:** Edge case in automatic migration logic.

**Solution:** Manually set `ecr_strategy` in YAML file:
```yaml
ecr_strategy: "local"  # or "cross_account"
```

## Best Practices

1. **Use Centralized Dev ECR for Most Cases** - Simpler CI/CD, build once deploy everywhere
2. **Tag Images with Git SHA** - Always know which code version is deployed
3. **Use AWS Organizations** - Automatic cross-account access without manual policies
4. **Implement Image Scanning** - Enable ECR vulnerability scanning in dev
5. **Lifecycle Policies** - Clean up old images to reduce costs
6. **Manual Prod Deploys** - Use EventBridge for explicit production deployments
7. **Test in Staging First** - Deploy to staging before production

## Cost Optimization

- **Storage:** First 50 GB free per month, then $0.10/GB
- **Data Transfer:** Free within same region, charged for cross-region
- **Lifecycle Policies:** Automatically delete untagged/old images

**Example Lifecycle Policy:**
```json
{
  "rules": [
    {
      "rulePriority": 1,
      "description": "Keep last 10 images",
      "selection": {
        "tagStatus": "any",
        "countType": "imageCountMoreThan",
        "countNumber": 10
      },
      "action": {
        "type": "expire"
      }
    }
  ]
}
```

## Security Considerations

1. **Least Privilege:** Only grant pull access to environments that need it
2. **Image Scanning:** Enable vulnerability scanning on push
3. **Encryption:** Enable encryption at rest (automatic in ECR)
4. **Access Logs:** Enable CloudTrail for ECR API calls
5. **Private Endpoints:** Use VPC endpoints for ECR in production

## Next Steps

1. Choose your ECR strategy based on team size and security requirements
2. Update YAML configuration files
3. Run migration: `./meroku migrate all`
4. Apply infrastructure changes: `make infra-apply env=dev`
5. Configure CI/CD pipeline to push images
6. Test deployments across all environments

## References

- [AWS ECR Documentation](https://docs.aws.amazon.com/ecr/)
- [Cross-Account ECR Access](https://docs.aws.amazon.com/AmazonECR/latest/userguide/repository-policies.html)
- [GitHub Actions OIDC](https://docs.github.com/en/actions/deployment/security-hardening-your-deployments/configuring-openid-connect-in-amazon-web-services)
