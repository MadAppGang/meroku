# YAML Schema Migration System

## Overview

The meroku application includes a comprehensive YAML schema migration system that automatically upgrades configuration files to the latest format. This ensures backward compatibility when the schema evolves over time.

## Key Features

- **Automatic Migration**: YAML files are automatically migrated when loaded
- **Version Detection**: Intelligently detects the current schema version
- **Backup Creation**: Creates timestamped backups before migration
- **Manual Migration**: CLI commands for manual migration
- **Extensible**: Easy to add new migrations for future changes

## Schema Version History

| Version | Description | Key Changes |
|---------|-------------|-------------|
| 1 | Initial version | Base schema with no version field |
| 2 | Aurora Serverless v2 | Added `aurora`, `min_capacity`, `max_capacity` to postgres; Added ALB configuration |
| 3 | DNS Management | Added DNS fields: `zone_id`, `root_zone_id`, `is_dns_root`, `delegation_role_arn`, etc. |
| 4 | Backend Scaling | Added backend scaling config: `backend_desired_count`, `backend_autoscaling_*`, `backend_cpu`, `backend_memory` |
| 5 | Account Tracking | Added `account_id` and `aws_profile` for better AWS account management |
| 6 | Custom VPC | Added `use_default_vpc` and `vpc_cidr`; Removed deprecated VPC fields |
| 7 | ECR Strategy | Added `ecr_strategy`, `ecr_account_id`, `ecr_account_region` for flexible ECR configuration |
| 8 | ECR Trusted Accounts | Added `ecr_trusted_accounts` array for cross-account ECR pull access |
| 9 | Amplify Domains | `subdomain_prefix` replaces `custom_domain` + `enable_root_domain` |
| 10 | Per-service ECR | Added `ecr_config` to services, event_processor_tasks and scheduled_tasks |
| 11 | awsvpc Ports | `host_port` forced to match `container_port` |
| 12 | Postgres Booleans | All postgres boolean fields written explicitly |
| 13 | Multi-rule EventBridge | `rules[]` array in event_processor_tasks |
| 14 | CloudFront | CloudFront CDN configuration |
| 15 | Multiple CloudFront | `cloudfront_distributions[]` array |
| 16 | State Locking | `state_lock_table` for DynamoDB state locking |
| 17 | Multi-domain SES | `domains[]` array with optional per-domain `zone_id` |
| 18 | Global test_emails | `test_emails` moved to global SES level |
| 19 | Service enable/disable | `enabled` on services |
| 20 | Task enable/disable | `enabled` on scheduled_tasks and event_processor_tasks |
| 21 | AppSync authorizer | `jwks_uri`, `jwt_issuer`, `jwt_audience` — written **empty**, never guessed |
| 22 | CI/CD auto-deploy | `backend_auto_deploy` and per-target `auto_deploy` |
| 23 | AppSync auth modes | `auth_mode` and `api_key_enabled` on `pubsub_appsync` |
| 24 | Cognito key repair | `cognito.dashboard_callback_ur_ls` → `dashboard_callback_urls` |

Current version: **v24**

### A note on v23's two defaults

v23 is the one migration that does **not** write the safer of two values
everywhere, and the reasoning is worth knowing before you copy the pattern.

- `auth_mode` is inferred as `lambda` for every existing config. Not because
  `lambda` is preferred — the two native modes are cheaper and run no Lambda —
  but because `authentication_type` was hardcoded to `AWS_LAMBDA` before the
  setting existed. Writing anything else would change what the next apply
  deploys. Note this holds even where `auth_lambda: false`: that flag only ever
  chose whose authorizer *source* was packaged.
- `api_key_enabled` is written `false` where AppSync is disabled and **`true`
  where it is enabled**. An enabled environment has a live API key in AWS right
  now, because the module created one unconditionally. Writing `false` would
  delete that key on the next apply and break any client holding it. Breaking a
  working API to close a weakness nobody reported that day is the worse failure,
  so the migration preserves the credential and prints a loud notice explaining
  what it is and how to remove it. "Default off" holds for every **new**
  environment and for every environment with nothing to lose.

### A note on v24 — repairing a key rather than adding one

v24 is the only migration so far that **renames** an existing key instead of
adding a new one, so it is worth saying why that is a migration at all.

