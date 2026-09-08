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
	// SourceTerraform is a deploy asked for by `terraform apply` itself, through
	// aws_lambda_invocation.{backend,services}_revision.
	//
	// It is split out of SourceManual for one reason: it is the only path whose
	// caller is a SYNCHRONOUS Invoke rather than an asynchronous EventBridge
	// delivery, and that difference is what makes the permission-propagation
	// poll safe here and unsafe everywhere else. awaitingPropagation in retry.go
	// carries the full argument.
	SourceTerraform Source = "terraform"
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
	// ImageURI is the image to run.
	//
	// Set, it means "register a revision of this family pinning this image, and
	// deploy that revision". Empty, it means "deploy the family and let ECS
	// resolve the latest ACTIVE revision" — which is the right answer for a
	// configuration change, where Terraform has just registered the revision
	// that carries it. Required for a scheduled task, which has no service to
	// update and so has nothing else to deploy.
	ImageURI string
	// TaskDefinition pins a specific revision or ARN. Manual deploys only;
	// empty means "let ECS resolve the latest ACTIVE revision of the family".
	//
	// It outranks ImageURI: an operator naming a revision is asking for THAT
	// revision, and registering a new one would silently give them a different
	// one.
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
	// now is the clock fitsDeadline measures the remaining invocation budget
	// against. It is injected alongside sleep so a test can drive a fake clock:
	// the permission-propagation poll is bounded by the deadline and by nothing
	// else, so a test of "it gives up rather than running past the deadline"
	// would otherwise be a wall-clock race spinning on instant fake sleeps.
	now func() time.Time
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
		now:    time.Now,
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
//
// Two budgets share the loop, because they answer different questions.
//
//   - RETRIES, for a transient AWS fault. Bounded by cfg.MaxRetries (2) with an
//     exponential backoff from cfg.RetryBaseDelay (1s): three attempts over
//     roughly three seconds, then give up.
//   - PROPAGATION POLLS, for an authorization refusal on the Terraform path.
//     Bounded by the invocation deadline and by nothing else, because the thing
//     being waited on — IAM eventual consistency after
//     aws_iam_role_policy_attachment.lambda_ecs — took longer than the entire
//     retry budget in the run that produced this code. awaitingPropagation in
//     retry.go records the timestamps and the scoping argument.
//
// They are counted separately on purpose. A poll must not consume the retry
// budget (three attempts would expire ~3s in, well before the observed 6.4s),
// and a retry must not inherit the poll's deadline-shaped bound (that is how a
// permanent condition eats a whole invocation).
func (d *Deployer) attempt(ctx context.Context, log *slog.Logger, req Request, target config.Target) (Result, error) {
	var (
		lastErr  error
		attempts int
		retries  int
		polls    int
	)

loop:
	for {
		attempts++

		// &req, not req: call pins a revision it registered back onto the
		// request, so a retry of the UpdateService that follows redeploys THAT
		// revision instead of registering another one. RegisterTaskDefinition
		// is not idempotent — without the write-back, a single throttled update
		// would turn one image push into up to three revisions, two of which
		// nothing ever ran. attempt holds its own copy of the Request, so the
		// mutation cannot escape this deployment.
		res, err := d.call(ctx, &req, target)
		if err == nil {
			if polls > 0 {
				log.Info("permission propagated; deployment accepted",
					"polls", polls, "attempts", attempts)
			}
			return res, nil
		}
		lastErr = err

		var delay time.Duration
		switch {
		case awaitingPropagation(req.Source, err):
			polls++
			delay = propagationDelay(polls, d.jitter())
			// Logged at every poll, with the reason, so an operator reading
			// CloudWatch sees a deliberate wait and what it is waiting for —
			// not an unexplained pause between two log lines.
			log.Warn("ecs call is not authorized yet; polling until the IAM policy propagates",
				"attempt", attempts,
				"poll", polls,
				"delay", delay.String(),
				"reason", "iam eventual consistency after the policy attachment; the poll is bounded by the invocation deadline and failing it fails the apply",
				"error", err)

		case Retryable(err):
			log.Error("deployment attempt failed", "attempt", attempts, "error", err)
			if retries >= d.cfg.MaxRetries {
				break loop
			}
			retries++
			delay = backoff(d.cfg.RetryBaseDelay, retries, d.jitter())
			log.Warn("retrying deployment",
				"attempt", attempts+1, "max_attempts", d.cfg.MaxRetries+1, "delay", delay.String())

		default:
			log.Error("deployment failed with a non-retryable error", "attempt", attempts, "error", err)
			return Result{}, err
		}

		if !d.fitsDeadline(ctx, delay) {
			log.Warn("stopping retries: not enough invocation time left",
				"attempt", attempts+1, "planned_delay", delay.String(), "polls", polls)
			break loop
		}
		if err := d.sleep(ctx, delay); err != nil {
			lastErr = fmt.Errorf("retry aborted: %w", err)
			break loop
		}
	}

	// Report what was actually tried, not the ceiling. The loop breaks early when
	// the remaining invocation time cannot fit the next backoff, and when the
	// context is cancelled mid-sleep — so MaxRetries+1 overstated the effort in
	// exactly the cases an operator is most likely to be reading the message.
	err := fmt.Errorf("deployment of %q failed after %d attempt(s): %w", req.ID, attempts, lastErr)

	// The condition is "this deployment engaged the propagation poll and still
	// did not go through", not "the last error happened to be AccessDenied".
	// Those differ in the case that matters most: the poll runs out of
	// invocation budget and lastErr is a cancelled sleep rather than the
	// refusal. Retryable(context.DeadlineExceeded) is false, so without the
	// wrap that outcome would be answered with `ignored` and a nil error — the
	// green apply with nothing deployed, which is the defect. A permanent error
	// cannot reach here after a poll: the default branch above returns it
	// immediately and unwrapped.
	if polls > 0 {
		err = fmt.Errorf("%w: %w", ErrPropagationTimeout, err)
	}
	return Result{}, err
}

