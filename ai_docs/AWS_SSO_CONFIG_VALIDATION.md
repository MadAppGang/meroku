# AWS SSO Configuration Validation - Complete Reference

## Overview

This document provides a comprehensive reference for validating AWS SSO configurations, based on official AWS documentation (2025) and the current implementation in `app/aws_sso_inspector.go`.

## Configuration Types

### 1. Modern SSO (Recommended) - Uses SSO Sessions

**Profile Section** `[profile myprofile]`:
```ini
[profile dev]
sso_session = my-sso-session
sso_account_id = 123456789012
sso_role_name = DevRole
region = us-east-1          # Optional
output = json               # Optional
```

**SSO-Session Section** `[sso-session name]`:
```ini
[sso-session my-sso-session]
sso_start_url = https://mycompany.awsapps.com/start
sso_region = us-east-1
sso_registration_scopes = sso:account:access  # Optional, defaults to sso:account:access
```

### 2. Legacy SSO (Not Recommended) - Direct SSO Fields

All fields in profile section:
```ini
[profile dev]
sso_start_url = https://mycompany.awsapps.com/start
sso_region = us-east-1
sso_account_id = 123456789012
sso_role_name = DevRole
region = us-east-1          # Optional
output = json               # Optional
```

### 3. SSO + Role Assumption (Chaining)

SSO profile used as source for role assumption:
```ini
[profile sso-base]
sso_session = my-sso
sso_account_id = 111111111111
sso_role_name = BaseRole

[profile assumed-role]
role_arn = arn:aws:iam::222222222222:role/AssumedRole
source_profile = sso-base
role_session_name = my-session  # Optional
external_id = xyz123            # Optional
duration_seconds = 3600         # Optional (900-43200)
mfa_serial = arn:aws:iam::...   # Optional

[sso-session my-sso]
sso_start_url = https://mycompany.awsapps.com/start
sso_region = us-east-1
```

## Required vs Optional Fields

### Modern SSO Profile Section

**Required:**
- `sso_session` - References the sso-session section name
- `sso_account_id` - AWS account ID (12 digits)
- `sso_role_name` - IAM role name to assume

**Optional:**
- `region` - Default AWS region for this profile
- `output` - Output format (json, yaml, text, table)

**Special Case:**
- For bearer token authentication, `sso_account_id` and `sso_role_name` become optional

### SSO-Session Section

**Required:**
- `sso_start_url` - IAM Identity Center portal URL (must be HTTPS)
- `sso_region` - Region hosting the Identity Center directory

**Optional:**
- `sso_registration_scopes` - OAuth 2.0 scopes (default: `sso:account:access`)

### Legacy SSO Profile Section

**Required:**
- `sso_start_url` - IAM Identity Center portal URL
- `sso_region` - Region for Identity Center
- `sso_account_id` - AWS account ID
- `sso_role_name` - IAM role name

**Optional:**
- `region` - Default AWS region
- `output` - Output format

## Additional Profile Settings

These settings can be used in ANY profile type (including SSO profiles):

### Credential & Authentication
- `aws_access_key_id` - Access key (static credentials)
- `aws_secret_access_key` - Secret key (static credentials)
- `aws_session_token` - Temporary session token
- `credential_process` - External command for credentials
- `credential_source` - Source for assume-role (Environment, Ec2InstanceMetadata, EcsContainer)
- `web_identity_token_file` - Path to OAuth/OIDC token file

### Role Assumption
- `role_arn` - IAM role ARN to assume
- `source_profile` - Profile to use for role assumption
- `role_session_name` - Custom session name
- `external_id` - Third-party external ID
- `duration_seconds` - Session duration (900-43200 seconds)
- `mfa_serial` - MFA device ARN or serial number

### Regional & Endpoints
- `region` - Default AWS region
- `endpoint_url` - Custom service endpoint
- `sts_regional_endpoints` - STS endpoint selection (legacy, regional)
- `account_id_endpoint_mode` - Account-based endpoints (preferred, disabled, required)
- `aws_account_id` - Account ID for endpoint routing
- `use_fips_endpoint` - Enable FIPS 140-2 endpoints (true, false)

