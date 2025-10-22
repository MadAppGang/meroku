# AI Troubleshooting Agent - Refined Integration Plan

## Overview

This document outlines the **integrated** approach for adding an autonomous AI troubleshooting agent directly into the terraform apply error flow. The agent is **NOT a separate menu item** but a seamless part of the error handling experience.

## User Flow

### Current Flow
```
Terraform apply → Error occurs → Error screen with:
  - Error details
  - [a] Ask AI (gets suggestions, doesn't execute)
  - [Enter] Return to menu
```

### New Integrated Flow
```
Terraform apply → Error occurs → Error screen with:
  - Error details
  - [a] Ask AI (existing - suggests fixes only)
  - [s] Solve with AI ← NEW autonomous agent
  - [Enter] Return to menu

User selects "Solve with AI" →
  Screen transitions to Agent Flow View →
  Agent autonomously:
    1. Analyzes error
    2. Plans troubleshooting steps
    3. Executes tools (AWS CLI, shell commands, file operations)
    4. Shows real-time progress
    5. Iterates until fixed or max attempts reached
  → Returns to main menu (success or failure)
```

## Key Integration Points

### 1. Error Screen Modifications

**File:** `app/terraform_apply_tui.go` and `app/terraform_plan_modern_tui.go`

**Current State:**
- Error screen shows when `applyState.hasErrors == true && applyState.applyComplete == true`
- Footer shows `[a] AI Help` if `isAIHelperAvailable()` returns true
- Pressing 'a' triggers `fetchAIHelp()` which shows suggestions

**Required Changes:**
- Add new footer option: `[s] Solve with AI (autonomous)`
- Handle 's' key press to initiate agent flow
- Pass complete error context to agent (no re-parsing needed)

### 2. New View Mode

**File:** `app/terraform_plan_modern_tui.go`

**Current View Modes:**
```go
const (
    dashboardView viewMode = iota
    applyView
    fullScreenDiffView
    aiHelpView  // Existing - shows suggestions only
)
```

**Add New Mode:**
```go
const (
    dashboardView viewMode = iota
    applyView
    fullScreenDiffView
    aiHelpView          // Existing - shows suggestions
    aiAgentView         // NEW - autonomous agent execution
)
```

### 3. Agent Context Structure

**File:** `app/ai_agent_types.go` (NEW)

```go
// AgentContext contains all information needed for autonomous troubleshooting
type AgentContext struct {
    // Error information
    ErrorMessages   []string              // Collected terraform errors
    Diagnostics     map[string]*DiagnosticInfo // Full diagnostic details
    FailedResources []completedResource   // Resources that failed

    // Environment context
    Environment     string   // dev/prod/staging
    WorkingDir      string   // Current terraform directory
    AWSProfile      string   // AWS profile being used
    AWSRegion       string   // AWS region
    AccountID       string   // AWS account ID

    // Terraform state
    TerraformState  string   // Recent terraform state output
    PlanOutput      string   // Original plan output
    ApplyLogs       []logEntry // Full apply logs
}

// AgentStep represents a single step in the troubleshooting flow
type AgentStep struct {
    ID          int           // Step number
    Type        StepType      // analyze, aws_cli, shell, file_read, file_write
    Description string        // Human-readable description
    Command     string        // Command being executed (if applicable)
    Status      StepStatus    // pending, running, completed, failed
    Output      string        // Command output or analysis result
    Duration    time.Duration // Execution time
    Error       string        // Error message if failed
    Expandable  bool          // Can user expand to see details?
    Expanded    bool          // Currently expanded?
}

type StepType string

const (
    StepAnalyze    StepType = "analyze"     // AI analysis
    StepAWSCLI     StepType = "aws_cli"     // AWS CLI command
    StepShell      StepType = "shell"       // Shell command
    StepFileRead   StepType = "file_read"   // Read file
    StepFileWrite  StepType = "file_write"  // Write/edit file
    StepTerraform  StepType = "terraform"   // Terraform command
)

type StepStatus string

const (
    StepPending   StepStatus = "pending"
    StepRunning   StepStatus = "running"
    StepCompleted StepStatus = "completed"
    StepFailed    StepStatus = "failed"
    StepSkipped   StepStatus = "skipped"
)

// AgentState tracks the agent's execution state
type AgentState struct {
    Context        AgentContext
    Steps          []AgentStep
    CurrentStep    int
    IsRunning      bool
    IsPaused       bool
    MaxIterations  int
    Iteration      int
    Success        bool
    FinalMessage   string
    StartTime      time.Time

    // UI state
    ScrollOffset   int
    SelectedStep   int
    ShowHelp       bool
}
```

