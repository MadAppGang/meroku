# AI Agent State Diagram

## Complete State Machine

```
┌─────────────────────────────────────────────────────────────────┐
│                        MEROKU APPLICATION                        │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ↓
                    ┌──────────────────┐
                    │   Main Menu      │
                    │  [dashboardView] │
                    └────────┬─────────┘
                             │ User selects "Apply"
                             │
                             ↓
                    ┌──────────────────┐
                    │   Apply View     │
                    │  [applyView]     │
                    │  Status: Running │
                    └────────┬─────────┘
                             │
                    ┌────────┴────────┐
                    │                 │
                    ↓                 ↓
          ┌──────────────────┐   ┌──────────────────┐
          │   Apply Success  │   │   Apply Failed   │
          │  [applyView]     │   │  [applyView]     │
          │  ✅ Complete     │   │  ❌ hasErrors    │
          └────────┬─────────┘   └────────┬─────────┘
                   │                      │
                   │                      │ User has 3 options:
                   │                      │
                   │              ┌───────┴────────────┬──────────────┐
                   │              │                    │              │
                   │         User presses 'a'    User presses 's'  User presses Enter
                   │              │                    │              │
                   │              ↓                    ↓              │
                   │     ┌──────────────────┐  ┌──────────────────┐  │
                   │     │  AI Help View    │  │  AI Agent View   │  │
                   │     │ [aiHelpView]     │  │ [aiAgentView]    │  │
                   │     │ Suggestions Only │  │ Autonomous Fix   │  │
                   │     └────────┬─────────┘  └────────┬─────────┘  │
                   │              │                     │             │
                   │              │ Esc/Enter           │             │
                   │              │                     │             │
                   │              ↓                     │             │
                   │     ┌──────────────────┐          │             │
                   │     │   Apply View     │          │             │
                   │     │  [applyView]     │          │             │
                   │     │  (back to error) │          │             │
                   │     └────────┬─────────┘          │             │
                   │              │                     │             │
                   │              │ Enter               │             │
                   └──────────────┴─────────────────────┴─────────────┘
                                  │
                                  ↓
                         ┌──────────────────┐
                         │   Main Menu      │
                         │  [dashboardView] │
                         └──────────────────┘
```

---

## AI Agent View Internal States

```
┌─────────────────────────────────────────────────────────────────┐
│                       AI AGENT VIEW                              │
│                      [aiAgentView]                               │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ↓
                    ┌──────────────────┐
                    │   Initializing   │
                    │  Loading: true   │
                    │  🔄 Spinner      │
                    └────────┬─────────┘
                             │ Agent initialized
                             │ agentStartMsg
                             ↓
                    ┌──────────────────┐
                    │   Iteration 1    │
                    │  Running: true   │
                    │  Steps: []       │
                    └────────┬─────────┘
                             │
                    ┌────────┴────────────────────┐
                    │                             │
                    ↓                             ↓
          ┌──────────────────┐         ┌──────────────────┐
          │  Executing Steps │         │     Paused       │
          │  ⏳ Step Running │         │  IsPaused: true  │
          └────────┬─────────┘         └────────┬─────────┘
                   │                            │
                   │ All steps complete          │ User presses 'p'
                   │                            │ agentResumedMsg
                   ↓                            │
          ┌──────────────────┐                 │
          │ Check If Solved  │                 │
          │ isProblemSolved()│◄────────────────┘
          └────────┬─────────┘
                   │
          ┌────────┴────────┐
          │                 │
          ↓                 ↓
┌──────────────────┐   ┌──────────────────┐
│  Problem Solved  │   │ Problem Persists │
│  Success: true   │   │ Iteration++      │
└────────┬─────────┘   └────────┬─────────┘
         │                      │
         │                      │ Iteration <= 5?
         │                      │
         │              ┌───────┴────────┐
         │              │                │
         │             Yes               No
         │              │                │
         │              ↓                ↓
         │     ┌──────────────────┐   ┌──────────────────┐
         │     │   Iteration N    │   │  Max Iterations  │
         │     │  Running: true   │   │  Success: false  │
         │     └────────┬─────────┘   └────────┬─────────┘
         │              │                      │
         │              │ (loop)               │
         │              └──────────────────────┘
         │                                     │
         └─────────────────────────────────────┘
                          │
                          ↓
                 ┌──────────────────┐
                 │   Complete       │
                 │  IsRunning: false│
                 │  Show Results    │
                 └────────┬─────────┘
                          │
                          │ User presses Enter
                          │
                          ↓
                 ┌──────────────────┐
                 │   Main Menu      │
                 │  [dashboardView] │
                 └──────────────────┘
```

---

## Step Execution State Machine

