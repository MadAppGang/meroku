package handlers

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"madappgang.com/infrastructure/ci_lambda/config"
	"madappgang.com/infrastructure/ci_lambda/deployer"
	"madappgang.com/infrastructure/ci_lambda/services"
	"madappgang.com/infrastructure/ci_lambda/utils"
)

// EventHandler handles CloudWatch events
type EventHandler struct {
	config   *config.Config
	deployer *deployer.Deployer
	slackSvc *services.SlackService
	logger   *utils.Logger
}

// NewEventHandler creates a new event handler
func NewEventHandler(
	cfg *config.Config,
	dep *deployer.Deployer,
	slackSvc *services.SlackService,
	logger *utils.Logger,
) *EventHandler {
	return &EventHandler{
		config:   cfg,
		deployer: dep,
		slackSvc: slackSvc,
		logger:   logger,
	}
}

// HandleEvent routes events to appropriate handlers
func (h *EventHandler) HandleEvent(ctx context.Context, event events.CloudWatchEvent) (string, error) {
	log := h.logger.WithFields(map[string]interface{}{
		"event_id":     event.ID,
		"event_source": event.Source,
		"detail_type":  event.DetailType,
	})

	log.Info("Received event", nil)

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
