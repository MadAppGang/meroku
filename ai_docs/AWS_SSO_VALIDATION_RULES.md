# AWS SSO Profile Validation Rules

**Based on Official AWS CLI Documentation (2025)**

This document defines the static validation rules for AWS SSO profile configuration, extracted from official AWS documentation.

## Configuration Format Overview

AWS CLI v2 supports two SSO configuration formats:

1. **Modern SSO (Recommended)** - Uses `sso-session` sections with token refresh support (AWS CLI v2.22.0+)
2. **Legacy SSO** - Direct profile configuration without token refresh

## Modern SSO Configuration (Recommended)

### Profile Section: `[profile <name>]`

**Required Fields:**
- `sso_session` (string) - Name reference to an `[sso-session]` section
- `sso_account_id` (string) - AWS account ID (12 digits, e.g., "111122223333")
- `sso_role_name` (string) - IAM role name (e.g., "SampleRole", "AdministratorAccess")

**Optional Fields:**
- `region` (string) - Default AWS region for CLI commands (e.g., "us-east-1")
- `output` (string) - Default output format: "json", "yaml", "text", or "table"

**Example:**
```ini
[profile dev]
sso_session = my-sso
sso_account_id = 111122223333
sso_role_name = SampleRole
region = us-west-2
output = json
```

### SSO-Session Section: `[sso-session <name>]`

**Required Fields:**
- `sso_region` (string) - AWS region hosting IAM Identity Center (e.g., "us-east-1")
- `sso_start_url` (string) - IAM Identity Center start URL (e.g., "https://my-sso-portal.awsapps.com/start")

**Optional Fields:**
- `sso_registration_scopes` (string) - OAuth 2.0 scopes (defaults to "sso:account:access")

**Alternative to sso_start_url (AWS CLI v2.22.0+):**
- `sso_issuer_url` (string) - Identity Center issuer URL (can be used instead of start URL)

**Example:**
```ini
[sso-session my-sso]
sso_region = us-east-1
sso_start_url = https://my-sso-portal.awsapps.com/start
sso_registration_scopes = sso:account:access
```

### Complete Modern Configuration Example:
```ini
[profile dev]
sso_session = my-sso
sso_account_id = 111122223333
sso_role_name = SampleRole
region = us-west-2
output = json

[sso-session my-sso]
sso_region = us-east-1
sso_start_url = https://my-sso-portal.awsapps.com/start
sso_registration_scopes = sso:account:access
```

## Legacy SSO Configuration

### Profile Section: `[profile <name>]`

**Required Fields:**
- `sso_start_url` (string) - IAM Identity Center start URL
- `sso_region` (string) - AWS region hosting IAM Identity Center
- `sso_account_id` (string) - AWS account ID (12 digits)
- `sso_role_name` (string) - IAM role name

**Optional Fields:**
- `region` (string) - Default AWS region for CLI commands
- `output` (string) - Default output format

**Example:**
```ini
[profile my-dev-profile]
sso_start_url = https://my-sso-portal.awsapps.com/start
sso_region = us-east-1
sso_account_id = 123456789011
sso_role_name = readOnly
region = us-west-2
output = json
```

**Note:** Legacy configuration does NOT support automatic token refresh. AWS recommends using modern SSO configuration with `sso-session` sections.

## Validation Rules

### Rule 1: AWS CLI Availability
- **Check:** AWS CLI command exists in PATH
- **Validation:** Run `which aws` (Linux/macOS) or `where aws` (Windows)
- **Fix:** Install AWS CLI v2
  - macOS: `brew install awscli`
  - Linux: Download installer from AWS
  - Windows: Download MSI installer

### Rule 2: AWS CLI Version
- **Check:** AWS CLI version >= 2.0.0
- **Validation:** Parse output of `aws --version`
- **Fix:** Upgrade to AWS CLI v2 (see Rule 1)
- **Minimum Required:** v2.0.0
- **Recommended:** v2.22.0+ (for modern SSO with token refresh)

