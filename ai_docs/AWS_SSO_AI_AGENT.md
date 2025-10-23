# AWS SSO AI Agent - Intelligent Setup & Troubleshooting

**Autonomous AI agent for AWS SSO configuration using ReAct pattern**

This document describes the AI-powered agent that can analyze, diagnose, and automatically fix AWS SSO configuration issues through intelligent reasoning and action.

## Overview

The AWS SSO AI Agent provides an alternative to the manual wizard approach. Instead of following a fixed script, the agent:

1. **Analyzes** the current AWS configuration state
2. **Reasons** about what's wrong and what information is needed
3. **Asks** targeted questions to gather missing information
4. **Acts** by executing commands and writing configuration
5. **Validates** the result and iterates until working

This is similar to the existing Terraform troubleshooting agent but focused on AWS SSO setup.

## User Experience

### Trigger Points

The AI agent can be invoked in three ways:

1. **Automatic (on validation failure):**
   ```
   ❌ AWS SSO validation failed for profile 'dev'

   Choose how to fix:
   [1] Interactive Wizard (step-by-step)
   [2] AI Agent (automatic troubleshooting)
   [3] Skip (continue without fixing)

   ? Your choice: 2
   ```

2. **From main menu:**
   ```
   Main Menu:
   1. Generate Infrastructure
   2. Apply Changes
   3. DNS Setup
   4. 🤖 AI Agent - Troubleshoot Issues
   5. 🔐 AWS SSO Setup Wizard
   6. 🤖 AI Agent - AWS SSO Setup  ← New option
   ```

3. **CLI command:**
   ```bash
   ./meroku sso agent --profile dev
   ./meroku sso agent --all  # Fix all environments
   ```

### Example Interaction Flow

```
🤖 AI Agent: AWS SSO Setup

📋 Environment: dev (from project/dev.yaml)
📍 Config file: ~/.aws/config

🔍 [Analyzing current configuration...]

🤔 THINKING:
   - Profile 'dev' not found in ~/.aws/config
   - Need SSO configuration for profile 'dev'
   - Have partial information from dev.yaml:
     * account_id: 123456789012
     * region: us-east-1
   - Missing: sso_start_url, sso_region, sso_role_name

💬 I need some information to set up AWS SSO for the 'dev' environment:

? What is your AWS SSO start URL?
  This is typically provided by your AWS administrator.
  Example: https://mycompany.awsapps.com/start

  URL: https://acme-corp.awsapps.com/start

? What AWS region hosts your IAM Identity Center?
  This is where your SSO portal is configured.
  Common: us-east-1, us-west-2, eu-west-1

  Region [us-east-1]: ⏎

? What IAM role should this profile use?
  Common roles: AdministratorAccess, PowerUserAccess, ReadOnlyAccess

  Role: AdministratorAccess

✅ Got all the information I need!

🤔 THINKING:
   - Will create modern SSO configuration with sso-session
   - Profile 'dev' will reference sso-session 'default-sso'
   - Need to create backup of config file first

🔧 ACTION: Creating backup of ~/.aws/config
✅ Backup created: ~/.aws/config.backup.20251022_144530

🔧 ACTION: Writing SSO configuration
   - Creating [sso-session default-sso] section
   - Creating [profile dev] section
✅ Configuration written successfully

🤔 THINKING:
   - Configuration is written but SSO token not obtained yet
   - Need to authenticate with AWS SSO
   - Will run: aws sso login --profile dev

🔧 ACTION: Authenticating with AWS SSO
   Running: aws sso login --profile dev

   [Browser opens for SSO authentication]

✅ Successfully authenticated!

🤔 THINKING:
   - Authentication succeeded
   - Should verify credentials work with an API call
   - Will call: aws sts get-caller-identity --profile dev

🔧 ACTION: Verifying credentials
   Running: aws sts get-caller-identity --profile dev

   Response:
   {
     "UserId": "AROAXXXXXXXXX:user@example.com",
     "Account": "123456789012",
     "Arn": "arn:aws:sts::123456789012:assumed-role/AdministratorAccess/user@example.com"
   }

🤔 THINKING:
   - API call succeeded
   - Account ID matches dev.yaml: 123456789012 ✓
   - Profile is fully configured and working
   - Setup complete!

✅ SUCCESS! AWS SSO profile 'dev' is ready to use.

Summary:
  ✓ Created SSO configuration in ~/.aws/config
  ✓ Authenticated with AWS SSO
  ✓ Verified credentials work
  ✓ Account ID matches configuration

You can now use this profile with AWS CLI commands:
  aws s3 ls --profile dev

Or set it as default:
  export AWS_PROFILE=dev
```

