package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"madappgang.com/infrastructure/ci_lambda/config"
	"madappgang.com/infrastructure/ci_lambda/deployer"
	"madappgang.com/infrastructure/ci_lambda/services"
	"madappgang.com/infrastructure/ci_lambda/utils"
)

// --- Mocks ---

type mockDeployer struct {
	deployCalls         []deployer.DeployOptions
	deployMultipleCalls [][]deployer.DeployOptions
	deployResult        *deployer.DeployResult
	deployMultiResults  []*deployer.DeployResult
}

func (m *mockDeployer) Deploy(opts deployer.DeployOptions) *deployer.DeployResult {
	m.deployCalls = append(m.deployCalls, opts)
	if m.deployResult != nil {
		return m.deployResult
	}
	return &deployer.DeployResult{
		Success:           true,
		ServiceIdentifier: opts.ServiceIdentifier,
		ServiceName:       "mock_service",
		Message:           "mock deployment succeeded",
	}
}

func (m *mockDeployer) DeployMultiple(opts []deployer.DeployOptions) []*deployer.DeployResult {
	m.deployMultipleCalls = append(m.deployMultipleCalls, opts)
	if m.deployMultiResults != nil {
		return m.deployMultiResults
	}
	results := make([]*deployer.DeployResult, len(opts))
	for i, o := range opts {
		results[i] = &deployer.DeployResult{
			Success:           true,
			ServiceIdentifier: o.ServiceIdentifier,
			ServiceName:       "mock_service",
			Message:           "mock deployment succeeded",
		}
	}
	return results
}

type mockSlack struct {
	notifications []services.NotificationData
}

func (m *mockSlack) SendNotification(data services.NotificationData) error {
	m.notifications = append(m.notifications, data)
	return nil
}

// --- Helpers ---

func testConfig() *config.Config {
	return &config.Config{
		ProjectName:              "myapp",
		Environment:              "dev",
		AWSRegion:                "us-east-1",
		ClusterName:              "myapp_cluster_dev",
		LogLevel:                 config.LogLevelInfo,
		DeploymentTimeoutSeconds: 60,
		MaxDeploymentRetries:     0,
		EnableECRMonitoring:      true,
		EnableSSMMonitoring:      true,
		EnableS3Monitoring:       true,
		EnableManualDeploy:       true,
		ServiceMap: map[string]config.ServiceMapping{
			"backend": {
				ServiceName: "myapp_service_dev",
				TaskFamily:  "myapp_backend_dev",
				Type:        config.ServiceMappingTypeService,
			},
			"api": {
				ServiceName: "myapp_service_api_dev",
				TaskFamily:  "myapp_service_api_dev",
				Type:        config.ServiceMappingTypeService,
			},
			"task:cleanup": {
				TaskFamily: "myapp_task_cleanup_dev",
				Type:       config.ServiceMappingTypeScheduledTask,
			},
		},
		S3ToServiceMap: map[string][]config.S3ServiceFile{
			"api": {
				{Bucket: "myapp-env-dev", Key: "api/.env"},
			},
		},
	}
}

func buildHandler(cfg *config.Config, dep *mockDeployer, slack *mockSlack) *EventHandlerV2 {
	logger := utils.NewLogger(cfg)
	return NewEventHandlerV2(cfg, dep, slack, logger)
}

func makeEvent(source, detailType string, detail interface{}) events.CloudWatchEvent {
	detailJSON, _ := json.Marshal(detail)
	return events.CloudWatchEvent{
		ID:         "test-event-id",
		Source:     source,
		DetailType: detailType,
		Detail:     detailJSON,
		Region:     "us-east-1",
		AccountID:  "123456789012",
	}
}

// --- ECR Event Tests ---

func TestHandleECREvent_SuccessfulPush(t *testing.T) {
	cfg := testConfig()
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("aws.ecr", "ECR Image Action", ECRImagePushEventDetail{
		RepositoryName: "myapp_service_api",
		Tag:            "latest",
		Action:         "PUSH",
		Result:         "SUCCESS",
	})

	result, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	assert.NotEmpty(t, result)
	require.Len(t, dep.deployCalls, 1)
	assert.Equal(t, "api", dep.deployCalls[0].ServiceIdentifier)
	assert.Equal(t, "ECR", dep.deployCalls[0].SourceEvent)
	assert.Contains(t, dep.deployCalls[0].Reason, "myapp_service_api:latest")
}

