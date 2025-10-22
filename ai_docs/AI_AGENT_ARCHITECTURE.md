# AI Agent Architecture

## Overview

The AI Agent is an autonomous infrastructure troubleshooting system that uses the **ReAct pattern** (Reasoning + Acting) to analyze and fix deployment errors. Unlike traditional error helpers that provide static suggestions, this agent thinks iteratively, observes results, and adapts its strategy in real-time.

## Architecture Pattern: ReAct Loop

### What is ReAct?

ReAct stands for **Reasoning + Acting**. It's an AI agent pattern where the LLM:
1. **Thinks** about what to do next based on observations
2. **Acts** by executing a tool/command
3. **Observes** the result
4. **Repeats** until the problem is solved

This creates an adaptive, intelligent debugging process rather than a pre-planned script.

### Why ReAct vs Pre-Planning?

**Wrong Approach (Pre-Planning):**
```
Agent: "I'll create a 5-step plan to fix this"
  1. Check ECS service
  2. Get task events
  3. Force deployment
  4. Wait for stabilization
  5. Verify health

Problem: What if step 1 reveals the IAM role is missing?
The pre-planned steps become irrelevant!
```

**Right Approach (ReAct):**
```
Iteration 1:
  THOUGHT: "Need to check ECS service status"
  ACTION: aws_cli
  COMMAND: aws ecs describe-services...
  OBSERVATION: "0 running tasks"

Iteration 2:
  THOUGHT: "Why no tasks? Check task definition"
  ACTION: aws_cli
  COMMAND: aws ecs describe-task-definition...
  OBSERVATION: "Invalid IAM role ARN"

Iteration 3:
  THOUGHT: "IAM role missing, need to add it to terraform"
  ACTION: file_edit
  COMMAND: FILE:env/dev/main.tf|OLD:...|NEW:...
  OBSERVATION: "File updated successfully"

Iteration 4:
  THOUGHT: "Apply terraform to create the role"
  ACTION: terraform_apply
  COMMAND: terraform apply
  OBSERVATION: "Apply complete, role created"

Iteration 5:
  THOUGHT: "Service should work now, verify"
  ACTION: aws_cli
  COMMAND: aws ecs describe-services...
  OBSERVATION: "1 running task, service healthy"

Iteration 6:
  THOUGHT: "Problem solved!"
  ACTION: complete
  COMMAND: Success
```

Each step is **dynamically chosen** based on what the agent learned from the previous step.

## System Components

### 1. Core ReAct Loop (`ai_agent_react_loop.go`)

**Responsibilities:**
- Orchestrates the think → act → observe cycle
- Manages iteration history and context
- Enforces iteration limits (prevents infinite loops)
- Sends updates to the TUI in real-time

**Key Types:**
```go
type AgentIteration struct {
    Number      int           // Iteration sequence number
    Thought     string        // LLM's reasoning
    Action      string        // Tool to use
    Command     string        // Exact command
    Output      string        // Result of execution
    Status      string        // running, success, failed
    Duration    time.Duration // How long it took
    ErrorDetail string        // Error if failed
}

type AgentState struct {
    Iterations      []AgentIteration
    CurrentThinking bool
    IsComplete      bool
    FinalOutcome    string // "success", "failed", "cancelled"
    IterationLimit  int    // Max 20 iterations by default
    Context         *AgentContext
}
```

**Main Loop:**
```go
for iteration := 1; iteration <= maxIterations; iteration++ {
    // 1. Think - LLM decides next action
    thought, action, command := agent.think()

    // 2. Check if complete
    if action == "complete" {
        break
    }

    // 3. Act - Execute the action
    output, err := agent.act(action, command)

    // 4. Observe - Record result
    iteration := AgentIteration{
        Thought: thought,
        Action:  action,
        Output:  output,
        // ...
    }

    // 5. Add to history for next iteration
    agent.state.Iterations = append(agent.state.Iterations, iteration)

    // 6. Send update to TUI
    agent.sendUpdate(AgentUpdate{...})
}
```

