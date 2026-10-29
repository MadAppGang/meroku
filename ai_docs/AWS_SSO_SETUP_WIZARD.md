# AWS SSO Setup Wizard - Implementation Plan

## Overview

Transform the AWS profile validation experience from **error-throwing validation** into an **intelligent setup wizard** that automatically detects, prompts for missing information, and fixes AWS SSO profile configuration.

## User Experience Goals

1. **Zero Manual File Editing** - Never ask users to edit `~/.aws/config` manually
2. **Smart Detection** - Detect what's already configured and only ask for missing pieces
3. **One-Command Setup** - Automatically run `aws sso login` after config is created
4. **Multi-Environment Support** - Handle dev, staging, prod in one session
5. **Sensible Defaults** - Pre-fill common values (us-east-1, AdministratorAccess)
6. **Context Awareness** - Use data from YAML files to pre-fill prompts

## Ideal User Flow

### Scenario 1: Brand New User (No AWS Config)

```
$ ./meroku

🔍 Checking AWS configuration...

❌ No AWS profiles found in ~/.aws/config

Let's set up your AWS SSO configuration!

📋 I need a few details to get started:

? SSO Start URL (e.g., https://mycompany.awsapps.com/start): [user enters]
? SSO Region [us-east-1]: [Enter]
? SSO Session Name [mycompany]: [Enter]

✅ SSO session 'mycompany' configured

Now let's create profiles for your environments:

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Environment: dev
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

ℹ️  Detected from dev.yaml:
   • Region: us-east-1

? AWS Account ID: [user enters 12 digits]
? Role Name [AdministratorAccess]: [Enter]
? Default Region [us-east-1]: [Enter]

✅ Writing profile 'dev' to ~/.aws/config...
✅ Profile 'dev' created successfully!

🔐 Authenticating with AWS SSO...
[Runs: aws sso login --profile dev]
[Browser opens for SSO]

✅ Profile 'dev' is ready!

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

✅ AWS SSO setup complete! You're ready to deploy.
```

### Scenario 2: Existing Config, Missing Profile

```
$ ./meroku

🔍 Checking AWS configuration...

❌ Profile 'staging' not found in ~/.aws/config

✅ Found existing SSO session: mycompany (https://mycompany.awsapps.com/start)

Let's create profile 'staging':

ℹ️  Detected from staging.yaml:
   • Region: us-west-2
   • Environment: staging

? Use existing SSO session 'mycompany'? [Y/n]: [Enter]
? AWS Account ID: [user enters]
? Role Name [AdministratorAccess]: [Enter]
? Default Region [us-west-2]: [Enter]

✅ Writing profile 'staging' to ~/.aws/config...
✅ Profile 'staging' created!

🔐 Running: aws sso login --profile staging
[Browser opens]

✅ Profile 'staging' is ready!
```

### Scenario 3: Profile Exists, Incomplete/Invalid

```
$ ./meroku

🔍 Checking AWS configuration...

⚠️  Profile 'dev' found but missing required fields:
   • sso_account_id (not set)
   • region (not set)

Let's fix this configuration:

ℹ️  Current profile 'dev':
   • SSO Session: mycompany
   • SSO Region: us-east-1

? AWS Account ID: [pre-filled from dev.yaml if available]
? Default Region [us-east-1]: [pre-filled from dev.yaml]

✅ Updating profile 'dev' in ~/.aws/config...
✅ Profile 'dev' updated successfully!

🔐 Testing credentials...
[Runs: aws sts get-caller-identity --profile dev]

✅ Profile 'dev' is working correctly!
```

## Architecture

### Component Structure