func TestHandleECREvent_BackendPush(t *testing.T) {
	cfg := testConfig()
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("aws.ecr", "ECR Image Action", ECRImagePushEventDetail{
		RepositoryName: "myapp_backend",
		Tag:            "v1.0",
		Action:         "PUSH",
		Result:         "SUCCESS",
	})

	_, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	require.Len(t, dep.deployCalls, 1)
	// Empty string is the backend service identifier
	assert.Equal(t, "", dep.deployCalls[0].ServiceIdentifier)
}

func TestHandleECREvent_ScheduledTaskPush(t *testing.T) {
	cfg := testConfig()
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("aws.ecr", "ECR Image Action", ECRImagePushEventDetail{
		RepositoryName: "myapp_task_cleanup",
		Tag:            "abc123",
		Action:         "PUSH",
		Result:         "SUCCESS",
	})

	_, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	require.Len(t, dep.deployCalls, 1)
	assert.Equal(t, "task:cleanup", dep.deployCalls[0].ServiceIdentifier)
	// For scheduled tasks, TaskDefinition should contain the full image URI
	assert.Contains(t, dep.deployCalls[0].TaskDefinition, "myapp_task_cleanup:abc123")
}

func TestHandleECREvent_NonPushAction_Skipped(t *testing.T) {
	cfg := testConfig()
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("aws.ecr", "ECR Image Action", ECRImagePushEventDetail{
		RepositoryName: "myapp_service_api",
		Tag:            "latest",
		Action:         "PULL",
		Result:         "SUCCESS",
	})

	result, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	assert.Contains(t, result, "Skipped")
	assert.Empty(t, dep.deployCalls, "Deploy should not be called for non-PUSH events")
}

func TestHandleECREvent_FailedPush_Skipped(t *testing.T) {
	cfg := testConfig()
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("aws.ecr", "ECR Image Action", ECRImagePushEventDetail{
		RepositoryName: "myapp_service_api",
		Tag:            "latest",
		Action:         "PUSH",
		Result:         "FAILURE",
	})

	result, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	assert.Contains(t, result, "Skipped")
	assert.Empty(t, dep.deployCalls)
}

func TestHandleECREvent_MonitoringDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.EnableECRMonitoring = false
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("aws.ecr", "ECR Image Action", ECRImagePushEventDetail{
		RepositoryName: "myapp_service_api",
		Tag:            "latest",
		Action:         "PUSH",
		Result:         "SUCCESS",
	})

	result, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	assert.Contains(t, result, "ECR monitoring disabled")
	assert.Empty(t, dep.deployCalls)
}

// --- ECS Event Tests ---

func TestHandleECSEvent_SlackDisabled_Skipped(t *testing.T) {
	cfg := testConfig()
	cfg.SlackWebhookURL = "" // No Slack
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("aws.ecs", "ECS Deployment State Change", ECSServiceDeployEvent{
		EventType:    "INFO",
		EventName:    ECSEventNameCompleted,
		DeploymentID: "ecs-svc/123",
	})

	result, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	assert.Contains(t, result, "Slack notifications disabled")
	assert.Empty(t, slack.notifications)
}

func TestHandleECSEvent_SteadyState_Skipped(t *testing.T) {
	cfg := testConfig()
	cfg.SlackWebhookURL = "https://hooks.slack.com/test"
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("aws.ecs", "ECS Deployment State Change", ECSServiceDeployEvent{
		EventType: "INFO",
		EventName: ECSEventNameServiceSteady,
	})

	result, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	assert.Contains(t, result, "Skipped SERVICE_STEADY_STATE")
	assert.Empty(t, slack.notifications, "No notification for steady state")
}

func TestHandleECSEvent_Completed_SendsSuccessNotification(t *testing.T) {
	cfg := testConfig()
	cfg.SlackWebhookURL = "https://hooks.slack.com/test"
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("aws.ecs", "ECS Deployment State Change", ECSServiceDeployEvent{
		EventType:    "INFO",
		EventName:    ECSEventNameCompleted,
		DeploymentID: "ecs-svc/123",
	})
	event.Resources = []string{"arn:aws:ecs:us-east-1:123:service/myapp_service_api_dev"}

	_, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	require.Len(t, slack.notifications, 1)
	assert.Equal(t, services.NotificationSuccess, slack.notifications[0].Type)
}

