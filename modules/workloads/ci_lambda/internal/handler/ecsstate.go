package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"madappgang.com/infrastructure/ci_lambda/internal/slack"
)

// ECS service action / deployment state change event names.
const (
	ecsDeploymentInProgress = "SERVICE_DEPLOYMENT_IN_PROGRESS"
	ecsDeploymentCompleted  = "SERVICE_DEPLOYMENT_COMPLETED"
	ecsDeploymentFailed     = "SERVICE_DEPLOYMENT_FAILED"
	ecsSteadyState          = "SERVICE_STEADY_STATE"
	ecsTaskStartImpaired    = "SERVICE_TASK_START_IMPAIRED"
)

type ecsStateDetail struct {
	EventType    string `json:"eventType"`
	EventName    string `json:"eventName"`
	Reason       string `json:"reason"`
	DeploymentID string `json:"deploymentId"`
}

// ecsState notifies and nothing else. It never deploys, and it never returns
// an error: a webhook outage used to fail the invocation, which made
// EventBridge retry and Slack receive the same notification several times.
func (h *Handler) ecsState(ctx context.Context, log *slog.Logger, ev events.CloudWatchEvent) (Response, error) {
	var d ecsStateDetail
	if err := json.Unmarshal(ev.Detail, &d); err != nil {
		log.Warn("ECS state event detail could not be parsed", "error", err)
		return ignored("unparsable ECS state event detail"), nil
	}

	if d.EventName == ecsSteadyState {
		return ignored("SERVICE_STEADY_STATE is suppressed"), nil
	}

	serviceARN := ""
	if len(ev.Resources) > 0 {
		serviceARN = ev.Resources[0]
	}

	log.Info("ECS state change",
		"ecs_event_name", d.EventName,
		"ecs_event_type", d.EventType,
		"service_arn", serviceARN,
		"deployment_id", d.DeploymentID)

	// Slack gets the service name; the full ARN stays in the log line above.
	// An ARN is 90-odd characters of account id and cluster path wrapped around
	// the one word the reader wants, and it is the first thing in the message.
	serviceName := serviceARN
	if i := strings.LastIndex(serviceName, "/"); i >= 0 {
		serviceName = serviceName[i+1:]
	}

	level := slack.LevelInfo
	switch d.EventName {
	case ecsDeploymentCompleted:
		level = slack.LevelSuccess
	case ecsDeploymentFailed, ecsTaskStartImpaired:
		level = slack.LevelError
	case ecsDeploymentInProgress:
		level = slack.LevelInfo
	}

	h.slack.Notify(ctx, slack.Message{
		Level:        level,
		Env:          h.cfg.Env,
		Service:      serviceName,
		State:        d.EventName,
		Reason:       d.Reason,
		DeploymentID: d.DeploymentID,
	})

	return Response{Status: StatusNotified, Detail: d.EventName}, nil
}
