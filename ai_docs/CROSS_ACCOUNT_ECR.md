# Cross-Account ECR Dropdown Feature

**Status**: ✅ Implemented (Schema v8)

## Overview

Intelligent dropdown for cross-account ECR configuration that automatically discovers available ECR sources, manages bidirectional YAML updates, and provides deployment status indicators.

## Problem Analysis

### Current Gap

**What works:**
- ✅ Consumer account gets IAM policy to pull images
- ✅ UI validation for account ID and region
- ✅ YAML save/load mechanism

**What's missing:**
- ❌ Source ECR repository policy doesn't allow consumer account
- ❌ Manual entry is error-prone (copy-paste account IDs)
- ❌ No visibility into which environments have ECR repositories
- ❌ Source environment doesn't track which accounts have access

**For cross-account ECR to work, you need BOTH sides:**
1. **Consumer side**: IAM policy allowing ECS tasks to pull from external ECR
2. **Source side**: ECR repository policy allowing external accounts to pull (MISSING!)

## Proposed Solution

### 1. Schema Changes (v8)

Add new field to track which accounts can access this environment's ECR:

```yaml
# dev.yaml (source environment with ECR)
project: myapp
env: dev
ecr_strategy: local
ecr_trusted_accounts:  # NEW FIELD - Schema v8
  - account_id: "234567890123"
    env: staging
    region: ap-southeast-2
  - account_id: "345678901234"
    env: prod
    region: us-east-1

# staging.yaml (consumer environment)
project: myapp
env: staging
ecr_strategy: cross_account
ecr_account_id: "123456789012"      # Points to dev account
ecr_account_region: ap-southeast-2   # Points to dev region
```

### 2. UI Enhancement

Replace manual text input with intelligent dropdown:

```
┌─────────────────────────────────────────────────┐
│ ⚙️  ECR Configuration                            │
├─────────────────────────────────────────────────┤
│                                                 │
│ ○ Create ECR Repository                        │
│   Create a new ECR repository in this account  │
│                                                 │
│ ● Use Cross-Account ECR                        │
│   Use ECR from another environment             │
│                                                 │
│   Source Environment: [dev ▼]                  │
│   ┌───────────────────────────────────────┐   │
│   │ dev (123456789012, ap-southeast-2)    │   │
│   │ prod (234567890123, us-east-1)        │   │
│   └───────────────────────────────────────┘   │
│                                                 │
│   ℹ️  This will update dev.yaml to grant       │
│      access from staging environment           │
│                                                 │
│   📊 ECR Repository URL:                       │
│   123456789012.dkr.ecr.ap-southeast-2.         │
│   amazonaws.com/myapp_backend                  │
│                                                 │
└─────────────────────────────────────────────────┘
```

**Dropdown shows:**
- Environment name (e.g., "dev")
- Account ID (e.g., "123456789012")
- Region (e.g., "ap-southeast-2")
- Only environments with `ecr_strategy: local`

### 3. API Endpoints

#### GET /api/environments/ecr-sources

Returns list of environments with local ECR repositories.

**Response:**
```json
{
  "sources": [
    {
      "name": "dev",
      "account_id": "123456789012",
      "region": "ap-southeast-2",
      "ecr_strategy": "local",
      "trusted_accounts": [
        {
          "account_id": "234567890123",
          "env": "staging",
          "region": "ap-southeast-2"
        }
      ]
    },
    {
      "name": "prod",
      "account_id": "234567890123",
      "region": "us-east-1",
      "ecr_strategy": "local",
      "trusted_accounts": []
    }
  ]
}
```

#### POST /api/environments/configure-cross-account-ecr

Configures cross-account ECR access bidirectionally.

**Request:**
```json
{
  "source_env": "dev",
  "target_env": "staging"
}
```

**Actions:**
1. Load `staging.yaml` and update:
   - `ecr_strategy: "cross_account"`
   - `ecr_account_id: "<dev_account_id>"`
   - `ecr_account_region: "<dev_region>"`
