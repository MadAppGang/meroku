package deployer

import (
	"fmt"
	"time"

	"madappgang.com/infrastructure/ci_lambda/config"
	"madappgang.com/infrastructure/ci_lambda/services"
	"madappgang.com/infrastructure/ci_lambda/utils"
)

// DeployerV2 orchestrates ECS deployments using V2 architecture (direct resource names)
type DeployerV2 struct {
	ecsSvc   *services.ECSServiceV2
	slackSvc *services.SlackService
	config   *config.Config
	logger   *utils.Logger
}

// NewDeployerV2 creates a new V2 deployer instance
func NewDeployerV2(
	ecsSvc *services.ECSServiceV2,
	slackSvc *services.SlackService,
	cfg *config.Config,
	logger *utils.Logger,
) *DeployerV2 {
	return &DeployerV2{
		ecsSvc:   ecsSvc,
		slackSvc: slackSvc,
		config:   cfg,
		logger:   logger,
	}
}

// DeployOptions contains options for a deployment
type DeployOptions struct {
	ServiceIdentifier string // Service ID (empty string for backend, "api" for api, etc.)
	TaskDefinition    string // Optional: if empty, uses latest
	Reason            string // Reason for deployment (for logging/notifications)
	SourceEvent       string // Source of the deployment trigger (ECR, SSM, S3, manual)
}

// DeployResult contains the result of a deployment operation
type DeployResult struct {
	Success           bool
	ServiceIdentifier string
	ServiceName       string // Actual ECS service name
	ClusterName       string
	TaskDefinition    string
	DeploymentID      string
	Message           string
	Error             error
}

// Deploy performs an ECS service deployment with retries and notifications
func (d *DeployerV2) Deploy(opts DeployOptions) *DeployResult {
	log := d.logger.WithFields(map[string]interface{}{
		"service_id": opts.ServiceIdentifier,
		"source":     opts.SourceEvent,
		"reason":     opts.Reason,
	})

	log.Info("Starting deployment (V2)", nil)

	// Get actual service name for display
	serviceName := "(unknown)"
	if mapping, err := d.config.GetServiceMapping(opts.ServiceIdentifier); err == nil {
		serviceName = mapping.ServiceName
	}

	// Send initial notification
	if d.config.EnableSlackNotifications {
		_ = d.slackSvc.SendNotification(services.NotificationData{
			Type:      services.NotificationInfo,
			Service:   serviceName,
			StateName: "DEPLOYMENT_INITIATING",
			Reason:    opts.Reason,
		})
	}

	// Attempt deployment with retries
	var lastErr error
	var result *services.DeploymentResult

	maxRetries := d.config.MaxDeploymentRetries
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Warn("Retrying deployment", map[string]interface{}{
				"attempt":     attempt,
				"max_retries": maxRetries,
			})
			// Exponential backoff
			time.Sleep(time.Duration(attempt) * 5 * time.Second)
		}

		// Perform deployment using V2 service (direct resource lookup)
		result, lastErr = d.ecsSvc.Deploy(services.DeploymentRequest{
			ServiceIdentifier: opts.ServiceIdentifier,
			TaskDefinition:    opts.TaskDefinition,
			ForceNewDeploy:    true,
		})

		if lastErr == nil {
			// Success!
			break
		}

		log.Error("Deployment attempt failed", map[string]interface{}{
			"attempt": attempt + 1,
			"error":   lastErr.Error(),
		})
	}

	// Check final result
	if lastErr != nil {
		// All attempts failed
		log.Error("Deployment failed after all retries", map[string]interface{}{
			"attempts": maxRetries + 1,
			"error":    lastErr.Error(),
		})

		// Send failure notification
		if d.config.EnableSlackNotifications {
			_ = d.slackSvc.SendDeploymentFailure(
				serviceName,
				"",
				fmt.Sprintf("Failed after %d attempts: %s", maxRetries+1, lastErr.Error()),
			)
		}

		return &DeployResult{
			Success:           false,
			ServiceIdentifier: opts.ServiceIdentifier,
			ServiceName:       serviceName,
			Message:           fmt.Sprintf("Deployment failed after %d attempts", maxRetries+1),
			Error:             lastErr,
		}
	}

	// Success!
	log.Info("Deployment completed successfully", map[string]interface{}{
		"deployment_id":   result.DeploymentID,
		"task_definition": result.TaskDefinition,
		"cluster":         result.ClusterName,
		"service_name":    result.ServiceName,
	})

	// Send success notification
	if d.config.EnableSlackNotifications {
		_ = d.slackSvc.SendDeploymentSuccess(
			result.ServiceName,
			result.DeploymentID,
			result.TaskDefinition,
		)
	}

	return &DeployResult{
		Success:           true,
		ServiceIdentifier: result.ServiceIdentifier,
		ServiceName:       result.ServiceName,
		ClusterName:       result.ClusterName,
		TaskDefinition:    result.TaskDefinition,
		DeploymentID:      result.DeploymentID,
		Message:           result.Message,
		Error:             nil,
	}
}

// DeployMultiple deploys multiple services (useful for S3 env file changes affecting multiple services)
func (d *DeployerV2) DeployMultiple(services []DeployOptions) []*DeployResult {
	results := make([]*DeployResult, len(services))

	d.logger.Info("Deploying multiple services (V2)", map[string]interface{}{
		"count":    len(services),
		"services": extractServiceIdentifiers(services),
	})

	for i, opts := range services {
		results[i] = d.Deploy(opts)
	}

	// Summary log
	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}

	d.logger.Info("Multiple deployments completed", map[string]interface{}{
		"total":      len(results),
		"successful": successCount,
		"failed":     len(results) - successCount,
	})

	return results
}

func extractServiceIdentifiers(opts []DeployOptions) []string {
	ids := make([]string, len(opts))
	for i, opt := range opts {
		if opt.ServiceIdentifier == "" {
			ids[i] = "(backend)"
		} else {
			ids[i] = opt.ServiceIdentifier
		}
	}
	return ids
}
