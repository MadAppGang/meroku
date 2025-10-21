package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all Lambda configuration loaded from environment variables
type Config struct {
	// Core Configuration
	ProjectName string
	Environment string
	AWSRegion   string
	LogLevel    LogLevel

	// Naming Patterns - allows customization of resource naming
	ClusterNamePattern  string
	ServiceNamePattern  string
	TaskFamilyPattern   string
	BackendServiceName  string

	// Slack Configuration
	SlackWebhookURL           string
	EnableSlackNotifications  bool

	// S3 Service Configuration
	// Maps service names to their S3 env files
	// Format: {"service_name": [{"bucket": "...", "key": "..."}]}
	ServiceConfig map[string][]S3File

	// Deployment Configuration
	DeploymentTimeoutSeconds int
	MaxDeploymentRetries     int
	DryRun                   bool

	// Feature Flags
	EnableECRMonitoring   bool
	EnableSSMMonitoring   bool
	EnableS3Monitoring    bool
	EnableManualDeploy    bool
}

// S3File represents an S3 object reference
type S3File struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
}

// LogLevel represents logging verbosity
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// LoadFromEnv loads configuration from environment variables with validation
func LoadFromEnv() (*Config, error) {
	cfg := &Config{
		// Required fields
		ProjectName: getEnv("PROJECT_NAME", ""),
		Environment: getEnv("PROJECT_ENV", ""),

		// AWS Configuration
		AWSRegion: getEnv("AWS_REGION", "us-east-1"),

		// Logging
		LogLevel: LogLevel(getEnv("LOG_LEVEL", "info")),

		// Naming Patterns (with sensible defaults)
		ClusterNamePattern:  getEnv("CLUSTER_NAME_PATTERN", "{project}_cluster_{env}"),
		ServiceNamePattern:  getEnv("SERVICE_NAME_PATTERN", "{project}_service_{name}_{env}"),
		TaskFamilyPattern:   getEnv("TASK_FAMILY_PATTERN", "{project}_service_{name}_{env}"),
		BackendServiceName:  getEnv("BACKEND_SERVICE_NAME", ""),

		// Slack Configuration
		SlackWebhookURL:          getEnv("SLACK_WEBHOOK_URL", ""),
		EnableSlackNotifications: getBoolEnv("ENABLE_SLACK_NOTIFICATIONS", true),

		// Deployment Configuration
		DeploymentTimeoutSeconds: getIntEnv("DEPLOYMENT_TIMEOUT_SECONDS", 600),
		MaxDeploymentRetries:     getIntEnv("MAX_DEPLOYMENT_RETRIES", 2),
		DryRun:                   getBoolEnv("DRY_RUN", false),

		// Feature Flags
		EnableECRMonitoring: getBoolEnv("ENABLE_ECR_MONITORING", true),
		EnableSSMMonitoring: getBoolEnv("ENABLE_SSM_MONITORING", true),
		EnableS3Monitoring:  getBoolEnv("ENABLE_S3_MONITORING", true),
		EnableManualDeploy:  getBoolEnv("ENABLE_MANUAL_DEPLOY", true),
	}

	// Parse SERVICE_CONFIG JSON if provided
	serviceConfigJSON := getEnv("SERVICE_CONFIG", "{}")
	if err := json.Unmarshal([]byte(serviceConfigJSON), &cfg.ServiceConfig); err != nil {
		return nil, fmt.Errorf("failed to parse SERVICE_CONFIG: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks that all required configuration is present and valid
func (c *Config) Validate() error {
	var errors []string

	// Required fields
	if c.ProjectName == "" {
		errors = append(errors, "PROJECT_NAME is required")
	}
	if c.Environment == "" {
		errors = append(errors, "PROJECT_ENV is required")
	}

	// Log level validation
	validLogLevels := map[LogLevel]bool{
		LogLevelDebug: true,
		LogLevelInfo:  true,
		LogLevelWarn:  true,
		LogLevelError: true,
	}
	if !validLogLevels[c.LogLevel] {
		errors = append(errors, fmt.Sprintf("LOG_LEVEL must be one of: debug, info, warn, error (got: %s)", c.LogLevel))
	}

	// Slack validation
	if c.EnableSlackNotifications && c.SlackWebhookURL == "" {
		errors = append(errors, "SLACK_WEBHOOK_URL is required when ENABLE_SLACK_NOTIFICATIONS is true")
	}

	// Pattern validation
	if !strings.Contains(c.ClusterNamePattern, "{project}") || !strings.Contains(c.ClusterNamePattern, "{env}") {
		errors = append(errors, "CLUSTER_NAME_PATTERN must contain {project} and {env} placeholders")
	}
	if !strings.Contains(c.ServiceNamePattern, "{project}") || !strings.Contains(c.ServiceNamePattern, "{env}") {
		errors = append(errors, "SERVICE_NAME_PATTERN must contain {project} and {env} placeholders")
	}
	if !strings.Contains(c.TaskFamilyPattern, "{project}") || !strings.Contains(c.TaskFamilyPattern, "{env}") {
		errors = append(errors, "TASK_FAMILY_PATTERN must contain {project} and {env} placeholders")
	}

	// Deployment configuration
	if c.DeploymentTimeoutSeconds <= 0 {
		errors = append(errors, "DEPLOYMENT_TIMEOUT_SECONDS must be positive")
	}
	if c.MaxDeploymentRetries < 0 {
		errors = append(errors, "MAX_DEPLOYMENT_RETRIES must be non-negative")
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation errors:\n  - %s", strings.Join(errors, "\n  - "))
	}

	return nil
}

// GetClusterName returns the ECS cluster name for this environment
func (c *Config) GetClusterName() string {
	return c.applyPattern(c.ClusterNamePattern, "", "")
}

// GetServiceName returns the ECS service name for a given service
func (c *Config) GetServiceName(serviceName string) string {
	return c.applyPattern(c.ServiceNamePattern, serviceName, "")
}

// GetTaskFamily returns the task definition family for a given service
func (c *Config) GetTaskFamily(serviceName string) string {
	return c.applyPattern(c.TaskFamilyPattern, serviceName, "")
}

// applyPattern replaces placeholders in pattern strings
func (c *Config) applyPattern(pattern, serviceName, imageTag string) string {
	result := pattern
	result = strings.ReplaceAll(result, "{project}", c.ProjectName)
	result = strings.ReplaceAll(result, "{env}", c.Environment)

	// Handle backend service special case - remove {name} placeholder entirely
	if serviceName == "" || serviceName == c.BackendServiceName {
		// For backend: {project}_service_{name}_{env} → {project}_service_{env}
		result = strings.ReplaceAll(result, "_{name}", "")
		result = strings.ReplaceAll(result, "{name}_", "")
		result = strings.ReplaceAll(result, "{name}", "")
	} else {
		// For named services: replace {name} with actual service name
		result = strings.ReplaceAll(result, "{name}", serviceName)
	}

	result = strings.ReplaceAll(result, "{tag}", imageTag)

	return result
}

// IsBackendService checks if a service name refers to the backend service
func (c *Config) IsBackendService(serviceName string) bool {
	return serviceName == "" || serviceName == c.BackendServiceName || serviceName == "backend"
}

// NormalizeServiceName normalizes service names (backend -> empty string)
func (c *Config) NormalizeServiceName(serviceName string) string {
	if c.IsBackendService(serviceName) {
		return ""
	}
	return serviceName
}

// Helper functions for environment variable parsing

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		// Parse various boolean representations
		lower := strings.ToLower(value)
		switch lower {
		case "true", "1", "yes", "on", "enabled":
			return true
		case "false", "0", "no", "off", "disabled":
			return false
		}
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
