package main

import (
	"context"
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

	// The CI Lambda is built by terraform itself (modules/workloads/lambda.tf:
	// null_resource.build_ci_lambda -> .build/<env>/ci_lambda.zip, linux/arm64).
	// meroku used to build a second, linux/amd64 copy here that nothing ever
	// read; the build and its pre-flight checks are gone, and the Go toolchain
	// requirement is stated once, by the provisioner that actually needs it.

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

	// Decide which deployment plan this environment needs before touching
	// terraform. An undelegated DNS zone otherwise parks the whole apply on ACM
	// validation, because module.workloads consumes certificate ARNs that come
	// from aws_acm_certificate_validation.
	ctx := context.Background()
	needsZoneBootstrap := false

	dnsResult, dnsErr := checkDNSPreflight(ctx, e)
	if dnsErr != nil {
		// Fail open: an unreachable Route53 or resolver must not block a deploy
		// that would otherwise succeed. Say so rather than pretending it passed.
		fmt.Printf("\n⚠️  Could not verify DNS delegation: %v\n", dnsErr)
		fmt.Println("   Continuing anyway — if the domain is not delegated, certificate")
		fmt.Println("   validation will time out and the apply will fail.")
	} else {
		fmt.Printf("\n%s", describeDNSPreflight(dnsResult))

		switch dnsResult.Plan {
		case dnsPlanBlocked:
			// The zone exists but is not delegated. meroku can usually fix this
			// itself, so hand off to the DNS setup screen rather than stopping.
			outcome, err := runDNSSetupTUI(e, dnsResult)
			if err != nil {
				fmt.Printf("\n❌ DNS setup failed: %v\n", err)
				os.Exit(1)
			}
			if applyDNSOutcome(env, &e, outcome) {
				// The config changed, so the generated terraform is now stale.
				applyTemplate(env)
			}

		case dnsPlanMissingZone:
			fmt.Println("\n❌ Deployment stopped: DNS configuration problem.")
			fmt.Println("   Fix the issue above, then run the deploy again.")
			os.Exit(1)

		case dnsPlanBootstrap:
			// Two-phase deploy. Phase 1 needs terraform, so it has to wait until
			// after the chdir and init below.
			needsZoneBootstrap = true
		}
	}

	err = os.Chdir(filepath.Join("env", env))
	if err != nil {
		fmt.Println("Error changing directory to env folder:", err)
		os.Exit(1)
	}
	terraformInitIfNeeded()

	// Phase 1 of a two-phase deploy: create the hosted zone on its own, set up
	// delegation, and only then run the full apply. Splitting is what makes a
	// brand-new domain deployable at all — in one pass the certificate validation
	// waits on a delegation that cannot exist until the zone it is waiting for has
	// been created.
	if needsZoneBootstrap {
		outcome, err := runDNSBootstrapAndDelegate(ctx, e)
		if err != nil {
			fmt.Printf("\n❌ DNS bootstrap failed: %v\n", err)
			os.Exit(1)
		}

		// Regeneration reads <env>.yaml and writes env/<env>/, both relative to
		// the project root — but we are inside env/<env> by now, so it has to be
		// done from there and the directory restored afterwards.
		if outcome.SkipDomain {
			if err := os.Chdir(wd); err != nil {
				fmt.Println("Error returning to the project directory:", err)
				os.Exit(1)
			}
			applyDNSOutcome(env, &e, outcome)
			applyTemplate(env)
			if err := os.Chdir(filepath.Join("env", env)); err != nil {
				fmt.Println("Error changing directory to env folder:", err)
				os.Exit(1)
			}
			terraformInitIfNeeded()
		} else {
			applyDNSOutcome(env, &e, outcome)
		}

		fmt.Println("\n🌐 Phase 2 of 2: deploying everything else")
	}

	return runTerraformApply()
}

