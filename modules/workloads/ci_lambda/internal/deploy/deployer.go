// Package deploy turns a resolved identifier into an ECS deployment.
package deploy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"madappgang.com/infrastructure/ci_lambda/internal/awsecs"
	"madappgang.com/infrastructure/ci_lambda/internal/config"
	"madappgang.com/infrastructure/ci_lambda/internal/slack"
)

// Source records what triggered a deployment.
type Source string

const (
	SourceECR    Source = "ecr"
	SourceSSM    Source = "ssm"
	SourceS3     Source = "s3"
	SourceManual Source = "manual"
)

var (
	// ErrUnknownTarget means the identifier is not in the target map. That is
	// a configuration fact, not a transient failure: it fails on the first
	// attempt and never reaches AWS.
	ErrUnknownTarget = errors.New("unknown deployment target")
	// ErrInvalidRequest means the request cannot be carried out as asked.
	ErrInvalidRequest = errors.New("invalid deployment request")
)

// Request is one deployment.
type Request struct {
	// ID is an identifier that already resolved against a Terraform-emitted
	// map: "backend", a service name, or "task:{name}".
	ID string
	// ImageURI is the image to run. Scheduled tasks only.
	ImageURI string
	// TaskDefinition pins a specific revision or ARN. Manual deploys only;
	// empty means "let ECS resolve the latest ACTIVE revision of the family".
	TaskDefinition string
	Reason         string
	Source         Source
}

// Result describes a completed deployment.
type Result struct {
	ID             string
	ServiceName    string
	TaskDefinition string
	DeploymentID   string
	Kind           config.Kind
}

// ECS is the deployment surface the Deployer needs.
type ECS interface {
	UpdateService(context.Context, awsecs.UpdateRequest) (awsecs.UpdateResult, error)
	RegisterRevisionWithImage(ctx context.Context, family, imageURI string) (string, error)
}

// Deployer applies a retry policy and notification policy around ECS calls.
type Deployer struct {
	cfg   *config.Config
	ecs   ECS
	slack slack.Notifier
	log   *slog.Logger

	// injected for tests
	sleep  func(context.Context, time.Duration) error
	jitter func() float64
}

// New builds a Deployer.
func New(cfg *config.Config, e ECS, n slack.Notifier, log *slog.Logger) *Deployer {
	return &Deployer{
		cfg:    cfg,
		ecs:    e,
		slack:  n,
		log:    log,
		sleep:  sleepCtx,
		jitter: defaultJitter,
	}
}

// Deploy performs one deployment.
func (d *Deployer) Deploy(ctx context.Context, req Request) (Result, error) {
	log := d.log.With("target", req.ID, "source", string(req.Source), "reason", req.Reason)

	// Resolution happens once, outside the retry loop.
	target, ok := d.cfg.Target(req.ID)
	if !ok {
		err := fmt.Errorf("%w: %q", ErrUnknownTarget, req.ID)
		log.Error("deployment target not configured", "error", err)
		return Result{}, err
	}

	displayName := target.ServiceName
	if displayName == "" {
		displayName = target.TaskFamily
	}

	d.slack.Notify(ctx, slack.Message{
		Level:   slack.LevelInfo,
		Env:     d.cfg.Env,
		Service: displayName,
		State:   "DEPLOYMENT_INITIATING",
		Reason:  req.Reason,
	})

	log.Info("deployment starting",
		"kind", string(target.Kind),
		"service_name", target.ServiceName,
		"task_family", target.TaskFamily,
		"cluster", d.cfg.Cluster)

	result, err := d.attempt(ctx, log, req, target)
	if err != nil {
		log.Error("deployment failed", "error", err, "retryable", Retryable(err))
		d.slack.Notify(ctx, slack.Message{
			Level:   slack.LevelError,
			Env:     d.cfg.Env,
			Service: displayName,
			State:   "DEPLOYMENT_FAILED",
			Reason:  err.Error(),
		})
		return Result{}, err
	}

	log.Info("deployment requested",
		"task_definition", result.TaskDefinition,
		"deployment_id", result.DeploymentID)

	d.slack.Notify(ctx, slack.Message{
		Level:        slack.LevelSuccess,
		Env:          d.cfg.Env,
		Service:      displayName,
		State:        "DEPLOYMENT_STARTED",
		DeploymentID: result.DeploymentID,
		TaskDef:      result.TaskDefinition,
	})

	return result, nil
}