### Multi-Environment Setup

```bash
./meroku sso agent --all
```

```
🤖 AI Agent: AWS SSO Setup (All Environments)

📋 Found 3 environments: dev, staging, prod
🔍 Analyzing all configurations...

🤔 THINKING:
   - Profile 'dev': NOT found
   - Profile 'staging': NOT found
   - Profile 'prod': FOUND but expired token
   - All environments likely use same SSO portal
   - Can reuse sso-session for all profiles

💬 I'll set up SSO for all three environments.

? What is your AWS SSO start URL?
  (This will be shared across all environments)

  URL: https://acme-corp.awsapps.com/start

? What AWS region hosts your IAM Identity Center?

  Region [us-east-1]: ⏎

Now I need role information for each environment:

? Role for 'dev' environment: AdministratorAccess
? Role for 'staging' environment: PowerUserAccess
? Role for 'prod' environment: ReadOnlyAccess

✅ Got all the information I need!

🤔 THINKING:
   - Will create one shared sso-session section
   - Three profiles will reference the same session
   - Each profile has different account_id and role
   - Only need to login once (session is shared)

🔧 ACTION: Writing configuration for all environments
✅ Configuration written

🔧 ACTION: Authenticating with AWS SSO (once for all)
   Running: aws sso login --sso-session default-sso
✅ Successfully authenticated!

🔧 ACTION: Verifying all profiles
   ✓ dev: Account 123456789012 - Working
   ✓ staging: Account 987654321098 - Working
   ✓ prod: Account 555666777888 - Working

✅ SUCCESS! All 3 AWS SSO profiles are ready!
```

## ReAct Pattern Implementation

### Cycle Structure

```
LOOP until success or max iterations (10):
  1. OBSERVE: Gather current state
  2. THINK: Reason about what to do next
  3. ACT: Execute command or ask question
  4. VALIDATE: Check if problem is solved
```

### Agent Capabilities

**OBSERVE Actions:**
- Read `~/.aws/config` file
- Parse AWS CLI version
- Read project YAML files for context
- Check profile existence
- Inspect SSO token cache

**THINK Actions:**
- Analyze configuration completeness
- Identify missing fields
- Determine question priority
- Choose appropriate fix strategy
- Validate against rules from AWS_SSO_VALIDATION_RULES.md

**ACT Actions:**
- Ask user questions (targeted, context-aware)
- Write/update `~/.aws/config`
- Execute `aws sso login`
- Execute `aws sts get-caller-identity`
- Create backups
- Show results to user

**VALIDATE Actions:**
- Parse command outputs
- Compare expected vs actual
- Verify credentials work
- Check account ID matches YAML

## Implementation Structure

### File Organization

```
app/
├── internal/
│   └── aws/
│       ├── validator/          # Existing validation package
│       │   ├── validator.go
│       │   └── rules.go
│       └── ssoagent/          # New AI agent package
│           ├── agent.go       # Main agent logic
│           ├── context.go     # Context gathering
│           ├── prompts.go     # LLM prompt templates
│           ├── actions.go     # Available actions
│           └── runner.go      # ReAct loop executor
```

### Agent Context Structure

