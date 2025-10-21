package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"madappgang.com/infrastructure/ci_lambda/deployer"
	"madappgang.com/infrastructure/ci_lambda/utils"
)

// ECRImagePushEventDetail represents the detail section of an ECR event
type ECRImagePushEventDetail struct {
	RepositoryName string `json:"repository-name"`
	Tag            string `json:"image-tag"`
	Action         string `json:"action-type"`
	Result         string `json:"result"`
}

// handleECREvent processes ECR image push events
func (h *EventHandler) handleECREvent(ctx context.Context, event events.CloudWatchEvent) (string, error) {
	log := h.logger.WithFields(map[string]interface{}{
		"event_type": "ecr",
		"event_id":   event.ID,
	})

	// Parse event detail
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

	// Filter events - only process successful pushes
	if detail.Action != "PUSH" {
		log.Info("Skipping non-PUSH event", map[string]interface{}{
			"action": detail.Action,
		})
		return fmt.Sprintf("Skipped event with action: %s", detail.Action), nil
	}

	if detail.Result != "SUCCESS" {
		log.Info("Skipping failed push event", map[string]interface{}{
			"result": detail.Result,
		})
		return fmt.Sprintf("Skipped event with result: %s", detail.Result), nil
	}

	// Extract service name from repository name
	serviceName, err := utils.GetServiceNameFromRepoName(detail.RepositoryName, h.config.ProjectName)
	if err != nil {
		log.Warn("Unable to extract service name from repository", map[string]interface{}{
			"repository": detail.RepositoryName,
			"error":      err.Error(),
		})
		return "", fmt.Errorf("unable to extract service name from repo name %s: %w", detail.RepositoryName, err)
	}

	log.Info("Extracted service name from ECR repository", map[string]interface{}{
		"repository": detail.RepositoryName,
		"service":    serviceName,
	})

	// Trigger deployment
	result := h.deployer.Deploy(deployer.DeployOptions{
		ServiceName: serviceName,
		Reason:      fmt.Sprintf("New ECR image pushed: %s:%s", detail.RepositoryName, detail.Tag),
		SourceEvent: "ECR",
	})

	if result.Error != nil {
		log.Error("Deployment failed", map[string]interface{}{
			"service": serviceName,
			"error":   result.Error.Error(),
		})
		return "", result.Error
	}

	log.Info("ECR-triggered deployment completed", map[string]interface{}{
		"service":         result.ServiceName,
		"deployment_id":   result.DeploymentID,
		"task_definition": result.TaskDefinition,
	})

	return result.Message, nil
}
