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

	// Enhanced context
	AWSConfigContent  string            // Full ~/.aws/config file
	YAMLContent       map[string]string // Parsed YAML files
	ValidationHistory []interface{}     // Track validation results
	SearchResults     []interface{}     // Web search results cache

	// History
	ActionHistory []SSOAgentAction
	Iteration     int

	// State management
	StateFilePath   string
	TotalIterations int // Across all runs
	RunNumber       int // Which run (1, 2, 3...)
	LastSaveTime    time.Time
}

// SSOAgentAction represents an action taken by the agent
type SSOAgentAction struct {
	Type        string // "think", "ask", "exec", "write", "validate", and new tools
	Description string
	Command     string
	Question    string
	Answer      string
	Result      string
	Error       error

	// Enhanced metadata
	Timestamp    time.Time
	Duration     time.Duration
	RetryCount   int
	ToolMetadata map[string]interface{} // Tool-specific data
}

// SSOAgent handles AWS SSO setup using ReAct pattern
type SSOAgent struct {
	client        *anthropic.Client
	inspector     *ProfileInspector
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

	inspector, err := NewProfileInspector()
	if err != nil {
		return nil, fmt.Errorf("failed to create inspector: %w", err)
	}

	return &SSOAgent{
		client:        &client,
		inspector:     inspector,
		maxIterations: 15,
	}, nil
}