### Rule 3: Config File Exists
- **Check:** `~/.aws/config` file exists
- **Validation:** Check file existence
- **Fix:** Create file with permissions 600 (read/write for user only)

### Rule 4: Config File Format
- **Check:** File is valid INI format
- **Validation:** Parse file with INI parser
- **Fix:** Show syntax errors and line numbers, offer to regenerate

### Rule 5: Profile Exists
- **Check:** Profile referenced in YAML exists in config
- **Validation:** Section `[profile <name>]` or `[default]` exists
- **Fix:** Create profile section with required fields (wizard)

### Rule 6: SSO Session Reference (Modern Config)
- **Check:** If using modern config, `sso_session` field exists and points to valid section
- **Validation:**
  - Key `sso_session` exists in profile
  - Section `[sso-session <name>]` exists
- **Fix:** Create referenced sso-session section (wizard)

### Rule 7: Required SSO Fields (Modern Config)

**Profile Section:**
- **Check:** `sso_session`, `sso_account_id`, `sso_role_name` all present
- **Validation:** Keys exist and have non-empty values
- **Fix:** Prompt user for missing values, write to config

**SSO-Session Section:**
- **Check:** `sso_region`, `sso_start_url` both present
- **Validation:** Keys exist and have non-empty values
- **Fix:** Prompt user for missing values, write to config

### Rule 8: Required SSO Fields (Legacy Config)
- **Check:** `sso_start_url`, `sso_region`, `sso_account_id`, `sso_role_name` all present
- **Validation:** Keys exist and have non-empty values
- **Fix:** Prompt user for missing values, write to config
- **Recommendation:** Suggest migrating to modern config

### Rule 9: Field Format Validation

**sso_start_url:**
- **Format:** Must be valid HTTPS URL
- **Pattern:** `https://*.awsapps.com/start` or `https://*`
- **Example:** `https://my-company.awsapps.com/start`

**sso_region:**
- **Format:** Valid AWS region code
- **Pattern:** `[a-z]{2}-[a-z]+-\d+`
- **Examples:** `us-east-1`, `eu-west-1`, `ap-southeast-2`

**sso_account_id:**
- **Format:** 12-digit string
- **Pattern:** `^\d{12}$`
- **Example:** `111122223333`

**sso_role_name:**
- **Format:** Valid IAM role name
- **Pattern:** `^[\w+=,.@-]+$`
- **Examples:** `AdministratorAccess`, `PowerUserAccess`, `ReadOnlyAccess`

**region (optional):**
- **Format:** Valid AWS region code (same as sso_region)

**output (optional):**
- **Format:** One of: `json`, `yaml`, `text`, `table`
- **Default:** `json` if not specified

**sso_registration_scopes (optional):**
- **Format:** Comma-delimited list of OAuth scopes
- **Default:** `sso:account:access`
- **Examples:** `sso:account:access`, `sso:account:access,openid`

### Rule 10: SSO Token Validity
- **Check:** Can retrieve credentials using profile
- **Validation:** Run `aws sts get-caller-identity --profile <name>`
- **Fix:** Run `aws sso login --profile <name>` to authenticate

### Rule 11: Account ID Match (if specified in YAML)
- **Check:** Account ID from STS matches YAML configuration
- **Validation:** Compare `account_id` from project YAML with STS response
- **Fix:** Update YAML with correct account ID from AWS or update config

### Rule 12: Region Match (if specified in YAML)
- **Check:** Region in profile matches YAML configuration
- **Validation:** Compare `region` from project YAML with config
- **Fix:** Update config to match YAML or update YAML

## Auto-Fix Strategy

When validation fails, the wizard should:

1. **Detect Configuration Type:**
   - Check if profile uses `sso_session` field → Modern
   - Check if profile has `sso_start_url` directly → Legacy
   - If neither → Incomplete/new profile