2. Load `dev.yaml` and update:
   - Add staging to `ecr_trusted_accounts[]`
3. Save both files
4. Create backups before modification

**Response:**
```json
{
  "success": true,
  "modified_files": ["staging.yaml", "dev.yaml"],
  "source_env": {
    "name": "dev",
    "account_id": "123456789012",
    "region": "ap-southeast-2"
  },
  "target_env": {
    "name": "staging",
    "account_id": "234567890123",
    "region": "ap-southeast-2"
  },
  "next_steps": [
    "Apply changes to source: make infra-apply env=dev",
    "Apply changes to target: make infra-apply env=staging",
    "Push images to: 123456789012.dkr.ecr.ap-southeast-2.amazonaws.com/myapp_backend"
  ]
}
```

### 4. Terraform Module Enhancement

Add ECR repository policy to allow cross-account access.

**File:** `modules/workloads/ecr.tf`

```hcl
# ECR Repository Policy allowing cross-account access
data "aws_iam_policy_document" "ecr_repository_policy" {
  count = var.ecr_strategy == "local" && length(var.ecr_trusted_accounts) > 0 ? 1 : 0

  statement {
    sid    = "AllowCrossAccountPull"
    effect = "Allow"

    principals {
      type        = "AWS"
      identifiers = [for acct in var.ecr_trusted_accounts : "arn:aws:iam::${acct.account_id}:root"]
    }

    actions = [
      "ecr:GetAuthorizationToken",
      "ecr:BatchCheckLayerAvailability",
      "ecr:BatchGetImage",
      "ecr:GetDownloadUrlForLayer",
      "ecr:DescribeImages",
      "ecr:DescribeRepositories"
    ]
  }
}

# Apply policy to backend repository
resource "aws_ecr_repository_policy" "backend" {
  count      = var.ecr_strategy == "local" && length(var.ecr_trusted_accounts) > 0 ? 1 : 0
  repository = aws_ecr_repository.backend[0].name
  policy     = data.aws_iam_policy_document.ecr_repository_policy[0].json
}

# Apply policy to service repositories
resource "aws_ecr_repository_policy" "services" {
  for_each   = var.ecr_strategy == "local" && length(var.ecr_trusted_accounts) > 0 ? { for s in var.services : s.name => s } : {}
  repository = aws_ecr_repository.services[each.key].name
  policy     = data.aws_iam_policy_document.ecr_repository_policy[0].json
}

# Apply policy to task repositories
resource "aws_ecr_repository_policy" "tasks" {
  for_each   = var.ecr_strategy == "local" && length(var.ecr_trusted_accounts) > 0 ? { for t in var.tasks : t.name => t } : {}
  repository = aws_ecr_repository.tasks[each.key].name
  policy     = data.aws_iam_policy_document.ecr_repository_policy[0].json
}
```

**File:** `modules/workloads/variables.tf`

```hcl
variable "ecr_trusted_accounts" {
  description = "List of AWS accounts allowed to pull from this environment's ECR repositories"
  type = list(object({
    account_id = string
    env        = string
    region     = string
  }))
  default = []
}
```

**File:** `templates/workloads.tf.hbs`

```hbs
module "workloads" {
  source = "../../modules/workloads"

  # ... other variables ...

  ecr_strategy         = "{{ecr_strategy}}"
  ecr_account_id       = "{{ecr_account_id}}"
  ecr_account_region   = "{{ecr_account_region}}"

  {{#if ecr_trusted_accounts}}
  ecr_trusted_accounts = [
    {{#each ecr_trusted_accounts}}
    {
      account_id = "{{account_id}}"
      env        = "{{env}}"
      region     = "{{region}}"
    },
    {{/each}}
  ]
  {{else}}
  ecr_trusted_accounts = []
  {{/if}}
}
```

### 5. Go Backend Implementation

#### Model Changes

**File:** `app/model.go`

