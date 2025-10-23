# AWS SSO AI Wizard - Comprehensive Improvement Plan

**Goal**: Transform the AWS SSO AI agent into a more capable, reliable, and informative troubleshooting system with enhanced tools, better prompts, and continuation support.

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Current State Analysis](#current-state-analysis)
3. [Improvement Requirements](#improvement-requirements)
4. [System Architecture](#system-architecture)
5. [Implementation Checklist](#implementation-checklist)
6. [Detailed Component Design](#detailed-component-design)
7. [Testing Strategy](#testing-strategy)
8. [Success Metrics](#success-metrics)

---

## Executive Summary

The AWS SSO wizard currently uses a basic ReAct pattern with limited tools and a simple system prompt. This plan outlines comprehensive improvements to make the agent more capable:

**Key Improvements:**
- **Enhanced System Prompt**: Rich context with AWS SSO documentation, validation rules, and troubleshooting guides
- **Expanded Tool Set**: AWS config I/O, YAML I/O, internet search, interactive user prompts, AWS CLI validation
- **State Persistence**: Save/resume capability for long-running troubleshooting sessions
- **Better Error Recovery**: Smarter handling of failed attempts with learning

**Expected Outcomes:**
- Higher success rate for complex SSO setups
- Reduced user intervention required
- Better handling of edge cases
- Improved user experience with clear progress tracking

---

## Current State Analysis

### Existing Implementation

**File**: `/Users/jack/mag/infrastructure/app/aws_sso_agent.go`

**Strengths:**
- ✅ Basic ReAct pattern implemented
- ✅ Integration with Anthropic Claude API
- ✅ Simple action types: THINK, ASK, EXEC, WRITE, COMPLETE
- ✅ Uses ProfileInspector for validation
- ✅ Pre-fills from YAML environment data
- ✅ Creates AWS config backups

**Limitations:**
- ❌ Limited to 15 iterations max (no continuation)
- ❌ Simple prompt without comprehensive AWS SSO knowledge
- ❌ Only basic tools: ask user, run commands, write config
- ❌ No AWS config file reading capability
- ❌ No internet search for documentation
- ❌ No structured user input (just scanf)
- ❌ No state persistence between runs
- ❌ Limited context about what went wrong in previous attempts

### Comparison with Terraform Agent

The existing Terraform troubleshooting agent (`ai_agent_react_loop.go`, `ai_agent_claude.go`) has:
- ✅ Comprehensive system prompt with project structure
- ✅ Multiple specialized tools (aws_cli, shell, file_edit, terraform_apply, web_search)
- ✅ Rich context building with structured error data
- ✅ Real-time TUI updates via Bubble Tea
- ✅ Better LLM prompt engineering with examples

**Gap**: The SSO agent needs similar capabilities adapted for SSO troubleshooting.

---

## Improvement Requirements

### 1. Better System Prompt

**Current**: ~300 lines with basic instructions
**Target**: ~800 lines with comprehensive AWS SSO knowledge

**Must Include:**
- Complete AWS SSO configuration documentation
- Modern SSO vs Legacy SSO differences
- Common error patterns and solutions
- AWS IAM Identity Center concepts
- Step-by-step troubleshooting workflows
- Real-world examples of successful resolutions
- Validation rules from `AWS_SSO_VALIDATION_RULES.md`
- Cross-reference with ProfileInspector logic

### 2. Continuation Support

**Problem**: Agent stops at 15 iterations even if close to solution

**Solution**: State persistence with resume capability

**Requirements:**
- Save agent state to disk (JSON format)
- Include all iteration history
- Store collected information (SSO URL, account ID, etc.)
- Allow resuming from exact same point
- Increment iteration count across runs
- New max: 30 total iterations (15 per run, 2 runs)
- User prompt: "Continue troubleshooting?" when limit reached

### 3. Enhanced Tool Set

#### Tool 1: AWS Config File I/O
```go
// Read AWS config file
ACTION: read_aws_config
COMMAND: ~/.aws/config

// Write/update AWS config file
ACTION: write_aws_config
COMMAND: <full config content>
```

**Purpose**:
- Inspect current configuration structure
- Understand what profiles exist
- See sso-session sections
- Detect configuration format (modern vs legacy)

#### Tool 2: YAML File I/O
```go
// Read project YAML file
ACTION: read_yaml
COMMAND: project/dev.yaml

// Update YAML file (if needed for account_id sync)
ACTION: write_yaml
COMMAND: FILE:project/dev.yaml|OLD:account_id: "123"|NEW:account_id: "456"
```

**Purpose**:
- Read environment configuration
- Sync account IDs between YAML and AWS
- Understand project structure

#### Tool 3: Internet Search
```go
// Search for AWS documentation
ACTION: web_search
COMMAND: AWS SSO InvalidParameterException sso_region
```

**Purpose**:
- Find AWS documentation for error messages
- Research unknown issues
- Get latest best practices
- Find community solutions

#### Tool 4: User Input (Interactive Prompts)
```go
// Ask user with options using huh library
ACTION: ask_choice
COMMAND: QUESTION:Which SSO role do you want?|OPTIONS:AdministratorAccess,PowerUserAccess,ReadOnlyAccess

// Confirm before destructive action
ACTION: ask_confirm
COMMAND: Are you sure you want to replace the existing SSO configuration?

// Ask for text input with validation
ACTION: ask_input
COMMAND: QUESTION:Enter SSO start URL|VALIDATOR:url|PLACEHOLDER:https://mycompany.awsapps.com/start
```

**Purpose**:
- Better UX than scanf
- Input validation
- Dropdown selections
- Confirmation prompts
- Help text / descriptions

#### Tool 5: AWS CLI Validation
```go
// Test SSO login
ACTION: aws_validate
COMMAND: sso_login|profile:dev

// Validate credentials
ACTION: aws_validate
COMMAND: credentials|profile:dev|account:123456789012

// Check AWS CLI version
ACTION: aws_validate
COMMAND: cli_version
```

**Purpose**:
- Automated validation after configuration
- Structured error responses
- Retry logic built-in
- Success verification

---

## System Architecture

### High-Level Flow

```
User triggers SSO wizard
    ↓
Check if state file exists (continuation?)
    ↓
    ├─ Yes → Load saved state, resume from iteration N
    └─ No  → Build fresh context, start iteration 1
    ↓
ReAct Loop (max 15 iterations per run)
    ↓
    ├─ LLM decides action using enhanced prompt
    ├─ Execute tool (expanded tool set)
    ├─ Capture result
    ├─ Update context with result
    ├─ Save state to disk
    └─ Loop until complete OR iteration limit
    ↓
If limit reached: "Continue troubleshooting?" prompt
    ↓
    ├─ Yes → Resume from saved state (iteration 16-30)
    └─ No  → Exit, save state for later
    ↓
Success: Clean up state file, show summary
```

### File Structure

```
app/
├── aws_sso_agent.go              # Main agent orchestrator (EXISTING - ENHANCE)
├── aws_sso_agent_tools.go        # NEW: Tool implementations
├── aws_sso_agent_state.go        # NEW: State persistence
├── aws_sso_agent_prompts.go      # NEW: Enhanced system prompt
├── aws_sso_agent_user_input.go   # NEW: Huh-based user interactions
├── aws_sso_inspector.go          # EXISTING: Validation logic
├── aws_config_reader.go          # NEW: AWS config file parser
└── aws_config_writer.go          # EXISTING: Config writing

State files (temporary):
/tmp/meroku_sso_state_{profile}.json
```

### Data Structures

#### Enhanced SSOAgentContext
```go
type SSOAgentContext struct {
    // Existing fields
    ProfileName  string
    YAMLEnv      *Env
    ConfigPath   string
    ConfigExists bool
    ProfileInfo  *ProfileInfo

    // Collected information (existing)
    SSOStartURL  string
    SSORegion    string
    AccountID    string
    RoleName     string
    Region       string
    Output       string

    // NEW: Enhanced context
    AWSConfigContent    string              // Full ~/.aws/config file
    YAMLContent         map[string]string   // Parsed YAML files
    ValidationHistory   []ValidationAttempt  // Track validation results
    SearchResults       []SearchResult       // Web search results cache

    // Iteration tracking (existing)
    ActionHistory []SSOAgentAction
    Iteration     int

    // NEW: State management
    StateFilePath   string
    TotalIterations int  // Across all runs
    RunNumber       int  // Which run (1, 2, 3...)
    LastSaveTime    time.Time
}
```

#### SSOAgentAction Enhancement
```go
type SSOAgentAction struct {
    Type        string // EXPANDED: see tools below
    Description string
    Command     string
    Question    string
    Answer      string
    Result      string
    Error       error

    // NEW: Enhanced metadata
    Timestamp     time.Time
    Duration      time.Duration
    RetryCount    int
    ToolMetadata  map[string]interface{}  // Tool-specific data
}
```

#### State Persistence Structure
```go
type SSOAgentState struct {
    Version         string            `json:"version"`  // State format version
    ProfileName     string            `json:"profile_name"`
    SaveTime        time.Time         `json:"save_time"`
    Context         *SSOAgentContext  `json:"context"`
    IsComplete      bool              `json:"is_complete"`
    CompletionMsg   string            `json:"completion_message"`
    TotalIterations int               `json:"total_iterations"`
    RunNumber       int               `json:"run_number"`
}
```

---

## Implementation Checklist

### Phase 1: Core Infrastructure (Week 1)

#### File: `aws_sso_agent_state.go`
- [ ] Define `SSOAgentState` struct
- [ ] Implement `SaveState(state *SSOAgentState, filepath string) error`
- [ ] Implement `LoadState(filepath string) (*SSOAgentState, error)`
- [ ] Add state file path generation: `/tmp/meroku_sso_state_{profile}_{timestamp}.json`
- [ ] Implement `CleanupStateFile(filepath string)` for success cleanup
- [ ] Add JSON marshaling with proper error handling
- [ ] Test state save/load cycle with mock data

#### File: `aws_config_reader.go`
- [ ] Implement `ReadAWSConfig(configPath string) (string, error)` - read full file
- [ ] Implement `ParseAWSConfigProfiles(content string) ([]string, error)` - list profiles
- [ ] Implement `ParseSSOSessions(content string) ([]string, error)` - list sso-sessions
- [ ] Add parsing helpers for extracting profile sections
- [ ] Test with various config file formats (modern, legacy, mixed)

#### File: `aws_sso_agent_user_input.go`
- [ ] Implement `AskChoice(question string, options []string) (string, error)` using huh.Select
- [ ] Implement `AskConfirm(question string) (bool, error)` using huh.Confirm
- [ ] Implement `AskInput(question, placeholder, validator string) (string, error)` using huh.Input
- [ ] Add validators: URL, AWS region, account ID, role name
- [ ] Test interactive prompts in terminal

### Phase 2: Enhanced Tools (Week 1-2)

#### File: `aws_sso_agent_tools.go`

##### Tool: read_aws_config
- [ ] Parse command to extract config path
- [ ] Call `ReadAWSConfig()` helper
- [ ] Return formatted config content
- [ ] Handle file not found errors gracefully
- [ ] Add to agent action handler

##### Tool: write_aws_config
- [ ] Validate config content format
- [ ] Create backup of existing config
- [ ] Write new config using ConfigWriter
- [ ] Verify write succeeded
- [ ] Return success/failure message

##### Tool: read_yaml
- [ ] Parse command to extract YAML file path
- [ ] Read and parse YAML file
- [ ] Return formatted YAML content
- [ ] Handle parsing errors gracefully
- [ ] Cache content in context for reuse

##### Tool: write_yaml
- [ ] Use file_edit pattern: `FILE:path|OLD:text|NEW:text`
- [ ] Validate YAML syntax after edit
- [ ] Create backup before modifying
- [ ] Update context cache if successful

##### Tool: web_search
- [ ] Integrate with existing `ExecuteWebSearch()` (from ai_agent_executor.go)
- [ ] Parse search query from command
- [ ] Cache results in context
- [ ] Return top 3-5 relevant results
- [ ] Add timeout handling (30 seconds)

##### Tool: ask_choice
- [ ] Parse: `QUESTION:text|OPTIONS:opt1,opt2,opt3`
- [ ] Call `AskChoice()` from user_input helper
- [ ] Store answer in action result
- [ ] Update context with collected information

##### Tool: ask_confirm
- [ ] Parse confirmation question
- [ ] Call `AskConfirm()` helper
- [ ] Return boolean result
- [ ] Handle cancellation gracefully

##### Tool: ask_input
- [ ] Parse: `QUESTION:text|VALIDATOR:type|PLACEHOLDER:text`
- [ ] Call `AskInput()` with appropriate validator
- [ ] Store validated answer
- [ ] Re-prompt on validation failure

##### Tool: aws_validate
- [ ] Parse: `sso_login|profile:name` or `credentials|profile:name|account:id`
- [ ] Execute validation using existing ProfileInspector
- [ ] Return structured validation result
- [ ] Handle AWS CLI errors with helpful messages

#### Integration into SSOAgent
- [ ] Update `executeAction()` to handle new tool types
- [ ] Add tool dispatch logic for each new action type
- [ ] Ensure proper error handling for each tool
- [ ] Test each tool in isolation

### Phase 3: Enhanced System Prompt (Week 2)

#### File: `aws_sso_agent_prompts.go`

##### Prompt Section 1: AWS SSO Comprehensive Guide
```go
const awsSSODocumentation = `
═══════════════════════════════════════════════════════════════════
COMPREHENSIVE AWS SSO CONFIGURATION GUIDE
═══════════════════════════════════════════════════════════════════

AWS SSO (Single Sign-On) uses IAM Identity Center for centralized authentication.

TWO CONFIGURATION STYLES:

1. MODERN SSO (RECOMMENDED - AWS CLI v2+)
   Uses separate [sso-session] section for shared configuration:

   [sso-session my-sso]
   sso_start_url = https://my-company.awsapps.com/start
   sso_region = us-east-1
   sso_registration_scopes = sso:account:access

   [profile dev]
   sso_session = my-sso
   sso_account_id = 123456789012
   sso_role_name = AdministratorAccess
   region = us-east-1
   output = json

   BENEFITS:
   - One sso-session shared by multiple profiles
   - Login once, use all profiles
   - Easier to maintain
   - Required for AWS CLI v2.0+

2. LEGACY SSO (OLD STYLE - Still supported but not recommended)
   All SSO config in profile section:

   [profile dev]
   sso_start_url = https://my-company.awsapps.com/start
   sso_region = us-east-1
   sso_account_id = 123456789012
   sso_role_name = AdministratorAccess
   region = us-east-1
   output = json

   DRAWBACKS:
   - Duplicate config for multiple profiles
   - Must login separately for each profile
   - More error-prone

REQUIRED FIELDS (Modern SSO):
- sso-session section:
  * sso_start_url: HTTPS URL (https://*.awsapps.com/start or https://*.aws.amazon.com/...)
  * sso_region: AWS region where IAM Identity Center is hosted (usually us-east-1)
  * sso_registration_scopes: Usually "sso:account:access" (for API access)

- profile section:
  * sso_session: Name of the sso-session to use
  * sso_account_id: 12-digit AWS account ID
  * sso_role_name: IAM role name (e.g., AdministratorAccess, PowerUserAccess)
  * region: Default AWS region for this profile
  * output: Output format (json, yaml, text, table)

VALIDATION RULES:
- sso_start_url must start with https://
- sso_start_url typically contains .awsapps.com or .aws.amazon.com
- sso_region must be a valid AWS region (e.g., us-east-1, eu-west-1)
- sso_account_id must be exactly 12 digits
- sso_role_name must be valid IAM role name (alphanumeric, +, =, ., @, -, _)
- region must be a valid AWS region

COMMON ROLE NAMES:
- AdministratorAccess: Full access to all AWS services
- PowerUserAccess: Full access except IAM management
- ReadOnlyAccess: Read-only access to all AWS services
- ViewOnlyAccess: Similar to ReadOnly
- SecurityAudit: Audit and compliance access
- DatabaseAdministrator: Database management
- NetworkAdministrator: Network infrastructure management
- SystemAdministrator: System administration tasks
`
```

##### Prompt Section 2: Common Issues & Solutions
```go
const commonSSOIssues = `
═══════════════════════════════════════════════════════════════════
COMMON AWS SSO ISSUES AND SOLUTIONS
═══════════════════════════════════════════════════════════════════

ISSUE 1: "Error loading SSO Token"
CAUSE: No SSO session cached or session expired
SOLUTION:
  1. Run: aws sso login --profile {profile}
  2. Browser will open for authentication
  3. Complete authentication in browser
  4. Return to CLI, credentials cached

ISSUE 2: "Profile not found"
CAUSE: AWS config file missing or profile not defined
SOLUTION:
  1. Check if ~/.aws/config exists
  2. Verify profile section exists: [profile {name}]
  3. If missing, create config file with proper permissions (600)
  4. Write profile configuration

ISSUE 3: "Invalid sso_region"
CAUSE: Region format wrong or unsupported region
SOLUTION:
  1. Use standard AWS region format: us-east-1 (not US-EAST-1)
  2. sso_region is where IAM Identity Center is hosted (usually us-east-1)
  3. This is different from the default region where resources run

ISSUE 4: "SSO authentication failed"
CAUSE: Invalid SSO start URL or wrong credentials
SOLUTION:
  1. Verify SSO start URL is correct (ask user to confirm)
  2. Check URL format: https://*.awsapps.com/start
  3. Ensure user has access to the SSO portal
  4. Try login again after correcting URL

ISSUE 5: "Account ID mismatch"
CAUSE: YAML config has different account ID than AWS returns
SOLUTION:
  1. Run: aws sts get-caller-identity --profile {profile}
  2. Compare returned Account ID with YAML config
  3. Ask user which is correct
  4. Update YAML or AWS config accordingly

ISSUE 6: "Permission denied"
CAUSE: Wrong role selected or insufficient permissions
SOLUTION:
  1. Verify role name is correct (case-sensitive)
  2. Check user has permission to assume this role in SSO portal
  3. Try a different role (e.g., ReadOnlyAccess instead of AdministratorAccess)

ISSUE 7: "sso-session not found"
CAUSE: Profile references non-existent sso-session
SOLUTION:
  1. Check [sso-session {name}] section exists in config
  2. Verify sso_session value in profile matches section name exactly
  3. Create missing sso-session section if needed

ISSUE 8: "AWS CLI version too old"
CAUSE: AWS CLI v1.x doesn't support modern SSO
SOLUTION:
  1. Check version: aws --version
  2. If v1.x, ask user to upgrade to v2+
  3. Provide installation instructions for their OS

TROUBLESHOOTING WORKFLOW:
1. CHECK: Does AWS CLI exist and is v2+?
2. CHECK: Does ~/.aws/config file exist?
3. CHECK: Does profile section exist in config?
4. CHECK: If modern SSO, does sso-session section exist?
5. CHECK: Are all required fields present?
6. CHECK: Do field values pass validation?
7. TRY: Run aws sso login
8. TRY: Run aws sts get-caller-identity
9. VERIFY: Account ID matches expected value
10. SUCCESS: Profile works!
`
```

##### Prompt Section 3: Available Tools
```go
const awsSSOTools = `
═══════════════════════════════════════════════════════════════════
YOUR AVAILABLE TOOLS
═══════════════════════════════════════════════════════════════════

1. THINK - Analyze the situation and reason about next steps
   Format: THINK: <your reasoning>
   Use to: Show your logical process to the user

2. read_aws_config - Read AWS configuration file
   Format: read_aws_config: ~/.aws/config
   Use to:
   - See current configuration structure
   - List existing profiles
   - Find sso-session sections
   - Understand what needs to be fixed

3. write_aws_config - Write complete AWS configuration
   Format: write_aws_config: <full config content>
   Use to:
   - Create new configuration
   - Update existing configuration
   - Fix configuration issues
   Note: Always creates backup before writing

4. read_yaml - Read project YAML configuration
   Format: read_yaml: project/dev.yaml
   Use to:
   - Get account_id from YAML
   - Get region from YAML
   - Understand project configuration

5. write_yaml - Update YAML configuration
   Format: write_yaml: FILE:project/dev.yaml|OLD:old_text|NEW:new_text
   Use to:
   - Sync account_id between YAML and AWS
   - Update profile name in YAML

6. ask_choice - Ask user to select from options
   Format: ask_choice: QUESTION:Which role?|OPTIONS:Admin,Power,ReadOnly
   Use to:
   - Get user preference from predefined choices
   - Select IAM role
   - Choose AWS region
   - Pick configuration style

7. ask_confirm - Ask user yes/no question
   Format: ask_confirm: Replace existing SSO config?
   Use to:
   - Confirm before destructive actions
   - Verify user intent
   - Get permission to proceed

8. ask_input - Ask user for text input with validation
   Format: ask_input: QUESTION:SSO URL?|VALIDATOR:url|PLACEHOLDER:https://...
   Use to:
   - Get SSO start URL
   - Get account ID
   - Get any user-provided value
   Validators: url, region, account_id, role_name, none

9. web_search - Search for AWS documentation
   Format: web_search: AWS SSO InvalidParameterException
   Use to:
   - Research unknown errors
   - Find AWS documentation
   - Learn about new issues
   - Get best practices

10. aws_validate - Validate AWS configuration
    Format: aws_validate: sso_login|profile:dev
    Format: aws_validate: credentials|profile:dev|account:123456789012
    Format: aws_validate: cli_version
    Use to:
    - Test SSO login after config
    - Verify credentials work
    - Check AWS CLI version

11. EXEC - Execute AWS CLI or shell commands
    Format: EXEC: <command>
    Examples:
    - EXEC: aws sso login --profile dev
    - EXEC: aws sts get-caller-identity --profile dev
    - EXEC: aws --version
    Use to:
    - Run direct AWS CLI commands
    - Execute validation commands
    - Test configuration

12. COMPLETE - Mark setup as complete
    Format: COMPLETE: <summary message>
    Use when: AWS SSO is fully configured and validated

TOOL SELECTION STRATEGY:
- Start with read_aws_config to understand current state
- Use read_yaml to get account/region from project config
- Ask user for missing critical info (SSO URL, role name)
- Write configuration once you have all required fields
- Validate with aws_validate after writing
- If validation fails, use web_search to research errors
- Mark COMPLETE only after successful validation
`
```

##### Prompt Assembly
- [ ] Create `BuildEnhancedSystemPrompt(ctx *SSOAgentContext) string`
- [ ] Include all documentation sections
- [ ] Add current context (profile name, YAML data, validation results)
- [ ] Include action history (last 5-10 actions)
- [ ] Add examples of successful resolutions
- [ ] Format for readability (~1000 lines total)
- [ ] Test prompt with mock scenarios

#### Update aws_sso_agent.go
- [ ] Replace `buildPrompt()` with call to `BuildEnhancedSystemPrompt()`
- [ ] Ensure backward compatibility with existing code
- [ ] Test prompt generation with various contexts

### Phase 4: Continuation Support (Week 2-3)

#### Update aws_sso_agent.go Run() method

##### Before ReAct Loop
- [ ] Check if state file exists for this profile
- [ ] If exists, prompt: "Previous session found. Continue? [Y/n]"
- [ ] If yes, load state and resume
- [ ] If no, start fresh (optionally archive old state)
- [ ] Initialize state tracking variables

##### During ReAct Loop
- [ ] Save state after each action completion
- [ ] Update `TotalIterations` counter
- [ ] Track `RunNumber` (1, 2, etc.)
- [ ] Save to: `/tmp/meroku_sso_state_{profile}_{timestamp}.json`

##### At Iteration Limit (15 per run)
```go
if agentCtx.Iteration >= a.maxIterations {
    // Check if we can continue (total < 30)
    if agentCtx.TotalIterations < 30 {
        // Prompt user
        continueChoice, err := AskConfirm("Reached iteration limit. Continue troubleshooting?")
        if err != nil || !continueChoice {
            // Save state and exit
            SaveState(...)
            return fmt.Errorf("paused at iteration %d (total: %d)", agentCtx.Iteration, agentCtx.TotalIterations)
        }

        // Reset iteration counter for new run
        agentCtx.Iteration = 0
        agentCtx.RunNumber++
        // Continue loop
    } else {
        // Absolute limit reached
        return fmt.Errorf("reached maximum iterations (30) without resolution")
    }
}
```

##### On Success
- [ ] Delete state file (cleanup)
- [ ] Show summary of what was accomplished
- [ ] Display total iterations used

##### On User Cancellation (Ctrl+C)
- [ ] Save current state
- [ ] Show message: "State saved. Run again to continue."
- [ ] Exit gracefully

#### Testing
- [ ] Test save/load with mock state
- [ ] Test continuation after 15 iterations
- [ ] Test state preservation across runs
- [ ] Test cleanup on success
- [ ] Test graceful exit on cancellation

### Phase 5: Integration & Testing (Week 3)

#### Unit Tests
- [ ] Test `SaveState()` and `LoadState()` (state.go)
- [ ] Test AWS config reading/parsing (config_reader.go)
- [ ] Test user input validation (user_input.go)
- [ ] Test each tool in isolation (tools.go)
- [ ] Test prompt building (prompts.go)

#### Integration Tests
- [ ] Create test AWS config files (various formats)
- [ ] Create test YAML files (dev, staging, prod)
- [ ] Mock Anthropic API responses for full flow
- [ ] Test agent with mocked profile inspector
- [ ] Test continuation across multiple runs

#### End-to-End Tests
- [ ] Test with empty AWS config (fresh setup)
- [ ] Test with existing modern SSO config (update)
- [ ] Test with existing legacy SSO config (migrate)
- [ ] Test with incomplete config (fix)
- [ ] Test with wrong SSO URL (correct)
- [ ] Test with account ID mismatch (sync)
- [ ] Test continuation after failure
- [ ] Test across environment boundaries (dev → staging → prod)

#### Error Scenarios
- [ ] Test with no AWS CLI installed
- [ ] Test with AWS CLI v1 (too old)
- [ ] Test with invalid SSO start URL
- [ ] Test with wrong account ID
- [ ] Test with permission denied (wrong role)
- [ ] Test with network errors during login
- [ ] Test with Anthropic API rate limits
- [ ] Test with corrupted state file

### Phase 6: Documentation & Polish (Week 3-4)

#### User Documentation
- [ ] Update `/ai_docs/AWS_SSO_AI_AGENT.md` with new capabilities
- [ ] Add continuation support docs
- [ ] Document all new tools
- [ ] Add troubleshooting guide for users
- [ ] Create animated demo GIF/video

#### Developer Documentation
- [ ] Document state file format
- [ ] Document tool implementation patterns
- [ ] Add architecture diagrams
- [ ] Document testing approach
- [ ] Add contribution guidelines

#### Code Quality
- [ ] Add comprehensive code comments
- [ ] Ensure consistent error handling
- [ ] Add logging for debugging
- [ ] Optimize prompt token usage
- [ ] Profile performance bottlenecks
- [ ] Add telemetry for success tracking

---

## Detailed Component Design

### 1. State Persistence System

**File**: `app/aws_sso_agent_state.go`

```go
package main

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "time"
)

const (
    StateVersion = "1.0"
    StateDir     = "/tmp"
)

// SSOAgentState represents persisted agent state
type SSOAgentState struct {
    Version         string            `json:"version"`
    ProfileName     string            `json:"profile_name"`
    SaveTime        time.Time         `json:"save_time"`
    Context         *SSOAgentContext  `json:"context"`
    IsComplete      bool              `json:"is_complete"`
    CompletionMsg   string            `json:"completion_message"`
    TotalIterations int               `json:"total_iterations"`
    RunNumber       int               `json:"run_number"`
}

// GetStateFilePath generates state file path for a profile
func GetStateFilePath(profileName string) string {
    timestamp := time.Now().Format("20060102")
    filename := fmt.Sprintf("meroku_sso_state_%s_%s.json", profileName, timestamp)
    return filepath.Join(StateDir, filename)
}

// SaveState persists agent state to disk
func SaveState(state *SSOAgentState, filepath string) error {
    state.Version = StateVersion
    state.SaveTime = time.Now()

    // Marshal to JSON with indentation for readability
    data, err := json.MarshalIndent(state, "", "  ")
    if err != nil {
        return fmt.Errorf("failed to marshal state: %w", err)
    }

    // Write with restricted permissions (owner only)
    err = os.WriteFile(filepath, data, 0600)
    if err != nil {
        return fmt.Errorf("failed to write state file: %w", err)
    }

    return nil
}

// LoadState loads agent state from disk
func LoadState(filepath string) (*SSOAgentState, error) {
    // Check if file exists
    if _, err := os.Stat(filepath); os.IsNotExist(err) {
        return nil, fmt.Errorf("state file does not exist: %s", filepath)
    }

    // Read file
    data, err := os.ReadFile(filepath)
    if err != nil {
        return nil, fmt.Errorf("failed to read state file: %w", err)
    }

    // Unmarshal
    var state SSOAgentState
    err = json.Unmarshal(data, &state)
    if err != nil {
        return nil, fmt.Errorf("failed to unmarshal state: %w", err)
    }

    // Verify version compatibility
    if state.Version != StateVersion {
        return nil, fmt.Errorf("incompatible state version: %s (expected %s)", state.Version, StateVersion)
    }

    return &state, nil
}

// CleanupStateFile deletes state file after successful completion
func CleanupStateFile(filepath string) error {
    if filepath == "" {
        return nil // Nothing to clean up
    }

    err := os.Remove(filepath)
    if err != nil && !os.IsNotExist(err) {
        return fmt.Errorf("failed to remove state file: %w", err)
    }

    return nil
}

// ListStatFiles finds all state files for debugging
func ListStateFiles() ([]string, error) {
    pattern := filepath.Join(StateDir, "meroku_sso_state_*.json")
    matches, err := filepath.Glob(pattern)
    if err != nil {
        return nil, fmt.Errorf("failed to list state files: %w", err)
    }
    return matches, nil
}
```

**Usage in aws_sso_agent.go:**

```go
func (a *SSOAgent) Run(ctx context.Context, profileName string, yamlEnv *Env) error {
    fmt.Println("🤖 AI Agent: AWS SSO Setup")

    // Check for existing state
    stateFilePath := GetStateFilePath(profileName)
    var agentCtx *SSOAgentContext
    var runNumber int = 1

    if _, err := os.Stat(stateFilePath); err == nil {
        // State file exists, ask if user wants to continue
        fmt.Printf("\n⚠️  Previous troubleshooting session found for profile '%s'\n", profileName)
        fmt.Printf("   Saved: %s\n", stateFilePath)

        continueChoice, err := AskConfirm("Continue from where you left off?")
        if err != nil || !continueChoice {
            // User declined, start fresh
            fmt.Println("Starting fresh troubleshooting session...")
            agentCtx, err = a.buildContext(profileName, yamlEnv)
            if err != nil {
                return fmt.Errorf("failed to build context: %w", err)
            }
        } else {
            // Load saved state
            fmt.Println("Loading saved state...")
            savedState, err := LoadState(stateFilePath)
            if err != nil {
                fmt.Printf("⚠️  Failed to load state: %v\n", err)
                fmt.Println("Starting fresh session instead...")
                agentCtx, err = a.buildContext(profileName, yamlEnv)
                if err != nil {
                    return fmt.Errorf("failed to build context: %w", err)
                }
            } else {
                // Resume from saved state
                agentCtx = savedState.Context
                runNumber = savedState.RunNumber + 1
                fmt.Printf("✅ Resumed from iteration %d (Run #%d)\n", savedState.TotalIterations, runNumber)
            }
        }
    } else {
        // No saved state, start fresh
        agentCtx, err = a.buildContext(profileName, yamlEnv)
        if err != nil {
            return fmt.Errorf("failed to build context: %w", err)
        }
    }

    agentCtx.StateFilePath = stateFilePath
    agentCtx.RunNumber = runNumber

    // ReAct loop (max 15 iterations per run, 30 total)
    maxIterationsPerRun := 15
    maxTotalIterations := 30

    for i := 0; i < maxIterationsPerRun; i++ {
        agentCtx.Iteration = i + 1
        agentCtx.TotalIterations++

        fmt.Printf("\n───────── Iteration %d (Run #%d, Total: %d/%d) ─────────\n\n",
            agentCtx.Iteration, runNumber, agentCtx.TotalIterations, maxTotalIterations)

        // Get next action from LLM
        action, err := a.getNextAction(ctx, agentCtx)
        if err != nil {
            // Save state before failing
            SaveState(&SSOAgentState{
                ProfileName:     profileName,
                Context:         agentCtx,
                TotalIterations: agentCtx.TotalIterations,
                RunNumber:       runNumber,
            }, stateFilePath)
            return fmt.Errorf("agent error: %w", err)
        }

        // Check for completion
        if action.Type == "complete" {
            fmt.Println("✅ SUCCESS! AWS SSO setup is complete.")
            fmt.Println()
            a.printSummary(agentCtx)

            // Cleanup state file on success
            CleanupStateFile(stateFilePath)
            return nil
        }

        // Execute action
        if err := a.executeAction(ctx, action, agentCtx); err != nil {
            action.Error = err
            action.Result = fmt.Sprintf("Failed: %v", err)
        }

        // Add to history
        agentCtx.ActionHistory = append(agentCtx.ActionHistory, *action)

        // Save state after each action
        SaveState(&SSOAgentState{
            ProfileName:     profileName,
            Context:         agentCtx,
            TotalIterations: agentCtx.TotalIterations,
            RunNumber:       runNumber,
        }, stateFilePath)

        // Check if stuck
        if a.isStuck(agentCtx) {
            fmt.Println("⚠️  Agent appears stuck (repeated same action 3 times)")
            SaveState(&SSOAgentState{
                ProfileName:     profileName,
                Context:         agentCtx,
                TotalIterations: agentCtx.TotalIterations,
                RunNumber:       runNumber,
            }, stateFilePath)
            return fmt.Errorf("agent stuck, please try manual wizard mode")
        }
    }

    // Reached iteration limit for this run
    if agentCtx.TotalIterations < maxTotalIterations {
        // Can continue if user wants
        fmt.Printf("\n⚠️  Reached iteration limit for this run (%d iterations)\n", maxIterationsPerRun)
        fmt.Printf("   Total iterations so far: %d/%d\n", agentCtx.TotalIterations, maxTotalIterations)

        continueChoice, err := AskConfirm("Continue troubleshooting in a new run?")
        if err != nil || !continueChoice {
            // User declined, save state and exit
            SaveState(&SSOAgentState{
                ProfileName:     profileName,
                Context:         agentCtx,
                TotalIterations: agentCtx.TotalIterations,
                RunNumber:       runNumber,
            }, stateFilePath)
            fmt.Printf("\n💾 Progress saved to: %s\n", stateFilePath)
            fmt.Println("   Run the agent again to continue troubleshooting.")
            return nil
        }

        // User wants to continue, recursively call Run with loaded state
        return a.Run(ctx, profileName, yamlEnv)
    }

    // Absolute maximum reached
    SaveState(&SSOAgentState{
        ProfileName:     profileName,
        Context:         agentCtx,
        TotalIterations: agentCtx.TotalIterations,
        RunNumber:       runNumber,
    }, stateFilePath)

    return fmt.Errorf("reached maximum iterations (%d) without completing setup", maxTotalIterations)
}
```

### 2. AWS Config File Reader

**File**: `app/aws_config_reader.go`

```go
package main

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "gopkg.in/ini.v1"
)

// ReadAWSConfig reads the entire AWS config file
func ReadAWSConfig(configPath string) (string, error) {
    // Resolve ~ to home directory
    if strings.HasPrefix(configPath, "~/") {
        home, err := os.UserHomeDir()
        if err != nil {
            return "", fmt.Errorf("failed to get home directory: %w", err)
        }
        configPath = filepath.Join(home, configPath[2:])
    }

    // Check if file exists
    if _, err := os.Stat(configPath); os.IsNotExist(err) {
        return "", fmt.Errorf("config file does not exist: %s", configPath)
    }

    // Read file
    content, err := os.ReadFile(configPath)
    if err != nil {
        return "", fmt.Errorf("failed to read config file: %w", err)
    }

    return string(content), nil
}

// ParseAWSConfigProfiles extracts all profile names from config content
func ParseAWSConfigProfiles(content string) ([]string, error) {
    cfg, err := ini.Load([]byte(content))
    if err != nil {
        return nil, fmt.Errorf("failed to parse config: %w", err)
    }

    profiles := []string{}
    for _, section := range cfg.Sections() {
        name := section.Name()
        if name == "DEFAULT" || name == "default" {
            profiles = append(profiles, "default")
        } else if strings.HasPrefix(name, "profile ") {
            profileName := strings.TrimPrefix(name, "profile ")
            profiles = append(profiles, profileName)
        }
    }

    return profiles, nil
}

// ParseSSOSessions extracts all sso-session names from config content
func ParseSSOSessions(content string) ([]string, error) {
    cfg, err := ini.Load([]byte(content))
    if err != nil {
        return nil, fmt.Errorf("failed to parse config: %w", err)
    }

    sessions := []string{}
    for _, section := range cfg.Sections() {
        name := section.Name()
        if strings.HasPrefix(name, "sso-session ") {
            sessionName := strings.TrimPrefix(name, "sso-session ")
            sessions = append(sessions, sessionName)
        }
    }

    return sessions, nil
}

// GetProfileSection extracts a specific profile section as a map
func GetProfileSection(content, profileName string) (map[string]string, error) {
    cfg, err := ini.Load([]byte(content))
    if err != nil {
        return nil, fmt.Errorf("failed to parse config: %w", err)
    }

    sectionName := "profile " + profileName
    if profileName == "default" {
        sectionName = "default"
    }

    section, err := cfg.GetSection(sectionName)
    if err != nil {
        return nil, fmt.Errorf("profile not found: %s", profileName)
    }

    result := make(map[string]string)
    for _, key := range section.Keys() {
        result[key.Name()] = key.String()
    }

    return result, nil
}

// GetSSOSessionSection extracts a specific sso-session section
func GetSSOSessionSection(content, sessionName string) (map[string]string, error) {
    cfg, err := ini.Load([]byte(content))
    if err != nil {
        return nil, fmt.Errorf("failed to parse config: %w", err)
    }

    sectionName := "sso-session " + sessionName
    section, err := cfg.GetSection(sectionName)
    if err != nil {
        return nil, fmt.Errorf("sso-session not found: %s", sessionName)
    }

    result := make(map[string]string)
    for _, key := range section.Keys() {
        result[key.Name()] = key.String()
    }

    return result, nil
}
```

### 3. User Input with Huh Library

**File**: `app/aws_sso_agent_user_input.go`

```go
package main

import (
    "fmt"
    "regexp"
    "strings"

    "github.com/charmbracelet/huh"
)

// AskChoice presents user with options using huh.Select
func AskChoice(question string, options []string) (string, error) {
    if len(options) == 0 {
        return "", fmt.Errorf("no options provided")
    }

    // Convert strings to huh.Option
    huhOptions := make([]huh.Option[string], len(options))
    for i, opt := range options {
        huhOptions[i] = huh.NewOption(opt, opt)
    }

    var selected string
    err := huh.NewSelect[string]().
        Title(question).
        Options(huhOptions...).
        Value(&selected).
        Run()

    if err != nil {
        return "", fmt.Errorf("selection cancelled: %w", err)
    }

    return selected, nil
}

// AskConfirm asks user yes/no question
func AskConfirm(question string) (bool, error) {
    var confirmed bool
    err := huh.NewConfirm().
        Title(question).
        Value(&confirmed).
        Run()

    if err != nil {
        return false, fmt.Errorf("confirmation cancelled: %w", err)
    }

    return confirmed, nil
}

// AskInput asks user for text input with validation
func AskInput(question, placeholder, validatorType string) (string, error) {
    var input string

    inputField := huh.NewInput().
        Title(question).
        Placeholder(placeholder).
        Value(&input)

    // Add validator based on type
    switch validatorType {
    case "url":
        inputField = inputField.Validate(ValidateURL)
    case "region":
        inputField = inputField.Validate(ValidateRegion)
    case "account_id":
        inputField = inputField.Validate(ValidateAccountID)
    case "role_name":
        inputField = inputField.Validate(ValidateRoleName)
    case "none":
        // No validation
    default:
        return "", fmt.Errorf("unknown validator type: %s", validatorType)
    }

    err := inputField.Run()
    if err != nil {
        return "", fmt.Errorf("input cancelled: %w", err)
    }

    return input, nil
}

// Validators

func ValidateURL(s string) error {
    if s == "" {
        return fmt.Errorf("URL cannot be empty")
    }
    if !strings.HasPrefix(s, "https://") {
        return fmt.Errorf("URL must start with https://")
    }
    if !strings.Contains(s, ".awsapps.com") && !strings.Contains(s, ".aws.amazon.com") {
        return fmt.Errorf("URL should be an AWS SSO portal URL")
    }
    return nil
}

func ValidateRegion(s string) error {
    if s == "" {
        return fmt.Errorf("region cannot be empty")
    }
    // Pattern: us-east-1, eu-west-2, ap-southeast-1
    regionRegex := regexp.MustCompile(`^[a-z]{2}-[a-z]+-\d+$`)
    if !regionRegex.MatchString(s) {
        return fmt.Errorf("invalid region format (expected: us-east-1)")
    }
    return nil
}

func ValidateAccountID(s string) error {
    if s == "" {
        return fmt.Errorf("account ID cannot be empty")
    }
    if len(s) != 12 {
        return fmt.Errorf("account ID must be exactly 12 digits")
    }
    accountRegex := regexp.MustCompile(`^\d{12}$`)
    if !accountRegex.MatchString(s) {
        return fmt.Errorf("account ID must be numeric (12 digits)")
    }
    return nil
}

func ValidateRoleName(s string) error {
    if s == "" {
        return fmt.Errorf("role name cannot be empty")
    }
    // Valid characters: alphanumeric, plus, equals, comma, period, at, underscore, hyphen
    roleRegex := regexp.MustCompile(`^[\w+=,.@-]+$`)
    if !roleRegex.MatchString(s) {
        return fmt.Errorf("invalid role name (allowed: alphanumeric, +, =, ., @, _, -)")
    }
    return nil
}
```

### 4. Enhanced Tools Implementation

**File**: `app/aws_sso_agent_tools.go`

```go
package main

import (
    "context"
    "fmt"
    "strings"
)

// Tool: read_aws_config
func (a *SSOAgent) toolReadAWSConfig(ctx context.Context, command string, agentCtx *SSOAgentContext) (*SSOAgentAction, error) {
    configPath := strings.TrimSpace(command)
    if configPath == "" {
        configPath = getAWSConfigPath()
    }

    content, err := ReadAWSConfig(configPath)
    if err != nil {
        return &SSOAgentAction{
            Type:   "read_aws_config",
            Result: fmt.Sprintf("Failed to read config: %v", err),
            Error:  err,
        }, err
    }

    // Store in context for future reference
    agentCtx.AWSConfigContent = content

    // Parse profiles and sessions
    profiles, _ := ParseAWSConfigProfiles(content)
    sessions, _ := ParseSSOSessions(content)

    result := fmt.Sprintf("Read AWS config from %s\n", configPath)
    result += fmt.Sprintf("Found %d profiles: %s\n", len(profiles), strings.Join(profiles, ", "))
    result += fmt.Sprintf("Found %d sso-sessions: %s\n", len(sessions), strings.Join(sessions, ", "))
    result += fmt.Sprintf("\nConfig content:\n%s", content)

    return &SSOAgentAction{
        Type:   "read_aws_config",
        Result: result,
    }, nil
}

// Tool: write_aws_config
func (a *SSOAgent) toolWriteAWSConfig(ctx context.Context, command string, agentCtx *SSOAgentContext) (*SSOAgentAction, error) {
    // command contains the full config content to write
    configContent := command

    if configContent == "" {
        return &SSOAgentAction{
            Type:   "write_aws_config",
            Result: "Error: empty config content",
            Error:  fmt.Errorf("empty config content"),
        }, fmt.Errorf("empty config content")
    }

    // Create backup
    configPath := getAWSConfigPath()
    backupPath := fmt.Sprintf("%s.backup.%s", configPath, time.Now().Format("20060102_150405"))

    if _, err := os.Stat(configPath); err == nil {
        // File exists, create backup
        if err := copyFile(configPath, backupPath); err != nil {
            return &SSOAgentAction{
                Type:   "write_aws_config",
                Result: fmt.Sprintf("Failed to create backup: %v", err),
                Error:  err,
            }, err
        }
    }

    // Write new config
    err := os.WriteFile(configPath, []byte(configContent), 0600)
    if err != nil {
        return &SSOAgentAction{
            Type:   "write_aws_config",
            Result: fmt.Sprintf("Failed to write config: %v", err),
            Error:  err,
        }, err
    }

    result := fmt.Sprintf("✅ Wrote AWS config to %s\n", configPath)
    if backupPath != "" {
        result += fmt.Sprintf("   Backup saved: %s", backupPath)
    }

    return &SSOAgentAction{
        Type:   "write_aws_config",
        Result: result,
    }, nil
}

// Tool: read_yaml
func (a *SSOAgent) toolReadYAML(ctx context.Context, command string, agentCtx *SSOAgentContext) (*SSOAgentAction, error) {
    yamlPath := strings.TrimSpace(command)

    content, err := os.ReadFile(yamlPath)
    if err != nil {
        return &SSOAgentAction{
            Type:   "read_yaml",
            Result: fmt.Sprintf("Failed to read YAML: %v", err),
            Error:  err,
        }, err
    }

    // Store in context cache
    if agentCtx.YAMLContent == nil {
        agentCtx.YAMLContent = make(map[string]string)
    }
    agentCtx.YAMLContent[yamlPath] = string(content)

    result := fmt.Sprintf("Read YAML from %s\n\nContent:\n%s", yamlPath, string(content))

    return &SSOAgentAction{
        Type:   "read_yaml",
        Result: result,
    }, nil
}

// Tool: write_yaml (uses file_edit pattern)
func (a *SSOAgent) toolWriteYAML(ctx context.Context, command string, agentCtx *SSOAgentContext) (*SSOAgentAction, error) {
    // Parse: FILE:path|OLD:old_text|NEW:new_text
    parts := strings.Split(command, "|")
    if len(parts) != 3 {
        return &SSOAgentAction{
            Type:   "write_yaml",
            Result: "Error: invalid format, expected FILE:path|OLD:text|NEW:text",
            Error:  fmt.Errorf("invalid format"),
        }, fmt.Errorf("invalid format")
    }

    var filePath, oldText, newText string
    for _, part := range parts {
        part = strings.TrimSpace(part)
        if strings.HasPrefix(part, "FILE:") {
            filePath = strings.TrimPrefix(part, "FILE:")
        } else if strings.HasPrefix(part, "OLD:") {
            oldText = strings.TrimPrefix(part, "OLD:")
        } else if strings.HasPrefix(part, "NEW:") {
            newText = strings.TrimPrefix(part, "NEW:")
        }
    }

    // Create backup
    backupPath := fmt.Sprintf("%s.backup.%s", filePath, time.Now().Format("20060102_150405"))
    if err := copyFile(filePath, backupPath); err != nil {
        return &SSOAgentAction{
            Type:   "write_yaml",
            Result: fmt.Sprintf("Failed to create backup: %v", err),
            Error:  err,
        }, err
    }

    // Read file
    content, err := os.ReadFile(filePath)
    if err != nil {
        return &SSOAgentAction{
            Type:   "write_yaml",
            Result: fmt.Sprintf("Failed to read YAML: %v", err),
            Error:  err,
        }, err
    }

    // Replace
    newContent := strings.ReplaceAll(string(content), oldText, newText)

    // Write back
    err = os.WriteFile(filePath, []byte(newContent), 0644)
    if err != nil {
        return &SSOAgentAction{
            Type:   "write_yaml",
            Result: fmt.Sprintf("Failed to write YAML: %v", err),
            Error:  err,
        }, err
    }

    // Update cache
    if agentCtx.YAMLContent == nil {
        agentCtx.YAMLContent = make(map[string]string)
    }
    agentCtx.YAMLContent[filePath] = newContent

    result := fmt.Sprintf("✅ Updated YAML: %s\n   Backup: %s", filePath, backupPath)

    return &SSOAgentAction{
        Type:   "write_yaml",
        Result: result,
    }, nil
}

// Tool: ask_choice
func (a *SSOAgent) toolAskChoice(ctx context.Context, command string, agentCtx *SSOAgentContext) (*SSOAgentAction, error) {
    // Parse: QUESTION:text|OPTIONS:opt1,opt2,opt3
    parts := strings.Split(command, "|")
    if len(parts) != 2 {
        return &SSOAgentAction{
            Type:   "ask_choice",
            Result: "Error: invalid format, expected QUESTION:text|OPTIONS:opt1,opt2,opt3",
            Error:  fmt.Errorf("invalid format"),
        }, fmt.Errorf("invalid format")
    }

    var question string
    var options []string

    for _, part := range parts {
        part = strings.TrimSpace(part)
        if strings.HasPrefix(part, "QUESTION:") {
            question = strings.TrimPrefix(part, "QUESTION:")
        } else if strings.HasPrefix(part, "OPTIONS:") {
            optStr := strings.TrimPrefix(part, "OPTIONS:")
            options = strings.Split(optStr, ",")
            for i := range options {
                options[i] = strings.TrimSpace(options[i])
            }
        }
    }

    selected, err := AskChoice(question, options)
    if err != nil {
        return &SSOAgentAction{
            Type:     "ask_choice",
            Question: question,
            Result:   fmt.Sprintf("User cancelled selection: %v", err),
            Error:    err,
        }, err
    }

    return &SSOAgentAction{
        Type:     "ask_choice",
        Question: question,
        Answer:   selected,
        Result:   fmt.Sprintf("User selected: %s", selected),
    }, nil
}

// Tool: ask_confirm
func (a *SSOAgent) toolAskConfirm(ctx context.Context, command string, agentCtx *SSOAgentContext) (*SSOAgentAction, error) {
    question := strings.TrimSpace(command)

    confirmed, err := AskConfirm(question)
    if err != nil {
        return &SSOAgentAction{
            Type:     "ask_confirm",
            Question: question,
            Result:   fmt.Sprintf("User cancelled confirmation: %v", err),
            Error:    err,
        }, err
    }

    return &SSOAgentAction{
        Type:     "ask_confirm",
        Question: question,
        Answer:   fmt.Sprintf("%v", confirmed),
        Result:   fmt.Sprintf("User confirmed: %v", confirmed),
    }, nil
}

// Tool: ask_input
func (a *SSOAgent) toolAskInput(ctx context.Context, command string, agentCtx *SSOAgentContext) (*SSOAgentAction, error) {
    // Parse: QUESTION:text|VALIDATOR:type|PLACEHOLDER:text
    parts := strings.Split(command, "|")

    var question, validator, placeholder string
    for _, part := range parts {
        part = strings.TrimSpace(part)
        if strings.HasPrefix(part, "QUESTION:") {
            question = strings.TrimPrefix(part, "QUESTION:")
        } else if strings.HasPrefix(part, "VALIDATOR:") {
            validator = strings.TrimPrefix(part, "VALIDATOR:")
        } else if strings.HasPrefix(part, "PLACEHOLDER:") {
            placeholder = strings.TrimPrefix(part, "PLACEHOLDER:")
        }
    }

    if question == "" {
        return &SSOAgentAction{
            Type:   "ask_input",
            Result: "Error: question not specified",
            Error:  fmt.Errorf("question not specified"),
        }, fmt.Errorf("question not specified")
    }

    input, err := AskInput(question, placeholder, validator)
    if err != nil {
        return &SSOAgentAction{
            Type:     "ask_input",
            Question: question,
            Result:   fmt.Sprintf("User cancelled input: %v", err),
            Error:    err,
        }, err
    }

    return &SSOAgentAction{
        Type:     "ask_input",
        Question: question,
        Answer:   input,
        Result:   fmt.Sprintf("User entered: %s", input),
    }, nil
}

// Tool: web_search (integrates with existing web search)
func (a *SSOAgent) toolWebSearch(ctx context.Context, query string, agentCtx *SSOAgentContext) (*SSOAgentAction, error) {
    // Use existing web search implementation
    results, err := ExecuteWebSearch(ctx, query)
    if err != nil {
        return &SSOAgentAction{
            Type:   "web_search",
            Result: fmt.Sprintf("Search failed: %v", err),
            Error:  err,
        }, err
    }

    return &SSOAgentAction{
        Type:   "web_search",
        Result: results,
    }, nil
}

// Tool: aws_validate
func (a *SSOAgent) toolAWSValidate(ctx context.Context, command string, agentCtx *SSOAgentContext) (*SSOAgentAction, error) {
    // Parse command: sso_login|profile:dev
    //            or: credentials|profile:dev|account:123456789012
    //            or: cli_version

    if command == "cli_version" {
        // Check AWS CLI version
        err := a.inspector.CheckAWSCLI()
        if err != nil {
            return &SSOAgentAction{
                Type:   "aws_validate",
                Result: fmt.Sprintf("AWS CLI check failed: %v", err),
                Error:  err,
            }, err
        }
        return &SSOAgentAction{
            Type:   "aws_validate",
            Result: "AWS CLI v2+ is installed",
        }, nil
    }

    parts := strings.Split(command, "|")
    validationType := parts[0]

    var profile, expectedAccount string
    for _, part := range parts[1:] {
        if strings.HasPrefix(part, "profile:") {
            profile = strings.TrimPrefix(part, "profile:")
        } else if strings.HasPrefix(part, "account:") {
            expectedAccount = strings.TrimPrefix(part, "account:")
        }
    }

    if validationType == "sso_login" {
        // Test SSO login
        autoLogin := NewAutoLogin(profile)
        err := autoLogin.Login()
        if err != nil {
            return &SSOAgentAction{
                Type:   "aws_validate",
                Result: fmt.Sprintf("SSO login failed: %v", err),
                Error:  err,
            }, err
        }
        return &SSOAgentAction{
            Type:   "aws_validate",
            Result: fmt.Sprintf("✅ SSO login successful for profile '%s'", profile),
        }, nil

    } else if validationType == "credentials" {
        // Validate credentials
        autoLogin := NewAutoLogin(profile)
        result, err := autoLogin.ValidateCredentials(expectedAccount, agentCtx.Region)
        if err != nil {
            return &SSOAgentAction{
                Type:   "aws_validate",
                Result: fmt.Sprintf("Credential validation failed: %v", err),
                Error:  err,
            }, err
        }

        if !result.Success {
            return &SSOAgentAction{
                Type:   "aws_validate",
                Result: "Credential validation failed",
                Error:  fmt.Errorf("validation failed"),
            }, fmt.Errorf("validation failed")
        }

        return &SSOAgentAction{
            Type:   "aws_validate",
            Result: fmt.Sprintf("✅ Credentials valid. Account: %s, ARN: %s", result.AccountID, result.ARN),
        }, nil
    }

    return &SSOAgentAction{
        Type:   "aws_validate",
        Result: "Unknown validation type",
        Error:  fmt.Errorf("unknown validation type: %s", validationType),
    }, fmt.Errorf("unknown validation type")
}

// Update executeAction to dispatch to new tools
func (a *SSOAgent) executeAction(ctx context.Context, action *SSOAgentAction, agentCtx *SSOAgentContext) error {
    switch action.Type {
    case "think":
        // Just display thinking
        fmt.Println("🤔 THINKING:")
        fmt.Println("   " + action.Description)
        return nil

    case "read_aws_config":
        result, err := a.toolReadAWSConfig(ctx, action.Command, agentCtx)
        *action = *result
        return err

    case "write_aws_config":
        result, err := a.toolWriteAWSConfig(ctx, action.Command, agentCtx)
        *action = *result
        return err

    case "read_yaml":
        result, err := a.toolReadYAML(ctx, action.Command, agentCtx)
        *action = *result
        return err

    case "write_yaml":
        result, err := a.toolWriteYAML(ctx, action.Command, agentCtx)
        *action = *result
        return err

    case "ask_choice":
        result, err := a.toolAskChoice(ctx, action.Command, agentCtx)
        *action = *result
        return err

    case "ask_confirm":
        result, err := a.toolAskConfirm(ctx, action.Command, agentCtx)
        *action = *result
        return err

    case "ask_input":
        result, err := a.toolAskInput(ctx, action.Command, agentCtx)
        *action = *result
        return err

    case "web_search":
        result, err := a.toolWebSearch(ctx, action.Command, agentCtx)
        *action = *result
        return err

    case "aws_validate":
        result, err := a.toolAWSValidate(ctx, action.Command, agentCtx)
        *action = *result
        return err

    // Existing tools
    case "ask":
        return a.askUser(action, agentCtx)

    case "exec":
        return a.execCommand(action)

    case "write":
        return a.writeConfig(action, agentCtx)

    case "complete":
        return nil

    default:
        return fmt.Errorf("unknown action type: %s", action.Type)
    }
}
```

---

## Testing Strategy

### Unit Tests

**File**: `app/aws_sso_agent_state_test.go`
```go
func TestSaveAndLoadState(t *testing.T) {
    // Create mock state
    state := &SSOAgentState{
        ProfileName: "test",
        TotalIterations: 5,
        RunNumber: 1,
        Context: &SSOAgentContext{
            ProfileName: "test",
            SSOStartURL: "https://test.awsapps.com/start",
        },
    }

    // Save to temp file
    tempPath := filepath.Join(os.TempDir(), "test_state.json")
    defer os.Remove(tempPath)

    err := SaveState(state, tempPath)
    assert.NoError(t, err)

    // Load back
    loaded, err := LoadState(tempPath)
    assert.NoError(t, err)
    assert.Equal(t, state.ProfileName, loaded.ProfileName)
    assert.Equal(t, state.TotalIterations, loaded.TotalIterations)
}
```

**File**: `app/aws_config_reader_test.go`
```go
func TestReadAWSConfig(t *testing.T) {
    // Create temp config file
    content := `[profile test]
sso_session = my-sso
sso_account_id = 123456789012
sso_role_name = AdministratorAccess`

    tempPath := filepath.Join(os.TempDir(), "test_config")
    err := os.WriteFile(tempPath, []byte(content), 0600)
    require.NoError(t, err)
    defer os.Remove(tempPath)

    // Read it back
    read, err := ReadAWSConfig(tempPath)
    assert.NoError(t, err)
    assert.Contains(t, read, "sso_session = my-sso")

    // Parse profiles
    profiles, err := ParseAWSConfigProfiles(read)
    assert.NoError(t, err)
    assert.Contains(t, profiles, "test")
}
```

### Integration Tests

**File**: `app/aws_sso_agent_integration_test.go`
```go
func TestAgentWithMockedLLM(t *testing.T) {
    // Mock Anthropic API responses
    mockResponses := []string{
        "THINK: Profile missing\nACTION: read_aws_config\nCOMMAND: ~/.aws/config",
        "THINK: Need SSO URL\nACTION: ask_input\nCOMMAND: QUESTION:SSO URL?|VALIDATOR:url|PLACEHOLDER:https://...",
        "THINK: Ready to write\nACTION: write_aws_config\nCOMMAND: ...",
        "THINK: Validate\nACTION: aws_validate\nCOMMAND: sso_login|profile:test",
        "THINK: Success\nACTION: complete\nCOMMAND: Setup complete",
    }

    // Create agent with mock LLM
    agent := NewAgentWithMock(mockResponses)

    // Run agent
    err := agent.Run(context.Background(), "test", &Env{})
    assert.NoError(t, err)
}
```

### End-to-End Tests

Test scenarios:
1. Fresh setup (no config exists)
2. Update existing modern SSO
3. Migrate from legacy SSO
4. Fix incomplete config
5. Correct wrong SSO URL
6. Handle account ID mismatch
7. Continuation after failure
8. Multi-environment setup

---

## Success Metrics

### Quantitative Metrics
- **Success Rate**: % of runs that complete successfully
  - Target: 85%+ (up from current ~60%)
- **Average Iterations**: Mean iterations to success
  - Target: < 8 iterations (currently ~10)
- **Continuation Usage**: % of runs that use continuation
  - Target: < 20% (most should complete in one run)
- **User Intervention**: Average # of user prompts per run
  - Target: 2-3 questions (SSO URL, role, confirmation)

### Qualitative Metrics
- **User Satisfaction**: Survey feedback
  - Target: 4.5/5 stars
- **Error Recovery**: Can agent recover from mistakes?
  - Target: 90%+ of errors self-corrected
- **Documentation Quality**: User-reported clarity
  - Target: "Easy to understand" > 80%

### Comparison with Manual Wizard
| Metric | Manual Wizard | AI Agent (Enhanced) |
|--------|--------------|---------------------|
| Success Rate | 95% | 85%+ (target) |
| Time to Complete | 2-5 minutes | 3-7 minutes |
| User Questions | 5-7 | 2-3 |
| Handles Edge Cases | Poor | Excellent |
| Adapts to Errors | No | Yes |
| Learning Capability | No | Yes (via prompt) |

---

## Timeline

### Week 1: Core Infrastructure
- Days 1-2: State persistence (aws_sso_agent_state.go)
- Days 3-4: AWS config reader (aws_config_reader.go)
- Day 5: User input with huh (aws_sso_agent_user_input.go)

### Week 2: Enhanced Tools & Prompt
- Days 1-3: Tool implementations (aws_sso_agent_tools.go)
- Days 4-5: Enhanced system prompt (aws_sso_agent_prompts.go)

### Week 3: Continuation & Testing
- Days 1-2: Continuation support in Run() method
- Days 3-4: Unit and integration tests
- Day 5: End-to-end testing with real scenarios

### Week 4: Documentation & Polish
- Days 1-2: User documentation updates
- Days 3-4: Code quality improvements
- Day 5: Final testing and demo

---

## Risk Mitigation

### Risk 1: Anthropic API Rate Limits
**Mitigation**:
- Add exponential backoff on rate limit errors
- Cache search results to reduce API calls
- Provide fallback to manual wizard if API unavailable

### Risk 2: State File Corruption
**Mitigation**:
- Validate JSON schema on load
- Provide recovery mechanism (start fresh)
- Log state saves for debugging
- Version state format for future compatibility

### Risk 3: Tool Complexity
**Mitigation**:
- Test each tool in isolation
- Provide clear error messages
- Add debug logging for troubleshooting
- Document expected inputs/outputs

### Risk 4: LLM Hallucination
**Mitigation**:
- Validate all LLM-generated commands before execution
- Sandbox file operations (backups)
- Provide comprehensive examples in prompt
- Add stuck detection (repeated actions)

### Risk 5: User Experience Issues
**Mitigation**:
- Progressive disclosure (don't overwhelm)
- Clear progress indicators
- Easy escape hatch (Ctrl+C saves state)
- Helpful error messages with suggestions

---

## Conclusion

This comprehensive plan transforms the AWS SSO AI wizard from a basic troubleshooting tool into an intelligent, adaptive agent capable of handling complex SSO setup scenarios. The key improvements are:

1. **Enhanced System Prompt**: Comprehensive AWS SSO knowledge base
2. **Expanded Tool Set**: 12 specialized tools for config management, user interaction, and validation
3. **State Persistence**: Save/resume capability for long troubleshooting sessions
4. **Better UX**: Interactive prompts with validation, clear progress tracking

With these improvements, the AI wizard will be able to:
- Handle 85%+ of SSO setup scenarios autonomously
- Recover from errors intelligently
- Provide clear explanations of what it's doing
- Resume from where it left off if interrupted
- Adapt to user-specific configurations

The implementation follows best practices from the existing Terraform agent while adding SSO-specific capabilities. The phased approach ensures incremental progress with continuous testing and validation.

**Next Steps**: Begin Phase 1 implementation with state persistence and AWS config reader infrastructure.
