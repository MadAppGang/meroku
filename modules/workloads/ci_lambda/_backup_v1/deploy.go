package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"madappgang.com/infrastructure/ci_lambda/deployer"
)

// ManualDeployEventDetail represents a manual deployment trigger
type ManualDeployEventDetail struct {
	Service        string `json:"service"`
	TaskDefinition string `json:"task_definition,omitempty"` // Optional: specific task def to deploy
	Reason         string `json:"reason,omitempty"`          // Optional: reason for deployment
}

// handleManualDeployEvent processes manual deployment events
// Triggered via EventBridge with custom event source "action.production" or "action.deploy"
//
// Example CLI command:
// aws events put-events --entries \
//   'Source=action.production,DetailType=DEPLOY,Detail="{\"service\":\"api\",\"reason\":\"Hotfix deployment\"}",EventBusName=default'
func (h *EventHandler) handleManualDeployEvent(ctx context.Context, event events.CloudWatchEvent) (string, error) {
	log := h.logger.WithFields(map[string]interface{}{
		"event_type": "manual_deploy",
		"event_id":   event.ID,
		"source":     event.Source,
	})

	// Parse event detail
	var detail ManualDeployEventDetail
	if err := json.Unmarshal(event.Detail, &detail); err != nil {
		log.Error("Failed to unmarshal manual deploy event", map[string]interface{}{
			"error": err.Error(),
		})
		return "", fmt.Errorf("failed to unmarshal deploy event detail: %w", err)
	}

	// Validate required fields
	if detail.Service == "" {
		log.Error("Manual deploy event missing service field", nil)
		return "", fmt.Errorf("manual deploy event must include 'service' field")
	}

	log.Info("Processing manual deploy event", map[string]interface{}{
		"service":         detail.Service,
		"task_definition": detail.TaskDefinition,
		"reason":          detail.Reason,
	})

	// Set default reason if not provided
	reason := detail.Reason
	if reason == "" {
		reason = "Manual deployment triggered"
	}

	// Trigger deployment
	result := h.deployer.Deploy(deployer.DeployOptions{
		ServiceName:    detail.Service,
		TaskDefinition: detail.TaskDefinition, // May be empty - will use latest
		Reason:         reason,
		SourceEvent:    "MANUAL",
	})

	if result.Error != nil {
		log.Error("Manual deployment failed", map[string]interface{}{
			"service": detail.Service,
			"error":   result.Error.Error(),
		})
		return "", result.Error
	}

	log.Info("Manual deployment completed", map[string]interface{}{
		"service":         result.ServiceName,
		"deployment_id":   result.DeploymentID,
		"task_definition": result.TaskDefinition,
	})

	return result.Message, nil
}