### S3-Specific
- `s3.addressing_style` - Bucket addressing (path, virtual, auto)
- `s3.use_accelerate_endpoint` - S3 Transfer Acceleration (true, false)
- `s3.use_dualstack_endpoint` - IPv4/IPv6 dual-stack (true, false)
- `s3.payload_signing_enabled` - SHA256 payload signing (true, false)
- `s3.max_concurrent_requests` - Parallel transfers (default: 10)
- `s3.max_queue_size` - Task queue size (default: 1000)
- `s3.multipart_threshold` - Size for multipart (e.g., 10MB)
- `s3.multipart_chunksize` - Chunk size (min: 5MB)
- `s3.max_bandwidth` - Transfer bandwidth limit (e.g., 50MB/s)

### SSL/TLS
- `ca_bundle` - Custom CA certificate bundle path (.pem)

### Retry & Performance
- `retry_mode` - Retry strategy (standard, legacy, adaptive)
- `max_attempts` - Maximum retry attempts
- `tcp_keepalive` - TCP keep-alive packets (true, false)

### Output & Validation
- `output` - Output format (json, text, table, yaml)
- `cli_timestamp_format` - Timestamp format (iso8601, wire)
- `parameter_validation` - Client-side validation (true, false)
- `request_checksum_calculation` - Checksum timing (when_supported, when_required)
- `response_checksum_validation` - Response validation (when_supported, when_required)

### Advanced
- `services` - Reference to services section for custom endpoints
- `api_versions` - API version overrides
- `sdk_ua_app_id` - Application identifier (max 50 chars)
- `sigv4a_signing_region_set` - SigV4a signing regions (comma-delimited)

## Current Implementation Analysis

### What We Validate Correctly ✅

1. **Modern SSO Profile Fields:**
   - ✅ `sso_session` (required in profile)
   - ✅ `sso_account_id` (required in profile)
   - ✅ `sso_role_name` (required in profile)
   - ✅ `region` (optional in profile)
   - ✅ `output` (optional in profile)

2. **SSO-Session Fields:**
   - ✅ `sso_start_url` (required in sso-session)
   - ✅ `sso_region` (required in sso-session)
   - ✅ `sso_registration_scopes` (optional in sso-session)

3. **Legacy SSO Profile Fields:**
   - ✅ `sso_start_url` (required in profile)
   - ✅ `sso_region` (required in profile)
   - ✅ `sso_account_id` (required in profile)
   - ✅ `sso_role_name` (required in profile)

4. **Field Validation:**
   - ✅ URL format validation for `sso_start_url`
   - ✅ Region format validation
   - ✅ Account ID format (12 digits)
   - ✅ Role name format
   - ✅ Output format validation

5. **Error Reporting (FIXED):**
   - ✅ Now shows specific fields missing from sso-session
   - ✅ Previously showed generic "sso_session_incomplete"

### What We Don't Validate ⚠️

1. **Profile Type Detection:**
   - ❓ Don't distinguish SSO + Role Assumption profiles
   - ❓ Don't validate `source_profile` pointing to SSO profiles
   - ❓ Don't check for conflicting credential types

2. **Additional Optional Fields:**
   - ℹ️ Don't validate role assumption fields (`role_arn`, `source_profile`, etc.)
   - ℹ️ Don't validate advanced settings (retry, SSL, S3 config, etc.)
   - ℹ️ This is likely intentional - we only validate SSO-specific fields

3. **Bearer Token Special Case:**
   - ❓ Don't handle bearer token authentication where `sso_account_id` and `sso_role_name` are optional

4. **Validation Edge Cases:**
   - ❓ Don't check for conflicting settings (`source_profile` + `credential_source`)
   - ❓ Don't validate `sso_registration_scopes` format
   - ❓ Don't check session name length/format

### Profile Type Detection Issues

Current logic in `detectProfileType()` (lines 141-169):

```go
// Check for modern SSO (uses sso_session reference)
if section.HasKey("sso_session") {
    return ProfileTypeModernSSO
}

// Check for legacy SSO (has sso_start_url directly)
if section.HasKey("sso_start_url") {
    return ProfileTypeLegacySSO
}

// Check for incomplete SSO profile
if section.HasKey("sso_account_id") || section.HasKey("sso_role_name") || section.HasKey("sso_region") {
    return ProfileTypeModernSSO  // ⚠️ Assumes modern SSO
}
```

