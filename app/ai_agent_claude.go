package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AgentLLMClient handles LLM interactions for the agent
type AgentLLMClient struct {
	client *anthropic.Client
}

// AgentResponse represents the LLM's decision for the next action
type AgentResponse struct {
	Thought string // The agent's reasoning
	Action  string // Tool to use: aws_cli, shell, file_edit, terraform_apply, complete
	Command string // The exact command to execute
}

// NewAgentLLMClient creates a new LLM client for the agent
func NewAgentLLMClient() (*AgentLLMClient, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &AgentLLMClient{
		client: &client,
	}, nil
}

// GetNextAction asks the LLM to decide the next action based on context and history
func (c *AgentLLMClient) GetNextAction(ctx context.Context, agentCtx *AgentContext, history string) (*AgentResponse, error) {
	prompt := c.buildPrompt(agentCtx, history)

	// Call Claude with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	message, err := c.client.Messages.New(timeoutCtx, anthropic.MessageNewParams{
		Model:     "claude-sonnet-4-5-20250929", // Latest Claude Sonnet 4.5 - best for coding and agents
		MaxTokens: 2048,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})

	if err != nil {
		return nil, fmt.Errorf("LLM API call failed: %w", err)
	}

	if len(message.Content) == 0 {
		return nil, fmt.Errorf("empty response from LLM")
	}

	responseText := message.Content[0].Text

	// Parse the response
	return c.parseResponse(responseText)
}