```
┌─────────────────────────────────────────────────────┐
│         AWS SSO Setup Wizard Entry Point            │
│               (aws_sso_setup_wizard.go)             │
└────────────────────┬────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────┐
│           Profile Inspector & Analyzer               │
│   • Detect existing profiles and SSO sessions        │
│   • Parse ~/.aws/config file                         │
│   • Validate profile completeness                    │
│   • Match profiles to YAML environment configs       │
└────────────────────┬────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────┐
│         Interactive Setup Wizard (Bubble Tea)        │
│   • Smart prompts for missing information            │
│   • Pre-fill from YAML configs and existing data     │
│   • Show detected vs. required info                  │
│   • Multi-step form with progress indicators         │
└────────────────────┬────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────┐
│            Config File Writer & Updater              │
│   • Safely update ~/.aws/config                      │
│   • Preserve existing profiles                       │
│   • Handle SSO sessions and profile sections         │
│   • Backup before making changes                     │
└────────────────────┬────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────┐
│          Auto-Login & Validation                     │
│   • Run 'aws sso login --profile <name>'             │
│   • Test credentials with STS GetCallerIdentity      │
│   • Update YAML files with account_id and profile    │
│   • Confirm setup success                            │
└─────────────────────────────────────────────────────┘
```

## Implementation Details

### 1. Profile Inspector

**File:** `app/aws_sso_profile_inspector.go`

```go
type ProfileInspector struct {
    ConfigPath string
}

type ProfileAnalysis struct {
    ProfileName      string
    Exists           bool
    IsComplete       bool
    MissingFields    []string
    SSOSessionName   string
    SSOStartURL      string
    SSORegion        string
    SSOAccountID     string
    RoleName         string
    Region           string
}

type SSOSessionInfo struct {
    Name               string
    StartURL           string
    Region             string
    RegistrationScopes string
    Exists             bool
}

// InspectProfile analyzes a profile and returns what's configured and what's missing
func (pi *ProfileInspector) InspectProfile(profileName string) ProfileAnalysis

// ListSSOSessions returns all SSO sessions configured in ~/.aws/config
func (pi *ProfileInspector) ListSSOSessions() []SSOSessionInfo

// InspectEnvironmentProfiles checks all profiles needed for project/*.yaml files
func (pi *ProfileInspector) InspectEnvironmentProfiles() map[string]ProfileAnalysis
```

**Key Features:**
- Parse `~/.aws/config` to detect existing profiles and SSO sessions
- Identify missing or incomplete profile configurations
- Return structured analysis for decision-making
- Handle both SSO-style profiles and legacy IAM profiles

### 2. Setup Wizard TUI

**File:** `app/aws_sso_setup_wizard_tui.go`

```go
type SetupWizardModel struct {
    step              int
    environments      []string
    currentEnv        string
    profileAnalysis   map[string]ProfileAnalysis
    ssoSessions       []SSOSessionInfo
    userInputs        map[string]map[string]string // env -> field -> value
    yamlConfigs       map[string]Env               // env -> Env struct
}

// Main wizard entry point
func RunSSOSetupWizard(environments []string) error

// Step functions
func (m SetupWizardModel) promptSSOSession() tea.Cmd
func (m SetupWizardModel) promptAccountID(env string) tea.Cmd
func (m SetupWizardModel) promptRoleName(env string) tea.Cmd
func (m SetupWizardModel) promptRegion(env string) tea.Cmd
func (m SetupWizardModel) confirmChanges() tea.Cmd
```

**UX Principles:**
- **Progressive Disclosure**: Only ask for missing information
- **Smart Defaults**: Pre-fill from YAML configs and existing profiles
- **Context Display**: Show what we detected vs. what we need
- **Visual Progress**: Step indicator (1/4, 2/4, etc.)
- **Confirmation**: Summary screen before writing changes

**Example Screen:**

```
┌─────────────────────────────────────────────────────┐
│ AWS SSO Setup Wizard                       Step 2/4 │
├─────────────────────────────────────────────────────┤
│                                                      │
│ Setting up profile: dev                             │
│                                                      │
│ ℹ️  Detected Configuration:                          │
│   • Region: us-east-1 (from dev.yaml)                │
│   • SSO Session: mycompany                           │
│   • SSO Region: us-east-1                            │
│                                                      │
│ ❓ Missing Information:                              │
│   • AWS Account ID (required)                        │
│                                                      │
│ ┌──────────────────────────────────────────┐        │
│ │ AWS Account ID: 123456789012_            │        │
│ └──────────────────────────────────────────┘        │
│                                                      │
│ Role Name [AdministratorAccess]: ____________       │
│                                                      │
│ Default Region [us-east-1]: ____________             │
│                                                      │
│ [Continue]  [Skip]  [Cancel]                        │
│                                                      │
└─────────────────────────────────────────────────────┘
```

