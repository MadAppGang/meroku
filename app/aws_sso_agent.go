package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// SSOAgentContext contains all information for the SSO agent
type SSOAgentContext struct {
	ProfileName  string
	YAMLEnv      *Env
	ConfigPath   string
	ConfigExists bool
	ProfileInfo  *ProfileInfo

	// Collected information
	SSOStartURL  string
	SSORegion    string
	AccountID    string
	RoleName     string
	Region       string
	Output       string

	// History
	ActionHistory []SSOAgentAction
	Iteration     int
}

// SSOAgentAction represents an action taken by the agent
type SSOAgentAction struct {
	Type        string // "think", "ask", "exec", "write", "validate"
	Description string
	Command     string
	Question    string
	Answer      string
	Result      string
	Error       error
}

// SSOAgent handles AWS SSO setup using ReAct pattern
type SSOAgent struct {
	client      *anthropic.Client
	maxIterations int
}

// NewSSOAgent creates a new SSO agent
func NewSSOAgent() (*SSOAgent, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set\n\nPlease set it with:\n  export ANTHROPIC_API_KEY=your_key_here\n\nGet your key from: https://console.anthropic.com/settings/keys")
	}

	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
	)

	return &SSOAgent{
		client:        &client,
		maxIterations: 15,
	}, nil
}

// Run executes the agent's ReAct loop
func (a *SSOAgent) Run(ctx context.Context, profileName string, yamlEnv *Env) error {
	fmt.Println("🤖 AI Agent: AWS SSO Setup")
	fmt.Println()
	fmt.Printf("📋 Profile: %s\n", profileName)
	if yamlEnv != nil {
		fmt.Printf("📋 Environment: %s (account: %s, region: %s)\n",
			yamlEnv.Env, yamlEnv.AccountID, yamlEnv.Region)
	}
	fmt.Println()

	// Build context
	agentCtx, err := a.buildContext(profileName, yamlEnv)
	if err != nil {
		return fmt.Errorf("failed to build context: %w", err)
	}

	// ReAct loop
	for i := 0; i < a.maxIterations; i++ {
		agentCtx.Iteration = i + 1

		fmt.Printf("\n───────────────────────────── Iteration %d/%d ─────────────────────────────\n\n", i+1, a.maxIterations)

		// Get next action from LLM
		action, err := a.getNextAction(ctx, agentCtx)
		if err != nil {
			return fmt.Errorf("agent error: %w", err)
		}

		// Check for completion
		if action.Type == "complete" {
			fmt.Println("✅ SUCCESS! AWS SSO setup is complete.")
			fmt.Println()
			a.printSummary(agentCtx)
			return nil
		}

		// Execute action
		if err := a.executeAction(ctx, action, agentCtx); err != nil {
			// Agent handles its own errors, continue to next iteration
			action.Error = err
			action.Result = fmt.Sprintf("Failed: %v", err)
		}

		// Add to history
		agentCtx.ActionHistory = append(agentCtx.ActionHistory, *action)

		// Check if stuck (same action 3 times in a row)
		if a.isStuck(agentCtx) {
			return fmt.Errorf("agent appears stuck (repeated same action 3 times). Please try manual wizard mode")
		}
	}

	return fmt.Errorf("agent reached max iterations (%d) without completing setup", a.maxIterations)
}

// buildContext gathers initial context
func (a *SSOAgent) buildContext(profileName string, yamlEnv *Env) (*SSOAgentContext, error) {
	inspector, err := NewProfileInspector()
	if err != nil {
		return nil, err
	}

	ctx := &SSOAgentContext{
		ProfileName:   profileName,
		YAMLEnv:       yamlEnv,
		ConfigPath:    getAWSConfigPath(),
		ConfigExists:  true,
		ActionHistory: []SSOAgentAction{},
	}

	// Check if config exists
	if _, err := os.Stat(ctx.ConfigPath); os.IsNotExist(err) {
		ctx.ConfigExists = false
	}

	// Inspect profile
	profileInfo, err := inspector.InspectProfile(profileName)
	if err == nil {
		ctx.ProfileInfo = profileInfo
	}

	// Pre-fill from YAML
	if yamlEnv != nil {
		if yamlEnv.AccountID != "" {
			ctx.AccountID = yamlEnv.AccountID
		}
		if yamlEnv.Region != "" {
			ctx.Region = yamlEnv.Region
		}
	}

	return ctx, nil
}

