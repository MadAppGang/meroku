package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aymerick/raymond"
	"github.com/charmbracelet/huh"
	"github.com/samber/lo"
)

func deployMenu() {
	var env string

	// Use already selected environment if available
	if selectedEnvironment != "" {
		env = selectedEnvironment
		fmt.Printf("Deploying environment: %s\n", env)
	} else {
		// Only prompt for environment selection if none is selected
		envs, err := findFilesWithExts([]string{".yaml", ".yml"})
		if err != nil {
			panic(err)
		}
		// Filter out DNS config file
		var filteredEnvs []string
		for _, env := range envs {
			if env != "dns.yaml" {
				filteredEnvs = append(filteredEnvs, env)
			}
		}
		options := lo.Map(filteredEnvs, func(s string, _ int) huh.Option[string] {
			return huh.NewOption(fmt.Sprintf("Deploy %s environment", s), s)
		})
		options = append(options, huh.NewOption("Back to main menu", "go:back"))

		huh.NewSelect[string]().
			Title("Select an environment to deploy").
			Options(
				options...,
			).
			Value(&env).
			Run()

		switch env {
		case "go:back":
			return
		case "":
			fmt.Println("No environment selected")
			os.Exit(1)
		}
	}

	runCommandToDeploy(env)
}

func runCommandToDeploy(env string) error {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting current working directory:", err)
		os.Exit(1)
	}
	defer os.Chdir(wd)

	createFolderIfNotExists("env")
	err = createFolderIfNotExists(filepath.Join("env", env))
	if err != nil {
		fmt.Println("Error creating folder for environment:", err)
		os.Exit(1)
	}
	//
	applyTemplate(env)
	buildDeploymentLambda(env)

	e, err := loadEnv(env)
	if err != nil {
		fmt.Println("Error loading environment:", err)
		os.Exit(1)
	}

	// Run comprehensive AWS pre-flight checks BEFORE changing directory
	// This validates credentials, checks/creates S3 bucket, and handles SSO refresh
	fmt.Printf("\n🚀 Starting deployment for environment: %s\n", env)
	if err := AWSPreflightCheck(e); err != nil {
		fmt.Printf("\n%v\n\n", err)
		fmt.Println("❌ Pre-flight checks failed. Please fix the issues above and try again.")
		os.Exit(1)
	}

	err = os.Chdir(filepath.Join("env", env))
	if err != nil {
		fmt.Println("Error changing directory to env folder:", err)
		os.Exit(1)
	}
	terraformInitIfNeeded()
	return runTerraformApply()
}

func handleGenerateCommand(args []string) {
	// Register custom Handlebars helpers
	registerCustomHelpers()

	if len(args) == 0 {
		fmt.Println("Usage: meroku generate <environment>")
		fmt.Println("Example: meroku generate dev")
		fmt.Println("")
		fmt.Println("Generates Terraform configuration files from YAML templates.")
		os.Exit(1)
	}

	env := args[0]
	fmt.Printf("Generating Terraform configuration for environment: %s\n", env)

	// Check if environment file exists
	envFile := env + ".yaml"
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		fmt.Printf("Error: Environment file '%s' not found\n", envFile)
		os.Exit(1)
	}

	// Create env directory structure
	createFolderIfNotExists("env")
	if err := createFolderIfNotExists(filepath.Join("env", env)); err != nil {
		fmt.Printf("Error creating environment directory: %v\n", err)
		os.Exit(1)
	}

	// Generate template
	applyTemplate(env)

	fmt.Printf("✓ Generated: env/%s/main.tf\n", env)
}