**Issue:** This doesn't handle SSO + Role Assumption profiles correctly.

Example problematic profile:
```ini
[profile assumed-role]
sso_session = my-sso      # Has sso_session → Detected as ModernSSO
role_arn = arn:aws:...    # Also has role_arn → Should be SSO + AssumeRole
source_profile = base     # Contradiction!
```

## Recommendations

### 1. Improve Profile Type Detection

Add new profile type:
```go
const (
    ProfileTypeUnknown           ProfileType = "unknown"
    ProfileTypeModernSSO         ProfileType = "modern_sso"
    ProfileTypeLegacySSO         ProfileType = "legacy_sso"
    ProfileTypeStaticKeys        ProfileType = "static_keys"
    ProfileTypeAssumeRole        ProfileType = "assume_role"
    ProfileTypeSSOAssumeRole     ProfileType = "sso_assume_role"  // NEW
)
```

Detection logic:
```go
func (pi *ProfileInspector) detectProfileType(section *ini.Section) ProfileType {
    hasSSO := section.HasKey("sso_session")
    hasLegacySSO := section.HasKey("sso_start_url")
    hasRoleArn := section.HasKey("role_arn")
    hasSourceProfile := section.HasKey("source_profile")

    // SSO + Role Assumption (chaining)
    if (hasSSO || hasLegacySSO) && hasRoleArn {
        return ProfileTypeSSOAssumeRole
    }

    // Pure Role Assumption
    if hasRoleArn && hasSourceProfile {
        return ProfileTypeAssumeRole
    }

    // Modern SSO
    if hasSSO {
        return ProfileTypeModernSSO
    }

    // Legacy SSO
    if hasLegacySSO {
        return ProfileTypeLegacySSO
    }

    // ... rest of detection
}
```

### 2. Add Bearer Token Support

For profiles that only need bearer tokens, make account_id and role_name optional:

```go
func (pi *ProfileInspector) validateModernSSO(section *ini.Section, info *ProfileInfo) {
    // Check if this is a bearer token profile
    isBearerToken := section.HasKey("bearer_token_provider") // Or similar flag

    requiredProfileFields := map[string]*string{
        "sso_session": &info.SSOSession,
    }

    // Only require these for non-bearer-token profiles
    if !isBearerToken {
        requiredProfileFields["sso_account_id"] = &info.SSOAccountID
        requiredProfileFields["sso_role_name"] = &info.SSORoleName
    }

    // ... validation
}
```

### 3. Validate SSO Registration Scopes

Add validation for `sso_registration_scopes`:

```go
func validateSSORegistrationScopes(scopes string) error {
    if scopes == "" {
        return nil // Optional field
    }

    // At minimum, must include sso:account:access
    if !strings.Contains(scopes, "sso:account:access") {
        return fmt.Errorf("sso_registration_scopes must include 'sso:account:access'")
    }

    return nil
}
```

### 4. Detect Configuration Conflicts

Add conflict detection:

```go
func (pi *ProfileInspector) detectConflicts(section *ini.Section) []string {
    conflicts := []string{}

    // Can't have both source_profile and credential_source
    if section.HasKey("source_profile") && section.HasKey("credential_source") {
        conflicts = append(conflicts, "Cannot specify both source_profile and credential_source")
    }

    // Legacy SSO shouldn't have sso_session
    if section.HasKey("sso_start_url") && section.HasKey("sso_session") {
        conflicts = append(conflicts, "Cannot mix legacy SSO (sso_start_url) with modern SSO (sso_session)")
    }

    return conflicts
}
```

### 5. Improve Error Messages

Current (after fix):
```
Profile incomplete. Missing: sso_start_url (in sso-session 'my-sso'), sso_region (in sso-session 'my-sso')
```

Better:
```
Profile incomplete:
  Profile section missing: (none)
  SSO session 'my-sso' missing:
    - sso_start_url (e.g., https://mycompany.awsapps.com/start)
    - sso_region (e.g., us-east-1)
```

## Validation Decision Tree

