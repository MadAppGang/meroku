package main

import (
	"fmt"
	"regexp"
	"strings"
)

// ECR repository URI pattern: <account-id>.dkr.ecr.<region>.amazonaws.com/<repo-name>
var ecrURIPattern = regexp.MustCompile(`^\d{12}\.dkr\.ecr\.[a-z0-9-]+\.amazonaws\.com\/[a-zA-Z0-9_-]+$`)

// validateECRConfig validates ECR configuration for a service/task
// Returns an error if the configuration is invalid
func validateECRConfig(config *ECRConfig, serviceName string, env *Env) error {
	if config == nil {
		// No ECR config means use default (create_ecr)
		return nil
	}

	// Normalize mode to default if empty
	mode := config.Mode
	if mode == "" {
		mode = "create_ecr"
	}

	// Validate mode is one of the allowed values
	if mode != "create_ecr" && mode != "manual_repo" && mode != "use_existing" {
		return fmt.Errorf("service '%s': ecr_config.mode must be 'create_ecr', 'manual_repo', or 'use_existing', got '%s'", serviceName, mode)
	}

	// Mode-specific validation
	switch mode {
	case "create_ecr":
		// No additional validation needed - ECR will be created automatically
		return nil

	case "manual_repo":
		// Require repository_uri and validate format
		if config.RepositoryURI == "" {
			return fmt.Errorf("service '%s': ecr_config.repository_uri is required when mode is 'manual_repo'", serviceName)
		}
		if !ecrURIPattern.MatchString(config.RepositoryURI) {
			return fmt.Errorf("service '%s': ecr_config.repository_uri must be in format '<account-id>.dkr.ecr.<region>.amazonaws.com/<repo-name>', got '%s'", serviceName, config.RepositoryURI)
		}
		return nil

	case "use_existing":
		// Require source_service_name and source_service_type
		if config.SourceServiceName == "" {
			return fmt.Errorf("service '%s': ecr_config.source_service_name is required when mode is 'use_existing'", serviceName)
		}
		if config.SourceServiceType == "" {
			return fmt.Errorf("service '%s': ecr_config.source_service_type is required when mode is 'use_existing'", serviceName)
		}

		// Validate source_service_type
		if config.SourceServiceType != "services" && config.SourceServiceType != "event_processor_tasks" && config.SourceServiceType != "scheduled_tasks" {
			return fmt.Errorf("service '%s': ecr_config.source_service_type must be 'services', 'event_processor_tasks', or 'scheduled_tasks', got '%s'", serviceName, config.SourceServiceType)
		}

		// Validate that source service exists and has create_ecr mode
		if err := validateSourceServiceExists(config.SourceServiceName, config.SourceServiceType, serviceName, env); err != nil {
			return err
		}

		return nil

	default:
		return fmt.Errorf("service '%s': unknown ecr_config.mode '%s'", serviceName, mode)
	}
}

// validateSourceServiceExists checks that the source service exists and uses create_ecr mode
func validateSourceServiceExists(sourceServiceName, sourceServiceType, currentServiceName string, env *Env) error {
	var sourceConfig *ECRConfig
	var found bool

	switch sourceServiceType {
	case "services":
		for _, svc := range env.Services {
			if svc.Name == sourceServiceName {
				found = true
				sourceConfig = svc.ECRConfig
				break
			}
		}

	case "event_processor_tasks":
		for _, task := range env.EventProcessorTasks {
			if task.Name == sourceServiceName {
				found = true
				sourceConfig = task.ECRConfig
				break
			}
		}

	case "scheduled_tasks":
		for _, task := range env.ScheduledTasks {
			if task.Name == sourceServiceName {
				found = true
				sourceConfig = task.ECRConfig
				break
			}
		}

	default:
		return fmt.Errorf("service '%s': invalid source_service_type '%s'", currentServiceName, sourceServiceType)
	}

	if !found {
		return fmt.Errorf("service '%s': source service '%s' not found in %s", currentServiceName, sourceServiceName, sourceServiceType)
	}

	// Validate that source service uses create_ecr mode (default or explicit)
	sourceMode := "create_ecr"
	if sourceConfig != nil && sourceConfig.Mode != "" {
		sourceMode = sourceConfig.Mode
	}

	if sourceMode != "create_ecr" {
		return fmt.Errorf("service '%s': source service '%s' must have ecr_config.mode='create_ecr', but has mode='%s'", currentServiceName, sourceServiceName, sourceMode)
	}

	return nil
}

// ValidateAllECRConfigs validates ECR configurations for all services, event processors, and scheduled tasks
func ValidateAllECRConfigs(env *Env) error {
	var errors []string

	// Validate services
	for _, svc := range env.Services {
		if err := validateECRConfig(svc.ECRConfig, svc.Name, env); err != nil {
			errors = append(errors, err.Error())
		}
	}

	// Validate event processor tasks
	for _, task := range env.EventProcessorTasks {
		if err := validateECRConfig(task.ECRConfig, task.Name, env); err != nil {
			errors = append(errors, err.Error())
		}
	}

	// Validate scheduled tasks
	for _, task := range env.ScheduledTasks {
		if err := validateECRConfig(task.ECRConfig, task.Name, env); err != nil {
			errors = append(errors, err.Error())
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("ECR configuration validation failed:\n  - %s", strings.Join(errors, "\n  - "))
	}

	return nil
}
