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

Current version: **v5**

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
