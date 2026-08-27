package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

// Schema version history:
// 1: Initial version (no version field)
// 2: Added Aurora Serverless v2 support (aurora, min_capacity, max_capacity)
// 3: Added DNS management fields (zone_id, root_zone_id, etc.)
// 4: Added backend scaling configuration
// 5: Added ALB configuration
// 6: Added custom VPC configuration
// 7: Added ECR strategy configuration (ecr_strategy, ecr_account_id, ecr_account_region)
// 8: Added ECR trusted accounts for cross-account access (ecr_trusted_accounts)
// 9: Simplified Amplify domain configuration (subdomain_prefix replaces custom_domain + enable_root_domain)
// 10: Added per-service ECR configuration (ecr_config field in services, event_processor_tasks, scheduled_tasks)
// 11: Ensure host_port matches container_port for services (required for awsvpc network mode)
// 12: Ensure all postgres boolean fields have explicit default values
// 13: Multi-rule EventBridge support (rules[] array in event_processor_tasks)
// 14: CloudFront CDN configuration (cloudfront with origins, cache_behaviors, domain_aliases)
// 15: Multiple CloudFront distributions support (cloudfront_distributions[] array)
// 16: DynamoDB state locking support (state_lock_table field)
// 17: Multi-domain support for SES (domains[] array with optional zone_id per domain)
// 18: Move test_emails to global SES level (account-wide, not per-domain)
// 19: Add enabled field to services (default true, allows disabling without removing config)
// 20: Add enabled field to scheduled_tasks and event_processor_tasks (same pattern as services)
// 21: Add AppSync Lambda authorizer configuration (jwks_uri, jwt_issuer, jwt_audience)
// 22: Add auto_deploy to the backend, services and scheduled_tasks (CI/CD Lambda policy)
// 23: Add AppSync auth_mode (cognito/oidc/lambda) and explicit api_key_enabled
// 24: Repair the misspelt cognito.dashboard_callback_ur_ls key
// 25: Normalize scheduled_tasks[].container_command from a scalar to list(string)
// 26: Add runtime (fargate|ec2) and compute_pool to the backend and services, for EC2 capacity pools
// 27: Add egress_strategy (public_ip|nat_gateway|nat_gateway_ha) for how tasks reach the internet
// 28: Add workload.github_oidc_create_provider, for sharing one account's GitHub OIDC provider
const CurrentSchemaVersion = 28

// EnvWithVersion extends Env with a schema version field
type EnvWithVersion struct {
	SchemaVersion int `yaml:"schema_version,omitempty"`
	Env
}

// Migration represents a single migration step
type Migration struct {
	Version     int
	Description string
	Apply       func(data map[string]interface{}) error
}

// AllMigrations contains all available migrations in order
var AllMigrations = []Migration{
	{
		Version:     2,
		Description: "Add Aurora Serverless v2 support and ALB configuration",
		Apply:       migrateToV2,
	},
	{
		Version:     3,
		Description: "Add DNS management fields",
		Apply:       migrateToV3,
	},
	{
		Version:     4,
		Description: "Add backend scaling configuration",
		Apply:       migrateToV4,
	},
	{
		Version:     5,
		Description: "Add Account ID and AWS Profile fields",
		Apply:       migrateToV5,
	},
	{
		Version:     6,
		Description: "Add custom VPC configuration",
		Apply:       migrateToV6,
	},
	{
		Version:     7,
		Description: "Add ECR strategy configuration",
		Apply:       migrateToV7,
	},
	{
		Version:     8,
		Description: "Add ECR trusted accounts for cross-account access",
		Apply:       migrateToV8,
	},
	{
		Version:     9,
		Description: "Simplify Amplify domain configuration",
		Apply:       migrateToV9,
	},
	{
		Version:     10,
		Description: "Add per-service ECR configuration",
		Apply:       migrateV8ToV9,
	},
	{
		Version:     11,
		Description: "Ensure host_port matches container_port for services (awsvpc compatibility)",
		Apply:       migrateToV11,
	},
	{
		Version:     12,
		Description: "Ensure all postgres boolean fields have explicit default values",
		Apply:       migrateToV12,
	},
	{
		Version:     13,
		Description: "Multi-rule EventBridge support (rules[] array in event_processor_tasks)",
		Apply:       migrateToV13,
	},
	{
		Version:     14,
		Description: "CloudFront CDN configuration",
		Apply:       migrateToV14,
	},
	{
		Version:     15,
		Description: "Multiple CloudFront distributions support",
		Apply:       migrateToV15,
	},
	{
		Version:     16,
		Description: "DynamoDB state locking support",
		Apply:       migrateToV16,
	},
	{
		Version:     17,
		Description: "Add multi-domain support to SES configuration",
		Apply:       migrateToV17,
	},
	{
		Version:     18,
		Description: "Move test_emails to global SES level (account-wide)",
		Apply:       migrateToV18,
	},
	{
		Version:     19,
		Description: "Add enabled field to services (allows disabling without removing config)",
		Apply:       migrateToV19,
	},
	{
		Version:     20,
		Description: "Add enabled field to scheduled_tasks and event_processor_tasks",
		Apply:       migrateToV20,
	},
	{
		Version:     21,
		Description: "Add AppSync Lambda authorizer configuration (jwks_uri, jwt_issuer, jwt_audience)",
		Apply:       migrateToV21,
	},
	{
		Version:     22,
		Description: "Add auto_deploy to backend, services and scheduled_tasks (CI/CD auto-deploy policy)",
		Apply:       migrateToV22,
	},
	{
		Version:     23,
		Description: "Add AppSync auth_mode and explicit api_key_enabled",
		Apply:       migrateToV23,
	},
	{
		Version:     24,
		Description: "Repair the misspelt cognito.dashboard_callback_ur_ls key",
		Apply:       migrateToV24,
	},
	{
		Version:     25,
		Description: "Normalize scheduled_tasks container_command from a scalar to list(string)",
		Apply:       migrateToV25,
	},
	{
		Version:     26,
		Description: "Add backend_runtime and per-service runtime (EC2 capacity pools)",
		Apply:       migrateToV26,
	},
	{
		Version:     27,
		Description: "Add egress_strategy, defaulting to public_ip (existing behaviour)",
		Apply:       migrateToV27,
	},
	{
		Version:     28,
		Description: "Add github_oidc_create_provider, defaulting to true (existing behaviour)",
		Apply:       migrateToV28,
	},
}

// detectSchemaVersion attempts to detect the schema version of a YAML file
func detectSchemaVersion(data map[string]interface{}) int {
	// If schema_version field exists, check if v6 needs re-run (deprecated fields present)
	if version, ok := data["schema_version"].(int); ok {
		// If marked as v6 but has deprecated fields, re-run v6 migration
		if version == 6 {
			if _, hasAZCount := data["az_count"]; hasAZCount {
				return 5 // Force re-run of v6 migration
			}
			if _, hasPrivate := data["create_private_subnets"]; hasPrivate {
				return 5 // Force re-run of v6 migration
			}
			if _, hasNAT := data["enable_nat_gateway"]; hasNAT {
				return 5 // Force re-run of v6 migration
			}
		}
		return version
	}

	// Otherwise, detect based on fields present

	// Check for v5 fields (account_id, aws_profile)
	if _, hasAccountID := data["account_id"]; hasAccountID {
		return 5
	}

	// Check for v4 fields (backend scaling in workload)
	if workload, ok := data["workload"].(map[interface{}]interface{}); ok {
		if _, hasScaling := workload["backend_desired_count"]; hasScaling {
			return 4
		}
	}

	// Check for v3 fields (DNS management in domain)
	if domain, ok := data["domain"].(map[interface{}]interface{}); ok {
		if _, hasZoneID := domain["zone_id"]; hasZoneID {
			return 3
		}
	}

	// Check for v2 fields (Aurora in postgres)
	if postgres, ok := data["postgres"].(map[interface{}]interface{}); ok {
		if _, hasAurora := postgres["aurora"]; hasAurora {
			return 2
		}
	}

	// Default to version 1
	return 1
}

