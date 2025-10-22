package main

import (
	"context"
	"fmt"
	"time"
)

// AgentIteration represents a single think/act/observe cycle in the ReAct pattern
type AgentIteration struct {
	Number      int           `json:"number"`
	Thought     string        `json:"thought"`      // Agent's reasoning about what to do
	Action      string        `json:"action"`       // Tool type: aws_cli, shell, file_edit, terraform_apply
	Command     string        `json:"command"`      // Exact command or operation
	Output      string        `json:"output"`       // Command result/observation
	Status      string        `json:"status"`       // running, success, failed
	Duration    time.Duration `json:"duration"`
	Timestamp   time.Time     `json:"timestamp"`
	ErrorDetail string        `json:"error_detail"` // Detailed error if failed
}

// AgentState tracks the full execution state
type AgentState struct {
	Iterations       []AgentIteration `json:"iterations"`
	CurrentThinking  bool             `json:"current_thinking"`
	IsComplete       bool             `json:"is_complete"`
	FinalOutcome     string           `json:"final_outcome"`      // "success" or "failed"
	TotalDuration    time.Duration    `json:"total_duration"`
	IterationLimit   int              `json:"iteration_limit"`    // Maximum iterations to prevent infinite loops
	Context          *AgentContext    `json:"context"`
}

// AgentContext provides the environment and problem details to the agent
type AgentContext struct {
	Operation            string            `json:"operation"`              // "terraform_apply", "terraform_destroy", etc.
	Environment          string            `json:"environment"`            // "dev", "prod", etc.
	AWSProfile           string            `json:"aws_profile"`
	AWSRegion            string            `json:"aws_region"`
	WorkingDir           string            `json:"working_dir"`
	InitialError         string            `json:"initial_error"`          // The error that triggered the agent
	ResourceErrors       []string          `json:"resource_errors"`        // Multiple error messages if available
	StructuredErrorsJSON string            `json:"structured_errors_json"` // JSON-formatted error data
	AdditionalInfo       map[string]string `json:"additional_info"`        // Extra context (e.g., cluster name, service name)
}

// AgentUpdate is sent to the TUI to update the display
type AgentUpdate struct {
	Type       string          `json:"type"` // "thinking", "action_start", "action_complete", "finished", "error"
	Iteration  *AgentIteration `json:"iteration,omitempty"`
	Message    string          `json:"message,omitempty"`
	IsComplete bool            `json:"is_complete"`
	Success    bool            `json:"success"`
}

// AIAgent is the main agent controller
type AIAgent struct {
	state       *AgentState
	updateChan  chan AgentUpdate
	executor    *AgentExecutor
	llmClient   *AgentLLMClient
	ctx         context.Context
	cancelFunc  context.CancelFunc
}

// NewAIAgent creates a new autonomous agent
func NewAIAgent(agentContext *AgentContext, updateChan chan AgentUpdate) (*AIAgent, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create LLM client
	llmClient, err := NewAgentLLMClient()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	// Create executor
	executor := NewAgentExecutor(agentContext)

	agent := &AIAgent{
		state: &AgentState{
			Iterations:       []AgentIteration{},
			CurrentThinking:  false,
			IsComplete:       false,
			FinalOutcome:     "",
			IterationLimit:   20, // Prevent infinite loops
			Context:          agentContext,
		},
		updateChan: updateChan,
		executor:   executor,
		llmClient:  llmClient,
		ctx:        ctx,
		cancelFunc: cancel,
	}

	return agent, nil
}

