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

// validateAppSyncAuthorizer is the single source of truth for "is this AppSync
// authorizer safe to deploy".
//
// The AppSync Lambda authorizer trusts exactly one thing: the JWKS endpoint it
// fetches signing keys from. It used to fall back to a hardcoded third-party
// endpoint, so an unconfigured API accepted tokens minted by a stranger.
// modules/appsync/variables.tf now declares jwks_uri with no default; this check
// turns the resulting terraform failure into an actionable message at generate
// time, before anyone waits on an apply.
//
// The requirement is keyed on `enabled`, not on `auth_lambda`: the module builds
// the authorizer for every instantiation (see the auth_lambda local in
// modules/appsync/variables.tf), so terraform demands jwks_uri whenever the
// module is present. authLambda only sharpens the wording.
func validateAppSyncAuthorizer(enabled, authLambda bool, jwksURI string) error {
	if !enabled {
		return nil
	}

	jwksURI = strings.TrimSpace(jwksURI)

	if jwksURI == "" {
		trigger := "pubsub_appsync.enabled is true"
		if authLambda {
			trigger = "pubsub_appsync.auth_lambda is true"
		}
		return fmt.Errorf(`pubsub_appsync.jwks_uri is required when %s

The AppSync Lambda authorizer verifies every JWT against this JWKS endpoint, and
it has no default: whoever controls that endpoint can mint tokens your API will
accept, so meroku will not pick one for you.

Set it in your <env>.yaml:

  pubsub_appsync:
    auth_lambda: true
    jwks_uri: "https://<your-idp-host>/.well-known/jwks.json"

Optionally pin the claims your provider issues:

    jwt_issuer: "https://<your-idp-host>/"
    jwt_audience: "<your-api-audience>"

Or set pubsub_appsync.enabled: false if you do not need AppSync in this
environment. The module always deploys the authorizer, so there is no way to
run AppSync without choosing a key source.`, trigger)
	}

	// Mirrors the module's own validation, but fails here with an explanation
	// instead of a terraform plan error.
	if !strings.HasPrefix(jwksURI, "https://") {
		return fmt.Errorf(`pubsub_appsync.jwks_uri must be an https:// URL, got %q

Over plaintext http an attacker on the network can swap the signing keys and
mint tokens your API accepts`, jwksURI)
	}

	return nil
}

// validateAppSyncConfig validates the AppSync block of a loaded Env struct.
func validateAppSyncConfig(env *Env) error {
	if env == nil {
		return nil
	}
	a := env.AppSyncPubSub
	return validateAppSyncAuthorizer(a.Enabled, a.AuthLambda, a.JWKSURI)
}

// validateAppSyncConfigMap validates the AppSync block of the raw map that is
// handed to the template engine. Generation renders from the map, not the
// struct, so this is the check that actually stands between a misconfigured
// project and a generated main.tf.
func validateAppSyncConfigMap(envMap map[string]interface{}) error {
	raw, exists := envMap["pubsub_appsync"]
	if !exists || raw == nil {
		return nil
	}

	// YAML gives map[interface{}]interface{}; convertToJSONCompatible gives
	// map[string]interface{}. Accept both so the check cannot be bypassed by
	// whichever loader the caller used.
	appsync := map[string]interface{}{}
	switch m := raw.(type) {
	case map[string]interface{}:
		appsync = m
	case map[interface{}]interface{}:
		for k, v := range m {
			if ks, ok := k.(string); ok {
				appsync[ks] = v
			}
		}
	default:
		return nil
	}

	enabled, _ := appsync["enabled"].(bool)
	authLambda, _ := appsync["auth_lambda"].(bool)
	jwksURI, _ := appsync["jwks_uri"].(string)

	return validateAppSyncAuthorizer(enabled, authLambda, jwksURI)
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