```go
type ECRTrustedAccount struct {
    AccountID string `yaml:"account_id"`
    Env       string `yaml:"env"`
    Region    string `yaml:"region"`
}

type Env struct {
    // ... existing fields ...

    // ECR Configuration (Schema v7)
    ECRStrategy      string `yaml:"ecr_strategy,omitempty"`
    ECRAccountID     string `yaml:"ecr_account_id,omitempty"`
    ECRAccountRegion string `yaml:"ecr_account_region,omitempty"`

    // ECR Trusted Accounts (Schema v8)
    ECRTrustedAccounts []ECRTrustedAccount `yaml:"ecr_trusted_accounts,omitempty"`

    // ... other fields ...
}
```

#### API Handlers

**File:** `app/api.go`

```go
// GET /api/environments/ecr-sources
func getECRSources(w http.ResponseWriter, r *http.Request) {
    projectDir := "project"
    files, err := os.ReadDir(projectDir)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    var sources []map[string]interface{}

    for _, file := range files {
        if !strings.HasSuffix(file.Name(), ".yaml") {
            continue
        }

        data, err := os.ReadFile(filepath.Join(projectDir, file.Name()))
        if err != nil {
            continue
        }

        var env Env
        if err := yaml.Unmarshal(data, &env); err != nil {
            continue
        }

        // Only include environments with local ECR strategy
        if env.ECRStrategy == "local" {
            sources = append(sources, map[string]interface{}{
                "name":             env.Env,
                "account_id":       env.AccountID,
                "region":           env.Region,
                "ecr_strategy":     env.ECRStrategy,
                "trusted_accounts": env.ECRTrustedAccounts,
            })
        }
    }

    json.NewEncoder(w).Encode(map[string]interface{}{
        "sources": sources,
    })
}

// POST /api/environments/configure-cross-account-ecr
func configureCrossAccountECR(w http.ResponseWriter, r *http.Request) {
    var req struct {
        SourceEnv string `json:"source_env"`
        TargetEnv string `json:"target_env"`
    }

    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    projectDir := "project"

    // Load source environment
    sourceFile := filepath.Join(projectDir, req.SourceEnv+".yaml")
    sourceData, err := os.ReadFile(sourceFile)
    if err != nil {
        http.Error(w, "Source environment not found", http.StatusNotFound)
        return
    }

    var sourceEnv Env
    if err := yaml.Unmarshal(sourceData, &sourceEnv); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    if sourceEnv.ECRStrategy != "local" {
        http.Error(w, "Source environment does not have local ECR", http.StatusBadRequest)
        return
    }

    // Load target environment
    targetFile := filepath.Join(projectDir, req.TargetEnv+".yaml")
    targetData, err := os.ReadFile(targetFile)
    if err != nil {
        http.Error(w, "Target environment not found", http.StatusNotFound)
        return
    }

    var targetEnv Env
    if err := yaml.Unmarshal(targetData, &targetEnv); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // Create backups
    timestamp := time.Now().Format("20060102_150405")
    os.WriteFile(sourceFile+".backup_"+timestamp, sourceData, 0644)
    os.WriteFile(targetFile+".backup_"+timestamp, targetData, 0644)

    // Update target environment
    targetEnv.ECRStrategy = "cross_account"
    targetEnv.ECRAccountID = sourceEnv.AccountID
    targetEnv.ECRAccountRegion = sourceEnv.Region

    // Update source environment trusted accounts
    alreadyTrusted := false
    for _, acct := range sourceEnv.ECRTrustedAccounts {
        if acct.AccountID == targetEnv.AccountID && acct.Env == targetEnv.Env {
            alreadyTrusted = true
            break
        }
    }

    if !alreadyTrusted {
        sourceEnv.ECRTrustedAccounts = append(sourceEnv.ECRTrustedAccounts, ECRTrustedAccount{
            AccountID: targetEnv.AccountID,
            Env:       targetEnv.Env,
            Region:    targetEnv.Region,
        })
    }

    // Save both environments
    if err := saveEnvToFile(sourceEnv, sourceFile); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    if err := saveEnvToFile(targetEnv, targetFile); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // Return success response
    response := map[string]interface{}{
        "success":        true,
        "modified_files": []string{req.TargetEnv + ".yaml", req.SourceEnv + ".yaml"},
        "source_env": map[string]string{
            "name":       sourceEnv.Env,
            "account_id": sourceEnv.AccountID,
            "region":     sourceEnv.Region,
        },
        "target_env": map[string]string{
            "name":       targetEnv.Env,
            "account_id": targetEnv.AccountID,
            "region":     targetEnv.Region,
        },
        "next_steps": []string{
            fmt.Sprintf("Apply changes to source: make infra-apply env=%s", req.SourceEnv),
            fmt.Sprintf("Apply changes to target: make infra-apply env=%s", req.TargetEnv),
            fmt.Sprintf("Push images to: %s.dkr.ecr.%s.amazonaws.com/%s_backend",
                sourceEnv.AccountID, sourceEnv.Region, sourceEnv.Project),
        },
    }

    json.NewEncoder(w).Encode(response)
}
```

