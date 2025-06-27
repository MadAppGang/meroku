# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a comprehensive Terraform Infrastructure as Code (IaC) repository that provides a reusable, modular AWS infrastructure setup. It includes:
- Terraform modules for AWS services (20+ modules including ECS, RDS, ALB, Cognito, Lambda, etc.)
- Go CLI application ("meroku") for interactive infrastructure management using Bubble Tea TUI framework
- React+TypeScript web frontend for visual infrastructure management
- Gomplate-based templating system for environment configuration
- GitHub Actions CI/CD workflows with OIDC authentication

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

# Build CI Lambda specifically
make buildlambda
```

### Version Management
```bash
# Get current version
make infra-version

# Increment version (patch/minor/major)
make infra-increment-version level=patch
```

### Go CLI Application
```bash
# Run the Go CLI tool
cd app && go run .

# Build the CLI
cd app && go build -o meroku

# Run Go tests (in modules with tests)
cd modules/workloads/ci_lambda && go test ./...
```

### Web Frontend Development
```bash
cd web

# Install dependencies (uses pnpm)
pnpm install

# Development server
pnpm dev

# Production build
pnpm build

# Run Storybook for component development
pnpm storybook

# Lint code (uses Biome)
pnpm lint

# Preview production build
pnpm preview
```

### Testing
```bash
# Go tests
cd modules/workloads/ci_lambda && go test ./...
cd modules/workloads/ci_lambda/utils && go test

# JavaScript tests
cd modules/appsync/auth_lambda && npm test
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
  - AWS SDK Go v2 for AWS operations
  - Raymond (Handlebars) for templating
  - Manages AWS profiles, environments, and deployments

- `/web/` - React web application
  - Vite-based build system
  - TypeScript for type safety
  - shadcn/ui component library
  - ReactFlow for infrastructure visualization
  - Tailwind CSS for styling
  - Storybook for component development

- `/project/` - Template files for new projects
  - `dev.yaml` - Development environment configuration template
  - `Makefile` - Template makefile for projects

- `/env/` - Generated Terraform configurations (git-ignored)

- `/receipts/` - Example configurations for various tech stacks
  - Docker files
  - GitHub Actions workflows
  - CI/CD configurations

### Configuration Flow
1. Users define environment in YAML files (dev.yaml, prod.yaml)
2. Gomplate processes templates to generate Terraform files in `/env/`
3. Terraform applies the infrastructure
4. CI/CD workflows deploy applications to the created infrastructure

### Key AWS Services Used
- **ECS with Fargate** - Container orchestration
- **ALB** - Load balancing with path-based routing
- **RDS PostgreSQL** - Database with read replicas support
- **Cognito** - Authentication with user pools and identity pools
- **EventBridge** - Event-driven architecture
- **Service Connect** - Service discovery and mesh
- **ECR** - Container registry
- **Route53** - DNS management
- **S3** - Storage for static assets and Terraform state
- **SES** - Email service
- **Lambda** - Serverless functions
- **SQS** - Message queuing
- **AppSync** - GraphQL API with resolvers
- **X-Ray** - Distributed tracing
- **CloudWatch** - Logging and monitoring
- **Secrets Manager** - Secure credential storage
- **Parameter Store** - Configuration management
- **VPC** - Network isolation with public/private subnets
- **CloudFront** - CDN for static assets

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
   - ECS automatically deploys new version via EventBridge watcher

4. **CI/CD**:
   - GitHub Actions workflows handle automated deployments
   - Uses OIDC for AWS authentication (passwordless)
   - Automatically builds and pushes containers on merge
   - Production deployments require explicit EventBridge command

## Important Configuration Files

- **dev.yaml/prod.yaml** - Environment-specific configuration
- **modules/workloads/main.tmpl** - Main infrastructure template
- **receipts/github_workflow/** - CI/CD workflow examples
- **modules/*/variables.tmpl** - Module variable definitions
- **YAML_SPECIFICATION.md** - Complete YAML configuration reference
- **web/biome.json** - Frontend linting configuration
- **app/go.mod** - Go dependencies

## Remote Debugging

The infrastructure supports remote debugging for ECS tasks:
- Configure `remote_debugging_enabled: true` in YAML
- Set `remote_debugging_port` (e.g., 9229 for Node.js)
- Access via bastion host on private subnet

## Code Style and Conventions

### Frontend (TypeScript/React)
- Tab indentation, double quotes (enforced by Biome)
- Component files use PascalCase
- Stories co-located with components
- shadcn/ui components in `/web/src/components/ui/`
- Custom components in `/web/src/components/`

### Go
- Standard Go formatting (gofmt)
- Error handling follows Go idioms
- AWS SDK v2 patterns

### Terraform
- Module-based architecture
- Variables defined in `variables.tf`
- Outputs in `outputs.tf`
- Main logic in `main.tf`

## Security Considerations

- All sensitive data stored in AWS Secrets Manager
- IAM roles follow least privilege principle
- VPC with private/public subnet separation
- Cognito for user authentication
- Security groups restrict access appropriately
- OIDC for GitHub Actions (no stored credentials)
- TLS/SSL enforced for all public endpoints

## Troubleshooting

- **Terraform state issues**: Check S3 backend configuration
- **ECS deployment failures**: Check CloudWatch logs, task definitions
- **Domain issues**: Verify Route53 and certificate validation
- **Build failures**: Check Lambda build output, Go version compatibility
- **Frontend issues**: Check browser console, Vite build output
- **Linting errors**: Run `pnpm lint` in web directory

## Version
Current version: v2.2.5 (see version.txt)