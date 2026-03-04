package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setEnv sets environment variables for testing and returns a cleanup function.
func setEnv(t *testing.T, kv map[string]string) func() {
	t.Helper()
	for k, v := range kv {
		require.NoError(t, os.Setenv(k, v))
	}
	return func() {
		for k := range kv {
			os.Unsetenv(k)
		}
	}
}

func TestIsScheduledTask(t *testing.T) {
	cfg := &Config{
		ServiceMap: map[string]ServiceMapping{
			"": {
				ServiceName: "myapp_service_dev",
				TaskFamily:  "myapp_backend_dev",
				Type:        ServiceMappingTypeService,
			},
			"api": {
				ServiceName: "myapp_service_api_dev",
				TaskFamily:  "myapp_service_api_dev",
				Type:        ServiceMappingTypeService,
			},
			"task:cleanup": {
				TaskFamily: "myapp_task_cleanup_dev",
				Type:       ServiceMappingTypeScheduledTask,
			},
		},
	}

	assert.False(t, cfg.IsScheduledTask(""), "backend should not be a scheduled task")
	assert.False(t, cfg.IsScheduledTask("api"), "named service should not be a scheduled task")
	assert.True(t, cfg.IsScheduledTask("task:cleanup"), "task:cleanup should be a scheduled task")
	assert.False(t, cfg.IsScheduledTask("nonexistent"), "missing key should return false")
}

func TestValidate_ScheduledTaskAllowsEmptyServiceName(t *testing.T) {
	cfg := &Config{
		ProjectName:              "myapp",
		Environment:              "dev",
		ClusterName:              "myapp_cluster_dev",
		LogLevel:                 LogLevelInfo,
		DeploymentTimeoutSeconds: 600,
		MaxDeploymentRetries:     2,
		ServiceMap: map[string]ServiceMapping{
			"task:cleanup": {
				TaskFamily: "myapp_task_cleanup_dev",
				Type:       ServiceMappingTypeScheduledTask,
				// ServiceName intentionally empty — allowed for scheduled tasks
			},
		},
	}

	err := cfg.Validate()
	assert.NoError(t, err, "empty ServiceName should be allowed for scheduled tasks")
}

func TestValidate_RegularServiceRequiresServiceName(t *testing.T) {
	cfg := &Config{
		ProjectName:              "myapp",
		Environment:              "dev",
		ClusterName:              "myapp_cluster_dev",
		LogLevel:                 LogLevelInfo,
		DeploymentTimeoutSeconds: 600,
		MaxDeploymentRetries:     2,
		ServiceMap: map[string]ServiceMapping{
			"api": {
				TaskFamily: "myapp_service_api_dev",
				Type:       ServiceMappingTypeService,
				// ServiceName missing — should fail validation
			},
		},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service_name is required")
}

func TestLoadFromEnv_ScheduledTaskMap(t *testing.T) {
	cleanup := setEnv(t, map[string]string{
		"PROJECT_NAME":     "myapp",
		"PROJECT_ENV":      "dev",
		"ECS_CLUSTER_NAME": "myapp_cluster_dev",
		"ECS_SERVICE_MAP": `{
			"": {"service_name": "myapp_service_dev", "task_family": "myapp_backend_dev"}
		}`,
		"SCHEDULED_TASK_MAP": `{
			"task:cleanup": {"task_family": "myapp_task_cleanup_dev"}
		}`,
	})
	defer cleanup()

	cfg, err := LoadFromEnv()
	require.NoError(t, err)

	// Regular service should be present and have type "service" (default)
	assert.Contains(t, cfg.ServiceMap, "")

	// Scheduled task should be merged in with type "scheduled_task"
	require.Contains(t, cfg.ServiceMap, "task:cleanup")
	taskMapping := cfg.ServiceMap["task:cleanup"]
	assert.Equal(t, ServiceMappingTypeScheduledTask, taskMapping.Type)
	assert.Equal(t, "myapp_task_cleanup_dev", taskMapping.TaskFamily)
	assert.Empty(t, taskMapping.ServiceName)

	assert.True(t, cfg.IsScheduledTask("task:cleanup"))
	assert.False(t, cfg.IsScheduledTask(""))
}

func TestLoadFromEnv_InvalidScheduledTaskMap(t *testing.T) {
	cleanup := setEnv(t, map[string]string{
		"PROJECT_NAME":       "myapp",
		"PROJECT_ENV":        "dev",
		"ECS_CLUSTER_NAME":   "myapp_cluster_dev",
		"ECS_SERVICE_MAP":    `{"": {"service_name": "svc", "task_family": "fam"}}`,
		"SCHEDULED_TASK_MAP": `not-valid-json`,
	})
	defer cleanup()

	_, err := LoadFromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SCHEDULED_TASK_MAP")
}