### 6. Schema Migration (v7 → v8)

**File:** `app/migrations.go`

```go
func migrateSchemaV7ToV8(env *Env) {
    // Initialize ECRTrustedAccounts if nil
    if env.ECRTrustedAccounts == nil {
        env.ECRTrustedAccounts = []ECRTrustedAccount{}
    }

    env.SchemaVersion = 8
}
```

### 7. React UI Implementation

**File:** `web/src/components/ECRNodeProperties.tsx`

```typescript
import { useState, useEffect } from 'react';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Info } from 'lucide-react';

interface ECRSource {
  name: string;
  account_id: string;
  region: string;
  ecr_strategy: string;
  trusted_accounts: Array<{
    account_id: string;
    env: string;
    region: string;
  }>;
}

export function ECRNodeProperties({ config, onConfigChange, accountInfo }: ECRNodePropertiesProps) {
  const [ecrMode, setEcrMode] = useState<'create' | 'cross-account'>(
    config.ecr_strategy === 'cross_account' ? 'cross-account' : 'create'
  );
  const [ecrSources, setEcrSources] = useState<ECRSource[]>([]);
  const [selectedSource, setSelectedSource] = useState<string>('');
  const [isConfiguring, setIsConfiguring] = useState(false);
  const [configResult, setConfigResult] = useState<any>(null);

  // Load available ECR sources
  useEffect(() => {
    fetch('/api/environments/ecr-sources')
      .then(res => res.json())
      .then(data => {
        setEcrSources(data.sources || []);

        // Pre-select current source if in cross-account mode
        if (config.ecr_strategy === 'cross_account' && config.ecr_account_id) {
          const currentSource = data.sources.find(
            (s: ECRSource) => s.account_id === config.ecr_account_id
          );
          if (currentSource) {
            setSelectedSource(currentSource.name);
          }
        }
      })
      .catch(err => console.error('Failed to load ECR sources:', err));
  }, [config.ecr_strategy, config.ecr_account_id]);

  const handleModeChange = (mode: string) => {
    const newMode = mode as 'create' | 'cross-account';
    setEcrMode(newMode);

    if (newMode === 'create') {
      onConfigChange({
        ...config,
        ecr_strategy: 'local',
        ecr_account_id: undefined,
        ecr_account_region: undefined,
      });
      setConfigResult(null);
    } else {
      onConfigChange({
        ...config,
        ecr_strategy: 'cross_account',
      });
    }
  };

  const handleSourceSelection = async (sourceName: string) => {
    setSelectedSource(sourceName);
    setIsConfiguring(true);
    setConfigResult(null);

    try {
      const response = await fetch('/api/environments/configure-cross-account-ecr', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          source_env: sourceName,
          target_env: config.env,
        }),
      });

      if (!response.ok) {
        throw new Error('Failed to configure cross-account ECR');
      }

      const result = await response.json();
      setConfigResult(result);

      // Update local config
      onConfigChange({
        ...config,
        ecr_strategy: 'cross_account',
        ecr_account_id: result.source_env.account_id,
        ecr_account_region: result.source_env.region,
      });
    } catch (error) {
      console.error('Configuration error:', error);
      alert('Failed to configure cross-account ECR. Check console for details.');
    } finally {
      setIsConfiguring(false);
    }
  };

  const selectedSourceData = ecrSources.find(s => s.name === selectedSource);

  return (
    <div className="space-y-4 p-4">
      <div>
        <h3 className="text-lg font-semibold mb-2">ECR Configuration</h3>
        <p className="text-sm text-muted-foreground mb-4">
          Configure container registry for your application
        </p>
      </div>

      {/* Radio Button Group */}
      <div className="space-y-3">
        {/* Create ECR Option */}
        <label className="flex items-start space-x-3 p-3 border rounded cursor-pointer hover:bg-accent/50">
          <input
            type="radio"
            name="ecr-mode"
            value="create"
            checked={ecrMode === 'create'}
            onChange={(e) => handleModeChange(e.target.value)}
            className="mt-1"
          />
          <div>
            <div className="font-medium">Create ECR Repository</div>
            <div className="text-sm text-muted-foreground">
              Create a new ECR repository in this AWS account
            </div>
          </div>
        </label>

        {/* Cross-Account ECR Option */}
        <label className="flex items-start space-x-3 p-3 border rounded cursor-pointer hover:bg-accent/50">
          <input
            type="radio"
            name="ecr-mode"
            value="cross-account"
            checked={ecrMode === 'cross-account'}
            onChange={(e) => handleModeChange(e.target.value)}
            className="mt-1"
          />
          <div className="flex-1">
            <div className="font-medium">Use Cross-Account ECR</div>
            <div className="text-sm text-muted-foreground mb-3">
              Use an ECR repository from another environment
            </div>

            {/* Dropdown appears when selected */}
            {ecrMode === 'cross-account' && (
              <div className="space-y-3 mt-3">
                <div>
                  <label className="text-sm font-medium mb-2 block">
                    Source Environment *
                  </label>
                  <Select
                    value={selectedSource}
                    onValueChange={handleSourceSelection}
                    disabled={isConfiguring}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="Select environment with ECR" />
                    </SelectTrigger>
                    <SelectContent>
                      {ecrSources.length === 0 && (
                        <SelectItem value="none" disabled>
                          No environments with local ECR found
                        </SelectItem>
                      )}
                      {ecrSources.map((source) => (
                        <SelectItem key={source.name} value={source.name}>
                          {source.name} ({source.account_id}, {source.region})
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                {/* Info Alert */}
                {selectedSourceData && (
                  <Alert>
                    <Info className="h-4 w-4" />
                    <AlertDescription>
                      This will update <strong>{selectedSource}.yaml</strong> to grant
                      access from <strong>{config.env}</strong> environment
                    </AlertDescription>
                  </Alert>
                )}

                {/* ECR URL Preview */}
                {selectedSourceData && (
                  <div className="p-3 bg-muted rounded text-sm">
                    <div className="font-medium mb-1">ECR Repository URL:</div>
                    <code className="text-xs break-all">
                      {selectedSourceData.account_id}.dkr.ecr.{selectedSourceData.region}.amazonaws.com/{config.project}_backend
                    </code>
                  </div>
                )}

                {/* Configuration Result */}
                {configResult && (
                  <Alert className="bg-green-50 border-green-200">
                    <AlertDescription>
                      <div className="font-medium mb-2">✅ Cross-Account ECR Configured</div>
                      <div className="text-sm space-y-1">
                        <div>Modified Files:</div>
                        <ul className="list-disc list-inside ml-2">
                          {configResult.modified_files.map((file: string) => (
                            <li key={file}>{file}</li>
                          ))}
                        </ul>
                        <div className="mt-2">Next Steps:</div>
                        <ol className="list-decimal list-inside ml-2">
                          {configResult.next_steps.map((step: string, idx: number) => (
                            <li key={idx}>{step}</li>
                          ))}
                        </ol>
                      </div>
                    </AlertDescription>
                  </Alert>
                )}
              </div>
            )}
          </div>
        </label>
      </div>
    </div>
  );
}
```

