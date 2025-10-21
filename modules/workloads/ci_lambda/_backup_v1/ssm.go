package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/aws/aws-lambda-go/events"
	"madappgang.com/infrastructure/ci_lambda/deployer"
)

// SSMEventDetail represents SSM Parameter Store change events
type SSMEventDetail struct {
	Operation   string `json:"operation"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// handleSSMEvent processes SSM parameter change events
func (h *EventHandler) handleSSMEvent(ctx context.Context, event events.CloudWatchEvent) (string, error) {
	log := h.logger.WithFields(map[string]interface{}{
		"event_type": "ssm",
		"event_id":   event.ID,
	})

	// Parse event detail
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

	// Extract service name from parameter path
	// Expected format: /{env}/{project}/{service}/parameter_name
	// Example: /dev/myproject/api/DATABASE_URL
	serviceName, err := h.extractServiceFromSSMPath(detail.Name)
	if err != nil {
		log.Warn("Unable to extract service from SSM parameter path", map[string]interface{}{
			"parameter": detail.Name,
			"error":     err.Error(),
		})
		return fmt.Sprintf("SSM parameter %s does not match expected pattern, skipping", detail.Name), nil
	}

	log.Info("Extracted service from SSM parameter", map[string]interface{}{
		"parameter": detail.Name,
		"service":   serviceName,
	})

	// Trigger deployment
	result := h.deployer.Deploy(deployer.DeployOptions{
		ServiceName: serviceName,
		Reason:      fmt.Sprintf("SSM parameter changed: %s (%s)", detail.Name, detail.Operation),
		SourceEvent: "SSM",
	})

	if result.Error != nil {
		log.Error("Deployment failed", map[string]interface{}{
			"service": serviceName,
			"error":   result.Error.Error(),
		})
		return "", result.Error
	}

	log.Info("SSM-triggered deployment completed", map[string]interface{}{
		"service":         result.ServiceName,
		"deployment_id":   result.DeploymentID,
		"task_definition": result.TaskDefinition,
	})

	return result.Message, nil
}

// extractServiceFromSSMPath extracts service name from SSM parameter path
// Expected format: /{env}/{project}/{service}/parameter_name
func (h *EventHandler) extractServiceFromSSMPath(parameterPath string) (string, error) {
	// Build regex pattern: /env/project/service/...
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
	if serviceName == "backend" || serviceName == h.config.BackendServiceName {
		return "", nil // Empty string represents backend service
	}

	return serviceName, nil
}
