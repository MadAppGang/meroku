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

	// Create a fresh context with timeout for this LLM call
	// Use background context to avoid inheriting cancelled state from previous iterations
	// But also check if the parent context is already cancelled (user stopped agent)
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("agent cancelled: %w", ctx.Err())
	default:
	}

	// Create fresh timeout context from background (60 seconds for complex prompts)
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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
   ⚠️  This step is REQUIRED after any YAML changes!
3. Meroku generates: dev.yaml → env/dev/main.tf (applies Handlebars templates)
4. User runs: ./meroku --apply --env dev (applies Terraform to AWS)
5. Terraform creates/updates AWS resources

KEY CONCEPT: YAML → Generate → Terraform → Apply → AWS
Missing the "Generate" step = changes won't take effect!

CRITICAL FILE RULES:
✓ EDIT: dev.yaml, prod.yaml, staging.yaml (source configuration in ROOT)
✓ READ: env/dev/*.tf, env/prod/*.tf (generated, for diagnostics only)
✗ NEVER EDIT: env/*/*.tf files directly (will be overwritten on next regeneration)

═══════════════════════════════════════════════════════════════════
REGENERATING TERRAFORM (CRITICAL STEP)
═══════════════════════════════════════════════════════════════════

⚠️  IMPORTANT: After editing ANY YAML file, you MUST regenerate Terraform files!

COMMAND: ./meroku --generate --env {environment}