### 3. Config File Writer

**File:** `app/aws_config_writer.go`

```go
type ConfigWriter struct {
    ConfigPath   string
    BackupPath   string
}

type ProfileConfig struct {
    ProfileName     string
    SSOSessionName  string
    SSOAccountID    string
    SSORoleName     string
    Region          string
}

type SSOSessionConfig struct {
    Name               string
    StartURL           string
    Region             string
    RegistrationScopes string
}

// CreateOrUpdateProfile writes or updates a profile in ~/.aws/config
func (cw *ConfigWriter) CreateOrUpdateProfile(profile ProfileConfig) error

// CreateOrUpdateSSOSession writes or updates an SSO session
func (cw *ConfigWriter) CreateOrUpdateSSOSession(session SSOSessionConfig) error

// BackupConfig creates a timestamped backup of ~/.aws/config
func (cw *ConfigWriter) BackupConfig() error

// ValidateConfig checks if the config file is valid after changes
func (cw *ConfigWriter) ValidateConfig() error
```

**Key Features:**
- **Safe Updates**: Backup before making changes
- **Preserve Existing**: Don't touch other profiles
- **Atomic Writes**: Write to temp file, then rename
- **Validation**: Verify config is parseable after update
- **Idempotent**: Re-running doesn't create duplicates

**Config Format Generated:**

```ini
[sso-session mycompany]
sso_start_url = https://mycompany.awsapps.com/start
sso_region = us-east-1
sso_registration_scopes = sso:account:access

[profile dev]
credential_process = aws configure export-credentials --profile dev
sso_session = mycompany
sso_account_id = 123456789012
sso_role_name = AdministratorAccess
region = us-east-1

[profile staging]
credential_process = aws configure export-credentials --profile staging
sso_session = mycompany
sso_account_id = 987654321098
sso_role_name = AdministratorAccess
region = us-west-2
```

### 4. Auto-Login & Validation

**File:** `app/aws_sso_auto_login.go`

```go
// PerformSSOLogin runs 'aws sso login' for the given profile
func PerformSSOLogin(profileName string) error

// ValidateProfileCredentials tests if a profile works by calling STS
func ValidateProfileCredentials(profileName string) (*sts.GetCallerIdentityOutput, error)

// UpdateYAMLWithProfile updates environment YAML files with profile info
func UpdateYAMLWithProfile(envName, profileName, accountID string) error

// RunPostSetupValidation performs full validation after setup
func RunPostSetupValidation(profiles []string) error
```

**Post-Setup Flow:**

```
1. Write profile to ~/.aws/config
2. Run: aws sso login --profile <name>
3. Wait for user to complete browser auth
4. Test: aws sts get-caller-identity --profile <name>
5. If success:
   - Extract account ID from response
   - Update project/<env>.yaml with:
     • aws_profile: <name>
     • account_id: <account_id>
   - Save YAML file
6. Display success message
```

## Integration Points

### 1. Pre-Flight Check Enhancement

**Current:** `app/aws_preflight.go` - Only validates and errors

**Enhanced:** Offer to run setup wizard when validation fails

```go
func AWSPreflightCheck(env Env) error {
    // Existing validation logic...

    if awsProfile == "" {
        fmt.Println("❌ AWS profile not configured")

        var runSetup string
        huh.NewConfirm().
            Title("Would you like to set up AWS SSO now?").
            Value(&runSetup).
            Run()

        if runSetup {
            return RunSSOSetupWizard([]string{env.Env})
        }

        return fmt.Errorf("AWS profile required")
    }

    // Continue validation...
}
```

### 2. Main Menu Integration

**File:** `app/main.go`

Add new menu option:

```go
choices := []string{
    "Deploy Infrastructure",
    "Plan Infrastructure Changes",
    "⚙️  AWS SSO Setup Wizard",  // NEW
    "🔐 Select AWS Profile",
    "🌐 DNS Setup",
    "🤖 Fix with AI (Claude Code)",
    "📊 View Infrastructure State",
    "🗑️  Destroy Infrastructure",
    "Exit",
}
```

### 3. Environment Selector Enhancement

**File:** `app/env_selector.go`

