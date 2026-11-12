# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## !! THIS REPO IS PUBLIC - NEVER COMMIT SECRETS !!

**ABSOLUTE RULE: No real infrastructure data, investigation files, or debug output may EVER be committed to this repository.**

This repo is **public on GitHub**. Any committed secret is immediately exposed to the world.

**NEVER commit:**
- Real Terraform plan/state JSON files (they contain embedded secrets: SSH keys, API keys, passwords, account IDs)
- AWS credentials, SSH keys, API tokens, or any secret material
- Debug/investigation output from real infrastructure
- Real YAML configs with account IDs or credentials
- Any file from a real project's `.terraform/` directory

**For test data:** Always use **synthetic/fake data** with dummy values. Never copy-paste from real infrastructure output.

**If you need test fixtures:** Generate them manually with placeholder values like `REDACTED`, `000000000000`, `fake-key-do-not-use`.

## Project Overview

This is a comprehensive Terraform Infrastructure as Code (IaC) repository that provides a reusable, modular AWS infrastructure setup. It includes:

- Terraform modules for AWS services (20+ modules including ECS, RDS, ALB, Cognito, Lambda, etc.)
- Go CLI application ("meroku") for interactive infrastructure management using Bubble Tea TUI framework
- React+TypeScript web frontend for visual infrastructure management
- Handlebars-based templating system (using Raymond Go package) for environment configuration
- GitHub Actions CI/CD workflows with OIDC authentication

## Template and Configuration Management Strategy

### Dual Approach: Migrations + Default Helper

We use **both** migrations and the `default` Handlebars helper for managing configuration values:

#### When to Use Migrations

**Use migrations for core infrastructure fields that should always be explicit in YAML:**

```go
// migrations.go - Example v12
booleanDefaults := map[string]interface{}{
    "multi_az":                             false,
    "storage_encrypted":                    true,
    "iam_database_authentication_enabled":  false,
}
```

**Template:**
```handlebars
multi_az = {{postgres.multi_az}}
storage_encrypted = {{postgres.storage_encrypted}}
```

**Benefits:**
- YAML is self-documenting (all values visible)
- Single source of truth for defaults (migration code)
- Type safety at load time
- Fields always exist when template renders

**Use for:**
- Core infrastructure booleans (security, HA, encryption)
- Required fields that should never be missing
- Fields where explicit values improve clarity

#### When to Use Default Helper

**Use `default` helper for optional fields with sensible fallbacks:**

```handlebars
instance_class = "{{default postgres.instance_class "db.t4g.micro"}}"
allocated_storage = {{default postgres.allocated_storage 20}}
backend_cpu = "{{default workload.backend_cpu "256"}}"
```

**Benefits:**
- Defaults visible in template (documentation)
- Less YAML clutter for optional fields
- Flexibility to override when needed
- Works correctly with `false`, `0`, `0.0` (fixed in our implementation)

**Use for:**
- Optional configuration with sensible defaults
- Fields that vary by use case
- Non-critical settings

#### Default Helper Behavior

The `default` helper in `app/raymond.go` has been fixed to handle all types correctly:

- `nil` (field missing from YAML) → returns default
- `false` → returns `false` (valid value)
- `0` → returns `0` (valid value)
- `0.0` → returns `0.0` (valid value)
- `""` (empty string) → returns default
- `[]` (empty array) → returns default

**Example:**
```yaml
# YAML
postgres:
  min_capacity: 0  # Explicitly set to 0 (valid for Aurora pausing)

# Template
min_capacity = {{default postgres.min_capacity 0.5}}

# Result: 0 (uses explicit value, not default)
```

#### When to Use Exists Helper

**Use `exists` helper when you need to distinguish "value is 0/false" from "value is missing":**

```handlebars
{{!-- Aurora capacity: 0 is valid (pause when idle), distinguish from missing --}}
{{#if (exists postgres.min_capacity)}}
  min_capacity = {{postgres.min_capacity}}
{{/if}}
```

**Why `exists` vs `{{#if}}`:**
- `{{#if value}}` checks **truthiness** → 0, false, "", [] are all falsy
- `{{#if (exists value)}}` checks **presence** → only nil is false

**Use cases:**
- Aurora Serverless v2 capacity where 0 means "pause when idle"
- Numeric fields where 0 is a valid configuration
- Boolean fields where false needs different handling than missing

**Pattern comparison:**

```handlebars
{{!-- OLD PATTERN (complex, hard to read) --}}
{{#if (or min_capacity (eq min_capacity 0))}}
  min_capacity = {{min_capacity}}
{{/if}}

{{!-- NEW PATTERN (simple, clear intent) --}}
{{#if (exists min_capacity)}}
  min_capacity = {{min_capacity}}
{{/if}}
```