func applyTemplate(env string) {
	// Read the template file
	templateContent, err := os.ReadFile(filepath.Join("infrastructure", "env", "main.hbs"))
	if err != nil {
		fmt.Printf("error reading template file: %v", err)
		os.Exit(1)
	}

	envMap, err := loadEnvToMap(env + ".yaml")
	if err != nil {
		fmt.Printf("error loading environment: %v", err)
		os.Exit(1)
	}
	// Filter out disabled services (enabled=false) before rendering
	filterDisabledServices(envMap)

	envMap["modules"] = "../../infrastructure/modules"
	envMap["custom_modules"] = "../../custom"

	// Check for custom pre/post modules and set flags
	envMap["has_custom_pre"] = hasCustomModule("pre")
	envMap["has_custom_post"] = hasCustomModule("post")

	// Create a new template and parse the content
	tmpl, err := raymond.Parse(string(templateContent))
	if err != nil {
		fmt.Printf("error parsing template: %v", err)
		os.Exit(1)
	}
	// Execute the template with the environment data
	result, err := tmpl.Exec(envMap)
	if err != nil {
		fmt.Printf("Error executing template: %+v\n", err)
		os.Exit(1)
	}

	// Append custom terraform files
	customTf := getCustomTerraformContent(env)
	if customTf != "" {
		result = result + "\n" + customTf
	}

	// Generate bridge file for custom terraform reference
	generateBridgeFile(env, envMap)

	os.WriteFile(filepath.Join("env", env, "main.tf"), []byte(result), 0o644)
}

// filterDisabledServices removes services with enabled=false from the env map
// so they are not rendered into the Terraform output.
// Services with enabled=true or no enabled field are kept.
func filterDisabledServices(envMap map[string]interface{}) {
	servicesRaw, ok := envMap["services"]
	if !ok || servicesRaw == nil {
		return
	}

	services, ok := servicesRaw.([]interface{})
	if !ok {
		return
	}

	filtered := make([]interface{}, 0, len(services))
	for _, svc := range services {
		svcMap, ok := svc.(map[string]interface{})
		if !ok {
			filtered = append(filtered, svc)
			continue
		}

		enabled, hasEnabled := svcMap["enabled"]
		if !hasEnabled || enabled == true {
			filtered = append(filtered, svc)
		}
	}

	envMap["services"] = filtered
}

// hasCustomModule checks if a custom pre or post module exists
func hasCustomModule(moduleType string) bool {
	mainTfPath := filepath.Join("custom", moduleType, "main.tf")
	_, err := os.Stat(mainTfPath)
	return err == nil
}

// getCustomTerraformContent reads and concatenates custom .tf files
func getCustomTerraformContent(env string) string {
	var content strings.Builder

	// Collect files from _shared and environment-specific directories
	dirs := []string{
		filepath.Join("custom", "terraform", "_shared"),
		filepath.Join("custom", "terraform", env),
	}

	filesFound := false
	for _, dir := range dirs {
		files, err := filepath.Glob(filepath.Join(dir, "*.tf"))
		if err != nil {
			continue
		}

		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				fmt.Printf("⚠️  Warning: Could not read custom terraform file %s: %v\n", file, err)
				continue
			}

			if !filesFound {
				content.WriteString("\n# ============================================================================\n")
				content.WriteString("# CUSTOM TERRAFORM (from custom/terraform/)\n")
				content.WriteString("# ============================================================================\n\n")
				filesFound = true
			}

			relPath, _ := filepath.Rel(".", file)
			content.WriteString(fmt.Sprintf("# Source: %s\n", relPath))
			content.Write(data)
			content.WriteString("\n\n")
		}
	}

	if filesFound {
		fmt.Printf("✓ Included custom terraform files\n")
	}

	return content.String()
}

