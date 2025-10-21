# V2 Architecture: Direct Resource Naming

## The Problem with Pattern-Based Naming

### Original Approach (Fragile)

The Lambda function constructed ECS resource names using string patterns:

```go
// Lambda code
clusterName := fmt.Sprintf("%s_cluster_%s", projectName, env)
serviceName := fmt.Sprintf("%s_service_%s_%s", projectName, name, env)
```

**Problems:**
1. **Pattern Mismatch Risk** - If Terraform changes naming, Lambda breaks
2. **Not Testable** - Can't verify names are correct until runtime
3. **Debugging Nightmare** - "Service not found" - is it pattern wrong? Typo? Service doesn't exist?
4. **Inflexible** - Locked into one naming convention

### Example Failure Scenario

```
Terraform creates: "myproject_service_api_dev"
Lambda constructs: "myproject-service-api-dev"  # Dash instead of underscore!

Result: "Service not found" error in production
```

## The Solution: Terraform Tells Lambda Exact Names

### New Approach (Robust)

Terraform passes **actual resource names** it created:

```hcl
# lambda.tf
environment {
  variables = {
    ECS_CLUSTER_NAME = aws_ecs_cluster.main.name
    ECS_SERVICE_MAP = jsonencode({
      "" = {
        service_name = aws_ecs_service.backend.name
        task_family  = aws_ecs_task_definition.backend.family
      }
      "api" = {
        service_name = aws_ecs_service.services["api"].name
        task_family  = aws_ecs_task_definition.services["api"].family
      }
    })
  }
}
```

Lambda does simple lookup:

```go
// Lambda code
clusterName := config.GetClusterName()  // Direct lookup
serviceName, _ := config.GetServiceName("api")  // Direct lookup
```

## Architecture Comparison

### V1: Pattern-Based (Original)

```
┌─────────────┐
│  Terraform  │
└──────┬──────┘
       │
       │ Creates resources with names
       │
       ▼
┌─────────────────────────────────┐
│  AWS ECS                        │
│  • myproject_cluster_dev        │
│  • myproject_service_dev        │
│  • myproject_service_api_dev    │
└─────────────────────────────────┘
       ▲
       │
       │ Constructs names using patterns
       │
┌──────┴──────┐
│   Lambda    │  ❌ Risk: Pattern mismatch
└─────────────┘
```

### V2: Direct Naming (New)

```
┌─────────────┐
│  Terraform  │
└──────┬──────┘
       │
       │ Creates resources AND
       │ tells Lambda exact names
       │
       ├─────────────────────────────┐
       ▼                             ▼
┌─────────────────────────────────┐ │
│  AWS ECS                        │ │
│  • myproject_cluster_dev        │ │
│  • myproject_service_dev        │ │
│  • myproject_service_api_dev    │ │
└─────────────────────────────────┘ │
       ▲                             │
       │                             │
       │ Uses exact names            │
       │                             │
┌──────┴──────┐                     │
│   Lambda    │ ◄───────────────────┘
└─────────────┘     ECS_SERVICE_MAP
       ✅ Zero risk: Uses exact names
```

## Data Flow

### Environment Variable Structure

**ECS_CLUSTER_NAME** (string):
```
"myproject_cluster_dev"
```

**ECS_SERVICE_MAP** (JSON):
```json
{
  "": {
    "service_name": "myproject_service_dev",
    "task_family": "myproject_service_dev"
  },
  "api": {
    "service_name": "myproject_service_api_dev",
    "task_family": "myproject_service_api_dev"
  },
  "worker": {
    "service_name": "myproject_service_worker_dev",
    "task_family": "myproject_service_worker_dev"
  }
}
```

**S3_SERVICE_MAP** (JSON):
```json
{
  "api": [
    {
      "bucket": "myproject-env-dev",
      "key": "api/.env"
    }
  ],
  "worker": [
    {
      "bucket": "myproject-env-dev",
      "key": "worker/.env"
    }
  ]
}
```

### Service Identifier Flow

```
ECR Event
   ↓
Repository: "myproject_service_api"
   ↓
Extract service ID: "api"
   ↓
Lookup in ECS_SERVICE_MAP["api"]
   ↓
Get actual names:
  - service_name: "myproject_service_api_dev"
  - task_family: "myproject_service_api_dev"
   ↓
Deploy to ECS using actual names
```

## Key Benefits

### 1. Guaranteed Correctness
```
✅ Terraform creates resource
✅ Terraform tells Lambda the name
✅ Lambda uses exact name
❌ Pattern mismatch impossible
```

### 2. Integration Testing
```bash
# Test BEFORE deploying to production
make integration-test

# Output:
✅ Checking service: api → myproject_service_api_dev... OK
✅ Checking service: worker → myproject_service_worker_dev... OK
```

### 3. Flexible Naming Conventions
```hcl
# V1: Locked into pattern
service_name = "${var.project}_service_${var.name}_${var.env}"

# V2: Use any naming you want
service_name = "${var.env}-${var.name}-svc"  # Works!
service_name = "svc-${var.name}-${var.env}"  # Works!
service_name = "${var.name}.${var.env}"      # Works!
```

### 4. Clear Error Messages
```
# V1: Confusing
Error: Service not found: myproject-service-api-dev
# Is the pattern wrong? Typo? Service doesn't exist?

# V2: Clear
Error: service 'api' not found in ECS_SERVICE_MAP
# Missing from config - add to Terraform
```