func TestHandleECSEvent_Failed_SendsErrorNotification(t *testing.T) {
	cfg := testConfig()
	cfg.SlackWebhookURL = "https://hooks.slack.com/test"
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("aws.ecs", "ECS Deployment State Change", ECSServiceDeployEvent{
		EventType:    "ERROR",
		EventName:    ECSEventNameFailed,
		Reason:       "Task failed health check",
		DeploymentID: "ecs-svc/456",
	})

	_, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	require.Len(t, slack.notifications, 1)
	assert.Equal(t, services.NotificationError, slack.notifications[0].Type)
}

// --- SSM Event Tests ---

func TestHandleSSMEvent_ValidPath(t *testing.T) {
	cfg := testConfig()
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("aws.ssm", "Parameter Store Change", SSMEventDetail{
		Operation: "Update",
		Name:      "/dev/myapp/api/DB_HOST",
		Type:      "String",
	})

	_, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	require.Len(t, dep.deployCalls, 1)
	assert.Equal(t, "api", dep.deployCalls[0].ServiceIdentifier)
	assert.Equal(t, "SSM", dep.deployCalls[0].SourceEvent)
}

func TestHandleSSMEvent_BackendPath(t *testing.T) {
	cfg := testConfig()
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("aws.ssm", "Parameter Store Change", SSMEventDetail{
		Operation: "Update",
		Name:      "/dev/myapp/backend/SECRET_KEY",
		Type:      "SecureString",
	})

	_, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	require.Len(t, dep.deployCalls, 1)
	// Backend returns empty string as service identifier
	assert.Equal(t, "", dep.deployCalls[0].ServiceIdentifier)
}

func TestHandleSSMEvent_UnmatchedPath_Skipped(t *testing.T) {
	cfg := testConfig()
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("aws.ssm", "Parameter Store Change", SSMEventDetail{
		Operation: "Update",
		Name:      "/some/other/path",
		Type:      "String",
	})

	result, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	assert.Contains(t, result, "does not match expected pattern")
	assert.Empty(t, dep.deployCalls)
}

func TestHandleSSMEvent_MonitoringDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.EnableSSMMonitoring = false
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("aws.ssm", "Parameter Store Change", SSMEventDetail{
		Operation: "Update",
		Name:      "/dev/myapp/api/DB_HOST",
	})

	result, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	assert.Contains(t, result, "SSM monitoring disabled")
	assert.Empty(t, dep.deployCalls)
}

// --- S3 Event Tests ---

func TestHandleS3Event_MatchingFile(t *testing.T) {
	cfg := testConfig()
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("aws.s3", "AWS API Call via CloudTrail", S3EventDetail{
		EventName: "PutObject",
		RequestParameters: struct {
			BucketName string `json:"bucketName"`
			Key        string `json:"key"`
		}{
			BucketName: "myapp-env-dev",
			Key:        "api/.env",
		},
	})

	_, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	require.Len(t, dep.deployMultipleCalls, 1)
	assert.Len(t, dep.deployMultipleCalls[0], 1)
	assert.Equal(t, "api", dep.deployMultipleCalls[0][0].ServiceIdentifier)
	assert.Equal(t, "S3", dep.deployMultipleCalls[0][0].SourceEvent)
}

func TestHandleS3Event_NoMatchingService(t *testing.T) {
	cfg := testConfig()
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("aws.s3", "AWS API Call via CloudTrail", S3EventDetail{
		EventName: "PutObject",
		RequestParameters: struct {
			BucketName string `json:"bucketName"`
			Key        string `json:"key"`
		}{
			BucketName: "unknown-bucket",
			Key:        "unknown/.env",
		},
	})

	result, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	assert.Contains(t, result, "No service configured")
	assert.Empty(t, dep.deployMultipleCalls)
}