// migrateToV2 adds Aurora Serverless v2 support
func migrateToV2(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v2: Adding Aurora Serverless v2 and ALB support")

	// Add postgres fields if postgres exists
	if postgres, ok := data["postgres"].(map[interface{}]interface{}); ok {
		if _, exists := postgres["aurora"]; !exists {
			postgres["aurora"] = false
			postgres["min_capacity"] = 0.5
			postgres["max_capacity"] = 1.0
		}
	}

	// Add ALB configuration if it doesn't exist
	if _, exists := data["alb"]; !exists {
		data["alb"] = map[string]interface{}{
			"enabled": false,
		}
	}

	return nil
}

// migrateToV3 adds DNS management fields
func migrateToV3(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v3: Adding DNS management fields")

	// Add domain fields if domain exists
	if domain, ok := data["domain"].(map[interface{}]interface{}); ok {
		// Only add zone_id if create_domain_zone is false (using existing zone)
		// Don't add it for new zones - the domain module handles this internally
		createDomainZone := true // Default to true
		if val, exists := domain["create_domain_zone"]; exists {
			if boolVal, ok := val.(bool); ok {
				createDomainZone = boolVal
			}
		}

		// Only add zone_id for existing zones
		if !createDomainZone {
			if _, exists := domain["zone_id"]; !exists {
				domain["zone_id"] = ""
			}
		}

		// Add other DNS management fields
		if _, exists := domain["root_zone_id"]; !exists {
			domain["root_zone_id"] = ""
		}
		if _, exists := domain["root_account_id"]; !exists {
			domain["root_account_id"] = ""
		}
		if _, exists := domain["is_dns_root"]; !exists {
			domain["is_dns_root"] = false
		}
		if _, exists := domain["dns_root_account_id"]; !exists {
			domain["dns_root_account_id"] = ""
		}
		if _, exists := domain["delegation_role_arn"]; !exists {
			domain["delegation_role_arn"] = ""
		}
		if _, exists := domain["api_domain_prefix"]; !exists {
			domain["api_domain_prefix"] = ""
		}
		if _, exists := domain["add_env_domain_prefix"]; !exists {
			// Default to true for non-production environments
			// This ensures dev/staging environments get env prefix (e.g., api.dev.example.com)
			env, _ := data["env"].(string)
			isProd := env == "prod" || env == "production"
			domain["add_env_domain_prefix"] = !isProd
		}
	}

	return nil
}

// migrateToV4 adds backend scaling configuration
func migrateToV4(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v4: Adding backend scaling configuration")

	// Add workload fields if workload exists
	if workload, ok := data["workload"].(map[interface{}]interface{}); ok {
		// Fix zero values for backend_desired_count
		if desiredCount, exists := workload["backend_desired_count"]; !exists {
			workload["backend_desired_count"] = 1
		} else if countInt, ok := desiredCount.(int); ok && countInt == 0 {
			workload["backend_desired_count"] = 1
		}

		if _, exists := workload["backend_autoscaling_enabled"]; !exists {
			workload["backend_autoscaling_enabled"] = false
		}

		// Fix zero values for min_capacity
		if minCap, exists := workload["backend_autoscaling_min_capacity"]; !exists {
			workload["backend_autoscaling_min_capacity"] = 1
		} else if capInt, ok := minCap.(int); ok && capInt == 0 {
			workload["backend_autoscaling_min_capacity"] = 1
		}

		// Fix zero values for max_capacity
		if maxCap, exists := workload["backend_autoscaling_max_capacity"]; !exists {
			workload["backend_autoscaling_max_capacity"] = 4
		} else if capInt, ok := maxCap.(int); ok && capInt == 0 {
			workload["backend_autoscaling_max_capacity"] = 4
		}

		// Fix empty values for CPU
		if cpu, exists := workload["backend_cpu"]; !exists || cpu == "" {
			workload["backend_cpu"] = "256"
		}

		// Fix empty values for memory
		if memory, exists := workload["backend_memory"]; !exists || memory == "" {
			workload["backend_memory"] = "512"
		}

		if _, exists := workload["backend_alb_domain_name"]; !exists {
			workload["backend_alb_domain_name"] = ""
		}
	}

	return nil
}

// migrateToV5 adds account_id and aws_profile
func migrateToV5(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v5: Adding Account ID and AWS Profile fields")

	if _, exists := data["account_id"]; !exists {
		data["account_id"] = ""
	}
	if _, exists := data["aws_profile"]; !exists {
		data["aws_profile"] = ""
	}

	return nil
}

// migrateToV6 adds custom VPC configuration
func migrateToV6(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v6: Adding custom VPC configuration")

	// Add use_default_vpc flag (true for backward compatibility with existing projects)
	// Existing projects without this field were using AWS default VPC
	if _, exists := data["use_default_vpc"]; !exists {
		data["use_default_vpc"] = true // Keep existing projects on default VPC
		fmt.Println("    ℹ️  Setting use_default_vpc=true for backward compatibility")
	}

	// Add VPC configuration fields (only used when use_default_vpc = false)
	// vpc_cidr is optional - VPC module has default of 10.0.0.0/16
	// Only fix if empty string (keep it if not specified)
	if vpcCIDR, exists := data["vpc_cidr"]; exists && vpcCIDR == "" {
		data["vpc_cidr"] = "10.0.0.0/16"
		fmt.Println("    ℹ️  Fixing empty vpc_cidr → 10.0.0.0/16")
	}

	// Remove deprecated fields
	if _, exists := data["az_count"]; exists {
		delete(data, "az_count")
		fmt.Println("    🗑️  Removed az_count (now hardcoded to 2 in VPC module)")
	}
	if _, exists := data["create_private_subnets"]; exists {
		delete(data, "create_private_subnets")
		fmt.Println("    🗑️  Removed create_private_subnets (deprecated)")
	}
	if _, exists := data["enable_nat_gateway"]; exists {
		delete(data, "enable_nat_gateway")
		fmt.Println("    🗑️  Removed enable_nat_gateway (deprecated)")
	}

	return nil
}

// migrateToV7 adds ECR strategy configuration
func migrateToV7(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v7: Adding ECR strategy configuration")

	// Only add ecr_strategy if it doesn't exist
	if _, exists := data["ecr_strategy"]; !exists {
		// Determine strategy based on existing configuration
		env, _ := data["env"].(string)
		ecrAccountID, hasECRAccountID := data["ecr_account_id"]

		// Strategy determination:
		// 1. If env is "dev", default to "local" (dev owns ECR)
		// 2. If ecr_account_id is set, use "cross_account" (pulling from another account)
		// 3. Otherwise, use "local" (each environment has its own ECR)

		if env == "dev" {
			data["ecr_strategy"] = "local"
			fmt.Println("    ℹ️  Setting ecr_strategy=local for dev environment")
		} else if hasECRAccountID && ecrAccountID != nil && ecrAccountID != "" {
			data["ecr_strategy"] = "cross_account"
			fmt.Println("    ℹ️  Setting ecr_strategy=cross_account (ecr_account_id is set)")
		} else {
			data["ecr_strategy"] = "local"
			fmt.Println("    ℹ️  Setting ecr_strategy=local (isolated environment)")
		}
	}

	// Ensure ecr_account_id and ecr_account_region exist (even if empty)
	if _, exists := data["ecr_account_id"]; !exists {
		data["ecr_account_id"] = nil
	}
	if _, exists := data["ecr_account_region"]; !exists {
		data["ecr_account_region"] = nil
	}

	return nil
}

// migrateToV8 adds ECR trusted accounts for cross-account access
func migrateToV8(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v8: Adding ECR trusted accounts configuration")

	// Add ecr_trusted_accounts field if it doesn't exist
	if _, exists := data["ecr_trusted_accounts"]; !exists {
		data["ecr_trusted_accounts"] = []interface{}{}
		fmt.Println("    ℹ️  Initialized empty ecr_trusted_accounts array")
	}

	return nil
}