```go
// Context passed to the AI agent
type AgentContext struct {
    // Input
    ProfileName      string            // "dev", "staging", "prod"
    YAMLPath         string            // path to project/dev.yaml
    YAMLConfig       *YAMLConfig       // parsed YAML content
    ConfigPath       string            // ~/.aws/config path

    // Discovered state
    ConfigExists     bool
    ConfigContent    string            // raw file content
    ProfileExists    bool
    ProfileConfig    *ProfileConfig    // parsed profile if exists

    // Validation results
    ValidationErrors []ValidationError
    MissingFields    []string

    // History
    ActionHistory    []Action
    ConversationLog  []Message
}

// Actions the agent can take
type Action struct {
    Type        ActionType
    Description string
    Command     string        // for exec actions
    Question    string        // for ask actions
    ConfigDiff  string        // for write actions
    Result      interface{}
    Error       error
}

type ActionType string
const (
    ActionTypeAsk       ActionType = "ask"       // Ask user a question
    ActionTypeExec      ActionType = "exec"      // Execute shell command
    ActionTypeWrite     ActionType = "write"     // Write config file
    ActionTypeValidate  ActionType = "validate"  // Run validation
    ActionTypeThink     ActionType = "think"     // Internal reasoning
)
```

### Prompt Template

```go
const agentSystemPrompt = `You are an expert AWS SSO configuration assistant. Your goal is to help users set up AWS SSO profiles correctly.

You can take the following actions:
1. ASK: Ask the user a targeted question to gather missing information
2. EXEC: Execute AWS CLI commands (aws sso login, aws sts get-caller-identity, etc.)
3. WRITE: Write or update the ~/.aws/config file
4. VALIDATE: Check if the configuration is correct
5. THINK: Reason about what to do next

Use the ReAct pattern:
- THINK about the current situation and what you need
- ACT by asking questions, running commands, or writing config
- OBSERVE the results
- Repeat until the configuration works

Current situation:
{{.Context}}

Validation rules:
{{.ValidationRules}}

Recent actions:
{{.ActionHistory}}

What should you do next?`
```

### Agent Implementation

