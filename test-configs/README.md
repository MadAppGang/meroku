# AWS Config Test Files

This directory contains AWS config files for testing different SSO scenarios without modifying your actual `~/.aws/config`.

## Usage

Run meroku with a custom config file:

```bash
./app/meroku --aws-config test-configs/aws-config-empty
```

Or set as environment variable:

```bash
export AWS_CONFIG_FILE=$PWD/test-configs/aws-config-modern-sso
./app/meroku
```

## Test Scenarios

### 1. Empty Config (`aws-config-empty`)
**Scenario:** Fresh install, no AWS configuration at all

**Use case:** Test the wizard from scratch

**Expected behavior:**
- Validation should fail (profile not found)
- Wizard should guide through complete setup

**Test command:**
```bash
./app/meroku --aws-config test-configs/aws-config-empty
```

### 2. Modern SSO (`aws-config-modern-sso`)
**Scenario:** Properly configured modern SSO with sso-session (recommended format)

**Profiles:**
- `dev`: Account 123456789012, AdministratorAccess, us-east-1
- `staging`: Account 234567890123, PowerUserAccess, us-west-2

**Use case:** Test validation on correct configuration

**Expected behavior:**
- Validation should pass (if SSO token is valid)
- Should show profile is already configured
- May fail on token validation (can't actually login in test mode)

**Test command:**
```bash
./app/meroku --aws-config test-configs/aws-config-modern-sso
```

### 3. Legacy SSO (`aws-config-legacy-sso`)
**Scenario:** Old-style SSO configuration (no token refresh support)

**Profiles:**
- `dev`: Account 123456789012, AdministratorAccess
- `prod`: Account 999888777666, ReadOnlyAccess

**Use case:** Test legacy config detection and migration suggestions

**Expected behavior:**
- Validation should recognize as legacy SSO
- Should work but recommend migrating to modern SSO

**Test command:**
```bash
./app/meroku --aws-config test-configs/aws-config-legacy-sso
```

### 4. Incomplete Config (`aws-config-incomplete`)
**Scenario:** Partially configured profiles with missing required fields

**Profiles:**
- `dev`: Missing sso_session/sso_start_url
- `staging`: Missing sso_account_id and sso_role_name, references non-existent sso-session
- `prod`: Completely empty

**Use case:** Test validation error detection and fix suggestions

**Expected behavior:**
- Validation should fail with specific missing field errors
- Wizard should complete missing fields

**Test command:**
```bash
./app/meroku --aws-config test-configs/aws-config-incomplete
```

## Testing Workflow

### Test Validation System

```bash
# Test with complete config (should pass)
./app/meroku --aws-config test-configs/aws-config-modern-sso
# Select: "✓ Validate AWS Configuration"

# Test with incomplete config (should fail with details)
./app/meroku --aws-config test-configs/aws-config-incomplete
# Select: "✓ Validate AWS Configuration"
```

### Test Interactive Wizard

```bash
# Start with empty config
./app/meroku --aws-config test-configs/aws-config-empty
# Select: "🔐 AWS SSO Setup Wizard"
# Follow prompts to configure a profile
```

### Test the Claude Code handoff

```bash
# Start with incomplete config
./app/meroku --aws-config test-configs/aws-config-incomplete
# Select: "🔐 AWS SSO Tools" → "🤖 Fix with AI (Claude Code)"
# Inside tmux with `claude` on PATH, Claude Code opens in a pane to the right.
# Otherwise meroku prints the briefing path and the command to start it.
# The briefing (.meroku/claude/<time>-sso.md) should name the profile, the
# missing fields and the AWS config path.
```

### Test Config Writing

```bash
# Use empty config and run wizard
./app/meroku --aws-config test-configs/aws-config-empty

# After wizard completes, check the file was updated:
cat test-configs/aws-config-empty

# Check backup was created:
ls -la test-configs/aws-config-empty.backup.*
```

## Creating New Test Configs

To create a new test scenario:

1. Create a new file: `test-configs/aws-config-<scenario-name>`
2. Add appropriate config content (can be empty, partial, or complete)
3. Document the scenario in this README
4. Test with: `./app/meroku --aws-config test-configs/aws-config-<scenario-name>`

## Example: Testing Account ID Mismatch

Create a config with wrong account ID to test mismatch detection:

```ini
# test-configs/aws-config-wrong-account
[profile dev]
sso_session = my-sso
sso_account_id = 999999999999  # Wrong account ID
sso_role_name = AdministratorAccess
region = us-east-1

[sso-session my-sso]
sso_region = us-east-1
sso_start_url = https://my-company.awsapps.com/start
```

If your project/dev.yaml has `account_id: 123456789012`, validation should detect the mismatch.

## Notes

- These test configs won't actually work for AWS API calls (no real SSO credentials)
- SSO token validation will fail (expected - this is for testing config validation only)
- Backups will be created in the same directory as the config file
- You can modify these files directly to test different scenarios
- The custom config path is stored globally, so it applies to all SSO operations during the session
