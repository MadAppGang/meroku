package main

import (
	"fmt"
	"os"
	"strings"
)

// offerAIAgentHelp offers the new ReAct-based AI agent for error recovery
// This replaces the simple AI helper with an autonomous agent
func offerAIAgentHelp(ctx ErrorContext) error {
	if !isAIHelperAvailable() {
		fmt.Println("\n⚠️  AI Agent requires ANTHROPIC_API_KEY to be set")
		fmt.Println("   Set it with: export ANTHROPIC_API_KEY=your_key_here")
		return fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	// Ask user if they want to use the AI agent
	if !promptForAIAgent() {
		return nil
	}

	// Get current working directory
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Convert ErrorContext to AgentContext
	agentContext := &AgentContext{
		Operation:      ctx.Operation,
		Environment:    ctx.Environment,
		AWSProfile:     ctx.AWSProfile,
		AWSRegion:      ctx.AWSRegion,
		WorkingDir:     wd,
		InitialError:   strings.Join(ctx.Errors, "\n\n"),
		ResourceErrors: ctx.Errors,
		AdditionalInfo: make(map[string]string),
	}

	// Run the AI Agent TUI
	fmt.Println("\n🚀 Starting AI Agent...")
	fmt.Println("   The agent will analyze the problem and attempt to fix it autonomously.")
	fmt.Println()

	err = RunAIAgentTUI(agentContext)
	if err != nil {
		return fmt.Errorf("AI agent failed: %w", err)
	}

	return nil
}

// promptForAIAgent asks the user if they want to use the autonomous AI agent
func promptForAIAgent() bool {
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("🤖 Autonomous AI Agent Available!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println("The AI agent will:")
	fmt.Println("  • Analyze the errors autonomously")
	fmt.Println("  • Investigate AWS resources and configuration")
	fmt.Println("  • Identify root causes")
	fmt.Println("  • Apply fixes automatically")
	fmt.Println("  • Verify the solution works")
	fmt.Println()
	fmt.Println("You'll see each step as it happens and can stop at any time.")
	fmt.Println()
	fmt.Print("   Start the AI agent? (y/n): ")

	var response string
	fmt.Scanln(&response)
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// offerAIAgentFromMenu allows running the AI agent from the main menu
// This is useful for when users want to proactively troubleshoot
func offerAIAgentFromMenu() error {
	if !isAIHelperAvailable() {
		fmt.Println("\n⚠️  AI Agent requires ANTHROPIC_API_KEY to be set")
		fmt.Println("   Set it with: export ANTHROPIC_API_KEY=your_key_here")
		fmt.Println()
		fmt.Print("Press Enter to continue...")
		fmt.Scanln()
		return fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	// Get environment
	env := selectedEnvironment
	if env == "" {
		fmt.Println("No environment selected. Please select an environment first.")
		fmt.Print("Press Enter to continue...")
		fmt.Scanln()
		return fmt.Errorf("no environment selected")
	}

	// Get AWS profile
	awsProfile := selectedAWSProfile
	if awsProfile == "" {
		fmt.Println("No AWS profile selected. Please configure AWS profile first.")
		fmt.Print("Press Enter to continue...")
		fmt.Scanln()
		return fmt.Errorf("no AWS profile selected")
	}

	// Prompt for problem description
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("🤖 AI Agent - Describe the Problem")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println("What issue would you like the AI agent to investigate?")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  • ECS service not starting")
	fmt.Println("  • Terraform apply failing")
	fmt.Println("  • DNS not resolving")
	fmt.Println("  • Database connection issues")
	fmt.Println()
	fmt.Print("Problem: ")

	var problemDescription string
	// Read full line including spaces
	_, err := fmt.Scanln(&problemDescription)
	if err != nil {
		// If Scanln fails, try reading the whole line
		reader := os.Stdin
		buf := make([]byte, 1024)
		n, _ := reader.Read(buf)
		problemDescription = string(buf[:n])
	}

	problemDescription = strings.TrimSpace(problemDescription)
	if problemDescription == "" {
		fmt.Println("No problem description provided.")
		fmt.Print("Press Enter to continue...")
		fmt.Scanln()
		return fmt.Errorf("no problem description")
	}

	// Get working directory
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Get AWS region from environment config
	awsRegion := "us-east-1" // default
	envConfig, err := loadEnv(env + ".yaml")
	if err != nil {
		fmt.Printf("Warning: Could not load environment config: %v\n", err)
	} else if envConfig.Region != "" {
		awsRegion = envConfig.Region
	}

	// Create agent context
	agentContext := &AgentContext{
		Operation:      "troubleshooting",
		Environment:    env,
		AWSProfile:     awsProfile,
		AWSRegion:      awsRegion,
		WorkingDir:     wd,
		InitialError:   problemDescription,
		ResourceErrors: []string{problemDescription},
		AdditionalInfo: make(map[string]string),
	}

	// Run the AI Agent TUI
	fmt.Println("\n🚀 Starting AI Agent...")
	fmt.Println("   The agent will analyze the problem and attempt to fix it autonomously.")
	fmt.Println()

	err = RunAIAgentTUI(agentContext)
	if err != nil {
		fmt.Printf("\n❌ AI agent failed: %v\n", err)
		fmt.Print("Press Enter to continue...")
		fmt.Scanln()
		return err
	}

	fmt.Print("\nPress Enter to return to main menu...")
	fmt.Scanln()
	return nil
}
