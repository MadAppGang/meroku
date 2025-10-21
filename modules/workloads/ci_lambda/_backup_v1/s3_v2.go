package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"madappgang.com/infrastructure/ci_lambda/deployer"
)

// S3EventDetail represents S3 object change events from CloudTrail
type S3EventDetail struct {
	RequestParameters struct {
		BucketName string `json:"bucketName"`
		Key        string `json:"key"`
	} `json:"requestParameters"`
	EventName string `json:"eventName"` // PutObject, DeleteObject, etc.
}

// handleS3Event processes S3 object change events using direct service lookups
func (h *EventHandler) handleS3EventV2(ctx context.Context, event events.CloudWatchEvent) (string, error) {
	log := h.logger.WithFields(map[string]interface{}{
		"event_type": "s3",
		"event_id":   event.ID,
	})

	// Parse event detail
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

	// Use config to find services that use this S3 file
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

	// Build deployment options for each service
	deployOpts := make([]deployer.DeployOptions, len(affectedServices))
	for i, serviceID := range affectedServices {
		deployOpts[i] = deployer.DeployOptions{
			ServiceName: serviceID,
			Reason:      fmt.Sprintf("S3 env file changed: s3://%s/%s", bucketName, objectKey),
			SourceEvent: "S3",
		}
	}

	// Deploy all affected services
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
