package main

import (
	"strings"
	"testing"
)

func TestValidateECRConfig_CreateECR(t *testing.T) {
	env := &Env{}

	// Test nil config (should default to create_ecr)
	err := validateECRConfig(nil, "test-service", env)
	if err != nil {
		t.Errorf("Expected nil config to be valid, got error: %v", err)
	}

	// Test explicit create_ecr mode
	config := &ECRConfig{Mode: "create_ecr"}
	err = validateECRConfig(config, "test-service", env)
	if err != nil {
		t.Errorf("Expected create_ecr mode to be valid, got error: %v", err)
	}

	// Test empty mode (should default to create_ecr)
	config = &ECRConfig{Mode: ""}
	err = validateECRConfig(config, "test-service", env)
	if err != nil {
		t.Errorf("Expected empty mode to default to create_ecr, got error: %v", err)
	}
}

func TestValidateECRConfig_ManualRepo(t *testing.T) {
	env := &Env{}

	// Test valid manual repository URI
	config := &ECRConfig{
		Mode:          "manual_repo",
		RepositoryURI: "123456789012.dkr.ecr.us-east-1.amazonaws.com/my-repo",
	}
	err := validateECRConfig(config, "test-service", env)
	if err != nil {
		t.Errorf("Expected valid manual repo config to be valid, got error: %v", err)
	}

	// Test missing repository_uri
	config = &ECRConfig{
		Mode: "manual_repo",
	}
	err = validateECRConfig(config, "test-service", env)
	if err == nil {
		t.Error("Expected error for manual_repo without repository_uri")
	}
	if !strings.Contains(err.Error(), "repository_uri is required") {
		t.Errorf("Expected error about missing repository_uri, got: %v", err)
	}

	// Test invalid repository URI format
	testCases := []string{
		"invalid-uri",
		"123456789012.dkr.ecr.us-east-1.amazonaws.com", // Missing repo name
		"not-a-number.dkr.ecr.us-east-1.amazonaws.com/repo",
		"123456789012.dkr.ecr..amazonaws.com/repo",     // Invalid region
		"123456789012.s3.us-east-1.amazonaws.com/repo", // Wrong service
	}

	for _, invalidURI := range testCases {
		config = &ECRConfig{
			Mode:          "manual_repo",
			RepositoryURI: invalidURI,
		}
		err = validateECRConfig(config, "test-service", env)
		if err == nil {
			t.Errorf("Expected error for invalid URI '%s'", invalidURI)
		}
		if !strings.Contains(err.Error(), "must be in format") {
			t.Errorf("Expected format error for URI '%s', got: %v", invalidURI, err)
		}
	}
}

func TestValidateECRConfig_UseExisting(t *testing.T) {
	// Create env with source services
	env := &Env{
		Services: []Service{
			{
				Name:      "api",
				ECRConfig: &ECRConfig{Mode: "create_ecr"},
			},
			{
				Name:      "worker",
				ECRConfig: &ECRConfig{Mode: "manual_repo", RepositoryURI: "123456789012.dkr.ecr.us-east-1.amazonaws.com/worker"},
			},
		},
		EventProcessorTasks: []EventProcessorTask{
			{
				Name:      "processor",
				ECRConfig: &ECRConfig{Mode: "create_ecr"},
			},
		},
		ScheduledTasks: []ScheduledTask{
			{
				Name:      "daily-job",
				ECRConfig: &ECRConfig{Mode: "create_ecr"},
			},
		},
	}

	// Test valid use_existing from services
	config := &ECRConfig{
		Mode:              "use_existing",
		SourceServiceName: "api",
		SourceServiceType: "services",
	}
	err := validateECRConfig(config, "consumer", env)
	if err != nil {
		t.Errorf("Expected valid use_existing config to be valid, got error: %v", err)
	}

	// Test valid use_existing from event_processor_tasks
	config = &ECRConfig{
		Mode:              "use_existing",
		SourceServiceName: "processor",
		SourceServiceType: "event_processor_tasks",
	}
	err = validateECRConfig(config, "consumer", env)
	if err != nil {
		t.Errorf("Expected valid use_existing from event processor to be valid, got error: %v", err)
	}

	// Test valid use_existing from scheduled_tasks
	config = &ECRConfig{
		Mode:              "use_existing",
		SourceServiceName: "daily-job",
		SourceServiceType: "scheduled_tasks",
	}
	err = validateECRConfig(config, "consumer", env)
	if err != nil {
		t.Errorf("Expected valid use_existing from scheduled task to be valid, got error: %v", err)
	}

	// Test missing source_service_name
	config = &ECRConfig{
		Mode:              "use_existing",
		SourceServiceType: "services",
	}
	err = validateECRConfig(config, "consumer", env)
	if err == nil {
		t.Error("Expected error for use_existing without source_service_name")
	}
	if !strings.Contains(err.Error(), "source_service_name is required") {
		t.Errorf("Expected error about missing source_service_name, got: %v", err)
	}

	// Test missing source_service_type
	config = &ECRConfig{
		Mode:              "use_existing",
		SourceServiceName: "api",
	}
	err = validateECRConfig(config, "consumer", env)
	if err == nil {
		t.Error("Expected error for use_existing without source_service_type")
	}
	if !strings.Contains(err.Error(), "source_service_type is required") {
		t.Errorf("Expected error about missing source_service_type, got: %v", err)
	}

	// Test invalid source_service_type
	config = &ECRConfig{
		Mode:              "use_existing",
		SourceServiceName: "api",
		SourceServiceType: "invalid_type",
	}
	err = validateECRConfig(config, "consumer", env)
	if err == nil {
		t.Error("Expected error for invalid source_service_type")
	}
	if !strings.Contains(err.Error(), "must be 'services', 'event_processor_tasks', or 'scheduled_tasks'") {
		t.Errorf("Expected error about invalid source_service_type, got: %v", err)
	}

	// Test non-existent source service
	config = &ECRConfig{
		Mode:              "use_existing",
		SourceServiceName: "nonexistent",
		SourceServiceType: "services",
	}
	err = validateECRConfig(config, "consumer", env)
	if err == nil {
		t.Error("Expected error for non-existent source service")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected error about service not found, got: %v", err)
	}

	// Test source service with non-create_ecr mode
	config = &ECRConfig{
		Mode:              "use_existing",
		SourceServiceName: "worker",
		SourceServiceType: "services",
	}
	err = validateECRConfig(config, "consumer", env)
	if err == nil {
		t.Error("Expected error for source service with manual_repo mode")
	}
	if !strings.Contains(err.Error(), "must have ecr_config.mode='create_ecr'") {
		t.Errorf("Expected error about source mode, got: %v", err)
	}
}

