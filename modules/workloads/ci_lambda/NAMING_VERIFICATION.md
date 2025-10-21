# Naming Pattern Verification

## Terraform Resource Naming (Source of Truth)

From the actual Terraform files:

1. **ECS Cluster** (`modules/workloads/main.tf:2`):
   ```hcl
   name = "${var.project}_cluster_${var.env}"
   ```
   **Pattern**: `{project}_cluster_{env}`

2. **Named ECS Services** (`modules/workloads/services.tf:92`):
   ```hcl
   name = "${var.project}_service_${each.key}_${var.env}"
   ```
   **Pattern**: `{project}_service_{name}_{env}`

3. **Backend ECS Service** (`modules/workloads/backend.tf:2`):
   ```hcl
   local.backend_name = "${var.project}_service_${var.env}"
   ```
   **Pattern**: `{project}_service_{env}` (no `{name}` placeholder!)

4. **Named Task Definitions** (`modules/workloads/services.tf:138`):
   ```hcl
   family = "${var.project}_service_${each.key}_${var.env}"
   ```
   **Pattern**: `{project}_service_{name}_{env}`

5. **Backend Task Definition** (`modules/workloads/backend.tf:89`):
   ```hcl
   family = local.backend_name  # = "${var.project}_service_${var.env}"
   ```
   **Pattern**: `{project}_service_{env}` (no `{name}` placeholder!)

## Lambda Environment Variables (lambda.tf:61-63)

```hcl
CLUSTER_NAME_PATTERN  = "{project}_cluster_{env}"
SERVICE_NAME_PATTERN  = "{project}_service_{name}_{env}"
TASK_FAMILY_PATTERN   = "{project}_service_{name}_{env}"
BACKEND_SERVICE_NAME  = ""
```

## Pattern Application Logic

The key is in `config.go:applyPattern()`:

### For Backend Service (serviceName = "" or "backend"):
1. Replace `{project}` → actual project name
2. Replace `{env}` → actual environment
3. **Remove all `{name}` variations** (`_{name}`, `{name}_`, `{name}`)
4. Replace `{tag}` if present

**Example:**
```
Pattern:  {project}_service_{name}_{env}
Project:  myproject
Env:      dev
Service:  "" (backend)

Steps:
1. {project} → myproject_service_{name}_{env}
2. {env}     → myproject_service_{name}_dev
3. Remove _{name} → myproject_service_dev ✅ CORRECT!
```

### For Named Services (serviceName = "api", "worker", etc.):
1. Replace `{project}` → actual project name
2. Replace `{env}` → actual environment
3. **Replace `{name}` → actual service name**
4. Replace `{tag}` if present

**Example:**
```
Pattern:  {project}_service_{name}_{env}
Project:  myproject
Env:      dev
Service:  api

Steps:
1. {project} → myproject_service_{name}_{env}
2. {env}     → myproject_service_{name}_dev
3. {name}    → myproject_service_api_dev ✅ CORRECT!
```

## Verification Tests

### Test 1: Backend Service Name
```
Input:    serviceName = ""
Pattern:  {project}_service_{name}_{env}
Expected: myproject_service_dev
Result:   myproject_service_dev ✅
```

### Test 2: Backend Task Family
```
Input:    serviceName = ""
Pattern:  {project}_service_{name}_{env}
Expected: myproject_service_dev
Result:   myproject_service_dev ✅
```

### Test 3: Named Service (api)
```
Input:    serviceName = "api"
Pattern:  {project}_service_{name}_{env}
Expected: myproject_service_api_dev
Result:   myproject_service_api_dev ✅
```

### Test 4: Named Service (worker_queue)
```
Input:    serviceName = "worker_queue"
Pattern:  {project}_service_{name}_{env}
Expected: myproject_service_worker_queue_dev
Result:   myproject_service_worker_queue_dev ✅
```

### Test 5: Cluster Name (backend)
```
Input:    serviceName = ""
Pattern:  {project}_cluster_{env}
Expected: myproject_cluster_dev
Result:   myproject_cluster_dev ✅
```

### Test 6: Cluster Name (named service)
```
Input:    serviceName = "api"
Pattern:  {project}_cluster_{env}
Expected: myproject_cluster_dev
Result:   myproject_cluster_dev ✅
```

## Edge Cases

### Backend Service Aliases
The backend service can be identified by:
- Empty string `""`
- `"backend"`
- `BACKEND_SERVICE_NAME` environment variable

All produce the same result:
```
{project}_service_{env}
```

### Service Name with Underscores
Service names can contain underscores (e.g., `worker_queue`):
```
Input:    serviceName = "worker_queue"
Pattern:  {project}_service_{name}_{env}
Result:   myproject_service_worker_queue_dev ✅
```

### Custom Naming Patterns
Users can override patterns if they have different conventions:
```hcl
# Non-standard naming (example)
CLUSTER_NAME_PATTERN  = "{project}-{env}-cluster"
SERVICE_NAME_PATTERN  = "{env}-{project}-{name}-svc"
TASK_FAMILY_PATTERN   = "{project}-{name}-task-{env}"
```

Results:
- Cluster:  `myproject-dev-cluster`
- Service:  `dev-myproject-api-svc`
- Task:     `myproject-api-task-dev`

## Conclusion

✅ **All naming patterns are VERIFIED and CORRECT** after the fix to `config.go:applyPattern()`.

The patterns in `lambda.tf` accurately reflect the Terraform resource naming conventions, and the Go code correctly handles both backend and named services.

## Bug Fixed

**Original Bug (FIXED):**
The code was incorrectly replacing `_service_` with `_` for backend services, which would have produced `myproject_dev` instead of `myproject_service_dev`.

**Fix Applied:**
Now correctly removes only the `{name}` placeholder variations, preserving `_service_` in the pattern.

**Commit Message:**
```
fix(lambda): Correct backend service naming pattern application

Backend services should be named {project}_service_{env}, not {project}_{env}.
Updated applyPattern() to remove {name} placeholder without removing _service_ prefix.
```