// migrateV8ToV9 adds per-service ECR configuration (actually migrating to v10)
func migrateV8ToV9(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v10: Adding per-service ECR configuration")

	// Helper function to add default ECR config to a list of items
	addDefaultECRConfig := func(items []interface{}, itemType string) int {
		count := 0
		for _, itemRaw := range items {
			itemMap, ok := itemRaw.(map[interface{}]interface{})
			if !ok {
				continue
			}

			// Only add if ecr_config doesn't already exist
			if _, exists := itemMap["ecr_config"]; !exists {
				itemMap["ecr_config"] = map[string]interface{}{
					"mode": "create_ecr",
				}
				count++
			}
		}
		return count
	}

	totalMigrated := 0

	// Migrate services
	if servicesRaw, exists := data["services"]; exists {
		if services, ok := servicesRaw.([]interface{}); ok {
			count := addDefaultECRConfig(services, "services")
			totalMigrated += count
			if count > 0 {
				fmt.Printf("    ✓ Added default ECR config to %d service(s)\n", count)
			}
		}
	}

	// Migrate event_processor_tasks
	if tasksRaw, exists := data["event_processor_tasks"]; exists {
		if tasks, ok := tasksRaw.([]interface{}); ok {
			count := addDefaultECRConfig(tasks, "event_processor_tasks")
			totalMigrated += count
			if count > 0 {
				fmt.Printf("    ✓ Added default ECR config to %d event processor task(s)\n", count)
			}
		}
	}

	// Migrate scheduled_tasks
	if tasksRaw, exists := data["scheduled_tasks"]; exists {
		if tasks, ok := tasksRaw.([]interface{}); ok {
			count := addDefaultECRConfig(tasks, "scheduled_tasks")
			totalMigrated += count
			if count > 0 {
				fmt.Printf("    ✓ Added default ECR config to %d scheduled task(s)\n", count)
			}
		}
	}

	if totalMigrated == 0 {
		fmt.Println("    ℹ️  No services/tasks to migrate")
	} else {
		fmt.Printf("    ✓ Total items migrated: %d\n", totalMigrated)
	}

	return nil
}

// migrateToV9 simplifies Amplify domain configuration
func migrateToV9(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v9: Simplifying Amplify domain configuration")

	// Check if amplify_apps exists
	amplifyAppsRaw, exists := data["amplify_apps"]
	if !exists || amplifyAppsRaw == nil {
		fmt.Println("    ℹ️  No amplify_apps to migrate")
		return nil
	}

	amplifyApps, ok := amplifyAppsRaw.([]interface{})
	if !ok {
		fmt.Println("    ⚠️  amplify_apps is not an array, skipping migration")
		return nil
	}

	// Get domain configuration for parsing
	var domainName string
	var env string
	var addEnvPrefix bool

	if domainRaw, exists := data["domain"]; exists {
		if domainMap, ok := domainRaw.(map[interface{}]interface{}); ok {
			if dn, ok := domainMap["domain_name"].(string); ok {
				domainName = dn
			}
			if aep, ok := domainMap["add_env_domain_prefix"].(bool); ok {
				addEnvPrefix = aep
			}
		}
	}

	if envRaw, exists := data["env"]; exists {
		if e, ok := envRaw.(string); ok {
			env = e
		}
	}

	migrationCount := 0

	// Migrate each app
	for i, appRaw := range amplifyApps {
		appMap, ok := appRaw.(map[interface{}]interface{})
		if !ok {
			continue
		}

		// Check if app has old format (custom_domain or enable_root_domain)
		customDomain, hasCustomDomain := appMap["custom_domain"].(string)
		_, hasEnableRoot := appMap["enable_root_domain"]

		if !hasCustomDomain && !hasEnableRoot {
			// Already in new format or no domain config
			continue
		}

		// Remove enable_root_domain (no longer needed)
		if hasEnableRoot {
			delete(appMap, "enable_root_domain")
		}

		// Parse custom_domain to extract subdomain_prefix
		if customDomain != "" && domainName != "" {
			// Try to extract subdomain prefix
			// Examples:
			//   app.sava-p.com with domain sava-p.com → prefix: app
			//   app.dev.sava-p.com with domain sava-p.com and env dev → prefix: app

			// Remove the base domain
			prefix := customDomain
			if len(customDomain) > len(domainName) &&
				customDomain[len(customDomain)-len(domainName):] == domainName {
				prefix = customDomain[:len(customDomain)-len(domainName)-1] // Remove domain and dot
			}

			// If env prefix was used, remove it
			if addEnvPrefix && env != "" && env != "prod" {
				envPrefix := env + "."
				if len(prefix) > len(envPrefix) && prefix[len(prefix)-len(envPrefix):] == envPrefix {
					prefix = prefix[:len(prefix)-len(envPrefix)-1] // Remove env prefix and dot
				}
			}

			// Set subdomain_prefix
			if prefix != "" {
				appMap["subdomain_prefix"] = prefix
				fmt.Printf("    ✓ App %d: Extracted subdomain_prefix '%s' from custom_domain '%s'\n",
					i+1, prefix, customDomain)
			}

			// Remove custom_domain (will be auto-constructed)
			delete(appMap, "custom_domain")
			migrationCount++
		} else if customDomain != "" {
			// Keep custom_domain as-is if we can't parse it (manual override)
			fmt.Printf("    ⚠️  App %d: Keeping custom_domain '%s' (couldn't parse or no base domain)\n",
				i+1, customDomain)
		}
	}

	if migrationCount > 0 {
		fmt.Printf("    ✓ Migrated %d Amplify app(s) to simplified domain configuration\n", migrationCount)
	} else {
		fmt.Println("    ℹ️  No Amplify apps needed migration")
	}

	return nil
}

// migrateToV11 ensures host_port matches container_port for all services
func migrateToV11(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v11: Ensuring host_port matches container_port for awsvpc compatibility")

	// Check if services exist
	servicesRaw, exists := data["services"]
	if !exists || servicesRaw == nil {
		fmt.Println("    ℹ️  No services to migrate")
		return nil
	}

	services, ok := servicesRaw.([]interface{})
	if !ok {
		fmt.Println("    ⚠️  services is not an array, skipping migration")
		return nil
	}

	fixedCount := 0

	// Iterate through all services
	for _, serviceRaw := range services {
		serviceMap, ok := serviceRaw.(map[interface{}]interface{})
		if !ok {
			continue
		}

		// Get service name for logging
		serviceName, _ := serviceMap["name"].(string)

		// Get container_port
		containerPort, hasContainerPort := serviceMap["container_port"]
		if !hasContainerPort {
			continue
		}

		// Check if host_port exists and matches
		hostPort, hasHostPort := serviceMap["host_port"]

		// If host_port is missing or doesn't match container_port, fix it
		if !hasHostPort || hostPort != containerPort {
			serviceMap["host_port"] = containerPort
			fixedCount++
			if serviceName != "" {
				fmt.Printf("    ✓ Service '%s': Set host_port=%v to match container_port\n", serviceName, containerPort)
			} else {
				fmt.Printf("    ✓ Set host_port=%v to match container_port\n", containerPort)
			}
		}
	}

	if fixedCount == 0 {
		fmt.Println("    ℹ️  All services already have matching host_port")
	} else {
		fmt.Printf("    ✓ Fixed %d service(s) with mismatched or missing host_port\n", fixedCount)
	}

	return nil
}

// migrateToV12 ensures all postgres boolean fields have explicit default values
func migrateToV12(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v12: Ensuring all postgres boolean fields have explicit values")

	// Check if postgres exists
	postgresRaw, exists := data["postgres"]
	if !exists || postgresRaw == nil {
		fmt.Println("    ℹ️  No postgres configuration to migrate")
		return nil
	}

	postgres, ok := postgresRaw.(map[interface{}]interface{})
	if !ok {
		fmt.Println("    ⚠️  postgres is not a map, skipping migration")
		return nil
	}

	fieldsAdded := 0

	// Define default values for RDS boolean fields
	booleanDefaults := map[string]interface{}{
		"multi_az":                            false,
		"storage_encrypted":                   true,
		"deletion_protection":                 false,
		"skip_final_snapshot":                 true,
		"iam_database_authentication_enabled": false,
	}

	// Add missing boolean fields with defaults
	for field, defaultValue := range booleanDefaults {
		if _, exists := postgres[field]; !exists {
			postgres[field] = defaultValue
			fieldsAdded++
			fmt.Printf("    ✓ Added %s = %v (default)\n", field, defaultValue)
		}
	}

	if fieldsAdded == 0 {
		fmt.Println("    ℹ️  All postgres boolean fields already have explicit values")
	} else {
		fmt.Printf("    ✓ Added %d missing postgres boolean field(s)\n", fieldsAdded)
	}

	return nil
}