2. **Collect Missing Information:**
   - Pre-fill from project YAML (`account_id`, `region`)
   - Pre-fill from existing config (partial data)
   - Only prompt for missing fields

3. **Interactive Prompts (with defaults):**
   ```
   ? SSO Start URL: [                                              ]
     Example: https://mycompany.awsapps.com/start

   ? SSO Region: [us-east-1]

   ? Account ID: [123456789012] (detected from dev.yaml)

   ? Role Name: [AdministratorAccess]
     Common roles: AdministratorAccess, PowerUserAccess, ReadOnlyAccess

   ? Default Region: [us-east-1] (detected from dev.yaml)

   ? Output Format: [json]
     Options: json, yaml, text, table
   ```

4. **Write Configuration:**
   - Create backup: `~/.aws/config.backup.<timestamp>`
   - Parse existing config (preserve other profiles)
   - Add/update profile and sso-session sections
   - Write atomically (temp file + rename)
   - Set file permissions: 600

5. **Automatic Login:**
   - Execute: `aws sso login --profile <name>`
   - Browser opens for authentication
   - Wait for success confirmation

6. **Validation:**
   - Run: `aws sts get-caller-identity --profile <name>`
   - Verify account ID matches expected
   - Display success confirmation

## Common Scenarios

### Scenario 1: Fresh Install (No Config)
**Detection:**
- `~/.aws/config` does not exist

**Fix Steps:**
1. Create `~/.aws/` directory (permissions 700)
2. Create empty `~/.aws/config` (permissions 600)
3. Run full setup wizard for each YAML environment
4. Write complete modern SSO configuration
5. Run `aws sso login` for each profile

### Scenario 2: Partial Config (Missing SSO Fields)
**Detection:**
- Profile exists but missing required SSO fields

**Fix Steps:**
1. Detect configuration type (modern vs legacy)
2. Show existing values
3. Prompt only for missing fields
4. Update config file in-place
5. Run `aws sso login` if credentials missing

### Scenario 3: Legacy Config (Needs Migration)
**Detection:**
- Profile has `sso_start_url` directly (no `sso_session`)

**Fix Steps:**
1. Ask: "Migrate to modern SSO configuration? (Recommended)"
2. If yes:
   - Create `[sso-session]` section
   - Update profile to reference session
   - Remove duplicate fields from profile
3. If no:
   - Validate legacy config completeness
   - Fix missing fields only

### Scenario 4: Expired Token
**Detection:**
- Config valid but `aws sts get-caller-identity` fails with token error

**Fix Steps:**
1. Display: "SSO token expired"
2. Run: `aws sso login --profile <name>`
3. Re-validate credentials

### Scenario 5: Wrong Account ID
**Detection:**
- STS returns different account ID than YAML specifies

**Fix Steps:**
1. Display both account IDs
2. Ask: "Which is correct?"
   - Option A: Update YAML to match AWS
   - Option B: Update AWS config to match YAML (wrong profile selected)
3. Update selected file
4. Re-validate

## Implementation Notes

### File Operations
- Always create backups before modifying config
- Use atomic writes (temp file + rename)
- Preserve comments and formatting where possible
- Set proper file permissions (600 for config, 700 for directory)

### User Experience
- Show progress indicators during operations
- Provide helpful examples and defaults
- Allow cancellation at any step
- Confirm before making changes
- Display clear success/failure messages

### Error Handling
- Catch and explain AWS SDK errors
- Detect network issues
- Handle browser automation failures
- Provide recovery steps for all error scenarios

### Security
- Never log or display secret keys
- Protect config file permissions
- Clear temp files after operations
- Use secure temp directory

## References

- [AWS CLI SSO Configuration](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-sso.html)
- [Shared Config File Format](https://docs.aws.amazon.com/sdkref/latest/guide/file-format.html)
- [IAM Identity Center OAuth Scopes](https://docs.aws.amazon.com/singlesignon/latest/userguide/customermanagedapps-saml2-oauth2.html#oidc-concept)