### 8. User Notification

After successful configuration, show a modal or notification:

```
┌─────────────────────────────────────────────────┐
│ ✅ Cross-Account ECR Configured                 │
├─────────────────────────────────────────────────┤
│                                                 │
│ Modified Files:                                 │
│   • staging.yaml - Cross-account ECR config    │
│   • dev.yaml - Granted access to staging       │
│                                                 │
│ Next Steps:                                     │
│   1. Apply source changes:                      │
│      make infra-apply env=dev                  │
│                                                 │
│   2. Apply target changes:                      │
│      make infra-apply env=staging              │
│                                                 │
│   3. Push images to:                            │
│      123456789012.dkr.ecr.ap-southeast-2.      │
│      amazonaws.com/myapp_backend               │
│                                                 │
│ [ View Changes ] [ Close ]                      │
└─────────────────────────────────────────────────┘
```

## Implementation Checklist

### Phase 1: Backend Foundation
- [ ] Add `ECRTrustedAccounts` field to `Env` struct in `app/model.go`
- [ ] Create schema migration v7→v8 in `app/migrations.go`
- [ ] Add `GET /api/environments/ecr-sources` endpoint
- [ ] Add `POST /api/environments/configure-cross-account-ecr` endpoint
- [ ] Test YAML bidirectional updates with backups