## Code Organization

### New Files

**config/config_v2.go** - Configuration with direct lookups
```go
type Config struct {
    ClusterName string
    ServiceMap  map[string]ServiceMapping
    S3ToServiceMap map[string][]S3ServiceFile
}

func (c *Config) GetServiceName(id string) (string, error)
func (c *Config) GetTaskFamily(id string) (string, error)
func (c *Config) GetServicesForS3File(bucket, key string) []string
```

**services/ecs_v2.go** - ECS operations with direct names
```go
type ECSServiceV2 struct {
    client *ecs.ECS
    config *config.Config
}

func (s *ECSServiceV2) Deploy(req DeploymentRequest) (*DeploymentResult, error)
func (s *ECSServiceV2) VerifyServiceExists(id string) error
```

**cmd/integration_test.go** - Integration test command
```go
func main() {
    // 1. Load config
    // 2. Connect to ECS
    // 3. Verify all services exist
    // 4. Report results
}
```

### File Purpose

| File | Purpose |
|------|---------|
| `config/config_v2.go` | Load and validate actual resource names |
| `services/ecs_v2.go` | Deploy using actual names |
| `handlers/s3_v2.go` | S3 events with direct service lookup |
| `cmd/integration_test.go` | Verify all services exist |
| `Makefile` | Build and test commands |

## Migration Path

### Phase 1: Add V2 Alongside V1 (Current)

✅ New files created (`*_v2.go`)
✅ Old files remain unchanged
✅ Both approaches available

### Phase 2: Test V2 Thoroughly

```bash
# 1. Build integration test
make build-test

# 2. Run against dev
AWS_PROFILE=dev make integration-test

# 3. Run against staging
AWS_PROFILE=staging make integration-test
```

### Phase 3: Switch to V2

```go
// main.go
// Before:
ecsSvc, err := services.NewECSService(cfg, logger)

// After:
ecsSvc, err := services.NewECSServiceV2(cfg, logger)
```

### Phase 4: Remove V1 Code

After V2 proven in production:
- Delete `config/config.go`
- Delete `services/ecs.go`
- Rename `*_v2.go` → `*.go`

## Testing Strategy

### Unit Tests
```go
func TestConfigLoad(t *testing.T) {
    os.Setenv("ECS_SERVICE_MAP", `{"api": {"service_name": "test"}}`)
    cfg, err := config.LoadFromEnv()
    assert.NoError(t, err)
    assert.Equal(t, "test", cfg.ServiceMap["api"].ServiceName)
}
```

### Integration Tests
```bash
# Verify all services exist in ECS
./integration_test

# Expected output:
✅ ALL INTEGRATION TESTS PASSED!
```

### E2E Tests
```bash
# 1. Push Docker image to ECR
docker push ...

# 2. Lambda should auto-deploy
# 3. Verify deployment in ECS
aws ecs describe-services ...
```

## Operational Benefits

### Deployment Confidence
```
Before: 😰 Hope the pattern is correct
After:  😎 Integration test verified it works
```

### Debugging Speed
```
Before: 30 min to figure out pattern mismatch
After:  Instant - integration test shows exact error
```

### Flexibility
```
Before: Change naming → Update pattern in Lambda code
After:  Change naming → Terraform auto-updates Lambda config
```

## Real-World Example

### Scenario: Rename Service Convention

**Before (V1)**:
```hcl
# Change Terraform naming
resource "aws_ecs_service" "services" {
  name = "${var.project}-${each.key}-service-${var.env}"  # New naming!
}
```

Lambda breaks because pattern still says:
```go
serviceName := fmt.Sprintf("%s_service_%s_%s", project, name, env)
// Still constructs: myproject_service_api_dev
// But Terraform created: myproject-api-service-dev
```

**After (V2)**:
```hcl
# Change Terraform naming
resource "aws_ecs_service" "services" {
  name = "${var.project}-${each.key}-service-${var.env}"  # New naming!
}

# Lambda automatically gets new name
locals {
  ecs_service_map = jsonencode({
    "api" = {
      service_name = aws_ecs_service.services["api"].name  # myproject-api-service-dev
    }
  })
}
```

Lambda works immediately because it uses exact name from Terraform!

## Summary

| Aspect | V1 (Pattern) | V2 (Direct) |
|--------|--------------|-------------|
| **Reliability** | ❌ Pattern can mismatch | ✅ Uses exact names |
| **Testability** | ❌ Only at runtime | ✅ Integration test |
| **Flexibility** | ❌ Locked to pattern | ✅ Any naming works |
| **Debugging** | ❌ Confusing errors | ✅ Clear errors |
| **Confidence** | 😰 Hope it works | 😎 Know it works |

## Conclusion

**V2 Architecture eliminates an entire class of bugs by using actual resource names from Terraform instead of constructing them from patterns.**

This is the **"ultra-thinking"** approach you requested:
1. ✅ We know exact ECS cluster name → Use it directly
2. ✅ We know exact service names → Use them directly
3. ✅ We can test before deploying → Integration test
4. ✅ No pattern errors possible → Guaranteed correctness

**Result**: Bullet-proof deployment system that can't fail due to naming issues.