### 2. Tool Executor (`ai_agent_executor.go`)

**Responsibilities:**
- Executes different action types
- Manages AWS environment variables
- Handles command timeouts
- Provides error details

**Supported Tools:**

1. **aws_cli** - Run AWS CLI commands
   - Automatically sets AWS_PROFILE and AWS_REGION
   - Example: `aws ecs describe-services --cluster dev_cluster --services dev_service`

2. **shell** - Run arbitrary shell commands
   - Example: `grep -r "iam_role" env/dev/`

3. **file_edit** - Edit configuration files
   - Format: `FILE:path|OLD:old_text|NEW:new_text`
   - Example: `FILE:env/dev/main.tf|OLD:task_role_arn = ""|NEW:task_role_arn = aws_iam_role.ecs_task_execution_role.arn`

4. **terraform_plan** - Preview infrastructure changes
   - Runs in the appropriate env directory
   - Returns plan output for review

5. **terraform_apply** - Apply infrastructure changes
   - Automatically adds `-auto-approve` flag
   - Timeout: 15 minutes
   - Use carefully!

6. **complete** - Mark problem as solved
   - Ends the agent loop
   - Example: `Success: ECS service is healthy`

**Execution Flow:**
```go
func (e *AgentExecutor) ExecuteAWSCLI(ctx context.Context, command string) (string, error) {
    // Set AWS environment
    env := append(os.Environ(),
        "AWS_PROFILE=" + e.context.AWSProfile,
        "AWS_REGION=" + e.context.AWSRegion,
    )

    // Execute with timeout
    cmd := exec.CommandContext(ctx, "sh", "-c", command)
    cmd.Env = env

    // Capture output
    output, err := cmd.CombinedOutput()
    return string(output), err
}
```

### 3. LLM Integration (`ai_agent_claude.go`)

**Responsibilities:**
- Communicates with Claude API
- Constructs ReAct-style prompts
- Parses LLM responses into structured actions

**Prompt Structure:**
```
System: You are an autonomous AWS infrastructure troubleshooting agent.

CONTEXT:
- Operation: terraform_apply
- Environment: dev
- AWS Profile: alpha-dev
- AWS Region: ap-southeast-2

INITIAL ERROR:
[Terraform error message]

PREVIOUS ACTIONS:
[History of what the agent has already tried]

AVAILABLE TOOLS:
1. aws_cli - Run AWS CLI commands
2. shell - Run shell commands
3. file_edit - Edit files
4. terraform_plan - Preview changes
5. terraform_apply - Apply changes
6. complete - Mark as solved

TASK:
Decide on ONE action to take next.

Respond in this format:
THOUGHT: [Your reasoning]
ACTION: [Tool name]
COMMAND: [Exact command]
```

**Response Parsing:**
The LLM response is parsed to extract:
- **THOUGHT**: The agent's reasoning (shown to user)
- **ACTION**: Which tool to use (validated against allowed tools)
- **COMMAND**: The exact command to execute

**Example Response:**
```
THOUGHT: The ECS service has 0 running tasks. I should check recent task failures to see why tasks aren't starting successfully.
ACTION: aws_cli
COMMAND: aws ecs describe-tasks --cluster dev_cluster --tasks $(aws ecs list-tasks --cluster dev_cluster --service dev_service --desired-status STOPPED --region ap-southeast-2 --query 'taskArns[0]' --output text) --region ap-southeast-2
```

### 4. Bubble Tea TUI (`ai_agent_tui.go`)

**Responsibilities:**
- Renders agent state in real-time
- Shows iterations as they're added (not pre-planned!)
- Displays thinking status and progress
- Allows user to stop the agent

**UI States:**

1. **Thinking State:**
```
┌─────────────────────────────────────────┐
│ 🤖 AI Agent - Autonomous Troubleshooter │
│                                         │
│ 💭 Analyzing situation...               │
│                                         │
│ Iterations: 3 | Success: 2 | Failed: 1  │
│                                         │
│ ✓ 1. Checked ECS service status         │
│ ✓ 2. Examined task definition           │
│ ✓ 3. Identified missing IAM role        │
│ → 4. Thinking about next step...        │
│                                         │
│ [q] Stop agent                          │
└─────────────────────────────────────────┘
```