### Phase 2: Terraform Module
- [ ] Add `ecr_trusted_accounts` variable to `modules/workloads/variables.tf`
- [ ] Create ECR repository policy document in `modules/workloads/ecr.tf`
- [ ] Add repository policy resources for backend, services, tasks
- [ ] Update `templates/workloads.tf.hbs` to pass `ecr_trusted_accounts`
- [ ] Test Terraform plan with cross-account configuration

### Phase 3: Frontend UI
- [ ] Update TypeScript types in `web/src/types/yamlConfig.ts`
- [ ] Replace manual inputs with dropdown in `ECRNodeProperties.tsx`
- [ ] Add environment selection handler with API integration
- [ ] Implement loading states and error handling
- [ ] Add success notification with next steps
- [ ] Test UI flow from selection to notification

### Phase 4: Integration Testing
- [ ] Test dropdown loads available ECR sources
- [ ] Test environment selection updates both YAML files
- [ ] Test Terraform apply on source environment (creates repository policy)
- [ ] Test Terraform apply on target environment (creates IAM policy)
- [ ] Test actual cross-account ECR pull from ECS
- [ ] Verify backups are created before modifications

### Phase 5: Documentation
- [ ] Update `CLAUDE.md` with new ECR workflow
- [ ] Add migration notes for Schema v8
- [ ] Document new API endpoints
- [ ] Create troubleshooting guide for cross-account access
- [ ] Update `ai_docs/MIGRATIONS.md`

## Key Implementation Notes

### Bidirectional Update Pattern

Similar to DNS delegation but for ECR:

1. **Consumer side** (`staging.yaml`):
   - Sets `ecr_strategy: cross_account`
   - Points to source account and region
   - Gets IAM policy to pull images

2. **Source side** (`dev.yaml`):
   - Adds consumer to `ecr_trusted_accounts`
   - Creates ECR repository policy
   - Allows consumer account to pull

### API Response Structure

The `configure-cross-account-ecr` endpoint should return:
- Success status
- List of modified files
- Source and target environment details
- Clear next steps for user

### Error Handling

Handle these scenarios:
- Source environment not found
- Source doesn't have local ECR (`ecr_strategy != "local"`)
- Target environment not found
- YAML parsing errors
- File write errors
- Duplicate trust relationships (idempotent operation)

### Terraform Conditional Logic

Repository policy only created when:
```hcl
count = var.ecr_strategy == "local" && length(var.ecr_trusted_accounts) > 0 ? 1 : 0
```