```
┌─────────────────────────────────────────────────────────────────┐
│                     AGENT STEP EXECUTION                         │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ↓
                    ┌──────────────────┐
                    │   Step Created   │
                    │  Status: Pending │
                    │  ⏸  Icon         │
                    └────────┬─────────┘
                             │ agentStepStartMsg
                             │
                             ↓
                    ┌──────────────────┐
                    │  Step Starting   │
                    │  Status: Running │
                    │  ⏳ Icon         │
                    └────────┬─────────┘
                             │
                    ┌────────┴────────┐
                    │                 │
                    ↓                 ↓
          ┌──────────────────┐   ┌──────────────────┐
          │  Execute Command │   │   AI Analysis    │
          │  (AWS/Shell/File)│   │  (No execution)  │
          └────────┬─────────┘   └────────┬─────────┘
                   │                      │
          ┌────────┴────────┐             │
          │                 │             │
          ↓                 ↓             │
┌──────────────────┐   ┌──────────────────┐
│  Command Success │   │  Command Failed  │
│  err == nil      │   │  err != nil      │
└────────┬─────────┘   └────────┬─────────┘
         │                      │
         │                      │
         │                      │ handleStepError()
         │                      │
         │              ┌───────┴────────┐
         │              │                │
         │         Fatal Error       Recoverable
         │              │                │
         │              ↓                ↓
         │     ┌──────────────────┐   ┌──────────────────┐
         │     │   Step Failed    │   │  Step Skipped    │
         │     │  Status: Failed  │   │  Status: Skipped │
         │     │  ❌ Icon         │   │  ⏭ Icon          │
         │     └────────┬─────────┘   └────────┬─────────┘
         │              │                      │
         └──────────────┴──────────────────────┘
                        │
                        │ agentStepCompleteMsg
                        │
                        ↓
               ┌──────────────────┐
               │  Step Complete   │
               │  Status: Completed│
               │  ✅ Icon         │
               │  Duration set    │
               └────────┬─────────┘
                        │
                        ↓
                  Next step or
                  iteration complete
```

---

## User Interaction Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                     USER INTERACTIONS                            │
└─────────────────────────────────────────────────────────────────┘

ERROR SCREEN (applyView with hasErrors):
  ┌────────────────────────────────────┐
  │  User Keyboard Input               │
  ├────────────────────────────────────┤
  │  'a' → Go to aiHelpView            │ (Existing - suggestions)
  │  's' → Go to aiAgentView           │ (NEW - autonomous)
  │  Enter → Go to dashboardView       │ (Return to menu)
  │  'q' → tea.Quit                    │ (Quit app)
  └────────────────────────────────────┘

AI AGENT VIEW (aiAgentView):
  ┌────────────────────────────────────┐
  │  User Keyboard Input               │
  ├────────────────────────────────────┤
  │  '↑' → Navigate up (steps list)    │
  │  '↓' → Navigate down (steps list)  │
  │  Space → Expand/collapse step      │
  │  Enter → Expand/collapse step      │
  │  'p' → Pause/resume agent          │
  │  'q' → Stop agent, back to apply   │
  │  Esc → Back to applyView           │ (if complete)
  │  '?' → Show help                   │
  └────────────────────────────────────┘

AI AGENT COMPLETE (aiAgentView with IsRunning=false):
  ┌────────────────────────────────────┐
  │  User Keyboard Input               │
  ├────────────────────────────────────┤
  │  Enter → Go to dashboardView       │ (Return to menu)
  │  'd' → Show detailed results       │
  │  'q' → tea.Quit                    │ (Quit app)
  └────────────────────────────────────┘
```

---

## Message Flow Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                     BUBBLE TEA MESSAGES                          │
└─────────────────────────────────────────────────────────────────┘

User presses 's':
  tea.KeyMsg("s")
    → Check conditions (error state, AI available)
    → m.currentView = aiAgentView
    → Return m.startAgent()
    ↓

m.startAgent():
  → Build AgentContext from m.applyState
  → Initialize m.agentState
  → Return agentStartMsg{}
    ↓

Update() receives agentStartMsg:
  → m.agentState.IsRunning = true
  → Return m.runAgentIteration()
    ↓

m.runAgentIteration():
  → Call Claude API (async)
  → Parse response into steps
  → For each step:
      → Send agentStepStartMsg{step}
      → Execute step (async)
      → Send agentStepCompleteMsg{stepID, output, err}
  → Check if solved
  → Send agentIterationCompleteMsg{iteration, success}
    ↓

Update() receives agentStepStartMsg:
  → Add step to m.agentState.Steps
  → Update step.Status = Running
  → Re-render view
    ↓

Update() receives agentStepCompleteMsg:
  → Update step.Status = Completed/Failed
  → Update step.Output = output
  → Update step.Duration = elapsed
  → Re-render view
    ↓

Update() receives agentIterationCompleteMsg:
  → If success:
      → Send agentCompleteMsg{success: true}
  → Else if iteration < maxIterations:
      → m.agentState.Iteration++
      → Return m.runAgentIteration() (next iteration)
  → Else:
      → Send agentCompleteMsg{success: false}
    ↓

Update() receives agentCompleteMsg:
  → m.agentState.IsRunning = false
  → m.agentState.Success = msg.success
  → m.agentState.FinalMessage = msg.message
  → Re-render view (show completion screen)
    ↓

User presses Enter:
  → m.currentView = dashboardView
  → Return to main menu
```