2. **Action Running State:**
```
┌─────────────────────────────────────────┐
│ 🤖 AI Agent - Autonomous Troubleshooter │
│                                         │
│ 🔧 Executing: terraform apply           │
│                                         │
│ Iterations: 4 | Success: 3 | Failed: 0  │
│                                         │
│ ✓ 1. Checked ECS service - 0 tasks      │
│ ✓ 2. Found task definition errors       │
│ ✓ 3. Updated terraform config           │
│ → 4. Applying terraform... (running)    │
│                                         │
│ [q] Stop agent                          │
└─────────────────────────────────────────┘
```

3. **Complete State:**
```
┌─────────────────────────────────────────┐
│ 🤖 AI Agent - Autonomous Troubleshooter │
│                                         │
│ ✅ Problem resolved successfully!       │
│                                         │
│ Iterations: 7 | Success: 7 | Failed: 0  │
│                                         │
│ ✓ 1. Checked ECS service - 0 tasks      │
│ ✓ 2. Found invalid IAM role             │
│ ✓ 3. Confirmed IAM role missing         │
│ ✓ 4. Updated terraform config           │
│ ✓ 5. Applied terraform - role created   │
│ ✓ 6. Forced new deployment              │
│ ✓ 7. Verified service healthy           │
│                                         │
│ [q] Quit and return to menu             │
└─────────────────────────────────────────┘
```

**Key Features:**
- **Dynamic Updates**: Steps appear as the agent creates them, not upfront
- **Real-time Status**: Shows whether agent is thinking or acting
- **Scrollable History**: Can review all iterations
- **Interrupt Support**: Press 'q' to stop the agent at any time

**Update Messages:**
```go
type AgentUpdate struct {
    Type       string          // "thinking", "action_start", "action_complete", "finished"
    Iteration  *AgentIteration // The iteration being updated
    Message    string          // Human-readable status
    IsComplete bool
    Success    bool
}
```

### 5. Integration Layer (`ai_agent_integration.go`)

**Responsibilities:**
- Bridges the agent with existing infrastructure code
- Converts `ErrorContext` to `AgentContext`
- Provides menu integration
- Handles user prompts

**Entry Points:**

1. **From Terraform Errors:**
```go
// When terraform apply/destroy fails
ctx := ErrorContext{
    Operation: "apply",
    Environment: "dev",
    Errors: []string{"ECS service deployment failed"},
}
offerAIAgentHelp(ctx)
```

2. **From Main Menu:**
```go
// User selects "AI Agent - Troubleshoot Issues"
offerAIAgentFromMenu()
// Prompts for problem description
// Runs agent with user-provided context
```

## Data Flow

```
User Action (terraform apply fails)
    ↓
ErrorContext created with error details
    ↓
offerAIAgentHelp() prompts user
    ↓
AgentContext created
    ↓
AIAgent initialized with LLM client & executor
    ↓
┌─────────────────────────────────────────┐
│         ReAct Loop Begins               │
│                                         │
│  1. LLM thinks → generates THOUGHT,     │
│     ACTION, COMMAND                     │
│         ↓                               │
│  2. Executor runs command               │
│         ↓                               │
│  3. Output captured as OBSERVATION      │
│         ↓                               │
│  4. AgentIteration created              │
│         ↓                               │
│  5. Update sent to TUI                  │
│         ↓                               │
│  6. History updated for next iteration  │
│         ↓                               │
│  7. Loop continues until complete       │
│     or max iterations reached           │
└─────────────────────────────────────────┘
    ↓
Final result shown in TUI
    ↓
User returns to main menu
```

## Context Accumulation

Each iteration adds to the agent's knowledge:

**Iteration 1:**
```
Context: Initial error only
History: None
```

**Iteration 2:**
```
Context: Initial error + Iteration 1 result
History: What agent tried, what it learned
```

