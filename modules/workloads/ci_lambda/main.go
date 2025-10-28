package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"madappgang.com/infrastructure/ci_lambda/config"
	"madappgang.com/infrastructure/ci_lambda/deployer"
	"madappgang.com/infrastructure/ci_lambda/handlers"
	"madappgang.com/infrastructure/ci_lambda/services"
	"madappgang.com/infrastructure/ci_lambda/utils"
)

// Application holds all initialized components
type Application struct {
	config   *config.Config
	logger   *utils.Logger
	ecsSvc   *services.ECSServiceV2
	slackSvc *services.SlackService
	deployer *deployer.DeployerV2
	handler  *handlers.EventHandlerV2
}

// Initialize sets up all application components using V2 architecture
func Initialize() (*Application, error) {
	// Load configuration from environment variables (V2 with direct resource names)
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize logger
	logger := utils.NewLogger(cfg)
	logger.Info("Lambda function initializing (V2 Architecture)", map[string]interface{}{
		"project":         cfg.ProjectName,
		"environment":     cfg.Environment,
		"region":          cfg.AWSRegion,
		"cluster":         cfg.GetClusterName(),
		"log_level":       cfg.LogLevel,
		"dry_run":         cfg.DryRun,
		"services_count":  len(cfg.ServiceMap),
		"architecture":    "v2-direct-naming",
	})

	// Initialize ECS service V2 (uses direct resource names)
	ecsSvc, err := services.NewECSServiceV2(cfg, logger)
	if err != nil {
		logger.Error("Failed to initialize ECS service", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to initialize ECS service: %w", err)
	}
	logger.Info("ECS service initialized (V2)", map[string]interface{}{
		"cluster": cfg.GetClusterName(),
	})

	// Initialize Slack service
	slackSvc, err := services.NewSlackService(cfg, logger)
	if err != nil {
		logger.Error("Failed to initialize Slack service", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to initialize Slack service: %w", err)
	}
	logger.Info("Slack service initialized", map[string]interface{}{
		"enabled": cfg.SlackWebhookURL != "",
	})

	// Initialize deployer (wraps ECS and Slack services)
	dep := deployer.NewDeployerV2(ecsSvc, slackSvc, cfg, logger)
	logger.Info("Deployer initialized (V2)", map[string]interface{}{
		"max_retries": cfg.MaxDeploymentRetries,
		"timeout":     cfg.DeploymentTimeoutSeconds,
	})

	// Initialize event handler
	handler := handlers.NewEventHandlerV2(cfg, dep, slackSvc, logger)
	logger.Info("Event handler initialized (V2)", map[string]interface{}{
		"ecr_monitoring": cfg.EnableECRMonitoring,
		"ssm_monitoring": cfg.EnableSSMMonitoring,
		"s3_monitoring":  cfg.EnableS3Monitoring,
		"manual_deploy":  cfg.EnableManualDeploy,
	})

	// Log service configuration summary
	logger.Info("Service configuration", map[string]interface{}{
		"services": cfg.ListAllServices(),
	})

	logger.Info("Lambda function initialization complete (V2 Architecture)", nil)

	return &Application{
		config:   cfg,
		logger:   logger,
		ecsSvc:   ecsSvc,
		slackSvc: slackSvc,
		deployer: dep,
		handler:  handler,
	}, nil
}

// HandleEvent processes CloudWatch events
func (app *Application) HandleEvent(ctx context.Context, event events.CloudWatchEvent) (string, error) {
	app.logger.Info("Lambda invocation started", map[string]interface{}{
		"event_id":     event.ID,
		"event_source": event.Source,
		"detail_type":  event.DetailType,
		"region":       event.Region,
		"architecture": "v2-direct-naming",
	})

	// Delegate to event handler
	result, err := app.handler.HandleEvent(ctx, event)

	if err != nil {
		app.logger.Error("Event processing failed", map[string]interface{}{
			"event_id": event.ID,
			"error":    err.Error(),
		})
		return "", err
	}

	app.logger.Info("Event processing completed successfully", map[string]interface{}{
		"event_id": event.ID,
		"result":   result,
	})

	return result, nil
}

// Global application instance (initialized once per Lambda container)
var app *Application

func main() {
	var err error

	// Initialize application (runs once per Lambda container lifecycle)
	app, err = Initialize()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to initialize application: %v\n", err)
		os.Exit(1)
	}

	// Start Lambda runtime
	lambda.Start(app.HandleEvent)
}
