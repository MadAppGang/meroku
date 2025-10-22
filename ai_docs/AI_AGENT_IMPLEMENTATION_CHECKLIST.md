# AI Agent Implementation Checklist

## Phase 1: Foundation Setup ✓

### 1.1 Type Definitions
- [ ] Create `app/ai_agent_types.go`
  - [ ] Define `AgentContext` struct
  - [ ] Define `AgentStep` struct
  - [ ] Define `AgentState` struct
  - [ ] Define `StepType` enum (analyze, aws_cli, shell, file_read, file_write, terraform)
  - [ ] Define `StepStatus` enum (pending, running, completed, failed, skipped)
  - [ ] Define Bubble Tea message types:
    - [ ] `agentStartMsg`
    - [ ] `agentStepStartMsg`
    - [ ] `agentStepCompleteMsg`
    - [ ] `agentIterationCompleteMsg`
    - [ ] `agentCompleteMsg`
    - [ ] `agentErrorMsg`
    - [ ] `agentPausedMsg`
    - [ ] `agentResumedMsg`

### 1.2 Context Building
- [ ] Create `app/ai_agent_context.go`
  - [ ] `buildAgentContext(m *modernPlanModel) AgentContext`
    - [ ] Extract error messages from `m.applyState.logs`
    - [ ] Copy diagnostics from `m.applyState.diagnostics`
    - [ ] Get failed resources from `m.applyState.completed`
    - [ ] Detect environment from working directory
    - [ ] Get AWS profile from environment variables
    - [ ] Detect AWS region from terraform config
    - [ ] Get account ID from YAML config
    - [ ] Capture terraform state output
    - [ ] Store apply logs
  - [ ] `detectEnvironment() string` (dev/prod/staging from pwd)
  - [ ] `detectAWSProfile() string` (from AWS_PROFILE env var)
  - [ ] `detectAWSRegion() string` (from terraform vars or AWS config)
  - [ ] `getTerraformState() string` (run `terraform show`)

### 1.3 Utility Functions
- [ ] Create `app/ai_agent_utils.go`
  - [ ] `formatDuration(d time.Duration) string`
  - [ ] `truncateOutput(output string, maxLines int) string`
  - [ ] `parseCommandArgs(cmd string) []string`
  - [ ] `sanitizeOutput(output string) string`
  - [ ] `getStatusIcon(status StepStatus) string`
  - [ ] `getStatusColor(status StepStatus) lipgloss.Color`

---

## Phase 2: TUI Rendering ✓

### 2.1 Main View Renderer
- [ ] Create `app/ai_agent_tui.go`
  - [ ] `renderAgentView() string`
    - [ ] Render header with status and iteration counter
    - [ ] Render step list
    - [ ] Render expanded step details (if selected)
    - [ ] Render footer with controls
    - [ ] Handle loading state
    - [ ] Handle completion state (success/failure)

### 2.2 Component Renderers
- [ ] `renderAgentHeader() string`
  - [ ] Show "🤖 AI Agent - Autonomous Troubleshooting"
  - [ ] Show environment name
  - [ ] Show iteration counter (1/5, 2/5, etc.)
  - [ ] Show overall status (Running, Paused, Success, Failed)
  - [ ] Show elapsed time

- [ ] `renderAgentSteps() string`
  - [ ] Show scrollable list of steps
  - [ ] Color-code by status (green=completed, yellow=running, gray=pending, red=failed)
  - [ ] Show status icons (✅ ⏳ ⏸ ❌ ⏭)
  - [ ] Show step number and description
  - [ ] Show step duration
  - [ ] Show command (for executable steps)
  - [ ] Show brief output preview
  - [ ] Highlight selected step

- [ ] `renderAgentStepDetails() string`
  - [ ] Show expanded view of selected step
  - [ ] Show full command
  - [ ] Show full output (scrollable)
  - [ ] Show error details (if failed)
  - [ ] Show duration

- [ ] `renderAgentFooter() string`
  - [ ] Show controls based on state:
    - [ ] Running: `[↑↓] Navigate  [Space] Expand  [p] Pause  [q] Stop`
    - [ ] Paused: `[p] Resume  [q] Stop`
    - [ ] Complete: `[Enter] Return to menu  [d] View Details`
  - [ ] Show help hint: `[?] Help`