// generateBridgeFile creates a _bridge.tf file exposing all module outputs for custom terraform
func generateBridgeFile(env string, envMap map[string]interface{}) {
	project, _ := envMap["project"].(string)
	region, _ := envMap["region"].(string)
	accountID, _ := envMap["account_id"].(string)

	// Check which modules are enabled
	postgresEnabled := false
	if pg, ok := envMap["postgres"].(map[string]interface{}); ok {
		if enabled, ok := pg["enabled"].(bool); ok {
			postgresEnabled = enabled
		}
	}

	domainEnabled := false
	if domain, ok := envMap["domain"].(map[string]interface{}); ok {
		if enabled, ok := domain["enabled"].(bool); ok {
			domainEnabled = enabled
		}
	}

	useDefaultVPC := true
	if val, ok := envMap["use_default_vpc"].(bool); ok {
		useDefaultVPC = val
	}

	bridge := fmt.Sprintf(`# ============================================================================
# BRIDGE FILE - Auto-generated by meroku
# Exposes all module outputs for use in custom terraform code
# DO NOT EDIT - This file is regenerated on each deployment
# ============================================================================

locals {
  bridge = {
    # Project context
    project    = "%s"
    env        = "%s"
    region     = "%s"
    account_id = "%s"

    # VPC
    vpc_id     = local.vpc_id
    subnet_ids = local.subnet_ids

    # Workloads module outputs
    api_endpoint         = module.workloads.api_gateway_endpoint
    api_gateway_id       = module.workloads.api_gateway_id
    ecs_cluster_arn      = module.workloads.ecr_cluster.arn
    ecs_cluster_name     = module.workloads.ecr_cluster.name
    backend_ecr_repo_url = module.workloads.backend_ecr_repo_url
    backend_task_role    = module.workloads.backend_task_role_name
    backend_cloud_map_arn = module.workloads.backend_cloud_map_arn
`, project, env, region, accountID)

	// Add domain outputs if enabled
	if domainEnabled {
		bridge += `
    # Domain module outputs (domain.enabled = true)
    domain_zone_id            = module.domain.zone_id
    api_domain                = module.domain.api_domain_name
    api_certificate_arn       = module.domain.api_certificate_arn
    subdomains_certificate_arn = module.domain.subdomains_certificate_arn
`
	} else {
		bridge += `
    # Domain module outputs (domain.enabled = false)
    domain_zone_id            = ""
    api_domain                = ""
    api_certificate_arn       = ""
    subdomains_certificate_arn = ""
`
	}

	// Add postgres outputs if enabled
	if postgresEnabled {
		bridge += `
    # Postgres module outputs (postgres.enabled = true)
    db_endpoint = module.postgres.endpoint
    db_user     = module.postgres.user
    db_name     = module.postgres.db_name
`
	} else {
		bridge += `
    # Postgres module outputs (postgres.enabled = false)
    db_endpoint = ""
    db_user     = ""
    db_name     = ""
`
	}

	// Add VPC-specific outputs
	if !useDefaultVPC {
		bridge += `
    # Custom VPC outputs (use_default_vpc = false)
    vpc_cidr = module.vpc.vpc_cidr
`
	}

	bridge += `  }
}

# Convenience outputs for terraform state reference
output "bridge" {
  description = "All bridge values for custom terraform reference"
  value       = local.bridge
  sensitive   = false
}
`

	bridgePath := filepath.Join("env", env, "_bridge.tf")
	if err := os.WriteFile(bridgePath, []byte(bridge), 0o644); err != nil {
		fmt.Printf("⚠️  Warning: Could not write bridge file: %v\n", err)
		return
	}
	fmt.Printf("✓ Generated: env/%s/_bridge.tf\n", env)
}

func buildDeploymentLambda(env string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("error getting current working directory: %w", err)
	}
	defer os.Chdir(wd)

	os.RemoveAll(filepath.Join("env", env, "ci_lambda.zip"))
	os.Chdir("infrastructure/modules/workloads/ci_lambda")
	os.RemoveAll("bootstrap")

	os.Setenv("GOOS", "linux")
	os.Setenv("GOARCH", "amd64")
	if _, err := runCommandWithOutput("go", "build", "-o", "bootstrap", "."); err != nil {
		return fmt.Errorf("error building deployment lambda: %w", err)
	}

	return nil
}