### 4. Agent Flow View UI

**File:** `app/ai_agent_tui.go` (NEW)

**Layout:**
```
┌─────────────────────────────────────────────────────────────┐
│ 🤖 AI Agent - Autonomous Troubleshooting                    │
│ Environment: dev | Iteration: 1/5 | Status: Running         │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│ Troubleshooting Steps                                       │
├─────────────────────────────────────────────────────────────┤
│ ✅ 1. Analyze Error                                         │
│     └─ Identified: IAM permission denied on S3              │
│ ⏳ 2. Check IAM Permissions (Running...)                    │
│     $ aws iam get-user-policy --user-name terraform-user    │
│     └─ Output: [Expandable - press Space]                   │
│ ⏸  3. Update IAM Policy (Pending)                           │
│ ⏸  4. Retry Terraform Apply (Pending)                       │
│                                                             │
│ [Scroll for more steps]                                     │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│ Step 2 Details (Expanded)                                   │
├─────────────────────────────────────────────────────────────┤
│ Command: aws iam get-user-policy --user-name terraform-user │
│                                                             │
│ Output:                                                     │
│ {                                                           │
│   "PolicyName": "TerraformPolicy",                          │
│   "PolicyDocument": {                                       │
│     "Version": "2012-10-17",                                │
│     "Statement": [...]                                      │
│   }                                                         │
│ }                                                           │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│ [↑↓] Navigate  [Space] Expand  [p] Pause  [q] Stop  [?] Help│
└─────────────────────────────────────────────────────────────┘
```

**Features:**
- Real-time step-by-step progress display
- Color-coded status icons (✅ completed, ⏳ running, ⏸ pending, ❌ failed)
- Expandable steps to view command output
- Scrollable list of steps
- Agent iteration counter
- Pause/resume controls
- Clear visual feedback

### 5. Message Types

**File:** `app/ai_agent_types.go` (NEW)

```go
// Bubble Tea messages for agent flow

type agentStartMsg struct{}

type agentStepStartMsg struct {
    step AgentStep
}

type agentStepCompleteMsg struct {
    stepID int
    output string
    err    error
}

type agentIterationCompleteMsg struct {
    iteration int
    success   bool
}

type agentCompleteMsg struct {
    success bool
    message string
}

type agentErrorMsg struct {
    err error
}

type agentPausedMsg struct{}

type agentResumedMsg struct{}
```

### 6. Agent Execution Engine

**File:** `app/ai_agent_executor.go` (NEW)

**Core Functions:**