### 2.3 Loading and Completion States
- [ ] `renderAgentLoading() string`
  - [ ] Show spinner animation
  - [ ] Show "Initializing AI agent..." message

- [ ] `renderAgentComplete(success bool, message string) string`
  - [ ] Show success banner (✅) or failure banner (❌)
  - [ ] Show final message
  - [ ] Show total elapsed time
  - [ ] Show summary of changes made
  - [ ] Show next steps

---

## Phase 3: Execution Engine ✓

### 3.1 Agent Lifecycle
- [ ] Create `app/ai_agent_executor.go`
  - [ ] `startAgent() tea.Cmd`
    - [ ] Build AgentContext from current state
    - [ ] Initialize AgentState
    - [ ] Send `agentStartMsg`
    - [ ] Start first iteration
  - [ ] `runAgentIteration() tea.Cmd`
    - [ ] Call Claude API with context
    - [ ] Parse response into steps
    - [ ] Execute steps sequentially
    - [ ] Check if problem is solved
    - [ ] Decide whether to continue or stop
    - [ ] Send appropriate message

### 3.2 Step Execution
- [ ] `executeStep(step AgentStep) (output string, err error)`
  - [ ] Handle `StepAnalyze`: Return description (no execution)
  - [ ] Handle `StepAWSCLI`: Execute AWS CLI command
  - [ ] Handle `StepShell`: Execute shell command
  - [ ] Handle `StepFileRead`: Read file contents
  - [ ] Handle `StepFileWrite`: Write file contents
  - [ ] Handle `StepTerraform`: Execute terraform command
  - [ ] Capture output and errors
  - [ ] Set appropriate timeout
  - [ ] Handle command cancellation

### 3.3 Problem Resolution Detection
- [ ] `isProblemSolved() bool`
  - [ ] Re-run terraform plan to check for errors
  - [ ] Parse plan output
  - [ ] Return true if no errors detected
  - [ ] Return false if errors persist

### 3.4 Error Handling
- [ ] `handleStepError(step AgentStep, err error) StepAction`
  - [ ] Classify error (fatal vs recoverable)
  - [ ] Decide whether to retry, skip, or abort
  - [ ] Update step status accordingly

---

## Phase 4: Claude API Integration ✓

### 4.1 API Client
- [ ] Create `app/ai_agent_claude.go`
  - [ ] `callClaudeForPlan(ctx AgentContext) (plan string, err error)`
    - [ ] Build prompt with error context
    - [ ] Call Claude API (Messages API)
    - [ ] Parse response
    - [ ] Handle API errors
    - [ ] Handle rate limits
  - [ ] `parsePlanIntoSteps(plan string) []AgentStep`
    - [ ] Parse Claude's response format
    - [ ] Extract step type, description, command
    - [ ] Validate steps
    - [ ] Return structured steps

### 4.2 Prompt Engineering
- [ ] Design system prompt for autonomous agent
  - [ ] Define agent's role and capabilities
  - [ ] Define available tools (AWS CLI, shell, file ops)
  - [ ] Define output format expectations
  - [ ] Include safety guidelines
  - [ ] Include iteration limits

- [ ] Design user prompt format
  - [ ] Include error messages
  - [ ] Include environment context
  - [ ] Include AWS context
  - [ ] Include terraform state
  - [ ] Ask for troubleshooting plan
  - [ ] Request structured output

---

## Phase 5: Tool Execution ✓

### 5.1 Tool Executors
- [ ] Create `app/ai_agent_tools.go`
  - [ ] `executeAWSCLI(cmd string) (output string, err error)`
    - [ ] Parse AWS CLI command
    - [ ] Validate command safety
    - [ ] Execute with timeout
    - [ ] Capture output
    - [ ] Handle AWS errors

  - [ ] `executeShell(cmd string) (output string, err error)`
    - [ ] Validate command safety (no destructive operations)
    - [ ] Execute with timeout
    - [ ] Capture stdout and stderr
    - [ ] Handle errors

  - [ ] `executeFileRead(path string) (content string, err error)`
    - [ ] Validate path exists
    - [ ] Read file contents
    - [ ] Handle binary files
    - [ ] Limit file size

  - [ ] `executeFileWrite(path, content string) error`
    - [ ] Create backup before writing
    - [ ] Validate path is within project
    - [ ] Write contents
    - [ ] Set appropriate permissions

  - [ ] `executeTerraform(cmd string) (output string, err error)`
    - [ ] Parse terraform command
    - [ ] Validate command (only allow safe commands)
    - [ ] Execute with timeout
    - [ ] Capture output
    - [ ] Handle terraform errors

