package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ServiceMapping holds actual ECS resource names from Terraform
type ServiceMapping struct {
	ServiceName string `json:"service_name"` // Actual ECS service name
	TaskFamily  string `json:"task_family"`  // Actual task definition family
}

// S3ServiceFile represents an S3 file used by a service
type S3ServiceFile struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
}

// Config holds all Lambda configuration loaded from environment variables
type Config struct {
	// Core Configuration
	ProjectName string
	Environment string
	AWSRegion   string
	LogLevel    LogLevel

	// ACTUAL ECS Resource Names from Terraform
	ClusterName     string
	ServiceMap      map[string]ServiceMapping // Service identifier → actual ECS names
	S3ToServiceMap  map[string][]S3ServiceFile // Service identifier → S3 files

	// Slack Configuration
	SlackWebhookURL          string
	EnableSlackNotifications bool

	// Deployment Configuration
	DeploymentTimeoutSeconds int
	MaxDeploymentRetries     int
	DryRun                   bool

	// Feature Flags
	EnableECRMonitoring bool
	EnableSSMMonitoring bool
	EnableS3Monitoring  bool
	EnableManualDeploy  bool
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

		// Actual ECS Resource Names from Terraform
		ClusterName: getEnv("ECS_CLUSTER_NAME", ""),

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

	// Parse ECS_SERVICE_MAP JSON
	serviceMapJSON := getEnv("ECS_SERVICE_MAP", "{}")
	if err := json.Unmarshal([]byte(serviceMapJSON), &cfg.ServiceMap); err != nil {
		return nil, fmt.Errorf("failed to parse ECS_SERVICE_MAP: %w", err)
	}

	// Parse S3_SERVICE_MAP JSON
	s3MapJSON := getEnv("S3_SERVICE_MAP", "{}")
	if err := json.Unmarshal([]byte(s3MapJSON), &cfg.S3ToServiceMap); err != nil {
		return nil, fmt.Errorf("failed to parse S3_SERVICE_MAP: %w", err)
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
	if c.ClusterName == "" {
		errors = append(errors, "ECS_CLUSTER_NAME is required")
	}

	// Service map validation
	if len(c.ServiceMap) == 0 {
		errors = append(errors, "ECS_SERVICE_MAP is required and must contain at least one service")
	}

	// Validate each service mapping
	for serviceID, mapping := range c.ServiceMap {
		if mapping.ServiceName == "" {
			errors = append(errors, fmt.Sprintf("service '%s': service_name is required", serviceID))
		}
		if mapping.TaskFamily == "" {
			errors = append(errors, fmt.Sprintf("service '%s': task_family is required", serviceID))
		}
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

// GetServiceMapping returns the ECS service and task family names for a service identifier
func (c *Config) GetServiceMapping(serviceIdentifier string) (ServiceMapping, error) {
	mapping, exists := c.ServiceMap[serviceIdentifier]
	if !exists {
		return ServiceMapping{}, fmt.Errorf("service '%s' not found in ECS_SERVICE_MAP", serviceIdentifier)
	}
	return mapping, nil
}

// GetClusterName returns the actual ECS cluster name
func (c *Config) GetClusterName() string {
	return c.ClusterName
}

// GetServiceName returns the actual ECS service name for a service identifier
func (c *Config) GetServiceName(serviceIdentifier string) (string, error) {
	mapping, err := c.GetServiceMapping(serviceIdentifier)
	if err != nil {
		return "", err
	}
	return mapping.ServiceName, nil
}

// GetTaskFamily returns the actual task definition family for a service identifier
func (c *Config) GetTaskFamily(serviceIdentifier string) (string, error) {
	mapping, err := c.GetServiceMapping(serviceIdentifier)
	if err != nil {
		return "", err
	}
	return mapping.TaskFamily, nil
}

// GetServicesForS3File returns all service identifiers that use a specific S3 file
func (c *Config) GetServicesForS3File(bucket, key string) []string {
	var services []string

	for serviceID, files := range c.S3ToServiceMap {
		for _, file := range files {
			if file.Bucket == bucket && file.Key == key {
				services = append(services, serviceID)
				break // Move to next service
			}
		}
	}

	return services
}

// ListAllServices returns all service identifiers
func (c *Config) ListAllServices() []string {
	services := make([]string, 0, len(c.ServiceMap))
	for serviceID := range c.ServiceMap {
		services = append(services, serviceID)
	}
	return services
}

// GetServiceInfo returns detailed info about a service for debugging
func (c *Config) GetServiceInfo(serviceIdentifier string) map[string]interface{} {
	mapping, err := c.GetServiceMapping(serviceIdentifier)
	if err != nil {
		return map[string]interface{}{
			"error": err.Error(),
		}
	}

	s3Files := c.S3ToServiceMap[serviceIdentifier]

	return map[string]interface{}{
		"identifier":   serviceIdentifier,
		"service_name": mapping.ServiceName,
		"task_family":  mapping.TaskFamily,
		"cluster_name": c.ClusterName,
		"s3_files":     s3Files,
	}
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
