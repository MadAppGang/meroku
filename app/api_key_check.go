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
		Foreground(adapt("#db2777", "#FF6B9D")).
		MarginTop(1).
		MarginBottom(1)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(adapt("#6d28d9", "#874BFD")).
		Padding(1, 2).
		Width(70)

	highlightStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(adapt("#6d28d9", "#7D56F4"))

	linkStyle := lipgloss.NewStyle().
		Foreground(adapt("#038256", "#04B575")).
		Underline(true)

	warningStyle := lipgloss.NewStyle().
		Foreground(adapt("#b45309", "#FFAA00")).
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

Meroku includes powerful AI capabilities powered by Claude:

  %s Autonomous Error Resolution
    Automatically investigate and fix deployment errors using
    advanced reasoning and AWS CLI commands

  %s Intelligent Infrastructure Analysis
    Get insights about configuration, cost optimization,
    and security recommendations

  %s Interactive Troubleshooting
    Ask questions about deployments, get error explanations,
    and receive step-by-step guidance

  %s Terraform Plan Analysis
    Understand complex changes with AI-powered explanations
    before you apply


%s
To unlock these features, get an Anthropic API key.

%s Get your API key:
   %s

%s Set it as an environment variable:
   export ANTHROPIC_API_KEY=your_key_here

   Or add to your shell profile (~/.bashrc, ~/.zshrc):
   echo 'export ANTHROPIC_API_KEY=your_key_here' >> ~/.zshrc


%s Meroku works without an API key, but AI features
will be unavailable. Press Enter to continue...`,
		warningStyle.Render("⚠️  ANTHROPIC_API_KEY NOT FOUND"),
		highlightStyle.Render("🔍"),
		highlightStyle.Render("💡"),
		highlightStyle.Render("🛠️"),
		highlightStyle.Render("📊"),
		titleStyle.Render("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"),
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
