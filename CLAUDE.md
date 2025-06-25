# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Terraform Infrastructure as Code (IaC) repository that provides a reusable, modular AWS infrastructure setup. It includes:
- Terraform modules for AWS services (ECS, RDS, ALB, Cognito, etc.)
- Go CLI application ("meroku") for interactive infrastructure management
- Gomplate-based templating system for environment configuration
- GitHub Actions CI/CD workflows

## Common Commands

### Infrastructure Management
```bash
# Initialize Terraform (run after creating env/*.tf files)
make infra-init env=dev

# Update Terraform modules
make infra-update env=dev

# Plan infrastructure changes
make infra-plan env=dev

# Apply infrastructure changes
make infra-apply env=dev

# Destroy infrastructure
make infra-destroy env=dev

# Show current infrastructure state
make infra-show env=dev

# Generate Terraform files from YAML config
make infra-gen-dev    # For dev environment
make infra-gen-prod   # For prod environment

# Import existing AWS resources
make infra-import env=dev
```

### Lambda Functions
```bash
# Build Lambda functions
make infra-build-lambdas
```

### Version Management
```bash
# Get current version
make infra-version

# Increment version (patch/minor/major)
make infra-increment-version level=patch
```

### CLI Application
```bash
# Run the Go CLI tool
cd app && go run .

# Build the CLI
cd app && go build -o meroku
```

## Architecture

### Directory Structure
- `/modules/` - Terraform modules for AWS services
  - `alb/` - Application Load Balancer
  - `cognito/` - Authentication
  - `postgres/` - RDS PostgreSQL
  - `ecs_service/` & `ecs_task/` - Container services
  - `workloads/` - Main infrastructure orchestration
  - `eventbridge/`, `appsync/`, `lambda/`, etc. - Other AWS services

- `/app/` - Go CLI application for infrastructure management
  - Uses Bubble Tea framework for TUI
  - Manages AWS profiles, environments, and deployments

- `/project/` - Template files for new projects
  - `dev.yaml` - Development environment configuration template
  - `Makefile` - Template makefile for projects

- `/env/` - Generated Terraform configurations (git-ignored)

### Configuration Flow
1. Users define environment in YAML files (dev.yaml, prod.yaml)
2. Gomplate processes templates to generate Terraform files in `/env/`
3. Terraform applies the infrastructure

### Key AWS Services Used
- **ECS with Fargate** - Container orchestration
- **ALB** - Load balancing
- **RDS PostgreSQL** - Database
- **Cognito** - Authentication
- **EventBridge** - Event-driven architecture
- **Service Connect** - Service discovery
- **ECR** - Container registry
- **Route53** - DNS management
- **S3** - Storage
- **SES** - Email service
- **Lambda** - Serverless functions
- **SQS** - Message queuing
- **AppSync** - GraphQL API
- **X-Ray** - Distributed tracing

## Development Workflow

1. **Setup Environment**:
   - Copy `project/dev.yaml` to your project root
   - Customize values (app name, domain, AWS settings)
   - Run `make infra-gen-dev` to generate Terraform files

2. **Deploy Infrastructure**:
   - `make infra-init env=dev` - Initialize Terraform
   - `make infra-plan env=dev` - Review changes
   - `make infra-apply env=dev` - Apply changes

3. **Deploy Application**:
   - Push container image to ECR
   - ECS automatically deploys new version

4. **CI/CD**:
   - GitHub Actions workflows handle automated deployments
   - Uses OIDC for AWS authentication
   - Automatically builds and pushes containers on merge

## Important Configuration Files

- **dev.yaml/prod.yaml** - Environment-specific configuration
- **modules/workloads/main.tmpl** - Main infrastructure template
- **receipts/github_workflow/** - CI/CD workflow examples
- **modules/*/variables.tmpl** - Module variable definitions

## Remote Debugging

The infrastructure supports remote debugging for ECS tasks:
- Configure `remote_debugging_enabled: true` in YAML
- Set `remote_debugging_port` (e.g., 9229 for Node.js)
- Access via bastion host on private subnet

## Security Considerations

- All sensitive data stored in AWS Secrets Manager
- IAM roles follow least privilege principle
- VPC with private/public subnet separation
- Cognito for user authentication
- Security groups restrict access appropriately

## Troubleshooting

- **Terraform state issues**: Check S3 backend configuration
- **ECS deployment failures**: Check CloudWatch logs, task definitions
- **Domain issues**: Verify Route53 and certificate validation
- **Build failures**: Check Lambda build output, Go version compatibility

## Version
Current version: v2.2.5 (see version.txt)