// migrateToV13 converts legacy single-rule EventBridge tasks to multi-rule format
func migrateToV13(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v13: Converting EventBridge tasks to multi-rule format")

	// Check if event_processor_tasks exist
	tasksRaw, exists := data["event_processor_tasks"]
	if !exists || tasksRaw == nil {
		fmt.Println("    ℹ️  No event_processor_tasks to migrate")
		return nil
	}

	tasks, ok := tasksRaw.([]interface{})
	if !ok {
		fmt.Println("    ⚠️  event_processor_tasks is not an array, skipping migration")
		return nil
	}

	migratedCount := 0

	for _, taskRaw := range tasks {
		taskMap, ok := taskRaw.(map[interface{}]interface{})
		if !ok {
			continue
		}

		taskName, _ := taskMap["name"].(string)

		// Skip if already has rules[] array (already migrated)
		if _, hasRules := taskMap["rules"]; hasRules {
			continue
		}

		// Check for legacy fields
		ruleName, hasRuleName := taskMap["rule_name"].(string)
		if !hasRuleName || ruleName == "" {
			continue
		}

		// Get sources and detail_types
		var sources []interface{}
		var detailTypes []interface{}

		if srcRaw, ok := taskMap["sources"]; ok {
			if srcArr, ok := srcRaw.([]interface{}); ok {
				sources = srcArr
			}
		}

		if dtRaw, ok := taskMap["detail_types"]; ok {
			if dtArr, ok := dtRaw.([]interface{}); ok {
				detailTypes = dtArr
			}
		}

		// Create new rules array with the legacy rule
		newRule := map[interface{}]interface{}{
			"name":         ruleName,
			"sources":      sources,
			"detail_types": detailTypes,
		}

		taskMap["rules"] = []interface{}{newRule}

		// Remove legacy fields (keep them for backward compatibility in Terraform)
		// We don't delete them because the template checks for rules[] first

		migratedCount++
		fmt.Printf("    ✓ Task '%s': Converted rule '%s' to rules[] array\n", taskName, ruleName)
	}

	if migratedCount == 0 {
		fmt.Println("    ℹ️  All event_processor_tasks already use rules[] format or have no rules")
	} else {
		fmt.Printf("    ✓ Migrated %d event_processor_task(s) to multi-rule format\n", migratedCount)
	}

	return nil
}

// migrateToV14 adds CloudFront CDN configuration
func migrateToV14(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v14: Adding CloudFront CDN configuration")

	// Add cloudfront configuration if it doesn't exist
	if _, exists := data["cloudfront"]; !exists {
		data["cloudfront"] = map[string]interface{}{
			"enabled": false,
		}
		fmt.Println("    ℹ️  Initialized CloudFront configuration (disabled by default)")
	}

	return nil
}

// migrateToV15 converts single cloudfront to cloudfront_distributions array
func migrateToV15(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v15: Converting to multiple CloudFront distributions support")

	// Check if cloudfront_distributions already exists
	if _, exists := data["cloudfront_distributions"]; exists {
		fmt.Println("    ℹ️  cloudfront_distributions already exists, skipping conversion")
		return nil
	}

	// Check if old cloudfront config exists
	cloudfrontRaw, exists := data["cloudfront"]
	if !exists || cloudfrontRaw == nil {
		// Initialize empty array
		data["cloudfront_distributions"] = []interface{}{}
		fmt.Println("    ℹ️  Initialized empty cloudfront_distributions array")
		return nil
	}

	cloudfront, ok := cloudfrontRaw.(map[interface{}]interface{})
	if !ok {
		// Try with string keys
		if cfStr, okStr := cloudfrontRaw.(map[string]interface{}); okStr {
			// Convert to new format
			data["cloudfront_distributions"] = []interface{}{}
			delete(data, "cloudfront")
			fmt.Println("    ℹ️  Removed old cloudfront config, initialized empty array")

			// Check if it was enabled
			if enabled, hasEnabled := cfStr["enabled"]; hasEnabled {
				if enabledBool, isBool := enabled.(bool); isBool && enabledBool {
					// Convert the old config to first distribution
					projectName, _ := data["project"].(string)
					if projectName == "" {
						projectName = "default"
					}

					cfStr["name"] = projectName + "-cdn"
					data["cloudfront_distributions"] = []interface{}{cfStr}
					fmt.Printf("    ✓ Converted existing CloudFront config to distribution '%s-cdn'\n", projectName)
				}
			}
			return nil
		}

		data["cloudfront_distributions"] = []interface{}{}
		delete(data, "cloudfront")
		fmt.Println("    ⚠️  Could not parse old cloudfront config, initialized empty array")
		return nil
	}

	// Check if cloudfront was enabled
	enabled := false
	if enabledVal, hasEnabled := cloudfront["enabled"]; hasEnabled {
		if enabledBool, isBool := enabledVal.(bool); isBool {
			enabled = enabledBool
		}
	}

	if !enabled {
		// Not enabled, just remove old config and initialize empty array
		data["cloudfront_distributions"] = []interface{}{}
		delete(data, "cloudfront")
		fmt.Println("    ℹ️  Old CloudFront was disabled, initialized empty cloudfront_distributions array")
		return nil
	}

	// Convert enabled cloudfront to first distribution in array
	projectName := "default"
	if pn, ok := data["project"].(string); ok && pn != "" {
		projectName = pn
	}

	// Add name to the existing config
	cloudfront["name"] = projectName + "-cdn"

	// Create new array with the converted distribution
	data["cloudfront_distributions"] = []interface{}{cloudfront}

	// Remove old cloudfront key
	delete(data, "cloudfront")

	fmt.Printf("    ✓ Converted existing CloudFront config to distribution '%s-cdn'\n", projectName)
	return nil
}

// migrateToV16 adds DynamoDB state locking support with auto-generated table name
func migrateToV16(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v16: Adding DynamoDB state locking support")

	// Get project and env for generating table name
	project, _ := data["project"].(string)
	env, _ := data["env"].(string)

	// Only auto-generate if project and env exist
	if project != "" && env != "" {
		// Check if state_lock_table is not already set
		if _, exists := data["state_lock_table"]; !exists {
			// Generate table name following same pattern as S3 bucket
			tableName := fmt.Sprintf("%s-terraform-locks-%s", project, env)
			data["state_lock_table"] = tableName
			fmt.Printf("    ✓ Auto-generated state_lock_table: %s\n", tableName)
			fmt.Println("    ℹ️  DynamoDB table will be created automatically on first terraform init")
		} else {
			fmt.Println("    ℹ️  Using existing state_lock_table configuration")
		}
	} else {
		fmt.Println("    ⚠️  Cannot auto-generate state_lock_table: project or env not set")
	}

	return nil
}