```
Is AWS config file present?
├─ NO → Error: "Config file not found at ~/.aws/config"
└─ YES
   └─ Does profile section exist?
      ├─ NO → Error: "Profile 'name' not found"
      └─ YES → Detect profile type
         ├─ Has sso_session + role_arn? → SSO + AssumeRole
         │  ├─ Validate SSO fields
         │  ├─ Validate sso-session section
         │  └─ Validate role assumption fields
         │
         ├─ Has sso_session? → Modern SSO
         │  ├─ Required: sso_session, sso_account_id, sso_role_name
         │  └─ Validate referenced sso-session section
         │     └─ Required: sso_start_url, sso_region
         │
         ├─ Has sso_start_url? → Legacy SSO
         │  └─ Required: sso_start_url, sso_region, sso_account_id, sso_role_name
         │
         ├─ Has role_arn + source_profile? → Role Assumption
         │  └─ Validate source_profile exists
         │
         ├─ Has aws_access_key_id? → Static Keys
         │  └─ Let AWS SDK validate
         │
         └─ Otherwise → Unknown
            └─ Error: "Cannot determine profile type"
```

## Testing Scenarios

### Valid Configurations

1. **Modern SSO - Complete:**
```ini
[profile dev]
sso_session = main
sso_account_id = 123456789012
sso_role_name = DevRole

[sso-session main]
sso_start_url = https://company.awsapps.com/start
sso_region = us-east-1
```

2. **Modern SSO - With Optional Fields:**
```ini
[profile dev]
sso_session = main
sso_account_id = 123456789012
sso_role_name = DevRole
region = us-west-2
output = json

[sso-session main]
sso_start_url = https://company.awsapps.com/start
sso_region = us-east-1
sso_registration_scopes = sso:account:access
```

3. **Legacy SSO:**
```ini
[profile old-dev]
sso_start_url = https://company.awsapps.com/start
sso_region = us-east-1
sso_account_id = 123456789012
sso_role_name = DevRole
```

4. **SSO + Role Assumption:**
```ini
[profile sso-base]
sso_session = main
sso_account_id = 111111111111
sso_role_name = BaseRole

[profile cross-account]
role_arn = arn:aws:iam::222222222222:role/CrossAccountRole
source_profile = sso-base

[sso-session main]
sso_start_url = https://company.awsapps.com/start
sso_region = us-east-1
```

### Invalid Configurations

1. **Modern SSO - Missing sso-session:**
```ini
[profile dev]
sso_session = nonexistent  # ❌ sso-session doesn't exist
sso_account_id = 123456789012
sso_role_name = DevRole
```
Expected: "sso_session_not_found (in sso-session 'nonexistent')"

2. **Modern SSO - Incomplete sso-session:**
```ini
[profile dev]
sso_session = main
sso_account_id = 123456789012
sso_role_name = DevRole

[sso-session main]
sso_region = us-east-1  # ❌ Missing sso_start_url
```
Expected: "sso_start_url (in sso-session 'main')"

3. **Modern SSO - Missing profile fields:**
```ini
[profile dev]
sso_session = main  # ❌ Missing sso_account_id and sso_role_name

[sso-session main]
sso_start_url = https://company.awsapps.com/start
sso_region = us-east-1
```
Expected: "sso_account_id, sso_role_name"

4. **Mixed Legacy + Modern:**
```ini
[profile confused]
sso_start_url = https://company.awsapps.com/start  # ❌ Conflict
sso_session = main  # ❌ Conflict
sso_account_id = 123456789012
sso_role_name = DevRole
```
Expected: "Cannot mix legacy SSO with modern SSO"

## Summary

### Current State ✅
- Correctly validates modern SSO profiles
- Correctly validates legacy SSO profiles
- Properly reports missing fields from sso-session sections (after fix)
- Validates field formats (URLs, regions, account IDs)

### Gaps ⚠️
- Doesn't handle SSO + Role Assumption profiles
- Doesn't check for configuration conflicts
- Doesn't support bearer token authentication
- Doesn't validate optional advanced settings
- Could provide more helpful error messages

### Recommendation
The current implementation is **sufficient for basic SSO validation** but could be enhanced to handle edge cases and provide better user experience. The recent fix to show specific missing sso-session fields addresses the main user complaint.

For most users, the current validation will work well. Consider the enhancements above if users report issues with:
- Role assumption chaining from SSO profiles
- Bearer token authentication
- Configuration conflicts causing unexpected behavior
