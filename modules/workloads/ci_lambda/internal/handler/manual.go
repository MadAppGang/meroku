package handler

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/aws/aws-lambda-go/events"
	"madappgang.com/infrastructure/ci_lambda/internal/deploy"
)

// manualDetail is the payload the generated GitHub Actions workflows send.
//
// project and env are optional *here* because there are two rules in front of
// this handler, and only one of them can require them:
//
//   - aws_cloudwatch_event_rule.ci_manual_deploy accepts sources whose name
//     carries the environment ("action.{env}", "github.actions.{env}", plus
//     "action.production" in a production environment). It has no detail filter,
//     because EventBridge requires every key named in a pattern to be present
//     and the payloads already in the wild send only {"service": "..."}.
//     Filtering there would silently kill every existing pipeline. Events on
//     those sources are already environment-safe: another environment's rule
//     does not list them.
//   - aws_cloudwatch_event_rule.ci_manual_deploy_global accepts environment
//     agnostic sources ("action.deploy") and *does* require detail.project and
//     detail.env, because nothing else about such an event says which
//     environment it means.
//
// So a legacy payload arrives with neither field and is accepted within its own
// environment, and a current payload arrives with both and is checked below.
// The check is what stops a second meroku project in the same account being
// deployed by this one's event, which the source list alone cannot do.
type manualDetail struct {
	Service        string `json:"service"`
	Project        string `json:"project"`
	Env            string `json:"env"`
	TaskDefinition string `json:"task_definition"`
	ImageURI       string `json:"image_uri"`
	Reason         string `json:"reason"`
}

// manual handles a DEPLOY / SERVICE_DEPLOY event.
func (h *Handler) manual(ctx context.Context, log *slog.Logger, ev events.CloudWatchEvent) (Response, error) {
	var d manualDetail
	if err := json.Unmarshal(ev.Detail, &d); err != nil {
		log.Warn("manual deploy detail could not be parsed", "error", err)
		return ignored("unparsable manual deploy detail"), nil
	}

	// Scoping lives here rather than in the event pattern. When a payload does
	// say which project or environment it means, we honour it.
	if d.Project != "" && d.Project != h.cfg.Project {
		log.Info("manual deploy is for another project", "event_project", d.Project)
		return ignored("manual deploy targets project " + d.Project), nil
	}
	if d.Env != "" && d.Env != h.cfg.Env {
		log.Info("manual deploy is for another environment", "event_env", d.Env)
		return ignored("manual deploy targets environment " + d.Env), nil
	}

	if d.Service == "" {
		log.Warn("manual deploy detail has no service field")
		return ignored("manual deploy detail must include a service field"), nil
	}

	reason := d.Reason
	if reason == "" {
		reason = "Manual deployment triggered"
	}

	// auto_deploy is deliberately NOT consulted here. It answers "may an event
	// redeploy this on its own?", and a DEPLOY event is somebody asking for this
	// exact deployment. Turning off automatic deploys in prod must not also take
	// away the button that deploys prod.
	log.Info("manual deploy resolved", "target", d.Service)
	return h.deployOne(ctx, log, deploy.Request{
		ID:             d.Service,
		TaskDefinition: d.TaskDefinition,
		ImageURI:       d.ImageURI,
		Reason:         reason,
		Source:         deploy.SourceManual,
	})
}
