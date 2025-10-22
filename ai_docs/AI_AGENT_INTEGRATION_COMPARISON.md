# AI Agent Integration - Before vs After Comparison

## Visual Flow Comparison

### ❌ OLD APPROACH (Separate Menu Item)

```
Main Menu
├─ Plan Infrastructure
├─ Apply Infrastructure
├─ DNS Setup
├─ AI Troubleshoot ← Separate menu item
└─ Exit

User flow:
1. Select "Apply Infrastructure"
2. Terraform fails
3. Return to main menu
4. Select "AI Troubleshoot" ← Extra step, context lost
5. Agent runs
6. Return to main menu
7. Manually retry apply
```

**Problems:**
- Context is lost between apply and troubleshooting
- User must manually navigate back
- Feels disconnected and bolted-on
- No seamless integration with error flow

---

### ✅ NEW APPROACH (Integrated into Error Flow)

```
Main Menu
├─ Plan Infrastructure
├─ Apply Infrastructure
└─ DNS Setup

User flow:
1. Select "Apply Infrastructure"
2. Terraform fails → Error screen immediately shows:
   ┌──────────────────────────────────────┐
   │ ❌ Apply Failed                      │
   ├──────────────────────────────────────┤
   │ Error: aws_s3_bucket.example         │
   │ AccessDenied: User is not authorized │
   ├──────────────────────────────────────┤
   │ [a] Ask AI (suggestions only)        │
   │ [s] Solve with AI (auto-fix) ← NEW  │
   │ [Enter] Return to menu               │
   └──────────────────────────────────────┘
3. Press 's' → Seamless transition to agent
4. Agent runs → Fixes issue → Returns to menu
```

**Benefits:**
- Context preserved (errors, environment, state)
- Immediate access from error screen
- Natural workflow integration
- Two clear options: manual (ask) vs automatic (solve)

---

## Detailed Screen Flow

### Current Flow (With Existing "Ask AI")

```
┌─────────────────────────────────────────────┐
│ Apply View - Running                        │
│                                             │
│ ⏳ Applying terraform changes...            │
│                                             │
│ [Resources updating...]                     │
└─────────────────────────────────────────────┘
                    ↓ Error occurs
┌─────────────────────────────────────────────┐
│ Apply View - Failed                         │
│                                             │
│ ❌ 3 errors detected                        │
│                                             │
│ Completed Resources:                        │
│   ❌ aws_s3_bucket.example (failed)         │
│   ✅ aws_vpc.main (success)                 │
│                                             │
│ Logs:                                       │
│   Error: AccessDenied on S3 CreateBucket    │
│                                             │
│ [a] AI Help • [Enter] Continue • [q] Quit   │
└─────────────────────────────────────────────┘
                    ↓ User presses 'a'
┌─────────────────────────────────────────────┐
│ 🤖 AI Error Help (Suggestions Only)         │
├─────────────────────────────────────────────┤
│ ❌ Original Error                           │
│ ─────────────────────────────────────       │
│ Error: AccessDenied: User is not authorized │
│ to perform: s3:CreateBucket                 │
│                                             │
│ 🔍 Root Cause Analysis                      │
│ ─────────────────────────────────────       │
│ The IAM user/role lacks s3:CreateBucket     │
│ permission. This is a policy issue.         │
│                                             │
│ 📋 Suggested Fix                            │
│ ─────────────────────────────────────       │
│ 1. Run: aws iam get-user-policy             │
│ 2. Add s3:CreateBucket to policy            │
│ 3. Run: aws iam put-user-policy             │
│ 4. Retry terraform apply                    │
│                                             │
│ ⚠️  AI-generated suggestions - review first │
│                                             │
│ [Esc] Back • [q] Quit                       │
└─────────────────────────────────────────────┘
                    ↓ User manually executes
                    ↓ User returns to menu
                    ↓ User selects "Apply" again
```

**This is the CURRENT flow** - AI suggests, but doesn't execute.

---

### New Flow (With Integrated "Solve with AI")

