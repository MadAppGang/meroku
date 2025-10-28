package services

import (
	"bytes"
	"fmt"
	"net/http"
	"text/template"
	"time"

	"madappgang.com/infrastructure/ci_lambda/config"
	"madappgang.com/infrastructure/ci_lambda/utils"
)

// SlackService handles Slack notifications
type SlackService struct {
	webhookURL string
	enabled    bool
	httpClient *http.Client
	config     *config.Config
	logger     *utils.Logger
	templates  *SlackTemplates
}

// SlackTemplates holds compiled notification templates
type SlackTemplates struct {
	Success *template.Template
	Error   *template.Template
	Info    *template.Template
}

// NotificationLevel represents the severity of a notification
type NotificationType string

const (
	NotificationSuccess NotificationType = "success"
	NotificationError   NotificationType = "error"
	NotificationInfo    NotificationType = "info"
	NotificationWarning NotificationType = "warning"
)

// NotificationData contains data for Slack messages
type NotificationData struct {
	Type          NotificationType
	Environment   string
	Service       string
	Reason        string
	StateName     string
	DeploymentID  string
	TaskDef       string
	EventID       string
	Timestamp     time.Time
}

// NewSlackService creates a new Slack notification service
func NewSlackService(cfg *config.Config, logger *utils.Logger) (*SlackService, error) {
	service := &SlackService{
		webhookURL: cfg.SlackWebhookURL,
		enabled:    cfg.SlackWebhookURL != "",
		httpClient: &http.Client{Timeout: 10 * time.Second},
		config:     cfg,
		logger:     logger,
	}

	// Load templates
	if err := service.loadTemplates(); err != nil {
		return nil, fmt.Errorf("failed to load Slack templates: %w", err)
	}

	return service, nil
}

// SendNotification sends a notification to Slack
func (s *SlackService) SendNotification(data NotificationData) error {
	if !s.enabled {
		s.logger.Debug("Slack notifications disabled, skipping", map[string]interface{}{
			"service": data.Service,
			"type":    data.Type,
		})
		return nil
	}

	// Set timestamp if not provided
	if data.Timestamp.IsZero() {
		data.Timestamp = time.Now()
	}

	// Add environment
	if data.Environment == "" {
		data.Environment = s.config.Environment
	}

	// Select template based on notification type
	var tmpl *template.Template
	switch data.Type {
	case NotificationSuccess:
		tmpl = s.templates.Success
	case NotificationError:
		tmpl = s.templates.Error
	default:
		tmpl = s.templates.Info
	}

	// Render template
	var payload bytes.Buffer
	if err := tmpl.Execute(&payload, data); err != nil {
		s.logger.Error("Failed to render Slack template", map[string]interface{}{
			"error": err.Error(),
			"type":  data.Type,
		})
		return fmt.Errorf("failed to render template: %w", err)
	}

	// Send HTTP request
	req, err := http.NewRequest(http.MethodPost, s.webhookURL, &payload)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	s.logger.Debug("Sending Slack notification", map[string]interface{}{
		"service": data.Service,
		"type":    data.Type,
	})

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error("Failed to send Slack notification", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.logger.Error("Slack webhook returned error status", map[string]interface{}{
			"status_code": resp.StatusCode,
			"status":      resp.Status,
		})
		return fmt.Errorf("slack webhook returned status: %s", resp.Status)
	}

	s.logger.Info("Slack notification sent successfully", map[string]interface{}{
		"service": data.Service,
		"type":    data.Type,
	})

	return nil
}

// SendDeploymentStarted sends a notification for deployment start
func (s *SlackService) SendDeploymentStarted(serviceName, deploymentID string) error {
	return s.SendNotification(NotificationData{
		Type:         NotificationInfo,
		Service:      serviceName,
		StateName:    "DEPLOYMENT_STARTED",
		DeploymentID: deploymentID,
		Reason:       "New deployment initiated",
	})
}

// SendDeploymentSuccess sends a notification for successful deployment
func (s *SlackService) SendDeploymentSuccess(serviceName, deploymentID, taskDef string) error {
	return s.SendNotification(NotificationData{
		Type:         NotificationSuccess,
		Service:      serviceName,
		StateName:    "DEPLOYMENT_COMPLETED",
		DeploymentID: deploymentID,
		TaskDef:      taskDef,
		Reason:       "Deployment completed successfully",
	})
}

// SendDeploymentFailure sends a notification for failed deployment
func (s *SlackService) SendDeploymentFailure(serviceName, deploymentID, reason string) error {
	return s.SendNotification(NotificationData{
		Type:         NotificationError,
		Service:      serviceName,
		StateName:    "DEPLOYMENT_FAILED",
		DeploymentID: deploymentID,
		Reason:       reason,
	})
}

// loadTemplates loads and compiles Slack message templates
func (s *SlackService) loadTemplates() error {
	// Success template
	successTmpl := `{
		"blocks": [
			{
				"type": "section",
				"text": {
					"type": "mrkdwn",
					"text": ":white_check_mark: *Deployment Successful*\n*Environment:* {{.Environment}}\n*Service:* {{.Service}}\n*Status:* {{.StateName}}"
				}
			}
			{{if .DeploymentID}},
			{
				"type": "context",
				"elements": [
					{
						"type": "mrkdwn",
						"text": "Deployment ID: {{.DeploymentID}}"
					}
				]
			}
			{{end}}
			{{if .TaskDef}},
			{
				"type": "context",
				"elements": [
					{
						"type": "mrkdwn",
						"text": "Task Definition: {{.TaskDef}}"
					}
				]
			}
			{{end}}
		]
	}`

	// Error template
	errorTmpl := `{
		"blocks": [
			{
				"type": "section",
				"text": {
					"type": "mrkdwn",
					"text": ":x: *Deployment Failed*\n*Environment:* {{.Environment}}\n*Service:* {{.Service}}\n*Status:* {{.StateName}}"
				}
			}
			{{if .Reason}},
			{
				"type": "section",
				"text": {
					"type": "mrkdwn",
					"text": "*Reason:* {{.Reason}}"
				}
			}
			{{end}}
			{{if .DeploymentID}},
			{
				"type": "context",
				"elements": [
					{
						"type": "mrkdwn",
						"text": "Deployment ID: {{.DeploymentID}}"
					}
				]
			}
			{{end}}
		]
	}`

	// Info template
	infoTmpl := `{
		"blocks": [
			{
				"type": "section",
				"text": {
					"type": "mrkdwn",
					"text": ":information_source: *Deployment Update*\n*Environment:* {{.Environment}}\n*Service:* {{.Service}}\n*Status:* {{.StateName}}"
				}
			}
			{{if .Reason}},
			{
				"type": "section",
				"text": {
					"type": "mrkdwn",
					"text": "{{.Reason}}"
				}
			}
			{{end}}
		]
	}`

	var err error
	s.templates = &SlackTemplates{}

	s.templates.Success, err = template.New("success").Parse(successTmpl)
	if err != nil {
		return fmt.Errorf("failed to parse success template: %w", err)
	}

	s.templates.Error, err = template.New("error").Parse(errorTmpl)
	if err != nil {
		return fmt.Errorf("failed to parse error template: %w", err)
	}

	s.templates.Info, err = template.New("info").Parse(infoTmpl)
	if err != nil {
		return fmt.Errorf("failed to parse info template: %w", err)
	}

	return nil
}
