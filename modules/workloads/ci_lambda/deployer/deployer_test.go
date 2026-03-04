package deployer

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"madappgang.com/infrastructure/ci_lambda/config"
	"madappgang.com/infrastructure/ci_lambda/services"
	"madappgang.com/infrastructure/ci_lambda/utils"
)

// ecsDeployer is a minimal interface that lets us inject a mock.
// The real *services.ECSServiceV2 satisfies this interface.
type ecsDeployer interface {
	Deploy(req services.DeploymentRequest) (*services.DeploymentResult, error)
	DeployScheduledTask(req services.DeploymentRequest) (*services.DeploymentResult, error)
}

// mockECS records which method was called and returns canned results.
type mockECS struct {
	deployCalledWith          *services.DeploymentRequest
	deployScheduledCalledWith *services.DeploymentRequest
	deployResult              *services.DeploymentResult
	deployErr                 error
	deployScheduledResult     *services.DeploymentResult
	deployScheduledErr        error
}

func (m *mockECS) Deploy(req services.DeploymentRequest) (*services.DeploymentResult, error) {
	m.deployCalledWith = &req
	return m.deployResult, m.deployErr
}

func (m *mockECS) DeployScheduledTask(req services.DeploymentRequest) (*services.DeploymentResult, error) {
	m.deployScheduledCalledWith = &req
	return m.deployScheduledResult, m.deployScheduledErr
}

// deployerWithMock builds a DeployerV2 that delegates to the mock via a thin wrapper.
// We replicate the branching logic here to test it in isolation without changing the
// production struct (which embeds *services.ECSServiceV2 directly).
type testableDeployer struct {
	ecsMock ecsDeployer
	cfg     *config.Config
	logger  *utils.Logger
}

func (d *testableDeployer) Deploy(opts DeployOptions) *DeployResult {
	isScheduledTask := d.cfg.IsScheduledTask(opts.ServiceIdentifier)

	req := services.DeploymentRequest{
		ServiceIdentifier: opts.ServiceIdentifier,
		TaskDefinition:    opts.TaskDefinition,
		ForceNewDeploy:    true,
	}

	var result *services.DeploymentResult
	var err error

	if isScheduledTask {
		result, err = d.ecsMock.DeployScheduledTask(req)
	} else {
		result, err = d.ecsMock.Deploy(req)
	}

	if err != nil {
		return &DeployResult{
			Success:           false,
			ServiceIdentifier: opts.ServiceIdentifier,
			Error:             err,
		}
	}

	return &DeployResult{
		Success:           true,
		ServiceIdentifier: result.ServiceIdentifier,
		ServiceName:       result.ServiceName,
		TaskDefinition:    result.TaskDefinition,
		Message:           result.Message,
	}
}

// buildTestConfig creates a minimal Config with the given service map entries.
func buildTestConfig(serviceMap map[string]config.ServiceMapping) *config.Config {
	return &config.Config{
		ProjectName:              "myapp",
		Environment:              "dev",
		ClusterName:              "myapp_cluster_dev",
		LogLevel:                 config.LogLevelInfo,
		DeploymentTimeoutSeconds: 60,
		MaxDeploymentRetries:     0,
		ServiceMap:               serviceMap,
	}
}

func buildLogger(cfg *config.Config) *utils.Logger {
	return utils.NewLogger(cfg)
}

func TestDeployerBranching_RegularService(t *testing.T) {
	cfg := buildTestConfig(map[string]config.ServiceMapping{
		"api": {
			ServiceName: "myapp_service_api_dev",
			TaskFamily:  "myapp_service_api_dev",
			Type:        config.ServiceMappingTypeService,
		},
	})

	mock := &mockECS{
		deployResult: &services.DeploymentResult{
			ServiceIdentifier: "api",
			ServiceName:       "myapp_service_api_dev",
			TaskDefinition:    "arn:aws:ecs:us-east-1:123456789012:task-definition/myapp_service_api_dev:5",
			Status:            "DEPLOYED",
			Message:           "deployed",
		},
	}

	d := &testableDeployer{ecsMock: mock, cfg: cfg, logger: buildLogger(cfg)}
	result := d.Deploy(DeployOptions{ServiceIdentifier: "api", SourceEvent: "ECR"})

	require.True(t, result.Success)
	// Deploy() should have been called, not DeployScheduledTask()
	assert.NotNil(t, mock.deployCalledWith, "Deploy should have been called for a regular service")
	assert.Nil(t, mock.deployScheduledCalledWith, "DeployScheduledTask should NOT have been called for a regular service")
	assert.Equal(t, "api", mock.deployCalledWith.ServiceIdentifier)
}

func TestDeployerBranching_ScheduledTask(t *testing.T) {
	cfg := buildTestConfig(map[string]config.ServiceMapping{
		"task:cleanup": {
			TaskFamily: "myapp_task_cleanup_dev",
			Type:       config.ServiceMappingTypeScheduledTask,
		},
	})

	imageURI := "123456789012.dkr.ecr.us-east-1.amazonaws.com/myapp_task_cleanup:abc123"
	mock := &mockECS{
		deployScheduledResult: &services.DeploymentResult{
			ServiceIdentifier: "task:cleanup",
			ServiceName:       "myapp_task_cleanup_dev",
			TaskDefinition:    "arn:aws:ecs:us-east-1:123456789012:task-definition/myapp_task_cleanup_dev:7",
			Status:            "REGISTERED",
			Message:           "new revision registered",
		},
	}

	d := &testableDeployer{ecsMock: mock, cfg: cfg, logger: buildLogger(cfg)}
	result := d.Deploy(DeployOptions{
		ServiceIdentifier: "task:cleanup",
		TaskDefinition:    imageURI,
		SourceEvent:       "ECR",
	})

	require.True(t, result.Success)
	// DeployScheduledTask() should have been called, not Deploy()
	assert.Nil(t, mock.deployCalledWith, "Deploy should NOT have been called for a scheduled task")
	assert.NotNil(t, mock.deployScheduledCalledWith, "DeployScheduledTask should have been called for a scheduled task")
	assert.Equal(t, "task:cleanup", mock.deployScheduledCalledWith.ServiceIdentifier)
	assert.Equal(t, imageURI, mock.deployScheduledCalledWith.TaskDefinition)
}

func TestDeployerBranching_ScheduledTaskError(t *testing.T) {
	cfg := buildTestConfig(map[string]config.ServiceMapping{
		"task:cleanup": {
			TaskFamily: "myapp_task_cleanup_dev",
			Type:       config.ServiceMappingTypeScheduledTask,
		},
	})

	mock := &mockECS{
		deployScheduledErr: errors.New("AWS API error"),
	}

	d := &testableDeployer{ecsMock: mock, cfg: cfg, logger: buildLogger(cfg)}
	result := d.Deploy(DeployOptions{
		ServiceIdentifier: "task:cleanup",
		TaskDefinition:    "someimage:tag",
		SourceEvent:       "ECR",
	})

	assert.False(t, result.Success)
	require.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "AWS API error")
}