`Cognito.DashboardCallbackURLs` was tagged `yaml:"dashboard_callback_ur_ls"` — an
acronym-splitting snake_case conversion of the Go field name. Everything else in
the system used `dashboard_callback_urls`: `env/main.hbs`, `modules/cognito`, the
web UI's TypeScript types, and the documentation. The struct was the only thing
that disagreed, and it is the thing that reads and writes the file.

So every load silently dropped whatever URLs the user had configured, and every
save wrote the empty result back under the misspelt name. Fixing the tag alone
would not help anyone: their file still holds the wrong key, and the template
would keep reading a key that is not there. The repair has to happen on disk.

Where both keys exist, the correctly spelled one wins — it is the one the
template reads, so it is what is deployed today, and a migration must not change
what the next apply produces. The discarded value is printed rather than dropped
in silence.

Mode-specific optional fields (`oidc_issuer`, `oidc_client_id`,
`cognito_app_id_client_regex`, `required_claims`) are deliberately **not**
written by the migration. Per CLAUDE.md, optional fields with sensible fallbacks
belong to the `default` helper in the template; a migration writes the core
policy fields that should always be explicit. Writing an empty `oidc_issuer` into
every lambda-mode config would be noise nothing reads.

## How It Works

### Automatic Migration

When you load a YAML file (e.g., `dev.yaml`, `prod.yaml`), the system:

1. Reads the file and detects its current schema version
2. If the version is older than the current version, it:
   - Creates a timestamped backup (e.g., `dev.yaml.backup_20251015_211246`)
   - Applies all necessary migrations in sequence
   - Updates the `schema_version` field
   - Saves the migrated file

Example output:
```
═══════════════════════════════════════════════════════════
  YAML Schema Migration Required
═══════════════════════════════════════════════════════════
File: project/dev.yaml
  ✓ Backup created: project/dev.yaml.backup_20251015_211246
Schema version detected: v2 (current: v5)
Applying migrations...
  → Migrating to v3: Adding DNS management fields
  → Migrating to v4: Adding backend scaling configuration
  → Migrating to v5: Adding Account ID and AWS Profile fields
✓ Successfully migrated to v5
  ✓ Migrated file saved: project/dev.yaml
═══════════════════════════════════════════════════════════
```

### Manual Migration

You can manually migrate files using the CLI:

```bash
# Migrate a specific file
./meroku migrate dev.yaml

# Migrate all YAML files in the project directory
./meroku migrate all

# Show migration help
./meroku migrate
```

## Version Detection Logic

The system uses intelligent detection to determine the schema version:

1. **Explicit version**: If `schema_version` field exists, use it
2. **Field presence detection**: Otherwise, detect based on fields present:
   - v5: Has `account_id` or `aws_profile`
   - v4: Has `backend_desired_count` in workload
   - v3: Has `zone_id` in domain
   - v2: Has `aurora` in postgres
   - v1: Default (no version indicators)

## Migration Safety

### Backups

Every migration creates a timestamped backup file before making changes:
- Format: `<original-filename>.backup_YYYYMMDD_HHMMSS`
- Example: `dev.yaml.backup_20251015_211246`

### Idempotent Migrations

Migrations are idempotent - they only add missing fields and never remove or modify existing data. This means:
- Running a migration multiple times is safe
- Existing values are preserved
- Only new fields are added with default values

## Adding New Migrations

To add a new migration for schema changes:

1. **Update CurrentSchemaVersion** in `migrations.go`:
   ```go
   const CurrentSchemaVersion = 6  // Increment
   ```

2. **Add migration to AllMigrations**:
   ```go
   {
       Version:     6,
       Description: "Add new feature X",
       Apply:       migrateToV6,
   }
   ```

3. **Implement migration function**:
   ```go
   func migrateToV6(data map[string]interface{}) error {
       fmt.Println("  → Migrating to v6: Add new feature X")

       // Add new fields with defaults
       if _, exists := data["new_field"]; !exists {
           data["new_field"] = "default_value"
       }

       return nil
   }
   ```

4. **Update detection logic** in `detectSchemaVersion()`:
   ```go
   // Check for v6 fields
   if _, hasNewField := data["new_field"]; hasNewField {
       return 6
   }
   ```

