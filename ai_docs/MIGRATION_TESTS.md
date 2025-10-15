# Migration System Test Results

## Test Summary

All migration tests **PASS** ✅

Total Tests: **8 test suites** with **11 sub-tests**

```
PASS: TestDetectSchemaVersion (5 sub-tests)
PASS: TestMigrateToV2
PASS: TestMigrateToV3
PASS: TestMigrateToV4
PASS: TestMigrateToV5
PASS: TestApplyMigrationsChain
PASS: TestMigrationIdempotency
PASS: TestMigrateYAMLFileIntegration
PASS: TestMigrationPreservesExistingValues
```

## Test Coverage

### 1. Version Detection Tests (`TestDetectSchemaVersion`)

Tests the automatic detection of schema versions based on field presence.

**Sub-tests:**
- ✅ v1 - no version indicators
- ✅ v2 - has aurora
- ✅ v3 - has zone_id
- ✅ v4 - has backend_desired_count
- ✅ v5 - has account_id

**What it validates:**
- Correctly identifies v1 schemas (no version field)
- Detects v2 by presence of `aurora` field in postgres
- Detects v3 by presence of `zone_id` field in domain
- Detects v4 by presence of `backend_desired_count` in workload
- Detects v5 by presence of `account_id` at root level

### 2. Individual Migration Tests

Tests each migration function independently.

#### `TestMigrateToV2`
**What it validates:**
- Adds `aurora: false` to postgres section
- Adds `min_capacity: 0.5` to postgres section
- Adds `max_capacity: 1.0` to postgres section
- Adds `alb.enabled: false` configuration

#### `TestMigrateToV3`
**What it validates:**
- Adds `zone_id` to domain
- Adds `root_zone_id` to domain
- Adds `root_account_id` to domain
- Adds `is_dns_root: false` to domain
- Adds `dns_root_account_id` to domain
- Adds `delegation_role_arn` to domain
- Adds `api_domain_prefix` to domain
- Adds `add_env_domain_prefix: false` to domain

#### `TestMigrateToV4`
**What it validates:**
- Adds `backend_desired_count: 1` to workload
- Adds `backend_autoscaling_enabled: false` to workload
- Adds `backend_autoscaling_min_capacity: 1` to workload
- Adds `backend_autoscaling_max_capacity: 4` to workload
- Adds `backend_cpu: "256"` to workload
- Adds `backend_memory: "512"` to workload
- Adds `backend_alb_domain_name: ""` to workload

#### `TestMigrateToV5`
**What it validates:**
- Adds `account_id: ""` at root level
- Adds `aws_profile: ""` at root level

### 3. Full Migration Chain Test (`TestApplyMigrationsChain`)

Tests migrating from v1 all the way to v5 in one operation.

**What it validates:**
- Successfully applies all migrations sequentially
- Sets `schema_version: 5` after completion
- All v2 fields (aurora) are present
- All v3 fields (zone_id) are present
- All v4 fields (backend_desired_count) are present
- All v5 fields (account_id) are present

**Output:**
```
Schema version detected: v1 (current: v5)
Applying migrations...
  → Migrating to v2: Adding Aurora Serverless v2 and ALB support
  → Migrating to v3: Adding DNS management fields
  → Migrating to v4: Adding backend scaling configuration
  → Migrating to v5: Adding Account ID and AWS Profile fields
✓ Successfully migrated to v5
```

### 4. Idempotency Test (`TestMigrationIdempotency`)

Tests that running migrations multiple times produces the same result.

**What it validates:**
- First migration run succeeds
- Serializes result to YAML
- Deserializes and runs migration again
- Second run produces identical output
- Migration is safe to run multiple times

**Why this matters:**
- Ensures migrations don't double-add fields
- Safe if migration is interrupted and re-run
- Protects against data corruption

### 5. File Integration Test (`TestMigrateYAMLFileIntegration`)

Tests the complete file migration workflow including backup creation.