```go
// app/internal/aws/ssoagent/agent.go
package ssoagent

import (
    "context"
    "fmt"
    "github.com/anthropics/anthropic-sdk-go"
)

type Agent struct {
    client          *anthropic.Client
    maxIterations   int
    validationRules string  // from AWS_SSO_VALIDATION_RULES.md
}

func NewAgent() (*Agent, error) {
    apiKey := os.Getenv("ANTHROPIC_API_KEY")
    if apiKey == "" {
        return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
    }

    client := anthropic.NewClient(apiKey)

    // Load validation rules
    rules, err := loadValidationRules()
    if err != nil {
        return nil, err
    }

    return &Agent{
        client:          client,
        maxIterations:   10,
        validationRules: rules,
    }, nil
}

// Run executes the agent's ReAct loop
func (a *Agent) Run(ctx context.Context, agentCtx *AgentContext) error {
    fmt.Println("🤖 AI Agent: AWS SSO Setup")
    fmt.Println()

    for i := 0; i < a.maxIterations; i++ {
        // Build prompt with current context
        prompt := a.buildPrompt(agentCtx)

        // Call Claude API
        action, err := a.getNextAction(ctx, prompt)
        if err != nil {
            return fmt.Errorf("agent error: %w", err)
        }

        // Display thinking
        if action.Type == ActionTypeThink {
            fmt.Println("🤔 THINKING:")
            fmt.Println(indent(action.Description))
            fmt.Println()
            continue
        }

        // Execute action
        result, err := a.executeAction(ctx, action, agentCtx)
        if err != nil {
            // Agent handles its own errors
            agentCtx.ActionHistory = append(agentCtx.ActionHistory, Action{
                Type:   action.Type,
                Error:  err,
                Result: "Failed: " + err.Error(),
            })
            continue
        }

        // Update context with result
        agentCtx.ActionHistory = append(agentCtx.ActionHistory, Action{
            Type:   action.Type,
            Result: result,
        })

        // Check if complete
        if a.isComplete(agentCtx) {
            fmt.Println("✅ SUCCESS! AWS SSO setup complete.")
            return nil
        }
    }

    return fmt.Errorf("agent reached max iterations without completing setup")
}

// executeAction performs the requested action
func (a *Agent) executeAction(ctx context.Context, action Action, agentCtx *AgentContext) (interface{}, error) {
    switch action.Type {
    case ActionTypeAsk:
        return a.askUser(action.Question)

    case ActionTypeExec:
        return a.execCommand(ctx, action.Command)

    case ActionTypeWrite:
        return a.writeConfig(agentCtx.ConfigPath, action.ConfigDiff)

    case ActionTypeValidate:
        return a.runValidation(agentCtx)

    default:
        return nil, fmt.Errorf("unknown action type: %s", action.Type)
    }
}

// askUser prompts the user and waits for input
func (a *Agent) askUser(question string) (string, error) {
    fmt.Println("💬", question)
    fmt.Print("   Answer: ")

    var answer string
    _, err := fmt.Scanln(&answer)
    if err != nil {
        return "", err
    }

    return answer, nil
}

// execCommand runs an AWS CLI command
func (a *Agent) execCommand(ctx context.Context, command string) (string, error) {
    fmt.Println("🔧 ACTION:", command)

    cmd := exec.CommandContext(ctx, "bash", "-c", command)
    output, err := cmd.CombinedOutput()

    if err != nil {
        fmt.Println("❌ Failed:", err)
        return string(output), err
    }

    fmt.Println("✅ Success")
    return string(output), nil
}

// writeConfig updates the AWS config file
func (a *Agent) writeConfig(path string, diff string) (string, error) {
    fmt.Println("🔧 ACTION: Writing configuration")

    // Create backup
    backupPath := fmt.Sprintf("%s.backup.%s", path, time.Now().Format("20060102_150405"))
    if err := copyFile(path, backupPath); err != nil {
        return "", fmt.Errorf("backup failed: %w", err)
    }
    fmt.Println("   Backup:", backupPath)

    // Write new config (agent provides the complete content)
    if err := os.WriteFile(path, []byte(diff), 0600); err != nil {
        return "", fmt.Errorf("write failed: %w", err)
    }

    fmt.Println("✅ Configuration written")
    return "Config updated successfully", nil
}

// Claude API integration for deciding next action
func (a *Agent) getNextAction(ctx context.Context, prompt string) (Action, error) {
    resp, err := a.client.Messages.Create(ctx, anthropic.MessageCreateParams{
        Model:     "claude-sonnet-4-20250514",
        MaxTokens: 2000,
        Messages: []anthropic.MessageParam{
            anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
        },
    })

    if err != nil {
        return Action{}, err
    }

    // Parse response to extract action
    return a.parseAction(resp.Content[0].Text)
}

// parseAction converts Claude's response into an Action
func (a *Agent) parseAction(response string) (Action, error) {
    // Parse structured response from Claude
    // Expected format:
    // ACTION: <type>
    // <details>

    // Implementation parses the response and creates Action struct
    // ...
}
```

### Integration with Existing Code

```go
// app/cmd/root.go - Add to PreRunE validation hook
func validateAWSProfiles() error {
    v := validator.New()
    results, err := v.ValidateAllProfiles(ctx, yamlFiles)

    if err != nil || !allValid(results) {
        // Show validation errors
        printValidationResults(results)

        // Offer options
        fmt.Println("\nHow would you like to fix this?")
        fmt.Println("  [1] Interactive Wizard")
        fmt.Println("  [2] AI Agent (automatic)")
        fmt.Println("  [3] Skip for now")

        choice := promptChoice()

        switch choice {
        case "1":
            return runInteractiveWizard(yamlFiles)
        case "2":
            return runAIAgent(yamlFiles)
        case "3":
            return nil // skip
        }
    }

    return nil
}

func runAIAgent(yamlFiles []string) error {
    agent, err := ssoagent.NewAgent()
    if err != nil {
        return err
    }

    for _, yamlPath := range yamlFiles {
        ctx := ssoagent.BuildContext(yamlPath)
        if err := agent.Run(context.Background(), ctx); err != nil {
            return err
        }
    }

    return nil
}
```