```
┌─────────────────────────────────────────────┐
│ Apply View - Running                        │
│                                             │
│ ⏳ Applying terraform changes...            │
│                                             │
│ [Resources updating...]                     │
└─────────────────────────────────────────────┘
                    ↓ Error occurs
┌─────────────────────────────────────────────┐
│ Apply View - Failed                         │
│                                             │
│ ❌ 3 errors detected                        │
│                                             │
│ Completed Resources:                        │
│   ❌ aws_s3_bucket.example (failed)         │
│   ✅ aws_vpc.main (success)                 │
│                                             │
│ Logs:                                       │
│   Error: AccessDenied on S3 CreateBucket    │
│                                             │
│ [a] Ask AI • [s] Solve with AI • [Enter]    │
└─────────────────────────────────────────────┘
                    ↓ User presses 's' (NEW!)
┌─────────────────────────────────────────────┐
│ 🤖 AI Agent - Autonomous Troubleshooting    │
│ Environment: dev | Iteration: 1/5 | Running │
├─────────────────────────────────────────────┤
│ Troubleshooting Steps                       │
├─────────────────────────────────────────────┤
│ ✅ 1. Analyze Error (0.5s)                  │
│     └─ IAM policy missing s3:CreateBucket   │
│                                             │
│ ⏳ 2. Check IAM Policy (Running...)         │
│     $ aws iam get-user-policy               │
│     └─ [Output streaming...]                │
│                                             │
│ ⏸  3. Update IAM Policy (Pending)           │
│                                             │
│ ⏸  4. Retry Terraform Apply (Pending)       │
│                                             │
├─────────────────────────────────────────────┤
│ [↑↓] Navigate  [Space] Expand  [p] Pause    │
│ [q] Stop  [?] Help                          │
└─────────────────────────────────────────────┘
                    ↓ Agent continues...
┌─────────────────────────────────────────────┐
│ 🤖 AI Agent - Autonomous Troubleshooting    │
│ Environment: dev | Iteration: 1/5 | Running │
├─────────────────────────────────────────────┤
│ Troubleshooting Steps                       │
├─────────────────────────────────────────────┤
│ ✅ 1. Analyze Error (0.5s)                  │
│     └─ IAM policy missing s3:CreateBucket   │
│                                             │
│ ✅ 2. Check IAM Policy (1.2s)               │
│     $ aws iam get-user-policy               │
│     └─ Found missing permissions            │
│                                             │
│ ✅ 3. Update IAM Policy (0.8s)              │
│     $ aws iam put-user-policy               │
│     └─ Policy updated successfully          │
│                                             │
│ ⏳ 4. Retry Terraform Apply (Running...)    │
│     $ terraform apply -auto-approve         │
│     └─ [Output streaming...]                │
│                                             │
├─────────────────────────────────────────────┤
│ [↑↓] Navigate  [Space] Expand  [p] Pause    │
│ [q] Stop  [?] Help                          │
└─────────────────────────────────────────────┘
                    ↓ Apply succeeds!
┌─────────────────────────────────────────────┐
│ 🤖 AI Agent - Autonomous Troubleshooting    │
│ Environment: dev | Status: ✅ SUCCESS       │
├─────────────────────────────────────────────┤
│ Troubleshooting Steps                       │
├─────────────────────────────────────────────┤
│ ✅ 1. Analyze Error (0.5s)                  │
│ ✅ 2. Check IAM Policy (1.2s)               │
│ ✅ 3. Update IAM Policy (0.8s)              │
│ ✅ 4. Retry Terraform Apply (12.3s)         │
│     Apply complete! Resources: 1 added      │
│                                             │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  │
│                                             │
│ ✅ Successfully resolved the issue!         │
│ Total time: 14.8s                           │
│                                             │
│ Changes made:                               │
│ • Updated IAM policy for terraform-dev user │
│ • Added s3:CreateBucket permission          │
│ • Successfully deployed S3 bucket           │
│                                             │
├─────────────────────────────────────────────┤
│ [Enter] Return to menu  [d] View Details    │
└─────────────────────────────────────────────┘
                    ↓ User presses Enter
┌─────────────────────────────────────────────┐
│ Main Menu                                   │
│                                             │
│ > Plan Infrastructure                       │
│   Apply Infrastructure                      │
│   DNS Setup                                 │
│   Exit                                      │
│                                             │
│ Last action: Infrastructure applied (auto)  │
└─────────────────────────────────────────────┘
```

**This is the NEW flow** - Agent executes commands autonomously!

---

## Key Differences Summary

| Aspect | OLD (Separate Menu) | NEW (Integrated) |
|--------|-------------------|------------------|
| **Access Point** | Main menu item | Error screen option |
| **Context** | Lost (user navigates away) | Preserved (direct from error) |
| **User Steps** | 7+ steps | 2 steps (error → solve) |
| **Execution** | Manual (suggestions only) | Autonomous (auto-executes) |
| **Integration** | Separate feature | Native to error flow |
| **Transition** | Jump between views | Smooth screen replace |
| **Return Path** | Back to menu → reselect apply | Direct to menu (fixed) |
| **Options** | Only one AI option | Two options: ask vs solve |
| **Feel** | Bolted-on, separate | Native, integrated |