// migrateToV17 adds multi-domain support to SES configuration
func migrateToV17(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v17: Adding multi-domain support to SES configuration")

	ses, ok := data["ses"].(map[interface{}]interface{})
	if !ok || ses == nil {
		fmt.Println("    ℹ️  No SES configuration to migrate")
		return nil
	}

	// Check if already migrated (has domains array)
	if _, hasNewFormat := ses["domains"]; hasNewFormat {
		fmt.Println("    ℹ️  SES already has domains array format")
		return nil
	}

	// Check if legacy format exists
	domainName, hasDomain := ses["domain_name"]
	if !hasDomain {
		fmt.Println("    ℹ️  No legacy domain_name found, nothing to migrate")
		return nil
	}

	domainNameStr, ok := domainName.(string)
	if !ok || domainNameStr == "" {
		fmt.Println("    ℹ️  domain_name is empty, nothing to migrate")
		return nil
	}

	// Extract zone_id if it exists (will be populated at template render time)
	var zoneID interface{} = nil
	if domain, ok := data["domain"].(map[interface{}]interface{}); ok {
		if enabled, ok := domain["enabled"].(bool); ok && enabled {
			// Note: zone_id will be populated at template render time via module.domain.zone_id
			// For migration, we just note that it should use the domain zone
			// Leave it empty so template will populate it
			zoneID = nil
		}
	}

	// Create new domain entry
	newDomain := map[interface{}]interface{}{
		"domain": domainNameStr,
	}

	// Add zone_id if it was determined
	if zoneID != nil {
		newDomain["zone_id"] = zoneID
	}

	// Migrate test_emails to domain-specific if they exist
	if testEmails, ok := ses["test_emails"]; ok {
		newDomain["test_emails"] = testEmails
	}

	// Create domains array with single domain
	ses["domains"] = []interface{}{newDomain}

	// Delete legacy fields after migration to avoid duplicates
	delete(ses, "domain_name")
	delete(ses, "test_emails")

	// Add global default settings if they don't exist
	if _, ok := ses["enable_mail_from"]; !ok {
		ses["enable_mail_from"] = true
	}
	if _, ok := ses["mail_from_subdomain"]; !ok {
		ses["mail_from_subdomain"] = "bounce"
	}
	if _, ok := ses["dmarc_policy"]; !ok {
		ses["dmarc_policy"] = "none"
	}

	fmt.Printf("    ✓ Migrated legacy SES domain '%s' to domains array\n", domainNameStr)
	fmt.Println("    ✓ Removed legacy domain_name and test_emails fields")

	return nil
}

// migrateToV18 moves test_emails from per-domain to global SES level
// Test email identities in AWS SES are account-wide, not per-domain
func migrateToV18(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v18: Moving test_emails to global SES level")

	ses, ok := data["ses"].(map[interface{}]interface{})
	if !ok || ses == nil {
		fmt.Println("    ℹ️  No SES configuration to migrate")
		return nil
	}

	domains, ok := ses["domains"].([]interface{})
	if !ok || len(domains) == 0 {
		fmt.Println("    ℹ️  No domains array found")
		return nil
	}

	// Collect all test emails from domains
	allTestEmails := make(map[string]bool) // Use map for deduplication

	// Add existing global test_emails if any
	if existingEmails, ok := ses["test_emails"].([]interface{}); ok {
		for _, email := range existingEmails {
			if emailStr, ok := email.(string); ok && emailStr != "" {
				allTestEmails[emailStr] = true
			}
		}
	}

	// Collect test_emails from each domain and remove the field
	for _, d := range domains {
		domain, ok := d.(map[interface{}]interface{})
		if !ok {
			continue
		}

		if testEmails, ok := domain["test_emails"].([]interface{}); ok {
			for _, email := range testEmails {
				if emailStr, ok := email.(string); ok && emailStr != "" {
					allTestEmails[emailStr] = true
				}
			}
			// Remove test_emails from domain
			delete(domain, "test_emails")
		}
	}

	// Convert map to slice
	if len(allTestEmails) > 0 {
		emailSlice := make([]interface{}, 0, len(allTestEmails))
		for email := range allTestEmails {
			emailSlice = append(emailSlice, email)
		}
		ses["test_emails"] = emailSlice
		fmt.Printf("    ✓ Consolidated %d test email(s) to global level\n", len(emailSlice))
	} else {
		fmt.Println("    ℹ️  No test emails to migrate")
	}

	return nil
}

// migrateToV19 adds enabled=true to all existing services
func migrateToV19(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v19: Adding enabled field to services")

	servicesRaw, exists := data["services"]
	if !exists || servicesRaw == nil {
		fmt.Println("    ℹ️  No services to migrate")
		return nil
	}

	services, ok := servicesRaw.([]interface{})
	if !ok {
		fmt.Println("    ⚠️  services is not an array, skipping migration")
		return nil
	}

	updatedCount := 0
	for _, serviceRaw := range services {
		serviceMap, ok := serviceRaw.(map[interface{}]interface{})
		if !ok {
			continue
		}

		if _, hasEnabled := serviceMap["enabled"]; !hasEnabled {
			serviceMap["enabled"] = true
			updatedCount++
			if name, _ := serviceMap["name"].(string); name != "" {
				fmt.Printf("    ✓ Service '%s': Set enabled=true\n", name)
			}
		}
	}

	if updatedCount == 0 {
		fmt.Println("    ℹ️  All services already have enabled field")
	} else {
		fmt.Printf("    ✓ Set enabled=true on %d service(s)\n", updatedCount)
	}

	return nil
}

// migrateToV20 adds enabled=true to all existing scheduled_tasks and event_processor_tasks
func migrateToV20(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v20: Adding enabled field to scheduled_tasks and event_processor_tasks")

	for _, key := range []string{"scheduled_tasks", "event_processor_tasks"} {
		tasksRaw, exists := data[key]
		if !exists || tasksRaw == nil {
			fmt.Printf("    ℹ️  No %s to migrate\n", key)
			continue
		}

		tasks, ok := tasksRaw.([]interface{})
		if !ok {
			fmt.Printf("    ⚠️  %s is not an array, skipping\n", key)
			continue
		}

		updatedCount := 0
		for _, taskRaw := range tasks {
			taskMap, ok := taskRaw.(map[interface{}]interface{})
			if !ok {
				continue
			}

			if _, hasEnabled := taskMap["enabled"]; !hasEnabled {
				taskMap["enabled"] = true
				updatedCount++
				if name, _ := taskMap["name"].(string); name != "" {
					fmt.Printf("    ✓ %s '%s': Set enabled=true\n", key, name)
				}
			}
		}

		if updatedCount == 0 {
			fmt.Printf("    ℹ️  All %s already have enabled field\n", key)
		} else {
			fmt.Printf("    ✓ Set enabled=true on %d %s\n", updatedCount, key)
		}
	}

	return nil
}

// migrateToV21 adds the AppSync Lambda authorizer keys to pubsub_appsync.
//
// The authorizer used to fall back to a hardcoded third-party JWKS endpoint when
// nothing was configured, which meant anyone able to mint a token there could
// call the API. modules/appsync now requires jwks_uri with no default, so the
// value has to come from YAML.
//
// This migration deliberately writes an EMPTY jwks_uri: guessing an identity
// provider would recreate the same silent-trust bug in a new place. An empty
// value is caught by validateAppSyncConfig before terraform ever runs.
func migrateToV21(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v21: Adding AppSync Lambda authorizer configuration")

	appsyncRaw, exists := data["pubsub_appsync"]
	if !exists || appsyncRaw == nil {
		fmt.Println("    ℹ️  No pubsub_appsync configuration to migrate")
		return nil
	}

	appsync, ok := appsyncRaw.(map[interface{}]interface{})
	if !ok {
		fmt.Println("    ⚠️  pubsub_appsync is not a map, skipping migration")
		return nil
	}

	fieldsAdded := 0
	for _, field := range []string{"jwks_uri", "jwt_issuer", "jwt_audience"} {
		if _, hasField := appsync[field]; !hasField {
			appsync[field] = ""
			fieldsAdded++
			fmt.Printf("    ✓ Added %s = \"\" (must be set by you, never guessed)\n", field)
		}
	}

	if fieldsAdded == 0 {
		fmt.Println("    ℹ️  AppSync authorizer fields already present")
	} else {
		fmt.Printf("    ✓ Added %d AppSync authorizer field(s)\n", fieldsAdded)
	}

	// Tell the user now rather than letting them discover it at deploy time.
	authLambda, _ := appsync["auth_lambda"].(bool)
	jwksURI, _ := appsync["jwks_uri"].(string)
	if authLambda && strings.TrimSpace(jwksURI) == "" {
		fmt.Println("    ⚠️  pubsub_appsync.auth_lambda is enabled but pubsub_appsync.jwks_uri is empty.")
		fmt.Println("       Set it to your identity provider's JWKS URL before deploying, e.g.")
		fmt.Println("       jwks_uri: \"https://<your-idp-host>/.well-known/jwks.json\"")
	}

	return nil
}

