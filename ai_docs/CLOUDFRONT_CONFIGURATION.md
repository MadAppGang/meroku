# CloudFront CDN Configuration

This document describes the CloudFront CDN configuration added in schema version 14.

## Overview

CloudFront enables content delivery network (CDN) capabilities for your infrastructure, with support for:

- **Multiple origin types**: S3, Amplify, ALB, or custom URLs
- **Wildcard domains**: Support for patterns like `*.app.example.com`
- **Path-based routing**: Route `/api/*` to ALB, `/*` to Amplify
- **SPA mode**: Handle 404 errors by returning index.html
- **Automatic certificate management**: Creates ACM certificates in us-east-1

## Multi-Tenant SaaS Pattern

CloudFront is ideal for multi-tenant SaaS applications where each tenant gets a unique subdomain:

```
company1.app.example.com → CloudFront → Amplify
company2.app.example.com → CloudFront → Amplify
company3.app.example.com → CloudFront → Amplify
```

The same React app serves all tenants. Tenant detection happens client-side:

```typescript
// In React app
const getTenant = () => {
  const hostname = window.location.hostname;
  // company1.app.example.com → ["company1", "app", "example", "com"]
  const parts = hostname.split('.');
  return parts.length >= 4 ? parts[0] : 'default';
};
```

## YAML Configuration

### Basic Configuration

```yaml
cloudfront:
  enabled: true
  domain_aliases:
    - "*.app.example.com"
    - "app.example.com"
  spa_mode: true
  origins:
    - name: frontend
      type: amplify
      amplify_app_name: myapp
```

### Full Configuration

```yaml
cloudfront:
  enabled: true

  # Custom domains (including wildcards)
  domain_aliases:
    - "*.app.example.com"
    - "app.example.com"

  # Origins (content sources)
  origins:
    - name: frontend
      type: amplify
      amplify_app_name: circlmini

    - name: api
      type: alb  # Uses ALB from workloads module

    - name: static
      type: s3
      bucket_name: my-static-assets
      use_oac: true  # Origin Access Control for S3

    - name: external
      type: custom
      domain_name: api.external-service.com
      protocol_policy: https-only

  # Path-based routing
  cache_behaviors:
    - path_pattern: "/api/*"
      origin_name: api
      forward_query_string: true
      forward_headers: ["Host", "Origin", "Authorization"]
      forward_cookies: all
      min_ttl: 0
      default_ttl: 0
      max_ttl: 0

    - path_pattern: "/static/*"
      origin_name: static
      min_ttl: 86400
      default_ttl: 604800
      max_ttl: 31536000

  # SPA mode (404 → index.html)
  spa_mode: true
  default_root_object: index.html

  # Price class (cost vs edge locations)
  price_class: PriceClass_100  # US & Europe only

  # Access logging
  logging:
    enabled: true
    bucket_name: my-logs-bucket
    prefix: cloudfront/
    include_cookies: false
```

## Origin Types

### Amplify Origin

Routes traffic to an existing Amplify app:

```yaml
origins:
  - name: frontend
    type: amplify
    amplify_app_name: myapp  # Must match name in amplify_apps[]
```

### S3 Origin

Serves static files from an S3 bucket with Origin Access Control:

```yaml
origins:
  - name: assets
    type: s3
    bucket_name: my-bucket
    use_oac: true  # Recommended: secure S3 access
```

### ALB Origin

Routes to the Application Load Balancer (requires `alb.enabled: true`):

```yaml
origins:
  - name: backend
    type: alb
    # domain_name is auto-resolved from workloads module
```

### Custom Origin

Routes to any external HTTPS endpoint:

```yaml
origins:
  - name: external-api
    type: custom
    domain_name: api.external-service.com
    protocol_policy: https-only
    custom_headers:
      X-Custom-Header: "value"
```

## Cache Behaviors

Cache behaviors define path-based routing rules:

```yaml
cache_behaviors:
  - path_pattern: "/api/*"
    origin_name: backend
    allowed_methods: ["DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"]
    cached_methods: ["GET", "HEAD"]
    forward_query_string: true
    forward_headers: ["Host", "Origin", "Authorization"]
    forward_cookies: all
    viewer_protocol_policy: redirect-to-https
    min_ttl: 0
    default_ttl: 0
    max_ttl: 0
    compress: true
```

### Path Pattern Examples

- `/api/*` - All API requests
- `/static/*` - Static assets
- `/images/*` - Image files
- `*.js` - JavaScript files (file extension match)

## Certificate Management

CloudFront automatically creates and validates ACM certificates:

1. Certificate is created in **us-east-1** (required by CloudFront)
2. DNS validation records are created in Route53
3. Certificate covers all domain aliases (including wildcards)

```hcl
# Auto-generated certificate covers:
# - *.app.example.com (wildcard)
# - app.example.com (apex)
```

## Price Classes

| Price Class | Edge Locations | Best For |
|-------------|----------------|----------|
| `PriceClass_100` | US, Canada, Europe | Cost-sensitive, US/EU users |
| `PriceClass_200` | US, Canada, Europe, Asia, Africa, Middle East | Balanced coverage |
| `PriceClass_All` | All edge locations globally | Maximum performance |

## Multi-Tenant Implementation

### Frontend (React)

```typescript
// src/utils/tenant.ts
export function getTenantFromSubdomain(): string {
  const hostname = window.location.hostname;
  const parts = hostname.split('.');

  // company1.app.example.com → ["company1", "app", "example", "com"]
  if (parts.length >= 4 && parts[1] === 'app') {
    return parts[0]; // "company1"
  }
  return 'default';
}

// Usage in API calls
const tenant = getTenantFromSubdomain();
fetch('https://api.example.com/data', {
  headers: {
    'X-Tenant-ID': tenant,
  }
});
```

### Backend (Go)

```go
func getTenantFromOrigin(r *http.Request) string {
    origin := r.Header.Get("Origin")
    // origin = "https://company1.app.example.com"

    u, err := url.Parse(origin)
    if err != nil {
        return ""
    }

    parts := strings.Split(u.Host, ".")
    if len(parts) >= 4 && parts[1] == "app" {
        return parts[0] // "company1"
    }
    return ""
}
```

## Terraform Outputs

When CloudFront is enabled, the following outputs are available:

```hcl
output "cloudfront_distribution_id" {
  value = module.cloudfront.distribution_id
}

output "cloudfront_distribution_domain" {
  value = module.cloudfront.distribution_domain_name
}

output "cloudfront_domain_aliases" {
  value = module.cloudfront.domain_aliases
}
```

## Deployment

1. Enable CloudFront in your YAML configuration
2. Run `make infra-gen-{env}` to generate Terraform files
3. Run `make infra-plan env={env}` to preview changes
4. Run `make infra-apply env={env}` to deploy

## Troubleshooting

### Certificate Validation Fails

- Ensure domain is configured with Route53
- Check that `domain.enabled: true` is set
- Verify DNS zone exists and has proper access

### 403 Errors on S3 Origin

- Enable `use_oac: true` on S3 origins
- Ensure S3 bucket allows CloudFront OAC access

### Cache Issues

- For dynamic content, set `min_ttl: 0`, `default_ttl: 0`, `max_ttl: 0`
- Use proper cache headers in your application
- Invalidate cache: `aws cloudfront create-invalidation --distribution-id <ID> --paths "/*"`
