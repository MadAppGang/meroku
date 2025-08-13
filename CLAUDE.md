# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a comprehensive Terraform Infrastructure as Code (IaC) repository that provides a reusable, modular AWS infrastructure setup. It includes:
- Terraform modules for AWS services (20+ modules including ECS, RDS, ALB, Cognito, Lambda, etc.)
- Go CLI application ("meroku") for interactive infrastructure management using Bubble Tea TUI framework
- React+TypeScript web frontend for visual infrastructure management
- Handlebars-based templating system (using Raymond Go package) for environment configuration
- GitHub Actions CI/CD workflows with OIDC authentication

## Important Architecture Decisions

### VPC Endpoints (Deprecated)
**Note**: VPC endpoints are NO LONGER USED in this infrastructure due to cost considerations (~$27/month per interface endpoint). Instead, we rely on:
- Security groups for access control
- Internet Gateway for outbound connectivity
- Service-to-service communication through the VPC

All VPC endpoint code in `modules/workloads/ecs_endpoints.tf` is commented out and should remain so.

### API Gateway vs ALB
The infrastructure supports two ingress patterns:
- **Default (enable_alb: false)**: API Gateway → ECS Services
- **Alternative (enable_alb: true)**: ALB → ECS Services

Note: Currently, both resources are created regardless of the setting, but only one is used for traffic routing.

## Memories

- Always keep all AI-related documentation, created by AI or intended to be consumed by AI, in the @ai_docs/ folder

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

### Development Commands
```bash
# Run the TUI application
make tui

# Run the web frontend
make web

# Build the CLI
make build

# Run tests
make test

# Generate code from templates
make generate

# Test terraform plan diff viewer (for debugging)
./meroku --renderdiff terraform-plan.json
```

## Project Structure

```
infrastructure/
├── modules/          # Terraform modules for AWS services
├── env/             # Environment-specific Terraform configurations
├── project/         # YAML configuration files (dev.yaml, prod.yaml)
├── templates/       # Handlebars templates for Terraform generation
├── app/            # Go CLI application (meroku)
├── web/            # React+TypeScript frontend
└── scripts/        # Utility scripts
```

## Key Configuration Files

- `project/dev.yaml` - Development environment configuration
- `project/prod.yaml` - Production environment configuration
- `env/dev/*.tf` - Generated Terraform files for dev (DO NOT EDIT MANUALLY)
- `env/prod/*.tf` - Generated Terraform files for prod (DO NOT EDIT MANUALLY)

## Working with the Codebase

1. **Making Infrastructure Changes**: Edit YAML files in `project/`, then run `make infra-gen-{env}`
2. **Adding New Services**: Update the `services` array in YAML configuration
3. **Modifying Terraform Modules**: Edit files in `modules/` directory
4. **Updating Templates**: Modify Handlebars templates in `templates/`

## Testing Guidelines

- Always run `make infra-plan env={env}` before applying changes
- Test infrastructure changes in dev environment first
- Use `make test` to run Go tests
- Frontend tests: `cd web && npm test`

## Terraform Plan Viewer

The meroku CLI includes an advanced Terraform plan viewer with the following features:
- Visual tree view of resources organized by provider and service
- Detailed attribute diff display with proper formatting
- Support for resource replacements (shows both delete and create phases)
- Scrollable detail views for reviewing all changes
- Color-coded changes (green for create, yellow for update, red for delete)

To test the plan viewer with a JSON file:
```bash
./meroku --renderdiff path/to/terraform-plan.json
```

The viewer properly handles:
- Replace operations by showing both delete and create as separate items
- Complex nested attributes and arrays
- Long strings and multi-line values
- Null values and empty collections

## Security Considerations

- Never commit secrets or credentials
- Use AWS SSM Parameter Store for sensitive values
- Security groups control service access (no VPC endpoints needed)
- Enable encryption at rest for all data stores
- Use IAM roles for service authentication

## Cost Optimization

- VPC endpoints are disabled to save costs
- Use appropriate instance sizes for ECS tasks
- Enable auto-scaling where appropriate
- Monitor CloudWatch costs (logs retention is set to 30 days)
- when we bump version number,we need to create tag and change version.txt file content