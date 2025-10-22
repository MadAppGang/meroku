---
description: Implement features with multi-agent quality workflow
---

## Task

Implement a feature using a multi-agent quality assurance workflow with the following stages:

### Workflow Diagram

```
┌──────────────────────────────────────────────────┐
│  Stage 1: PLANNING                               │
│  Agent: full-stack-terraform-architect           │
│  Output: Implementation plan                     │
└────────────────┬─────────────────────────────────┘
                 │
                 ▼
         ┌───────────────┐
         │ User Approval │ ──► Reject ──┐
         └───────┬───────┘              │
                 │ Approve              │
                 ▼                      │
┌──────────────────────────────────────┼───────────┐
│  Stage 2: IMPLEMENTATION             │           │
│  Agent: full-stack-terraform-architect◄──────────┘
│  Output: Code implementation                     │
└────────────────┬─────────────────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────────────────┐
│  Stage 3: CODE REVIEW LOOP (max 5 iterations)    │
│  Agent: senior-code-reviewer                     │
│         ▼                                        │
│    ┌─────────┐         Has Issues               │
│    │ Review  │──────────────┐                    │
│    └────┬────┘              │                    │
│         │ Approved          ▼                    │
│         │            ┌──────────────┐            │
│         │            │ Architect    │            │
│         │            │ Fix Issues   │            │
│         │            └──────┬───────┘            │
│         │                   │                    │
│         │                   └──► Re-review       │
└─────────┼──────────────────────────────────────┘
          │
          ▼
┌──────────────────────────────────────────────────┐
│  Stage 4: QA VALIDATION LOOP (until passing)     │
│  Agent: qa-implementation-validator              │
│         ▼                                        │
│    ┌─────────┐         Tests Fail               │
│    │ Run QA  │──────────────┐                    │
│    └────┬────┘              │                    │
│         │ All Pass          ▼                    │
│         │            ┌──────────────┐            │
│         │            │ Architect    │            │
│         │            │ Fix + Review │───┐        │
│         │            └──────────────┘   │        │
│         │                   ▲           │        │
│         │                   └───────────┘        │
└─────────┼──────────────────────────────────────┘
          │
          ▼
    ✅ COMPLETE
```

### Stage Descriptions

### Stage 1: Planning
1. Launch **full-stack-terraform-architect** agent to create implementation plan
2. Wait for plan to be generated
3. Present plan to user and request approval
4. If user rejects, refine plan and go back to step 3
5. If user approves, proceed to Stage 2

### Stage 2: Implementation
1. Launch **full-stack-terraform-architect** agent to implement the approved plan
2. Include the approved plan in the prompt
3. Wait for implementation to complete
4. Proceed to Stage 3

### Stage 3: Code Review Loop
1. Launch **senior-code-reviewer** agent to review the implementation
2. Wait for review feedback
3. If reviewer is satisfied:
   - Proceed to Stage 4
4. If reviewer has feedback:
   - Launch **full-stack-terraform-architect** agent with review feedback
   - Wait for fixes to be implemented
   - Go back to step 1 (send to reviewer again)

### Stage 4: QA Validation Loop
1. Launch **qa-implementation-validator** agent to validate tests
2. Wait for QA validation results
3. If all tests pass and QA is satisfied:
   - Mark feature as COMPLETE
   - Congratulate the user
   - End workflow
4. If tests fail or issues found:
   - Launch **full-stack-terraform-architect** agent with QA feedback
   - Include both original plan and QA issues
   - Wait for fixes to be implemented
   - Go back to Stage 3 (send through code review again)

### Important Guidelines

- **Wait between stages**: Each agent runs independently, wait for completion before next stage
- **Track iteration count**: Stop after 5 review iterations and ask user for guidance
- **Preserve context**: Always include original plan + all feedback in subsequent prompts
- **Clear state transitions**: Explicitly state which stage we're in
- **User visibility**: Show user what's happening at each stage

### State Tracking

Maintain a state object throughout the workflow:

```
WORKFLOW_STATE:
  feature: "$ARGUMENTS"
  current_stage: "planning" | "implementation" | "code_review" | "qa_validation" | "complete"
  plan: <approved plan text>
  review_iteration: 0
  qa_iteration: 0
  feedback_history: [
    { stage: "code_review", iteration: 1, feedback: "..." },
    { stage: "qa", iteration: 1, feedback: "..." }
  ]
```

Before each agent call, show current state:
```
📍 Current Stage: Stage 3 - Code Review (Iteration 2/5)
📋 Feature: Cross-account ECR dropdown
🔄 Previous Feedback: <summary>
```

### Agent Prompts

**Planning Prompt (full-stack-terraform-architect):**
```
Plan implementation for: $ARGUMENTS

Create a comprehensive implementation plan covering:
- Go backend changes (models, API endpoints, migrations)
- Terraform module updates (resources, variables, outputs)
- React frontend changes (components, types, API integration)
- Handlebars template updates
- Testing strategy

Provide detailed checklist with file paths and code changes.
```

**Implementation Prompt (full-stack-terraform-architect):**
```
Implement the following approved plan:

[APPROVED_PLAN]

Follow the checklist exactly and implement all changes across the stack.
```

**Code Review Prompt (senior-code-reviewer):**
```
Review the implementation that was just completed.

Original Plan:
[PLAN]

Check for:
- Code quality and best practices
- Security vulnerabilities
- Architectural consistency
- Error handling
- TypeScript/Go type safety

Provide specific feedback or approve if satisfied.
```

**QA Validation Prompt (qa-implementation-validator):**
```
Validate the implementation and test coverage.

Original Plan:
[PLAN]

Tasks:
1. Verify all tests exist and run
2. Check test coverage for new code
3. Run tests and report results
4. Validate against requirements

If tests fail, provide detailed error analysis.
```

### Exit Conditions

**Success Exit:**
- ✅ QA agent reports all tests passing
- ✅ No implementation issues found
- ✅ Requirements validated
- ➡️ Display success message and workflow summary

**Manual Exit (after 5 iterations):**
- ⚠️ Review or QA loop exceeds 5 iterations
- ➡️ Show summary of issues
- ➡️ Ask user: "Continue automated loop?" or "Manual intervention needed?"
- ➡️ If manual: Pause and wait for user fixes

**Error Exit:**
- ❌ Agent fails to complete task
- ❌ Cannot parse agent response
- ❌ File access errors
- ➡️ Report error and current state
- ➡️ Ask user how to proceed

### Feature Argument

$ARGUMENTS - Description of feature to implement (e.g., "cross-account ECR dropdown with bidirectional YAML updates")

### Example Usage

```
/implement cross-account ECR dropdown with environment selector
```

This will:
1. Plan the feature with full-stack-terraform-architect
2. Show you the plan for approval
3. Implement across Go backend, Terraform modules, React frontend
4. Iterate with code reviewer until quality standards met
5. Validate with QA until all tests pass
6. Report completion with summary