// buildPrompt constructs the ReAct-style prompt
func (c *AgentLLMClient) buildPrompt(agentCtx *AgentContext, history string) string {
	systemContext := fmt.Sprintf(`You are an autonomous AWS infrastructure troubleshooting agent for the Meroku platform. Your goal is to analyze and fix infrastructure deployment errors.

═══════════════════════════════════════════════════════════════════
ABOUT MEROKU
═══════════════════════════════════════════════════════════════════

Meroku is an Infrastructure-as-Code automation tool that simplifies AWS infrastructure management using:
- YAML configuration files (human-friendly)
- Terraform modules (reusable infrastructure components)
- Handlebars templates (dynamic Terraform generation)
- Go CLI with Bubble Tea TUI (interactive management)

PROJECT STRUCTURE (User's Working Directory):
/Users/jack/merokudemo1/      # PROJECT ROOT (current working directory)
├── dev.yaml                  # Development environment config (SOURCE OF TRUTH)
├── prod.yaml                 # Production environment config
├── staging.yaml              # Staging environment config (if exists)
├── env/                      # Generated Terraform files (DO NOT EDIT MANUALLY)
│   ├── dev/                 # Generated for dev environment
│   │   ├── main.tf          # Auto-generated from dev.yaml
│   │   ├── variables.tf
│   │   └── terraform.tfstate
│   └── prod/                # Generated for prod environment
│       ├── main.tf          # Auto-generated from prod.yaml
│       └── terraform.tfstate
└── meroku                    # Meroku CLI binary (run from this directory)

HOW MEROKU WORKS:
1. User edits YAML config: dev.yaml in root (defines infrastructure declaratively)
2. User runs: ./meroku --generate --env dev (regenerates Terraform from YAML)
3. Meroku generates: dev.yaml → env/dev/main.tf (applies Handlebars templates)
4. User runs: ./meroku --apply --env dev (applies Terraform to AWS)
5. Terraform creates/updates AWS resources

CRITICAL FILE RULES:
✓ EDIT: dev.yaml, prod.yaml, staging.yaml (source configuration in ROOT)
✓ READ: env/dev/*.tf, env/prod/*.tf (generated, for diagnostics only)
✗ NEVER EDIT: env/*/*.tf files directly (will be overwritten on next regeneration)

If you need to fix infrastructure:
1. Edit {environment}.yaml in ROOT (e.g., dev.yaml)
2. Regenerate Terraform: ./meroku --generate --env {environment}
3. Apply changes: ./meroku --apply --env {environment}
   OR: cd env/{environment} && terraform apply

═══════════════════════════════════════════════════════════════════
CURRENT CONTEXT
═══════════════════════════════════════════════════════════════════

- Operation: %s
- Environment: %s
- AWS Profile: %s
- AWS Region: %s
- Working Directory: %s
- YAML Config: %s.yaml (in root directory)

INITIAL ERROR:
%s

COMMON MEROKU RESOURCES:
- ECS Cluster: %s_cluster_%s
- ECS Service: %s_service_%s
- ALB: %s_alb_%s
- RDS Instance: %s_db_%s
- ECR Repository: %s_repository_%s

═══════════════════════════════════════════════════════════════════
AVAILABLE TOOLS
═══════════════════════════════════════════════════════════════════

1. aws_cli - Run AWS CLI commands (e.g., describe resources, check status)
2. shell - Run shell commands (e.g., grep, find, cat files)
3. file_edit - Edit configuration files (format: FILE:path|OLD:old_text|NEW:new_text)
4. terraform_plan - Run terraform plan to preview changes
5. terraform_apply - Apply terraform changes (use carefully!)
6. web_search - Search the web for documentation and solutions
7. complete - Mark the problem as solved (use when fixed)

═══════════════════════════════════════════════════════════════════
SYSTEMATIC DEBUGGING APPROACH
═══════════════════════════════════════════════════════════════════

1. UNDERSTAND: Check AWS resource status (aws ecs describe-services, etc.)
2. INVESTIGATE: Review configuration (cat {env}.yaml, cat env/{env}/main.tf)
3. RESEARCH: Search for error messages if unclear (web_search)
4. IDENTIFY: Determine root cause (IAM? Networking? Configuration?)
5. FIX: Edit YAML config (file_edit {env}.yaml)
6. REGENERATE: Run ./meroku --generate --env {env} (updates env/{env}/main.tf)
7. APPLY: Run ./meroku --apply --env {env} OR cd env/{env} && terraform apply
8. VERIFY: Check resource status again to confirm fix

COMMON ISSUES IN MEROKU:
- ECS tasks fail to start → Check IAM roles, ECR pull permissions, task definition
- RDS connection failures → Check security groups, VPC configuration
- ALB health checks failing → Check target group configuration, service health
- Terraform state conflicts → Check env/{env}/terraform.tfstate
- Missing resources → Check {env}.yaml has correct schema_version
- Cross-account ECR → Check ecr_strategy and ecr_trusted_accounts in YAML
- Config changes not applied → Must regenerate: ./meroku --generate --env {env}

═══════════════════════════════════════════════════════════════════
TASK
═══════════════════════════════════════════════════════════════════

Analyze the situation and decide on ONE action to take next

Respond in this EXACT format:
THOUGHT: [Your reasoning about what to investigate or fix next]
ACTION: [One of: aws_cli, shell, file_edit, terraform_plan, terraform_apply, web_search, complete]
COMMAND: [Exact command to run or search query]

EXAMPLES:

Example 1 - Checking ECS service:
THOUGHT: The error mentions ECS service deployment failure. I should check the service status first to see if tasks are running and what errors might be present.
ACTION: aws_cli
COMMAND: aws ecs describe-services --cluster %s_cluster_%s --services %s_service_%s --region %s

Example 2 - Checking logs:
THOUGHT: The service shows no running tasks. I need to check recent task failures to see why tasks aren't starting.
ACTION: aws_cli
COMMAND: aws ecs describe-tasks --cluster %s_cluster_%s --tasks $(aws ecs list-tasks --cluster %s_cluster_%s --service %s_service_%s --desired-status STOPPED --region %s --query 'taskArns[0]' --output text) --region %s

Example 3 - Editing YAML config:
THOUGHT: The task definition is missing an IAM role configuration. I need to add the IAM role to the YAML configuration and regenerate terraform.
ACTION: file_edit
COMMAND: FILE:%s.yaml|OLD:enable_ecs_task_role: false|NEW:enable_ecs_task_role: true

Example 4 - Regenerating after YAML edit:
THOUGHT: I've updated the YAML configuration. Now I need to regenerate the terraform files so the changes take effect.
ACTION: shell
COMMAND: cd %s && ./meroku --generate --env %s

Example 5 - Searching for documentation:
THOUGHT: The error message mentions "InvalidParameterException" but I'm not sure what parameter is invalid. Let me search for AWS documentation on this error.
ACTION: web_search
COMMAND: AWS ECS InvalidParameterException task definition

Example 6 - Applying fix:
THOUGHT: I've updated and regenerated the terraform configuration. Now I need to apply the changes to AWS.
ACTION: terraform_apply
COMMAND: terraform apply

Example 7 - Problem solved:
THOUGHT: The ECS service is now running with 1 healthy task. The deployment error has been resolved.
ACTION: complete
COMMAND: Success: ECS service is healthy with running tasks

IMPORTANT:
- Only take ONE action per iteration
- Always explain your reasoning in THOUGHT
- Use specific resource names from the context
- For file edits, use the exact format: FILE:path|OLD:old_text|NEW:new_text
- CRITICAL: After editing YAML files, ALWAYS regenerate: ./meroku --generate --env {environment}
- Mark as complete only when you've verified the fix worked
- If stuck, try a different approach or ask for more information`,
		agentCtx.Operation,
		agentCtx.Environment,
		agentCtx.AWSProfile,
		agentCtx.AWSRegion,
		agentCtx.WorkingDir,
		agentCtx.Environment, // YAML Config: {env}.yaml (in root directory)
		agentCtx.InitialError,
		// COMMON MEROKU RESOURCES section
		agentCtx.Environment, agentCtx.Environment, // ECS Cluster
		agentCtx.Environment, agentCtx.Environment, // ECS Service
		agentCtx.Environment, agentCtx.Environment, // ALB
		agentCtx.Environment, agentCtx.Environment, // RDS Instance
		agentCtx.Environment, agentCtx.Environment, // ECR Repository
		// Example 1 - Checking ECS service
		agentCtx.Environment, agentCtx.Environment, agentCtx.Environment, agentCtx.Environment, agentCtx.AWSRegion,
		// Example 2 - Checking logs
		agentCtx.Environment, agentCtx.Environment, agentCtx.Environment, agentCtx.Environment, agentCtx.Environment, agentCtx.Environment, agentCtx.AWSRegion, agentCtx.AWSRegion,
		// Example 3 - Editing YAML config
		agentCtx.Environment,
		// Example 4 - Regenerating after YAML edit
		agentCtx.WorkingDir, agentCtx.Environment,
	)

	if history != "" {
		systemContext += fmt.Sprintf("\n\nPREVIOUS ACTIONS:\n%s", history)
	} else {
		systemContext += "\n\nThis is your first action. Start by investigating the error."
	}

	return systemContext
}