func TestValidateECRConfig_InvalidMode(t *testing.T) {
	env := &Env{}

	config := &ECRConfig{
		Mode: "invalid_mode",
	}
	err := validateECRConfig(config, "test-service", env)
	if err == nil {
		t.Error("Expected error for invalid mode")
	}
	if !strings.Contains(err.Error(), "must be 'create_ecr', 'manual_repo', or 'use_existing'") {
		t.Errorf("Expected error about invalid mode, got: %v", err)
	}
}

func TestValidateAllECRConfigs(t *testing.T) {
	// Test environment with valid configurations
	env := &Env{
		Services: []Service{
			{Name: "api", ECRConfig: &ECRConfig{Mode: "create_ecr"}},
			{Name: "worker", ECRConfig: &ECRConfig{
				Mode:              "use_existing",
				SourceServiceName: "api",
				SourceServiceType: "services",
			}},
		},
		EventProcessorTasks: []EventProcessorTask{
			{Name: "processor", ECRConfig: &ECRConfig{Mode: "create_ecr"}},
		},
		ScheduledTasks: []ScheduledTask{
			{Name: "daily", ECRConfig: &ECRConfig{
				Mode:              "use_existing",
				SourceServiceName: "api",
				SourceServiceType: "services",
			}},
		},
	}

	err := ValidateAllECRConfigs(env)
	if err != nil {
		t.Errorf("Expected valid config to pass validation, got error: %v", err)
	}

	// Test environment with invalid configurations
	env = &Env{
		Services: []Service{
			{Name: "api", ECRConfig: &ECRConfig{Mode: "invalid_mode"}},
			{Name: "worker", ECRConfig: &ECRConfig{Mode: "manual_repo"}}, // Missing repository_uri
		},
	}

	err = ValidateAllECRConfigs(env)
	if err == nil {
		t.Error("Expected validation to fail for invalid configs")
	}
	// Should contain multiple errors
	if !strings.Contains(err.Error(), "api") || !strings.Contains(err.Error(), "worker") {
		t.Errorf("Expected errors for both services, got: %v", err)
	}
}

func TestValidateSourceServiceExists_DefaultMode(t *testing.T) {
	// Test that services with nil ECRConfig are treated as create_ecr
	env := &Env{
		Services: []Service{
			{Name: "api", ECRConfig: nil}, // Should default to create_ecr
		},
	}

	config := &ECRConfig{
		Mode:              "use_existing",
		SourceServiceName: "api",
		SourceServiceType: "services",
	}
	err := validateECRConfig(config, "worker", env)
	if err != nil {
		t.Errorf("Expected nil ECRConfig to be treated as create_ecr, got error: %v", err)
	}

	// Also test with empty mode
	env.Services[0].ECRConfig = &ECRConfig{Mode: ""}
	err = validateECRConfig(config, "worker", env)
	if err != nil {
		t.Errorf("Expected empty mode to be treated as create_ecr, got error: %v", err)
	}
}