---

## Code Integration Comparison

### OLD Approach (Separate Menu Item)

```go
// In main.go or menu.go
func showMainMenu() {
    options := []string{
        "Plan Infrastructure",
        "Apply Infrastructure",
        "DNS Setup",
        "AI Troubleshoot", // ← Separate item
        "Exit",
    }
    // Handle selection...
}

// Completely separate flow
func runAITroubleshoot() {
    // User must manually provide error context
    // No access to terraform state
    // Disconnected from apply flow
}
```

**Problems:**
- No access to `m.applyState`
- No access to `m.applyState.diagnostics`
- User must re-describe the error
- Can't automatically detect environment

---

### NEW Approach (Integrated)

```go
// In terraform_plan_modern_tui.go
func (m *modernPlanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        // ... existing code ...

        // NEW: Handle 's' key in error state
        case msg.String() == "s":
            if m.currentView == applyView &&
               m.applyState != nil &&
               m.applyState.applyComplete &&
               m.applyState.hasErrors &&
               isAIHelperAvailable() {

                // Direct access to ALL context!
                m.currentView = aiAgentView
                return m, m.startAgent() // ← Passes full context
            }
    }
}

// Agent has FULL access to error context
func (m *modernPlanModel) startAgent() tea.Cmd {
    return func() tea.Msg {
        ctx := AgentContext{
            // Direct from applyState - no re-parsing!
            ErrorMessages:   extractFromLogs(m.applyState.logs),
            Diagnostics:     m.applyState.diagnostics,
            FailedResources: getFailedFromCompleted(m.applyState.completed),
            Environment:     detectFromWorkingDir(),
            AWSProfile:      os.Getenv("AWS_PROFILE"),
            // ... full context available
        }
        // ... start agent with context
    }
}
```

**Benefits:**
- Direct access to all error state
- No need to re-parse or re-collect
- AWS credentials already set up
- Environment already detected
- Seamless integration

---

## Footer Comparison

### Current Footer (Error State)

```
┌────────────────────────────────────────────────┐
│ [a] AI Help • [Enter] Continue • [q] Quit      │
└────────────────────────────────────────────────┘
```

Only one AI option - shows suggestions.

---

### NEW Footer (Error State)

```
┌───────────────────────────────────────────────────────────┐
│ [a] Ask AI (suggestions) • [s] Solve with AI (auto-fix)  │
│ [Enter] Continue • [q] Quit                               │
└───────────────────────────────────────────────────────────┘
```

**Two distinct options:**
- `[a]` Ask AI: Get suggestions only (existing behavior)
- `[s]` Solve with AI: Autonomous agent fixes it (new!)

Clear distinction between manual and automatic modes.

---

## Implementation File Changes

### OLD Approach
```
New Files:
- app/ai_agent_menu.go      (new menu item)
- app/ai_agent_standalone.go (separate flow)

Modified Files:
- app/main.go or menu.go    (add menu item)

Integration: Minimal, separate feature
```

---

### NEW Approach
```
New Files:
- app/ai_agent_types.go     (shared types)
- app/ai_agent_tui.go       (TUI rendering)
- app/ai_agent_executor.go  (execution engine)
- app/ai_agent_claude.go    (Claude API)
- app/ai_agent_tools.go     (tool execution)
- app/ai_agent_context.go   (context building)

Modified Files:
- app/terraform_plan_modern_tui.go (integrate into Update/View)
- app/terraform_apply_tui.go       (add agentState field)

Integration: Deep, native integration with error flow
```

---

## User Experience Comparison

### OLD: User has error and wants help

1. Run `make infra-apply env=dev`
2. Terraform fails with error
3. Exit back to shell
4. Run `./meroku` again
5. Navigate to "AI Troubleshoot" menu item
6. Enter error manually or hope it auto-detects
7. Wait for suggestions
8. Manually execute suggested commands
9. Exit meroku
10. Run `make infra-apply env=dev` again
11. Hope it works

**Total: 11 steps, lots of context switching**

---

### NEW: User has error and wants help

1. Run `make infra-apply env=dev`
2. Terraform fails with error → Press `s`
3. Agent autonomously fixes it
4. Done!

**Total: 3 steps, fully automated**

---

## Visual State Transition Diagram