// Run executes the agent's ReAct loop with state persistence
func (a *SSOAgent) Run(ctx context.Context, profileName string, yamlEnv *Env) error {
	fmt.Println("🤖 AI Agent: AWS SSO Setup")
	fmt.Println()

	// Check for existing state
	stateFilePath := GetStateFilePath(profileName)
	var agentCtx *SSOAgentContext
	var runNumber int = 1

	if _, err := os.Stat(stateFilePath); err == nil {
		// State file exists, ask if user wants to continue
		fmt.Printf("\n⚠️  Previous troubleshooting session found for profile '%s'\n", profileName)
		fmt.Printf("   Saved: %s\n", stateFilePath)

		continueChoice, err := AskConfirm("Continue from where you left off?")
		if err != nil || !continueChoice {
			// User declined, start fresh
			fmt.Println("Starting fresh troubleshooting session...")
			agentCtx, err = a.buildContext(profileName, yamlEnv)
			if err != nil {
				return fmt.Errorf("failed to build context: %w", err)
			}
		} else {
			// Load saved state
			fmt.Println("Loading saved state...")
			savedState, err := LoadState(stateFilePath)
			if err != nil {
				fmt.Printf("⚠️  Failed to load state: %v\n", err)
				fmt.Println("Starting fresh session instead...")
				agentCtx, err = a.buildContext(profileName, yamlEnv)
				if err != nil {
					return fmt.Errorf("failed to build context: %w", err)
				}
			} else {
				// Resume from saved state
				agentCtx = savedState.Context
				runNumber = savedState.RunNumber + 1
				fmt.Printf("✅ Resumed from iteration %d (Run #%d)\n", savedState.TotalIterations, runNumber)
			}
		}
	} else {
		// No saved state, start fresh
		fmt.Printf("📋 Profile: %s\n", profileName)
		if yamlEnv != nil {
			fmt.Printf("📋 Environment: %s (account: %s, region: %s)\n",
				yamlEnv.Env, yamlEnv.AccountID, yamlEnv.Region)
		}
		fmt.Println()

		agentCtx, err = a.buildContext(profileName, yamlEnv)
		if err != nil {
			return fmt.Errorf("failed to build context: %w", err)
		}
	}

	agentCtx.StateFilePath = stateFilePath
	agentCtx.RunNumber = runNumber

	// ReAct loop (max 15 iterations per run, 30 total)
	maxIterationsPerRun := 15
	maxTotalIterations := 30

	for i := 0; i < maxIterationsPerRun; i++ {
		agentCtx.Iteration = i + 1
		agentCtx.TotalIterations++

		fmt.Printf("\n───────── Iteration %d (Run #%d, Total: %d/%d) ─────────\n\n",
			agentCtx.Iteration, runNumber, agentCtx.TotalIterations, maxTotalIterations)

		// Get next action from LLM
		action, err := a.getNextAction(ctx, agentCtx)
		if err != nil {
			// Save state before failing
			SaveState(&SSOAgentState{
				ProfileName:     profileName,
				Context:         agentCtx,
				TotalIterations: agentCtx.TotalIterations,
				RunNumber:       runNumber,
			}, stateFilePath)
			return fmt.Errorf("agent error: %w", err)
		}

		// Check for completion
		if action.Type == "complete" {
			fmt.Println("✅ SUCCESS! AWS SSO setup is complete.")
			fmt.Println()
			a.printSummary(agentCtx)

			// Cleanup state file on success
			CleanupStateFile(stateFilePath)
			return nil
		}

		// Execute action
		if err := a.executeAction(ctx, action, agentCtx); err != nil {
			action.Error = err
			action.Result = fmt.Sprintf("Failed: %v", err)
		}

		// Add to history
		agentCtx.ActionHistory = append(agentCtx.ActionHistory, *action)

		// Save state after each action
		SaveState(&SSOAgentState{
			ProfileName:     profileName,
			Context:         agentCtx,
			TotalIterations: agentCtx.TotalIterations,
			RunNumber:       runNumber,
		}, stateFilePath)

		// Check if stuck
		if a.isStuck(agentCtx) {
			fmt.Println("⚠️  Agent appears stuck (repeated same action 3 times)")
			SaveState(&SSOAgentState{
				ProfileName:     profileName,
				Context:         agentCtx,
				TotalIterations: agentCtx.TotalIterations,
				RunNumber:       runNumber,
			}, stateFilePath)
			return fmt.Errorf("agent stuck, please try manual wizard mode")
		}
	}

	// Reached iteration limit for this run
	if agentCtx.TotalIterations < maxTotalIterations {
		// Can continue if user wants
		fmt.Printf("\n⚠️  Reached iteration limit for this run (%d iterations)\n", maxIterationsPerRun)
		fmt.Printf("   Total iterations so far: %d/%d\n", agentCtx.TotalIterations, maxTotalIterations)

		continueChoice, err := AskConfirm("Continue troubleshooting in a new run?")
		if err != nil || !continueChoice {
			// User declined, save state and exit
			SaveState(&SSOAgentState{
				ProfileName:     profileName,
				Context:         agentCtx,
				TotalIterations: agentCtx.TotalIterations,
				RunNumber:       runNumber,
			}, stateFilePath)
			fmt.Printf("\n💾 Progress saved to: %s\n", stateFilePath)
			fmt.Println("   Run the agent again to continue troubleshooting.")
			return nil
		}

		// User wants to continue, recursively call Run
		return a.Run(ctx, profileName, yamlEnv)
	}

	// Absolute maximum reached
	SaveState(&SSOAgentState{
		ProfileName:     profileName,
		Context:         agentCtx,
		TotalIterations: agentCtx.TotalIterations,
		RunNumber:       runNumber,
	}, stateFilePath)

	return fmt.Errorf("reached maximum iterations (%d) without completing setup", maxTotalIterations)
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

	// Debug logging
	debugLogPrompt(agentCtx.ProfileName, agentCtx.Iteration, prompt)

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

	// Debug logging
	debugLogResponse(agentCtx.ProfileName, agentCtx.Iteration, responseText)

	// Parse the response
	return a.parseResponse(responseText)
}

