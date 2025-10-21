package deployer

import (
	"fmt"
	"time"

	"madappgang.com/infrastructure/ci_lambda/config"
	"madappgang.com/infrastructure/ci_lambda/services"
	"madappgang.com/infrastructure/ci_lambda/utils"
)

// Deployer orchestrates ECS deployments and notifications
type Deployer struct {
	ecsSvc   *services.ECSService
	slackSvc *services.SlackService
	config   *config.Config
	logger   *utils.Logger
}

// NewDeployer creates a new deployer instance
func NewDeployer(
	ecsSvc *services.ECSService,
	slackSvc *services.SlackService,
	cfg *config.Config,
	logger *utils.Logger,
) *Deployer {
	return &Deployer{
		ecsSvc:   ecsSvc,
		slackSvc: slackSvc,
		config:   cfg,
		logger:   logger,
	}
}

// DeployOptions contains options for a deployment
type DeployOptions struct {
	ServiceName    string
	TaskDefinition string // Optional: if empty, uses latest
	Reason         string // Reason for deployment (for logging/notifications)
	SourceEvent    string // Source of the deployment trigger (ECR, SSM, S3, manual)
}

// DeployResult contains the result of a deployment operation
type DeployResult struct {
	Success        bool
	ServiceName    string
	ClusterName    string
	TaskDefinition string
	DeploymentID   string
	Message        string
	Error          error
}

// Deploy performs an ECS service deployment with retries and notifications
func (d *Deployer) Deploy(opts DeployOptions) *DeployResult {
	log := d.logger.WithFields(map[string]interface{}{
		"service": opts.ServiceName,
		"source":  opts.SourceEvent,
		"reason":  opts.Reason,
	})

	log.Info("Starting deployment", nil)

	// Normalize service name
	serviceName := d.config.NormalizeServiceName(opts.ServiceName)

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

		// Perform deployment
		result, lastErr = d.ecsSvc.Deploy(services.DeploymentRequest{
			ServiceName:    serviceName,
			TaskDefinition: opts.TaskDefinition,
			ForceNewDeploy: true,
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
			Success:     false,
			ServiceName: serviceName,
			Message:     fmt.Sprintf("Deployment failed after %d attempts", maxRetries+1),
			Error:       lastErr,
		}
	}

	// Success!
	log.Info("Deployment completed successfully", map[string]interface{}{
		"deployment_id":   result.DeploymentID,
		"task_definition": result.TaskDefinition,
		"cluster":         result.ClusterName,
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
		Success:        true,
		ServiceName:    result.ServiceName,
		ClusterName:    result.ClusterName,
		TaskDefinition: result.TaskDefinition,
		DeploymentID:   result.DeploymentID,
		Message:        result.Message,
		Error:          nil,
	}
}

// DeployMultiple deploys multiple services (useful for S3 env file changes affecting multiple services)
func (d *Deployer) DeployMultiple(services []DeployOptions) []*DeployResult {
	results := make([]*DeployResult, len(services))

	d.logger.Info("Deploying multiple services", map[string]interface{}{
		"count":    len(services),
		"services": extractServiceNames(services),
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

func extractServiceNames(opts []DeployOptions) []string {
	names := make([]string, len(opts))
	for i, opt := range opts {
		names[i] = opt.ServiceName
	}
	return names
}
