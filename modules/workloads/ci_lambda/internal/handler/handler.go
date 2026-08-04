// Package handler routes EventBridge events to deployments.
//
// Error policy: an error is returned only when a retry could plausibly
// succeed. Everything else — unknown source, unmapped repository, unparsable
// detail, a disabled feature flag — is an "ignored" response with a nil error.
// EventBridge invokes this function asynchronously, so returning an error for
// an event we will never be able to handle means retrying it for hours.
package handler

import (
	"context"
	"log/slog"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"madappgang.com/infrastructure/ci_lambda/internal/config"
	"madappgang.com/infrastructure/ci_lambda/internal/deploy"
	"madappgang.com/infrastructure/ci_lambda/internal/slack"
)

// Status values for Response.
const (
	StatusDeployed = "deployed"
	StatusIgnored  = "ignored"
	StatusNotified = "notified"
)

// Response is the invocation result. EventBridge discards it; it is visible in
// `aws lambda invoke` output and in the logs.
type Response struct {
	Status   string   `json:"status"`
	Detail   string   `json:"detail,omitempty"`
	Deployed []string `json:"deployed,omitempty"`
}

func ignored(detail string) Response {
	return Response{Status: StatusIgnored, Detail: detail}
}

// Deployer is the deployment surface the handler needs.
type Deployer interface {
	Deploy(context.Context, deploy.Request) (deploy.Result, error)
	DeployAll(context.Context, []deploy.Request) ([]deploy.Result, error)
}

// Handler routes events.
type Handler struct {
	cfg   *config.Config
	dep   Deployer
	slack slack.Notifier
	log   *slog.Logger
}

// New builds a Handler.
func New(cfg *config.Config, dep Deployer, n slack.Notifier, log *slog.Logger) *Handler {
	return &Handler{cfg: cfg, dep: dep, slack: n, log: log}
}

// Handle is the Lambda entry point.
func (h *Handler) Handle(ctx context.Context, ev events.CloudWatchEvent) (Response, error) {
	log := h.log.With(
		"event_id", ev.ID,
		"event_source", ev.Source,
		"detail_type", ev.DetailType,
	)
	log.Info("event received")

	switch ev.Source {
	case SourceECR:
		if !h.cfg.Enable.ECR {
			return ignored("ECR monitoring disabled"), nil
		}
		return h.ecr(ctx, log, ev)

	case SourceECS:
		return h.ecsState(ctx, log, ev)

	case SourceSSM:
		if !h.cfg.Enable.SSM {
			return ignored("SSM monitoring disabled"), nil
		}
		return h.ssm(ctx, log, ev)

	case SourceS3:
		if !h.cfg.Enable.S3 {
			return ignored("S3 monitoring disabled"), nil
		}
		return h.s3(ctx, log, ev)
	}

	// Manual deploys arrive on a custom source, and the set of sources that
	// generators emit has changed over time ("action.deploy",
	// "action.{env}", "github.actions.{env}"). Route on the detail-type, which
	// has stayed stable, and let the project/env check below do the scoping.
	switch ev.DetailType {
	case DetailTypeDeploy, DetailTypeServiceDeploy:
		if !h.cfg.Enable.Manual {
			return ignored("manual deploy disabled"), nil
		}
		return h.manual(ctx, log, ev)
	}

	log.Info("no handler for event", "source", ev.Source)
	return ignored("unsupported event source: " + ev.Source), nil
}

// autoDeployable drops the identifiers Terraform marked auto_deploy = false and
// says so in the log.
//
// The flag is read, never derived: config.Target.AutoDeploy is a value Terraform
// put on the target, exactly like service_name and task_family.
//
// Filtering here rather than in Terraform is what makes the disabled case
// explainable. If the target were simply left out of ECR_REPO_MAP the lookup in
// the caller would return nothing, and the only sentence this Lambda could
// honestly write is "no target uses repository X" — which reads as a naming
// bug and sends whoever is on call looking for one. A push to a disabled target
// still invokes the Lambda (one invocation, no ECS call) and still writes a line
// naming the reason.
func (h *Handler) autoDeployable(log *slog.Logger, ids []string) []string {
	enabled, disabled := h.cfg.PartitionByAutoDeploy(ids)
	if len(disabled) > 0 {
		log.Info("auto_deploy is disabled for some targets", "targets", disabled)
	}
	return enabled
}

// autoDeployDisabled is the response when every resolved target has
// auto_deploy = false. The identifiers are named so the log answers "why did my
// push do nothing" without a second lookup.
func autoDeployDisabled(ids []string) Response {
	return ignored("auto_deploy is disabled for " + strings.Join(ids, ", "))
}

// deployOne runs a single deployment and maps the outcome onto the error
// policy above.
func (h *Handler) deployOne(ctx context.Context, log *slog.Logger, req deploy.Request) (Response, error) {
	res, err := h.dep.Deploy(ctx, req)
	if err != nil {
		if deploy.Retryable(err) {
			return Response{}, err
		}
		log.Warn("deployment not attempted again", "error", err, "target", req.ID)
		return ignored(err.Error()), nil
	}
	return Response{Status: StatusDeployed, Deployed: []string{res.ID}}, nil
}

// deployMany runs a fan-out and maps the outcome onto the error policy above.
func (h *Handler) deployMany(ctx context.Context, log *slog.Logger, reqs []deploy.Request) (Response, error) {
	results, err := h.dep.DeployAll(ctx, reqs)

	deployed := make([]string, 0, len(results))
	for _, r := range results {
		deployed = append(deployed, r.ID)
	}

	if err != nil {
		if deploy.Retryable(err) {
			// Returning an error fails the invocation, so EventBridge redelivers
			// and every member of the fan-out is attempted again. That is safe —
			// a force-new-deployment is idempotent — but the ones that already
			// succeeded are about to be redeployed, and nothing else records
			// that they got as far as they did. Name them before unwinding, or
			// the only trace of partial progress is gone.
			if len(deployed) > 0 {
				log.Warn("retrying the whole fan-out; these already deployed and will be redeployed",
					"deployed", deployed, "error", err)
			}
			return Response{Deployed: deployed}, err
		}
		log.Warn("some deployments will not be retried", "error", err)
		return Response{Status: StatusIgnored, Detail: err.Error(), Deployed: deployed}, nil
	}
	return Response{Status: StatusDeployed, Deployed: deployed}, nil
}
