# Branch-Specific Subdomain Enhancement

## Overview
This enhancement adds support for branch-specific subdomain mapping in AWS Amplify apps, allowing different subdomains to point to different deployment branches.

## Key Features
- **Branch-Specific Subdomains**: Map specific subdomains to individual branches
- **Backward Compatibility**: Existing app-level subdomains continue to work
- **Flexible Configuration**: Mix app-level and branch-specific subdomains
- **Production Priority**: Root domain automatically maps to PRODUCTION branch

## Configuration Examples

### Basic Branch-Specific Subdomains
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
      - name: staging
        stage: BETA
        custom_subdomains: [staging, beta]
      - name: develop
        stage: DEVELOPMENT
        custom_subdomains: [dev, api-dev]
```

**Result:**
- `https://example.com` → main branch (root domain)
- `https://www.example.com` → main branch
- `https://app.example.com` → main branch
- `https://staging.example.com` → staging branch
- `https://beta.example.com` → staging branch
- `https://dev.example.com` → develop branch
- `https://api-dev.example.com` → develop branch

### Mixed Configuration (Legacy + Branch-Specific)
```yaml
amplify_apps:
  - name: main-web
    custom_domain: example.com
    sub_domains: [www]  # App-level (maps to first PRODUCTION branch)
    enable_root_domain: true
    branches:
      - name: main
        stage: PRODUCTION
        custom_subdomains: [app, api]  # Branch-specific
      - name: staging
        stage: BETA
        custom_subdomains: [staging, beta]
```

**Result:**
- `https://example.com` → main branch (root domain)
- `https://www.example.com` → main branch (app-level subdomain)
- `https://app.example.com` → main branch (branch-specific)
- `https://api.example.com` → main branch (branch-specific)
- `https://staging.example.com` → staging branch
- `https://beta.example.com` → staging branch

## Technical Implementation

### Terraform Changes

#### Variables
```hcl
# Added to branches object
custom_subdomains = optional(list(string), [])
```

#### Locals
```hcl
# Calculate branch-specific subdomain mappings
branch_subdomain_mappings = flatten([
  for app in local.normalized_apps : [
    for branch in app.branches : [
      for subdomain in branch.custom_subdomains : {
        app_name = app.name
        subdomain = subdomain
        branch_name = branch.name
        is_root = false
      }
    ] if length(branch.custom_subdomains) > 0
  ] if app.custom_domain != null && app.custom_domain != ""
])
```

#### Domain Association
```hcl
# Configure all subdomain mappings (legacy + branch-specific + root)
dynamic "sub_domain" {
  for_each = [
    for mapping in local.all_subdomain_mappings : mapping
    if mapping.app_name == each.key
  ]
  content {
    branch_name = sub_domain.value.branch_name
    prefix      = sub_domain.value.subdomain
  }
}
```

### Web UI Changes

#### Form State
```typescript
branches: [{
  name: 'main',
  stage: 'PRODUCTION',
  enable_auto_build: true,
  enable_pull_request_preview: false,
  environment_variables: {},
  custom_subdomains: [],
  custom_subdomains_text: ''
}]
```

#### UI Component
```jsx
<div>
  <Label htmlFor={`custom-subdomains-${index}`}>Custom Subdomains</Label>
  <Input
    id={`custom-subdomains-${index}`}
    value={branch.custom_subdomains_text || ''}
    onChange={(e) => updateBranch(index, 'custom_subdomains_text', e.target.value)}
    placeholder="api, staging, beta"
    className="mt-1"
  />
  <p className="text-xs text-gray-500 mt-1">
    Enter subdomain prefixes separated by commas. These will map specifically to this branch.
  </p>
</div>
```

## Use Cases

### 1. Environment-Specific APIs
```yaml
branches:
  - name: main
    stage: PRODUCTION
    custom_subdomains: [api, www]
  - name: staging
    stage: BETA
    custom_subdomains: [api-staging, staging]
  - name: develop
    stage: DEVELOPMENT
    custom_subdomains: [api-dev, dev]
```

### 2. Regional Deployments
```yaml
branches:
  - name: main-us
    stage: PRODUCTION
    custom_subdomains: [us, app-us]
  - name: main-eu
    stage: PRODUCTION
    custom_subdomains: [eu, app-eu]
  - name: main-ap
    stage: PRODUCTION
    custom_subdomains: [ap, app-ap]
```

### 3. Feature Branch Testing
```yaml
branches:
  - name: main
    stage: PRODUCTION
    custom_subdomains: [app, www]
  - name: feature-auth
    stage: EXPERIMENTAL
    custom_subdomains: [auth-test]
  - name: feature-ui
    stage: EXPERIMENTAL
    custom_subdomains: [ui-test]
```

## Migration Guide

### From App-Level to Branch-Specific

**Before:**
```yaml
amplify_apps:
  - name: webapp
    custom_domain: example.com
    sub_domains: [www, app, api]
    enable_root_domain: true
```

**After:**
```yaml
amplify_apps:
  - name: webapp
    custom_domain: example.com
    enable_root_domain: true
    branches:
      - name: main
        stage: PRODUCTION
        custom_subdomains: [www, app, api]
```

### Gradual Migration
You can migrate gradually by keeping app-level subdomains and adding branch-specific ones:

```yaml
amplify_apps:
  - name: webapp
    custom_domain: example.com
    sub_domains: [www]  # Keep existing
    enable_root_domain: true
    branches:
      - name: main
        stage: PRODUCTION
        custom_subdomains: [app]  # Add new branch-specific
      - name: staging
        stage: BETA
        custom_subdomains: [staging, beta]  # New staging subdomains
```

## Best Practices

1. **Use Clear Naming**: Choose descriptive subdomain names
   - `api-staging` instead of `s1`
   - `admin-beta` instead of `ab`

2. **Environment Consistency**: Use consistent patterns across environments
   - `api` for production
   - `api-staging` for staging  
   - `api-dev` for development

3. **Avoid Conflicts**: Don't use the same subdomain for multiple branches
   - ❌ Two branches both using `api` subdomain
   - ✅ One branch uses `api`, another uses `api-staging`

4. **Production Priority**: Always have at least one PRODUCTION branch for root domain mapping

5. **Documentation**: Document which subdomains map to which branches for your team

## Limitations

1. **Single Domain**: All subdomains must use the same root domain
2. **No Wildcards**: Subdomain prefixes must be explicit (no `*.api` patterns)
3. **DNS Propagation**: Changes may take up to 48 hours to propagate
4. **Certificate Limits**: AWS has limits on SSL certificates per domain

## Troubleshooting

### Subdomain Not Working
1. Check the branch exists and has the correct stage
2. Verify the subdomain is spelled correctly
3. Wait for DNS propagation (up to 48 hours)
4. Check AWS Amplify console for domain status

### Conflicting Subdomains
1. Ensure no two branches use the same subdomain
2. Check both app-level and branch-specific configurations
3. Review the `all_subdomain_mappings` in Terraform plan output

### Root Domain Issues
1. Ensure at least one branch has `stage: PRODUCTION`
2. Check `enable_root_domain` is set to true
3. Verify custom domain is properly configured