// attempt runs the AWS call, retrying only failures that could plausibly
// succeed on a second try.
func (d *Deployer) attempt(ctx context.Context, log *slog.Logger, req Request, target config.Target) (Result, error) {
	var lastErr error
	attempts := 0

	for i := 0; ; i++ {
		if i > 0 {
			delay := backoff(d.cfg.RetryBaseDelay, i, d.jitter())
			if !d.fitsDeadline(ctx, delay) {
				log.Warn("stopping retries: not enough invocation time left",
					"attempt", i+1, "planned_delay", delay.String())
				break
			}
			log.Warn("retrying deployment", "attempt", i+1, "max_attempts", d.cfg.MaxRetries+1, "delay", delay.String())
			if err := d.sleep(ctx, delay); err != nil {
				lastErr = fmt.Errorf("retry aborted: %w", err)
				break
			}
		}

		attempts++

		res, err := d.call(ctx, req, target)
		if err == nil {
			return res, nil
		}
		lastErr = err

		if !Retryable(err) {
			log.Error("deployment failed with a non-retryable error", "attempt", i+1, "error", err)
			return Result{}, err
		}
		log.Error("deployment attempt failed", "attempt", i+1, "error", err)

		if i >= d.cfg.MaxRetries {
			break
		}
	}

	// Report what was actually tried, not the ceiling. The loop breaks early when
	// the remaining invocation time cannot fit the next backoff, and when the
	// context is cancelled mid-sleep — so MaxRetries+1 overstated the effort in
	// exactly the cases an operator is most likely to be reading the message.
	return Result{}, fmt.Errorf("deployment of %q failed after %d attempt(s): %w", req.ID, attempts, lastErr)
}

func (d *Deployer) call(ctx context.Context, req Request, target config.Target) (Result, error) {
	if target.Kind == config.KindScheduledTask {
		if req.ImageURI == "" {
			return Result{}, fmt.Errorf(
				"%w: scheduled task %q can only be deployed with an image URI", ErrInvalidRequest, req.ID)
		}
		arn, err := d.ecs.RegisterRevisionWithImage(ctx, target.TaskFamily, req.ImageURI)
		if err != nil {
			return Result{}, err
		}
		return Result{
			ID:             req.ID,
			ServiceName:    target.TaskFamily,
			TaskDefinition: arn,
			Kind:           target.Kind,
		}, nil
	}

	// Hand ECS the family, not a revision we resolved ourselves.
	taskDef := target.TaskFamily
	if req.TaskDefinition != "" {
		taskDef = req.TaskDefinition
	}

	out, err := d.ecs.UpdateService(ctx, awsecs.UpdateRequest{
		ServiceName:    target.ServiceName,
		TaskDefinition: taskDef,
		Force:          true,
	})
	if err != nil {
		return Result{}, err
	}
	return Result{
		ID:             req.ID,
		ServiceName:    out.ServiceName,
		TaskDefinition: out.TaskDefinition,
		DeploymentID:   out.DeploymentID,
		Kind:           target.Kind,
	}, nil
}

// fitsDeadline reports whether the invocation has room for another sleep plus
// a call.
func (d *Deployer) fitsDeadline(ctx context.Context, delay time.Duration) bool {
	deadline, ok := ctx.Deadline()
	if !ok {
		return true
	}
	const callBudget = 2 * time.Second
	return time.Until(deadline) > delay+callBudget
}

// DeployAll deploys sequentially and never stops at the first failure.
//
// Fan-out here is one to three targets (an env file shared by a handful of
// services); concurrency would buy nothing and risk throttling.
func (d *Deployer) DeployAll(ctx context.Context, reqs []Request) ([]Result, error) {
	var (
		results []Result
		errs    []error
	)
	for _, req := range reqs {
		res, err := d.Deploy(ctx, req)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		results = append(results, res)
	}
	return results, errors.Join(errs...)
}
