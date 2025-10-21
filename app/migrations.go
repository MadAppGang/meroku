package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

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
const CurrentSchemaVersion = 7

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

// backupFile creates a timestamped backup of the original file
func backupFile(filepath string) error {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("failed to read file for backup: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	backupPath := filepath + ".backup_" + timestamp

	err = os.WriteFile(backupPath, data, 0644)
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