**Iteration N:**
```
Context: Initial error + All previous iterations
History: Full debugging journey so far
```

This allows the agent to:
- Learn from failures
- Avoid repeating unsuccessful actions
- Build on previous discoveries
- Converge toward a solution

## Safety Mechanisms

1. **Iteration Limit**: Max 20 iterations prevents infinite loops
2. **Timeouts**: Each command has a timeout (2-15 minutes depending on type)
3. **User Cancellation**: Press 'q' to stop at any time
4. **Failed Actions Don't Stop Agent**: Agent sees failure and adapts
5. **Terraform Auto-Approve**: Controlled by executor, not LLM

## Benefits Over Traditional Error Helpers

| Traditional Helper | AI Agent (ReAct) |
|-------------------|------------------|
| Shows one suggestion | Investigates systematically |
| Static commands | Adapts to observations |
| User must execute | Autonomous execution |
| No follow-up | Verifies fixes work |
| Limited context | Full conversation history |
| Can't recover from failures | Learns and adjusts |

## Example Agent Execution

**Problem**: ECS service deployment failing

**Agent's Journey:**

```
[00:00] Started agent
[00:02] Iteration 1: Check ECS service status
        → Output: Service has 0 running tasks
[00:05] Iteration 2: Check recent task failures
        → Output: Tasks failing with "CannotPullContainerError"
[00:08] Iteration 3: Examine task definition
        → Output: Task execution role ARN is empty
[00:10] Iteration 4: Search for IAM role in terraform
        → Output: No ECS task execution role defined
[00:12] Iteration 5: Add IAM role to terraform config
        → Output: File updated successfully
[00:15] Iteration 6: Apply terraform changes
        → Output: Role created, apply successful
[00:20] Iteration 7: Force new ECS deployment
        → Output: New deployment started
[00:25] Iteration 8: Wait and check service status
        → Output: Service stable, 1 task running
[00:27] Iteration 9: Problem solved!
        → Action: complete
```

The agent **discovered** the root cause (missing IAM role) by investigating systematically, rather than being told what to do.

## Integration Points

### 1. Terraform Apply Errors
Located in: `terraform_plan_modern_tui.go`

After apply fails, user is prompted:
```go
if isAIHelperAvailable() {
    offerAIAgentHelp(ErrorContext{
        Operation: "apply",
        Environment: env,
        Errors: errorMessages,
    })
}
```

### 2. Terraform Destroy Errors
Located in: `terraform_destroy_progress_tui.go`

Similar to apply errors.

### 3. Main Menu
Located in: `main_menu.go`

User can proactively run agent:
```
🤖 AI Agent - Troubleshoot Issues
```

## Future Enhancements

1. **Multi-Environment Awareness**: Agent can check related environments
2. **Cost Estimation**: Before applying, estimate cost impact
3. **Rollback Capability**: Undo changes if they don't work
4. **Learning Mode**: Save successful patterns for future use
5. **Parallel Actions**: Run multiple investigations simultaneously
6. **Visual Diffing**: Show before/after for file edits in TUI

## Debugging the Agent

**Enable Debug Logging:**
```go
// In ai_agent_react_loop.go
fmt.Printf("[DEBUG] LLM Response: %s\n", response)
fmt.Printf("[DEBUG] Parsed - Thought: %s, Action: %s\n", thought, action)
```

**Inspect Agent State:**
```go
// After agent completes
state := agent.GetState()
for _, iter := range state.Iterations {
    fmt.Printf("Iteration %d: %s → %s\n", iter.Number, iter.Action, iter.Status)
}
```

**Test Individual Tools:**
```go
executor := NewAgentExecutor(agentContext)
output, err := executor.ExecuteAWSCLI(ctx, "aws sts get-caller-identity")
fmt.Println(output)
```

## Conclusion

The AI Agent represents a paradigm shift from **prescriptive automation** (telling the system what to do) to **adaptive automation** (letting the system figure out what to do). By using the ReAct pattern, the agent behaves more like a human debugging a problem: investigating, learning, adapting, and verifying.
