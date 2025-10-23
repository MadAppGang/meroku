# AWS SSO AI Wizard - Architecture Diagram

Visual representation of the improved AWS SSO wizard architecture and data flow.

---

## System Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         AWS SSO AI WIZARD SYSTEM                             │
│                                                                              │
│  ┌─────────────┐    ┌──────────────┐    ┌─────────────┐    ┌─────────────┐│
│  │   User      │───▶│  Main Menu   │───▶│ SSO Agent   │───▶│  ReAct Loop ││
│  │  Interface  │◀───│  Selection   │◀───│ Coordinator │◀───│  Executor   ││
│  └─────────────┘    └──────────────┘    └─────────────┘    └─────────────┘│
│         │                                        │                  │        │
│         │                                        │                  │        │
│    ┌────▼────────────────────────────────────────▼──────────────────▼─────┐ │
│    │                         STATE MANAGEMENT                              │ │
│    │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐               │ │
│    │  │ Load State   │  │  Save State  │  │ Cleanup State│               │ │
│    │  │ (if exists)  │  │ (per action) │  │ (on success) │               │ │
│    │  └──────────────┘  └──────────────┘  └──────────────┘               │ │
│    │         /tmp/meroku_sso_state_{profile}_{date}.json                  │ │
│    └──────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Component Architecture

```
┌────────────────────────────────────────────────────────────────────────────┐
│                          COMPONENT STRUCTURE                                │
└────────────────────────────────────────────────────────────────────────────┘

app/
├── aws_sso_agent.go ◀──────────────┐ Main Agent Orchestrator
│   ├── Run()                       │ - ReAct loop coordination
│   ├── buildContext()              │ - Iteration management
│   ├── getNextAction()             │ - State persistence
│   ├── executeAction()             │ - Completion detection
│   ├── isStuck()                   │
│   └── printSummary()              │
│                                   │
├── aws_sso_agent_state.go ◀────────┤ State Persistence
│   ├── SaveState()                 │ - JSON serialization
│   ├── LoadState()                 │ - Version validation
│   ├── GetStateFilePath()          │ - Cleanup management
│   ├── CleanupStateFile()          │
│   └── ListStateFiles()            │
│                                   │
├── aws_config_reader.go ◀──────────┤ AWS Config I/O
│   ├── ReadAWSConfig()             │ - Parse INI format
│   ├── ParseAWSConfigProfiles()    │ - Extract profiles
│   ├── ParseSSOSessions()          │ - Extract sessions
│   ├── GetProfileSection()         │ - Section parsing
│   └── GetSSOSessionSection()      │
│                                   │
├── aws_sso_agent_user_input.go ◀───┤ User Interaction
│   ├── AskChoice()                 │ - Huh library integration
│   ├── AskConfirm()                │ - Input validation
│   ├── AskInput()                  │ - Dropdown selections
│   ├── ValidateURL()               │ - Confirmation dialogs
│   ├── ValidateRegion()            │
│   ├── ValidateAccountID()         │
│   └── ValidateRoleName()          │
│                                   │
├── aws_sso_agent_tools.go ◀────────┤ Tool Implementations
│   ├── toolReadAWSConfig()         │ - 12 specialized tools
│   ├── toolWriteAWSConfig()        │ - Command parsing
│   ├── toolReadYAML()              │ - Execution logic
│   ├── toolWriteYAML()             │ - Result formatting
│   ├── toolAskChoice()             │
│   ├── toolAskConfirm()            │
│   ├── toolAskInput()              │
│   ├── toolWebSearch()             │
│   ├── toolAWSValidate()           │
│   └── [existing tools]            │
│                                   │
├── aws_sso_agent_prompts.go ◀──────┤ Prompt Engineering
│   ├── BuildEnhancedSystemPrompt() │ - Comprehensive docs
│   ├── awsSSODocumentation         │ - Common issues
│   ├── commonSSOIssues             │ - Tool descriptions
│   └── awsSSOTools                 │ - Examples
│                                   │
├── aws_sso_inspector.go (existing) │ Validation Logic
│   ├── InspectProfile()            │ - Profile analysis
│   ├── CheckAWSCLI()               │ - Field validation
│   └── ValidateField()             │ - Type detection
│                                   │
└── aws_config_writer.go (existing) │ Config Writing
    ├── WriteModernSSOProfile()     │ - INI generation
    └── [backup logic]              │ - Backup creation
```