func TestHandleS3Event_MonitoringDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.EnableS3Monitoring = false
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("aws.s3", "AWS API Call via CloudTrail", S3EventDetail{
		EventName: "PutObject",
		RequestParameters: struct {
			BucketName string `json:"bucketName"`
			Key        string `json:"key"`
		}{
			BucketName: "myapp-env-dev",
			Key:        "api/.env",
		},
	})

	result, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	assert.Contains(t, result, "S3 monitoring disabled")
	assert.Empty(t, dep.deployMultipleCalls)
}

// --- Manual Deploy Event Tests ---

func TestHandleManualDeploy_ActionProduction(t *testing.T) {
	cfg := testConfig()
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("action.production", "DEPLOY", ManualDeployEventDetail{
		Service: "api",
		Reason:  "Production release v2.0",
	})

	_, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	require.Len(t, dep.deployCalls, 1)
	assert.Equal(t, "api", dep.deployCalls[0].ServiceIdentifier)
	assert.Equal(t, "MANUAL", dep.deployCalls[0].SourceEvent)
	assert.Equal(t, "Production release v2.0", dep.deployCalls[0].Reason)
}

func TestHandleManualDeploy_ActionDeploy(t *testing.T) {
	cfg := testConfig()
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("action.deploy", "DEPLOY", ManualDeployEventDetail{
		Service: "api",
	})

	_, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	require.Len(t, dep.deployCalls, 1)
	assert.Equal(t, "api", dep.deployCalls[0].ServiceIdentifier)
	assert.Equal(t, "MANUAL", dep.deployCalls[0].SourceEvent)
	// Default reason when none provided
	assert.Equal(t, "Manual deployment triggered", dep.deployCalls[0].Reason)
}

func TestHandleManualDeploy_MissingService_Error(t *testing.T) {
	cfg := testConfig()
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("action.production", "DEPLOY", ManualDeployEventDetail{
		Service: "", // Missing
	})

	_, err := h.HandleEvent(context.Background(), event)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "service")
	assert.Empty(t, dep.deployCalls)
}

func TestHandleManualDeploy_WithTaskDefinition(t *testing.T) {
	cfg := testConfig()
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("action.production", "DEPLOY", ManualDeployEventDetail{
		Service:        "api",
		TaskDefinition: "arn:aws:ecs:us-east-1:123:task-definition/myapp_api:42",
	})

	_, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	require.Len(t, dep.deployCalls, 1)
	assert.Equal(t, "arn:aws:ecs:us-east-1:123:task-definition/myapp_api:42", dep.deployCalls[0].TaskDefinition)
}

func TestHandleManualDeploy_Disabled(t *testing.T) {
	cfg := testConfig()
	cfg.EnableManualDeploy = false
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("action.production", "DEPLOY", ManualDeployEventDetail{
		Service: "api",
	})

	result, err := h.HandleEvent(context.Background(), event)

	require.NoError(t, err)
	assert.Contains(t, result, "Manual deploy disabled")
	assert.Empty(t, dep.deployCalls)
}

// --- Unknown Source ---

func TestHandleEvent_UnknownSource_Error(t *testing.T) {
	cfg := testConfig()
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	event := makeEvent("aws.unknown", "Unknown Event", map[string]string{})

	_, err := h.HandleEvent(context.Background(), event)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported event source")
}

// --- SSM Path Extraction (unit test for private method) ---

func TestExtractServiceFromSSMPath(t *testing.T) {
	cfg := testConfig()
	dep := &mockDeployer{}
	slack := &mockSlack{}
	h := buildHandler(cfg, dep, slack)

	tests := []struct {
		name        string
		path        string
		wantService string
		wantErr     bool
	}{
		{
			name:        "named service",
			path:        "/dev/myapp/api/DB_HOST",
			wantService: "api",
		},
		{
			name:        "backend service",
			path:        "/dev/myapp/backend/SECRET_KEY",
			wantService: "", // empty = backend
		},
		{
			name:        "without leading slash",
			path:        "dev/myapp/worker/REDIS_URL",
			wantService: "worker",
		},
		{
			name:    "unmatched path",
			path:    "/other/path/value",
			wantErr: true,
		},
		{
			name:    "too short",
			path:    "/dev",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, err := h.extractServiceFromSSMPath(tt.path)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantService, service)
			}
		})
	}
}
