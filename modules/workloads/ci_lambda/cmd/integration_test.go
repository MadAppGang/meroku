package main

import (
	"encoding/json"
	"fmt"
	"os"

	"madappgang.com/infrastructure/ci_lambda/config"
	"madappgang.com/infrastructure/ci_lambda/services"
	"madappgang.com/infrastructure/ci_lambda/utils"
)

// Integration test that verifies:
// 1. Configuration loads correctly
// 2. All services in config exist in ECS
// 3. All task definitions exist
// 4. Cluster exists and is accessible

func main() {
	fmt.Println("🧪 Running Lambda Integration Tests...")
	fmt.Println()

	// Load configuration
	fmt.Println("📋 Step 1: Loading configuration from environment variables")
	cfg, err := config.LoadFromEnv()
	if err != nil {
		fmt.Printf("❌ FAILED: Configuration load error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Configuration loaded successfully\n")
	fmt.Printf("   Project: %s\n", cfg.ProjectName)
	fmt.Printf("   Environment: %s\n", cfg.Environment)
	fmt.Printf("   Cluster: %s\n", cfg.GetClusterName())
	fmt.Printf("   Services configured: %d\n", len(cfg.ServiceMap))
	fmt.Println()

	// Initialize logger
	logger := utils.NewLogger(cfg)
	logger.Info("Integration test started", map[string]interface{}{
		"project": cfg.ProjectName,
		"env":     cfg.Environment,
	})

	// Initialize ECS service
	fmt.Println("🔌 Step 2: Connecting to AWS ECS")
	ecsSvc, err := services.NewECSServiceV2(cfg, logger)
	if err != nil {
		fmt.Printf("❌ FAILED: Cannot connect to ECS: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Connected to ECS in region %s\n", cfg.AWSRegion)
	fmt.Println()

	// List all services from config
	fmt.Println("📝 Step 3: Listing all configured services")
	allServices, err := ecsSvc.ListAllServices()
	if err != nil {
		fmt.Printf("❌ FAILED: Cannot list services: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d services in configuration:\n", len(allServices))
	for serviceID, info := range allServices {
		infoJSON, _ := json.MarshalIndent(info, "   ", "  ")
		displayID := serviceID
		if displayID == "" {
			displayID = "(backend)"
		}
		fmt.Printf("   • %s\n%s\n", displayID, string(infoJSON))
	}
	fmt.Println()

	// Verify each service exists in ECS
	fmt.Println("✅ Step 4: Verifying all services exist in ECS")
	var failedServices []string
	successCount := 0

	for _, serviceID := range cfg.ListAllServices() {
		displayID := serviceID
		if displayID == "" {
			displayID = "(backend)"
		}

		mapping, _ := cfg.GetServiceMapping(serviceID)
		fmt.Printf("   Checking service: %s → %s...", displayID, mapping.ServiceName)

		err := ecsSvc.VerifyServiceExists(serviceID)
		if err != nil {
			fmt.Printf(" ❌ FAILED\n")
			fmt.Printf("      Error: %v\n", err)
			failedServices = append(failedServices, displayID)
		} else {
			fmt.Printf(" ✅ OK\n")
			successCount++
		}
	}
	fmt.Println()

	// Summary
	fmt.Println("📊 Test Summary")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Total services:    %d\n", len(cfg.ServiceMap))
	fmt.Printf("Verified:          %d\n", successCount)
	fmt.Printf("Failed:            %d\n", len(failedServices))
	fmt.Println()

	if len(failedServices) > 0 {
		fmt.Println("❌ INTEGRATION TEST FAILED")
		fmt.Println("Failed services:")
		for _, svc := range failedServices {
			fmt.Printf("   • %s\n", svc)
		}
		fmt.Println()
		fmt.Println("Please check:")
		fmt.Println("  1. ECS_SERVICE_MAP environment variable is correct")
		fmt.Println("  2. All services have been deployed to ECS")
		fmt.Println("  3. AWS credentials and region are correct")
		os.Exit(1)
	}

	fmt.Println("✅ ALL INTEGRATION TESTS PASSED!")
	fmt.Println()
	fmt.Println("The Lambda function is ready to deploy services:")
	for _, serviceID := range cfg.ListAllServices() {
		displayID := serviceID
		if displayID == "" {
			displayID = "backend"
		}
		mapping, _ := cfg.GetServiceMapping(serviceID)
		fmt.Printf("   ✓ %s (%s)\n", displayID, mapping.ServiceName)
	}
	fmt.Println()

	// Optional: Test S3 mappings if configured
	if len(cfg.S3ToServiceMap) > 0 {
		fmt.Println("📦 S3 File Mappings:")
		for serviceID, files := range cfg.S3ToServiceMap {
			displayID := serviceID
			if displayID == "" {
				displayID = "backend"
			}
			fmt.Printf("   %s:\n", displayID)
			for _, file := range files {
				fmt.Printf("      - s3://%s/%s\n", file.Bucket, file.Key)
			}
		}
		fmt.Println()
	}

	logger.Info("Integration test completed successfully", map[string]interface{}{
		"total_services":   len(cfg.ServiceMap),
		"verified_services": successCount,
	})
}
