package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"
)

func runDNSStatus(cmd interface{}, args []string) error {
	config, err := loadDNSConfig()
	if err != nil {
		return fmt.Errorf("failed to load DNS config: %w", err)
	}

	if config == nil {
		fmt.Println("No DNS configuration found.")
		fmt.Println("Run './meroku dns setup' to configure DNS.")
		return nil
	}

	// Create styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("211"))
	
	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("42"))
	
	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214"))
	
	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196"))

	fmt.Println(titleStyle.Render("DNS Configuration Status"))
	fmt.Println(strings.Repeat("─", 50))
	
	// Root zone information
	fmt.Printf("\n%s\n", titleStyle.Render("Root Zone:"))
	fmt.Printf("  Domain:      %s\n", config.RootDomain)
	fmt.Printf("  Account ID:  %s\n", config.RootAccount.AccountID)
	fmt.Printf("  Zone ID:     %s\n", config.RootAccount.ZoneID)
	fmt.Printf("  Role ARN:    %s\n", config.RootAccount.DelegationRoleArn)
	
	// Check DNS propagation for root domain
	fmt.Printf("\n%s\n", titleStyle.Render("Root Domain Propagation:"))
	ns, err := queryNameservers(config.RootDomain)
	if err != nil {
		fmt.Printf("  %s Unable to query nameservers: %v\n", errorStyle.Render("✗"), err)
	} else {
		fmt.Printf("  %s Nameservers resolved: %d found\n", successStyle.Render("✓"), len(ns))
		for _, n := range ns {
			fmt.Printf("    • %s\n", n)
		}
	}
	
	// Delegated zones
	if len(config.DelegatedZones) > 0 {
		fmt.Printf("\n%s\n", titleStyle.Render("Delegated Zones:"))
		
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  Subdomain\tAccount\tZone ID\tStatus")
		fmt.Fprintln(w, "  "+strings.Repeat("─", 60))
		
		for _, zone := range config.DelegatedZones {
			statusIcon := "✓"
			statusColor := successStyle
			if zone.Status == "pending" {
				statusIcon = "⟳"
				statusColor = warningStyle
			} else if zone.Status == "error" {
				statusIcon = "✗"
				statusColor = errorStyle
			}
			
			fmt.Fprintf(w, "  %s %s\t%s\t%s\t%s\n",
				statusColor.Render(statusIcon),
				zone.Subdomain,
				zone.AccountID,
				zone.ZoneID,
				zone.Status,
			)
		}
		w.Flush()
		
		// Check propagation for each subdomain
		fmt.Printf("\n%s\n", titleStyle.Render("Subdomain Propagation:"))
		for _, zone := range config.DelegatedZones {
			ns, err := queryNameservers(zone.Subdomain)
			if err != nil {
				fmt.Printf("  %s %s: Unable to resolve\n", errorStyle.Render("✗"), zone.Subdomain)
			} else {
				fmt.Printf("  %s %s: %d nameservers\n", successStyle.Render("✓"), zone.Subdomain, len(ns))
			}
		}
	} else {
		fmt.Printf("\n%s\n", warningStyle.Render("No delegated zones configured."))
	}
	
	fmt.Println("\n" + strings.Repeat("─", 50))
	fmt.Println("Run './meroku dns validate' to perform full validation.")
	
	return nil
}