// migrateToV22 writes an explicit auto_deploy onto the backend, every service
// and every scheduled task.
//
// auto_deploy is CI policy, distinct from `enabled`: `enabled` says whether the
// thing exists in AWS at all, `auto_deploy` says whether an ECR push, an SSM
// change or an S3 env-file write may redeploy it without anyone asking. A manual
// deploy is unaffected either way.
//
// The default is computed from the environment — false in production, true
// everywhere else — because redeploying production the instant an image lands is
// the one case where nobody asked. Staging and every other pre-production
// environment keep auto-deploying: they exist to be deployed to, and turning
// them off would cost the fast feedback they are for. It is written into the
// YAML rather than left implicit, per CLAUDE.md's rule for core policy booleans:
// the file states the policy, and this function is the single source of the
// default.
//
// Two consequences worth being loud about:
//
//   - In a production environment this TURNS OFF automatic deploys that work
//     today for the backend and for services. That is the requested policy, not
//     an accident, and it is reversible by setting the field back to true. The
//     notice printed below says so at migration time rather than leaving it to
//     be discovered after a push does nothing.
//   - For scheduled tasks outside dev it changes nothing observable: no
//     automatic trigger can reach one there in the first place (modules/ecs_task
//     creates the task's ECR repository only in dev, and an SSM change
//     deliberately never redeploys a task). What it does fix is the honesty of
//     SCHEDULED_TASK_MAP, which used to be populated in every environment with
//     no way to tell which entries meant anything.
//
// An *absent* auto_deploy — a hand-written or not-yet-migrated file — still
// means true everywhere, in the Terraform variables and in the Lambda. That is
// deliberately the opposite default from this migration: absent must mean "keep
// doing what this project does today", while the migration states the policy.
func migrateToV22(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v22: Adding auto_deploy (CI/CD auto-deploy policy)")

	// Only production opts out. Both spellings are accepted because meroku has
	// never constrained the env name and both appear in the wild — matching just
	// "prod" would leave a "production" environment auto-deploying, which is the
	// single case this policy exists to prevent.
	env, _ := data["env"].(string)
	isProd := env == "prod" || env == "production"
	defaultAutoDeploy := !isProd

	// A config with no env is malformed — meroku requires it and will not deploy
	// without it — so this is reached by hand-written or truncated files only.
	// The policy keys off the name, and an unreadable name cannot be matched
	// against it, so say that plainly instead of quietly picking a side. Silently
	// choosing false here would disable a staging pipeline over a typo; silently
	// choosing true would arm production. The value below is stated, and so is
	// the fact that it was chosen without being able to read the environment.
	if env == "" {
		fmt.Println("    ⚠️  This config has no 'env' key, so the auto-deploy policy could not be")
		fmt.Printf("        matched against it. Defaulting to auto_deploy=%v. If this is a production\n", defaultAutoDeploy)
		fmt.Println("        config, set env and re-run, or set auto_deploy: false by hand.")
	}

	fmt.Printf("    ℹ️  env=%q → default auto_deploy=%v\n", env, defaultAutoDeploy)

	added := 0

	// Backend. workload may be absent in a minimal config; there is nothing to
	// annotate then, and the Terraform default (true) applies.
	if workload, ok := data["workload"].(map[interface{}]interface{}); ok {
		if _, has := workload["backend_auto_deploy"]; !has {
			workload["backend_auto_deploy"] = defaultAutoDeploy
			added++
			fmt.Printf("    ✓ backend: Set auto_deploy=%v\n", defaultAutoDeploy)
		}
	} else {
		fmt.Println("    ℹ️  No workload section; backend auto_deploy left to its default (true)")
	}

	// Services and scheduled tasks. event_processor_tasks are deliberately not
	// included: they are not in any of the CI Lambda's maps, so the field would
	// be configuration that nothing reads.
	for _, key := range []string{"services", "scheduled_tasks"} {
		itemsRaw, exists := data[key]
		if !exists || itemsRaw == nil {
			fmt.Printf("    ℹ️  No %s to migrate\n", key)
			continue
		}

		items, ok := itemsRaw.([]interface{})
		if !ok {
			fmt.Printf("    ⚠️  %s is not an array, skipping\n", key)
			continue
		}

		for _, itemRaw := range items {
			itemMap, ok := itemRaw.(map[interface{}]interface{})
			if !ok {
				continue
			}
			if _, has := itemMap["auto_deploy"]; has {
				continue
			}
			itemMap["auto_deploy"] = defaultAutoDeploy
			added++
			if name, _ := itemMap["name"].(string); name != "" {
				fmt.Printf("    ✓ %s '%s': Set auto_deploy=%v\n", key, name, defaultAutoDeploy)
			}
		}
	}

	switch {
	case added == 0:
		fmt.Println("    ℹ️  Every target already has an auto_deploy field")
	case defaultAutoDeploy:
		fmt.Printf("    ✓ Set auto_deploy=true on %d target(s)\n", added)
	default:
		fmt.Printf("    ✓ Set auto_deploy=false on %d target(s)\n", added)
		fmt.Printf("    ⚠️  %s is not a dev environment, so automatic deploys are now OFF for these targets.\n", env)
		fmt.Println("       An image push or an env change will be logged and ignored, with the reason named:")
		fmt.Println("         \"auto_deploy is disabled for <target>\"")
		fmt.Println("       Manual deploys (GitHub Actions DEPLOY / SERVICE_DEPLOY) are unaffected.")
		fmt.Println("       To keep a target deploying automatically, set auto_deploy: true on it and re-apply.")
	}

	return nil
}

// migrateToV23 writes AppSync's authorization mode and API-key policy into the
// YAML.
//
// modules/appsync used to hardcode `authentication_type = "AWS_LAMBDA"` and
// attach an unconditional API_KEY provider. Both are now choices, so both have
// to be stated somewhere. Two fields, two different calls:
//
// auth_mode -> "lambda", always.
//
//	The mode is inferred from what the environment deploys today, and today
//	every enabled AppSync API is AWS_LAMBDA — the hardcoded value. Note this is
//	true even where auth_lambda is false: that flag only ever chose whose
//	authorizer *source* was packaged (the project's tree or the module's), never
//	whether a Lambda authorizer was used. So "auth_lambda: true implies lambda"
//	is right, and so is "auth_lambda: false also implies lambda". Writing
//	anything else here would change what the next apply deploys.
//
// api_key_enabled -> true where AppSync is enabled, false where it is not.
//
//	This is the one place this migration does NOT write the new safe default,
//	and it is deliberate. An enabled environment has a live API key in AWS right
//	now; clients may be holding it. Writing false would delete that key on the
//	next apply and take those clients down — breaking a working API to fix a
//	weakness nobody asked us to fix today is a worse failure than the weakness.
//	So the migration preserves the credential and says loudly what it is and how
//	to remove it. "Default off" holds for every *new* environment (createEnv
//	sets false) and for every environment that has no key to lose.
//
// oidc_issuer / oidc_client_id are deliberately not added. They are mode-specific
// optional settings with no meaning outside oidc mode, and per CLAUDE.md that is
// the `default` helper's job, not a migration's — the migration writes the core
// policy fields that should always be explicit.
func migrateToV23(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v23: Adding AppSync auth_mode and api_key_enabled")

	appsyncRaw, exists := data["pubsub_appsync"]
	if !exists || appsyncRaw == nil {
		fmt.Println("    ℹ️  No pubsub_appsync configuration to migrate")
		return nil
	}

	appsync, ok := appsyncRaw.(map[interface{}]interface{})
	if !ok {
		fmt.Println("    ⚠️  pubsub_appsync is not a map, skipping migration")
		return nil
	}

	enabled, _ := appsync["enabled"].(bool)

	if existing, has := appsync["auth_mode"]; has {
		fmt.Printf("    ℹ️  auth_mode already set to %v, left alone\n", existing)
	} else {
		appsync["auth_mode"] = AppSyncAuthLambda
		fmt.Printf("    ✓ Added auth_mode = %q (what this module deploys today; nothing changes)\n", AppSyncAuthLambda)
		if enabled {
			fmt.Println("       AppSync now also supports:")
			fmt.Println("         auth_mode: \"cognito\"  — AWS validates tokens against this environment's user pool")
			fmt.Println("         auth_mode: \"oidc\"     — AWS validates tokens against your OIDC issuer")
			fmt.Println("       Both drop the authorizer Lambda entirely: no cold start, no invocation cost.")
		}
	}

	if existing, has := appsync["api_key_enabled"]; has {
		fmt.Printf("    ℹ️  api_key_enabled already set to %v, left alone\n", existing)
		return nil
	}

	appsync["api_key_enabled"] = enabled

	if !enabled {
		fmt.Println("    ✓ Added api_key_enabled = false (AppSync is disabled here, so there is no key to keep)")
		return nil
	}

	// Loud on purpose. This is the one value the migration sets to the less safe
	// of the two options, and the reader deserves to know that and to know the
	// one-line fix.
	fmt.Println("    ✓ Added api_key_enabled = true")
	fmt.Println()
	fmt.Println("    ⚠️  IMPORTANT — this environment has a live AppSync API key, and this")
	fmt.Println("        migration keeps it.")
	fmt.Println()
	fmt.Println("        Until now modules/appsync created an API key for every deployment and")
	fmt.Println("        exported it. That key is a bearer credential that BYPASSES token")
	fmt.Println("        verification completely: whoever holds it reaches every resolver without")
	fmt.Println("        presenting a JWT, so none of your issuer/audience/signature checks apply")
	fmt.Println("        to them.")
	fmt.Println()
	fmt.Println("        It is preserved rather than removed because setting it to false here")
	fmt.Println("        would delete the key on your next apply and break any client using it.")
	fmt.Println("        New environments default to false.")
	fmt.Println()
	fmt.Println("        To turn it off once you have confirmed nothing depends on it:")
	fmt.Println()
	fmt.Println("          pubsub_appsync:")
	fmt.Println("            api_key_enabled: false")
	fmt.Println()
	fmt.Println("        then re-generate and apply. Check first with:")
	fmt.Println("          aws appsync list-api-keys --api-id <your-api-id>")

	return nil
}