```go
// startAgent initiates the autonomous troubleshooting agent
func (m *modernPlanModel) startAgent() tea.Cmd {
    return func() tea.Msg {
        // Build context from current error state
        ctx := AgentContext{
            ErrorMessages:   collectErrorMessages(m.applyState),
            Diagnostics:     m.applyState.diagnostics,
            FailedResources: getFailedResources(m.applyState),
            Environment:     detectEnvironment(),
            WorkingDir:      getCurrentDir(),
            AWSProfile:      detectAWSProfile(),
            AWSRegion:       detectAWSRegion(),
            AccountID:       detectAccountID(),
            TerraformState:  getTerraformState(),
            PlanOutput:      m.planOutput,
            ApplyLogs:       m.applyState.logs,
        }

        // Initialize agent state
        m.agentState = &AgentState{
            Context:       ctx,
            Steps:         []AgentStep{},
            MaxIterations: 5,
            Iteration:     1,
            IsRunning:     true,
            StartTime:     time.Now(),
        }

        return agentStartMsg{}
    }
}

// runAgentIteration executes one iteration of the agent loop
func (m *modernPlanModel) runAgentIteration() tea.Cmd {
    return func() tea.Msg {
        // Call Claude API with context
        plan := callClaudeForPlan(m.agentState.Context)

        // Parse plan into steps
        steps := parsePlanIntoSteps(plan)
        m.agentState.Steps = append(m.agentState.Steps, steps...)

        // Execute steps sequentially
        for i, step := range steps {
            m.sendMsg(agentStepStartMsg{step: step})

            output, err := executeStep(step)

            m.sendMsg(agentStepCompleteMsg{
                stepID: step.ID,
                output: output,
                err:    err,
            })

            if err != nil {
                // Step failed - decide whether to continue or abort
                if isFatalError(err) {
                    return agentErrorMsg{err: err}
                }
            }
        }

        // Check if problem is solved
        if isProblemSolved() {
            return agentCompleteMsg{
                success: true,
                message: "Successfully resolved the infrastructure issue!",
            }
        }

        // Check iteration limit
        m.agentState.Iteration++
        if m.agentState.Iteration > m.agentState.MaxIterations {
            return agentCompleteMsg{
                success: false,
                message: "Reached maximum iterations without resolving the issue.",
            }
        }

        // Continue to next iteration
        return agentIterationCompleteMsg{
            iteration: m.agentState.Iteration - 1,
            success:   false,
        }
    }
}

// executeStep runs a single troubleshooting step
func executeStep(step AgentStep) (string, error) {
    switch step.Type {
    case StepAnalyze:
        // AI analysis - no execution needed
        return step.Description, nil

    case StepAWSCLI:
        // Execute AWS CLI command
        cmd := exec.Command("aws", parseAWSCommand(step.Command)...)
        output, err := cmd.CombinedOutput()
        return string(output), err

    case StepShell:
        // Execute shell command
        cmd := exec.Command("sh", "-c", step.Command)
        output, err := cmd.CombinedOutput()
        return string(output), err

    case StepFileRead:
        // Read file
        content, err := os.ReadFile(step.Command)
        return string(content), err

    case StepFileWrite:
        // Write file (step.Command is path, step.Output is content)
        err := os.WriteFile(step.Command, []byte(step.Output), 0644)
        return "File written successfully", err

    case StepTerraform:
        // Execute terraform command
        cmd := exec.Command("terraform", parseTerraformCommand(step.Command)...)
        output, err := cmd.CombinedOutput()
        return string(output), err

    default:
        return "", fmt.Errorf("unknown step type: %s", step.Type)
    }
}
```

### 7. Integration with Existing Error Flow

**File:** `app/terraform_plan_modern_tui.go`

**Modify the Update() function to handle 's' key:**

```go
case key.Matches(msg, m.keys.Apply):
    // Existing code for 'a' key (Ask AI)
    if m.currentView == applyView && m.applyState != nil && m.applyState.applyComplete {
        if m.applyState.hasErrors && isAIHelperAvailable() {
            // Show menu or handle 'a' vs 's'
            // 'a' → aiHelpView (existing - suggestions only)
            // 's' → aiAgentView (new - autonomous execution)
        }
    }

// Add new key binding for 's'
case msg.String() == "s":
    // Only available when errors exist and agent is available
    if m.currentView == applyView &&
       m.applyState != nil &&
       m.applyState.applyComplete &&
       m.applyState.hasErrors &&
       isAIHelperAvailable() {

        // Transition to agent view
        m.currentView = aiAgentView

        // Start the agent
        return m, m.startAgent()
    }
```

**Modify the footer to show both options:**

```go
func (m *modernPlanModel) renderApplyFooter() string {
    help := "[Tab] Switch Section  "

    if m.applyState.selectedSection == 2 {
        help += "[↑↓] Scroll Logs  "
    }

    if m.applyState.applyComplete {
        if m.applyState.hasErrors && isAIHelperAvailable() {
            help += "[a] Ask AI (suggestions)  [s] Solve with AI (autonomous)  "
        }
        help += "[Enter] Continue  "
    }

    help += "[q] Quit"
    return footerStyle.Width(m.width).Render(help)
}
```

### 8. Smooth Transitions

**Screen Flow:**

```
Apply View (with errors)
    ↓ User presses 's'
Agent View (loading)
    ↓ Agent initialized
Agent View (iteration 1)
    ↓ Steps execute
Agent View (iteration 2)
    ↓ ...
Agent View (complete - success/failure)
    ↓ User presses Enter
Main Menu
```

**Key Points:**
- No jarring screen changes - smooth fade or instant replace
- Context is preserved (user can return to apply view if needed)
- Clear visual indication of which mode is active
- Agent view uses full screen (replaces apply view completely)

### 9. Context Passing Strategy

**NO re-parsing needed** - everything is already available:

```go
// From terraform_apply_tui.go and modernPlanModel
type AgentContext struct {
    // Already available in m.applyState
    ErrorMessages:   extractFromLogs(m.applyState.logs)
    Diagnostics:     m.applyState.diagnostics  // Direct copy
    FailedResources: getFailedFromCompleted(m.applyState.completed)

    // Already in environment
    Environment:     detectFromWorkingDir()    // env/dev, env/prod, etc.
    WorkingDir:      os.Getwd()               // Current dir
    AWSProfile:      os.Getenv("AWS_PROFILE")  // From env
    AWSRegion:       detectFromTerraform()     // From terraform vars
    AccountID:       m.yamlConfig.AccountID    // From YAML config

    // Available from existing structures
    TerraformState:  runTerraformShow()        // terraform show
    PlanOutput:      m.rawPlanJSON             // Original plan
    ApplyLogs:       m.applyState.logs         // Full logs
}
```

### 10. User Control Features

**Pause/Resume:**
- User can press 'p' to pause agent mid-execution
- Current step completes, then agent waits
- Press 'p' again to resume

**Stop:**
- User can press 'q' to stop agent completely
- Returns to apply view

**Review Mode (Optional Future Enhancement):**
- Before executing each step, agent can ask for confirmation
- Toggle with 'r' key

**Expand Steps:**
- Press Space or Enter on a step to expand/collapse details
- Shows full command output

### 11. Visual Feedback

**Status Indicators:**
- ✅ Green checkmark - Step completed successfully
- ⏳ Yellow spinner - Step currently running
- ⏸ Gray pause icon - Step pending
- ❌ Red X - Step failed
- ⏭ Blue arrow - Step skipped

**Progress:**
- Overall iteration counter (1/5, 2/5, etc.)
- Per-step duration timers
- Overall elapsed time

**Real-time Updates:**
- Agent messages stream to UI as they happen
- Command output updates as it's received
- No full-screen refreshes - only update changed sections

## Implementation Phases

### Phase 1: Foundation (Files to Create)
1. **`app/ai_agent_types.go`**
   - Define AgentContext, AgentState, AgentStep
   - Define message types
   - Define constants and enums

2. **`app/ai_agent_tui.go`**
   - renderAgentView() - Main view function
   - renderAgentSteps() - Step list renderer
   - renderAgentStepDetails() - Expanded step view
   - renderAgentHeader() - Header with status
   - renderAgentFooter() - Footer with controls

3. **`app/ai_agent_executor.go`**
   - startAgent() - Initialize agent
   - runAgentIteration() - Execute one iteration
   - executeStep() - Run a single step
   - callClaudeForPlan() - API integration
   - parsePlanIntoSteps() - Parse Claude response

### Phase 2: Integration (Files to Modify)
1. **`app/terraform_plan_modern_tui.go`**
   - Add aiAgentView to viewMode enum
   - Add 's' key handler
   - Add case for aiAgentView in Update()
   - Add case for aiAgentView in View()
   - Handle agent messages in Update()

2. **`app/terraform_apply_tui.go`**
   - Add agentState field to modernPlanModel
   - Update footer rendering to show both options

### Phase 3: Polish
1. **Error Handling**
   - Graceful failures
   - Timeout handling
   - Network error recovery

2. **User Experience**
   - Smooth animations
   - Keyboard shortcuts help
   - Clear error messages

3. **Documentation**
   - Update README with agent feature
   - Add examples of agent solving real issues

## Files Summary

### New Files (9 files)

1. **`ai_docs/AI_AGENT_REFINED_PLAN.md`** (this file)
   - Integration plan and architecture

2. **`app/ai_agent_types.go`**
   - Type definitions for agent system
   - ~200 lines

3. **`app/ai_agent_tui.go`**
   - TUI rendering for agent view
   - ~500 lines

4. **`app/ai_agent_executor.go`**
   - Agent execution engine
   - ~600 lines

5. **`app/ai_agent_claude.go`**
   - Claude API integration
   - ~300 lines

6. **`app/ai_agent_tools.go`**
   - Tool execution (AWS CLI, shell, etc.)
   - ~400 lines

7. **`app/ai_agent_context.go`**
   - Context building and management
   - ~200 lines

8. **`app/ai_agent_utils.go`**
   - Utility functions
   - ~100 lines

9. **`app/ai_agent_test.go`**
   - Unit tests for agent
   - ~300 lines