// getNextAction asks Claude to decide the next action
func (a *SSOAgent) getNextAction(ctx context.Context, agentCtx *SSOAgentContext) (*SSOAgentAction, error) {
	prompt := a.buildPrompt(agentCtx)

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	message, err := a.client.Messages.New(timeoutCtx, anthropic.MessageNewParams{
		Model:     "claude-sonnet-4-5-20250929",
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
	return a.parseResponse(responseText)
}

// buildPrompt constructs the prompt for the LLM
func (a *SSOAgent) buildPrompt(agentCtx *SSOAgentContext) string {
	return fmt.Sprintf(`You are an AWS SSO configuration expert. Your goal is to set up AWS SSO profiles correctly and efficiently.

═══════════════════════════════════════════════════════════════════
CURRENT SITUATION
═══════════════════════════════════════════════════════════════════

Profile Name: %s
Config File: %s
Config Exists: %v

Profile Status:
%s

Available Information:
- From YAML: AccountID=%s, Region=%s
- Collected: SSOStartURL=%s, SSORegion=%s, AccountID=%s, RoleName=%s, Region=%s

═══════════════════════════════════════════════════════════════════
YOUR CAPABILITIES
═══════════════════════════════════════════════════════════════════

You can take the following actions:

1. THINK - Analyze the situation and reason about what to do next
   Format: THINK: <your reasoning>

2. ASK - Ask the user a specific question to gather missing information
   Format: ASK: <question>
   Use this when you need information that's not available

3. EXEC - Execute an AWS CLI command
   Format: EXEC: <command>
   Examples:
   - EXEC: aws sso login --profile dev
   - EXEC: aws sts get-caller-identity --profile dev
   - EXEC: aws --version

4. WRITE - Write AWS configuration
   Format: WRITE: <profile_name>|<sso_start_url>|<sso_region>|<account_id>|<role_name>|<region>|<output>
   Example: WRITE: dev|https://mycompany.awsapps.com/start|us-east-1|123456789012|AdministratorAccess|us-east-1|json
   This will create a modern SSO configuration with sso-session

5. COMPLETE - Mark the setup as complete
   Format: COMPLETE: <summary message>
   Use this when AWS SSO is fully configured and validated

═══════════════════════════════════════════════════════════════════
VALIDATION RULES (from AWS documentation)
═══════════════════════════════════════════════════════════════════

Modern SSO Configuration (RECOMMENDED):
- Profile section needs: sso_session (name), sso_account_id (12 digits), sso_role_name
- SSO-session section needs: sso_start_url (https://...), sso_region (e.g., us-east-1)

Required Field Formats:
- sso_start_url: Must be HTTPS URL, typically https://*.awsapps.com/start
- sso_region: Valid AWS region (e.g., us-east-1, eu-west-1)
- sso_account_id: Exactly 12 digits
- sso_role_name: Valid IAM role name (e.g., AdministratorAccess, PowerUserAccess)
- region: Valid AWS region
- output: One of: json, yaml, text, table

Common Role Names:
- AdministratorAccess (full access)
- PowerUserAccess (most services, limited IAM)
- ReadOnlyAccess (read-only)

═══════════════════════════════════════════════════════════════════
STRATEGY
═══════════════════════════════════════════════════════════════════

1. Analyze what information is missing
2. Ask user for missing critical information (SSO start URL is most important)
3. Use information from YAML when available (don't ask for what you have)
4. Write configuration once you have all required fields
5. Execute "aws sso login" to authenticate
6. Validate with "aws sts get-caller-identity"
7. Verify account ID matches expected value
8. Mark as COMPLETE when everything works

Ask questions in priority order:
1. SSO Start URL (critical, can't proceed without it)
2. Role Name (has good defaults like AdministratorAccess)
3. SSO Region (defaults to us-east-1)
4. Other optional fields

Don't ask for information you already have from YAML or previous answers!

═══════════════════════════════════════════════════════════════════
ACTION HISTORY (Recent actions taken)
═══════════════════════════════════════════════════════════════════

%s

═══════════════════════════════════════════════════════════════════
WHAT SHOULD YOU DO NEXT?
═══════════════════════════════════════════════════════════════════

Respond with ONE action in the format specified above (THINK, ASK, EXEC, WRITE, or COMPLETE).

IMPORTANT:
- Start with THINK to show your reasoning
- Ask ONE question at a time
- Use YAML data to avoid redundant questions
- Validate credentials after configuration
- Be concise and helpful
`,
		agentCtx.ProfileName,
		agentCtx.ConfigPath,
		agentCtx.ConfigExists,
		a.formatProfileStatus(agentCtx),
		agentCtx.YAMLEnv.AccountID,
		agentCtx.YAMLEnv.Region,
		agentCtx.SSOStartURL,
		agentCtx.SSORegion,
		agentCtx.AccountID,
		agentCtx.RoleName,
		agentCtx.Region,
		a.formatHistory(agentCtx))
}

// formatProfileStatus formats the profile status for the prompt
func (a *SSOAgent) formatProfileStatus(agentCtx *SSOAgentContext) string {
	if agentCtx.ProfileInfo == nil {
		return "Profile not analyzed yet"
	}

	info := agentCtx.ProfileInfo
	if !info.Exists {
		return "Profile does not exist - needs to be created"
	}

	if info.Complete {
		return fmt.Sprintf("Profile exists and is complete (Type: %s)", info.Type)
	}

	return fmt.Sprintf("Profile exists but incomplete (Type: %s, Missing: %s)",
		info.Type, strings.Join(info.MissingFields, ", "))
}

// formatHistory formats recent actions for the prompt
func (a *SSOAgent) formatHistory(agentCtx *SSOAgentContext) string {
	if len(agentCtx.ActionHistory) == 0 {
		return "No actions taken yet"
	}

	var b strings.Builder
	// Show last 5 actions
	start := len(agentCtx.ActionHistory) - 5
	if start < 0 {
		start = 0
	}

	for i := start; i < len(agentCtx.ActionHistory); i++ {
		action := agentCtx.ActionHistory[i]
		b.WriteString(fmt.Sprintf("\n%d. %s", i+1, action.Type))
		if action.Description != "" {
			b.WriteString(fmt.Sprintf(": %s", action.Description))
		}
		if action.Question != "" {
			b.WriteString(fmt.Sprintf(" | Question: %s | Answer: %s", action.Question, action.Answer))
		}
		if action.Result != "" {
			b.WriteString(fmt.Sprintf(" | Result: %s", action.Result))
		}
		if action.Error != nil {
			b.WriteString(fmt.Sprintf(" | Error: %v", action.Error))
		}
	}

	return b.String()
}

// parseResponse parses the LLM's response into an action
func (a *SSOAgent) parseResponse(response string) (*SSOAgentAction, error) {
	lines := strings.Split(strings.TrimSpace(response), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty response")
	}

	firstLine := strings.TrimSpace(lines[0])

	if strings.HasPrefix(firstLine, "THINK:") {
		thought := strings.TrimPrefix(firstLine, "THINK:")
		thought = strings.TrimSpace(thought)
		return &SSOAgentAction{
			Type:        "think",
			Description: thought,
		}, nil
	}

	if strings.HasPrefix(firstLine, "ASK:") {
		question := strings.TrimPrefix(firstLine, "ASK:")
		question = strings.TrimSpace(question)
		return &SSOAgentAction{
			Type:     "ask",
			Question: question,
		}, nil
	}

	if strings.HasPrefix(firstLine, "EXEC:") {
		command := strings.TrimPrefix(firstLine, "EXEC:")
		command = strings.TrimSpace(command)
		return &SSOAgentAction{
			Type:    "exec",
			Command: command,
		}, nil
	}

	if strings.HasPrefix(firstLine, "WRITE:") {
		configLine := strings.TrimPrefix(firstLine, "WRITE:")
		configLine = strings.TrimSpace(configLine)
		return &SSOAgentAction{
			Type:        "write",
			Description: configLine,
		}, nil
	}

	if strings.HasPrefix(firstLine, "COMPLETE:") {
		summary := strings.TrimPrefix(firstLine, "COMPLETE:")
		summary = strings.TrimSpace(summary)
		return &SSOAgentAction{
			Type:        "complete",
			Description: summary,
		}, nil
	}

	return nil, fmt.Errorf("unrecognized action format: %s", firstLine)
}

// executeAction performs the action
func (a *SSOAgent) executeAction(ctx context.Context, action *SSOAgentAction, agentCtx *SSOAgentContext) error {
	switch action.Type {
	case "think":
		fmt.Println("🤔 THINKING:")
		fmt.Println("   " + action.Description)
		return nil

	case "ask":
		return a.askUser(action, agentCtx)

	case "exec":
		return a.execCommand(action)

	case "write":
		return a.writeConfig(action, agentCtx)

	case "complete":
		// Nothing to execute for complete
		return nil

	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}
}

// askUser prompts the user for input
func (a *SSOAgent) askUser(action *SSOAgentAction, agentCtx *SSOAgentContext) error {
	fmt.Println()
	fmt.Println("💬 " + action.Question)
	fmt.Print("   Answer: ")

	var answer string
	fmt.Scanln(&answer)
	answer = strings.TrimSpace(answer)

	action.Answer = answer
	action.Result = "User answered: " + answer

	// Store the answer in context based on question context
	questionLower := strings.ToLower(action.Question)
	if strings.Contains(questionLower, "start url") || strings.Contains(questionLower, "sso url") {
		agentCtx.SSOStartURL = answer
	} else if strings.Contains(questionLower, "sso region") {
		agentCtx.SSORegion = answer
	} else if strings.Contains(questionLower, "account") {
		agentCtx.AccountID = answer
	} else if strings.Contains(questionLower, "role") {
		agentCtx.RoleName = answer
	} else if strings.Contains(questionLower, "region") && !strings.Contains(questionLower, "sso") {
		agentCtx.Region = answer
	} else if strings.Contains(questionLower, "output") {
		agentCtx.Output = answer
	}

	return nil
}

// execCommand executes an AWS CLI command
func (a *SSOAgent) execCommand(action *SSOAgentAction) error {
	fmt.Println()
	fmt.Println("🔧 ACTION: " + action.Command)

	output, err := runCommandWithOutput("bash", "-c", action.Command)
	if err != nil {
		action.Result = fmt.Sprintf("Failed: %v", err)
		fmt.Println("❌ Failed")
		return err
	}

	action.Result = "Success: " + output
	fmt.Println("✅ Success")
	return nil
}

// writeConfig writes AWS SSO configuration
func (a *SSOAgent) writeConfig(action *SSOAgentAction, agentCtx *SSOAgentContext) error {
	fmt.Println()
	fmt.Println("🔧 ACTION: Writing AWS SSO configuration")

	// Parse the config line: profile|start_url|sso_region|account_id|role|region|output
	parts := strings.Split(action.Description, "|")
	if len(parts) < 7 {
		return fmt.Errorf("invalid WRITE format, expected: profile|start_url|sso_region|account_id|role|region|output")
	}

	writer := NewConfigWriter()
	opts := ModernSSOProfileOptions{
		ProfileName:           strings.TrimSpace(parts[0]),
		SSOSessionName:        "default-sso",
		SSOStartURL:           strings.TrimSpace(parts[1]),
		SSORegion:             strings.TrimSpace(parts[2]),
		SSOAccountID:          strings.TrimSpace(parts[3]),
		SSORoleName:           strings.TrimSpace(parts[4]),
		Region:                strings.TrimSpace(parts[5]),
		Output:                strings.TrimSpace(parts[6]),
		SSORegistrationScopes: "sso:account:access",
	}

	if err := writer.WriteModernSSOProfile(opts); err != nil {
		action.Result = fmt.Sprintf("Failed: %v", err)
		return err
	}

	action.Result = "Configuration written successfully"
	return nil
}

// isStuck checks if the agent is repeating the same action
func (a *SSOAgent) isStuck(agentCtx *SSOAgentContext) bool {
	if len(agentCtx.ActionHistory) < 3 {
		return false
	}

	last3 := agentCtx.ActionHistory[len(agentCtx.ActionHistory)-3:]
	// Check if same action type and description
	return last3[0].Type == last3[1].Type && last3[1].Type == last3[2].Type &&
		last3[0].Description == last3[1].Description && last3[1].Description == last3[2].Description
}

// printSummary prints a summary of what was accomplished
func (a *SSOAgent) printSummary(agentCtx *SSOAgentContext) {
	fmt.Println("Summary:")
	fmt.Printf("  ✓ Profile '%s' configured\n", agentCtx.ProfileName)
	if agentCtx.AccountID != "" {
		fmt.Printf("  ✓ Account ID: %s\n", agentCtx.AccountID)
	}
	if agentCtx.Region != "" {
		fmt.Printf("  ✓ Region: %s\n", agentCtx.Region)
	}
	fmt.Printf("  ✓ Total iterations: %d\n", agentCtx.Iteration)
	fmt.Println()
	fmt.Println("You can now use this profile with AWS CLI commands:")
	fmt.Printf("  aws s3 ls --profile %s\n", agentCtx.ProfileName)
	fmt.Println()
}

// RunSSOAgent runs the AI agent for AWS SSO setup
func RunSSOAgent(profileName string, yamlEnv *Env) error {
	agent, err := NewSSOAgent()
	if err != nil {
		return err
	}

	ctx := context.Background()
	return agent.Run(ctx, profileName, yamlEnv)
}