// legacyDashboardCallbackURLsKey is the misspelt key earlier versions of meroku
// wrote. app/model.go tagged Cognito.DashboardCallbackURLs with it — an
// acronym-splitting snake_case conversion — while env/main.hbs, modules/cognito
// and the web UI all use dashboard_callback_urls. Any config saved by those
// versions carries it, so it has to be repaired on disk rather than only fixed
// at the source.
const legacyDashboardCallbackURLsKey = "dashboard_callback_ur_ls"

const dashboardCallbackURLsKey = "dashboard_callback_urls"

func migrateToV24(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v24: Repairing the cognito.dashboard_callback_ur_ls key")

	cognito, ok := data["cognito"].(map[interface{}]interface{})
	if !ok {
		fmt.Println("    ℹ️  No cognito configuration to repair")
		return nil
	}

	legacy, hasLegacy := cognito[legacyDashboardCallbackURLsKey]
	if !hasLegacy {
		fmt.Println("    ✓ Already using dashboard_callback_urls, nothing to repair")
		return nil
	}

	// The misspelt key always goes, whatever happens to its value.
	delete(cognito, legacyDashboardCallbackURLsKey)

	current, hasCurrent := cognito[dashboardCallbackURLsKey]
	switch {
	case !hasCurrent || isEmptyList(current):
		// The normal case. The correct key is absent because nothing ever wrote
		// it, or empty because a hand edit added it without values.
		cognito[dashboardCallbackURLsKey] = legacy
		fmt.Printf("    ✓ Moved %v onto dashboard_callback_urls\n", legacy)
	case isEmptyList(legacy):
		fmt.Println("    ✓ Dropped the empty dashboard_callback_ur_ls key")
	default:
		// Both hold values. The correctly spelled one is what the template reads
		// and therefore what is deployed today, so it wins; the other is reported
		// rather than silently discarded.
		fmt.Printf("    ⚠️  Both keys held values. Kept dashboard_callback_urls = %v\n", current)
		fmt.Printf("        and discarded dashboard_callback_ur_ls = %v, which was never\n", legacy)
		fmt.Println("        read by the template and so was never deployed.")
	}

	return nil
}

// isEmptyList reports whether a decoded YAML value is an absent or empty
// sequence — the two ways "no URLs are configured" shows up in a file.
func isEmptyList(value interface{}) bool {
	if value == nil {
		return true
	}
	list, ok := value.([]interface{})
	return ok && len(list) == 0
}

// applyMigrations applies all necessary migrations to bring data to current version
func applyMigrations(data map[string]interface{}, currentVersion int) error {
	if currentVersion >= CurrentSchemaVersion {
		return nil
	}

	fmt.Printf("Schema version detected: v%d (current: v%d)\n", currentVersion, CurrentSchemaVersion)
	fmt.Println("Applying migrations...")

	for _, migration := range AllMigrations {
		if migration.Version > currentVersion {
			if err := migration.Apply(data); err != nil {
				return fmt.Errorf("migration to v%d failed: %w", migration.Version, err)
			}
		}
	}

	// Set the current schema version
	data["schema_version"] = CurrentSchemaVersion
	fmt.Printf("✓ Successfully migrated to v%d\n", CurrentSchemaVersion)

	return nil
}

// backupFile creates a timestamped backup of the original file in the backup/ directory
func backupFile(filepath string) error {
	backupPath, err := CreateProjectBackup(filepath)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	fmt.Printf("  ✓ Backup created: %s\n", backupPath)
	return nil
}

// loadEnvWithMigration loads a YAML file and applies migrations if needed
func loadEnvWithMigration(name string) (Env, error) {
	var e Env

	// Try loading from multiple possible paths
	possiblePaths := []string{
		name + ".yaml",
		"project/" + name + ".yaml",
		"../../project/" + name + ".yaml",
		"../" + name + ".yaml",
	}

	var yamlPath string
	var data []byte
	var lastErr error

	for _, path := range possiblePaths {
		var err error
		data, err = os.ReadFile(path)
		if err != nil {
			lastErr = err
			continue
		}
		yamlPath = path
		break
	}

	if yamlPath == "" {
		return e, fmt.Errorf("error reading YAML file from any location: %v", lastErr)
	}

	// First unmarshal to map to detect version
	var dataMap map[string]interface{}
	if err := yaml.Unmarshal(data, &dataMap); err != nil {
		return e, fmt.Errorf("error unmarshaling YAML: %v", err)
	}

	// Detect and apply migrations
	currentVersion := detectSchemaVersion(dataMap)

	if currentVersion < CurrentSchemaVersion {
		fmt.Printf("\n═══════════════════════════════════════════════════════════\n")
		fmt.Printf("  YAML Schema Migration Required\n")
		fmt.Printf("═══════════════════════════════════════════════════════════\n")
		fmt.Printf("File: %s\n", yamlPath)

		// Create backup
		if err := backupFile(yamlPath); err != nil {
			return e, fmt.Errorf("failed to create backup: %w", err)
		}

		// Apply migrations
		if err := applyMigrations(dataMap, currentVersion); err != nil {
			return e, fmt.Errorf("migration failed: %w", err)
		}

		// Save migrated data
		migratedData, err := yaml.Marshal(dataMap)
		if err != nil {
			return e, fmt.Errorf("error marshaling migrated data: %v", err)
		}

		if err := os.WriteFile(yamlPath, migratedData, 0644); err != nil {
			return e, fmt.Errorf("error writing migrated file: %v", err)
		}

		fmt.Printf("  ✓ Migrated file saved: %s\n", yamlPath)
		fmt.Printf("═══════════════════════════════════════════════════════════\n\n")

		// Re-read the migrated data
		data = migratedData
	}

	// Unmarshal to Env struct
	if err := yaml.Unmarshal(data, &e); err != nil {
		return e, fmt.Errorf("error unmarshaling YAML to Env struct: %v", err)
	}

	return e, nil
}

// migrateFileIfNeeded brings a config up to the current schema version, and says
// nothing when it is already there.
//
// This is the form the automatic paths need. MigrateYAMLFile announces "already
// at current version" because someone typed `meroku migrate` and is owed an
// answer; printing that on every template render would be noise, and noise is
// what stops people reading the line that matters.
//
// A file that cannot be read is not an error here. The callers -- template
// rendering, config loading -- produce far better messages about a missing or
// unreadable config than this function can, and they are about to run anyway.
// Failing early would replace a good error with a worse one.
func migrateFileIfNeeded(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var dataMap map[string]interface{}
	if err := yaml.Unmarshal(data, &dataMap); err != nil {
		return nil
	}

	if detectSchemaVersion(dataMap) >= CurrentSchemaVersion {
		return nil
	}

	return MigrateYAMLFile(path)
}