### 5.2 Safety Checks
- [ ] `validateCommand(cmd string) error`
  - [ ] Block destructive commands (rm -rf, etc.)
  - [ ] Block commands that modify system files
  - [ ] Allow terraform, AWS CLI, and safe shell commands
  - [ ] Return error if command is unsafe

- [ ] `validateFilePath(path string) error`
  - [ ] Ensure path is within project directory
  - [ ] Block system paths (/etc, /usr, etc.)
  - [ ] Block hidden files (.env secrets, etc.)
  - [ ] Return error if path is unsafe

---

## Phase 6: Integration with Error Flow ✓

### 6.1 Modify Existing Files
- [ ] Update `app/terraform_plan_modern_tui.go`
  - [ ] Add `aiAgentView` to `viewMode` enum
  - [ ] Add `agentState *AgentState` field to `modernPlanModel`
  - [ ] Handle 's' key press in `Update()`:
    ```go
    case msg.String() == "s":
        if m.currentView == applyView &&
           m.applyState != nil &&
           m.applyState.applyComplete &&
           m.applyState.hasErrors &&
           isAIHelperAvailable() {
            m.currentView = aiAgentView
            return m, m.startAgent()
        }
    ```
  - [ ] Add case for `aiAgentView` in `View()`:
    ```go
    case aiAgentView:
        return m.renderAgentView()
    ```
  - [ ] Handle agent messages in `Update()`:
    - [ ] `agentStartMsg`
    - [ ] `agentStepStartMsg`
    - [ ] `agentStepCompleteMsg`
    - [ ] `agentIterationCompleteMsg`
    - [ ] `agentCompleteMsg`
    - [ ] `agentErrorMsg`
    - [ ] `agentPausedMsg`
    - [ ] `agentResumedMsg`
  - [ ] Update footer rendering:
    ```go
    if m.applyState.hasErrors && isAIHelperAvailable() {
        help += "[a] Ask AI (suggestions)  [s] Solve with AI (auto-fix)  "
    }
    ```

- [ ] Update `app/terraform_apply_tui.go`
  - [ ] (No changes needed - types are in modernPlanModel)

### 6.2 Keyboard Navigation
- [ ] In `aiAgentView`:
  - [ ] `↑`/`↓`: Navigate steps
  - [ ] `Space`/`Enter`: Expand/collapse selected step
  - [ ] `p`: Pause/resume agent
  - [ ] `q`: Stop agent and return to apply view
  - [ ] `Esc`: Return to apply view (if complete)
  - [ ] `?`: Show help

### 6.3 State Transitions
- [ ] `applyView` (error state) → `aiAgentView` (on 's' key)
- [ ] `aiAgentView` (running) → `aiAgentView` (paused) (on 'p' key)
- [ ] `aiAgentView` (paused) → `aiAgentView` (running) (on 'p' key)
- [ ] `aiAgentView` (complete) → `dashboardView` (on Enter)
- [ ] `aiAgentView` (stopped) → `applyView` (on 'q' key)

---

## Phase 7: Testing ✓

### 7.1 Unit Tests
- [ ] Create `app/ai_agent_test.go`
  - [ ] Test `buildAgentContext()` with mock apply state
  - [ ] Test `executeStep()` with different step types
  - [ ] Test `validateCommand()` with safe and unsafe commands
  - [ ] Test `validateFilePath()` with valid and invalid paths
  - [ ] Test `parsePlanIntoSteps()` with sample Claude responses
  - [ ] Test context detection functions

### 7.2 Integration Tests
- [ ] Test full agent flow with mock terraform errors:
  - [ ] IAM permission error
  - [ ] S3 bucket name conflict
  - [ ] VPC CIDR overlap
  - [ ] Security group rule error
- [ ] Test pause/resume functionality
- [ ] Test stop functionality
- [ ] Test iteration limit
- [ ] Test timeout handling

