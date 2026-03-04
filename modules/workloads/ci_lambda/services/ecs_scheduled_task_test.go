package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"madappgang.com/infrastructure/ci_lambda/config"
	"madappgang.com/infrastructure/ci_lambda/utils"
)

// buildTestECSService creates an ECSServiceV2 with a real (but uncalled) AWS client.
// Tests that exercise dry-run mode do not make network calls.
func buildTestECSService(t *testing.T, cfg *config.Config) *ECSServiceV2 {
	t.Helper()
	logger := utils.NewLogger(cfg)
	svc, err := NewECSServiceV2(cfg, logger)
	require.NoError(t, err)
	return svc
}

func minimalCfgWithScheduledTask() *config.Config {
	return &config.Config{
		ProjectName:              "myapp",
		Environment:              "dev",
		AWSRegion:                "us-east-1",
		ClusterName:              "myapp_cluster_dev",
		LogLevel:                 config.LogLevelInfo,
		DeploymentTimeoutSeconds: 60,
		MaxDeploymentRetries:     0,
		DryRun:                   true, // avoid real AWS calls
		ServiceMap: map[string]config.ServiceMapping{
			"task:cleanup": {
				TaskFamily: "myapp_task_cleanup_dev",
				Type:       config.ServiceMappingTypeScheduledTask,
			},
		},
	}
}

// TestDeployScheduledTask_DryRun verifies that in dry-run mode the method returns without
// error and reports a DRY_RUN task definition ARN.
func TestDeployScheduledTask_DryRun(t *testing.T) {
	cfg := minimalCfgWithScheduledTask()
	svc := buildTestECSService(t, cfg)

	imageURI := "123456789012.dkr.ecr.us-east-1.amazonaws.com/myapp_task_cleanup:abc123"

	result, err := svc.DeployScheduledTask(DeploymentRequest{
		ServiceIdentifier: "task:cleanup",
		TaskDefinition:    imageURI,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "task:cleanup", result.ServiceIdentifier)
	assert.Equal(t, "REGISTERED", result.Status)
	// In dry-run mode the ARN is "family:DRY_RUN"
	assert.Contains(t, result.TaskDefinition, "DRY_RUN")
}

// TestDeployScheduledTask_MissingMapping verifies an error is returned when the
// service identifier is not in the service map.
func TestDeployScheduledTask_MissingMapping(t *testing.T) {
	cfg := minimalCfgWithScheduledTask()
	svc := buildTestECSService(t, cfg)

	_, err := svc.DeployScheduledTask(DeploymentRequest{
		ServiceIdentifier: "task:nonexistent",
		TaskDefinition:    "some-image:tag",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "scheduled task not found")
}

// TestRegisterNewTaskDefinitionRevision_DryRun verifies that dry-run mode returns
// the expected placeholder ARN without calling AWS.
func TestRegisterNewTaskDefinitionRevision_DryRun(t *testing.T) {
	cfg := minimalCfgWithScheduledTask()
	svc := buildTestECSService(t, cfg)

	arn, err := svc.RegisterNewTaskDefinitionRevision(
		"myapp_task_cleanup_dev",
		"123456789012.dkr.ecr.us-east-1.amazonaws.com/myapp_task_cleanup:newtag",
	)

	require.NoError(t, err)
	assert.Equal(t, "myapp_task_cleanup_dev:DRY_RUN", arn)
}