### Modified Files (3 files)

1. **`app/terraform_plan_modern_tui.go`**
   - Add aiAgentView mode
   - Add 's' key handler
   - Integrate agent messages
   - ~50 lines changed

2. **`app/terraform_apply_tui.go`**
   - Add agentState field
   - ~10 lines changed

3. **`app/model.go`** (if exists, or terraform_plan_modern_tui.go)
   - Add agentState to modernPlanModel struct
   - ~5 lines changed

### Total Estimated Code
- **New:** ~2,600 lines
- **Modified:** ~65 lines
- **Total:** ~2,665 lines

## Key Differences from Previous Plan

### What Changed
1. **NOT a separate menu item** - Integrated into error flow
2. **Two options at error screen:**
   - `[a]` Ask AI - Non-autonomous suggestions (existing)
   - `[s]` Solve with AI - Autonomous agent (new)
3. **Smooth transition** - Agent view replaces current screen
4. **Context passing** - Direct from existing state, no re-parsing
5. **Reuses environment** - AWS profile, region, credentials already set
6. **Clear visual distinction** - Different view mode, not overlay

### What Stayed the Same
1. Autonomous execution loop
2. Tool usage (AWS CLI, shell, file ops)
3. Step-by-step UI with expandable details
4. Iteration limit (max 5)
5. Real-time progress display

## Success Criteria

1. ✅ User can press 's' on error screen to start agent
2. ✅ Agent view shows real-time step execution
3. ✅ Agent can execute AWS CLI commands
4. ✅ Agent can execute shell commands
5. ✅ Agent can read/write files
6. ✅ Agent iterates until success or max attempts
7. ✅ User can expand steps to see details
8. ✅ User can pause/stop agent
9. ✅ Smooth return to main menu after completion
10. ✅ Error handling is robust

## Next Steps

1. **Review this plan** - Confirm integration approach
2. **Create type definitions** - `ai_agent_types.go`
3. **Build TUI rendering** - `ai_agent_tui.go`
4. **Implement executor** - `ai_agent_executor.go`
5. **Integrate with error flow** - Modify existing files
6. **Test end-to-end** - With real terraform errors
7. **Polish UX** - Smooth animations, clear feedback
8. **Document** - Update README and examples

## Example Agent Flow

**Scenario:** S3 bucket creation fails due to IAM permissions

```
Terraform Apply Fails
  ↓
Error Screen:
  ❌ Failed: aws_s3_bucket.example
  Error: AccessDenied: User is not authorized to perform: s3:CreateBucket

  [a] Ask AI (get suggestions)  [s] Solve with AI (auto-fix)  [Enter] Return
  ↓ User presses 's'

Agent View:
  🤖 AI Agent - Autonomous Troubleshooting
  Environment: dev | Iteration: 1/5 | Status: Running

  Steps:
  ✅ 1. Analyze Error (0.5s)
      └─ Root cause: IAM policy missing s3:CreateBucket permission

  ⏳ 2. Check Current IAM Policy (Running...)
      $ aws iam get-user-policy --user-name terraform-dev

  ⏸ 3. Update IAM Policy (Pending)

  ⏸ 4. Retry Terraform Apply (Pending)

  ↓ Step 2 completes

  ✅ 2. Check Current IAM Policy (1.2s)
      $ aws iam get-user-policy --user-name terraform-dev
      └─ Found: Missing s3:CreateBucket and s3:PutBucketPolicy

  ⏳ 3. Update IAM Policy (Running...)
      Writing updated policy to iam-policy-update.json

  ↓ Step 3 completes

  ✅ 3. Update IAM Policy (0.8s)
      $ aws iam put-user-policy --user-name terraform-dev --policy-document file://iam-policy-update.json
      └─ Policy updated successfully

  ⏳ 4. Retry Terraform Apply (Running...)
      $ cd env/dev && terraform apply -auto-approve

  ↓ Step 4 completes

  ✅ 4. Retry Terraform Apply (12.3s)
      Apply complete! Resources: 1 added, 0 changed, 0 deleted.

  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  ✅ SUCCESS

  Successfully resolved the infrastructure issue!
  Total time: 14.8s

  [Enter] Return to menu
```

This flow demonstrates:
- Clear step-by-step progress
- Real commands being executed
- Success outcome
- User can see exactly what was done

---

**Ready for Implementation!**
