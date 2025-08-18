# DNS Setup Guide

## Quick Start

Run `./meroku` and select **🔧 Setup Custom Domain (DNS)**

## Setup Flow

### 1. Domain Entry
Enter your root domain (e.g., `example.com`)

### 2. Production Setup (if needed)
- If no `prod.yaml` exists, enter your 12-digit AWS Account ID
- System creates AWS profile automatically if missing
- Creates `prod.yaml` with configuration

### 3. Zone Creation
- Creates Route53 zone in production account
- Shows 4 AWS nameservers
- Creates IAM delegation role

### 4. Update Registrar
Copy nameservers (press `C` for all or `1-4` for individual) and update at your registrar:
- **GoDaddy**: DNS → Nameservers → Custom
- **Namecheap**: Domain → Nameservers → Custom DNS  
- **Cloudflare**: DNS → Nameservers tab

### 5. Subdomain Delegation
After propagation (~10min), select environments for subdomain delegation:
- `dev.example.com` → Dev account
- `staging.example.com` → Staging account

## Apply Infrastructure

```bash
# Generate Terraform
make infra-gen-dev
make infra-gen-prod

# Deploy
make infra-plan env=dev
make infra-apply env=dev
```

## DNS Commands

```bash
./meroku dns status     # Check configuration
./meroku dns validate   # Verify setup
./meroku dns remove     # Remove delegation
```

## Configuration Files

- `dns.yaml` - Central DNS state
- `prod.yaml`, `dev.yaml` - Environment configs (auto-created)
- All use root domain name (e.g., `example.com`)
- Prefixes added automatically via `add_env_domain_prefix`

## Service Endpoints

| Service | Production | Development |
|---------|------------|-------------|
| API | api.example.com | api.dev.example.com |
| App | app.example.com | app.dev.example.com |
| Root | example.com | dev.example.com |