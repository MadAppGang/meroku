# AWS SSO Setup Wizard - Flow Diagram

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                       USER ENTRY POINTS                          │
│                                                                  │
│  1. Main Menu → "AWS SSO Setup Wizard"                          │
│  2. Pre-flight Check → Profile validation fails                 │
│  3. Environment Selector → Missing profile detected             │
│  4. CLI Command → ./meroku sso setup                            │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                   PROFILE INSPECTOR                              │
│                  (Detection Phase)                               │
│                                                                  │
│  • Parse ~/.aws/config file                                     │
│  • List existing SSO sessions                                   │
│  • List existing profiles                                       │
│  • Load project/*.yaml files                                    │
│  • Match profiles to environments                               │
│  • Identify missing/incomplete configurations                   │
│                                                                  │
│  Output: ProfileAnalysis map[env]→analysis                      │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                 DECISION TREE                                    │
│                                                                  │
│  ┌──────────────────────────────────────────┐                  │
│  │ No SSO sessions exist?                    │                  │
│  └───┬───────────────────────────────┬──────┘                  │
│      │ YES                            │ NO                       │
│      ▼                                ▼                          │
│  ┌─────────────────┐      ┌────────────────────────┐          │
│  │ Create SSO      │      │ Select/Reuse existing  │          │
│  │ Session Wizard  │      │ SSO session            │          │
│  └────────┬────────┘      └────────┬───────────────┘          │
│           │                         │                            │
│           └─────────────┬───────────┘                           │
│                         │                                        │
│                         ▼                                        │
│  ┌──────────────────────────────────────────┐                  │
│  │ For each environment:                     │                  │
│  │   - Missing profile?                      │                  │
│  │   - Incomplete profile?                   │                  │
│  │   - Valid profile?                        │                  │
│  └───┬──────────────────────────────────────┘                  │
│      │                                                           │
│      ▼                                                           │
│  ┌──────────────────────────────────────────┐                  │
│  │ Build setup workflow:                     │                  │
│  │   • Collect missing fields                │                  │
│  │   • Pre-fill from YAML                    │                  │
│  │   • Apply defaults                        │                  │
│  └───┬──────────────────────────────────────┘                  │
└──────┼──────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│              INTERACTIVE WIZARD (Bubble Tea TUI)                 │
│                                                                  │
│  FOR EACH ENVIRONMENT:                                          │
│                                                                  │
│  ┌────────────────────────────────────────────┐                │
│  │ Step 1: Environment Overview                │                │
│  │                                              │                │
│  │ Environment: dev                             │                │
│  │ Detected:                                    │                │
│  │   ✓ Region: us-east-1 (from dev.yaml)       │                │
│  │   ✓ SSO Session: mycompany                  │                │
│  │ Missing:                                     │                │
│  │   ✗ Account ID                               │                │
│  │                                              │                │
│  │ [Continue to setup]                          │                │
│  └────────────────────────────────────────────┘                │
│                    ▼                                             │
│  ┌────────────────────────────────────────────┐                │
│  │ Step 2: Collect Missing Fields              │                │
│  │                                              │                │
│  │ AWS Account ID: [____________]               │                │
│  │   (12-digit number)                          │                │
│  │                                              │                │
│  │ Role Name: [AdministratorAccess]             │                │
│  │   (Pre-filled with default)                  │                │
│  │                                              │                │
│  │ Default Region: [us-east-1]                  │                │
│  │   (Pre-filled from YAML)                     │                │
│  │                                              │                │
│  │ [Continue] [Skip] [Cancel]                   │                │
│  └────────────────────────────────────────────┘                │
│                    ▼                                             │
│  ┌────────────────────────────────────────────┐                │
│  │ Step 3: Confirmation                         │                │
│  │                                              │                │
│  │ Ready to configure profile 'dev':            │                │
│  │   • SSO Session: mycompany                   │                │
│  │   • Account ID: 123456789012                 │                │
│  │   • Role: AdministratorAccess                │                │
│  │   • Region: us-east-1                        │                │
│  │                                              │                │
│  │ Changes will be written to ~/.aws/config     │                │
│  │                                              │                │
│  │ [Confirm] [Edit] [Cancel]                    │                │
│  └────────────────────────────────────────────┘                │
└──────┬──────────────────────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────────┐
│                    CONFIG WRITER                                 │
│                  (Write Phase)                                   │
│                                                                  │
│  Step 1: Backup existing config                                │
│    ~/.aws/config → ~/.aws/config.backup_20251022_153045        │
│                                                                  │
│  Step 2: Write/Update SSO session (if new)                     │
│    [sso-session mycompany]                                      │
│    sso_start_url = https://mycompany.awsapps.com/start         │
│    sso_region = us-east-1                                       │
│    sso_registration_scopes = sso:account:access                 │
│                                                                  │
│  Step 3: Write/Update profile                                  │
│    [profile dev]                                                │
│    credential_process = aws configure export-credentials ...    │
│    sso_session = mycompany                                      │
│    sso_account_id = 123456789012                                │
│    sso_role_name = AdministratorAccess                          │
│    region = us-east-1                                           │
│                                                                  │
│  Step 4: Validate config syntax                                │
│    Parse ~/.aws/config to ensure valid INI format              │
│                                                                  │
│  ✓ Config written successfully                                 │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                   AUTO-LOGIN & VALIDATION                        │
│                  (Authentication Phase)                          │
│                                                                  │
│  Step 1: Run SSO Login                                          │
│    Execute: aws sso login --profile dev                         │
│    → Opens browser for user authentication                      │
│    → User logs in via AWS SSO portal                            │
│    → Token saved to ~/.aws/sso/cache/                           │
│                                                                  │
│  Step 2: Wait for completion                                    │
│    Monitor command exit status                                  │
│                                                                  │
│  Step 3: Validate credentials                                   │
│    Call: STS.GetCallerIdentity()                                │
│    Response:                                                     │
│      • Account: 123456789012                                    │
│      • Arn: arn:aws:sts::123456789012:assumed-role/...         │
│      • UserId: AROAEXAMPLE:user@example.com                     │
│                                                                  │
│  Step 4: Update YAML file                                       │
│    project/dev.yaml:                                            │
│      account_id: "123456789012"                                 │
│      aws_profile: "dev"                                         │
│                                                                  │
│  Step 5: Set environment variable                               │
│    export AWS_PROFILE=dev                                       │
│                                                                  │
│  ✓ Profile 'dev' is ready!                                      │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                    SUCCESS SUMMARY                               │
│                                                                  │
│  ┌────────────────────────────────────────────┐                │
│  │ ✓ AWS SSO Setup Complete!                  │                │
│  │                                              │                │
│  │ Configured profiles:                         │                │
│  │   • dev (Account: 123456789012)             │                │
│  │   • staging (Account: 987654321098)         │                │
│  │   • prod (Account: 456789012345)            │                │
│  │                                              │                │
│  │ SSO Session: mycompany                       │                │
│  │ YAML files updated with account IDs          │                │
│  │                                              │                │
│  │ You're ready to deploy!                      │                │
│  │                                              │                │
│  │ [Continue to deployment] [Exit]              │                │
│  └────────────────────────────────────────────┘                │
└─────────────────────────────────────────────────────────────────┘
```

## Detailed Flow: Brand New User

```
START: ./meroku
    │
    ▼
┌─────────────────────────────────┐
│ Main Menu                        │
│ > AWS SSO Setup Wizard           │
└────────┬────────────────────────┘
         │
         ▼
┌─────────────────────────────────┐
│ Inspector detects:               │
│ • No ~/.aws/config exists        │
│ • 3 YAML files: dev, staging,    │
│   prod                           │
└────────┬────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────┐
│ Welcome Screen                                   │
│                                                  │
│ No AWS configuration found!                      │
│                                                  │
│ I'll help you set up AWS SSO for:               │
│   • dev (us-east-1)                              │
│   • staging (us-west-2)                          │
│   • prod (us-east-1)                             │
│                                                  │
│ This will take about 2 minutes.                  │
│                                                  │
│ [Get Started]                                    │
└─────────┬───────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────┐
│ SSO Session Setup                                │
│                                                  │
│ First, let's configure your SSO session.         │
│                                                  │
│ SSO Start URL:                                   │
│ [https://mycompany.awsapps.com/start]            │
│                                                  │
│ SSO Region: [us-east-1 ▼]                        │
│   (Type to search)                               │
│                                                  │
│ Session Name: [mycompany]                        │
│                                                  │
│ Need help? View AWS SSO documentation           │
│                                                  │
│ [Continue]                                       │
└─────────┬───────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────┐
│ Environment: dev (1/3)                           │
│                                                  │
│ ℹ From dev.yaml:                                 │
│   • Region: us-east-1                            │
│   • Environment: dev                             │
│                                                  │
│ ❓ What's your AWS Account ID for dev?           │
│ [123456789012]                                   │
│                                                  │
│ Role Name: [AdministratorAccess]                 │
│   (Most common role)                             │
│                                                  │
│ Region: [us-east-1]                              │
│   (From YAML)                                    │
│                                                  │
│ [Continue] [Skip Environment]                    │
└─────────┬───────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────┐
│ Writing Configuration...                         │
│                                                  │
│ ✓ Backup created                                 │
│ ✓ SSO session 'mycompany' written               │
│ ✓ Profile 'dev' written                          │
│ ✓ Config validated                               │
└─────────┬───────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────┐
│ SSO Login Required                               │
│                                                  │
│ Opening browser for AWS SSO authentication...    │
│                                                  │
│ Your browser should open automatically.          │
│ If not, visit:                                   │
│ https://mycompany.awsapps.com/start              │
│                                                  │
│ [Waiting for authentication...]                  │
│                                                  │
│ Press Ctrl+C to cancel                           │
└─────────┬───────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────┐
│ ✓ Authentication Successful!                     │
│                                                  │
│ Testing credentials...                           │
│                                                  │
│ ✓ Account verified: 123456789012                │
│ ✓ User: arn:aws:sts::123456789012:assumed-ro... │
│ ✓ dev.yaml updated                               │
└─────────┬───────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────┐
│ Environment: staging (2/3)                       │
│                                                  │
│ ℹ From staging.yaml:                             │
│   • Region: us-west-2                            │
│                                                  │
│ ℹ SSO Session: Using 'mycompany' (already auth) │
│                                                  │
│ AWS Account ID: [987654321098]                   │
│ Role Name: [AdministratorAccess]                 │
│ Region: [us-west-2]                              │
│                                                  │
│ [Continue] [Skip]                                │
└─────────┬───────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────┐
│ Writing Configuration...                         │
│                                                  │
│ ✓ Profile 'staging' written                      │
│ ✓ Already authenticated (reusing session)        │
│ ✓ staging.yaml updated                           │
└─────────┬───────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────┐
│ Environment: prod (3/3)                          │
│                                                  │
│ ℹ From prod.yaml:                                │
│   • Region: us-east-1                            │
│                                                  │
│ AWS Account ID: [456789012345]                   │
│ Role Name: [AdministratorAccess]                 │
│ Region: [us-east-1]                              │
│                                                  │
│ [Continue] [Skip]                                │
└─────────┬───────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────┐
│ Writing Configuration...                         │
│                                                  │
│ ✓ Profile 'prod' written                         │
│ ✓ Already authenticated (reusing session)        │
│ ✓ prod.yaml updated                              │
└─────────┬───────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────┐
│ ✓ Setup Complete!                                │
│                                                  │
│ Summary:                                         │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━             │
│ SSO Session:  mycompany                          │
│ Profiles:     3 configured                       │
│                                                  │
│ • dev        (123456789012 / us-east-1)         │
│ • staging    (987654321098 / us-west-2)         │
│ • prod       (456789012345 / us-east-1)         │
│                                                  │
│ Files Updated:                                   │
│ • ~/.aws/config                                  │
│ • project/dev.yaml                               │
│ • project/staging.yaml                           │
│ • project/prod.yaml                              │
│                                                  │
│ 🚀 You're ready to deploy!                       │
│                                                  │
│ [Deploy Now] [Back to Menu]                      │
└─────────────────────────────────────────────────┘
```

## Error Recovery Flow

```
┌─────────────────────────────────────────────────┐
│ SSO Login Failed                                 │
│                                                  │
│ ❌ Error: User cancelled authentication          │
│                                                  │
│ Profile 'dev' was created but not authenticated. │
│                                                  │
│ What would you like to do?                       │
│   • Retry SSO login now                          │
│   • Skip authentication (login later manually)   │
│   • Cancel setup                                 │
│                                                  │
│ Manual login command:                            │
│   aws sso login --profile dev                    │
│                                                  │
│ [Retry] [Skip] [Cancel]                          │
└─────────────────────────────────────────────────┘
           │
           ▼ (if Retry)
┌─────────────────────────────────────────────────┐
│ Retrying SSO Login...                            │
│                                                  │
│ Opening browser again...                         │
└─────────────────────────────────────────────────┘
```

```
┌─────────────────────────────────────────────────┐
│ Validation Failed                                │
│                                                  │
│ ❌ Failed to verify credentials for 'dev'        │
│                                                  │
│ Error: AccessDenied: User is not authorized     │
│        to perform: sts:GetCallerIdentity         │
│                                                  │
│ 🔧 Troubleshooting:                              │
│   1. Verify account ID is correct                │
│   2. Check role name matches IAM                 │
│   3. Confirm role is assigned to your user       │
│   4. Try re-authenticating                       │
│                                                  │
│ What would you like to do?                       │
│   • Re-enter account details                     │
│   • Retry validation                             │
│   • Skip validation (deploy may fail)            │
│   • View full error details                      │
│                                                  │
│ [Re-enter] [Retry] [Skip] [Details]              │
└─────────────────────────────────────────────────┘
```

## Integration Points

### 1. From Main Menu

```
┌─────────────────────────────────────────────────┐
│ Meroku Infrastructure Manager                   │
│                                                  │
│ > Deploy Infrastructure                          │
│   Plan Infrastructure Changes                    │
│   ⚙️  AWS SSO Setup Wizard ← NEW                 │
│   🔐 Select AWS Profile                          │
│   🌐 DNS Setup                                   │
│   🤖 Fix with AI (Claude Code)                   │
│   📊 View Infrastructure State                   │
│   🗑️  Destroy Infrastructure                     │
│   Exit                                           │
└─────────────────────────────────────────────────┘
```

### 2. From Pre-Flight Check

```
🚀 Starting deployment for environment: dev

🔍 Running AWS pre-flight checks...
❌ AWS_PROFILE not set

┌─────────────────────────────────────────────────┐
│ AWS Profile Not Configured                       │
│                                                  │
│ No AWS profile found for environment 'dev'.      │
│                                                  │
│ Would you like to set up AWS SSO now?            │
│                                                  │
│ [Yes, set up now] [No, set manually]             │
└─────────────────────────────────────────────────┘
           │
           ▼ (if Yes)
    [Launch wizard for 'dev' environment only]
```

### 3. From Environment Selector

```
┌─────────────────────────────────────────────────┐
│ Select Environment                               │
│                                                  │
│ ⚠️  Some environments need AWS setup:            │
│   • staging (no profile configured)              │
│   • prod (no profile configured)                 │
│                                                  │
│ Would you like to set them up now?               │
│                                                  │
│ [Set up now] [Continue anyway]                   │
└─────────────────────────────────────────────────┘
           │
           ▼ (if Set up now)
    [Launch wizard for staging & prod only]
```

## State Machine

```
┌──────────────┐
│   INITIAL    │
│ (No config)  │
└──────┬───────┘
       │
       ▼
┌──────────────┐     ┌──────────────┐
│ INSPECTING   │────▶│  DETECTED    │
│ (Analyzing)  │     │ (Config found)│
└──────────────┘     └──────┬───────┘
                             │
       ┌─────────────────────┴─────────────────────┐
       │                                            │
       ▼                                            ▼
┌──────────────┐                            ┌──────────────┐
│   PROMPTING  │                            │   COMPLETE   │
│(Collecting   │                            │(No action    │
│ user input)  │                            │  needed)     │
└──────┬───────┘                            └──────────────┘
       │
       ▼
┌──────────────┐
│   WRITING    │
│(Updating     │
│ config file) │
└──────┬───────┘
       │
       ▼
┌──────────────┐     ┌──────────────┐
│ AUTHENTICATING│────▶│   FAILED     │
│(SSO login)   │     │(Retry/Cancel)│
└──────┬───────┘     └──────────────┘
       │                     │
       │                     │
       ▼                     │
┌──────────────┐            │
│  VALIDATING  │            │
│(Testing      │────────────┘
│ credentials) │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│  UPDATING    │
│(Saving to    │
│  YAML files) │
└──────┬───────┘
       │
       ▼
┌──────────────┐
│   SUCCESS    │
│(Ready to     │
│  deploy)     │
└──────────────┘
```

## File Structure

```
infrastructure/
├── app/
│   ├── aws_sso_profile_inspector.go      ← NEW: Profile analysis
│   ├── aws_sso_setup_wizard_tui.go       ← NEW: Bubble Tea wizard
│   ├── aws_config_writer.go              ← NEW: Config file writer
│   ├── aws_sso_auto_login.go             ← NEW: Login automation
│   ├── aws_selector.go                   ← ENHANCE: Use wizard
│   ├── aws_preflight.go                  ← ENHANCE: Trigger wizard
│   ├── main.go                           ← ENHANCE: Add menu option
│   └── env_selector.go                   ← ENHANCE: Detect missing
│
├── ai_docs/
│   ├── AWS_SSO_SETUP_WIZARD.md           ← Implementation plan
│   ├── AWS_SSO_WIZARD_FLOW_DIAGRAM.md    ← This file
│   └── AWS_SSO_SETUP_USER_GUIDE.md       ← TODO: User docs
│
└── CLAUDE.md                              ← TODO: Update
```

## Summary

This wizard transforms AWS SSO setup from a **frustrating manual process** requiring documentation lookup and file editing into a **guided, intelligent experience** that:

1. **Detects** what you have
2. **Asks** only for what's missing
3. **Fixes** everything automatically
4. **Validates** it all works
5. **Updates** your project config

Zero manual file editing. Zero googling for AWS SSO docs. Just answer a few questions and you're ready to deploy.