### 7.3 End-to-End Tests
- [ ] Manual testing with real terraform project:
  - [ ] Run terraform apply
  - [ ] Introduce deliberate error (remove IAM permission)
  - [ ] Let it fail
  - [ ] Press 's' to start agent
  - [ ] Verify agent detects and fixes issue
  - [ ] Verify terraform apply succeeds

---

## Phase 8: Polish and Documentation ✓

### 8.1 Error Messages
- [ ] Add clear error messages for:
  - [ ] Claude API failures
  - [ ] Command execution failures
  - [ ] Timeout errors
  - [ ] Permission errors
  - [ ] Unsafe command detection

### 8.2 User Feedback
- [ ] Add progress indicators:
  - [ ] Spinner for running steps
  - [ ] Progress bar for iterations
  - [ ] Status icons for step completion
- [ ] Add helpful hints:
  - [ ] Explain what agent is doing
  - [ ] Show estimated time remaining
  - [ ] Suggest manual steps if agent fails

### 8.3 Documentation
- [ ] Update `README.md`:
  - [ ] Add "AI Autonomous Troubleshooting" section
  - [ ] Explain how to use the agent
  - [ ] Show example workflow
  - [ ] Document requirements (ANTHROPIC_API_KEY)
- [ ] Update `CLAUDE.md`:
  - [ ] Document agent architecture
  - [ ] Explain integration points
  - [ ] Add troubleshooting guide
- [ ] Create `ai_docs/AI_AGENT_USER_GUIDE.md`:
  - [ ] Step-by-step usage instructions
  - [ ] Common scenarios and fixes
  - [ ] FAQ

### 8.4 Safety Features
- [ ] Add confirmation prompts for destructive operations
- [ ] Implement dry-run mode (preview changes before executing)
- [ ] Add logging of all agent actions
- [ ] Add rollback capability (undo agent changes)

---

## Phase 9: Performance and Reliability ✓

### 9.1 Performance
- [ ] Optimize Claude API calls (batch requests if possible)
- [ ] Cache command outputs to avoid re-execution
- [ ] Add timeout controls for long-running commands
- [ ] Stream command output in real-time

### 9.2 Reliability
- [ ] Add retry logic for transient failures
- [ ] Implement exponential backoff for API rate limits
- [ ] Handle network errors gracefully
- [ ] Save agent state to disk (resume after crash)

### 9.3 Monitoring
- [ ] Log all agent actions to file
- [ ] Track success/failure rates
- [ ] Collect metrics on common error types
- [ ] Monitor API usage and costs

---

## Success Criteria

### Must Have (MVP)
- [x] User can press 's' on error screen to start agent
- [ ] Agent can analyze terraform errors
- [ ] Agent can execute AWS CLI commands
- [ ] Agent can execute shell commands
- [ ] Agent can retry terraform apply
- [ ] Agent shows real-time progress
- [ ] Agent returns to menu after completion
- [ ] Clear visual distinction from "Ask AI" feature

### Should Have (v1.0)
- [ ] User can pause/resume agent
- [ ] User can expand steps to see details
- [ ] Agent can read/write files
- [ ] Agent respects iteration limit
- [ ] Agent handles timeouts gracefully
- [ ] Comprehensive error messages
- [ ] Safety validation for all commands

### Nice to Have (Future)
- [ ] Dry-run mode
- [ ] Rollback capability
- [ ] Agent learns from past fixes
- [ ] Multi-step confirmation mode
- [ ] Integration with git (auto-commit fixes)
- [ ] Slack/email notifications for long-running fixes

---

## Implementation Order

1. **Start Here:** Phase 1 (Foundation)
2. **Then:** Phase 4 (Claude API) - Test API integration early
3. **Then:** Phase 2 (TUI) - Build visual interface
4. **Then:** Phase 3 (Executor) - Connect API to UI
5. **Then:** Phase 5 (Tools) - Implement tool execution
6. **Then:** Phase 6 (Integration) - Wire into error flow
7. **Then:** Phase 7 (Testing) - Validate everything works
8. **Finally:** Phase 8 & 9 (Polish & Performance)

---

## Current Status

- [x] Plan created
- [x] Architecture documented
- [x] Integration approach defined
- [ ] Implementation started

**Next Step:** Create `app/ai_agent_types.go` and begin Phase 1.