---

## ReAct Loop Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           REACT LOOP EXECUTION                               │
└─────────────────────────────────────────────────────────────────────────────┘

START
  │
  ├─▶ Check for saved state file
  │   ├─ Exists? ──▶ Prompt: "Continue previous session?"
  │   │   ├─ Yes ──▶ LoadState() ──▶ Resume from iteration N
  │   │   └─ No ───▶ Build fresh context
  │   └─ Not exists ──▶ Build fresh context
  │
  ├─▶ Initialize context
  │   ├─ ProfileName: "dev"
  │   ├─ YAMLEnv: loaded from project/dev.yaml
  │   ├─ ConfigPath: ~/.aws/config
  │   ├─ TotalIterations: 0 (or resumed count)
  │   └─ RunNumber: 1 (or incremented)
  │
  ▼
┌─────────────────────────────────────────┐
│    ITERATION LOOP (max 15 per run)      │
│                                         │
│  Iteration N (Run #R, Total: T/30)     │
│  ═══════════════════════════════════    │
│                                         │
│  1. THINK (LLM Decision)               │
│     │                                   │
│     ├─▶ Build enhanced system prompt   │
│     │   ├─ AWS SSO documentation       │
│     │   ├─ Common issues & solutions   │
│     │   ├─ Available tools (12)        │
│     │   ├─ Current context             │
│     │   └─ Action history (last 5-10)  │
│     │                                   │
│     ├─▶ Call Anthropic Claude API      │
│     │   ├─ Model: claude-sonnet-4-5    │
│     │   ├─ MaxTokens: 2048             │
│     │   └─ Timeout: 60 seconds         │
│     │                                   │
│     └─▶ Parse response                 │
│         ├─ THOUGHT: reasoning          │
│         ├─ ACTION: tool name           │
│         └─ COMMAND: exact command      │
│                                         │
│  2. CHECK COMPLETION                    │
│     │                                   │
│     └─▶ If ACTION == "complete"        │
│         ├─ Success! ──▶ Cleanup state  │
│         └─ Exit loop                   │
│                                         │
│  3. ACT (Execute Tool)                 │
│     │                                   │
│     ├─▶ Dispatch to tool handler       │
│     │   ├─ read_aws_config             │
│     │   ├─ write_aws_config            │
│     │   ├─ read_yaml                   │
│     │   ├─ write_yaml                  │
│     │   ├─ ask_choice                  │
│     │   ├─ ask_confirm                 │
│     │   ├─ ask_input                   │
│     │   ├─ web_search                  │
│     │   ├─ aws_validate                │
│     │   ├─ exec (AWS CLI)              │
│     │   └─ write (config)              │
│     │                                   │
│     └─▶ Capture result/error           │
│                                         │
│  4. OBSERVE (Record Results)           │
│     │                                   │
│     ├─▶ Create AgentIteration          │
│     │   ├─ Number: N                   │
│     │   ├─ Thought: from LLM           │
│     │   ├─ Action: tool used           │
│     │   ├─ Command: executed           │
│     │   ├─ Output: result              │
│     │   ├─ Status: success/failed      │
│     │   ├─ Duration: elapsed time      │
│     │   └─ Timestamp: now              │
│     │                                   │
│     └─▶ Append to ActionHistory        │
│                                         │
│  5. PERSIST STATE                       │
│     │                                   │
│     ├─▶ Update TotalIterations         │
│     ├─▶ Update context with results    │
│     └─▶ SaveState() to disk            │
│                                         │
│  6. CHECK STUCK                         │
│     │                                   │
│     └─▶ Same action 3 times in a row?  │
│         ├─ Yes ──▶ Error: Agent stuck  │
│         └─ No ───▶ Continue            │
│                                         │
│  N < 15? ──────────────────────────────┘
│     │
│     ├─ Yes ──▶ Next iteration
│     │
│     └─ No ───▶ Iteration limit reached
│                │
│                ▼
│       ┌────────────────────────────┐
│       │  ITERATION LIMIT HANDLER   │
│       │                            │
│       │  Total < 30?               │
│       │    ├─ Yes                  │
│       │    │  └─▶ Prompt user:     │
│       │    │      "Continue?"      │
│       │    │      ├─ Yes ──▶ NEW   │
│       │    │      │         RUN    │
│       │    │      │   (reset N=0)  │
│       │    │      │   (R++)        │
│       │    │      │                │
│       │    │      └─ No ──▶ Save & │
│       │    │              Exit     │
│       │    │                       │
│       │    └─ No ──▶ Max iterations│
│       │              reached (30)  │
│       │              Save & Fail   │
│       │                            │
│       └────────────────────────────┘
│
▼
END
  ├─▶ Success: Cleanup state file
  ├─▶ Failure: Preserve state file
  └─▶ Show summary with total iterations
```

---

## Tool Execution Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          TOOL EXECUTION FLOW                                 │
└─────────────────────────────────────────────────────────────────────────────┘

LLM generates:
  THOUGHT: "Need to read current AWS config to see what profiles exist"
  ACTION: read_aws_config
  COMMAND: ~/.aws/config

  │
  ├─▶ Agent.executeAction(action, context)
  │
  ├─▶ Switch on action.Type
  │   └─▶ case "read_aws_config":
  │
  ├─▶ Call toolReadAWSConfig(ctx, command, agentCtx)
  │   │
  │   ├─▶ Parse configPath from command
  │   │   └─▶ "~/.aws/config"
  │   │
  │   ├─▶ Resolve path
  │   │   └─▶ "/Users/jack/.aws/config"
  │   │
  │   ├─▶ ReadAWSConfig(configPath)
  │   │   ├─▶ os.ReadFile()
  │   │   └─▶ Return content string
  │   │
  │   ├─▶ ParseAWSConfigProfiles(content)
  │   │   ├─▶ ini.Load(content)
  │   │   ├─▶ Iterate sections
  │   │   └─▶ Extract profile names
  │   │       └─▶ ["default", "dev", "prod"]
  │   │
  │   ├─▶ ParseSSOSessions(content)
  │   │   ├─▶ ini.Load(content)
  │   │   ├─▶ Iterate sections
  │   │   └─▶ Extract session names
  │   │       └─▶ ["default-sso", "my-sso"]
  │   │
  │   ├─▶ Store content in agentCtx.AWSConfigContent
  │   │
  │   └─▶ Format result message
  │       ├─▶ "Read AWS config from /Users/jack/.aws/config"
  │       ├─▶ "Found 3 profiles: default, dev, prod"
  │       ├─▶ "Found 2 sso-sessions: default-sso, my-sso"
  │       └─▶ "\nConfig content:\n[...]"
  │
  ├─▶ Return SSOAgentAction
  │   ├─ Type: "read_aws_config"
  │   ├─ Result: formatted message
  │   └─ Error: nil (or error object)
  │
  └─▶ Add to ActionHistory
      └─▶ Available for next iteration's prompt
```

---

## User Input Flow (Huh Library)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         USER INPUT FLOW (HUH)                                │
└─────────────────────────────────────────────────────────────────────────────┘

LLM generates:
  ACTION: ask_input
  COMMAND: QUESTION:Enter SSO start URL|VALIDATOR:url|PLACEHOLDER:https://...

  │
  ├─▶ toolAskInput(ctx, command, agentCtx)
  │
  ├─▶ Parse command
  │   ├─▶ QUESTION: "Enter SSO start URL"
  │   ├─▶ VALIDATOR: "url"
  │   └─▶ PLACEHOLDER: "https://..."
  │
  ├─▶ Call AskInput(question, placeholder, validator)
  │   │
  │   ├─▶ Create huh.Input field
  │   │   ├─ Title: "Enter SSO start URL"
  │   │   ├─ Placeholder: "https://..."
  │   │   └─ Validator: ValidateURL
  │   │
  │   ├─▶ Display interactive prompt
  │   │   ┌─────────────────────────────────────┐
  │   │   │ Enter SSO start URL                │
  │   │   │ > https://mycompany.awsapps.com/.. │
  │   │   │   ────────────────────────────────  │
  │   │   │   https://...                      │
  │   │   └─────────────────────────────────────┘
  │   │
  │   ├─▶ User types input
  │   │   └─▶ "https://mycompany.awsapps.com/start"
  │   │
  │   ├─▶ ValidateURL(input)
  │   │   ├─ Starts with https:// ? ✓
  │   │   ├─ Contains .awsapps.com ? ✓
  │   │   └─ Valid ✓
  │   │
  │   └─▶ Return validated input
  │
  ├─▶ Create SSOAgentAction
  │   ├─ Type: "ask_input"
  │   ├─ Question: "Enter SSO start URL"
  │   ├─ Answer: "https://mycompany.awsapps.com/start"
  │   └─ Result: "User entered: https://..."
  │
  └─▶ Store in context
      └─▶ agentCtx.SSOStartURL = answer
```

---

## State Persistence Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        STATE PERSISTENCE FLOW                                │
└─────────────────────────────────────────────────────────────────────────────┘

SAVE STATE (after each action):

  agentCtx updated with action result
    │
    ├─▶ Create SSOAgentState struct
    │   ├─ Version: "1.0"
    │   ├─ ProfileName: "dev"
    │   ├─ SaveTime: now
    │   ├─ Context: agentCtx (includes all history)
    │   ├─ TotalIterations: 7
    │   └─ RunNumber: 1
    │
    ├─▶ Marshal to JSON (with indentation)
    │   └─▶ {
    │         "version": "1.0",
    │         "profile_name": "dev",
    │         "save_time": "2025-10-23T14:30:00Z",
    │         "context": {
    │           "profile_name": "dev",
    │           "sso_start_url": "https://...",
    │           "action_history": [...]
    │         },
    │         "total_iterations": 7,
    │         "run_number": 1
    │       }
    │
    ├─▶ Write to file
    │   └─▶ /tmp/meroku_sso_state_dev_20251023.json
    │       (permissions: 0600, owner only)
    │
    └─▶ Continue execution


LOAD STATE (at startup):

  Run() starts
    │
    ├─▶ GetStateFilePath("dev")
    │   └─▶ /tmp/meroku_sso_state_dev_20251023.json
    │
    ├─▶ Check if file exists
    │   └─▶ Yes
    │
    ├─▶ Prompt user with AskConfirm()
    │   ┌───────────────────────────────────────┐
    │   │ Previous session found for 'dev'     │
    │   │ Saved: 2025-10-23 14:30:00          │
    │   │                                      │
    │   │ Continue from where you left off?   │
    │   │  [Yes]  [No]                        │
    │   └───────────────────────────────────────┘
    │       │
    │       ├─ User selects "Yes"
    │       │
    │       ├─▶ LoadState(filepath)
    │       │   ├─▶ Read file
    │       │   ├─▶ Unmarshal JSON
    │       │   ├─▶ Verify version == "1.0"
    │       │   └─▶ Return SSOAgentState
    │       │
    │       ├─▶ Extract context
    │       │   ├─ agentCtx = state.Context
    │       │   ├─ TotalIterations = 7 (resume from here)
    │       │   ├─ RunNumber = 2 (increment)
    │       │   └─ ActionHistory = [...] (preserved)
    │       │
    │       └─▶ Resume ReAct loop from iteration 8
    │
    └─▶ Continue execution with loaded state


CLEANUP STATE (on success):

  Agent completes successfully
    │
    ├─▶ ACTION: complete
    │   └─▶ "AWS SSO setup is complete"
    │
    ├─▶ CleanupStateFile(stateFilePath)
    │   ├─▶ Delete: /tmp/meroku_sso_state_dev_20251023.json
    │   └─▶ User sees: "Success! 🎉"
    │
    └─▶ Exit with success
```

---

## Data Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            DATA FLOW                                         │
└─────────────────────────────────────────────────────────────────────────────┘

  User Input
      │
      ├─▶ Profile Name: "dev"
      │   Environment Selection
      │
      ▼
  ┌───────────────────────┐
  │  ProfileInspector     │───────┐
  │  ├─ InspectProfile()  │       │
  │  └─ CheckAWSCLI()     │       │ Validation
  └───────────────────────┘       │ Results
            │                     │
            ▼                     ▼
  ┌─────────────────────────────────────┐
  │      Build Agent Context            │
  │  ├─ ProfileName: "dev"              │
  │  ├─ YAMLEnv: from project/dev.yaml  │
  │  ├─ ConfigPath: ~/.aws/config       │
  │  ├─ ProfileInfo: validation results │
  │  └─ ActionHistory: []               │
  └─────────────────────────────────────┘
            │
            │ Pass to ReAct Loop
            ▼
  ┌─────────────────────────────────────┐
  │       Enhanced System Prompt        │
  │  ├─ AWS SSO Documentation           │
  │  ├─ Common Issues & Solutions       │
  │  ├─ Tool Descriptions (12 tools)    │
  │  ├─ Current Context                 │
  │  └─ Action History (last 5-10)      │
  └─────────────────────────────────────┘
            │
            │ Send to LLM
            ▼
  ┌─────────────────────────────────────┐
  │      Anthropic Claude API           │
  │  Model: claude-sonnet-4-5           │
  │  MaxTokens: 2048                    │
  │  Timeout: 60s                       │
  └─────────────────────────────────────┘
            │
            │ Return decision
            ▼
  ┌─────────────────────────────────────┐
  │         Parse Response              │
  │  ├─ THOUGHT: reasoning              │
  │  ├─ ACTION: tool name               │
  │  └─ COMMAND: parameters             │
  └─────────────────────────────────────┘
            │
            │ Execute tool
            ▼
  ┌─────────────────────────────────────┐
  │         Tool Dispatcher             │
  │  ├─ read_aws_config ────────────┐   │
  │  ├─ write_aws_config            │   │
  │  ├─ read_yaml                   │   │
  │  ├─ write_yaml                  │   │
  │  ├─ ask_choice ──────────────┐  │   │
  │  ├─ ask_confirm              │  │   │
  │  ├─ ask_input                │  │   │
  │  ├─ web_search               │  │   │
  │  ├─ aws_validate             │  │   │
  │  └─ [existing tools]         │  │   │
  └──────────────────────────────┼──┼───┘
            │                    │  │
            ▼                    │  │
  ┌─────────────────────────┐   │  │
  │   File System I/O       │◀──┘  │
  │  ├─ ~/.aws/config       │      │
  │  ├─ project/dev.yaml    │      │
  │  └─ Backups             │      │
  └─────────────────────────┘      │
                                   │
            ┌──────────────────────┘
            ▼
  ┌─────────────────────────┐
  │   User Interaction      │
  │  ├─ huh.Select          │
  │  ├─ huh.Confirm         │
  │  └─ huh.Input           │
  └─────────────────────────┘
            │
            │ Collect results
            ▼
  ┌─────────────────────────────────────┐
  │     Update Agent Context            │
  │  ├─ Add to ActionHistory            │
  │  ├─ Increment iteration count       │
  │  ├─ Store collected data            │
  │  │  └─ SSOStartURL, AccountID, etc. │
  │  └─ Cache read data                 │
  │     └─ AWSConfigContent, YAMLContent│
  └─────────────────────────────────────┘
            │
            │ Persist
            ▼
  ┌─────────────────────────────────────┐
  │       Save State to Disk            │
  │  File: /tmp/meroku_sso_state_*.json │
  │  Format: JSON                       │
  │  Permissions: 0600                  │
  └─────────────────────────────────────┘
            │
            │ Loop continues...
            ▼
  ┌─────────────────────────────────────┐
  │    Check Completion / Limits        │
  │  ├─ ACTION == "complete" ? ──▶ EXIT │
  │  ├─ Iteration < 15 ? ──▶ CONTINUE   │
  │  └─ Total < 30 ? ──▶ PROMPT USER    │
  └─────────────────────────────────────┘
```

---

## Tool Interaction Matrix

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        TOOL INTERACTION MATRIX                               │
└─────────────────────────────────────────────────────────────────────────────┘

Tool Name          | Reads From              | Writes To              | User?
─────────────────────────────────────────────────────────────────────────────
read_aws_config    | ~/.aws/config           | Context cache          | No
write_aws_config   | -                       | ~/.aws/config + backup | No
read_yaml          | project/*.yaml          | Context cache          | No
write_yaml         | project/*.yaml          | project/*.yaml + backup| No
ask_choice         | -                       | Context (answer)       | Yes
ask_confirm        | -                       | Context (boolean)      | Yes
ask_input          | -                       | Context (validated)    | Yes
web_search         | Internet (DuckDuckGo)   | Context cache          | No
aws_validate       | AWS API                 | Validation result      | No*
exec               | Shell                   | Command output         | No
write (legacy)     | -                       | ~/.aws/config          | No

* aws_validate may open browser for SSO login (indirect user interaction)
```

---

## Error Handling Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         ERROR HANDLING FLOW                                  │
└─────────────────────────────────────────────────────────────────────────────┘

Tool execution fails
    │
    ├─▶ Capture error
    │   ├─ Error: error object
    │   └─ Result: "Failed: <error message>"
    │
    ├─▶ Add to ActionHistory
    │   ├─ Type: tool name
    │   ├─ Status: "failed"
    │   ├─ Error: error details
    │   └─ Result: partial output + error
    │
    ├─▶ Save state (preserves error for context)
    │
    ├─▶ Continue to next iteration
    │   └─▶ LLM sees error in history
    │       └─▶ Adapts strategy
    │           ├─ Try different approach
    │           ├─ Search for solution
    │           ├─ Ask user for clarification
    │           └─ Fix root cause
    │
    └─▶ OR: If stuck (same error 3 times)
        └─▶ Exit with error
            ├─ Save state
            └─▶ Show: "Agent stuck, try manual wizard"


Anthropic API fails
    │
    ├─▶ Check error type
    │   ├─ Rate limit ──▶ Wait 60s ──▶ Retry
    │   ├─ Timeout ─────▶ Retry once
    │   ├─ Invalid key ─▶ Exit with error
    │   └─ Other ───────▶ Log and exit
    │
    └─▶ Save state before exit


User cancels (Ctrl+C)
    │
    ├─▶ Context cancellation detected
    │
    ├─▶ Save current state
    │   └─▶ /tmp/meroku_sso_state_*.json
    │
    └─▶ Show message:
        "State saved. Run again to continue."


Iteration limit reached (15)
    │
    ├─▶ Check total iterations
    │   ├─ Total < 30 ──▶ Prompt: "Continue?"
    │   │   ├─ Yes ──▶ Start new run
    │   │   └─ No ───▶ Save & exit
    │   │
    │   └─ Total >= 30 ──▶ Max limit reached
    │       └─▶ Save & fail
```

---

## Success Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           SUCCESS FLOW                                       │
└─────────────────────────────────────────────────────────────────────────────┘

LLM decides: ACTION = "complete"
    │
    ├─▶ Reason: "AWS SSO setup is complete. Credentials validated successfully."
    │
    ├─▶ Display success message
    │   ┌────────────────────────────────────────┐
    │   │ ✅ SUCCESS!                           │
    │   │                                       │
    │   │ AWS SSO setup is complete.            │
    │   │                                       │
    │   │ Summary:                              │
    │   │   ✓ Profile 'dev' configured          │
    │   │   ✓ Account ID: 123456789012         │
    │   │   ✓ Region: us-east-1                │
    │   │   ✓ Total iterations: 7              │
    │   │                                       │
    │   │ You can now use this profile:        │
    │   │   aws s3 ls --profile dev             │
    │   └────────────────────────────────────────┘
    │
    ├─▶ Cleanup state file
    │   └─▶ Delete: /tmp/meroku_sso_state_dev_*.json
    │
    └─▶ Exit with success (code 0)
```

---

## File Paths Reference

```
Configuration Files:
  ~/.aws/config                              AWS SSO configuration
  ~/.aws/config.backup_{timestamp}           Backup before modification
  /Users/jack/mag/infrastructure/project/dev.yaml     Project configuration
  /Users/jack/mag/infrastructure/project/dev.yaml.backup_{timestamp}  Backup

State Files:
  /tmp/meroku_sso_state_{profile}_{date}.json   Agent state persistence
  Format: JSON
  Permissions: 0600 (owner read/write only)
  Retention: Until completion or manual cleanup

Code Files:
  app/aws_sso_agent.go                      Main orchestrator (existing)
  app/aws_sso_agent_state.go                State persistence (new)
  app/aws_config_reader.go                  Config file I/O (new)
  app/aws_sso_agent_user_input.go           Huh library integration (new)
  app/aws_sso_agent_tools.go                Tool implementations (new)
  app/aws_sso_agent_prompts.go              Enhanced prompts (new)
  app/aws_sso_inspector.go                  Validation logic (existing)
  app/aws_config_writer.go                  Config writing (existing)
```

---

## Summary

This architecture provides:

1. **Comprehensive Tool Set**: 12 specialized tools for every aspect of SSO setup
2. **State Persistence**: Save/resume capability for complex scenarios
3. **Enhanced Prompts**: Rich context with documentation, examples, and history
4. **User Interaction**: Beautiful TUI prompts with validation
5. **Error Recovery**: Intelligent adaptation to failures
6. **Progress Tracking**: Clear visibility into what the agent is doing

The design follows the ReAct pattern (Reason → Act → Observe) while adding persistence, better prompts, and more capable tools to handle real-world AWS SSO complexity.