// Start begins the ReAct loop
func (a *AIAgent) Start() error {
	startTime := time.Now()
	defer func() {
		a.state.TotalDuration = time.Since(startTime)
	}()

	// Send initial update
	a.sendUpdate(AgentUpdate{
		Type:    "started",
		Message: "AI Agent started - analyzing problem...",
	})

	// Main ReAct loop
	for iteration := 1; iteration <= a.state.IterationLimit; iteration++ {
		// Check if cancelled
		select {
		case <-a.ctx.Done():
			a.state.IsComplete = true
			a.state.FinalOutcome = "cancelled"
			a.sendUpdate(AgentUpdate{
				Type:       "finished",
				IsComplete: true,
				Success:    false,
				Message:    "Agent cancelled by user",
			})
			return fmt.Errorf("cancelled by user")
		default:
		}

		// Create iteration
		iter := AgentIteration{
			Number:    iteration,
			Timestamp: time.Now(),
			Status:    "running",
		}

		// Step 1: Think - LLM decides what to do next
		a.state.CurrentThinking = true
		a.sendUpdate(AgentUpdate{
			Type:    "thinking",
			Message: fmt.Sprintf("Iteration %d: Analyzing situation...", iteration),
		})

		thought, action, command, err := a.think()
		if err != nil {
			iter.Status = "failed"
			iter.ErrorDetail = fmt.Sprintf("Thinking failed: %v", err)
			a.state.Iterations = append(a.state.Iterations, iter)
			a.sendUpdate(AgentUpdate{
				Type:      "error",
				Iteration: &iter,
				Message:   "Failed to generate next action",
			})
			return fmt.Errorf("thinking failed: %w", err)
		}

		iter.Thought = thought
		iter.Action = action
		iter.Command = command

		// Check if agent thinks it's done
		if action == "complete" {
			iter.Status = "success"
			iter.Output = "Problem solved!"
			a.state.Iterations = append(a.state.Iterations, iter)
			a.state.IsComplete = true
			a.state.FinalOutcome = "success"
			a.sendUpdate(AgentUpdate{
				Type:       "finished",
				Iteration:  &iter,
				IsComplete: true,
				Success:    true,
				Message:    "Problem resolved successfully!",
			})
			return nil
		}

		// Step 2: Act - Execute the action
		a.state.CurrentThinking = false
		a.sendUpdate(AgentUpdate{
			Type:      "action_start",
			Iteration: &iter,
			Message:   fmt.Sprintf("Executing: %s", command),
		})

		actionStart := time.Now()
		output, err := a.act(action, command)
		iter.Duration = time.Since(actionStart)

		if err != nil {
			iter.Status = "failed"
			iter.Output = output // May contain partial output
			iter.ErrorDetail = err.Error()
		} else {
			iter.Status = "success"
			iter.Output = output
		}

		// Step 3: Observe - Add iteration to history
		a.state.Iterations = append(a.state.Iterations, iter)
		a.sendUpdate(AgentUpdate{
			Type:      "action_complete",
			Iteration: &iter,
			Message:   fmt.Sprintf("Completed in %v", iter.Duration),
		})

		// If action failed but agent wants to continue, allow it
		// The LLM will see the failure in the next iteration's context
	}

	// Reached iteration limit
	a.state.IsComplete = true
	a.state.FinalOutcome = "failed"
	a.sendUpdate(AgentUpdate{
		Type:       "finished",
		IsComplete: true,
		Success:    false,
		Message:    fmt.Sprintf("Reached maximum iterations (%d) without resolving the problem", a.state.IterationLimit),
	})

	return fmt.Errorf("reached iteration limit without resolution")
}

// think uses the LLM to decide the next action
func (a *AIAgent) think() (thought, action, command string, err error) {
	// Build conversation history for context
	history := a.buildHistory()

	// Call LLM
	response, err := a.llmClient.GetNextAction(a.ctx, a.state.Context, history)
	if err != nil {
		return "", "", "", err
	}

	return response.Thought, response.Action, response.Command, nil
}

// act executes the chosen action
func (a *AIAgent) act(action, command string) (string, error) {
	switch action {
	case "aws_cli":
		return a.executor.ExecuteAWSCLI(a.ctx, command)
	case "shell":
		return a.executor.ExecuteShell(a.ctx, command)
	case "file_edit":
		return a.executor.ExecuteFileEdit(a.ctx, command)
	case "terraform_apply":
		return a.executor.ExecuteTerraformApply(a.ctx, command)
	case "terraform_plan":
		return a.executor.ExecuteTerraformPlan(a.ctx, command)
	case "web_search":
		return a.executor.ExecuteWebSearch(a.ctx, command)
	default:
		return "", fmt.Errorf("unknown action type: %s", action)
	}
}

// buildHistory creates a string representation of previous iterations for LLM context
func (a *AIAgent) buildHistory() string {
	if len(a.state.Iterations) == 0 {
		return "No previous actions taken."
	}

	var history string
	for _, iter := range a.state.Iterations {
		history += fmt.Sprintf("\n--- Iteration %d ---\n", iter.Number)
		history += fmt.Sprintf("THOUGHT: %s\n", iter.Thought)
		history += fmt.Sprintf("ACTION: %s\n", iter.Action)
		history += fmt.Sprintf("COMMAND: %s\n", iter.Command)
		history += fmt.Sprintf("OUTPUT: %s\n", truncateOutput(iter.Output, 500))
		if iter.Status == "failed" {
			history += fmt.Sprintf("ERROR: %s\n", iter.ErrorDetail)
		}
		history += fmt.Sprintf("STATUS: %s\n", iter.Status)
	}

	return history
}

// sendUpdate sends an update to the TUI
func (a *AIAgent) sendUpdate(update AgentUpdate) {
	select {
	case a.updateChan <- update:
	case <-a.ctx.Done():
		// Context cancelled, stop sending
	}
}

// Stop cancels the agent execution
func (a *AIAgent) Stop() {
	a.cancelFunc()
}

// GetState returns the current agent state (for debugging/inspection)
func (a *AIAgent) GetState() *AgentState {
	return a.state
}

// truncateOutput limits output length for LLM context
func truncateOutput(output string, maxLen int) string {
	if len(output) <= maxLen {
		return output
	}
	return output[:maxLen] + "\n... (output truncated)"
}