Detect missing profiles before environment selection:

```go
func selectEnvironment() (string, error) {
    envs := getEnvironments()

    // Check if any environments need profile setup
    needsSetup := []string{}
    for _, env := range envs {
        if !hasValidProfile(env) {
            needsSetup = append(needsSetup, env)
        }
    }

    if len(needsSetup) > 0 {
        fmt.Printf("⚠️  Some environments need AWS profile setup: %s\n",
            strings.Join(needsSetup, ", "))

        var runSetup bool
        huh.NewConfirm().
            Title("Set up AWS profiles now?").
            Value(&runSetup).
            Run()

        if runSetup {
            return "", RunSSOSetupWizard(needsSetup)
        }
    }

    // Continue with environment selection...
}
```

## Data Flow

### Input Sources (Priority Order)

1. **Existing ~/.aws/config** - What's already configured
2. **project/*.yaml files** - Region, environment name, existing account_id
3. **User Input** - Only for truly missing information
4. **Sensible Defaults** - us-east-1, AdministratorAccess, etc.

### Information Collection Matrix

| Field | Source Priority |
|-------|----------------|
| SSO Session Name | Existing config → User prompt → Default "mycompany" |
| SSO Start URL | Existing config → User prompt (required) |
| SSO Region | Existing config → User prompt → Default "us-east-1" |
| Profile Name | Environment name (dev, staging, prod) |
| Account ID | YAML file → User prompt (required) |
| Role Name | YAML → User prompt → Default "AdministratorAccess" |
| Region | YAML → User prompt → Default "us-east-1" |

### Output Targets

1. **~/.aws/config** - Profile and SSO session configuration
2. **project/<env>.yaml** - Update `aws_profile` and `account_id` fields
3. **Environment variable** - Set `AWS_PROFILE` for current session

## Error Handling

### Graceful Degradation

```go
// If SSO login fails, still save profile
if err := PerformSSOLogin(profileName); err != nil {
    fmt.Printf("⚠️  SSO login failed: %v\n", err)
    fmt.Printf("ℹ️  Profile saved. Run manually: aws sso login --profile %s\n", profileName)
    // Don't return error - profile is still useful
}
```

### Validation Failures

```go
// If credential validation fails, offer troubleshooting
if err := ValidateProfileCredentials(profileName); err != nil {
    fmt.Printf("❌ Profile validation failed: %v\n", err)
    fmt.Println("\n🔧 Troubleshooting steps:")
    fmt.Println("1. Verify SSO session: aws sso login --profile", profileName)
    fmt.Println("2. Check account ID matches your AWS console")
    fmt.Println("3. Verify role name is assigned to your user")
    fmt.Println("4. Check region is correct")

    var retry bool
    huh.NewConfirm().
        Title("Would you like to retry validation?").
        Value(&retry).
        Run()

    if retry {
        return ValidateProfileCredentials(profileName)
    }
}
```

### Backup and Recovery

```go
// Always backup before modifying config
backupPath := fmt.Sprintf("%s.backup_%s", configPath, time.Now().Format("20060102_150405"))
if err := copyFile(configPath, backupPath); err != nil {
    return fmt.Errorf("failed to backup config: %w", err)
}

fmt.Printf("✅ Backup created: %s\n", backupPath)

// If write fails, restore from backup
if err := writeConfig(newConfig); err != nil {
    fmt.Printf("❌ Failed to write config: %v\n", err)
    fmt.Printf("🔄 Restoring from backup...\n")
    copyFile(backupPath, configPath)
    return err
}
```

## Multi-Environment Workflow

### Batch Setup Mode

When setting up multiple environments:

```
🔍 Detected 3 environments: dev, staging, prod

Let's set them up one by one:

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Environment 1/3: dev
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

? AWS Account ID: 123456789012
? Role Name [AdministratorAccess]:
? Region [us-east-1]:

✅ Profile 'dev' configured
🔐 Running SSO login...
✅ Authentication successful!

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Environment 2/3: staging
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

? AWS Account ID: 987654321098
? Use same SSO session? [Y/n]: Y
? Role Name [AdministratorAccess]:
? Region [us-west-2]:

✅ Profile 'staging' configured
✅ Authentication successful! (using existing session)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Environment 3/3: prod
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

? AWS Account ID: 456789012345
? Use same SSO session? [Y/n]: Y
? Role Name [AdministratorAccess]:
? Region [us-east-1]:

✅ Profile 'prod' configured
✅ Authentication successful! (using existing session)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

✅ Setup complete! All 3 environments configured.

📝 Summary:
   • SSO Session: mycompany
   • Profiles created: dev, staging, prod
   • YAML files updated with account IDs and profiles

🚀 You're ready to deploy!
```

### Smart SSO Session Reuse

```go
// After first environment is configured
if len(previousSSO) > 0 {
    var reuseSSOSession bool
    huh.NewConfirm().
        Title(fmt.Sprintf("Use existing SSO session '%s'?", previousSSO)).
        Value(&reuseSSOSession).
        Run()

    if reuseSSOSession {
        // Skip SSO setup, reuse session
        // Only prompt for account-specific fields
    }
}
```

## Testing Strategy

### Manual Test Scenarios

1. **Fresh Install**
   - Delete ~/.aws directory
   - Run wizard
   - Verify complete setup flow

2. **Partial Config**
   - Create SSO session only
   - Run wizard
   - Verify profile creation

3. **Missing Fields**
   - Create profile with missing sso_account_id
   - Run wizard
   - Verify field addition

4. **Multi-Environment**
   - Have dev.yaml and prod.yaml
   - Run wizard
   - Verify batch setup

5. **Error Recovery**
   - Provide invalid account ID
   - Verify validation catches it
   - Test retry flow

### Automated Tests

```go
// app/aws_sso_setup_wizard_test.go

func TestProfileInspector_ParseConfig(t *testing.T)
func TestProfileInspector_DetectMissingFields(t *testing.T)
func TestConfigWriter_CreateProfile(t *testing.T)
func TestConfigWriter_UpdateProfile(t *testing.T)
func TestConfigWriter_BackupRestore(t *testing.T)
func TestSSOLogin_MockSuccess(t *testing.T)
func TestSSOLogin_MockFailure(t *testing.T)
func TestYAMLUpdate_AccountID(t *testing.T)
func TestYAMLUpdate_PreservesExisting(t *testing.T)
```

## CLI Commands

Add standalone commands for flexibility:

```bash
# Interactive wizard for all environments
./meroku sso setup

# Setup specific environment
./meroku sso setup dev

# Validate existing profiles
./meroku sso validate

# List configured profiles and their status
./meroku sso status

# Test profile credentials
./meroku sso test dev

# Re-login to refresh token
./meroku sso refresh dev
```

## Documentation Updates

### 1. Update CLAUDE.md

Add section:

```markdown
## AWS SSO Setup

The infrastructure includes an **AWS SSO Setup Wizard** that automatically configures your AWS profiles.

### Quick Start

```bash
./meroku sso setup
```

The wizard will:
1. Detect your existing AWS configuration
2. Prompt only for missing information
3. Write configuration to ~/.aws/config
4. Run `aws sso login` automatically
5. Update your YAML files with account IDs

### Manual Setup

If you prefer manual configuration, see:
[AWS SSO Configuration Guide](./ai_docs/AWS_SSO_SETUP_WIZARD.md)
```

### 2. Create User Guide

**File:** `ai_docs/AWS_SSO_SETUP_USER_GUIDE.md`

- Step-by-step tutorial with screenshots
- Common issues and troubleshooting
- FAQ section
- Video walkthrough link

### 3. Update README.md

Add to "Getting Started" section:

```markdown
## 3. Configure AWS SSO

Run the setup wizard to configure your AWS credentials:

```bash
./meroku sso setup
```

You'll need:
- Your organization's SSO start URL (e.g., https://mycompany.awsapps.com/start)
- AWS account IDs for each environment
- IAM role name (usually AdministratorAccess)
```

## Success Metrics

### User Experience Metrics

- **Time to First Deployment**: Reduce from 30min → 5min
- **Setup Completion Rate**: Increase from 60% → 95%
- **Support Tickets**: Reduce AWS config issues by 80%
- **User Satisfaction**: "Setup was easy" rating > 4.5/5

### Technical Metrics

- **Validation Success Rate**: > 95% of configurations work first try
- **Error Recovery Rate**: > 90% of users recover from errors without support
- **Multi-Env Setup Time**: < 2 minutes for 3 environments
- **Config Corruption Rate**: 0% (due to backup system)

## Implementation Timeline

### Phase 1: Core Components (Week 1)
- [ ] Profile Inspector implementation
- [ ] Config Writer with backup system
- [ ] Basic validation functions
- [ ] Unit tests for core logic

### Phase 2: Interactive Wizard (Week 2)
- [ ] Bubble Tea TUI for setup flow
- [ ] Smart prompts with pre-filling
- [ ] Multi-step form with progress
- [ ] Integration with existing menu

### Phase 3: Auto-Login & Integration (Week 3)
- [ ] SSO login automation
- [ ] Credential validation
- [ ] YAML file updates
- [ ] Pre-flight check integration

### Phase 4: Polish & Documentation (Week 4)
- [ ] Error handling and recovery
- [ ] Multi-environment batch mode
- [ ] User documentation
- [ ] Video tutorial
- [ ] Beta testing with real users

## Future Enhancements

### Advanced Features (Post-MVP)

1. **Profile Switching**
   - Quick switcher for multi-account setups
   - Remember last used profile per environment

2. **Organization Detection**
   - Auto-detect SSO start URL from common patterns
   - Suggest based on email domain

3. **Permission Validation**
   - Pre-check if role has required IAM permissions
   - Warn about missing permissions before deployment

4. **Team Sharing**
   - Export/import profile configurations (without secrets)
   - Share SSO session configs with team

5. **Cloud Formation Stack Sets**
   - Suggest StackSets for multi-account setups
   - Auto-configure delegation roles

6. **Integration with AWS Organizations**
   - List all accounts in organization
   - Batch setup for all accounts

## Appendix: Existing Code Audit

### What We Have Already

✅ **Profile Selection** (`aws_selector.go`)
- `selectAWSProfile()` - Interactive profile picker
- `getLocalAWSProfiles()` - Parse ~/.aws/config
- `createAWSProfile()` - Profile creation with forms
- `createSSOSession()` - SSO session setup
- `appendProfileToConfig()` - Write profile to config
- `getAWSAccountID()` - Get account ID via STS
- `runAWSSSO()` - Run aws sso login command

✅ **Validation** (`aws_preflight.go`)
- `AWSPreflightCheck()` - Pre-flight validation
- `validateAWSCredentials()` - Credential check
- `refreshSSOToken()` - Auto SSO refresh

### What Needs Enhancement

🔧 **Profile Inspector** (NEW)
- Structured analysis of profile completeness
- Detect missing fields systematically
- Return actionable recommendations

🔧 **Setup Wizard TUI** (NEW)
- Bubble Tea multi-step form
- Pre-fill from YAML configs
- Smart defaults and suggestions
- Progress indicators

🔧 **Config Writer** (ENHANCE)
- Safe updates with backup
- Atomic writes
- Validation after write
- Better error messages

🔧 **Integration** (ENHANCE)
- Call wizard from pre-flight check
- Add to main menu
- Standalone CLI commands

### Code Reuse Opportunities

We can reuse:
- `getLocalAWSProfiles()` - Profile parsing logic
- `getAWSAccountID()` - Account ID retrieval
- `runAWSSSO()` - SSO login execution
- `appendProfileToConfig()` - Config writing (with enhancements)
- Bubble Tea forms from existing UI patterns
- YAML loading/saving from `model.go`

We need to create:
- Profile completeness checker
- Multi-step wizard orchestration
- Backup and recovery system
- Batch setup for multiple environments
- Smart pre-filling logic

## Summary

This implementation plan transforms AWS SSO setup from a **frustrating manual process** into a **guided, automated experience**. Key principles:

1. **Detect Before Asking** - Only prompt for truly missing information
2. **Automate Everything** - No manual file editing required
3. **Fail Gracefully** - Recover from errors automatically
4. **Guide the User** - Clear progress and next steps
5. **Multi-Environment First** - Handle dev/staging/prod in one flow

The wizard integrates seamlessly with existing workflows while dramatically improving the new user experience.