**Other available helpers:**
- `or` - Logical OR: `{{#if (or a b)}}` (checks truthiness)
- `eq` - Equality: `{{#if (eq value 0)}}` (compares values)
- `default` - Fallback values: `{{default value 10}}`

**Tests:** All helpers have comprehensive test coverage in `app/raymond_test.go`

## Important Architecture Decisions

### VPC Configuration

**Default VPC Strategy**: New projects use **custom VPCs with 2 public subnets**, and every ECS
task gets its own public IP. Private subnets and a NAT Gateway exist only when an environment
opts into a NAT egress strategy.

Key decisions:
- **Custom VPC by default** (`use_default_vpc: false`) - Better isolation and control
- **Hardcoded to 2 AZs** (`local.az_count` in `modules/vpc/main.tf`) - minimum for HA, no option
- **Egress strategy** (`egress_strategy`, schema v27) decides how tasks reach the internet:

  | Value | Network | Cost |
  |---|---|---|
  | `public_ip` (default) | Public subnets only, a public IPv4 per task | $3.65/mo per address |
  | `nat_gateway` | Adds private subnets + one NAT; tasks lose their public IPs | ~$32.85/mo + $0.045/GB |
  | `nat_gateway_ha` | One NAT per AZ, survives an AZ loss | ~$65.70/mo + $0.045/GB |

  `app/egress_advisor.go` (web UI: `EgressAdvisor.tsx`) recommends a NAT from 6 services in
  non-prod and 11 in prod, where it becomes cheaper than per-task addresses. Switching is a
  one-line change and a rolling redeploy. Cost model: [Egress Cost Model](./ai_docs/EGRESS_COST_MODEL.md).
- **S3 gateway endpoint** is always created (free). In NAT environments it keeps S3 traffic,
  including every ECR image layer, off the NAT.
- Anything AWS will not give a public IP (CodeBuild or Lambda attached to the VPC) has internet
  access only in a NAT environment, from the private subnets.

Configuration options:
- `use_default_vpc`: `true` (use AWS default VPC) or `false` (create custom VPC)
- `egress_strategy`: `public_ip` | `nat_gateway` | `nat_gateway_ha` (see above)
- `vpc_cidr`: CIDR block for the custom VPC. New environments get a **random free /16** from
  10.0.0.0/8 (never 10.0.0.0/15 or 10.99.0.0/16, never a range a sibling env uses), written
  explicitly into the YAML. An unset key still builds the module default `10.0.0.0/16`.
- `allow_vpc_cidr_override`: Optional escape hatch that turns off the `vpc_cidr` rule checks
  for an environment already deployed with a range the rules now refuse.