WHY THIS IS CRITICAL:
- YAML files are the source of truth
- env/{env}/*.tf files are AUTO-GENERATED from YAML
- Changes to YAML don't take effect until regenerated
- Skipping regeneration = your changes won't be applied!

WHEN TO REGENERATE:
✓ After editing {env}.yaml (service config, IAM roles, etc.)
✓ After fixing configuration errors
✓ Before running terraform plan/apply
✓ When switching between different YAML versions

WORKFLOW:
1. Edit {environment}.yaml in ROOT (e.g., dev.yaml)
2. Regenerate: ./meroku --generate --env {environment}
3. Verify: Check env/{environment}/main.tf was updated
4. Apply using terraform_apply tool (AWS credentials auto-configured)

═══════════════════════════════════════════════════════════════════
CURRENT CONTEXT
═══════════════════════════════════════════════════════════════════

- Operation: %s
- Environment: %s
- AWS Profile: %s (from YAML config)
- AWS Region: %s (from YAML config - DO NOT use hardcoded regions!)
- Working Directory: %s
- YAML Config: %s.yaml (in root directory)

⚠️  CRITICAL: AWS Region is loaded from %s.yaml config file!
   - Always use the region shown above
   - DO NOT hardcode regions like us-east-1
   - The region varies by environment (dev, prod, etc.)

ERROR INFORMATION:

INITIAL ERROR (Human-Readable):
%s

STRUCTURED ERROR DATA (JSON):
The following JSON contains detailed structured error information from the Terraform deployment:

%s

This structured data includes:
- Resource addresses that failed
- Terraform actions attempted (create, update, delete)
- AWS API error messages
- Terraform diagnostic summaries
- Full error details with context
- Timestamps of failures

Use this JSON to extract specific error details and understand exactly what failed and why.

COMMON MEROKU RESOURCES:
- ECS Cluster: %s_cluster_%s
- ECS Service: %s_service_%s
- ALB: %s_alb_%s
- RDS Instance: %s_db_%s
- ECR Repository: %s_repository_%s

═══════════════════════════════════════════════════════════════════
AVAILABLE TOOLS (AWS Credentials Automatically Set)
═══════════════════════════════════════════════════════════════════

ALL TOOLS AUTOMATICALLY SET: AWS_PROFILE=%s, AWS_REGION=%s

1. aws_cli - Run AWS CLI commands (credentials auto-configured)
   Example: aws ecs describe-services --cluster name --services svc

2. shell - Run shell commands (credentials auto-configured)
   Example: cat dev.yaml | grep service_name

3. file_edit - Edit configuration files
   Format: FILE:path|OLD:old_text|NEW:new_text

4. terraform_plan - Preview terraform changes (credentials auto-configured)
   Example: terraform plan

5. terraform_apply - Apply terraform changes (credentials auto-configured)
   IMPORTANT: Use this tool, not raw shell commands for terraform!
   Example: terraform apply -auto-approve

6. web_search - Search the web for documentation
   Example: AWS ECS service deployment troubleshooting

7. complete - Mark problem as solved
   Example: Success: Service is now running

CRITICAL:
- ALWAYS use terraform_apply tool for terraform operations (not shell)
- NEVER manually set AWS_PROFILE in commands (automatically configured)
- AWS credentials are pre-configured for all tools

═══════════════════════════════════════════════════════════════════
SYSTEMATIC DEBUGGING APPROACH
═══════════════════════════════════════════════════════════════════

1. UNDERSTAND: Check AWS resource status (use aws_cli tool)
2. INVESTIGATE: Review configuration (use shell tool to cat files)
3. EXTRACT ERRORS: Get detailed Terraform errors (see EXTRACTING ERRORS below)
4. RESEARCH: Search for error messages if unclear (use web_search tool)
5. IDENTIFY: Determine root cause (IAM? Networking? Configuration?)
6. FIX: Edit YAML config (use file_edit tool on {env}.yaml)
7. REGENERATE: ⚠️ CRITICAL! Run ./meroku --generate --env {env} (shell tool)
   - Without this step, your YAML changes won't take effect!
   - This regenerates env/{env}/main.tf from {env}.yaml
   - ALWAYS do this after editing YAML files
8. VERIFY GENERATION: Check env/{env}/main.tf has your changes
9. APPLY: Use terraform_apply tool (NOT shell with raw terraform commands!)
10. VERIFY: Check resource status again to confirm fix (use aws_cli tool)

CRITICAL NOTES:
- Step 7 (REGENERATE) is NOT OPTIONAL - always required after YAML edits
- For step 9, ALWAYS use terraform_apply tool:
  ACTION: terraform_apply
  COMMAND: terraform apply -auto-approve
- DO NOT use shell tool with "cd env/{env} && terraform apply" - credentials won't work!

═══════════════════════════════════════════════════════════════════
EXTRACTING TERRAFORM ERRORS (CRITICAL TECHNIQUE)
═══════════════════════════════════════════════════════════════════

Terraform output is VERY VERBOSE. To see actual errors, use grep:

⚠️  IMPORTANT: Don't use 'cd' in commands - the working directory is already set!
    AWS_PROFILE and AWS_REGION are automatically exported in all shell commands.

CORRECT FORMAT (from project root):
  cd env/%s && terraform <command> 2>&1 | grep -A 10 "Error:"

OR SIMPLER (working directory already correct):
  terraform <command> 2>&1 | grep -A 10 "Error:"

WHY THIS WORKS:
- "2>&1" redirects stderr to stdout (captures all output)
- "grep -A 10 'Error:'" finds "Error:" and shows 10 lines after it
- This extracts the actual error message with full context
- AWS credentials are automatically exported before your command runs

WHEN TO USE:
- After terraform apply/plan fails
- When investigating deployment errors
- Before making configuration changes

EXAMPLE COMMANDS (from project root):
- cd env/%s && terraform plan 2>&1 | grep -A 10 "Error:"
- cd env/%s && terraform apply 2>&1 | grep -A 10 "Error:"
- cd env/%s && terraform plan 2>&1 | tail -50

NOTE: AWS_PROFILE=%s is automatically exported in ALL shell commands!

COMMON ISSUES IN MEROKU:
- Terraform errors buried in output → Use: terraform plan 2>&1 | grep -A 10 "Error:"
- Config changes not applied → ⚠️  FORGOT TO REGENERATE! Run: ./meroku --generate --env {env}
- "Resource not found" after YAML edit → You didn't regenerate Terraform files!
- ECS tasks fail to start → Check IAM roles, ECR pull permissions, task definition
- RDS connection failures → Check security groups, VPC configuration
- ALB health checks failing → Check target group configuration, service health
- Terraform state conflicts → Check env/{env}/terraform.tfstate
- Missing resources → Check {env}.yaml has correct schema_version
- Cross-account ECR → Check ecr_strategy and ecr_trusted_accounts in YAML
- AWS credential errors → Check AWS_PROFILE is set (auto-configured in all tools)

⚠️  MOST COMMON MISTAKE: Editing YAML but forgetting to regenerate!
    ALWAYS run: ./meroku --generate --env {env} after YAML changes

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
THOUGHT: The error mentions ECS service deployment failure. I should check the service status first to see if tasks are running and what errors might be present. I'll use the region from the context.
ACTION: aws_cli
COMMAND: aws ecs describe-services --cluster %s_cluster_%s --services %s_service_%s --region %s

Example 2 - Extracting Terraform errors:
THOUGHT: The terraform apply failed. I need to extract the actual error message from the verbose terraform output to understand what went wrong. I'll use grep to find "Error:" and get the context. AWS credentials are automatically exported so I don't need to set them.
ACTION: shell
COMMAND: cd env/%s && terraform plan 2>&1 | grep -A 10 "Error:"

Example 3 - Checking ECS task failures:
THOUGHT: The service shows no running tasks. I need to check recent task failures to see why tasks aren't starting.
ACTION: aws_cli
COMMAND: aws ecs describe-tasks --cluster %s_cluster_%s --tasks $(aws ecs list-tasks --cluster %s_cluster_%s --service %s_service_%s --desired-status STOPPED --region %s --query 'taskArns[0]' --output text) --region %s

Example 4 - Editing YAML config:
THOUGHT: The task definition is missing an IAM role configuration. I need to add the IAM role to the YAML configuration and regenerate terraform.
ACTION: file_edit
COMMAND: FILE:%s.yaml|OLD:enable_ecs_task_role: false|NEW:enable_ecs_task_role: true

Example 5 - Regenerating after YAML edit (CRITICAL STEP):
THOUGHT: I've updated the YAML configuration. Now I MUST regenerate the terraform files - this is critical because YAML changes don't take effect until regenerated. Without this step, my changes won't be applied!
ACTION: shell
COMMAND: cd %s && ./meroku --generate --env %s

Example 6 - Searching for documentation:
THOUGHT: The error message mentions "InvalidParameterException" but I'm not sure what parameter is invalid. Let me search for AWS documentation on this error.
ACTION: web_search
COMMAND: AWS ECS InvalidParameterException task definition

Example 7 - Applying fix:
THOUGHT: I've updated and regenerated the terraform configuration. Now I need to apply the changes to AWS. I'll use the terraform_apply tool which automatically sets AWS credentials.
ACTION: terraform_apply
COMMAND: terraform apply -auto-approve

Example 8 - Problem solved:
THOUGHT: The ECS service is now running with 1 healthy task. The deployment error has been resolved.
ACTION: complete
COMMAND: Success: ECS service is healthy with running tasks

IMPORTANT:
- Only take ONE action per iteration
- Always explain your reasoning in THOUGHT
- Use specific resource names from the context
- For file edits, use the exact format: FILE:path|OLD:old_text|NEW:new_text
- ⚠️  CRITICAL: After editing ANY YAML file, ALWAYS regenerate!
  Command: ./meroku --generate --env {environment}
  Why: YAML changes don't take effect until Terraform is regenerated
  Consequence: Skipping this = your fix won't work!
- After regeneration, verify env/{environment}/main.tf was updated
- Mark as complete only when you've verified the fix worked
- If stuck, try a different approach or ask for more information`,
		agentCtx.Operation,
		agentCtx.Environment,
		agentCtx.AWSProfile,
		agentCtx.AWSRegion,
		agentCtx.WorkingDir,
		agentCtx.Environment, // YAML Config: {env}.yaml (in root directory)
		agentCtx.Environment, // CRITICAL: AWS Region is loaded from {env}.yaml
		agentCtx.InitialError,
		agentCtx.StructuredErrorsJSON, // Structured error data in JSON format
		// COMMON MEROKU RESOURCES section
		agentCtx.Environment, agentCtx.Environment, // ECS Cluster
		agentCtx.Environment, agentCtx.Environment, // ECS Service
		agentCtx.Environment, agentCtx.Environment, // ALB
		agentCtx.Environment, agentCtx.Environment, // RDS Instance
		agentCtx.Environment, agentCtx.Environment, // ECR Repository
		// AVAILABLE TOOLS section - AWS credentials
		agentCtx.AWSProfile, agentCtx.AWSRegion, // ALL TOOLS AUTOMATICALLY SET line
		// EXTRACTING TERRAFORM ERRORS section
		agentCtx.Environment, // CORRECT FORMAT: cd env/%s
		agentCtx.Environment, agentCtx.Environment, agentCtx.Environment, // EXAMPLE COMMANDS (3x cd env/%s)
		agentCtx.AWSProfile, // NOTE: AWS_PROFILE=%s
		// Example 1 - Checking ECS service
		agentCtx.Environment, agentCtx.Environment, agentCtx.Environment, agentCtx.Environment, agentCtx.AWSRegion,
		// Example 2 - Extracting Terraform errors
		agentCtx.Environment, // cd env/%s
		// Example 3 - Checking ECS task failures
		agentCtx.Environment, agentCtx.Environment, agentCtx.Environment, agentCtx.Environment, agentCtx.Environment, agentCtx.Environment, agentCtx.AWSRegion, agentCtx.AWSRegion,
		// Example 4 - Editing YAML config
		agentCtx.Environment,
		// Example 5 - Regenerating after YAML edit
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
