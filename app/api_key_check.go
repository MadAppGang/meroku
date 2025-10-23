package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
)

// CheckAnthropicAPIKey checks if the ANTHROPIC_API_KEY is set
// Returns true if key is present, false otherwise
func CheckAnthropicAPIKey() bool {
	return os.Getenv("ANTHROPIC_API_KEY") != ""
}

// ShowAPIKeyRequiredScreen displays a friendly message when API key is missing
func ShowAPIKeyRequiredScreen() {
	// Define colors and styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FF6B9D")).
		MarginTop(1).
		MarginBottom(1)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#874BFD")).
		Padding(1, 2).
		Width(70)

	highlightStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4"))

	linkStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#04B575")).
		Underline(true)

	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFAA00")).
		Bold(true)

	// ASCII Art
	art := `
   ╔═══════════════════════════════════════════════════════════════════╗
   ║                                                                   ║
   ║     ███╗   ███╗███████╗██████╗  ██████╗ ██╗  ██╗██╗   ██╗       ║
   ║     ████╗ ████║██╔════╝██╔══██╗██╔═══██╗██║ ██╔╝██║   ██║       ║
   ║     ██╔████╔██║█████╗  ██████╔╝██║   ██║█████╔╝ ██║   ██║       ║
   ║     ██║╚██╔╝██║██╔══╝  ██╔══██╗██║   ██║██╔═██╗ ██║   ██║       ║
   ║     ██║ ╚═╝ ██║███████╗██║  ██║╚██████╔╝██║  ██╗╚██████╔╝       ║
   ║     ╚═╝     ╚═╝╚══════╝╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═╝ ╚═════╝        ║
   ║                                                                   ║
   ║              🤖  AI-Powered Infrastructure Management  🚀         ║
   ║                                                                   ║
   ╚═══════════════════════════════════════════════════════════════════╝
`

	fmt.Println(titleStyle.Render(art))

	// Information box
	infoContent := fmt.Sprintf(`%s

Meroku comes packed with powerful AI capabilities powered by Claude:

  %s Autonomous Error Resolution
    The AI agent can investigate and fix deployment errors automatically
    using advanced reasoning and AWS CLI commands

  %s Intelligent Infrastructure Analysis
    Get smart insights about your infrastructure configuration,
    cost optimization opportunities, and security recommendations

  %s Interactive Troubleshooting
    Ask questions about your deployments, get explanations of errors,
    and receive step-by-step guidance for complex tasks

  %s Terraform Plan Analysis
    Understand complex infrastructure changes with AI-powered
    explanations of what will happen before you apply


%s
To unlock these features, you'll need an Anthropic API key.

%s Get your API key here:
   %s

%s Set it as an environment variable:
   export ANTHROPIC_API_KEY=your_key_here

   Or add it to your shell profile (~/.bashrc, ~/.zshrc):
   echo 'export ANTHROPIC_API_KEY=your_key_here' >> ~/.zshrc


%s You can still use Meroku without an API key, but AI features will be unavailable.
Press Enter to continue...`,
		warningStyle.Render("⚠️  ANTHROPIC_API_KEY NOT FOUND"),
		highlightStyle.Render("🔍"),
		highlightStyle.Render("💡"),
		highlightStyle.Render("🛠️"),
		highlightStyle.Render("📊"),
		titleStyle.Render("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"),
		highlightStyle.Render("Step 1:"),
		linkStyle.Render("https://console.anthropic.com/settings/keys"),
		highlightStyle.Render("Step 2:"),
		warningStyle.Render("Note:"),
	)

	fmt.Println(boxStyle.Render(infoContent))
	fmt.Println()

	// Wait for user to press Enter
	fmt.Scanln()
}