// debugLogPrompt writes prompt to debug file
func debugLogPrompt(profileName string, iteration int, prompt string) {
	debugFile := fmt.Sprintf("/tmp/meroku_sso_debug_%s.log", profileName)
	f, err := os.OpenFile(debugFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	f.WriteString(fmt.Sprintf("\n=== ITERATION %d - PROMPT (%s) ===\n", iteration, timestamp))
	f.WriteString(prompt)
	f.WriteString("\n\n")
}

// debugLogResponse writes response to debug file
func debugLogResponse(profileName string, iteration int, response string) {
	debugFile := fmt.Sprintf("/tmp/meroku_sso_debug_%s.log", profileName)
	f, err := os.OpenFile(debugFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	f.WriteString(fmt.Sprintf("=== ITERATION %d - RESPONSE (%s) ===\n", iteration, timestamp))
	f.WriteString(response)
	f.WriteString("\n\n")
}

// buildPrompt constructs the enhanced prompt for the LLM
func (a *SSOAgent) buildPrompt(agentCtx *SSOAgentContext) string {
	return BuildEnhancedSystemPrompt(agentCtx)
}

// DEPRECATED: Old buildPrompt kept for reference
func (a *SSOAgent) buildPromptOld(agentCtx *SSOAgentContext) string {
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

	// Parse THINK - but if there's a second line, prefer the action on line 2
	if strings.HasPrefix(firstLine, "THINK:") {
		// If there's a second line, it should be the action
		if len(lines) > 1 {
			secondLine := strings.TrimSpace(lines[1])
			// Try to parse the second line as an action
			if secondLine != "" && !strings.HasPrefix(secondLine, "THINK:") {
				// Recursively parse the second line as the action
				action, err := a.parseActionLine(secondLine)
				if err == nil {
					// Successfully parsed an action on line 2
					thought := strings.TrimPrefix(firstLine, "THINK:")
					action.Description = strings.TrimSpace(thought) + " | " + action.Description
					return action, nil
				}
			}
		}

		// No valid action on line 2, just return the think
		thought := strings.TrimPrefix(firstLine, "THINK:")
		thought = strings.TrimSpace(thought)
		return &SSOAgentAction{
			Type:        "think",
			Description: thought,
			Timestamp:   time.Now(),
		}, nil
	}

	// Not a THINK line, parse as action directly
	return a.parseActionLine(firstLine)
}

// parseActionLine parses a single action line
func (a *SSOAgent) parseActionLine(line string) (*SSOAgentAction, error) {
	line = strings.TrimSpace(line)

	// Parse new enhanced tools
	if strings.HasPrefix(line, "read_aws_config:") || strings.HasPrefix(line, "ACTION: read_aws_config") {
		command := extractCommand(line, "read_aws_config")
		return &SSOAgentAction{
			Type:      "read_aws_config",
			Command:   command,
			Timestamp: time.Now(),
		}, nil
	}

	if strings.HasPrefix(line, "write_aws_config:") || strings.HasPrefix(line, "ACTION: write_aws_config") {
		command := extractCommand(line, "write_aws_config")
		return &SSOAgentAction{
			Type:      "write_aws_config",
			Command:   command,
			Timestamp: time.Now(),
		}, nil
	}

	if strings.HasPrefix(line, "read_yaml:") || strings.HasPrefix(line, "ACTION: read_yaml") {
		command := extractCommand(line, "read_yaml")
		return &SSOAgentAction{
			Type:      "read_yaml",
			Command:   command,
			Timestamp: time.Now(),
		}, nil
	}

	if strings.HasPrefix(line, "write_yaml:") || strings.HasPrefix(line, "ACTION: write_yaml") {
		command := extractCommand(line, "write_yaml")
		return &SSOAgentAction{
			Type:      "write_yaml",
			Command:   command,
			Timestamp: time.Now(),
		}, nil
	}

	if strings.HasPrefix(line, "ask_choice:") || strings.HasPrefix(line, "ACTION: ask_choice") {
		command := extractCommand(line, "ask_choice")
		return &SSOAgentAction{
			Type:      "ask_choice",
			Command:   command,
			Timestamp: time.Now(),
		}, nil
	}

	if strings.HasPrefix(line, "ask_confirm:") || strings.HasPrefix(line, "ACTION: ask_confirm") {
		command := extractCommand(line, "ask_confirm")
		return &SSOAgentAction{
			Type:      "ask_confirm",
			Command:   command,
			Timestamp: time.Now(),
		}, nil
	}

	if strings.HasPrefix(line, "ask_input:") || strings.HasPrefix(line, "ACTION: ask_input") {
		command := extractCommand(line, "ask_input")
		return &SSOAgentAction{
			Type:      "ask_input",
			Command:   command,
			Timestamp: time.Now(),
		}, nil
	}

	if strings.HasPrefix(line, "web_search:") || strings.HasPrefix(line, "ACTION: web_search") {
		command := extractCommand(line, "web_search")
		return &SSOAgentAction{
			Type:      "web_search",
			Command:   command,
			Timestamp: time.Now(),
		}, nil
	}

	if strings.HasPrefix(line, "aws_validate:") || strings.HasPrefix(line, "ACTION: aws_validate") {
		command := extractCommand(line, "aws_validate")
		return &SSOAgentAction{
			Type:      "aws_validate",
			Command:   command,
			Timestamp: time.Now(),
		}, nil
	}

	// Existing tool parsing
	if strings.HasPrefix(line, "ASK:") {
		question := strings.TrimPrefix(line, "ASK:")
		question = strings.TrimSpace(question)
		return &SSOAgentAction{
			Type:      "ask",
			Question:  question,
			Timestamp: time.Now(),
		}, nil
	}

	if strings.HasPrefix(line, "EXEC:") {
		command := strings.TrimPrefix(line, "EXEC:")
		command = strings.TrimSpace(command)
		return &SSOAgentAction{
			Type:      "exec",
			Command:   command,
			Timestamp: time.Now(),
		}, nil
	}

	if strings.HasPrefix(line, "WRITE:") {
		configLine := strings.TrimPrefix(line, "WRITE:")
		configLine = strings.TrimSpace(configLine)
		return &SSOAgentAction{
			Type:        "write",
			Description: configLine,
			Timestamp:   time.Now(),
		}, nil
	}

	if strings.HasPrefix(line, "COMPLETE:") {
		summary := strings.TrimPrefix(line, "COMPLETE:")
		summary = strings.TrimSpace(summary)
		return &SSOAgentAction{
			Type:        "complete",
			Description: summary,
			Timestamp:   time.Now(),
		}, nil
	}

	return nil, fmt.Errorf("unrecognized action format: %s", line)
}

// extractCommand extracts command from action line
func extractCommand(line, actionType string) string {
	// Handle "ACTION: type" format
	if strings.HasPrefix(line, "ACTION: "+actionType) {
		line = strings.TrimPrefix(line, "ACTION: "+actionType)
	} else {
		line = strings.TrimPrefix(line, actionType+":")
	}
	return strings.TrimSpace(line)
}

// executeAction performs the action with enhanced tool support
func (a *SSOAgent) executeAction(ctx context.Context, action *SSOAgentAction, agentCtx *SSOAgentContext) error {
	startTime := time.Now()
	defer func() {
		action.Duration = time.Since(startTime)
	}()

	switch action.Type {
	case "think":
		fmt.Println("🤔 THINKING:")
		fmt.Println("   " + action.Description)
		return nil

	// New enhanced tools
	case "read_aws_config":
		result, err := a.toolReadAWSConfig(ctx, action.Command, agentCtx)
		*action = *result
		return err

	case "write_aws_config":
		result, err := a.toolWriteAWSConfig(ctx, action.Command, agentCtx)
		*action = *result
		return err

	case "read_yaml":
		result, err := a.toolReadYAML(ctx, action.Command, agentCtx)
		*action = *result
		return err

	case "write_yaml":
		result, err := a.toolWriteYAML(ctx, action.Command, agentCtx)
		*action = *result
		return err

	case "ask_choice":
		result, err := a.toolAskChoice(ctx, action.Command, agentCtx)
		*action = *result
		return err

	case "ask_confirm":
		result, err := a.toolAskConfirm(ctx, action.Command, agentCtx)
		*action = *result
		return err

	case "ask_input":
		result, err := a.toolAskInput(ctx, action.Command, agentCtx)
		*action = *result
		return err

	case "web_search":
		result, err := a.toolWebSearch(ctx, action.Command, agentCtx)
		*action = *result
		return err

	case "aws_validate":
		result, err := a.toolAWSValidate(ctx, action.Command, agentCtx)
		*action = *result
		return err

	// Existing tools
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

	// Check for 3+ consecutive "think" actions (analysis paralysis)
	last3 := agentCtx.ActionHistory[len(agentCtx.ActionHistory)-3:]
	if last3[0].Type == "think" && last3[1].Type == "think" && last3[2].Type == "think" {
		fmt.Println("⚠️  Detected analysis paralysis (3 consecutive thinks without action)")
		return true
	}

	// Check if same action type and description (original stuck detection)
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