This ensures:
- No policy created if no trusted accounts
- No policy created if using cross-account strategy
- Policy updates when trusted accounts list changes

## Architecture Comparison

### DNS Delegation Pattern (Active Access)
```
Subdomain Account → Assumes Role → Root Account → Modifies DNS
```

### ECR Cross-Account Pattern (Passive Access)
```
Consumer Account → IAM Policy → Source ECR Repository Policy → Allows Pull
```

**Key Difference:**
- DNS: Requires role assumption for active modification
- ECR: Uses resource-based policy for passive read access

## Testing Strategy

### Unit Tests
- YAML marshaling/unmarshaling with new fields
- Migration v7→v8 logic
- API endpoint request/response handling

### Integration Tests
1. Create dev environment with `ecr_strategy: local`
2. Create staging environment with `ecr_strategy: cross_account`
3. Call API to configure cross-account access
4. Verify both YAML files updated correctly
5. Apply Terraform to dev (creates repository policy)
6. Apply Terraform to staging (creates IAM policy)
7. Test actual ECR pull from staging ECS task

### Manual Tests
- UI dropdown shows correct environments
- Selection updates configuration immediately
- Notification shows correct next steps
- Files can be reverted from backups
- Multiple environments can trust same source

## Future Enhancements

1. **Bulk Configuration**: Select multiple target environments at once
2. **Access Removal**: UI to remove trust relationships
3. **Validation**: Pre-flight checks before applying Terraform
4. **Organization Detection**: Auto-detect if accounts are in same AWS Org
5. **Repository Browser**: Show which repositories exist in source account
6. **Cost Estimation**: Show ECR storage costs for shared repositories

## Related Files Reference

| Component | File Path | Lines |
|-----------|-----------|-------|
| **Current YAML Schema** | `app/model.go` | 11-41 |
| **Current ECR UI** | `web/src/components/ECRNodeProperties.tsx` | 18-179 |
| **ECR Terraform Module** | `modules/workloads/ecr.tf` | 1-74 |
| **Cross-Account IAM Policy** | `modules/workloads/backend.tf` | 428-496 |
| **Terraform Variables** | `modules/workloads/variables.tf` | 135-152 |
| **DNS Delegation Reference** | `modules/dns-root/main.tf` | 1-68 |
| **YAML Save Function** | `app/model.go` | 367-381 |
| **API Endpoints** | `app/api.go` | 102-176 |
| **Frontend Config Save** | `web/src/App.tsx` | 125-167 |
| **Migration System** | `app/migrations.go` | Full file |

## Questions to Resolve

1. Should we auto-apply Terraform changes, or require manual apply?
   - **Recommendation**: Manual apply for safety

2. What happens if source environment is deleted?
   - **Recommendation**: Add validation check before deletion

3. Should we support cross-region ECR replication?
   - **Recommendation**: Phase 2 enhancement

4. How to handle circular dependencies (A trusts B, B trusts A)?
   - **Recommendation**: Allow it, each is independent

5. Should we validate AWS Organizations membership?
   - **Recommendation**: Nice-to-have, not required for functionality

## Success Criteria