## Agent Behavior Examples

### Scenario 1: Detecting Configuration Type

```
🤔 THINKING:
   - ~/.aws/config exists
   - Profile 'dev' found
   - Has 'sso_session' field → Modern SSO config
   - sso_session points to 'my-sso' → Need to check that section
   - [sso-session my-sso] section found
   - Missing 'sso_start_url' in sso-session
   - This is a critical field, need to ask user
```

### Scenario 2: Smart Question Prioritization

```
🤔 THINKING:
   - Need: sso_start_url, sso_region, sso_role_name
   - Start URL is most important (can't proceed without it)
   - sso_region has sensible default (us-east-1)
   - Role name has common options (can suggest)
   - Will ask for start URL first, then role
```

### Scenario 3: Error Recovery

```
🔧 ACTION: Running: aws sso login --profile dev
❌ Failed: Error loading SSO Token: Token for https://d-xxxxx.awsapps.com/start does not exist

🤔 THINKING:
   - Login failed but for a different start URL
   - User's config has old/wrong start URL
   - Need to ask for correct start URL
   - Should update config with new URL
   - Then retry login

💬 The SSO login failed. It looks like the start URL in your config might be outdated.

? What is the correct AWS SSO start URL?
  Current (not working): https://d-xxxxx.awsapps.com/start

  New URL: https://acme-corp.awsapps.com/start

🔧 ACTION: Updating configuration with new start URL
✅ Updated

🔧 ACTION: Retrying: aws sso login --profile dev
✅ Success! Authenticated.
```

### Scenario 4: Account ID Mismatch

```
🔧 ACTION: Verifying credentials
   Running: aws sts get-caller-identity --profile dev

   Account: 999888777666

🤔 THINKING:
   - API call succeeded but account ID mismatch
   - Expected (from dev.yaml): 123456789012
   - Actual (from AWS): 999888777666
   - Either YAML is wrong or user selected wrong role
   - Need to ask which is correct

💬 Account ID mismatch detected!

  Your dev.yaml says:     123456789012
  AWS API returned:       999888777666

? Which is correct?
  [1] Update dev.yaml to 999888777666 (AWS is correct)
  [2] Fix AWS config (YAML is correct, selected wrong role)

  Choice: 1

🔧 ACTION: Updating project/dev.yaml
   account_id: 123456789012 → 999888777666
✅ Updated

✅ Profile is now correctly configured!
```

## CLI Integration

### Command Structure

```bash
# Setup wizard (both options)
./meroku sso setup              # Shows menu: [1] Wizard [2] AI Agent
./meroku sso wizard             # Force wizard mode
./meroku sso agent              # Force AI agent mode

# With profile selection
./meroku sso agent --profile dev
./meroku sso agent --profile staging
./meroku sso agent --all

# With options
./meroku sso agent --profile dev --max-iterations 15
./meroku sso agent --all --verbose
```

### Menu Integration

```
Main Menu:

Infrastructure:
  1. Generate Terraform from YAML
  2. Plan Infrastructure Changes
  3. Apply Infrastructure Changes
  4. Destroy Infrastructure

DNS:
  5. DNS Setup & Management

AWS Configuration:
  6. 🔐 AWS SSO Setup Wizard
  7. 🤖 AWS SSO AI Agent
  8. ✓ Validate AWS Configuration

AI Tools:
  9. 🤖 Terraform Troubleshooting Agent
  10. Exit

? Select an option:
```

## Benefits of AI Agent Approach

### vs. Manual Wizard

**Wizard:**
- Fixed question sequence
- Can't adapt to different scenarios
- Requires anticipating all edge cases
- May ask unnecessary questions

**AI Agent:**
- Adapts to situation
- Asks only relevant questions
- Handles unexpected configurations
- Can reason through complex scenarios

### vs. Pure Validation