// call makes the one AWS round trip an attempt consists of.
//
// req is a pointer because a successful RegisterRevisionWithImage is recorded
// on it — see the call site in attempt for why that matters.
func (d *Deployer) call(ctx context.Context, req *Request, target config.Target) (Result, error) {
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

	// Which revision this service is told to run, in strict precedence.
	//
	//  1. An explicit TaskDefinition. A manual pinned deploy is somebody naming
	//     a revision; registering anything would hand them a different one.
	//  2. An ImageURI. An ECR push knows the exact image it produced, and the
	//     only way to make a service run THAT image — and to leave behind a
	//     revision that still names it a month later — is to register one.
	//     Terraform's own revision cannot: it renders the container image as
	//     ":latest", so a rollback to it resolves to whatever ":latest" means at
	//     pull time.
	//  3. The bare family. A configuration deploy has no image to pin and does
	//     not need one: it happens because Terraform just registered the
	//     revision carrying the change, and the family resolves to exactly that
	//     revision. Registering a clone here would be actively wrong — the clone
	//     would copy container_definitions wholesale and could only ever restate
	//     what ECS is about to pick anyway.
	//
	// The revision registered in case 2 does not survive the next configuration
	// change: that path takes case 3, moving the service onto Terraform's
	// ":latest" revision. Accepted, and cheap to live with — the SHA-pinned
	// revisions stay ACTIVE and remain rollback targets, because Terraform
	// deregisters only the revisions it created and never sees these.
	taskDef := target.TaskFamily
	switch {
	case req.TaskDefinition != "":
		taskDef = req.TaskDefinition
	case req.ImageURI != "":
		arn, err := d.ecs.RegisterRevisionWithImage(ctx, target.TaskFamily, req.ImageURI)
		if err != nil {
			return Result{}, err
		}
		req.TaskDefinition = arn
		taskDef = arn
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
//
// This is the only bound on the permission-propagation poll, so it is also the
// thing that keeps that poll inside the Lambda's 60s timeout. It measures
// against d.now rather than time.Now so a test can drive the poll to exhaustion
// on a fake clock instead of racing a real one.
func (d *Deployer) fitsDeadline(ctx context.Context, delay time.Duration) bool {
	deadline, ok := ctx.Deadline()
	if !ok {
		return true
	}
	const callBudget = 2 * time.Second
	return deadline.Sub(d.now()) > delay+callBudget
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
