# Autonomous AWS Troubleshooting Agent - Implementation Plan

## Executive Summary

This document provides a comprehensive, production-ready implementation plan for an autonomous AWS troubleshooting agent integrated into the meroku CLI. The agent will autonomously diagnose and fix AWS deployment issues using a combination of AWS CLI, shell commands, web documentation, and LLM-powered reasoning.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Core Components](#core-components)
3. [Implementation Checklist](#implementation-checklist)
4. [File Structure](#file-structure)
5. [Code Specifications](#code-specifications)
6. [Testing Strategy](#testing-strategy)
7. [Integration Points](#integration-points)

---

## Architecture Overview

### High-Level Design

```
┌─────────────────────────────────────────────────────────────┐
│                    Meroku CLI Entry Point                    │
│                  (main.go, main_menu.go)                     │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│              Autonomous Agent Orchestrator                   │
│                  (agent/orchestrator.go)                     │
│  ┌───────────────────────────────────────────────────────┐  │
│  │ • Problem Analysis         • Execution Loop           │  │
│  │ • Plan Generation          • State Management         │  │
│  │ • Tool Selection           • Error Recovery           │  │
│  └───────────────────────────────────────────────────────┘  │
└────────┬──────────────┬──────────────┬─────────────┬────────┘
         │              │              │             │
         ▼              ▼              ▼             ▼
┌────────────┐  ┌──────────────┐  ┌─────────┐  ┌──────────┐
│   Tool     │  │   AWS CLI    │  │   LLM   │  │   Web    │
│  Registry  │  │   Wrapper    │  │ Client  │  │  Fetch   │
│            │  │              │  │         │  │          │
└────────────┘  └──────────────┘  └─────────┘  └──────────┘
         │              │              │             │
         └──────────────┴──────────────┴─────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                 Bubble Tea TUI Interface                     │
│              (agent/troubleshoot_tui.go)                     │
│  ┌───────────────────────────────────────────────────────┐  │
│  │ • Flow Diagram View        • Log Streaming            │  │
│  │ • Step-by-Step Progress    • Detail Expansion         │  │
│  │ • Tool Usage Display       • Interactive Controls     │  │
│  └───────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

### Agent Workflow

```
1. Problem Detection
   ├── Parse deployment errors (Terraform/ECS/CloudWatch)
   ├── Gather context (environment, AWS profile, region)
   └── Initialize agent state

2. Analysis & Planning
   ├── Send error context to LLM
   ├── Generate execution plan (5-10 steps)
   └── Identify required tools for each step

3. Execution Loop
   ├── Execute step (AWS CLI, shell, web fetch)
   ├── Capture output
   ├── Feed results to LLM for next step decision
   ├── Update TUI with progress
   └── Repeat until resolved or max iterations

4. Error Recovery
   ├── Detect failures
   ├── Re-analyze with failure context
   ├── Generate alternative approach
   └── Retry with different strategy

5. Resolution
   ├── Verify fix (health checks, deployment status)
   ├── Generate summary report
   └── Log lessons learned
```

---

## Core Components

### 1. Agent Package (`app/agent/`)

New package containing all agent-specific logic.

#### Files:
- `orchestrator.go` - Main agent orchestration logic
- `tools.go` - Tool registry and execution framework
- `llm_client.go` - LLM integration for autonomous reasoning
- `aws_wrapper.go` - AWS CLI wrapper with profile management
- `web_fetcher.go` - Documentation fetching from AWS docs
- `state.go` - Agent state management and persistence
- `planner.go` - Execution plan generation and management
- `error_parser.go` - Parse errors from various sources
- `troubleshoot_tui.go` - Bubble Tea TUI for agent visualization
- `types.go` - Shared types and interfaces

### 2. TUI Components (`app/agent/troubleshoot_tui.go`)

Beautiful Bubble Tea interface showing the agent's work in real-time.

#### Views:
1. **Flow Diagram View** - Visual representation of the troubleshooting process
2. **Step Detail View** - Expandable details for each execution step
3. **Tool Usage View** - Shows commands being executed
4. **Log Stream View** - Real-time output from tools
5. **Status Panel** - Current state, progress, errors

### 3. Tool System (`app/agent/tools.go`)

Extensible tool registry supporting multiple tool types.

#### Tool Types:
- **AWS CLI Tools** - Wrapper around AWS CLI commands
- **Shell Commands** - Execute arbitrary bash commands
- **Web Fetch** - Retrieve documentation from online sources
- **Terraform Commands** - Run terraform operations
- **Health Checks** - Verify service health

---

## Implementation Checklist

### Phase 1: Foundation (Week 1)

#### Agent Core Structure
- [ ] Create `app/agent/` package directory
- [ ] Implement `app/agent/types.go` with core types
  ```go
  type AgentState
  type ExecutionPlan
  type ExecutionStep
  type ToolResult
  type AgentConfig
  ```
- [ ] Implement `app/agent/state.go` for state management
  - [ ] State persistence to JSON
  - [ ] State restoration from file
  - [ ] State transitions (idle → analyzing → executing → recovering → resolved)
  - [ ] Thread-safe state updates

#### Tool Registry
- [ ] Implement `app/agent/tools.go` - Tool registry
  - [ ] Tool interface definition
  - [ ] Tool registration system
  - [ ] Tool execution framework
  - [ ] Result capture and formatting
- [ ] Implement `app/agent/aws_wrapper.go` - AWS CLI wrapper
  - [ ] Profile-aware AWS CLI execution
  - [ ] Common AWS commands (describe-services, get-logs, etc.)
  - [ ] Output parsing (JSON, text)
  - [ ] Error handling and retries
- [ ] Implement `app/agent/shell_tool.go` - Shell command execution
  - [ ] Safe command execution with timeout
  - [ ] Environment variable management
  - [ ] Output streaming
  - [ ] Exit code handling

#### LLM Integration
- [ ] Implement `app/agent/llm_client.go` - Claude API client
  - [ ] Extend existing Anthropic SDK usage
  - [ ] Structured prompt templates
  - [ ] Response parsing
  - [ ] Context window management
  - [ ] Rate limiting and retries

### Phase 2: Agent Logic (Week 2)

#### Error Parsing
- [ ] Implement `app/agent/error_parser.go`
  - [ ] Parse Terraform errors from apply/plan output
  - [ ] Parse ECS errors from task/service failures
  - [ ] Parse CloudWatch Logs errors
  - [ ] Parse AWS CLI errors
  - [ ] Extract actionable error details
  - [ ] Error categorization (auth, resource, quota, network, etc.)

#### Planning System
- [ ] Implement `app/agent/planner.go`
  - [ ] Generate execution plans from error context
  - [ ] Break down complex problems into steps
  - [ ] Estimate step execution time
  - [ ] Plan validation
  - [ ] Plan adjustment based on results

#### Orchestrator
- [ ] Implement `app/agent/orchestrator.go`
  - [ ] Main execution loop
  - [ ] Step execution and result handling
  - [ ] Error detection and recovery
  - [ ] Success criteria evaluation
  - [ ] Max iteration limits (default: 10)
  - [ ] Timeout handling (default: 30 minutes)

#### Web Documentation Fetcher
- [ ] Implement `app/agent/web_fetcher.go`
  - [ ] Fetch AWS documentation via HTTP
  - [ ] Parse relevant sections (HTML → text)
  - [ ] Cache fetched docs (in-memory + disk)
  - [ ] Handle rate limits
  - [ ] Integration with existing MCP Firecrawl tools

### Phase 3: TUI Interface (Week 3)

#### Base TUI Structure
- [ ] Implement `app/agent/troubleshoot_tui.go`
  - [ ] Bubble Tea model initialization
  - [ ] Message types for state updates
  - [ ] View rendering logic
  - [ ] Keyboard navigation
  - [ ] Help screen

#### Flow Diagram View
- [ ] Implement flow visualization
  - [ ] Node representation (steps)
  - [ ] Edge representation (dependencies)
  - [ ] Progress indicators (pending, running, success, error)
  - [ ] Current step highlighting
  - [ ] Expandable node details
- [ ] Layout algorithm
  - [ ] Vertical flow (top to bottom)
  - [ ] Automatic spacing
  - [ ] Scroll support for large plans

#### Step Detail Panel
- [ ] Implement detail view
  - [ ] Step description
  - [ ] Tool being used
  - [ ] Command/query being executed
  - [ ] Output preview (first 50 lines)
  - [ ] Timestamp and duration
  - [ ] Status (running, success, error)
  - [ ] Expandable full output

#### Tool Usage Display
- [ ] Implement tool visualization
  - [ ] Command syntax highlighting
  - [ ] AWS CLI command display
  - [ ] Shell command display
  - [ ] Web fetch URL display
  - [ ] Output streaming
  - [ ] Copy-to-clipboard support

#### Log Stream View
- [ ] Implement log viewer
  - [ ] Scrollable log window
  - [ ] Auto-scroll on new logs
  - [ ] Search/filter functionality
  - [ ] Export logs to file
  - [ ] Syntax highlighting for errors

#### Interactive Controls
- [ ] Implement control panel
  - [ ] Pause/Resume execution
  - [ ] Stop/Cancel execution
  - [ ] Skip current step
  - [ ] Retry failed step
  - [ ] View full plan
  - [ ] Export state

#### Status Panel
- [ ] Implement status display
  - [ ] Current state indicator
  - [ ] Progress bar (steps completed / total)
  - [ ] Time elapsed
  - [ ] Current AWS profile
  - [ ] Environment name
  - [ ] Error count
  - [ ] Success count

### Phase 4: Integration (Week 4)

#### Main Menu Integration
- [ ] Update `app/main_menu.go`
  - [ ] Add "Troubleshoot Deployment" menu option
  - [ ] Add keyboard shortcut (T)
- [ ] Update `app/cmd.go`
  - [ ] Add `troubleshoot` subcommand
  - [ ] CLI flags: `--env`, `--error-file`, `--auto-approve`

#### Terraform Integration
- [ ] Update `app/terraform_apply_tui.go`
  - [ ] Detect apply failures
  - [ ] Offer AI troubleshooting option
  - [ ] Pass error context to agent
- [ ] Update `app/terraform_plan_modern_tui.go`
  - [ ] Detect plan failures
  - [ ] Offer AI troubleshooting option
  - [ ] Pass error context to agent

#### Deployment Integration
- [ ] Update `app/deploy.go`
  - [ ] Catch deployment failures
  - [ ] Trigger agent on failure
  - [ ] Display agent progress

#### Error Context Collection
- [ ] Create `app/agent/context_builder.go`
  - [ ] Collect environment details
  - [ ] Collect AWS profile info
  - [ ] Collect recent logs (last 100 lines)
  - [ ] Collect Terraform state info
  - [ ] Collect ECS service status
  - [ ] Collect CloudWatch alarms

### Phase 5: Advanced Features (Week 5)

#### Multi-Step Planning
- [ ] Implement complex plan generation
  - [ ] Dependency tracking between steps
  - [ ] Parallel step execution where possible
  - [ ] Conditional steps (if/else logic)
  - [ ] Loop detection (retry with backoff)

#### Learning System
- [ ] Implement lesson storage
  - [ ] Store successful resolutions
  - [ ] Store failed attempts
  - [ ] Pattern recognition
  - [ ] Suggest known fixes first

#### Verification System
- [ ] Implement fix verification
  - [ ] Health check execution
  - [ ] ECS service stability check
  - [ ] Log monitoring for new errors
  - [ ] Rollback on verification failure

#### Enhanced Error Recovery
- [ ] Implement smart retry logic
  - [ ] Exponential backoff
  - [ ] Alternative approach generation
  - [ ] Escalation (manual intervention request)
  - [ ] Snapshot state before risky operations

#### Documentation Integration
- [ ] Integrate web documentation
  - [ ] AWS service documentation
  - [ ] Terraform provider docs
  - [ ] Common error solutions (Stack Overflow, GitHub issues)
  - [ ] Context-aware doc retrieval

### Phase 6: Polish & Production Readiness (Week 6)

#### Testing
- [ ] Unit tests for all components (80% coverage)
  - [ ] Tool execution tests
  - [ ] Error parser tests
  - [ ] LLM client tests (mocked)
  - [ ] State management tests
  - [ ] Planner tests
- [ ] Integration tests
  - [ ] End-to-end troubleshooting scenarios
  - [ ] Mock AWS responses
  - [ ] TUI rendering tests
- [ ] Manual testing scenarios
  - [ ] ECS service failure recovery
  - [ ] RDS connection issue resolution
  - [ ] IAM permission errors
  - [ ] Quota limit errors
  - [ ] Network connectivity issues

#### Documentation
- [ ] Create `ai_docs/AGENT_ARCHITECTURE.md`
  - [ ] Detailed architecture documentation
  - [ ] Component interaction diagrams
  - [ ] State machine diagrams
  - [ ] Tool catalog
- [ ] Create `ai_docs/AGENT_USAGE.md`
  - [ ] User guide
  - [ ] CLI command reference
  - [ ] TUI navigation guide
  - [ ] Troubleshooting tips
- [ ] Update `CLAUDE.md`
  - [ ] Add agent commands to Common Commands
  - [ ] Add agent overview to Project Overview
  - [ ] Add agent-specific guidelines

#### Performance Optimization
- [ ] Optimize LLM calls
  - [ ] Batch similar queries
  - [ ] Cache common responses
  - [ ] Reduce context size where possible
- [ ] Optimize tool execution
  - [ ] Parallel execution where safe
  - [ ] Command result caching
  - [ ] AWS SDK reuse
- [ ] Optimize TUI rendering
  - [ ] Lazy rendering for large plans
  - [ ] Virtual scrolling for logs
  - [ ] Debounced updates

#### Error Handling
- [ ] Comprehensive error handling
  - [ ] Graceful degradation (no ANTHROPIC_API_KEY)
  - [ ] Network error recovery
  - [ ] AWS CLI error handling
  - [ ] Timeout handling
  - [ ] User-friendly error messages

#### Security
- [ ] Security considerations
  - [ ] Validate all shell commands
  - [ ] Prevent command injection
  - [ ] Sanitize user inputs
  - [ ] Secure credential handling
  - [ ] Audit log of actions taken

---

## File Structure

```
infrastructure/
├── app/
│   ├── agent/                           # NEW: Agent package
│   │   ├── types.go                     # Core types and interfaces
│   │   ├── state.go                     # State management
│   │   ├── orchestrator.go              # Main orchestration logic
│   │   ├── planner.go                   # Plan generation
│   │   ├── tools.go                     # Tool registry
│   │   ├── aws_wrapper.go               # AWS CLI wrapper
│   │   ├── shell_tool.go                # Shell command tool
│   │   ├── web_fetcher.go               # Documentation fetcher
│   │   ├── llm_client.go                # Claude API client
│   │   ├── error_parser.go              # Error parsing logic
│   │   ├── context_builder.go           # Error context collection
│   │   ├── troubleshoot_tui.go          # Main TUI interface
│   │   ├── flow_diagram.go              # Flow diagram rendering
│   │   ├── step_detail.go               # Step detail panel
│   │   ├── tool_display.go              # Tool usage display
│   │   ├── log_viewer.go                # Log streaming view
│   │   ├── status_panel.go              # Status panel
│   │   ├── verification.go              # Fix verification
│   │   ├── learning.go                  # Learning system
│   │   ├── prompts.go                   # LLM prompt templates
│   │   ├── agent_test.go                # Core tests
│   │   ├── tools_test.go                # Tool tests
│   │   └── integration_test.go          # Integration tests
│   │
│   ├── main.go                          # UPDATED: Add troubleshoot command
│   ├── main_menu.go                     # UPDATED: Add menu option
│   ├── cmd.go                           # UPDATED: Add CLI subcommand
│   ├── terraform_apply_tui.go           # UPDATED: Trigger agent on error
│   ├── terraform_plan_modern_tui.go     # UPDATED: Trigger agent on error
│   ├── deploy.go                        # UPDATED: Trigger agent on error
│   └── ai_helper.go                     # UPDATED: Reuse some logic
│
├── ai_docs/                             # AI documentation
│   ├── AGENT_ARCHITECTURE.md            # NEW: Agent architecture
│   ├── AGENT_USAGE.md                   # NEW: Agent usage guide
│   ├── AGENT_PROMPT_LIBRARY.md          # NEW: Prompt templates
│   └── AUTONOMOUS_TROUBLESHOOT_AGENT_PLAN.md  # THIS FILE
│
└── CLAUDE.md                            # UPDATED: Add agent documentation
```

---

## Code Specifications

### 1. Core Types (`app/agent/types.go`)

```go
package agent

import (
    "context"
    "time"
)

// AgentState represents the current state of the autonomous agent
type AgentState string

const (
    StateIdle       AgentState = "idle"
    StateAnalyzing  AgentState = "analyzing"
    StateExecuting  AgentState = "executing"
    StateRecovering AgentState = "recovering"
    StateResolved   AgentState = "resolved"
    StateFailed     AgentState = "failed"
    StatePaused     AgentState = "paused"
)

// ProblemContext contains all information about the problem being solved
type ProblemContext struct {
    // Source information
    Source      string    // "terraform_apply", "ecs_deploy", "manual"
    Environment string    // "dev", "prod", etc.
    AWSProfile  string    // AWS profile name
    AWSRegion   string    // AWS region
    AccountID   string    // AWS account ID

    // Error information
    Errors      []ParsedError   // Structured errors
    RawErrors   []string        // Raw error messages

    // Context
    WorkingDir  string          // Current working directory
    Timestamp   time.Time       // When the problem occurred

    // Additional context
    RecentLogs      []string    // Last 100 lines of logs
    TerraformState  string      // Terraform state snapshot
    ECSServices     []string    // Affected ECS services
    CloudWatchAlarms []string   // Active alarms
}

// ParsedError represents a structured error with categorization
type ParsedError struct {
    Message     string      // Error message
    Category    ErrorCategory // Error category
    Service     string      // AWS service (ec2, ecs, rds, etc.)
    Resource    string      // Affected resource
    Severity    ErrorSeverity
    Suggestions []string    // Initial suggestions
}

type ErrorCategory string

const (
    ErrorCategoryAuth        ErrorCategory = "authentication"
    ErrorCategoryPermission  ErrorCategory = "permission"
    ErrorCategoryResource    ErrorCategory = "resource"
    ErrorCategoryQuota       ErrorCategory = "quota"
    ErrorCategoryNetwork     ErrorCategory = "network"
    ErrorCategoryConfig      ErrorCategory = "configuration"
    ErrorCategoryUnknown     ErrorCategory = "unknown"
)

type ErrorSeverity string

const (
    SeverityCritical ErrorSeverity = "critical"
    SeverityHigh     ErrorSeverity = "high"
    SeverityMedium   ErrorSeverity = "medium"
    SeverityLow      ErrorSeverity = "low"
)

// ExecutionPlan represents a plan to solve the problem
type ExecutionPlan struct {
    ID          string          // Unique plan ID
    Description string          // Human-readable description
    Steps       []ExecutionStep // Ordered steps
    EstimatedDuration time.Duration
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

// ExecutionStep represents a single step in the plan
type ExecutionStep struct {
    ID          string          // Unique step ID
    Number      int             // Step number (1, 2, 3...)
    Description string          // What this step does
    Tool        string          // Tool to use ("aws_cli", "shell", "web_fetch")
    Command     string          // Actual command/query to execute
    Args        map[string]interface{} // Tool-specific arguments

    // Execution state
    Status      StepStatus      // Current status
    StartedAt   *time.Time      // When execution started
    CompletedAt *time.Time      // When execution completed
    Duration    time.Duration   // How long it took

    // Results
    Output      string          // Command output
    Error       string          // Error if failed
    ExitCode    int             // Exit code (for shell commands)

    // Dependencies
    DependsOn   []string        // Step IDs this depends on

    // Verification
    VerifyCommand string        // Command to verify success
    VerifyExpected string       // Expected output for verification
}

type StepStatus string

const (
    StepStatusPending   StepStatus = "pending"
    StepStatusRunning   StepStatus = "running"
    StepStatusSuccess   StepStatus = "success"
    StepStatusFailed    StepStatus = "failed"
    StepStatusSkipped   StepStatus = "skipped"
    StepStatusRetrying  StepStatus = "retrying"
)

// Agent represents the autonomous troubleshooting agent
type Agent struct {
    // Configuration
    Config      *AgentConfig

    // State
    State       AgentState
    Context     *ProblemContext
    Plan        *ExecutionPlan
    CurrentStep int

    // Tools
    ToolRegistry *ToolRegistry
    LLMClient    *LLMClient

    // History
    ExecutionHistory []ExecutionStep
    LLMHistory       []LLMInteraction

    // Metrics
    StartTime    time.Time
    Iterations   int
    ToolUsage    map[string]int // Tool name → usage count
}

// AgentConfig holds configuration for the agent
type AgentConfig struct {
    // Execution limits
    MaxIterations   int           // Max iterations before giving up (default: 10)
    MaxDuration     time.Duration // Max total duration (default: 30 minutes)
    StepTimeout     time.Duration // Timeout per step (default: 5 minutes)

    // LLM settings
    LLMModel        string        // Claude model to use
    LLMMaxTokens    int           // Max tokens per request
    LLMTemperature  float64       // Temperature for creativity

    // Behavior
    AutoApprove     bool          // Auto-approve risky operations
    DryRun          bool          // Don't actually execute changes
    Verbose         bool          // Verbose logging

    // AWS settings
    AWSProfile      string        // AWS profile to use
    AWSRegion       string        // AWS region

    // State persistence
    StateFile       string        // Path to state file (for resume)
}

// Tool interface - all tools must implement this
type Tool interface {
    Name() string
    Description() string
    Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error)
    Validate(args map[string]interface{}) error
}

// ToolResult represents the result of tool execution
type ToolResult struct {
    Success     bool
    Output      string
    Error       string
    ExitCode    int
    Duration    time.Duration
    Metadata    map[string]interface{}
}

// LLMInteraction represents a single LLM API call
type LLMInteraction struct {
    Timestamp   time.Time
    Prompt      string
    Response    string
    TokensUsed  int
    Duration    time.Duration
}

// TUIState represents the state of the TUI
type TUIState struct {
    // View state
    CurrentView      ViewMode
    SelectedStep     int
    SelectedTool     int
    ScrollOffset     int

    // Panel states
    FlowExpanded     map[int]bool  // Step ID → expanded
    LogAutoScroll    bool
    ShowHelp         bool

    // Dimensions
    Width            int
    Height           int
}

type ViewMode string

const (
    ViewModeFlow     ViewMode = "flow"
    ViewModeDetail   ViewMode = "detail"
    ViewModeLog      ViewMode = "log"
    ViewModeHelp     ViewMode = "help"
)
```

### 2. Agent Orchestrator (`app/agent/orchestrator.go`)

```go
package agent

import (
    "context"
    "fmt"
    "time"
)

// NewAgent creates a new autonomous troubleshooting agent
func NewAgent(config *AgentConfig, problemCtx *ProblemContext) (*Agent, error) {
    // Initialize LLM client
    llmClient, err := NewLLMClient(config.LLMModel, config.LLMMaxTokens)
    if err != nil {
        return nil, fmt.Errorf("failed to initialize LLM client: %w", err)
    }

    // Initialize tool registry
    toolRegistry := NewToolRegistry()

    // Register standard tools
    toolRegistry.Register(NewAWSCLITool(config.AWSProfile, config.AWSRegion))
    toolRegistry.Register(NewShellTool())
    toolRegistry.Register(NewWebFetchTool())
    toolRegistry.Register(NewTerraformTool(config.AWSProfile))
    toolRegistry.Register(NewHealthCheckTool(config.AWSProfile, config.AWSRegion))

    return &Agent{
        Config:       config,
        State:        StateIdle,
        Context:      problemCtx,
        ToolRegistry: toolRegistry,
        LLMClient:    llmClient,
        StartTime:    time.Now(),
        ToolUsage:    make(map[string]int),
    }, nil
}

// Run starts the autonomous troubleshooting loop
func (a *Agent) Run(ctx context.Context) error {
    // Phase 1: Analyze the problem and generate plan
    if err := a.analyzeProblem(ctx); err != nil {
        return fmt.Errorf("failed to analyze problem: %w", err)
    }

    // Phase 2: Execute the plan
    if err := a.executePlan(ctx); err != nil {
        return fmt.Errorf("failed to execute plan: %w", err)
    }

    // Phase 3: Verify the fix
    if err := a.verifyResolution(ctx); err != nil {
        return fmt.Errorf("failed to verify resolution: %w", err)
    }

    return nil
}

// analyzeProblem uses LLM to analyze the problem and generate an execution plan
func (a *Agent) analyzeProblem(ctx context.Context) error {
    a.State = StateAnalyzing

    // Build analysis prompt
    prompt := a.buildAnalysisPrompt()

    // Call LLM
    response, err := a.LLMClient.Analyze(ctx, prompt)
    if err != nil {
        return fmt.Errorf("LLM analysis failed: %w", err)
    }

    // Parse response into execution plan
    plan, err := a.parseExecutionPlan(response)
    if err != nil {
        return fmt.Errorf("failed to parse execution plan: %w", err)
    }

    a.Plan = plan
    return nil
}

// executePlan executes the plan step by step
func (a *Agent) executePlan(ctx context.Context) error {
    a.State = StateExecuting

    for a.Iterations < a.Config.MaxIterations {
        // Check timeout
        if time.Since(a.StartTime) > a.Config.MaxDuration {
            return fmt.Errorf("max duration exceeded")
        }

        // Get next step to execute
        step := a.getNextStep()
        if step == nil {
            // Plan complete
            break
        }

        a.CurrentStep = step.Number

        // Execute step
        result, err := a.executeStep(ctx, step)
        if err != nil {
            // Try to recover
            if err := a.handleStepFailure(ctx, step, err); err != nil {
                return err
            }
            continue
        }

        // Update step with result
        step.Output = result.Output
        step.Status = StepStatusSuccess
        step.Duration = result.Duration
        now := time.Now()
        step.CompletedAt = &now

        a.ExecutionHistory = append(a.ExecutionHistory, *step)

        // Ask LLM if we should continue or adjust plan
        shouldContinue, adjustments := a.evaluateProgress(ctx, step, result)
        if !shouldContinue {
            break
        }

        if adjustments != nil {
            a.adjustPlan(adjustments)
        }

        a.Iterations++
    }

    return nil
}

// executeStep executes a single step using the appropriate tool
func (a *Agent) executeStep(ctx context.Context, step *ExecutionStep) (*ToolResult, error) {
    // Get tool from registry
    tool := a.ToolRegistry.Get(step.Tool)
    if tool == nil {
        return nil, fmt.Errorf("tool not found: %s", step.Tool)
    }

    // Validate arguments
    if err := tool.Validate(step.Args); err != nil {
        return nil, fmt.Errorf("invalid arguments: %w", err)
    }

    // Create step context with timeout
    stepCtx, cancel := context.WithTimeout(ctx, a.Config.StepTimeout)
    defer cancel()

    // Mark step as running
    step.Status = StepStatusRunning
    now := time.Now()
    step.StartedAt = &now

    // Execute tool
    result, err := tool.Execute(stepCtx, step.Args)
    if err != nil {
        step.Status = StepStatusFailed
        step.Error = err.Error()
        return nil, err
    }

    // Track tool usage
    a.ToolUsage[step.Tool]++

    return result, nil
}

// handleStepFailure attempts to recover from a step failure
func (a *Agent) handleStepFailure(ctx context.Context, step *ExecutionStep, err error) error {
    a.State = StateRecovering

    // Build recovery prompt
    prompt := a.buildRecoveryPrompt(step, err)

    // Ask LLM for recovery strategy
    response, err := a.LLMClient.Recover(ctx, prompt)
    if err != nil {
        return fmt.Errorf("recovery failed: %w", err)
    }

    // Parse recovery plan
    recoverySteps, err := a.parseRecoverySteps(response)
    if err != nil {
        return fmt.Errorf("failed to parse recovery steps: %w", err)
    }

    // Insert recovery steps into plan
    a.insertRecoverySteps(step.Number, recoverySteps)

    a.State = StateExecuting
    return nil
}

// verifyResolution verifies that the problem has been solved
func (a *Agent) verifyResolution(ctx context.Context) error {
    // Run verification checks
    checks := a.buildVerificationChecks()

    for _, check := range checks {
        result, err := a.executeStep(ctx, &check)
        if err != nil || !result.Success {
            a.State = StateFailed
            return fmt.Errorf("verification failed: %s", check.Description)
        }
    }

    a.State = StateResolved
    return nil
}

// Additional helper methods...
func (a *Agent) buildAnalysisPrompt() string { /* ... */ }
func (a *Agent) parseExecutionPlan(response string) (*ExecutionPlan, error) { /* ... */ }
func (a *Agent) getNextStep() *ExecutionStep { /* ... */ }
func (a *Agent) evaluateProgress(ctx context.Context, step *ExecutionStep, result *ToolResult) (bool, []ExecutionStep) { /* ... */ }
func (a *Agent) adjustPlan(adjustments []ExecutionStep) { /* ... */ }
func (a *Agent) buildRecoveryPrompt(step *ExecutionStep, err error) string { /* ... */ }
func (a *Agent) parseRecoverySteps(response string) ([]ExecutionStep, error) { /* ... */ }
func (a *Agent) insertRecoverySteps(afterStep int, steps []ExecutionStep) { /* ... */ }
func (a *Agent) buildVerificationChecks() []ExecutionStep { /* ... */ }
```

### 3. Tool Registry (`app/agent/tools.go`)

```go
package agent

import (
    "context"
    "fmt"
    "sync"
)

// ToolRegistry manages available tools
type ToolRegistry struct {
    tools map[string]Tool
    mu    sync.RWMutex
}

func NewToolRegistry() *ToolRegistry {
    return &ToolRegistry{
        tools: make(map[string]Tool),
    }
}

func (r *ToolRegistry) Register(tool Tool) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.tools[tool.Name()] = tool
}

func (r *ToolRegistry) Get(name string) Tool {
    r.mu.RLock()
    defer r.mu.RUnlock()
    return r.tools[name]
}

func (r *ToolRegistry) List() []Tool {
    r.mu.RLock()
    defer r.mu.RUnlock()

    tools := make([]Tool, 0, len(r.tools))
    for _, tool := range r.tools {
        tools = append(tools, tool)
    }
    return tools
}
```

### 4. AWS CLI Tool (`app/agent/aws_wrapper.go`)

```go
package agent

import (
    "context"
    "encoding/json"
    "fmt"
    "os/exec"
    "time"
)

// AWSCLITool wraps the AWS CLI with profile awareness
type AWSCLITool struct {
    profile string
    region  string
}

func NewAWSCLITool(profile, region string) *AWSCLITool {
    return &AWSCLITool{
        profile: profile,
        region:  region,
    }
}

func (t *AWSCLITool) Name() string {
    return "aws_cli"
}

func (t *AWSCLITool) Description() string {
    return "Execute AWS CLI commands with automatic profile management"
}

func (t *AWSCLITool) Validate(args map[string]interface{}) error {
    if _, ok := args["command"]; !ok {
        return fmt.Errorf("missing required argument: command")
    }
    return nil
}

func (t *AWSCLITool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
    command := args["command"].(string)

    // Build AWS CLI command
    cmdArgs := []string{
        "aws",
        "--profile", t.profile,
        "--region", t.region,
        "--output", "json",
    }
    cmdArgs = append(cmdArgs, parseCommand(command)...)

    // Execute command
    start := time.Now()
    cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)

    output, err := cmd.CombinedOutput()
    duration := time.Since(start)

    result := &ToolResult{
        Duration: duration,
    }

    if err != nil {
        result.Success = false
        result.Error = err.Error()
        result.Output = string(output)
        if exitErr, ok := err.(*exec.ExitError); ok {
            result.ExitCode = exitErr.ExitCode()
        }
        return result, nil // Return result, not error (we want to capture output)
    }

    result.Success = true
    result.Output = string(output)
    result.ExitCode = 0

    // Try to parse JSON output
    var jsonOutput interface{}
    if err := json.Unmarshal(output, &jsonOutput); err == nil {
        result.Metadata = map[string]interface{}{
            "json": jsonOutput,
        }
    }

    return result, nil
}

// Helper function to parse command string into arguments
func parseCommand(cmd string) []string {
    // Simple split for now, could be more sophisticated
    return strings.Fields(cmd)
}

// Common AWS CLI commands as convenience methods
func (t *AWSCLITool) DescribeECSService(ctx context.Context, cluster, service string) (*ToolResult, error) {
    return t.Execute(ctx, map[string]interface{}{
        "command": fmt.Sprintf("ecs describe-services --cluster %s --services %s", cluster, service),
    })
}

func (t *AWSCLITool) GetLogEvents(ctx context.Context, logGroup, logStream string, limit int) (*ToolResult, error) {
    return t.Execute(ctx, map[string]interface{}{
        "command": fmt.Sprintf("logs get-log-events --log-group-name %s --log-stream-name %s --limit %d", logGroup, logStream, limit),
    })
}

// ... more convenience methods
```

### 5. LLM Client (`app/agent/llm_client.go`)

```go
package agent

import (
    "context"
    "fmt"
    "os"
    "time"

    "github.com/anthropics/anthropic-sdk-go"
    "github.com/anthropics/anthropic-sdk-go/option"
)

// LLMClient handles interactions with Claude API
type LLMClient struct {
    client    *anthropic.Client
    model     string
    maxTokens int
    history   []LLMInteraction
}

func NewLLMClient(model string, maxTokens int) (*LLMClient, error) {
    apiKey := os.Getenv("ANTHROPIC_API_KEY")
    if apiKey == "" {
        return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
    }

    client := anthropic.NewClient(
        option.WithAPIKey(apiKey),
    )

    return &LLMClient{
        client:    client,
        model:     model,
        maxTokens: maxTokens,
        history:   make([]LLMInteraction, 0),
    }, nil
}

// Analyze analyzes a problem and generates an execution plan
func (c *LLMClient) Analyze(ctx context.Context, prompt string) (string, error) {
    return c.call(ctx, prompt, "You are an expert AWS infrastructure troubleshooter.")
}

// Recover generates a recovery plan after a step failure
func (c *LLMClient) Recover(ctx context.Context, prompt string) (string, error) {
    return c.call(ctx, prompt, "You are an expert at error recovery in AWS deployments.")
}

// Evaluate evaluates progress and determines next actions
func (c *LLMClient) Evaluate(ctx context.Context, prompt string) (string, error) {
    return c.call(ctx, prompt, "You are evaluating the progress of an AWS troubleshooting session.")
}

// call makes a Claude API call with the given prompt
func (c *LLMClient) call(ctx context.Context, prompt, systemPrompt string) (string, error) {
    start := time.Now()

    message, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
        Model:     c.model,
        MaxTokens: c.maxTokens,
        System:    systemPrompt,
        Messages: []anthropic.MessageParam{
            anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
        },
    })

    duration := time.Since(start)

    if err != nil {
        return "", fmt.Errorf("API call failed: %w", err)
    }

    if len(message.Content) == 0 {
        return "", fmt.Errorf("empty response from API")
    }

    responseText := message.Content[0].Text

    // Record interaction
    c.history = append(c.history, LLMInteraction{
        Timestamp:  time.Now(),
        Prompt:     prompt,
        Response:   responseText,
        TokensUsed: message.Usage.InputTokens + message.Usage.OutputTokens,
        Duration:   duration,
    })

    return responseText, nil
}

// GetTotalTokensUsed returns the total tokens used across all interactions
func (c *LLMClient) GetTotalTokensUsed() int {
    total := 0
    for _, interaction := range c.history {
        total += interaction.TokensUsed
    }
    return total
}
```

### 6. TUI Main Interface (`app/agent/troubleshoot_tui.go`)

```go
package agent

import (
    "fmt"
    "strings"

    "github.com/charmbracelet/bubbles/help"
    "github.com/charmbracelet/bubbles/key"
    "github.com/charmbracelet/bubbles/viewport"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
)

// Message types for TUI updates
type (
    // Agent state updates
    agentStateMsg struct{ state AgentState }
    planUpdatedMsg struct{ plan *ExecutionPlan }
    stepStartedMsg struct{ step *ExecutionStep }
    stepCompletedMsg struct{ step *ExecutionStep }
    stepFailedMsg struct{ step *ExecutionStep; err error }

    // User actions
    pauseMsg struct{}
    resumeMsg struct{}
    stopMsg struct{}
)

// TroubleshootModel is the Bubble Tea model for the troubleshooting TUI
type TroubleshootModel struct {
    agent          *Agent
    tuiState       *TUIState

    // Viewports for different panels
    flowViewport   viewport.Model
    detailViewport viewport.Model
    logViewport    viewport.Model

    // UI components
    help           help.Model
    keys           troubleshootKeyMap

    width          int
    height         int
}

type troubleshootKeyMap struct {
    Up        key.Binding
    Down      key.Binding
    PageUp    key.Binding
    PageDown  key.Binding
    Enter     key.Binding
    Tab       key.Binding
    Pause     key.Binding
    Stop      key.Binding
    Help      key.Binding
    Quit      key.Binding
}

func (k troubleshootKeyMap) ShortHelp() []key.Binding {
    return []key.Binding{k.Up, k.Down, k.Enter, k.Pause, k.Help, k.Quit}
}

func (k troubleshootKeyMap) FullHelp() [][]key.Binding {
    return [][]key.Binding{
        {k.Up, k.Down, k.PageUp, k.PageDown},
        {k.Enter, k.Tab, k.Pause, k.Stop},
        {k.Help, k.Quit},
    }
}

func newTroubleshootKeyMap() troubleshootKeyMap {
    return troubleshootKeyMap{
        Up: key.NewBinding(
            key.WithKeys("up", "k"),
            key.WithHelp("↑/k", "up"),
        ),
        Down: key.NewBinding(
            key.WithKeys("down", "j"),
            key.WithHelp("↓/j", "down"),
        ),
        PageUp: key.NewBinding(
            key.WithKeys("pgup", "b"),
            key.WithHelp("pgup/b", "page up"),
        ),
        PageDown: key.NewBinding(
            key.WithKeys("pgdown", "f"),
            key.WithHelp("pgdn/f", "page down"),
        ),
        Enter: key.NewBinding(
            key.WithKeys("enter"),
            key.WithHelp("enter", "expand/collapse"),
        ),
        Tab: key.NewBinding(
            key.WithKeys("tab"),
            key.WithHelp("tab", "switch view"),
        ),
        Pause: key.NewBinding(
            key.WithKeys("p"),
            key.WithHelp("p", "pause/resume"),
        ),
        Stop: key.NewBinding(
            key.WithKeys("s"),
            key.WithHelp("s", "stop"),
        ),
        Help: key.NewBinding(
            key.WithKeys("?"),
            key.WithHelp("?", "toggle help"),
        ),
        Quit: key.NewBinding(
            key.WithKeys("q", "ctrl+c"),
            key.WithHelp("q", "quit"),
        ),
    }
}

func NewTroubleshootModel(agent *Agent) *TroubleshootModel {
    return &TroubleshootModel{
        agent:    agent,
        tuiState: &TUIState{
            CurrentView:   ViewModeFlow,
            FlowExpanded:  make(map[int]bool),
            LogAutoScroll: true,
            ShowHelp:      false,
        },
        help: help.New(),
        keys: newTroubleshootKeyMap(),
    }
}

func (m *TroubleshootModel) Init() tea.Cmd {
    // Start the agent in a goroutine and send updates via messages
    return tea.Batch(
        m.startAgent(),
        m.tickCmd(),
    )
}

func (m *TroubleshootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmds []tea.Cmd

    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        m.width = msg.Width
        m.height = msg.Height
        m.updateViewportSizes()

    case tea.KeyMsg:
        switch {
        case key.Matches(msg, m.keys.Quit):
            return m, tea.Quit

        case key.Matches(msg, m.keys.Pause):
            if m.agent.State == StatePaused {
                return m, func() tea.Msg { return resumeMsg{} }
            } else {
                return m, func() tea.Msg { return pauseMsg{} }
            }

        case key.Matches(msg, m.keys.Stop):
            return m, func() tea.Msg { return stopMsg{} }

        case key.Matches(msg, m.keys.Help):
            m.tuiState.ShowHelp = !m.tuiState.ShowHelp

        case key.Matches(msg, m.keys.Tab):
            m.cycleView()

        case key.Matches(msg, m.keys.Enter):
            m.toggleStepExpansion()

        case key.Matches(msg, m.keys.Up):
            m.moveSelection(-1)

        case key.Matches(msg, m.keys.Down):
            m.moveSelection(1)
        }

    case agentStateMsg:
        m.agent.State = msg.state

    case planUpdatedMsg:
        m.agent.Plan = msg.plan

    case stepStartedMsg:
        // Update step in plan

    case stepCompletedMsg:
        // Update step in plan

    case stepFailedMsg:
        // Handle step failure

    case pauseMsg:
        m.agent.State = StatePaused

    case resumeMsg:
        m.agent.State = StateExecuting

    case stopMsg:
        m.agent.State = StateFailed
        return m, tea.Quit
    }

    // Update viewports
    var cmd tea.Cmd
    m.flowViewport, cmd = m.flowViewport.Update(msg)
    cmds = append(cmds, cmd)

    m.detailViewport, cmd = m.detailViewport.Update(msg)
    cmds = append(cmds, cmd)

    m.logViewport, cmd = m.logViewport.Update(msg)
    cmds = append(cmds, cmd)

    return m, tea.Batch(cmds...)
}

func (m *TroubleshootModel) View() string {
    if m.width == 0 {
        return "Initializing..."
    }

    // Build the view based on current view mode
    var content string

    switch m.tuiState.CurrentView {
    case ViewModeFlow:
        content = m.renderFlowView()
    case ViewModeDetail:
        content = m.renderDetailView()
    case ViewModeLog:
        content = m.renderLogView()
    case ViewModeHelp:
        content = m.renderHelpView()
    }

    // Add status bar at bottom
    statusBar := m.renderStatusBar()

    // Add help hint if not showing full help
    var helpHint string
    if !m.tuiState.ShowHelp {
        helpHint = m.help.View(m.keys)
    }

    return lipgloss.JoinVertical(
        lipgloss.Left,
        content,
        statusBar,
        helpHint,
    )
}

func (m *TroubleshootModel) renderFlowView() string {
    // Render the flow diagram showing all steps
    // See detailed implementation in flow_diagram.go
    return renderFlowDiagram(m.agent.Plan, m.tuiState.SelectedStep, m.tuiState.FlowExpanded)
}

func (m *TroubleshootModel) renderDetailView() string {
    // Render detailed view of selected step
    // See detailed implementation in step_detail.go
    if m.agent.Plan == nil || m.tuiState.SelectedStep >= len(m.agent.Plan.Steps) {
        return "No step selected"
    }
    step := &m.agent.Plan.Steps[m.tuiState.SelectedStep]
    return renderStepDetail(step)
}

func (m *TroubleshootModel) renderLogView() string {
    // Render log stream
    // See detailed implementation in log_viewer.go
    return m.logViewport.View()
}

func (m *TroubleshootModel) renderHelpView() string {
    return m.help.View(m.keys)
}

func (m *TroubleshootModel) renderStatusBar() string {
    // Status indicators
    stateColor := getStateColor(m.agent.State)
    stateIndicator := lipgloss.NewStyle().
        Foreground(stateColor).
        Bold(true).
        Render(string(m.agent.State))

    // Progress
    var progress string
    if m.agent.Plan != nil {
        completed := 0
        for _, step := range m.agent.Plan.Steps {
            if step.Status == StepStatusSuccess {
                completed++
            }
        }
        progress = fmt.Sprintf("%d/%d steps", completed, len(m.agent.Plan.Steps))
    }

    // Time elapsed
    elapsed := time.Since(m.agent.StartTime).Round(time.Second)

    // Build status bar
    leftSection := fmt.Sprintf("State: %s | %s | Time: %s",
        stateIndicator, progress, elapsed)

    rightSection := fmt.Sprintf("Env: %s | Profile: %s",
        m.agent.Context.Environment, m.agent.Context.AWSProfile)

    statusWidth := m.width
    padding := statusWidth - lipgloss.Width(leftSection) - lipgloss.Width(rightSection)
    if padding < 0 {
        padding = 0
    }

    statusBar := lipgloss.NewStyle().
        Background(lipgloss.Color("236")).
        Foreground(lipgloss.Color("250")).
        Width(statusWidth).
        Render(leftSection + strings.Repeat(" ", padding) + rightSection)

    return statusBar
}

// Helper methods
func (m *TroubleshootModel) startAgent() tea.Cmd {
    return func() tea.Msg {
        // Run agent in background, send messages for updates
        // This would be implemented with channels
        return nil
    }
}

func (m *TroubleshootModel) tickCmd() tea.Cmd {
    return tea.Tick(time.Second, func(t time.Time) tea.Msg {
        return agentStateMsg{state: m.agent.State}
    })
}

func (m *TroubleshootModel) updateViewportSizes() {
    // Calculate viewport sizes based on current view mode
    // Implementation depends on layout
}

func (m *TroubleshootModel) cycleView() {
    views := []ViewMode{ViewModeFlow, ViewModeDetail, ViewModeLog}
    for i, v := range views {
        if v == m.tuiState.CurrentView {
            m.tuiState.CurrentView = views[(i+1)%len(views)]
            break
        }
    }
}

func (m *TroubleshootModel) toggleStepExpansion() {
    if m.agent.Plan == nil {
        return
    }
    current := m.tuiState.FlowExpanded[m.tuiState.SelectedStep]
    m.tuiState.FlowExpanded[m.tuiState.SelectedStep] = !current
}

func (m *TroubleshootModel) moveSelection(delta int) {
    if m.agent.Plan == nil {
        return
    }
    m.tuiState.SelectedStep += delta
    if m.tuiState.SelectedStep < 0 {
        m.tuiState.SelectedStep = 0
    }
    if m.tuiState.SelectedStep >= len(m.agent.Plan.Steps) {
        m.tuiState.SelectedStep = len(m.agent.Plan.Steps) - 1
    }
}

func getStateColor(state AgentState) lipgloss.Color {
    switch state {
    case StateIdle:
        return lipgloss.Color("243") // Gray
    case StateAnalyzing:
        return lipgloss.Color("39")  // Blue
    case StateExecuting:
        return lipgloss.Color("226") // Yellow
    case StateRecovering:
        return lipgloss.Color("208") // Orange
    case StateResolved:
        return lipgloss.Color("82")  // Green
    case StateFailed:
        return lipgloss.Color("196") // Red
    case StatePaused:
        return lipgloss.Color("243") // Gray
    default:
        return lipgloss.Color("250") // White
    }
}
```

### 7. Prompt Templates (`app/agent/prompts.go`)

```go
package agent

import (
    "fmt"
    "strings"
)

// buildAnalysisPrompt creates the initial analysis prompt for the LLM
func (a *Agent) buildAnalysisPrompt() string {
    var prompt strings.Builder

    prompt.WriteString("You are an expert AWS infrastructure troubleshooter. ")
    prompt.WriteString("Analyze the following deployment error and create a step-by-step execution plan to fix it.\n\n")

    // Context
    prompt.WriteString("## Context\n")
    prompt.WriteString(fmt.Sprintf("- Environment: %s\n", a.Context.Environment))
    prompt.WriteString(fmt.Sprintf("- AWS Profile: %s\n", a.Context.AWSProfile))
    prompt.WriteString(fmt.Sprintf("- AWS Region: %s\n", a.Context.AWSRegion))
    prompt.WriteString(fmt.Sprintf("- Account ID: %s\n", a.Context.AccountID))
    prompt.WriteString(fmt.Sprintf("- Source: %s\n\n", a.Context.Source))

    // Errors
    prompt.WriteString("## Errors\n")
    for i, err := range a.Context.Errors {
        prompt.WriteString(fmt.Sprintf("%d. [%s/%s] %s\n",
            i+1, err.Category, err.Service, err.Message))
    }
    prompt.WriteString("\n")

    // Raw error output (if available)
    if len(a.Context.RawErrors) > 0 {
        prompt.WriteString("## Raw Error Output\n")
        prompt.WriteString("```\n")
        for _, raw := range a.Context.RawErrors {
            prompt.WriteString(raw)
            prompt.WriteString("\n")
        }
        prompt.WriteString("```\n\n")
    }

    // Recent logs (if available)
    if len(a.Context.RecentLogs) > 0 {
        prompt.WriteString("## Recent Logs (last 100 lines)\n")
        prompt.WriteString("```\n")
        for _, log := range a.Context.RecentLogs {
            prompt.WriteString(log)
            prompt.WriteString("\n")
        }
        prompt.WriteString("```\n\n")
    }

    // Available tools
    prompt.WriteString("## Available Tools\n")
    tools := a.ToolRegistry.List()
    for _, tool := range tools {
        prompt.WriteString(fmt.Sprintf("- **%s**: %s\n", tool.Name(), tool.Description()))
    }
    prompt.WriteString("\n")

    // Instructions
    prompt.WriteString("## Instructions\n")
    prompt.WriteString("Create a detailed execution plan to diagnose and fix this issue. ")
    prompt.WriteString("Format your response as JSON with the following structure:\n\n")
    prompt.WriteString("```json\n")
    prompt.WriteString(`{
  "description": "Brief description of the problem and approach",
  "estimated_duration": "5m",
  "steps": [
    {
      "number": 1,
      "description": "What this step does",
      "tool": "aws_cli|shell|web_fetch|terraform|health_check",
      "command": "Exact command to run",
      "args": {
        "key": "value"
      },
      "verify_command": "Optional command to verify success",
      "verify_expected": "Expected output from verification"
    }
  ]
}
`)
    prompt.WriteString("```\n\n")

    prompt.WriteString("Guidelines:\n")
    prompt.WriteString("1. Start with diagnostic commands to understand the problem\n")
    prompt.WriteString("2. Then execute fix commands\n")
    prompt.WriteString("3. Finally, verify the fix worked\n")
    prompt.WriteString("4. Keep the plan concise (5-10 steps max)\n")
    prompt.WriteString("5. Use specific AWS CLI commands, not placeholders\n")
    prompt.WriteString("6. Include proper error handling\n")
    prompt.WriteString("7. Consider dependencies between steps\n")

    return prompt.String()
}

// buildRecoveryPrompt creates a recovery prompt after a step failure
func (a *Agent) buildRecoveryPrompt(step *ExecutionStep, err error) string {
    var prompt strings.Builder

    prompt.WriteString("A step in the troubleshooting plan failed. Generate a recovery strategy.\n\n")

    prompt.WriteString("## Failed Step\n")
    prompt.WriteString(fmt.Sprintf("- Step %d: %s\n", step.Number, step.Description))
    prompt.WriteString(fmt.Sprintf("- Tool: %s\n", step.Tool))
    prompt.WriteString(fmt.Sprintf("- Command: %s\n", step.Command))
    prompt.WriteString(fmt.Sprintf("- Error: %s\n\n", err.Error()))

    if step.Output != "" {
        prompt.WriteString("## Output\n")
        prompt.WriteString("```\n")
        prompt.WriteString(step.Output)
        prompt.WriteString("\n```\n\n")
    }

    prompt.WriteString("## Current Plan Progress\n")
    for i, s := range a.Plan.Steps {
        status := "pending"
        if i < step.Number-1 {
            status = "completed"
        } else if i == step.Number-1 {
            status = "failed"
        }
        prompt.WriteString(fmt.Sprintf("- [%s] Step %d: %s\n", status, s.Number, s.Description))
    }
    prompt.WriteString("\n")

    prompt.WriteString("## Instructions\n")
    prompt.WriteString("Provide 1-3 alternative steps to recover from this failure. ")
    prompt.WriteString("Consider:\n")
    prompt.WriteString("1. Was the error due to missing permissions? → Add IAM policy\n")
    prompt.WriteString("2. Was it a transient error? → Retry with backoff\n")
    prompt.WriteString("3. Was it a wrong approach? → Try different method\n")
    prompt.WriteString("4. Is manual intervention needed? → Provide clear instructions\n\n")

    prompt.WriteString("Format as JSON:\n")
    prompt.WriteString("```json\n")
    prompt.WriteString(`{
  "recovery_steps": [
    {
      "number": 1,
      "description": "...",
      "tool": "...",
      "command": "...",
      "args": {}
    }
  ]
}
`)
    prompt.WriteString("```\n")

    return prompt.String()
}

// buildEvaluationPrompt creates a prompt to evaluate progress and decide next steps
func (a *Agent) buildEvaluationPrompt(step *ExecutionStep, result *ToolResult) string {
    var prompt strings.Builder

    prompt.WriteString("Evaluate the result of the last step and determine if we should continue, adjust, or stop.\n\n")

    prompt.WriteString("## Completed Step\n")
    prompt.WriteString(fmt.Sprintf("- Step %d: %s\n", step.Number, step.Description))
    prompt.WriteString(fmt.Sprintf("- Tool: %s\n", step.Tool))
    prompt.WriteString(fmt.Sprintf("- Success: %v\n", result.Success))
    prompt.WriteString(fmt.Sprintf("- Duration: %s\n\n", result.Duration))

    prompt.WriteString("## Output\n")
    prompt.WriteString("```\n")
    prompt.WriteString(result.Output)
    prompt.WriteString("\n```\n\n")

    prompt.WriteString("## Question\n")
    prompt.WriteString("Based on this output:\n")
    prompt.WriteString("1. Should we continue with the plan as-is? (YES/NO)\n")
    prompt.WriteString("2. Do we need to adjust the plan? (YES/NO)\n")
    prompt.WriteString("3. Is the problem already solved? (YES/NO)\n\n")

    prompt.WriteString("If adjustments are needed, provide new steps in JSON format.\n")
    prompt.WriteString("Format:\n")
    prompt.WriteString("```json\n")
    prompt.WriteString(`{
  "continue": true|false,
  "problem_solved": true|false,
  "adjustments": [
    {
      "number": 1,
      "description": "...",
      "tool": "...",
      "command": "...",
      "args": {}
    }
  ]
}
`)
    prompt.WriteString("```\n")

    return prompt.String()
}
```

---

## Testing Strategy

### Unit Tests

#### Tool Tests (`app/agent/tools_test.go`)
```go
func TestAWSCLITool_Execute(t *testing.T)
func TestShellTool_Execute(t *testing.T)
func TestWebFetchTool_Execute(t *testing.T)
func TestToolRegistry_Register(t *testing.T)
func TestToolRegistry_Get(t *testing.T)
```

#### Error Parser Tests (`app/agent/error_parser_test.go`)
```go
func TestParseError_Terraform(t *testing.T)
func TestParseError_ECS(t *testing.T)
func TestParseError_CloudWatch(t *testing.T)
func TestErrorCategorization(t *testing.T)
```

#### State Management Tests (`app/agent/state_test.go`)
```go
func TestState_Persistence(t *testing.T)
func TestState_Transitions(t *testing.T)
func TestState_Concurrency(t *testing.T)
```

#### Planner Tests (`app/agent/planner_test.go`)
```go
func TestPlanGeneration(t *testing.T)
func TestPlanValidation(t *testing.T)
func TestPlanAdjustment(t *testing.T)
```

### Integration Tests

#### End-to-End Scenarios (`app/agent/integration_test.go`)
```go
func TestScenario_ECSServiceFailure(t *testing.T) {
    // Mock: ECS service stuck in DRAINING state
    // Expected: Agent detects issue, restarts service
}

func TestScenario_IAMPermissionError(t *testing.T) {
    // Mock: Terraform apply fails due to missing IAM permission
    // Expected: Agent identifies missing permission, suggests fix
}

func TestScenario_RDSConnectionFailure(t *testing.T) {
    // Mock: ECS task can't connect to RDS
    // Expected: Agent checks security groups, suggests fix
}

func TestScenario_QuotaLimitError(t *testing.T) {
    // Mock: Service quota exceeded
    // Expected: Agent identifies quota issue, suggests increase
}

func TestScenario_NetworkConnectivityIssue(t *testing.T) {
    // Mock: Service can't reach internet
    // Expected: Agent checks route tables, NAT gateway, etc.
}
```

### Manual Testing Scenarios

1. **ECS Service Failure Recovery**
   - Manually stop an ECS service
   - Run agent, verify it detects and restarts service

2. **RDS Connection Issue**
   - Misconfigure RDS security group
   - Run agent, verify it identifies and fixes security group rules

3. **IAM Permission Errors**
   - Remove required IAM permission
   - Run terraform apply, trigger agent
   - Verify agent suggests correct permission

4. **Quota Limit Errors**
   - Simulate quota limit (mock AWS response)
   - Verify agent identifies quota and suggests increase

5. **Network Issues**
   - Misconfigure route table
   - Verify agent identifies routing issue

### TUI Testing

Use Bubble Tea's built-in testing utilities:

```go
func TestTUI_Rendering(t *testing.T) {
    model := NewTroubleshootModel(mockAgent)

    // Test initial render
    view := model.View()
    assert.Contains(t, view, "State: analyzing")

    // Test keyboard navigation
    model, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
    assert.Equal(t, 1, model.tuiState.SelectedStep)
}
```

---

## Integration Points

### 1. Main Menu Integration (`app/main_menu.go`)

```go
// Add to mainMenu() function
options := []huh.Option[string]{
    huh.NewOption("🌐 Edit environment with web UI", "api"),
    huh.NewOption("🚀 Deploy environment", "deploy"),
    huh.NewOption("🔧 Troubleshoot Deployment", "troubleshoot"),  // NEW
    huh.NewOption("✨ Create new environment", "create"),
    // ... existing options
}

// Add case in switch statement
case action == "troubleshoot":
    troubleshootMenu()
    return mainMenu()
```

### 2. CLI Command Integration (`app/cmd.go`)

```go
// Add new command
var troubleshootCmd = &cobra.Command{
    Use:   "troubleshoot",
    Short: "Autonomous troubleshooting for AWS deployment issues",
    Long:  `Analyze and automatically fix AWS deployment errors using AI.`,
    Run: func(cmd *cobra.Command, args []string) {
        runTroubleshoot()
    },
}

func init() {
    rootCmd.AddCommand(troubleshootCmd)

    troubleshootCmd.Flags().String("error-file", "", "Path to error log file")
    troubleshootCmd.Flags().Bool("auto-approve", false, "Auto-approve risky operations")
}
```

### 3. Terraform Apply Integration (`app/terraform_apply_tui.go`)

```go
// Add after apply failure
if applyFailed {
    fmt.Println("\nDeployment failed. Would you like AI assistance? (y/n)")
    var response string
    fmt.Scanln(&response)

    if response == "y" || response == "yes" {
        // Build problem context from apply errors
        ctx := buildProblemContextFromApplyErrors(applyErrors)

        // Launch troubleshooting agent
        runTroubleshootingAgent(ctx)
    }
}
```

### 4. Error Context Collection

```go
// app/agent/context_builder.go

func BuildProblemContextFromTerraform(errors []string, env string) *ProblemContext {
    ctx := &ProblemContext{
        Source:      "terraform_apply",
        Environment: env,
        AWSProfile:  selectedAWSProfile,
        AWSRegion:   getRegionFromEnv(env),
        AccountID:   getAccountIDFromEnv(env),
        RawErrors:   errors,
        WorkingDir:  getCurrentDir(),
        Timestamp:   time.Now(),
    }

    // Parse errors
    ctx.Errors = parseErrors(errors)

    // Collect additional context
    ctx.RecentLogs = getRecentCloudWatchLogs(env, 100)
    ctx.TerraformState = getTerraformState(env)
    ctx.ECSServices = listECSServices(env)
    ctx.CloudWatchAlarms = getActiveAlarms(env)

    return ctx
}
```

---

## Phase Timeline

### Week 1: Foundation
- Set up agent package structure
- Implement core types and interfaces
- Build tool registry and basic tools
- Create LLM client wrapper

### Week 2: Agent Logic
- Implement error parsing
- Build planning system
- Create orchestrator with execution loop
- Add recovery mechanisms

### Week 3: TUI Interface
- Design and implement flow diagram view
- Build step detail panel
- Create log viewer
- Add interactive controls

### Week 4: Integration
- Integrate with main menu
- Add CLI commands
- Hook into terraform operations
- Build error context collection

### Week 5: Advanced Features
- Multi-step planning with dependencies
- Learning system for common issues
- Fix verification system
- Enhanced error recovery

### Week 6: Polish & Testing
- Comprehensive unit tests
- Integration tests
- Manual testing scenarios
- Documentation
- Performance optimization
- Security review

---

## Success Criteria

The autonomous troubleshooting agent will be considered production-ready when:

1. **Functionality**
   - [ ] Can autonomously diagnose and fix at least 5 common AWS deployment errors
   - [ ] Execution loop works reliably with error recovery
   - [ ] All tools execute correctly with proper error handling
   - [ ] State persists and can be resumed

2. **User Experience**
   - [ ] TUI is responsive and intuitive
   - [ ] Flow diagram clearly shows progress
   - [ ] All views render correctly at various terminal sizes
   - [ ] Keyboard navigation works smoothly

3. **Integration**
   - [ ] Seamlessly integrates into existing meroku CLI
   - [ ] Triggered automatically on terraform failures
   - [ ] Uses existing AWS profile management
   - [ ] Respects user preferences and configurations

4. **Reliability**
   - [ ] 80%+ test coverage
   - [ ] All integration tests pass
   - [ ] Handles network errors gracefully
   - [ ] No credential leaks or security issues

5. **Documentation**
   - [ ] Architecture documented in ai_docs/
   - [ ] User guide available
   - [ ] CLAUDE.md updated with agent info
   - [ ] Code comments on complex logic

---

## Future Enhancements (Post-MVP)

1. **Multi-Agent Collaboration**
   - Specialized agents for different AWS services
   - Agent coordination and delegation

2. **Machine Learning**
   - Train on historical troubleshooting sessions
   - Predict likely issues before they occur
   - Suggest preventive measures

3. **Interactive Mode**
   - Ask user questions during troubleshooting
   - Request manual approval for risky operations
   - Suggest multiple fix options with trade-offs

4. **Advanced Visualization**
   - Dependency graphs
   - Timeline view of events
   - Resource impact analysis

5. **Collaboration Features**
   - Share troubleshooting sessions with team
   - Export diagnostic reports
   - Integration with incident management tools

---

## Appendix

### A. Example Execution Plan (JSON)

```json
{
  "description": "ECS service stuck in DRAINING state - restart service",
  "estimated_duration": "5m",
  "steps": [
    {
      "number": 1,
      "description": "Check ECS service status",
      "tool": "aws_cli",
      "command": "ecs describe-services --cluster my-cluster --services backend-service",
      "args": {
        "cluster": "my-cluster",
        "service": "backend-service"
      },
      "verify_command": "",
      "verify_expected": ""
    },
    {
      "number": 2,
      "description": "Get recent task events",
      "tool": "aws_cli",
      "command": "ecs describe-tasks --cluster my-cluster --tasks $(aws ecs list-tasks --cluster my-cluster --service-name backend-service --query 'taskArns[0]' --output text)",
      "args": {
        "cluster": "my-cluster",
        "service": "backend-service"
      },
      "verify_command": "",
      "verify_expected": ""
    },
    {
      "number": 3,
      "description": "Update service to force new deployment",
      "tool": "aws_cli",
      "command": "ecs update-service --cluster my-cluster --service backend-service --force-new-deployment",
      "args": {
        "cluster": "my-cluster",
        "service": "backend-service",
        "force_new_deployment": true
      },
      "verify_command": "ecs describe-services --cluster my-cluster --services backend-service --query 'services[0].deployments[0].status' --output text",
      "verify_expected": "PRIMARY"
    },
    {
      "number": 4,
      "description": "Wait for service to stabilize",
      "tool": "aws_cli",
      "command": "ecs wait services-stable --cluster my-cluster --services backend-service",
      "args": {
        "cluster": "my-cluster",
        "service": "backend-service"
      },
      "verify_command": "",
      "verify_expected": ""
    },
    {
      "number": 5,
      "description": "Verify service health",
      "tool": "health_check",
      "command": "check-ecs-service-health",
      "args": {
        "cluster": "my-cluster",
        "service": "backend-service"
      },
      "verify_command": "ecs describe-services --cluster my-cluster --services backend-service --query 'services[0].runningCount' --output text",
      "verify_expected": "2"
    }
  ]
}
```

### B. Example LLM Prompt Templates

See `app/agent/prompts.go` for full implementation.

### C. Error Categories Reference

| Category | Examples | Common Fixes |
|----------|----------|--------------|
| Authentication | AWS credentials expired, invalid access key | Refresh credentials, configure profile |
| Permission | IAM policy missing, access denied | Add required IAM permissions |
| Resource | Resource not found, dependency missing | Create missing resource, fix references |
| Quota | Service quota exceeded, limit reached | Request quota increase |
| Network | Connection timeout, DNS failure | Fix security groups, route tables |
| Configuration | Invalid parameter, misconfiguration | Fix configuration, validate inputs |

### D. Tool Catalog

| Tool | Purpose | Example Command |
|------|---------|-----------------|
| aws_cli | Execute AWS CLI commands | `ecs describe-services --cluster foo --services bar` |
| shell | Run shell commands | `terraform init` |
| web_fetch | Fetch documentation | `https://docs.aws.amazon.com/ecs/...` |
| terraform | Run terraform operations | `terraform plan -out=plan.json` |
| health_check | Verify service health | Check ECS service running count |

---

## Conclusion

This implementation plan provides a comprehensive roadmap for building a production-ready autonomous AWS troubleshooting agent integrated into the meroku CLI. The agent will significantly improve the developer experience by automatically diagnosing and fixing common deployment issues, reducing time-to-resolution and cognitive load.

The phased approach ensures steady progress with testable milestones, while the modular architecture allows for future enhancements and extensibility. The beautiful TUI interface will provide transparency into the agent's work, building trust and understanding.

By following this plan, we will create a powerful, autonomous troubleshooting tool that exemplifies the future of infrastructure management - intelligent, automated, and user-friendly.