## Migration Examples

### Example: Adding Aurora Support (v1 → v2)

Before (v1):
```yaml
postgres:
  enabled: true
  dbname: mydb
  username: admin
  engine_version: "14"
```

After (v2):
```yaml
postgres:
  enabled: true
  dbname: mydb
  username: admin
  engine_version: "14"
  aurora: false           # Added
  min_capacity: 0.5       # Added
  max_capacity: 1.0       # Added
schema_version: 2         # Added
```

### Example: Adding Account Tracking (v4 → v5)

Before (v4):
```yaml
project: myproject
env: dev
region: us-east-1
```

After (v5):
```yaml
project: myproject
env: dev
region: us-east-1
account_id: ""          # Added
aws_profile: ""         # Added
schema_version: 5       # Updated
```

### Example: Adding ECR Trusted Accounts (v7 → v8)

Before (v7):
```yaml
project: myproject
env: dev
ecr_strategy: local
ecr_account_id: ""
ecr_account_region: ""
schema_version: 7
```

After (v8):
```yaml
project: myproject
env: dev
ecr_strategy: local
ecr_account_id: ""
ecr_account_region: ""
ecr_trusted_accounts: []    # Added - empty array for new field
schema_version: 8           # Updated
```

**Note**: The `ecr_trusted_accounts` field enables bidirectional YAML updates when configuring cross-account ECR access. When a target environment is configured to pull from a source environment, both YAML files are automatically updated:
- **Target**: Gets `ecr_strategy: cross_account` with source account details
- **Source**: Adds target to `ecr_trusted_accounts` array for trust policy generation

## Troubleshooting

### Migration Failed

If a migration fails:
1. Check the error message for details
2. Restore from the backup file if needed
3. Fix any issues in the YAML file
4. Run the migration again

### Backup Not Created

If no backup is created, the file is already at the current version. No migration is needed.

### Manual Restoration

To restore from a backup:
```bash
cp dev.yaml.backup_20251015_211246 dev.yaml
```

## Best Practices

1. **Test in dev first**: Always test migrations on development environments before production
2. **Keep backups**: Don't delete backup files until you've verified the migration
3. **Version control**: Commit YAML files to git before and after migration
4. **Review changes**: Check the migrated file to ensure all fields are correct
5. **Manual migration**: Use `./meroku migrate all` when updating multiple environments

## Testing

The migration system has comprehensive test coverage with 8 test suites:

```bash
# Run all migration tests
cd app && go test -v

# Run specific test
go test -v -run TestApplyMigrationsChain
```

**Test Coverage:**
- ✅ Version detection (5 sub-tests for v1-v5)
- ✅ Individual migrations (v2, v3, v4, v5)
- ✅ Full migration chain (v1 → v5)
- ✅ Idempotency (safe to run multiple times)
- ✅ File I/O with backup creation
- ✅ Value preservation (existing data not modified)

All tests pass in < 1 second.

For detailed test documentation, see [Migration Tests](./MIGRATION_TESTS.md)

## Files

- `app/migrations.go` - Migration system implementation
- `app/migrations_test.go` - Comprehensive test suite
- `app/model.go` - YAML structure definitions and loading
- `app/main.go` - CLI command handling

## Technical Details

### Migration Flow

```
loadEnv()
  → loadEnvWithMigration()
    → Read YAML file
    → Unmarshal to map[string]interface{}
    → detectSchemaVersion()
    → applyMigrations() if needed
      → Create backup
      → Apply each migration in sequence
      → Set schema_version
      → Save file
    → Unmarshal to Env struct
    → Return Env
```

### Data Structure

Migrations work on `map[string]interface{}` to handle:
- Missing fields
- Different types
- Nested structures
- Array elements

After migration, the data is unmarshaled to the typed `Env` struct for use in the application.

## Future Enhancements

Potential improvements for the migration system:

1. **Rollback support**: Ability to downgrade to previous versions
2. **Dry-run mode**: Preview changes without applying
3. **Migration validation**: Verify migrated data is valid
4. **Custom migration hooks**: Allow projects to define custom migrations
5. **Migration history**: Track which migrations have been applied

---

**Note**: This migration system ensures that legacy YAML configuration files continue to work as the schema evolves, providing a smooth upgrade path for users.
