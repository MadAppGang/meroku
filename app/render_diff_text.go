package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// renderDiffToText renders the terraform plan diff as text output (non-interactive)
func renderDiffToText(planFile string) error {
	// Read the plan file
	planData, err := os.ReadFile(planFile)
	if err != nil {
		return fmt.Errorf("failed to read plan file: %w", err)
	}

	var plan TerraformPlanVisual
	if err := json.Unmarshal(planData, &plan); err != nil {
		return fmt.Errorf("failed to parse terraform plan JSON: %w", err)
	}

	// Header
	fmt.Println("\n╔══════════════════════════════════════════════════════════════════════╗")
	fmt.Printf("║                    TERRAFORM PLAN DIFF VIEWER                       ║\n")
	fmt.Printf("║                        Meroku v3.5.6                                ║\n")
	fmt.Printf("╚══════════════════════════════════════════════════════════════════════╝\n\n")

	// Count changes
	createCount := 0
	updateCount := 0
	deleteCount := 0
	replaceCount := 0

	for _, change := range plan.ResourceChanges {
		if len(change.Change.Actions) == 2 && 
		   change.Change.Actions[0] == "delete" && 
		   change.Change.Actions[1] == "create" {
			replaceCount++
		} else if len(change.Change.Actions) > 0 {
			switch change.Change.Actions[0] {
			case "create":
				createCount++
			case "update":
				updateCount++
			case "delete":
				deleteCount++
			}
		}
	}

	// Summary
	fmt.Printf("📊 SUMMARY: %d to add, %d to change, %d to destroy, %d to replace\n\n",
		createCount, updateCount, deleteCount, replaceCount)

	// Group by provider
	providerMap := make(map[string][]ResourceChange)
	for _, change := range plan.ResourceChanges {
		if len(change.Change.Actions) == 0 || 
		   (len(change.Change.Actions) == 1 && (change.Change.Actions[0] == "no-op" || change.Change.Actions[0] == "read")) {
			continue
		}
		
		parts := strings.Split(change.ProviderName, ".")
		provider := parts[len(parts)-1]
		if strings.Contains(provider, "/") {
			provider = strings.Split(provider, "/")[1]
		}
		
		// Handle replacements - show both delete and create
		if len(change.Change.Actions) == 2 && 
		   change.Change.Actions[0] == "delete" && 
		   change.Change.Actions[1] == "create" {
			// Add as replacement
			providerMap[provider] = append(providerMap[provider], change)
		} else {
			providerMap[provider] = append(providerMap[provider], change)
		}
	}

	// Display changes by provider
	for provider, changes := range providerMap {
		fmt.Printf("━━━ Provider: %s (%d changes) ━━━\n\n", provider, len(changes))
		
		for _, change := range changes {
			// Determine action and icon
			var action, icon string
			if len(change.Change.Actions) == 2 && 
			   change.Change.Actions[0] == "delete" && 
			   change.Change.Actions[1] == "create" {
				action = "REPLACE"
				icon = "🔄"
			} else {
				switch change.Change.Actions[0] {
				case "create":
					action = "CREATE"
					icon = "✅"
				case "update":
					action = "UPDATE"
					icon = "📝"
				case "delete":
					action = "DELETE"
					icon = "❌"
				default:
					action = strings.ToUpper(change.Change.Actions[0])
					icon = "•"
				}
			}
			
			fmt.Printf("%s %s: %s (%s)\n", icon, action, change.Address, change.Type)
			
			// Show key changes for updates and replacements
			if action == "UPDATE" || action == "REPLACE" {
				if change.Change.Before != nil && change.Change.After != nil {
					fmt.Println("  Changes:")
					
					// Find what's changing
					for key, beforeVal := range change.Change.Before {
						if afterVal, exists := change.Change.After[key]; exists {
							beforeStr := fmt.Sprintf("%v", beforeVal)
							afterStr := fmt.Sprintf("%v", afterVal)
							if beforeStr != afterStr && len(beforeStr) < 100 && len(afterStr) < 100 {
								fmt.Printf("    %s: %s → %s\n", key, beforeStr, afterStr)
							}
						}
					}
					
					// Find new keys
					for key, afterVal := range change.Change.After {
						if _, exists := change.Change.Before[key]; !exists {
							afterStr := fmt.Sprintf("%v", afterVal)
							if len(afterStr) < 100 {
								fmt.Printf("    + %s: %s\n", key, afterStr)
							}
						}
					}
				}
			}
			
			// Show what will be created
			if action == "CREATE" && change.Change.After != nil {
				fmt.Println("  Configuration:")
				count := 0
				for key, val := range change.Change.After {
					if count >= 5 {
						fmt.Printf("    ... and %d more attributes\n", len(change.Change.After)-5)
						break
					}
					valStr := fmt.Sprintf("%v", val)
					if len(valStr) > 60 {
						valStr = valStr[:57] + "..."
					}
					fmt.Printf("    %s: %s\n", key, valStr)
					count++
				}
			}
			
			fmt.Println()
		}
	}

	fmt.Println("════════════════════════════════════════════════════════════════════")
	fmt.Println("Use arrow keys to navigate in the full TUI version")
	fmt.Println("Run without --renderdiff flag for interactive mode")
	
	return nil
}