**VPC ranges must not overlap** if environments are ever to be peered or share a Transit
Gateway, and a VPC's primary range can never be changed — changing `vpc_cidr` replaces the VPC
and everything in it. Rules (errors at `meroku generate`): IPv4 network address, private
(10.0.0.0/8 or 192.168.0.0/16), not in 172.16.0.0/12, 169.254.0.0/16 or 198.19.0.0/16, size
/16–/19. Overlap with another env is a warning. `meroku vpc-cidr check|suggest|assign <env>
[--aws] [--force]` checks, suggests or writes a range (`--aws` also avoids every VPC in the env's
account and in every sibling env's account; `assign` writes only for environments not yet deployed). Code: `app/vpc_cidr*.go`,
`GET /api/vpc-cidr`. Full design: [VPC CIDR Allocation](./ai_docs/VPC_CIDR_ALLOCATION.md).

The default (`public_ip`) is the simplest layout that keeps high availability:
- **2 Availability Zones** - Minimum for HA, covers regional failures
- **Public subnets only** - Direct internet access, no NAT charge
- **Security groups** - Access control without network complexity
- **Cost-effective** - No NAT until the service count makes one cheaper, no interface VPC endpoints

**Why 2 AZs is hardcoded:**
- 2 AZs is the minimum for high availability
- Handles single AZ failure (most common outage scenario)
- 3 AZs adds cost with marginal benefit for 99% of applications
- Keeps configuration simple - one less thing to think about
- Power users can modify the VPC module directly if needed

**Migration Note**:
- Existing projects migrating from before schema v6 will keep `use_default_vpc: true` for backward compatibility
- The v6 migration removed the old fields `az_count`, `create_private_subnets`, `enable_nat_gateway`.
  Private subnets and NAT now come only from `egress_strategy` (v27), never from those keys.

### Interface VPC Endpoints (Not Used)

Interface endpoints (~$7.30/mo per AZ each) are not used. The interface endpoint code in
`modules/workloads/ecs_endpoints.tf` is commented out and should remain so. The free S3
**gateway** endpoint in `modules/vpc` is the one endpoint meroku creates.

### API Gateway vs ALB

The infrastructure supports two ingress patterns, switched by `alb.enabled` in the YAML
(rendered as `enable_alb`):

- **Default (`alb.enabled: false`)**: API Gateway → ECS Services
- **Alternative (`alb.enabled: true`)**: ALB → ECS Services, serving the same `api_domain`

The API Gateway API and the Cloud Map service exist in both modes. The ALB (`modules/alb`, `count = var.enabled`) exists only when enabled, and the backend's API Gateway integration and route only when it is not.

One Route 53 record (`aws_route53_record.backend`, `domain.tf`) serves `api_domain` in both
modes, so flipping `alb.enabled` updates its alias in place. Optional keys `alb.ssl_policy`,
`alb.deregistration_delay`, `workload.backend_health_check_grace_period` and
`alb.remove_api_gateway` (drops the API Gateway API, stage, log group and VPC Link; refused with
the ALB off, a `full_open` service or SNS webhooks) default to today's behaviour.
[ALB Settings](./ai_docs/ALB_SETTINGS.md).

## AWS Resource Naming Conventions

This section documents the naming patterns used for AWS resources created by meroku.

### Backend Service (`modules/workloads/backend.tf`)

The main backend service uses these patterns:

| Resource | Pattern | Example |
|----------|---------|---------|
| ECS Service | `${project}_service_${env}` | `myapp_service_dev` |
| ECS Cluster | `${project}_cluster_${env}` | `myapp_cluster_dev` |
| ECR Repository | `${project}_backend` | `myapp_backend` |
| CloudWatch Log Group | `${project}_backend_${env}` | `myapp_backend_dev` |
| Security Group | `${project}_backend_${env}` | `myapp_backend_dev` |
| IAM Task Role | `${project}_backend_task_${env}` | `myapp_backend_task_dev` |
| IAM Execution Role | `${project}_backend_task_execution_${env}` | `myapp_backend_task_execution_dev` |
| S3 Bucket | `${project}-backend-${env}${postfix}` | `myapp-backend-dev-abc12` |

### Named Services (`modules/workloads/services.tf`)

Additional services defined in YAML use these patterns:

| Resource | Pattern | Example |
|----------|---------|---------|
| ECS Service | `${project}_service_${name}_${env}` | `myapp_service_api_dev` |
| ECR Repository | `${project}_service_${name}` | `myapp_service_api` |
| CloudWatch Log Group | `${project}_service_${name}_${env}` | `myapp_service_api_dev` |
| Security Group | `${project}_service_${name}_${env}` | `myapp_service_api_dev` |
| IAM Task Role | `${project}_service_${name}_task_${env}` | `myapp_service_api_task_dev` |
| IAM Execution Role | `${project}_service_${name}_task_execution_${env}` | `myapp_service_api_task_execution_dev` |
| Target Group | `${project}-service-${name}-tg-${env}` | `myapp-service-api-tg-dev` |

## AWS Name Length Limits — `modules/naming`

**Never interpolate a capped AWS name at the resource. Declare it in the module's
`naming.tf` instead.** Hand-assembling `"${var.project}-<decoration>-${var.env}"`
is how a deploy came to fail with `"name" cannot be longer than 32 characters`,
pointing at a line of Terraform rather than at the service that was too long.

AWS caps are not generous: **32** (ALB, target group), **40** (App Runner), **63**
(S3 bucket, RDS identifier), **64** (IAM role, Lambda, EventBridge rule,
scheduler), **80** (SQS queue), **128** (IAM policy). A template like
`${project}-service-${name}-tg-${env}` spends 13 of a 32-character budget on the
words `service` and `tg`.

### The cascade — first form that fits wins

| # | Form | Example (`myapp` / `dev`) |
|---|------|-------------------------------|
| 1 | `legacy`, verbatim | `myapp-service-api-tg-dev` |
| 2 | `project` + `parts` + `env` | `myapp-orders-sync-dev` |
| 3 | `parts` + `md5:8` | `payments-4f2a9c1e` |

The ordering is the design: the identity is the only field that tells two names
apart in a console listing, so it is the **last** thing given up, never the first.

**Form 1 exists solely to protect what is already deployed.** Most AWS names are
ForceNew, and a target group a listener rule references cannot be deleted at all
— a rename is a destroy-and-recreate whose destroy blocks. Pass the resource's
current name as `legacy` and any name that fits today comes back byte-identical.

`suffix` (`.fifo`, an S3 bucket postfix) is counted against the budget and never
truncated.

### Adding a name

```hcl
# modules/<yours>/naming.tf
module "naming" {
  source  = "../naming"
  project = var.project
  env     = var.env

  requests = {
    my_role = {
      legacy    = "${var.project}_my_role_${var.env}"  # omit for a NEW resource
      parts     = ["my", "role"]                       # identity, no project/env
      limit     = 64
      separator = "_"
    }
  }
}
```

Then `name = module.naming.names["my_role"]`.

Assert `module.naming.collisions` is empty in a `precondition` where several
names share a shape — see `modules/workloads/services.tf`. The cascade lets forms
mix, so a `parts` list spelling out another request's decorated form can land on
a name already taken.

### Two implementations, one rule

The algorithm exists twice because name construction happens in two places that
cannot share code:

- **`modules/naming/`** — for `modules/**.tf`, which only see `var.project`/`var.env`.
- **`app/aws_name.go`** — for `env/main.hbs` (SQS/SNS), rendered by Go *before*
  Terraform runs. Exposed as the `awsName` Handlebars helper.

They are pinned together by `app/testdata/aws_name_vectors.json`, read by
`app/aws_name_test.go` and mirrored by `modules/naming/tests/cascade.tftest.hcl`.
Change one half without the other and a test fails.

Run them with `terraform -chdir=modules/naming test` and `go test ./app/...`.

Names capped at 128 or above (IAM policies, security groups, log groups, ECS task
definitions, SSM parameters) stay as literal templates — the cutoff is 80.

### Scheduled Tasks (`modules/ecs_task`)

Scheduled ECS tasks use these patterns:

| Resource | Pattern | Example |
|----------|---------|---------|
| ECR Repository | `${project}_task_${name}` | `myapp_task_cleanup` |
| Task Definition | `${project}_task_${name}_${env}` | `myapp_task_cleanup_dev` |
| Container | `${project}_container_${name}_${env}` | `myapp_container_cleanup_dev` |
| Security Group | `${project}_${name}_${env}` | `myapp_cleanup_dev` |
| IAM Task Role | `${project}_${name}_task_${env}` | `myapp_cleanup_task_dev` |
| IAM Execution Role | `${project}_scheduler_${name}_task_execution_${env}` | `myapp_scheduler_cleanup_task_execution_dev` |
| Scheduler Role | `${project}_scheduler_${name}_role_${env}` | `myapp_scheduler_cleanup_role_dev` |
| Schedule Group | `${project}-schedule-group-${env}-${name}` | `myapp-schedule-group-dev-cleanup` |

### pgAdmin (`modules/workloads/pgadmin.tf`)

The optional pgAdmin service uses:

| Resource | Pattern | Example |
|----------|---------|---------|
| ECS Service | `pgadmin_service_${env}` | `pgadmin_service_dev` |
| Container | `${project}_pgadmin_${env}` | `myapp_pgadmin_dev` |
| Security Group | `${project}_pgadmin_${env}` | `myapp_pgadmin_dev` |
| CloudWatch Log Group | `${project}-pgadmin_${env}` | `myapp-pgadmin_dev` |
| IAM Task Role | `${project}_pgadmin_task_${env}` | `myapp_pgadmin_task_dev` |

### GitHub Actions (`modules/workloads/github.tf`)

| Resource | Pattern | Example |
|----------|---------|---------|
| IAM Role | `${project}-${env}-github-actions-role` | `myapp-dev-github-actions-role` |

### Naming Convention Rules

1. **Underscores (`_`)** are used for most AWS resource names (ECS, IAM, ECR, CloudWatch)
2. **Hyphens (`-`)** are used for:
   - S3 bucket names (required by AWS)
   - ALB target groups (convention)
   - GitHub Actions roles (convention)
   - EventBridge schedule groups
3. **Environment suffix** (`_${env}` or `-${env}`) is always last
4. **Project prefix** (`${project}_` or `${project}-`) is always first

### CI/CD Workflow Configuration

When generating GitHub Actions workflows, use these patterns:

```yaml
env:
  ECR_REPOSITORY: ${project}_backend           # For backend
  ECR_REPOSITORY: ${project}_service_${name}   # For named services
  ECS_CLUSTER: ${project}_cluster_${env}
  ECS_SERVICE: ${project}_service_${env}       # For backend
  ECS_SERVICE: ${project}_service_${name}_${env}  # For named services
  IAM_ROLE: arn:aws:iam::${account}:role/${project}-${env}-github-actions-role
```

## SES DNS records — who owns which value

In a zone meroku **created**, the YAML is the whole truth for an SES domain's TXT
records (`aws_route53_record`, `allow_overwrite`). In a zone it did **not**
create (`ses.domains[].zone_id`, an additional domain with `zone_id`, a main
domain without `create_domain_zone`), meroku adds and removes only its own
values through `modules/ses/scripts/route53-txt.sh` (needs `/bin/sh` and AWS CLI
v2) and refuses at plan to replace a foreign SPF or DMARC value the YAML does
not set. Origin: `app/ses_zone_origin.go`. Full rules, migration and destroy
behaviour: [SES DNS Ownership](./ai_docs/SES_DNS_OWNERSHIP.md).

## Container env vars and SSM secrets

A container's `environment` comes from the YAML (`env_vars`, `backend_env_variables`,
tasks' `environment_variables`, all maps) plus meroku's own variables; its `secrets`
come from every SSM parameter under the workload's path, named by the upper-cased last
segment. A name in both is refused by ECS. **`ENV` is reserved**: meroku creates
`<path>/env` for every workload, so `ENV` is set with `aws ssm put-parameter`, never in
YAML; `meroku generate` refuses it (`app/env_reserved.go`).
[Container Env and SSM](./ai_docs/CONTAINER_ENV_AND_SSM.md).

## Backend health check and circuit breaker

`workload.backend_health_check` (container `healthCheck`) and
`workload.backend_circuit_breaker` (`deployment_circuit_breaker` on the backend ECS
service) are optional; absent keys render nothing. A breaker is turned off with
`enable: false`, never by removing the key (provider 6.x suppresses that diff).
[Backend Health Check](./ai_docs/BACKEND_HEALTH_CHECK.md).

## GitHub Actions runners (CodeBuild)

`github_runners` (optional; absent or `enabled: false` renders nothing, no
migration) creates one CodeBuild project per runner, with a `WORKFLOW_JOB_QUEUED`
webhook, in `modules/github_runners`. The CodeBuild webhook uses `manual_creation`
and the GitHub side is a `github_repository_webhook`/`github_organization_webhook`
(provider `integrations/github`, token read ephemerally from `token_secret`);
letting CodeBuild create the GitHub hook fails on the first apply because the new
service role has not propagated in IAM yet. All runners share one GitHub owner. Workflows use
`runs-on: codebuild-<project>-gh-<runner>-<env>-${{ github.run_id }}-${{ github.run_attempt }}`
(env output `github_runner_labels`). The GitHub token lives in a Secrets Manager
secret created outside Terraform and is set per project, never as an
account-wide `aws_codebuild_source_credential`. `vpc_access: true` requires a NAT
`egress_strategy` (CodeBuild in a VPC gets no public IP); `app/github_runners.go`
refuses otherwise on every generate. The runner role cannot deploy; deploys use
the GitHub OIDC role. [GitHub Runners](./ai_docs/GITHUB_RUNNERS.md).

## Fix with AI (Claude Code handoff)

meroku has no AI of its own and needs no API key. "Fix with AI" writes a briefing
about what failed and starts [Claude Code](https://claude.com/claude-code) beside
meroku to read it.

### Where it is offered

- **Key `a`** on the failed-apply screen, the failed-plan screen and the failed-destroy
  screen. Footer: `[a] Fix with AI (Claude Code)`. On the normal plan dashboard `a` is
  still Apply.
- **Main menu**: "🤖 Fix with AI (Claude Code)". Nothing has failed yet, so the briefing
  tells Claude to ask the user what is wrong.
- **AWS SSO**: "🤖 Fix with AI (Claude Code)" in the AWS SSO Tools menu, in the startup
  SSO check, and as choice 2 of the profile validator's fix prompt. The briefing names
  the profile, the fields the inspector found missing and the AWS config path. It holds
  no tokens or keys.

### What happens

1. meroku writes the briefing to `<project root>/.meroku/claude/<YYYYMMDD-HHMMSS>-<operation>.md`
   and creates `.meroku/.gitignore` containing `*` so the directory never gets committed.
   The briefing can contain raw Terraform/AWS error output.
2. Inside herdr or tmux with `claude` on PATH (`TERM_PROGRAM` picks the inner one when
   nested): meroku opens a pane to the right at the project root and starts `claude`
   there with a one-line prompt that points at the briefing file, plus
   `--settings '{"enabledPlugins":{"meroku@magus":true}}'` so the meroku skills from
   the magus marketplace load when installed. In herdr it waits for the new shell's
   prompt before typing, and checks that Claude started. The briefing itself is never
   put on the command line.
3. Otherwise: meroku prints the briefing's absolute path and the command to start
   Claude Code on it, and waits for Enter.

The project root, env name, AWS profile, region and account come from the env YAML
meroku loaded (threaded down from `runCommandToDeploy` / the menu), never from the
working directory or `$AWS_PROFILE`.

### The rules the briefing gives Claude

- Do NOT fix the problem directly in AWS (no console changes, no CLI/SDK writes).
- Do NOT edit the generated Terraform in `env/<env>/`; meroku regenerates it.
- Allowed fixes, in order: (a) a meroku bug, fixed in meroku itself
  (github.com/MadAppGang/meroku); (b) an edge case meroku cannot migrate, done as an
  agreed one-off manual step that gets written down; (c) a configuration problem,
  fixed in the env YAML and rerun through meroku.
- Read-only inspection (`describe`/`list`/`get`, `terraform state list/show`,
  `terraform plan`) is fine. Investigate and explain first; ask before any write.

### Code and docs

- Briefing text: `app/claude_briefing/briefing.md`, embedded with `go:embed`, so it
  always matches the binary that is running.
- Launcher and renderer: `app/claude_handoff.go`. Context builders (apply, plan,
  destroy, menu, SSO): `app/claude_handoff_context.go`.
- [Claude Code Handoff](./ai_docs/CLAUDE_CODE_HANDOFF.md) — how it works and how to
  change the briefing.

## Memories

- Always keep all AI-related documentation, created by AI or intended to be consumed by AI, in the @ai_docs/ folder

## Common Commands

Uses [Task](https://taskfile.dev) (`Taskfile.yml`) — install via `brew install go-task`.

### Infrastructure Management

```bash
# Initialize Terraform (run after creating env/*.tf files)
task infra-init env=dev

# Update Terraform modules
task infra-update env=dev

# Plan infrastructure changes
task infra-plan env=dev

# Apply infrastructure changes
task infra-apply env=dev

# Destroy infrastructure
task infra-destroy env=dev

# Show current infrastructure state
task infra-show env=dev

# Generate Terraform files from YAML config
task infra-gen-dev    # For dev environment
task infra-gen-prod   # For prod environment

# Import existing AWS resources
task infra-import env=dev
```

### First-Time Setup

```bash
# Enable shared git hooks (auto-rebuilds webapp on commit)
git config core.hooksPath .githooks
```

### Development Commands

```bash
# Run the TUI application
task tui

# Run the web frontend dev server
task web

# Build the CLI
task build

# Run tests
task test

# Run web tests
task test:web

# Lint web frontend
task lint:web

# Build web frontend into app/webapp (embedded in Go binary)
task web:build

# Generate code from templates
task generate

# Test terraform plan diff viewer (for debugging)
task renderdiff file=terraform-plan.json

# List all available tasks
task --list
```

### Web Build Pipeline

The React frontend (`web/`) builds into `app/webapp/` which is embedded into the Go binary. A **pre-commit hook** (`.githooks/pre-commit`) auto-rebuilds the webapp bundle whenever `web/src/` files are staged for commit.

- Hook auto-runs `pnpm exec vite build` and stages the output
- If the build fails, the commit is blocked
- Requires: `git config core.hooksPath .githooks` (one-time setup)

**Package manager: pnpm only.** `web/pnpm-lock.yaml` is the single source of truth, enforced in CI
via `pnpm install --frozen-lockfile`. The pnpm version is pinned in two places that must stay in
sync: `packageManager` in `web/package.json` and `PNPM_VERSION` in `.github/workflows/release.yml`.
Do not run `npm install`, `yarn`, or `bun install` in `web/` — their lockfiles are gitignored, and
they resolve `^` ranges differently than what CI ships.

## Project Structure

```
meroku/
├── modules/          # Terraform modules for AWS services
├── env/              # main.hbs: the Handlebars template every env's Terraform is rendered from
├── project/          # Example YAML configuration (dev.yaml)
├── app/              # Go CLI application (meroku); app/webapp holds the built web UI
├── web/              # React+TypeScript frontend
├── ai_docs/          # Documentation by and for AI agents
├── docs/             # Human documentation (DNS, ECR, CI/CD patterns)
└── scripts/          # Release and dev utility scripts
```

## Key Configuration Files

- `project/dev.yaml` - Development environment configuration
- `project/prod.yaml` - Production environment configuration
- `env/dev/*.tf` - Generated Terraform files for dev (DO NOT EDIT MANUALLY)
- `env/prod/*.tf` - Generated Terraform files for prod (DO NOT EDIT MANUALLY)

## Working with the Codebase

1. **Making Infrastructure Changes**: Edit YAML files in `project/`, then run `task infra-gen-dev` or `task infra-gen-prod`
2. **Adding New Services**: Update the `services` array in YAML configuration
3. **Modifying Terraform Modules**: Edit files in `modules/` directory
4. **Updating Templates**: Modify the Handlebars template `env/main.hbs`

## Testing Guidelines

- Always run `task infra-plan env={env}` before applying changes
- Test infrastructure changes in dev environment first
- Use `task test` to run Go tests
- Use `task ci` to run everything CI runs (both Go modules + Terraform), `task lambda:test` for the CI Lambda
- Frontend: there are no application tests; `task test:web` only prints that. Component stories are
  tested with `pnpm test-storybook` (vitest + Playwright) in `web/`.

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

## DNS Management

The infrastructure includes a comprehensive DNS management system that handles cross-account zone delegation:

### DNS Setup Workflow

1. **Initial Setup**: Run `./meroku` and select "DNS Setup" from the menu
2. **Root Zone Creation**: The wizard creates the root zone in the production account
3. **Automatic Configuration**: Environment files (dev.yaml, prod.yaml, staging.yaml) are automatically updated
4. **Subdomain Delegation**: Non-production environments get delegated subdomains (dev.example.com, staging.example.com)

### DNS Commands

```bash
# Run interactive DNS setup wizard
./meroku
# Then select "DNS Setup" from menu

# Check DNS configuration status
./meroku dns status

# Validate DNS propagation
./meroku dns validate

# Remove subdomain delegation
./meroku dns remove <subdomain>
```

### DNS Architecture

- **Root Zone**: Created in production account with delegation IAM role
- **Subdomain Zones**: Created in respective environment accounts
- **Cross-Account Access**: Uses IAM role assumption for NS record management
- **Automatic Delegation**: NS records are automatically created in root zone

### Configuration Files

- `dns.yaml`: Stores root zone information and delegated zones
- `project/*.yaml`: Environment files contain zone IDs and delegation info
- Domain names ALWAYS use root domain (e.g., "example.com") in all environments
- Environment prefixes are added automatically based on `add_env_domain_prefix` flag

### Documentation

For comprehensive DNS management details, refer to:
- [DNS Architecture Design](./docs/DNS_ARCHITECTURE.md) - System design and architecture documentation
- [DNS Setup](./docs/DNS_SETUP.md) - Step-by-step operational guide
- [DNS Troubleshooting](./docs/DNS_TROUBLESHOOTING.md)

## Cross-Account ECR Configuration

The infrastructure supports flexible ECR strategies for container image management with intelligent cross-account access.

### ECR Strategies

1. **Local Strategy** (`ecr_strategy: local`):
   - Creates ECR repositories in the same AWS account
   - Each environment manages its own container registry
   - Best for isolated environments or single-account setups

2. **Cross-Account Strategy** (`ecr_strategy: cross_account`):
   - Pulls container images from another AWS account's ECR
   - Ideal for dev→staging→prod promotion pipelines
   - Reduces image duplication and build costs

### Automated Cross-Account Setup

The web UI provides an intelligent dropdown for configuring cross-account ECR access:

1. **Automatic Discovery**: Lists all environments with local ECR repositories
2. **Deployment Status**: Shows whether trust policies are deployed to AWS
3. **Bidirectional Updates**: Automatically updates both source and target YAML files
4. **Trust Policy Management**: Creates ECR repository policies for cross-account pull access

### Configuration Workflow

#### Via Web UI (Recommended)

1. Navigate to the ECR configuration node in the web interface
2. Select "Use Cross-Account ECR" mode
3. Choose a source environment from the dropdown
4. The system will automatically:
   - Update the target environment to use cross-account mode
   - Add the target to the source's trusted accounts list
   - Create timestamped backups of both YAML files
5. Follow the displayed next steps to deploy

#### Manual Configuration

In `project/staging.yaml` (target environment):
```yaml
schema_version: 8
ecr_strategy: cross_account
ecr_account_id: "123456789012"  # Source account ID
ecr_account_region: us-east-1   # Source region
```

In `project/dev.yaml` (source environment):
```yaml
schema_version: 8
ecr_strategy: local
ecr_trusted_accounts:
  - account_id: "987654321098"  # Target account ID
    env: staging
    region: us-east-1
```

### Deployment Status Indicators

The web UI shows real-time deployment status:

- **Warning** (Yellow): Trust policy configured but not deployed to AWS
  - Action: Run `task infra-apply env=<source>`
- **Success** (Green): Trust policy deployed and cross-account access ready
- **Info** (Blue): Configuration changes will update both YAML files

### ECR Trust Policies

When `ecr_trusted_accounts` is configured, Terraform creates repository policies allowing pull access:

```hcl
# Automatically generated in modules/workloads/ecr.tf
resource "aws_ecr_repository_policy" "backend_trusted" {
  # Allows same-account full access
  # Allows cross-account pull-only access for trusted accounts
}
```

### CI/CD with Cross-Account ECR

The recommended deployment pattern uses EventBridge for event-driven deployments:

1. **Dev environment**: Builds and pushes images to local ECR
2. **EventBridge event**: Triggers deployment pipeline
3. **Staging/Prod**: Pulls images from dev account's ECR
4. **Deployment**: ECS services use cross-account images

See [CI/CD EventBridge Pattern](./docs/CI_CD_EVENTBRIDGE_PATTERN.md) for detailed implementation.

### Security Considerations

- Trust policies grant **pull-only access** (no push permissions)
- Access is scoped to specific AWS accounts and regions
- IAM permissions follow least-privilege principle
- All configuration changes create timestamped backups

### Troubleshooting

**Issue**: Cross-account ECR pull fails
- Verify trust policy is deployed: Check web UI deployment status
- Confirm account IDs match: Compare source and target YAML files
- Check ECS task role: Ensure it has ECR pull permissions

**Issue**: No ECR sources in dropdown
- Verify at least one environment has `ecr_strategy: local`
- Ensure environments are configured with `account_id` and `region`
- Check that YAML files are in the project directory

## YAML Schema Migration System

The infrastructure includes an automatic migration system for YAML configuration files to ensure backward compatibility as the schema evolves.

### Migration Features

- **Automatic Migration**: YAML files are automatically migrated when loaded
- **Backup Creation**: Creates timestamped backups before making changes
- **Version Tracking**: Tracks schema version in `schema_version` field
- **Safe Migrations**: Only adds new fields, never modifies existing data

### Migration Commands

```bash
# Migrate all YAML files in project directory
./meroku migrate all

# Migrate a specific file
./meroku migrate dev.yaml

# Show migration help and current version
./meroku migrate
```

### Current Schema Version

**The authority is `CurrentSchemaVersion` in `app/migrations.go`.** Read it there.
The per-version history is the comment block directly above that constant, and
`AllMigrations` below it is the list the migrator actually runs — the three move
together in one commit, so none of them can be stale while the others are right.

This section used to carry the number and a copied-out feature list instead. It
said "Version 8" for twenty versions, because a number transcribed into prose has
no way to be loudly wrong: nothing reads it, nothing tests it, and bumping the
constant does not touch it. Do not put the number back. Adding a migration means
editing `app/migrations.go` (constant, history comment, `AllMigrations`) and the
table in [ai_docs/MIGRATIONS.md](./ai_docs/MIGRATIONS.md) — not this file.

### How It Works

When you load a YAML file, the system:
1. Detects the current schema version
2. Creates a timestamped backup if migration needed
3. Applies all necessary migrations sequentially
4. Updates the `schema_version` field
5. Saves the migrated file

Example backup: `dev.yaml.backup_20251015_211246`

### Documentation

For detailed migration information, refer to:
- [YAML Schema Migrations](./ai_docs/MIGRATIONS.md) - Complete migration system documentation

## Security Considerations

- Never commit secrets or credentials
- Use AWS SSM Parameter Store for sensitive values
- Security groups control service access (no interface VPC endpoints needed)
- Enable encryption at rest for all data stores
- Use IAM roles for service authentication

## Cost Optimization

- Interface VPC endpoints are not used; the S3 gateway endpoint is free
- Stay on `egress_strategy: public_ip` until the egress advisor says a NAT is cheaper
- Use appropriate instance sizes for ECS tasks
- Enable auto-scaling where appropriate
- CloudWatch log retention: 7 days for backend, services, scheduled tasks, the CI Lambda and X-Ray;
  1 day for pgAdmin; 30 days for the domain log group (`modules/workloads/domain.tf`)

## Releases

A version bump creates a git tag and changes the content of `version.txt`. `task version` prints it.

## Layout reminder

The meroku app is in `app/`; the web interface it serves is built from `web/`.

## Code Search

Added by the `code-search` plugin.

### The tool surface

| Tool | Present | Takes |
|---|---|---|
| `code_search` | always | `query` (free-form), `intent`, `scope` |
| `find_dependencies`, `find_dependents`, `call_tree`, `find_implementations`, `impact` | only when the configured engine genuinely supports that operation | `symbol`, `depth` / `max_depth`, `scope` |
| `Read`, `Grep`, `Glob` | always | — |

**Read the tool list; never assume a structural tool exists.** Absence means the configured
engine cannot answer that class of question at all — not that it would be slower or
approximate. Say so in the report and answer with what is present.

`code_search` infers intent from the query, so write the real question. Pass `intent` only to
override an inference you have watched go wrong. Every response names the capability that
served it — read that field.

### Routing

| The request is about | Route to |
|---|---|
| meaning, behaviour or a flow | `code_search` |
| the shape of the system, or a **named** symbol and what touches it | `code_search`, then the structural tools |
| an exact literal, an occurrence count, or a filename pattern | `Grep` / `Glob` |

Text search is the correct tool for the third row — that is routing, not a fallback. The rule
is to name the method behind each finding, never to avoid a tool.

### Discipline

- Search before reading. Three or more files in one investigation means one ranked query first.
- Read only the line ranges returned, and re-resolve them before acting — edits move them.
- Never rank-truncate a result set; narrow with `scope` instead.
- An error and an empty result look alike and mean opposite things. Establish which you have.
- Centrality is tiers, not a threshold. Absent centrality means unknown, not low.
