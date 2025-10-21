package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/aws/aws-lambda-go/events"
	"madappgang.com/infrastructure/ci_lambda/config"
	"madappgang.com/infrastructure/ci_lambda/deployer"
	"madappgang.com/infrastructure/ci_lambda/services"
	"madappgang.com/infrastructure/ci_lambda/utils"
)

// Event type definitions

// ECRImagePushEventDetail represents ECR image push events
type ECRImagePushEventDetail struct {
	RepositoryName string `json:"repository-name"`
	Tag            string `json:"image-tag"`
	Action         string `json:"action-type"`
	Result         string `json:"result"`
}

// ECSServiceDeployEvent represents ECS deployment status events
type ECSServiceDeployEvent struct {
	EventType    string `json:"eventType"` // INFO, ERROR, WARN
	EventName    string `json:"eventName"` // SERVICE_DEPLOYMENT_IN_PROGRESS, etc.
	Reason       string `json:"reason"`
	DeploymentID string `json:"deploymentId"`
}

// Known ECS event names
const (
	ECSEventNameInProgress                        = "SERVICE_DEPLOYMENT_IN_PROGRESS"
	ECSEventNameCompleted                         = "SERVICE_DEPLOYMENT_COMPLETED"
	ECSEventNameFailed                            = "SERVICE_DEPLOYMENT_FAILED"
	ECSEventNameServiceSteady                     = "SERVICE_STEADY_STATE"
	ECSEventNameServiceTaskImpaired               = "SERVICE_TASK_START_IMPAIRED"
	ECSEventNameServiceDiscoveryInstanceUnhealthy = "SERVICE_DISCOVERY_INSTANCE_UNHEALTHY"
)