**What it validates:**
- Creates temporary test file with v1 schema
- Runs `MigrateYAMLFile()` function
- Backup file is created with timestamp format
- Original file is updated with migrations
- Migrated file has `schema_version: 5`
- All migration fields are present in the file
- File is valid YAML after migration

**Sample output:**
```
═══════════════════════════════════════════════════════════
  Migrating: /tmp/migration-test-1360012214/test.yaml
═══════════════════════════════════════════════════════════
  ✓ Backup created: test.yaml.backup_20251015_211921
Schema version detected: v1 (current: v5)
Applying migrations...
  → Migrating to v2: Adding Aurora Serverless v2 and ALB support
  → Migrating to v3: Adding DNS management fields
  → Migrating to v4: Adding backend scaling configuration
  → Migrating to v5: Adding Account ID and AWS Profile fields
✓ Successfully migrated to v5
  ✓ Migration complete!
═══════════════════════════════════════════════════════════
```

### 6. Value Preservation Test (`TestMigrationPreservesExistingValues`)

Tests that migrations don't modify or lose existing user values.

**What it validates:**
- Custom project name is preserved
- Custom environment is preserved
- Custom region is preserved
- Custom database name is preserved
- Custom username is preserved
- Custom port is preserved
- Custom boolean values are preserved
- Only NEW fields are added, existing fields untouched

**Test data:**
```yaml
project: myproject            # Preserved
env: production              # Preserved
region: eu-west-1            # Preserved
postgres:
  dbname: customdb           # Preserved
  username: customuser       # Preserved
  engine_version: "15"       # Preserved
```

## Running the Tests

### Run all tests
```bash
cd app
go test -v
```

### Run specific test
```bash
go test -v -run TestDetectSchemaVersion
go test -v -run TestMigrateToV2
go test -v -run TestApplyMigrationsChain
go test -v -run TestMigrationIdempotency
go test -v -run TestMigrateYAMLFileIntegration
go test -v -run TestMigrationPreservesExistingValues
```

### Run tests with coverage
```bash
go test -v -cover
```

## Test Data

The test suite includes complete YAML fixtures for each schema version:

- **v1YAMLFixture** - Initial schema with no version field
- **v2YAMLFixture** - Schema with Aurora support
- **v3YAMLFixture** - Schema with DNS management
- **v4YAMLFixture** - Schema with backend scaling
- **v5YAMLFixture** - Current schema with account tracking

These fixtures ensure tests are isolated and repeatable.

## Continuous Integration

These tests should be run:
- Before committing migration changes
- In CI/CD pipeline
- Before releasing new versions
- When adding new migrations

## Adding Tests for New Migrations

When adding a new migration (e.g., v6), add:

1. **Fixture**: Create `v6YAMLFixture` in `migrations_test.go`
2. **Detection test**: Add v6 case to `TestDetectSchemaVersion`
3. **Migration test**: Add `TestMigrateToV6` function
4. **Update chain test**: Verify v6 fields in `TestApplyMigrationsChain`

Example:
```go
func TestMigrateToV6(t *testing.T) {
    var data map[string]interface{}
    err := yaml.Unmarshal([]byte(v5YAMLFixture), &data)
    if err != nil {
        t.Fatalf("Failed to unmarshal YAML: %v", err)
    }

    err = migrateToV6(data)
    if err != nil {
        t.Fatalf("Migration to v6 failed: %v", err)
    }

    // Verify new fields...
}
```

## Test File Location

**File**: `app/migrations_test.go`

**Lines**: ~650 lines of comprehensive test coverage

## Benefits of These Tests

1. **Confidence**: Know migrations work before deploying
2. **Safety**: Catch breaking changes early
3. **Documentation**: Tests serve as examples
4. **Regression prevention**: Catch bugs in existing migrations
5. **Development speed**: Fast feedback loop

## Test Execution Time

All tests complete in **< 1 second**:
```
ok  	madappgang.com/meroku	0.269s
```

Fast enough to run on every save or commit.

---

**Status**: All tests passing ✅
**Last updated**: 2025-10-15
**Total test coverage**: Version detection, individual migrations, chain migration, idempotency, file I/O, value preservation