✅ User can see dropdown of available ECR sources
✅ Selecting source updates both YAML files automatically
✅ Source YAML gets `ecr_trusted_accounts` entry
✅ Target YAML gets cross-account ECR configuration
✅ Terraform creates ECR repository policy on source
✅ Terraform creates IAM policy on target
✅ ECS tasks can pull images from cross-account ECR
✅ User sees clear notification about what was changed
✅ Backups are created before modifications
✅ System is idempotent (re-running doesn't break)

## Timeline Estimate

- Phase 1 (Backend): 3-4 hours
- Phase 2 (Terraform): 2-3 hours
- Phase 3 (Frontend): 3-4 hours
- Phase 4 (Testing): 2-3 hours
- Phase 5 (Documentation): 1-2 hours

**Total: 11-16 hours**

---

## Implementation Summary

**Completed**: 2025-10-22

### What Was Implemented

1. **Schema v7→v8 Migration**
   - Added `ecr_trusted_accounts` field to YAML schema
   - Automatic migration with backup creation
   - Idempotent migration logic

2. **Backend Foundation (Go)**
   - `ECRTrustedAccount` model in `app/model.go`
   - Migration system update to v8 in `app/migrations.go`
   - API endpoints:
     - `GET /api/environments/ecr-sources` - Lists environments with local ECR
     - `POST /api/environments/configure-cross-account-ecr` - Configures cross-account access
   - Deployment status check via Terraform state file parsing
   - Bidirectional YAML update logic with backup creation

3. **Terraform Module Updates**
   - `ecr_trusted_accounts` variable in `modules/workloads/variables.tf`
   - ECR repository policy document for trusted accounts
   - Conditional resource creation based on trusted accounts array
   - Pull-only permissions for cross-account access
   - Same-account full access preserved
   - Template integration in `env/main.hbs`

4. **React Frontend (TypeScript)**
   - TypeScript type definitions for ECR sources and responses
   - API client functions for ECR operations
   - ECR component redesign with dropdown selector
   - Loading and error states
   - Deployment status indicators (Warning/Success)
   - Success notification with next steps
   - Automatic ECR URL preview

5. **Documentation**
   - EventBridge CI/CD pattern guide
   - CLAUDE.md cross-account ECR section
   - Migration documentation update
   - Implementation notes in this file

### Key Features

- **Automatic Discovery**: Scans all YAML files for local ECR environments
- **Deployment Status**: Checks Terraform state to show if policies are deployed
- **Bidirectional Updates**: Single action updates both source and target environments
- **Backup Safety**: Creates timestamped backups before modifications
- **User Guidance**: Clear next-step instructions after configuration
- **Idempotent**: Safe to re-run without breaking existing setup

### Files Modified

**Backend**:
- `app/model.go` - Added ECRTrustedAccount struct and field
- `app/migrations.go` - Added v8 migration
- `app/api.go` - Added ECR source and configuration endpoints
- `app/spa_server.go` - Registered new routes

**Terraform**:
- `modules/workloads/variables.tf` - Added ecr_trusted_accounts variable
- `modules/workloads/ecr.tf` - Added trust policy resources
- `modules/workloads/services.tf` - Updated ECR strategy logic
- `env/main.hbs` - Added template for ecr_trusted_accounts

**Frontend**:
- `web/src/types/yamlConfig.ts` - Added ecr_trusted_accounts type
- `web/src/api/infrastructure.ts` - Added ECR API functions and types
- `web/src/components/ECRNodeProperties.tsx` - Complete UI redesign

**Documentation**:
- `docs/CI_CD_EVENTBRIDGE_PATTERN.md` - New EventBridge deployment guide
- `CLAUDE.md` - Added cross-account ECR section
- `ai_docs/MIGRATIONS.md` - Updated with v8 migration details
- `ai_docs/CROSS_ACCOUNT_ECR.md` - Implementation documentation

### Testing Recommendations

1. **Migration Testing**
   ```bash
   # Test v7→v8 migration
   ./meroku migrate all
   ```

2. **API Testing**
   ```bash
   # Start web server and check endpoints
   ./meroku
   # Test: GET http://localhost:8080/api/environments/ecr-sources
   ```

3. **Terraform Testing**
   ```bash
   # Generate and plan
   make infra-gen-dev
   make infra-plan env=dev
   # Verify ecr_trusted_accounts parameter is present
   # Verify aws_ecr_repository_policy resources are conditional
   ```

4. **UI Testing**
   - Open web interface
   - Navigate to ECR configuration
   - Select cross-account mode
   - Verify dropdown populates
   - Test configuration flow
   - Check deployment status indicators

### Future Enhancements

- [ ] Support for removing trusted accounts via UI
- [ ] Bulk trust configuration for multiple environments
- [ ] ECR repository policy viewer in UI
- [ ] Cross-region ECR replication configuration
- [ ] Terraform plan preview before applying
- [ ] Automated testing of cross-account pull