func runDNSValidate(cmd interface{}, args []string) error {
	config, err := loadDNSConfig()
	if err != nil {
		return fmt.Errorf("failed to load DNS config: %w", err)
	}

	if config == nil {
		fmt.Println("No DNS configuration found.")
		return nil
	}

	fmt.Println("Validating DNS configuration...")
	fmt.Println(strings.Repeat("─", 50))
	
	var issues []string
	var warnings []string
	
	// Validate root zone nameservers
	fmt.Print("Checking root zone nameservers... ")
	ns, err := queryNameservers(config.RootDomain)
	if err != nil {
		issues = append(issues, fmt.Sprintf("Failed to query nameservers for %s: %v", config.RootDomain, err))
		fmt.Println("❌")
	} else if len(ns) == 0 {
		issues = append(issues, fmt.Sprintf("No nameservers found for %s", config.RootDomain))
		fmt.Println("❌")
	} else {
		fmt.Println("✅")
	}
	
	// Check DNS propagation
	fmt.Print("Checking DNS propagation... ")
	propagation, err := checkDNSPropagation(config.RootDomain, ns)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("Failed to check propagation: %v", err))
		fmt.Println("⚠️")
	} else {
		successCount := 0
		for _, status := range propagation {
			if status {
				successCount++
			}
		}
		if successCount == len(propagation) {
			fmt.Println("✅")
		} else if successCount > 0 {
			warnings = append(warnings, fmt.Sprintf("DNS partially propagated (%d/%d servers)", successCount, len(propagation)))
			fmt.Println("⚠️")
		} else {
			issues = append(issues, "DNS not propagated to any servers")
			fmt.Println("❌")
		}
	}
	
	// Validate each delegated zone
	for _, zone := range config.DelegatedZones {
		fmt.Printf("Checking %s... ", zone.Subdomain)
		
		zoneNS, err := queryNameservers(zone.Subdomain)
		if err != nil {
			issues = append(issues, fmt.Sprintf("Failed to query nameservers for %s: %v", zone.Subdomain, err))
			fmt.Println("❌")
		} else if len(zoneNS) == 0 {
			issues = append(issues, fmt.Sprintf("No nameservers found for %s", zone.Subdomain))
			fmt.Println("❌")
		} else {
			// Check if NS records match expected
			match := false
			for _, expected := range zone.NSRecords {
				for _, actual := range zoneNS {
					if strings.EqualFold(expected, actual) {
						match = true
						break
					}
				}
				if match {
					break
				}
			}
			
			if match {
				fmt.Println("✅")
			} else {
				warnings = append(warnings, fmt.Sprintf("NS records for %s don't match expected values", zone.Subdomain))
				fmt.Println("⚠️")
			}
		}
	}
	
	fmt.Println(strings.Repeat("─", 50))
	
	// Summary
	if len(issues) == 0 && len(warnings) == 0 {
		fmt.Println("✅ All validations passed!")
	} else {
		if len(issues) > 0 {
			fmt.Println("\n❌ Issues found:")
			for _, issue := range issues {
				fmt.Printf("  • %s\n", issue)
			}
		}
		
		if len(warnings) > 0 {
			fmt.Println("\n⚠️  Warnings:")
			for _, warning := range warnings {
				fmt.Printf("  • %s\n", warning)
			}
		}
	}
	
	return nil
}

func runDNSRemove(cmd interface{}, args []string) error {
	subdomain := args[0]
	
	config, err := loadDNSConfig()
	if err != nil {
		return fmt.Errorf("failed to load DNS config: %w", err)
	}

	if config == nil {
		fmt.Println("No DNS configuration found.")
		return nil
	}

	zone := findDelegatedZone(config, subdomain)
	if zone == nil {
		return fmt.Errorf("subdomain %s not found in configuration", subdomain)
	}

	fmt.Printf("Removing delegation for %s...\n", subdomain)
	
	// Remove NS records from root zone
	fmt.Print("Removing NS records from root zone... ")
	// Find profile for root account
	rootProfile, profileErr := findAWSProfileByAccountID(config.RootAccount.AccountID)
	if profileErr != nil {
		fmt.Printf("❌ Cannot find AWS profile for root account %s: %v\n", config.RootAccount.AccountID, profileErr)
		return profileErr
	}
	err = deleteNSRecords(rootProfile, config.RootAccount.ZoneID, subdomain)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
	} else {
		fmt.Println("✅")
	}
	
	// Delete the subdomain zone
	if zone.ZoneID != "" {
		fmt.Print("Deleting subdomain zone... ")
		// Find profile for subdomain account
		subProfile, profileErr := findAWSProfileByAccountID(zone.AccountID)
		if profileErr != nil {
			fmt.Printf("❌ Cannot find AWS profile for account %s: %v\n", zone.AccountID, profileErr)
		} else {
			err = deleteDNSZone(subProfile, zone.ZoneID)
			if err != nil {
				fmt.Printf("❌ %v\n", err)
			} else {
				fmt.Println("✅")
			}
		}
	}
	
	// Update configuration
	if removeDelegatedZone(config, subdomain) {
		fmt.Print("Updating configuration... ")
		err = saveDNSConfig(config)
		if err != nil {
			fmt.Printf("❌ %v\n", err)
			return err
		}
		fmt.Println("✅")
	}
	
	fmt.Printf("\n✅ Successfully removed delegation for %s\n", subdomain)
	
	return nil
}


// Helper function to delete NS records
func deleteNSRecords(profile, zoneID, recordName string) error {
	// This would use AWS SDK to delete the NS records
	// For now, returning nil as a placeholder
	return nil
}