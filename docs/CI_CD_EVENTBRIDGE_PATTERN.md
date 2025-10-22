# CI/CD with EventBridge Pattern

## Overview

This document describes the recommended pattern for deploying applications using AWS EventBridge instead of direct ECR push. This pattern provides better decoupling, observability, and flexibility for cross-account deployments.

## Why EventBridge Over Direct ECR Push?

### Advantages

1. **Decoupling**: Build and deployment systems are separated
2. **Event-Driven**: Deployments are triggered by events, not direct pushes
3. **Observability**: All deployment events are logged and traceable
4. **Cross-Account Support**: Events can be routed across AWS accounts
5. **Flexibility**: Easy to add additional deployment logic or notifications
6. **Auditability**: Complete audit trail of all deployment events

### Traditional vs EventBridge Pattern

**Traditional (Direct ECR Push)**:
```
GitHub Actions → ECR Push → Manual ECS Update
```

**EventBridge Pattern**:
```
GitHub Actions → ECR Push → EventBridge Event → Lambda/ECS Update
```

## Implementation

### 1. GitHub Actions Workflow

Create `.github/workflows/deploy.yml`:

```yaml
name: Deploy to ECS via EventBridge

on:
  push:
    branches: [main, develop]

permissions:
  id-token: write  # Required for OIDC
  contents: read

jobs:
  build-and-notify:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v3

      - name: Configure AWS credentials (OIDC)
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::${{ secrets.AWS_ACCOUNT_ID }}:role/github-oidc-role
          aws-region: us-east-1

      - name: Login to Amazon ECR
        id: login-ecr
        uses: aws-actions/amazon-ecr-login@v2

      - name: Build and push Docker image
        env:
          ECR_REGISTRY: ${{ steps.login-ecr.outputs.registry }}
          ECR_REPOSITORY: myproject_backend
          IMAGE_TAG: ${{ github.sha }}
        run: |
          docker build -t $ECR_REGISTRY/$ECR_REPOSITORY:$IMAGE_TAG .
          docker push $ECR_REGISTRY/$ECR_REPOSITORY:$IMAGE_TAG
          docker tag $ECR_REGISTRY/$ECR_REPOSITORY:$IMAGE_TAG $ECR_REGISTRY/$ECR_REPOSITORY:latest
          docker push $ECR_REGISTRY/$ECR_REPOSITORY:latest

      - name: Send EventBridge deployment event
        run: |
          aws events put-events --entries '[
            {
              "Source": "github.actions",
              "DetailType": "Deployment Ready",
              "Detail": "{\"repository\": \"'$ECR_REPOSITORY'\", \"tag\": \"'$IMAGE_TAG'\", \"branch\": \"'${GITHUB_REF#refs/heads/}'\", \"commit\": \"'$GITHUB_SHA'\"}",
              "EventBusName": "default"
            }
          ]'
```

### 2. Terraform EventBridge Rule

Add to your `modules/workloads/eventbridge.tf`:

```hcl
# EventBridge rule to trigger ECS deployment
resource "aws_cloudwatch_event_rule" "deployment_ready" {
  name        = "${var.project}-deployment-ready-${var.env}"
  description = "Triggers ECS deployment when new image is pushed"

  event_pattern = jsonencode({
    source      = ["github.actions"]
    detail-type = ["Deployment Ready"]
    detail = {
      repository = ["${var.project}_backend"]
    }
  })

  tags = {
    Name        = "${var.project}-deployment-ready-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
  }
}

# Lambda function to update ECS service
resource "aws_lambda_function" "ecs_deployer" {
  filename      = "lambda_deployment_trigger.zip"
  function_name = "${var.project}-ecs-deployer-${var.env}"
  role          = aws_iam_role.ecs_deployer.arn
  handler       = "index.handler"
  runtime       = "python3.11"

  environment {
    variables = {
      CLUSTER_NAME = aws_ecs_cluster.main.name
      SERVICE_NAME = aws_ecs_service.backend.name
    }
  }
}

# EventBridge target to invoke Lambda
resource "aws_cloudwatch_event_target" "ecs_deployer" {
  rule      = aws_cloudwatch_event_rule.deployment_ready.name
  target_id = "ECSDeployer"
  arn       = aws_lambda_function.ecs_deployer.arn
}

# Allow EventBridge to invoke Lambda
resource "aws_lambda_permission" "allow_eventbridge" {
  statement_id  = "AllowExecutionFromEventBridge"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.ecs_deployer.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.deployment_ready.arn
}

# IAM role for Lambda deployer
resource "aws_iam_role" "ecs_deployer" {
  name = "${var.project}-ecs-deployer-${var.env}"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action = "sts:AssumeRole"
      Effect = "Allow"
      Principal = {
        Service = "lambda.amazonaws.com"
      }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "ecs_deployer_basic" {
  role       = aws_iam_role.ecs_deployer.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy" "ecs_deployer_update" {
  name = "ecs-update-service"
  role = aws_iam_role.ecs_deployer.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "ecs:UpdateService",
          "ecs:DescribeServices"
        ]
        Resource = aws_ecs_service.backend.id
      }
    ]
  })
}
```

### 3. Lambda Deployer Function

Create `lambda_deployment_trigger/index.py`:

```python
import json
import os
import boto3

ecs = boto3.client('ecs')

def handler(event, context):
    """
    Triggered by EventBridge when new deployment is ready.
    Forces ECS service to redeploy with latest image.
    """
    cluster_name = os.environ['CLUSTER_NAME']
    service_name = os.environ['SERVICE_NAME']

    detail = event.get('detail', {})
    repository = detail.get('repository')
    tag = detail.get('tag')
    branch = detail.get('branch')

    print(f"Deployment event received: {repository}:{tag} from branch {branch}")

    try:
        # Force new deployment
        response = ecs.update_service(
            cluster=cluster_name,
            service=service_name,
            forceNewDeployment=True
        )

        print(f"ECS service {service_name} deployment initiated")

        return {
            'statusCode': 200,
            'body': json.dumps({
                'message': 'Deployment initiated',
                'cluster': cluster_name,
                'service': service_name,
                'repository': repository,
                'tag': tag
            })
        }
    except Exception as e:
        print(f"Error updating ECS service: {str(e)}")
        raise
```

## Cross-Account Event Routing (Optional)

For cross-account deployments, you can route events from a source account to target accounts.

### Source Account (e.g., dev with ECR)

```hcl
# Event bus in source account
resource "aws_cloudwatch_event_bus" "deployment_events" {
  name = "deployment-events"
}

# Allow target accounts to receive events
resource "aws_cloudwatch_event_bus_policy" "allow_target_accounts" {
  event_bus_name = aws_cloudwatch_event_bus.deployment_events.name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid    = "AllowTargetAccounts"
      Effect = "Allow"
      Principal = {
        AWS = [
          "arn:aws:iam::${var.staging_account_id}:root",
          "arn:aws:iam::${var.prod_account_id}:root"
        ]
      }
      Action   = "events:PutEvents"
      Resource = aws_cloudwatch_event_bus.deployment_events.arn
    }]
  })
}
```

### Target Account (e.g., staging or prod)

```hcl
# Rule to forward events to target account
resource "aws_cloudwatch_event_rule" "forward_deployment_events" {
  provider = aws.source  # Assume role in source account

  name           = "forward-to-${var.target_env}"
  event_bus_name = "deployment-events"

  event_pattern = jsonencode({
    source      = ["github.actions"]
    detail-type = ["Deployment Ready"]
  })
}

resource "aws_cloudwatch_event_target" "target_account_bus" {
  provider = aws.source

  rule           = aws_cloudwatch_event_rule.forward_deployment_events.name
  event_bus_name = aws_cloudwatch_event_rule.forward_deployment_events.event_bus_name
  arn            = "arn:aws:events:${var.region}:${var.target_account_id}:event-bus/default"
  role_arn       = aws_iam_role.eventbridge_forward.arn
}
```

## Monitoring and Debugging

### View EventBridge Events

```bash
# List recent events
aws events list-rules --region us-east-1

# Describe a specific rule
aws events describe-rule --name myproject-deployment-ready-dev

# View CloudWatch Logs for Lambda
aws logs tail /aws/lambda/myproject-ecs-deployer-dev --follow
```

### Test EventBridge Rule Manually

```bash
aws events put-events --entries '[
  {
    "Source": "github.actions",
    "DetailType": "Deployment Ready",
    "Detail": "{\"repository\": \"myproject_backend\", \"tag\": \"test-123\", \"branch\": \"main\", \"commit\": \"abc123\"}",
    "EventBusName": "default"
  }
]'
```

### CloudWatch Alarms

Set up alarms for failed deployments:

```hcl
resource "aws_cloudwatch_metric_alarm" "deployment_failures" {
  alarm_name          = "${var.project}-deployment-failures-${var.env}"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 1
  metric_name         = "Errors"
  namespace           = "AWS/Lambda"
  period              = 300
  statistic           = "Sum"
  threshold           = 0
  alarm_description   = "Alert when ECS deployment Lambda fails"

  dimensions = {
    FunctionName = aws_lambda_function.ecs_deployer.function_name
  }
}
```

## Best Practices

1. **Use Image Tags**: Always tag images with commit SHA for traceability
2. **Event Validation**: Validate event payload in Lambda before processing
3. **Idempotency**: Ensure deployment logic is idempotent
4. **Logging**: Log all deployment events for audit trail
5. **Error Handling**: Implement retry logic and dead-letter queues
6. **Security**: Use IAM roles with least-privilege permissions
7. **Testing**: Test EventBridge rules in non-production first

## Troubleshooting

### Events Not Triggering

1. Check EventBridge rule pattern matches your event
2. Verify Lambda function has correct permissions
3. Check CloudWatch Logs for Lambda errors
4. Ensure EventBridge rule is enabled

### Cross-Account Events Not Working

1. Verify IAM permissions on both accounts
2. Check event bus policies allow cross-account access
3. Ensure event format matches exactly
4. Test with manual put-events command

## Conclusion

The EventBridge pattern provides a robust, scalable, and observable CI/CD pipeline for AWS ECS deployments. It's particularly useful for:

- Multi-account architectures
- Complex deployment workflows
- Audit and compliance requirements
- Integration with existing event-driven systems

For simpler use cases, direct ECR push with ECS task definition updates may be sufficient, but EventBridge offers significant advantages as your infrastructure grows.