// MigrateYAMLFile migrates a single YAML file to the current schema version
func MigrateYAMLFile(filepath string) error {
	// Read the file
	data, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Unmarshal to map
	var dataMap map[string]interface{}
	if err := yaml.Unmarshal(data, &dataMap); err != nil {
		return fmt.Errorf("error unmarshaling YAML: %v", err)
	}

	// Detect version
	currentVersion := detectSchemaVersion(dataMap)

	if currentVersion >= CurrentSchemaVersion {
		fmt.Printf("File %s is already at current version (v%d)\n", filepath, currentVersion)
		return nil
	}

	fmt.Printf("\n═══════════════════════════════════════════════════════════\n")
	fmt.Printf("  Migrating: %s\n", filepath)
	fmt.Printf("═══════════════════════════════════════════════════════════\n")

	// Create backup
	if err := backupFile(filepath); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Apply migrations
	if err := applyMigrations(dataMap, currentVersion); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	// Save migrated data
	migratedData, err := yaml.Marshal(dataMap)
	if err != nil {
		return fmt.Errorf("error marshaling migrated data: %v", err)
	}

	if err := os.WriteFile(filepath, migratedData, 0644); err != nil {
		return fmt.Errorf("error writing migrated file: %v", err)
	}

	fmt.Printf("  ✓ Migration complete!\n")
	fmt.Printf("═══════════════════════════════════════════════════════════\n\n")

	return nil
}

// MigrateAllYAMLFiles migrates all YAML files in the project directory
func MigrateAllYAMLFiles() error {
	projectDir := "project"

	// Check if project directory exists
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		projectDir = "."
	}

	// Find all YAML files
	files, err := filepath.Glob(filepath.Join(projectDir, "*.yaml"))
	if err != nil {
		return fmt.Errorf("failed to find YAML files: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("No YAML files found to migrate")
		return nil
	}

	fmt.Printf("Found %d YAML file(s) to check for migration\n\n", len(files))

	for _, file := range files {
		if err := MigrateYAMLFile(file); err != nil {
			fmt.Printf("Error migrating %s: %v\n", file, err)
		}
	}

	return nil
}

// migrateToV25 normalizes scheduled_tasks[].container_command from a scalar
// string into a real list of arguments.
//
// The field is a list(string) in Terraform and is now a []string in the model,
// but it was a plain string for most of its life and the template rendered it
// raw. That let a scalar reach main.tf unquoted, so the values people wrote to
// work around it vary: a bare command, a shell-ish string, or a hand-written
// JSON array typed so the raw render would produce valid HCL. All three have to
// survive the change.
//
// A JSON-looking array is decoded, so `["npm","run","cron"]` becomes three
// arguments rather than one argument that happens to contain brackets. Anything
// else is kept verbatim as a single argument: splitting on whitespace would
// look right for `npm run cron` and quietly corrupt any command with a quoted
// argument containing a space.
func migrateToV25(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v25: Normalizing scheduled_tasks container_command to list(string)")

	tasksRaw, exists := data["scheduled_tasks"]
	if !exists || tasksRaw == nil {
		fmt.Println("    ℹ️  No scheduled_tasks to migrate")
		return nil
	}

	tasks, ok := tasksRaw.([]interface{})
	if !ok {
		fmt.Println("    ⚠️  scheduled_tasks is not an array, skipping migration")
		return nil
	}

	updatedCount := 0
	for _, taskRaw := range tasks {
		taskMap, ok := taskRaw.(map[interface{}]interface{})
		if !ok {
			continue
		}

		cmdRaw, hasCommand := taskMap["container_command"]
		if !hasCommand || cmdRaw == nil {
			continue
		}
		// Already a list. Keeps the migration idempotent, so a re-run and an
		// already-converted file are both no-ops.
		if _, isList := cmdRaw.([]interface{}); isList {
			continue
		}

		command, ok := cmdRaw.(string)
		if !ok {
			// Leave unexpected types for the Env decoder to reject with a
			// message naming the field, rather than mangling them here.
			continue
		}

		taskMap["container_command"] = scalarCommandToList(command)
		updatedCount++
		if name, _ := taskMap["name"].(string); name != "" {
			fmt.Printf("    ✓ scheduled_task '%s': converted container_command to a list\n", name)
		}
	}

	if updatedCount == 0 {
		fmt.Println("    ℹ️  All scheduled_tasks container_command values are already lists")
	} else {
		fmt.Printf("    ✓ Converted container_command on %d scheduled task(s)\n", updatedCount)
	}

	return nil
}

// scalarCommandToList turns one scalar container_command into its list form.
func scalarCommandToList(command string) []interface{} {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return []interface{}{}
	}

	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		var parsed []string
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
			arguments := make([]interface{}, 0, len(parsed))
			for _, argument := range parsed {
				arguments = append(arguments, argument)
			}
			return arguments
		}
		// Bracketed but not decodable as JSON. Falls through and is kept whole
		// rather than guessed at.
	}

	return []interface{}{command}
}

// migrateToV27 adds egress_strategy, which decides how ECS tasks reach the
// internet: their own public IPv4 address, a single NAT Gateway, or one NAT per
// AZ.
//
// Every existing environment gets "public_ip", which is exactly what meroku
// generated before this field existed, so the migration is a no-op in AWS: the
// next plan shows no changes. Operators opt into a NAT strategy when their
// service count makes it cheaper — see ai_docs/EGRESS_COST_MODEL.md for the
// thresholds, and app/egress_advisor.go for the advisory that surfaces them.
//
// Environments on the default VPC are pinned to public_ip regardless, because
// the default VPC has no private subnets for a NAT strategy to use.
func migrateToV27(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v27: Adding egress_strategy")

	if existing, ok := data["egress_strategy"].(string); ok && existing != "" {
		fmt.Printf("    ℹ️  egress_strategy already set to '%s', leaving it alone\n", existing)
		return nil
	}

	data["egress_strategy"] = string(EgressPublicIP)

	if useDefault, ok := data["use_default_vpc"].(bool); ok && useDefault {
		fmt.Println("    ✓ Set egress_strategy: public_ip (required — the default VPC has no private subnets)")
		return nil
	}

	fmt.Println("    ✓ Set egress_strategy: public_ip (current behaviour, no infrastructure change)")
	return nil
}

// migrateToV28 records that this project creates the account's GitHub OIDC
// provider, which is what every project did before the flag existed.
//
// Scoped to configs that actually enable OIDC. Writing the key into a project
// with enable_github_oidc: false would be describing a resource that config
// never creates, and every environment file in the repository would grow a line
// about a feature it does not use.
//
// The consequence is that a project enabling OIDC later has no key to flip, so
// the writers in aws_preflight.go and api.go both insert as well as flip. That
// is the right place for the tolerance: it also covers hand-written configs,
// which no migration ever sees.
func migrateToV28(data map[string]interface{}) error {
	fmt.Println("  → Migrating to v28: Adding workload.github_oidc_create_provider")

	workload, ok := data["workload"].(map[interface{}]interface{})
	if !ok {
		fmt.Println("    ℹ️  No workload section; nothing to annotate")
		return nil
	}

	if enabled, _ := workload["enable_github_oidc"].(bool); !enabled {
		fmt.Println("    ℹ️  GitHub OIDC is not enabled; leaving the key out")
		return nil
	}

	if existing, has := workload["github_oidc_create_provider"]; has {
		fmt.Printf("    ℹ️  github_oidc_create_provider already set to %v, leaving it alone\n", existing)
		return nil
	}

	workload["github_oidc_create_provider"] = true
	fmt.Println("    ✓ Set github_oidc_create_provider: true (current behaviour, no infrastructure change)")
	fmt.Println("       Set it to false if another project in this AWS account already owns the provider.")
	return nil
}
