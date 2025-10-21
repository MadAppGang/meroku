package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"madappgang.com/infrastructure/ci_lambda/services"
)

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

// handleECSEvent processes ECS deployment status events
func (h *EventHandler) handleECSEvent(ctx context.Context, event events.CloudWatchEvent) (string, error) {
	log := h.logger.WithFields(map[string]interface{}{
		"event_type": "ecs",
		"event_id":   event.ID,
	})

	// Skip if Slack notifications are disabled
	if !h.config.EnableSlackNotifications {
		log.Debug("Slack notifications disabled, skipping ECS event", nil)
		return "Slack notifications disabled", nil
	}

	// Parse event detail
	var detail ECSServiceDeployEvent
	if err := json.Unmarshal(event.Detail, &detail); err != nil {
		log.Error("Failed to unmarshal ECS event", map[string]interface{}{
			"error": err.Error(),
		})
		return "", fmt.Errorf("failed to unmarshal ECS event detail: %w", err)
	}

	// Extract service ARN from resources
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

	// Filter noisy events
	if detail.EventName == ECSEventNameServiceSteady {
		log.Debug("Skipping SERVICE_STEADY_STATE event (too noisy)", nil)
		return "Skipped SERVICE_STEADY_STATE event", nil
	}

	// Determine notification type
	var notificationType services.NotificationType
	switch detail.EventName {
	case ECSEventNameCompleted:
		notificationType = services.NotificationSuccess
	case ECSEventNameFailed, ECSEventNameServiceTaskImpaired:
		notificationType = services.NotificationError
	default:
		notificationType = services.NotificationInfo
	}

	// Send notification
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
		"event_name":      detail.EventName,
		"notification_type": notificationType,
	})

	return fmt.Sprintf("Sent Slack notification for %s event", detail.EventName), nil
}