---

## Error Flow Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                     ERROR HANDLING FLOW                          │
└─────────────────────────────────────────────────────────────────┘

Terraform Apply Fails:
  └─→ parseTerraformOutput() detects errors
      └─→ handleDiagnostic() stores error details
          └─→ m.applyState.diagnostics[resource] = diagnostic
          └─→ m.applyState.hasErrors = true
          └─→ m.applyState.errorCount++
      └─→ handleApplyError() marks resource failed
          └─→ m.applyState.completed += completedResource{Success: false}
      └─→ applyCompleteMsg{success: false}
          └─→ m.applyState.applyComplete = true
          └─→ View shows error screen
              ├─→ Option 'a': Ask AI (suggestions)
              └─→ Option 's': Solve with AI (agent) ← NEW

User selects 's':
  └─→ buildAgentContext() extracts:
      ├─→ ErrorMessages from m.applyState.logs
      ├─→ Diagnostics from m.applyState.diagnostics
      ├─→ FailedResources from m.applyState.completed
      ├─→ Environment from working directory
      ├─→ AWS credentials from environment
      └─→ Terraform state from disk
  └─→ Agent analyzes context
      └─→ Identifies root cause
      └─→ Plans fix steps
      └─→ Executes fix
      └─→ Retries terraform apply
          ├─→ Success: Problem solved!
          └─→ Failure: Try different approach or give up
```

---

## Context Preservation Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                  CONTEXT PRESERVATION                            │
└─────────────────────────────────────────────────────────────────┘

Terraform Apply Phase:
  m.applyState = {
    logs: []logEntry{...},                    ← Full terraform output
    diagnostics: map[string]*DiagnosticInfo{  ← Error details
      "aws_s3_bucket.example": {...}
    },
    completed: []completedResource{           ← Resource results
      {Address: "aws_s3_bucket.example", Success: false, Error: "..."}
    },
    ...
  }

  Environment Variables:
    AWS_PROFILE=dev-terraform
    AWS_REGION=us-east-1
    ANTHROPIC_API_KEY=sk-...

  Working Directory:
    /Users/jack/project/env/dev

User presses 's' → All context flows to agent:

  AgentContext = {
    ErrorMessages:   extractFromLogs(m.applyState.logs),
    Diagnostics:     m.applyState.diagnostics,      ← Direct copy!
    FailedResources: filterFailed(m.applyState.completed),
    Environment:     detectFromPath(pwd),           ← "dev"
    AWSProfile:      os.Getenv("AWS_PROFILE"),      ← "dev-terraform"
    AWSRegion:       detectFromTerraform(),         ← "us-east-1"
    AccountID:       m.yamlConfig.AccountID,        ← "123456789012"
    TerraformState:  runCmd("terraform show"),
    ApplyLogs:       m.applyState.logs,             ← Direct copy!
  }

Result: ZERO context lost, ZERO re-parsing needed!
```

---

## Visual View Transitions

```
┌─────────────────────────────────────────────────────────────────┐
│                    VIEW TRANSITIONS                              │
└─────────────────────────────────────────────────────────────────┘

Normal Apply Flow:
  dashboardView → applyView (running) → applyView (success) → dashboardView

Error Flow (Ask AI):
  dashboardView → applyView (running) → applyView (error)
    → User presses 'a'
    → aiHelpView (suggestions)
    → User presses Esc
    → applyView (error)
    → User presses Enter
    → dashboardView

Error Flow (Solve with AI) - NEW:
  dashboardView → applyView (running) → applyView (error)
    → User presses 's'
    → aiAgentView (loading)
    → aiAgentView (running iteration 1)
    → aiAgentView (running iteration 2)
    → ...
    → aiAgentView (complete - success/failure)
    → User presses Enter
    → dashboardView

Key Differences:
  - aiHelpView: Shows suggestions, returns to applyView
  - aiAgentView: Executes autonomously, returns to dashboardView
```

---

## Summary: Critical State Transitions

| From State | Event | To State | Action |
|------------|-------|----------|--------|
| `applyView` (error) | Press 's' | `aiAgentView` (loading) | Start agent |
| `aiAgentView` (loading) | Agent ready | `aiAgentView` (running) | Begin iteration 1 |
| `aiAgentView` (running) | Press 'p' | `aiAgentView` (paused) | Pause execution |
| `aiAgentView` (paused) | Press 'p' | `aiAgentView` (running) | Resume execution |
| `aiAgentView` (running) | Problem solved | `aiAgentView` (complete-success) | Show success |
| `aiAgentView` (running) | Max iterations | `aiAgentView` (complete-failure) | Show failure |
| `aiAgentView` (running) | Press 'q' | `applyView` (error) | Stop agent |
| `aiAgentView` (complete) | Press Enter | `dashboardView` | Return to menu |

This state machine ensures a **smooth, predictable user experience** with clear entry and exit points.
