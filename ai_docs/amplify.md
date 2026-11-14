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

## Rewrites and the /api proxy

`amplify_apps[].custom_rules` adds Amplify rewrite and redirect rules. They are
applied in the order written, **before** the SPA rule (`spa_mode`), and Amplify
uses the first rule that matches.

```yaml
amplify_apps:
  - name: web
    environment_variables:
      AMPLIFY_MONOREPO_APP_ROOT: apps/web
    custom_rules:
      - { source: "/api/<*>", workload: backend }              # → <backend base>/api/<*> (no public service named api)
      - { source: "/reports/<*>", workload: reports }          # → <base>/reports/<*>
      - { source: "/r/<*>", workload: reports, path: "/<*>" }  # → <base>/reports/<*>
      - { source: "/old-pricing", target: "/pricing", status: "301" }
```

A rule sets exactly one of:

- `target`: a fixed path or URL. `status` is `200` (rewrite, the default), `301`,
  `302` or `404-200` (serve the target only when the source path does not exist).
- `workload`: `backend` or a service with `network_access` `public` (the default)
  or `full_open`. meroku fills in the workload's base URL. The status is always
  `200`: Amplify fetches the response from the workload and returns it on the
  app's own host, so the frontend calls `/api/...` with no CORS setup.
  `path` (optional) replaces the source path on the workload.

`<*>` may appear once, at the end of `source`.

### The base URL of a workload

| Ingress | Domain | Base URL |
|---|---|---|
| `alb.enabled: true` | `domain.enabled` (required) | `https://<api domain>` |
| `alb.enabled: false` | `domain.enabled` and `domain.api_domain_prefix` set | `https://<api domain>` |
| `alb.enabled: false` | otherwise | the API Gateway execute-api endpoint, with its stage |

`<api domain>` is `<api_domain_prefix>.<env domain>`, or the env domain itself
when the prefix is empty. In ALB mode its alias record and certificate always
exist. The ALB's own `*.elb.amazonaws.com` hostname is never used, because it
does not match the certificate. In API Gateway mode the API domain has a DNS
record only when `api_domain_prefix` is set, so without one the rule points at
the execute-api endpoint.

The backend's URL is the base. A service's URL is the base plus `/<service>`,
the path both the ALB and API Gateway route to that service.

### How the target path is built

meroku joins the path at generate time (`app/amplify_rewrites.go`) and the module
only concatenates `workload_urls[workload] + target_path`:

| Rule | Target |
|---|---|
| `path` set | `<workload URL><path>` |
| backend, no `path` | `<base><source>` |
| service `s`, source starts with `/s/` | `<base>/s` + source without `/s`, so `/s/<*>` → `<base>/s/<*>` (no double prefix) |
| service `s`, any other source | `<base>/s<source>` |

### What generate refuses

`meroku generate` fails, naming the app and the rule, when:

- a rule sets both or neither of `target` and `workload`;
- `status` is not `200`, `301`, `302` or `404-200`, or a workload rule's status is not `200`;
- `<*>` appears more than once in `source`, or not at the end;
- the workload is not `backend` or a `public`/`full_open` service;
- `path` is set on a `target` rule, or does not start with `/`;
- a workload rule's `source` is not a path (a regex such as `</^...$/>`) and no `path` is set;
- a workload rule is used with `alb.enabled: true` and `domain.enabled: false`;
- the resolved API host is the host of an Amplify app (`subdomain_prefix.<env domain>`
  or `custom_domain`), because the rewrite would send the app's requests back to itself;
- a `backend` rule's target path is `/<s>`, starts with `/<s>/`, or is `/<s><*>`, where
  `<s>` is a `public` or `full_open` service. Both ingresses route `/<s>/...` to that
  service before the backend's catch-all (API Gateway `ANY /<s>/{proxy+}` over
  `ANY /{proxy+}`, ALB listener rule `/<s>/*`), so the request reaches the service with
  `/<s>` stripped, never the backend. `{source: "/api/<*>", workload: backend}` next to
  a public service `api` is the common case. Fix it by naming the service as the
  workload, or by setting `path` to one that does not start with `/<s>`
  (`{source: "/api/<*>", workload: backend, path: "/v1/<*>"}`). A service with
  `network_access` `internal` or `none` gets no route and does not collide.

A rule placed after a `/<*>` catch-all never matches; generate prints a warning.

### Limits of the proxy

- Server-sent events and WebSockets do not work through Amplify's `200` rewrite.
  The frontend calls `https://<api domain>` directly for those.
- Amplify does not publish its proxy timeout or its request body limit. The live
  check records what was measured; treat long requests and large uploads as
  candidates for calling the API domain directly.
- Amplify's CDN may cache a proxied response. API responses that must never be
  cached send `Cache-Control: no-store`.
- In a monorepo, set `AMPLIFY_MONOREPO_APP_ROOT` in the app-level
  `environment_variables`, as above. App-level values reach Amplify as written;
  `${...}` in them is not interpolated.

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