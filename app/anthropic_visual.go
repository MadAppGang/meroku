package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const visualizationPrompt = `I have a Terraform plan output in JSON format that shows infrastructure changes. Please create a comprehensive, interactive HTML visualization that displays:

1. **Summary Statistics**: Show total changes, creates, updates, and deletes in a visually appealing header with color-coded numbers

2. **Detailed Breakdown by Change Type**:
   - Group all deletions together with details about what's being removed
   - Show updates with before/after comparisons 
   - List new resources being created
   - Use clear icons and color coding (red for delete, blue for update, green for create)

3. **Visual Requirements**:
   - Dark theme with good contrast
   - Hover effects on resource items
   - Color-coded badges for change types
   - Show old values in red with strikethrough or background, new values in green
   - Use monospace font for technical values like domain names, IDs, and configuration values

4. **Information to Display for EACH Resource**:
   - Resource type and name (e.g., aws_instance.example)
   - Full resource address
   - Key configuration changes (for updates show before → after)
   - For creates: show main configuration values
   - For deletes: show what configuration is being removed
   - Group related resources together when possible

5. **Special Formatting**:
   - Domain/URL changes with clear old→new formatting
   - Infrastructure consolidation details
   - Use expandable/collapsible sections for resources with many attributes
   - Show JSON values in a readable format (not raw JSON strings)

CRITICAL: Make it easy to scan quickly but also include ALL the technical details. Every single resource in the JSON must be shown in the visualization. The visualization should help both technical and non-technical stakeholders understand what's changing.

Output ONLY the complete HTML code with all CSS and JavaScript inline. Start with <!DOCTYPE html> and create a single, self-contained file.`

type anthropicStreamRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
	Stream    bool               `json:"stream"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type streamEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Text string `json:"text"`
	} `json:"delta,omitempty"`
	Message struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message,omitempty"`
}

func callAnthropicForVisualizationWithProgress(planData interface{}) error {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY environment variable not set")
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Handle cancellation in a goroutine
	go func() {
		select {
		case <-sigChan:
			fmt.Println("\n\n🛑 Cancelling AI visualization generation...")
			cancel()
		case <-ctx.Done():
			return
		}
	}()

	fmt.Println("\n🤖 Asking AI to explain your infrastructure changes...")
	fmt.Println("   Press Ctrl+C to cancel")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Convert plan data to JSON
	planJSON, err := json.MarshalIndent(planData, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling plan data: %w", err)
	}

	// Always save the JSON being sent for debugging
	debugFile := "terraform-ai-request.json"
	os.WriteFile(debugFile, planJSON, 0o644)
	fmt.Printf("📝 Debug: Saved request data to %s (%d bytes)\n", debugFile, len(planJSON))

	// Also save a sample to see structure
	if len(planJSON) > 1000 {
		sampleFile := "terraform-ai-request-sample.json"
		os.WriteFile(sampleFile, planJSON[:1000], 0o644)
		fmt.Printf("📝 Debug: Saved sample (first 1000 bytes) to %s\n", sampleFile)
	}

	// Show summary of what we're sending
	// Use reflection or convert to map to access the data
	jsonBytes, _ := json.Marshal(planData)
	var planMap map[string]interface{}
	json.Unmarshal(jsonBytes, &planMap)

	if summaryData, ok := planMap["summary"].(map[string]interface{}); ok {
		fmt.Printf("\n📊 Plan Summary:\n")
		if total, ok := summaryData["total"].(float64); ok {
			fmt.Printf("   • Total changes: %d\n", int(total))
		}
		if create, ok := summaryData["create"].(float64); ok && create > 0 {
			fmt.Printf("   • ✚ Create: %d resources\n", int(create))
		}
		if update, ok := summaryData["update"].(float64); ok && update > 0 {
			fmt.Printf("   • ~ Update: %d resources\n", int(update))
		}
		if delete, ok := summaryData["delete"].(float64); ok && delete > 0 {
			fmt.Printf("   • ✗ Delete: %d resources\n", int(delete))
		}
		if replace, ok := summaryData["replace"].(float64); ok && replace > 0 {
			fmt.Printf("   • ↻ Replace: %d resources\n", int(replace))
		}
	}

	fmt.Println("\n🎨 Generating visualization...")

	// Create the request with streaming enabled
	reqBody := anthropicStreamRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 8192,
		Stream:    true,
		Messages: []anthropicMessage{
			{
				Role:    "user",
				Content: fmt.Sprintf("%s\n\nHere is the Terraform plan data to visualize:\n\n%s", visualizationPrompt, string(planJSON)),
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("error marshaling request: %w", err)
	}

	// Make the API request with context
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Process the streaming response
	var htmlContent strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	progressChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	charIndex := 0
	charCount := 0

	fmt.Print("\n")

	for scanner.Scan() {
		// Check if context was cancelled
		select {
		case <-ctx.Done():
			fmt.Println("\n\n❌ AI visualization generation cancelled")
			return fmt.Errorf("cancelled by user")
		default:
			// Continue processing
		}

		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		// Parse SSE data
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			// Skip the [DONE] message
			if data == "[DONE]" {
				continue
			}

			var event streamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			// Handle content delta
			if event.Type == "content_block_delta" && event.Delta.Text != "" {
				htmlContent.WriteString(event.Delta.Text)
				charCount += len(event.Delta.Text)

				// Show progress
				fmt.Printf("\r%s Generating HTML... (%d characters) [Press Ctrl+C to cancel]", progressChars[charIndex], charCount)
				charIndex = (charIndex + 1) % len(progressChars)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stream: %w", err)
	}

	// Check if we were cancelled
	select {
	case <-ctx.Done():
		return fmt.Errorf("cancelled by user")
	default:
		// Continue with normal flow
	}

	fmt.Printf("\r✅ Generated HTML (%d characters)                                        \n", charCount)

	// Only proceed if we have content
	if htmlContent.Len() == 0 {
		return fmt.Errorf("no HTML content generated")
	}

	// Save the HTML to a temporary file
	tmpFile, err := os.CreateTemp("", "terraform-visual-*.html")
	if err != nil {
		return fmt.Errorf("error creating temp file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(htmlContent.String()); err != nil {
		return fmt.Errorf("error writing HTML: %w", err)
	}

	fmt.Printf("📄 Saved to: %s\n", tmpFile.Name())

	// Open the HTML file in the default browser
	fmt.Print("🌐 Opening in browser...")
	if err := openBrowser(tmpFile.Name()); err != nil {
		return fmt.Errorf("error opening browser: %w", err)
	}
	fmt.Println(" Done!")

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("✨ AI explanation generated successfully!")
	fmt.Println("\nPress ENTER to return to the TUI...")
	
	// Use a channel to handle ENTER or cancellation
	done := make(chan bool)
	go func() {
		fmt.Scanln()
		done <- true
	}()

	select {
	case <-done:
		// User pressed ENTER
	case <-ctx.Done():
		// User cancelled
		fmt.Println("\nCancelled. Returning to TUI...")
	}

	// Clear screen before returning to TUI
	fmt.Print("\033[H\033[2J")

	return nil
}

// Keep the original function for backward compatibility
func callAnthropicForVisualization(planData interface{}) error {
	return callAnthropicForVisualizationWithProgress(planData)
}

func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // linux and others
		cmd = "xdg-open"
		args = []string{url}
	}

	return exec.Command(cmd, args...).Start()
}