### OLD (Separate Menu)
```
┌──────────────┐
│  Main Menu   │
└──────┬───────┘
       │ Select "Apply"
       ↓
┌──────────────┐
│ Apply Running│
└──────┬───────┘
       │ Error!
       ↓
┌──────────────┐
│ Apply Failed │
│ [Enter]      │
└──────┬───────┘
       │ Return
       ↓
┌──────────────┐
│  Main Menu   │ ← Back at menu
└──────┬───────┘
       │ Select "AI Troubleshoot"
       ↓
┌──────────────┐
│ Enter Error  │ ← Manual context entry
└──────┬───────┘
       │
       ↓
┌──────────────┐
│ AI Suggests  │ ← Only suggestions
└──────┬───────┘
       │ [Enter]
       ↓
┌──────────────┐
│  Main Menu   │ ← Back at menu again
└──────┬───────┘
       │ Select "Apply"
       ↓
┌──────────────┐
│ Apply Again  │ ← Manual retry
└──────────────┘
```

**Disconnected flow, lots of back-and-forth**

---

### NEW (Integrated)
```
┌──────────────┐
│  Main Menu   │
└──────┬───────┘
       │ Select "Apply"
       ↓
┌──────────────┐
│ Apply Running│
└──────┬───────┘
       │ Error!
       ↓
┌──────────────┐
│ Apply Failed │
│ [s] Solve    │ ← NEW option
└──────┬───────┘
       │ Press 's'
       ↓
┌──────────────┐
│  AI Agent    │ ← Direct transition
│   Running    │ ← Full context preserved
└──────┬───────┘
       │ Auto-executes
       │ Auto-fixes
       │ Auto-retries
       ↓
┌──────────────┐
│  AI Agent    │
│  ✅ Success  │
└──────┬───────┘
       │ [Enter]
       ↓
┌──────────────┐
│  Main Menu   │ ← Done!
└──────────────┘
```

**Seamless flow, fully automated**

---

## Context Preservation Example

### OLD: Context Lost

```
Error happens:
  - Error: AccessDenied on S3 CreateBucket
  - AWS_PROFILE=dev-terraform
  - Region: us-east-1
  - Account: 123456789012
  - Working directory: /Users/jack/project/env/dev

User returns to menu → ALL CONTEXT LOST

AI Troubleshoot menu:
  - No idea what the error was
  - No idea which environment
  - No idea which AWS profile
  - No idea which account
  - User must re-enter or script must re-detect
```

**Requires re-discovery or manual input**

---

### NEW: Context Preserved

```
Error happens in applyState:
  m.applyState.logs = [full terraform output]
  m.applyState.diagnostics = {
      "aws_s3_bucket.example": {
          Summary: "AccessDenied",
          Detail: "User is not authorized to perform: s3:CreateBucket"
      }
  }
  m.applyState.completed = [
      {Address: "aws_s3_bucket.example", Success: false, Error: "..."}
  ]
  os.Getenv("AWS_PROFILE") = "dev-terraform"
  Current directory = "/Users/jack/project/env/dev"

User presses 's' → ALL CONTEXT PASSED DIRECTLY

Agent receives:
  ctx := AgentContext{
      ErrorMessages:   ["AccessDenied: User is not authorized..."],
      Diagnostics:     m.applyState.diagnostics,  // Direct copy!
      FailedResources: [...],                     // Direct copy!
      Environment:     "dev",                     // Detected from path
      AWSProfile:      "dev-terraform",           // From env
      WorkingDir:      "/Users/jack/project/env/dev",
      // ... everything available immediately
  }
```

**Zero re-discovery needed, instant context**

---

## Summary: Why Integration is Better

### Problems with Separate Menu Item:
1. ❌ Context lost when returning to menu
2. ❌ Extra navigation steps
3. ❌ Feels disconnected from error
4. ❌ User must manually re-run apply
5. ❌ Doesn't leverage existing error state
6. ❌ Feels like a separate tool, not part of workflow

### Benefits of Integrated Approach:
1. ✅ Context preserved directly from error
2. ✅ Immediate access (one key press)
3. ✅ Feels native to the error flow
4. ✅ Agent can auto-retry after fixing
5. ✅ Leverages all existing state
6. ✅ Feels like a natural part of meroku

---

## Final Recommendation

**Use the INTEGRATED approach** (`[s]` key on error screen) because:

1. **User Experience:** Seamless, natural, fast
2. **Context:** Direct access to all error state
3. **Integration:** Native to meroku's workflow
4. **Simplicity:** Fewer steps, less confusion
5. **Professional:** Feels like a cohesive product, not a bolted-on feature

The integrated approach makes the AI agent feel like a **native feature of meroku**, not a separate tool that happens to use AI.