// parseResponse extracts THOUGHT, ACTION, and COMMAND from the LLM response
func (c *AgentLLMClient) parseResponse(response string) (*AgentResponse, error) {
	// Use regex to extract sections
	thoughtRegex := regexp.MustCompile(`(?i)THOUGHT:\s*(.+?)(?:\n|$)`)
	actionRegex := regexp.MustCompile(`(?i)ACTION:\s*(\w+)`)
	commandRegex := regexp.MustCompile(`(?i)COMMAND:\s*(.+?)(?:\n\n|$)`)

	thoughtMatch := thoughtRegex.FindStringSubmatch(response)
	actionMatch := actionRegex.FindStringSubmatch(response)
	commandMatch := commandRegex.FindStringSubmatch(response)

	if len(thoughtMatch) < 2 || len(actionMatch) < 2 || len(commandMatch) < 2 {
		// Fallback: try line-by-line parsing
		return c.parseResponseFallback(response)
	}

	thought := strings.TrimSpace(thoughtMatch[1])
	action := strings.ToLower(strings.TrimSpace(actionMatch[1]))
	command := strings.TrimSpace(commandMatch[1])

	// Validate action
	validActions := map[string]bool{
		"aws_cli":         true,
		"shell":           true,
		"file_edit":       true,
		"terraform_plan":  true,
		"terraform_apply": true,
		"web_search":      true,
		"complete":        true,
	}

	if !validActions[action] {
		return nil, fmt.Errorf("invalid action: %s (must be one of: aws_cli, shell, file_edit, terraform_plan, terraform_apply, web_search, complete)", action)
	}

	return &AgentResponse{
		Thought: thought,
		Action:  action,
		Command: command,
	}, nil
}

// parseResponseFallback uses line-by-line parsing as a backup
func (c *AgentLLMClient) parseResponseFallback(response string) (*AgentResponse, error) {
	lines := strings.Split(response, "\n")
	var thought, action, command string
	var currentSection string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(strings.ToUpper(line), "THOUGHT:") {
			currentSection = "thought"
			thought = strings.TrimSpace(strings.TrimPrefix(line, "THOUGHT:"))
			continue
		}

		if strings.HasPrefix(strings.ToUpper(line), "ACTION:") {
			currentSection = "action"
			action = strings.TrimSpace(strings.TrimPrefix(strings.ToUpper(line), "ACTION:"))
			action = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "ACTION:")))
			continue
		}

		if strings.HasPrefix(strings.ToUpper(line), "COMMAND:") {
			currentSection = "command"
			// Remove case-insensitive prefix
			upper := strings.ToUpper(line)
			if strings.HasPrefix(upper, "COMMAND:") {
				command = strings.TrimSpace(line[8:]) // Skip "COMMAND:" (8 chars)
			}
			continue
		}

		// Append to current section if we're in multi-line content
		if currentSection == "thought" && line != "" && !strings.Contains(line, "ACTION:") {
			thought += " " + line
		} else if currentSection == "command" && line != "" && !strings.Contains(line, "THOUGHT:") {
			command += "\n" + line
		}
	}

	if thought == "" || action == "" || command == "" {
		return nil, fmt.Errorf("failed to parse LLM response - missing required fields. Response: %s", response)
	}

	return &AgentResponse{
		Thought: strings.TrimSpace(thought),
		Action:  strings.TrimSpace(action),
		Command: strings.TrimSpace(command),
	}, nil
}
