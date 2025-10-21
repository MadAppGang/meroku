package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/charmbracelet/lipgloss"
)

// ErrorContext contains information about the error for AI analysis
type ErrorContext struct {
	Operation   string   // "apply" or "destroy"
	Environment string   // "dev", "prod", etc.
	AWSProfile  string   // Current AWS profile
	AWSRegion   string   // Current AWS region
	Errors      []string // Terraform error messages
	WorkingDir  string   // Current directory
}

// isAIHelperAvailable checks if the ANTHROPIC_API_KEY is set
func isAIHelperAvailable() bool {
	return os.Getenv("ANTHROPIC_API_KEY") != ""
}

// promptForAIHelp asks the user if they want AI assistance
func promptForAIHelp() bool {
	fmt.Println()
	fmt.Println(lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Bold(true).
		Render("🤖 AI-powered error recovery available!"))
	fmt.Print("   Would you like help fixing these errors? (y/n): ")

	var response string
	fmt.Scanln(&response)
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// getAIErrorSuggestions calls the Anthropic API to get error fix suggestions
func getAIErrorSuggestions(ctx ErrorContext) (string, []string, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return "", nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
	)

	// Build the error context
	errorText := strings.Join(ctx.Errors, "\n\n")

	// Construct the prompt
	prompt := fmt.Sprintf(`You are an AWS infrastructure expert helping debug Terraform errors.

Context:
- Operation: %s
- Environment: %s
- AWS Profile: %s
- AWS Region: %s
- Working Directory: %s

Errors:
%s

Please provide:
1. A brief explanation of what's wrong (2-3 sentences max)
2. Exact CLI commands to fix it

Format your response EXACTLY as:
PROBLEM:
<explanation>

COMMANDS:
<command 1>
<command 2>
<etc>

Important:
- Start commands with "cd %s" if needed
- Include "export AWS_PROFILE=%s" if AWS commands are needed
- Provide working, tested commands
- Be concise and specific`,
		ctx.Operation,
		ctx.Environment,
		ctx.AWSProfile,
		ctx.AWSRegion,
		ctx.WorkingDir,
		errorText,
		ctx.WorkingDir,
		ctx.AWSProfile,
	)

	fmt.Println()
	fmt.Println(lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Render("🔍 Analyzing errors with AI..."))

	// Call the API
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	message, err := client.Messages.New(timeoutCtx, anthropic.MessageNewParams{
		Model:     "claude-sonnet-4-5-20250929", // Latest Claude Sonnet 4.5 - best for coding and agents
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})

	if err != nil {
		return "", nil, fmt.Errorf("API call failed: %w", err)
	}

	// Extract the response
	if len(message.Content) == 0 {
		return "", nil, fmt.Errorf("empty response from API")
	}

	responseText := message.Content[0].Text

	// Parse the response
	problem, commands := parseAIResponse(responseText)

	return problem, commands, nil
}

// parseAIResponse parses the AI response into problem description and commands
func parseAIResponse(response string) (string, []string) {
	lines := strings.Split(response, "\n")
	var problem strings.Builder
	var commands []string
	inProblem := false
	inCommands := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "PROBLEM:") {
			inProblem = true
			inCommands = false
			continue
		}

		if strings.HasPrefix(line, "COMMANDS:") {
			inProblem = false
			inCommands = true
			continue
		}

		if inProblem && line != "" {
			if problem.Len() > 0 {
				problem.WriteString(" ")
			}
			problem.WriteString(line)
		}

		if inCommands && line != "" && !strings.HasPrefix(line, "#") {
			commands = append(commands, line)
		}
	}

	return problem.String(), commands
}

// displayAISuggestions formats and displays the AI suggestions
func displayAISuggestions(problem string, commands []string) {
	displayAISuggestionsWithContext(problem, commands, nil)
}

// displayAISuggestionsWithContext formats and displays the AI suggestions along with the original error
func displayAISuggestionsWithContext(problem string, commands []string, originalErrors []string) {
	divider := "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

	// Show original error first if provided
	if len(originalErrors) > 0 {
		fmt.Println()
		fmt.Println(divider)
		fmt.Println(lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true).
			Render("❌ Original Error"))
		fmt.Println(divider)
		fmt.Println()

		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Width(100)

		for _, err := range originalErrors {
			fmt.Println(errorStyle.Render(err))
			fmt.Println()
		}
	}

	fmt.Println()
	fmt.Println(divider)
	fmt.Println(lipgloss.NewStyle().
		Foreground(lipgloss.Color("226")).
		Bold(true).
		Render("💡 AI Analysis"))
	fmt.Println(divider)
	fmt.Println()

	// Word wrap the problem description
	problemStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Width(70)
	fmt.Println(problemStyle.Render(problem))

	fmt.Println()
	fmt.Println(divider)
	fmt.Println(lipgloss.NewStyle().
		Foreground(lipgloss.Color("82")).
		Bold(true).
		Render("📋 Suggested Fix"))
	fmt.Println(divider)
	fmt.Println()
	fmt.Println("Run these commands:")
	fmt.Println()

	// Display commands with syntax highlighting
	commandStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		PaddingLeft(2)

	for _, cmd := range commands {
		fmt.Println(commandStyle.Render(cmd))
	}

	fmt.Println()
	fmt.Println(divider)
	fmt.Println()

	// Disclaimer
	disclaimerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Italic(true)
	fmt.Println(disclaimerStyle.Render("⚠️  AI-generated suggestions - please review before running"))
	fmt.Println()
}

// offerAIHelp is the main entry point for AI error assistance
func offerAIHelp(ctx ErrorContext) {
	if !isAIHelperAvailable() {
		return
	}

	if !promptForAIHelp() {
		return
	}

	problem, commands, err := getAIErrorSuggestions(ctx)
	if err != nil {
		fmt.Printf("\n❌ Failed to get AI suggestions: %v\n", err)
		return
	}

	displayAISuggestions(problem, commands)

	fmt.Print("Press Enter to continue...")
	fmt.Scanln()
}
