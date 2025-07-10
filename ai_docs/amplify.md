# AWS Amplify Module

## Overview
This module provisions AWS Amplify applications for hosting static web applications with continuous deployment from GitHub repositories. It supports multiple applications, custom domains, environment variables, and pull request previews.

## Features
- Multiple Amplify apps support in a single module
- GitHub integration for continuous deployment
- Custom domain configuration with subdomains
- Environment variables management
- Pull request preview environments
- Support for various frontend frameworks (React, Vue, Next.js, etc.)

## Usage

### Basic Configuration
```yaml
amplify_apps:
  - name: main-web
    github_repository: https://github.com/username/repo
    branches:
      - name: main
        stage: PRODUCTION
```

### Full Configuration with Custom Domain
```yaml
amplify_apps:
  - name: main-web
    github_repository: https://github.com/username/repo
    custom_domain: example.com
    enable_root_domain: true
    branches:
      - name: main
        stage: PRODUCTION
        custom_subdomains: [www, app]
        environment_variables:
          REACT_APP_API_URL: https://api.example.com
          REACT_APP_ENV: production
      - name: develop
        stage: DEVELOPMENT
        custom_subdomains: [dev]
        enable_pull_request_preview: true
        environment_variables:
          REACT_APP_API_URL: https://api-dev.example.com
          REACT_APP_ENV: development
```

### Multiple Apps Configuration
```yaml
amplify_apps:
  - name: main-web
    github_repository: https://github.com/username/main-repo
    custom_domain: example.com
    enable_root_domain: true
    branches:
      - name: main
        stage: PRODUCTION
        custom_subdomains: [www]
    
  - name: admin-dashboard
    github_repository: https://github.com/username/admin-repo
    custom_domain: admin.example.com
    enable_root_domain: true
    branches:
      - name: main
        stage: PRODUCTION
      - name: staging
        stage: BETA
        custom_subdomains: [beta]
```

## Parameters

### Required Parameters
- `name`: Unique name for the Amplify app
- `github_repository`: Full GitHub repository URL
- `branches`: Array of branch configurations

### Branch Parameters
Each branch in the `branches` array supports:
- `name` (required): Git branch name
- `stage` (optional): Deployment stage (PRODUCTION, DEVELOPMENT, BETA, EXPERIMENTAL)
- `enable_auto_build` (optional): Enable automatic builds (default: true)
- `enable_pull_request_preview` (optional): Enable PR previews (default: false)
- `environment_variables` (optional): Branch-specific environment variables
- `custom_subdomains` (optional): List of subdomains for this branch

### Optional App Parameters
- `custom_domain`: Custom domain for the application
- `enable_root_domain`: Enable root domain access (default: false)

## Environment Variables

### Using Environment Variables
```yaml
environment_variables:
  REACT_APP_API_URL: https://api.example.com
  REACT_APP_COGNITO_REGION: us-east-1
  REACT_APP_COGNITO_USER_POOL_ID: ${cognito_user_pool_id}
  REACT_APP_COGNITO_CLIENT_ID: ${cognito_web_client_id}
```

### Variable Interpolation
The module supports variable interpolation using `${}` syntax:
- `${cognito_user_pool_id}`: References Cognito module output
- `${cognito_web_client_id}`: References Cognito web client ID
- `${GITHUB_TOKEN}`: References environment variable

## Domain Configuration

### Branch-Specific Subdomains
```yaml
amplify_apps:
  - name: main-web
    custom_domain: example.com
    enable_root_domain: true
    branches:
      - name: main
        stage: PRODUCTION
        custom_subdomains: [www, app]
      - name: staging
        stage: BETA
        custom_subdomains: [staging, beta]
      - name: develop
        stage: DEVELOPMENT
        custom_subdomains: [dev, api-dev]
```
This creates:
- https://example.com → main branch (root domain)
- https://www.example.com → main branch
- https://app.example.com → main branch
- https://staging.example.com → staging branch
- https://beta.example.com → staging branch
- https://dev.example.com → develop branch
- https://api-dev.example.com → develop branch


## Build Configuration

The module uses a standard build spec that:
1. Changes to the app directory
2. Runs `npm ci` to install dependencies
3. Runs `npm run build` to build the application
4. Serves files from the build directory
5. Caches node_modules for faster builds

## Outputs

The module provides the following outputs:
- `amplify_apps`: Map of all app details including IDs, ARNs, and URLs
- `app_ids`: Map of app names to Amplify app IDs
- `app_arns`: Map of app names to Amplify app ARNs
- `default_domains`: Map of app names to default Amplify domains
- `app_urls`: Map of app names to primary access URLs

## Security Considerations

1. **GitHub Token**: Use environment variables or AWS SSM Parameter Store for GitHub tokens
2. **Environment Variables**: Avoid storing sensitive data directly in environment variables
3. **Branch Protection**: Configure branch protection rules in GitHub
4. **Access Control**: Amplify apps are publicly accessible by default

## Common Patterns

### React App with API Integration
```yaml
- name: frontend
  github_repository: https://github.com/org/frontend
  custom_domain: app.example.com
  enable_root_domain: true
  branches:
    - name: main
      stage: PRODUCTION
      environment_variables:
        REACT_APP_API_URL: https://api.example.com
        REACT_APP_WS_URL: wss://ws.example.com
```

### Vue Admin Dashboard
```yaml
- name: admin
  github_repository: https://github.com/org/admin
  custom_domain: admin.example.com
  enable_root_domain: true
  branches:
    - name: main
      stage: PRODUCTION
      environment_variables:
        VUE_APP_API_URL: https://api.example.com
    - name: staging
      stage: BETA
      custom_subdomains: [staging]
      environment_variables:
        VUE_APP_API_URL: https://api-staging.example.com
```

### Next.js Marketing Site
```yaml
- name: marketing
  github_repository: https://github.com/org/marketing
  custom_domain: example.com
  enable_root_domain: true
  branches:
    - name: main
      stage: PRODUCTION
      custom_subdomains: [www]
```

## Troubleshooting

### Build Failures
1. Check the Amplify console for build logs
2. Verify the app_directory and build_directory paths
3. Ensure package.json has the correct build script
4. Check environment variable values

### Domain Issues
1. Verify domain ownership in Route53
2. Wait for DNS propagation (up to 48 hours)
3. Check SSL certificate status in Amplify console

### GitHub Integration
1. Ensure GitHub token has repo access
2. Verify repository URL is correct
3. Check branch name exists in the repository