**Validation Only:**
- Shows errors, user fixes manually
- No guidance on how to fix
- User might make mistakes

**AI Agent:**
- Fixes automatically
- Explains what it's doing
- Validates after each change
- Iterates until working

## Error Handling

### Agent Stuck Detection

```go
func (a *Agent) isStuck(ctx *AgentContext) bool {
    // Check if agent is repeating same actions
    if len(ctx.ActionHistory) < 3 {
        return false
    }

    last3 := ctx.ActionHistory[len(ctx.ActionHistory)-3:]

    // Same action type 3 times in a row
    if last3[0].Type == last3[1].Type && last3[1].Type == last3[2].Type {
        return true
    }

    return false
}

// In Run() method:
if a.isStuck(agentCtx) {
    fmt.Println("⚠️  Agent seems stuck. Would you like to:")
    fmt.Println("  [1] Continue automated")
    fmt.Println("  [2] Switch to manual wizard")
    fmt.Println("  [3] Stop and show debug info")

    // Handle user choice...
}
```

### Anthropic API Errors

```go
if err != nil {
    if strings.Contains(err.Error(), "rate limit") {
        fmt.Println("⏳ Rate limit hit, waiting 60s...")
        time.Sleep(60 * time.Second)
        continue
    }

    if strings.Contains(err.Error(), "API key") {
        return fmt.Errorf("ANTHROPIC_API_KEY not set or invalid")
    }

    // Fall back to wizard
    fmt.Println("⚠️  AI agent error, falling back to wizard...")
    return runInteractiveWizard(yamlFiles)
}
```

## Testing Strategy

### Unit Tests

```go
// Test agent action parsing
func TestParseAction(t *testing.T) {
    response := `ACTION: ASK
Question: What is your SSO start URL?`

    action, err := parseAction(response)
    assert.NoError(t, err)
    assert.Equal(t, ActionTypeAsk, action.Type)
}

// Test context building
func TestBuildContext(t *testing.T) {
    ctx := BuildContext("testdata/dev.yaml")
    assert.Equal(t, "dev", ctx.ProfileName)
    assert.NotNil(t, ctx.YAMLConfig)
}
```

### Integration Tests (with mocked LLM)

```go
func TestAgentFlowComplete(t *testing.T) {
    // Mock Anthropic API responses
    mockResponses := []string{
        `ACTION: THINK\nProfile missing, need to create it`,
        `ACTION: ASK\nWhat is your SSO start URL?`,
        `ACTION: WRITE\n[sso-session default-sso]...`,
        `ACTION: EXEC\naws sso login --profile dev`,
        `ACTION: VALIDATE\nCheck credentials work`,
    }

    agent := NewAgentWithMock(mockResponses)
    ctx := BuildTestContext()

    err := agent.Run(context.Background(), ctx)
    assert.NoError(t, err)
}
```

## Documentation

### User Documentation

```markdown
## AWS SSO AI Agent

The AI Agent can automatically configure your AWS SSO profiles by:
- Analyzing your current configuration
- Asking targeted questions
- Fixing issues automatically
- Verifying everything works

### When to Use

Use the AI Agent when:
- You're new to AWS SSO
- Your configuration is broken and you're not sure why
- You have multiple environments to set up
- You want automated troubleshooting

### How to Use

1. Run: `./meroku sso agent`
2. Answer the agent's questions
3. Wait for it to complete setup
4. Done!

The agent will:
✓ Create backups before changes
✓ Explain what it's doing
✓ Validate each step
✓ Show clear success/failure messages
```

## Summary

The AI Agent provides:
- ✅ **Intelligent troubleshooting** - Adapts to any scenario
- ✅ **Minimal user input** - Asks only essential questions
- ✅ **Automatic fixing** - No manual file editing
- ✅ **ReAct pattern** - Think → Act → Validate loop
- ✅ **Error recovery** - Handles failures gracefully
- ✅ **Integration** - Works with existing validation system

This complements the manual wizard by providing an autonomous option for users who prefer automated problem-solving.
