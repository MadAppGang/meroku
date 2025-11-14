package main

import (
	"fmt"
	"os"
	"path/filepath"

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
const CurrentSchemaVersion = 12

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
			domain["add_env_domain_prefix"] = false
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
		"multi_az":                             false,
		"storage_encrypted":                    true,
		"deletion_protection":                  false,
		"skip_final_snapshot":                  true,
		"iam_database_authentication_enabled":  false,
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