// generateEnvironmentFiles writes env/<env>/ from <env>.yaml — the whole of what
// `meroku generate` does to disk, and nothing else.
//
// Extracted so the state-reconnect recovery path can restore a missing directory
// without calling handleGenerateCommand, which ends by running the reconnect and
// would therefore re-enter whatever called it. Recovery calls this; only the
// command calls the command.
//
// The inputs are checked here rather than left to applyTemplate, which exits the
// process on a missing template. Recovery has just told the user their
// infrastructure is intact; ending that sentence with an abrupt os.Exit would be
// a poor way to keep the promise.
func generateEnvironmentFiles(env string) error {
	registerCustomHelpers()

	envFile := env + ".yaml"
	if _, err := os.Stat(envFile); err != nil {
		return fmt.Errorf("environment file '%s' not found", envFile)
	}
	templateFile := filepath.Join("infrastructure", "env", "main.hbs")
	if _, err := os.Stat(templateFile); err != nil {
		return fmt.Errorf("template '%s' not found — run this from the project root", templateFile)
	}

	if err := createFolderIfNotExists("env"); err != nil {
		return fmt.Errorf("creating env directory: %w", err)
	}
	if err := createFolderIfNotExists(filepath.Join("env", env)); err != nil {
		return fmt.Errorf("creating env/%s directory: %w", env, err)
	}

	applyTemplate(env)
	return nil
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

	if err := generateEnvironmentFiles(env); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Generated: env/%s/main.tf\n", env)

	// A freshly generated env/<env> with no .terraform is the shape this guards
	// against: the directory is here, but nothing links it to the backend that
	// holds the deployment. Every terraform command after this point would fail
	// with "Backend initialization required", which reads like the infrastructure
	// is gone.
	//
	// Generate is otherwise a local operation, and it stays one for everyone whose
	// directory is already initialised — inspectStateConnection short-circuits on
	// env/<env>/.terraform and only reaches S3 when the directory is not connected
	// to a backend. When it does reach out it asks before doing anything, and any
	// AWS problem degrades to the old behaviour instead of failing the generate.
	//
	// The sync it may offer cannot loop back here: it calls
	// generateEnvironmentFiles, not this command. And by this point env/<env>/
	// exists anyway, so it has nothing to write.
	cfg, loadErr := loadEnv(env)
	if loadErr != nil {
		return
	}
	if err := reconnectStateIfNeeded(context.Background(), env, cfg, filepath.Join("env", env)); err != nil {
		// Generation itself succeeded and its output is valid, so this is reported
		// rather than fatal. The user is told exactly what to run by hand.
		fmt.Printf("⚠️  Could not finish syncing with the deployed state: %v\n", err)
	}
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

	// Refuse to generate terraform that would deploy an authorizer trusting a
	// JWKS endpoint nobody chose. Failing here costs a second; the same mistake
	// found by terraform costs an apply.
	if err := validateAppSyncConfigMap(envMap); err != nil {
		fmt.Printf("\n❌ Invalid configuration in %s.yaml:\n\n%v\n\n", env, err)
		os.Exit(1)
	}

	// Filter out disabled services (enabled=false) before rendering
	filterDisabledItems(envMap, "services")
	filterDisabledItems(envMap, "scheduled_tasks")
	filterDisabledItems(envMap, "event_processor_tasks")

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

// filterDisabledItems removes items with enabled=false from the given key in env map
// so they are not rendered into the Terraform output.
// Items with enabled=true or no enabled field are kept.
func filterDisabledItems(envMap map[string]interface{}, key string) {
	itemsRaw, ok := envMap[key]
	if !ok || itemsRaw == nil {
		return
	}

	items, ok := itemsRaw.([]interface{})
	if !ok {
		return
	}

	filtered := make([]interface{}, 0, len(items))
	for _, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			filtered = append(filtered, item)
			continue
		}

		enabled, hasEnabled := itemMap["enabled"]
		if !hasEnabled || enabled == true {
			filtered = append(filtered, item)
		}
	}

	envMap[key] = filtered
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