// SSMEventDetail represents SSM Parameter Store change events
type SSMEventDetail struct {
	Operation   string `json:"operation"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// S3EventDetail represents S3 object change events from CloudTrail
type S3EventDetail struct {
	RequestParameters struct {
		BucketName string `json:"bucketName"`
		Key        string `json:"key"`
	} `json:"requestParameters"`
	EventName string `json:"eventName"` // PutObject, DeleteObject, etc.
}

// ManualDeployEventDetail represents a manual deployment trigger
type ManualDeployEventDetail struct {
	Service        string `json:"service"`
	TaskDefinition string `json:"task_definition,omitempty"` // Optional: specific task def to deploy
	Reason         string `json:"reason,omitempty"`          // Optional: reason for deployment
}

// EventHandlerV2 handles CloudWatch events using V2 architecture (direct resource lookups)
type EventHandlerV2 struct {
	config   *config.Config
	deployer *deployer.DeployerV2
	slackSvc *services.SlackService
	logger   *utils.Logger
}

// NewEventHandlerV2 creates a new V2 event handler
func NewEventHandlerV2(
	cfg *config.Config,
	dep *deployer.DeployerV2,
	slackSvc *services.SlackService,
	logger *utils.Logger,
) *EventHandlerV2 {
	return &EventHandlerV2{
		config:   cfg,
		deployer: dep,
		slackSvc: slackSvc,
		logger:   logger,
	}
}

// HandleEvent routes events to appropriate handlers
func (h *EventHandlerV2) HandleEvent(ctx context.Context, event events.CloudWatchEvent) (string, error) {
	log := h.logger.WithFields(map[string]interface{}{
		"event_id":     event.ID,
		"event_source": event.Source,
		"detail_type":  event.DetailType,
		"architecture": "v2",
	})

	log.Info("Received event (V2)", nil)

	// Route based on event source
	switch event.Source {
	case "aws.ecr":
		if !h.config.EnableECRMonitoring {
			log.Info("ECR monitoring disabled, skipping event", nil)
			return "ECR monitoring disabled", nil
		}
		return h.handleECREvent(ctx, event)

	case "aws.ecs":
		return h.handleECSEvent(ctx, event)

	case "aws.ssm":
		if !h.config.EnableSSMMonitoring {
			log.Info("SSM monitoring disabled, skipping event", nil)
			return "SSM monitoring disabled", nil
		}
		return h.handleSSMEvent(ctx, event)

	case "aws.s3":
		if !h.config.EnableS3Monitoring {
			log.Info("S3 monitoring disabled, skipping event", nil)
			return "S3 monitoring disabled", nil
		}
		return h.handleS3Event(ctx, event)

	case "action.production", "action.deploy":
		if !h.config.EnableManualDeploy {
			log.Info("Manual deploy disabled, skipping event", nil)
			return "Manual deploy disabled", nil
		}
		return h.handleManualDeployEvent(ctx, event)

	default:
		log.Warn("Unknown event source", map[string]interface{}{
			"source": event.Source,
		})
		return "", fmt.Errorf("unsupported event source: %s", event.Source)
	}
}

// handleECREvent processes ECR image push events (V2)
func (h *EventHandlerV2) handleECREvent(ctx context.Context, event events.CloudWatchEvent) (string, error) {
	log := h.logger.WithFields(map[string]interface{}{
		"event_type": "ecr",
		"event_id":   event.ID,
	})

	var detail ECRImagePushEventDetail
	if err := json.Unmarshal(event.Detail, &detail); err != nil {
		log.Error("Failed to unmarshal ECR event", map[string]interface{}{
			"error": err.Error(),
		})
		return "", fmt.Errorf("failed to unmarshal ECR event detail: %w", err)
	}

	log.Info("Processing ECR event", map[string]interface{}{
		"repository": detail.RepositoryName,
		"tag":        detail.Tag,
		"action":     detail.Action,
		"result":     detail.Result,
	})

	if detail.Action != "PUSH" || detail.Result != "SUCCESS" {
		log.Info("Skipping non-successful push event", map[string]interface{}{
			"action": detail.Action,
			"result": detail.Result,
		})
		return fmt.Sprintf("Skipped event: action=%s result=%s", detail.Action, detail.Result), nil
	}

	// Extract service identifier from repository name
	serviceID, err := utils.GetServiceNameFromRepoName(detail.RepositoryName, h.config.ProjectName)
	if err != nil {
		log.Warn("Unable to extract service name from repository", map[string]interface{}{
			"repository": detail.RepositoryName,
			"error":      err.Error(),
		})
		return "", fmt.Errorf("unable to extract service name from repo name %s: %w", detail.RepositoryName, err)
	}

	log.Info("Extracted service identifier from ECR repository", map[string]interface{}{
		"repository": detail.RepositoryName,
		"service_id": serviceID,
	})

	// Trigger deployment (V2 - uses direct resource lookup)
	result := h.deployer.Deploy(deployer.DeployOptions{
		ServiceIdentifier: serviceID,
		Reason:            fmt.Sprintf("New ECR image pushed: %s:%s", detail.RepositoryName, detail.Tag),
		SourceEvent:       "ECR",
	})

	if result.Error != nil {
		log.Error("Deployment failed", map[string]interface{}{
			"service_id": serviceID,
			"error":      result.Error.Error(),
		})
		return "", result.Error
	}

	log.Info("ECR-triggered deployment completed", map[string]interface{}{
		"service_id":      result.ServiceIdentifier,
		"service_name":    result.ServiceName,
		"deployment_id":   result.DeploymentID,
		"task_definition": result.TaskDefinition,
	})

	return result.Message, nil
}

// handleECSEvent processes ECS deployment status events (unchanged from V1)
func (h *EventHandlerV2) handleECSEvent(ctx context.Context, event events.CloudWatchEvent) (string, error) {
	// ECS event handling is the same in V2 (just notifications, no deployment)
	log := h.logger.WithFields(map[string]interface{}{
		"event_type": "ecs",
		"event_id":   event.ID,
	})

	if !h.config.EnableSlackNotifications {
		log.Debug("Slack notifications disabled, skipping ECS event", nil)
		return "Slack notifications disabled", nil
	}

	var detail ECSServiceDeployEvent
	if err := json.Unmarshal(event.Detail, &detail); err != nil {
		log.Error("Failed to unmarshal ECS event", map[string]interface{}{
			"error": err.Error(),
		})
		return "", fmt.Errorf("failed to unmarshal ECS event detail: %w", err)
	}

	serviceARN := ""
	if len(event.Resources) > 0 {
		serviceARN = event.Resources[0]
	}

	log.Info("Processing ECS event", map[string]interface{}{
		"event_type":    detail.EventType,
		"event_name":    detail.EventName,
		"service_arn":   serviceARN,
		"deployment_id": detail.DeploymentID,
	})

	if detail.EventName == ECSEventNameServiceSteady {
		log.Debug("Skipping SERVICE_STEADY_STATE event (too noisy)", nil)
		return "Skipped SERVICE_STEADY_STATE event", nil
	}

	var notificationType services.NotificationType
	switch detail.EventName {
	case ECSEventNameCompleted:
		notificationType = services.NotificationSuccess
	case ECSEventNameFailed, ECSEventNameServiceTaskImpaired:
		notificationType = services.NotificationError
	default:
		notificationType = services.NotificationInfo
	}

	err := h.slackSvc.SendNotification(services.NotificationData{
		Type:         notificationType,
		Service:      serviceARN,
		StateName:    detail.EventName,
		DeploymentID: detail.DeploymentID,
		Reason:       detail.Reason,
	})

	if err != nil {
		log.Error("Failed to send Slack notification", map[string]interface{}{
			"error": err.Error(),
		})
		return "", fmt.Errorf("failed to send Slack notification: %w", err)
	}

	log.Info("ECS event notification sent", map[string]interface{}{
		"event_name":        detail.EventName,
		"notification_type": notificationType,
	})

	return fmt.Sprintf("Sent Slack notification for %s event", detail.EventName), nil
}

// handleSSMEvent processes SSM parameter change events (V2)
func (h *EventHandlerV2) handleSSMEvent(ctx context.Context, event events.CloudWatchEvent) (string, error) {
	log := h.logger.WithFields(map[string]interface{}{
		"event_type": "ssm",
		"event_id":   event.ID,
	})

	var detail SSMEventDetail
	if err := json.Unmarshal(event.Detail, &detail); err != nil {
		log.Error("Failed to unmarshal SSM event", map[string]interface{}{
			"error": err.Error(),
		})
		return "", fmt.Errorf("failed to unmarshal SSM event detail: %w", err)
	}

	log.Info("Processing SSM event", map[string]interface{}{
		"operation":  detail.Operation,
		"parameter":  detail.Name,
		"param_type": detail.Type,
	})

	// Extract service identifier from parameter path
	serviceID, err := h.extractServiceFromSSMPath(detail.Name)
	if err != nil {
		log.Warn("Unable to extract service from SSM parameter path", map[string]interface{}{
			"parameter": detail.Name,
			"error":     err.Error(),
		})
		return fmt.Sprintf("SSM parameter %s does not match expected pattern, skipping", detail.Name), nil
	}

	log.Info("Extracted service from SSM parameter", map[string]interface{}{
		"parameter":  detail.Name,
		"service_id": serviceID,
	})

	// Trigger deployment (V2)
	result := h.deployer.Deploy(deployer.DeployOptions{
		ServiceIdentifier: serviceID,
		Reason:            fmt.Sprintf("SSM parameter changed: %s (%s)", detail.Name, detail.Operation),
		SourceEvent:       "SSM",
	})

	if result.Error != nil {
		log.Error("Deployment failed", map[string]interface{}{
			"service_id": serviceID,
			"error":      result.Error.Error(),
		})
		return "", result.Error
	}

	log.Info("SSM-triggered deployment completed", map[string]interface{}{
		"service_id":      result.ServiceIdentifier,
		"service_name":    result.ServiceName,
		"deployment_id":   result.DeploymentID,
		"task_definition": result.TaskDefinition,
	})

	return result.Message, nil
}

// handleS3Event processes S3 object change events (V2)
func (h *EventHandlerV2) handleS3Event(ctx context.Context, event events.CloudWatchEvent) (string, error) {
	log := h.logger.WithFields(map[string]interface{}{
		"event_type": "s3",
		"event_id":   event.ID,
	})

	var detail S3EventDetail
	if err := json.Unmarshal(event.Detail, &detail); err != nil {
		log.Error("Failed to unmarshal S3 event", map[string]interface{}{
			"error": err.Error(),
		})
		return "", fmt.Errorf("failed to unmarshal S3 event detail: %w", err)
	}

	bucketName := detail.RequestParameters.BucketName
	objectKey := detail.RequestParameters.Key

	log.Info("Processing S3 event", map[string]interface{}{
		"bucket":     bucketName,
		"key":        objectKey,
		"event_name": detail.EventName,
	})

	// Use V2 config to find services affected by this S3 file
	affectedServices := h.config.GetServicesForS3File(bucketName, objectKey)

	if len(affectedServices) == 0 {
		log.Warn("No services found for S3 file", map[string]interface{}{
			"bucket": bucketName,
			"key":    objectKey,
		})
		return fmt.Sprintf("No service configured for s3://%s/%s", bucketName, objectKey), nil
	}

	log.Info("Found services using S3 env file", map[string]interface{}{
		"bucket":   bucketName,
		"key":      objectKey,
		"services": affectedServices,
		"count":    len(affectedServices),
	})

	// Build deployment options
	deployOpts := make([]deployer.DeployOptions, len(affectedServices))
	for i, serviceID := range affectedServices {
		deployOpts[i] = deployer.DeployOptions{
			ServiceIdentifier: serviceID,
			Reason:            fmt.Sprintf("S3 env file changed: s3://%s/%s", bucketName, objectKey),
			SourceEvent:       "S3",
		}
	}

	// Deploy all affected services (V2)
	results := h.deployer.DeployMultiple(deployOpts)

	// Aggregate results
	successCount := 0
	var errors []string
	for _, result := range results {
		if result.Success {
			successCount++
		} else {
			errors = append(errors, fmt.Sprintf("%s: %v", result.ServiceName, result.Error))
		}
	}

	if len(errors) > 0 {
		log.Error("Some deployments failed", map[string]interface{}{
			"total":      len(results),
			"successful": successCount,
			"failed":     len(errors),
			"errors":     errors,
		})
		return "", fmt.Errorf("some deployments failed: %v", errors)
	}

	log.Info("S3-triggered deployments completed", map[string]interface{}{
		"services_deployed": len(results),
		"successful":        successCount,
	})

	return fmt.Sprintf("Successfully deployed %d services for S3 file change", successCount), nil
}

// handleManualDeployEvent processes manual deployment events (V2)
func (h *EventHandlerV2) handleManualDeployEvent(ctx context.Context, event events.CloudWatchEvent) (string, error) {
	log := h.logger.WithFields(map[string]interface{}{
		"event_type": "manual_deploy",
		"event_id":   event.ID,
		"source":     event.Source,
	})

	var detail ManualDeployEventDetail
	if err := json.Unmarshal(event.Detail, &detail); err != nil {
		log.Error("Failed to unmarshal manual deploy event", map[string]interface{}{
			"error": err.Error(),
		})
		return "", fmt.Errorf("failed to unmarshal deploy event detail: %w", err)
	}

	if detail.Service == "" {
		log.Error("Manual deploy event missing service field", nil)
		return "", fmt.Errorf("manual deploy event must include 'service' field")
	}

	log.Info("Processing manual deploy event", map[string]interface{}{
		"service_id":      detail.Service,
		"task_definition": detail.TaskDefinition,
		"reason":          detail.Reason,
	})

	reason := detail.Reason
	if reason == "" {
		reason = "Manual deployment triggered"
	}

	// Trigger deployment (V2)
	result := h.deployer.Deploy(deployer.DeployOptions{
		ServiceIdentifier: detail.Service,
		TaskDefinition:    detail.TaskDefinition,
		Reason:            reason,
		SourceEvent:       "MANUAL",
	})

	if result.Error != nil {
		log.Error("Manual deployment failed", map[string]interface{}{
			"service_id": detail.Service,
			"error":      result.Error.Error(),
		})
		return "", result.Error
	}

	log.Info("Manual deployment completed", map[string]interface{}{
		"service_id":      result.ServiceIdentifier,
		"service_name":    result.ServiceName,
		"deployment_id":   result.DeploymentID,
		"task_definition": result.TaskDefinition,
	})

	return result.Message, nil
}

// extractServiceFromSSMPath extracts service identifier from SSM parameter path
func (h *EventHandlerV2) extractServiceFromSSMPath(parameterPath string) (string, error) {
	pattern := fmt.Sprintf(`\/?%s\/%s\/(\w+)\/\w+$`,
		regexp.QuoteMeta(h.config.Environment),
		regexp.QuoteMeta(h.config.ProjectName))

	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(parameterPath)

	if len(match) != 2 {
		return "", fmt.Errorf("parameter path does not match pattern /{env}/{project}/{service}/param")
	}

	serviceName := match[1]

	// Handle backend service special case
	if serviceName == "backend" {
		return "", nil // Empty string represents backend service
	}

	return serviceName, nil
}
