# Backup System Documentation

## Overview

Meroku automatically creates timestamped backups before modifying any configuration files. All backups are now organized in dedicated `backup/` directories for better organization.

## How It Works

### Project Infrastructure Files

When meroku runs in your project infrastructure folder (e.g., `/Users/jack/dev/salpha/sava-p-infra/`):

```
Before (old behavior):
sava-p-infra/
├── dev.yaml
├── dev.yaml.backup_20251022_155657  ← Cluttered!
├── dev.yaml.backup_20251022_155701  ← Cluttered!
├── prod.yaml
├── prod.yaml.backup_20251022_155647  ← Cluttered!
└── ...

After (new behavior):
sava-p-infra/
├── dev.yaml
├── prod.yaml
├── .gitignore (contains backup/)
└── backup/
    ├── dev.yaml.backup_20251023_170747  ← Organized!
    ├── dev.yaml.backup_20251023_171234
    ├── prod.yaml.backup_20251023_170800
    └── ...
```

### AWS Config Files

For AWS SSO configuration files (`~/.aws/config`):

```
~/.aws/
├── config
├── credentials
└── backup/
    ├── config.backup.20251023_153045
    └── config.backup.20251023_154122
```

## Backup Triggers

Backups are automatically created when:

1. **YAML Migration** - Schema version upgrades (automatic on load)
2. **Manual Migration** - Running `./meroku migrate dev.yaml`
3. **AWS SSO Setup** - Writing AWS configuration files
4. **AI Agent Edits** - AI agent modifying Terraform or YAML files

## Backup Naming Convention

All backups use the format: `<filename>.backup_YYYYMMDD_HHMMSS`

Examples:
- `dev.yaml.backup_20251023_170747`
- `config.backup.20251023_153045`
- `main.tf.backup_20251023_140530`

## File Permissions

- **YAML files**: `0644` (readable by all)
- **AWS config**: `0600` (owner-only, secure)
- **Terraform files**: `0644` (readable by all)

## Backup Locations

### Project Infrastructure Backups

**Location**: `<project-folder>/backup/`

The backup folder is created in the same directory as your YAML files. Since meroku runs from your project infrastructure folder, backups are stored there.

**Example**:
```bash
cd /Users/jack/dev/salpha/sava-p-infra
./meroku migrate dev.yaml
# Backup created at: /Users/jack/dev/salpha/sava-p-infra/backup/dev.yaml.backup_20251023_170747
```

### AWS Config Backups

**Location**: `~/.aws/backup/`

All AWS configuration backups are stored in your AWS config directory.

## Migrating Existing Backup Files

If you have old backup files scattered in your project folder, use the migration script:

```bash
cd /Users/jack/dev/salpha/sava-p-infra
/path/to/infrastructure/scripts/migrate-backups.sh
```

This script will:
1. Create `backup/` directory
2. Move all `*.backup*` files into it
3. Add `backup/` to `.gitignore` (if not already present)

## .gitignore Configuration

Add this to your project infrastructure `.gitignore`:

```gitignore
# Backup files
backup/
```

This prevents backup files from being committed to version control.

## Implementation Details

### Centralized Backup Utility

All backup creation goes through `app/backup_util.go`:

```go
// Create a backup with default settings
backupPath, err := CreateProjectBackup("dev.yaml")

// Create a backup with custom permissions
backupPath, err := CreateFileBackupWithPermissions("main.tf", 0644)

// Create AWS config backup (0600 permissions)
backupPath, err := CreateAWSConfigBackup("~/.aws/config")
```

### Key Features

1. **Automatic directory creation** - Creates `backup/` if it doesn't exist
2. **Collision handling** - Adds microseconds to timestamp if file exists
3. **Flexible configuration** - Customizable permissions and directory structure
4. **Error handling** - Returns detailed errors for troubleshooting

## Restoring from Backup

To restore a file from backup:

```bash
# Copy the backup back to the original location
cp backup/dev.yaml.backup_20251023_170747 dev.yaml
```

Or compare changes:

```bash
# See what changed
diff dev.yaml backup/dev.yaml.backup_20251023_170747
```

## Cleaning Up Old Backups

Backups accumulate over time. To clean up old backups:

```bash
# Delete backups older than 30 days
find backup/ -name "*.backup_*" -mtime +30 -delete

# Keep only the 10 most recent backups per file
ls -t backup/dev.yaml.backup_* | tail -n +11 | xargs rm
```

## Testing

The backup system is covered by tests in:
- `app/migrations_test.go:467` - YAML migration backups
- `app/ai_agent_security_test.go:291` - AI agent file backups

Run tests:
```bash
cd /Users/jack/mag/infrastructure/app
go test -v -run TestMigrateYAMLFileIntegration
go test -v -run TestCreateFileBackup
```

## Troubleshooting

### Backup Directory Not Created

**Issue**: Backup directory doesn't exist

**Solution**: Check directory permissions:
```bash
ls -la /Users/jack/dev/salpha/sava-p-infra
# Should show write permissions for current user
```

### Backup Failed Error

**Issue**: "failed to create backup directory" error

**Solution**: Ensure you have write permissions:
```bash
chmod u+w /Users/jack/dev/salpha/sava-p-infra
```

### Old Backup Files Still Present

**Issue**: Backup files still in root directory after update

**Solution**: Run the migration script:
```bash
/path/to/infrastructure/scripts/migrate-backups.sh
```

## Architecture Notes

### Why Relative Paths Work

Meroku runs from the project infrastructure folder. When loading `dev.yaml`:

1. `loadEnvWithMigration("dev")` finds file at `"dev.yaml"` (relative path)
2. `filepath.Dir("dev.yaml")` returns `"."` (current directory)
3. Backup directory becomes `"./backup"`
4. Expands to full path: `/Users/jack/dev/salpha/sava-p-infra/backup/`

This ensures backups are always created in the correct location, regardless of where the meroku binary is located.

### Source Code vs Runtime

- **Meroku source**: `/Users/jack/mag/infrastructure/app/`
- **Meroku runs from**: `/Users/jack/dev/salpha/sava-p-infra/` (project folder)
- **Backups created in**: Project folder's `backup/` directory ✓

The implementation correctly handles this separation by using `filepath.Dir()` on the source file path, not the binary location.
