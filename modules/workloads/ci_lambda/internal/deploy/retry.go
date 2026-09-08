package deploy

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"net"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	smithy "github.com/aws/smithy-go"
	"madappgang.com/infrastructure/ci_lambda/internal/awsecs"
)

// jitterFraction is the +/- proportion applied to each backoff delay so that a
// burst of events does not retry in lockstep.
const jitterFraction = 0.2

// maxBackoff caps a single sleep. The whole invocation budget is 60s.
const maxBackoff = 10 * time.Second

// backoff returns the delay before the given retry attempt. attempt is 1 for
// the first retry.
//
// This is genuinely exponential: base * 2^(attempt-1), then jitter. The
// previous implementation multiplied linearly while the comment claimed
// exponential.
func backoff(base time.Duration, attempt int, jitter float64) time.Duration {
	if base <= 0 || attempt < 1 {
		return 0
	}
	d := float64(base) * math.Pow(2, float64(attempt-1))
	if d > float64(maxBackoff) {
		d = float64(maxBackoff)
	}
	// jitter is in [0,1); map it to [-jitterFraction, +jitterFraction).
	d *= 1 + jitterFraction*(2*jitter-1)
	if d < 0 {
		return 0
	}
	return time.Duration(d)
}

// sleepCtx waits for d, or returns early if the context is done.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func defaultJitter() float64 { return rand.Float64() } //nolint:gosec // jitter, not cryptography

// propagationPollInterval is the first wait of the permission-propagation poll
// (see awaitingPropagation). It is short on purpose: the poll adds ZERO latency
// when the permission is already in effect — the first ECS call succeeds and no
// poll happens at all — so the only thing this number trades is how quickly a
// racing apply notices that IAM has caught up.
const propagationPollInterval = time.Second

// propagationPollRamp is the poll after which the delay stops growing.
const propagationPollRamp = 3

// propagationDelay returns the wait before the given permission-propagation
// poll. poll is 1 for the first one.
//
// It ramps 1s, 2s, 4s and then holds at 4s. The ramp covers the observed race —
// the refusal recorded in awaitingPropagation was still live 6.4s after the
// policy attachment and had cleared by the next invocation — while the flat
// tail keeps a 60s invocation to roughly fifteen ECS calls rather than fifty-
// five. It reuses backoff() so the jitter and the maxBackoff cap are the same
// ones the ordinary retry path uses; the jitter matters here because a single
// apply fires the backend invocation and every service invocation at once, and
// three Lambdas polling in lockstep is three times the throttling risk for no
// gain.
func propagationDelay(poll int, jitter float64) time.Duration {
	if poll > propagationPollRamp {
		poll = propagationPollRamp
	}
	return backoff(propagationPollInterval, poll, jitter)
}

// ErrPropagationTimeout means a deployment was refused for want of an IAM
// permission until the invocation budget ran out.
//
// It exists to make that outcome LOUD, and that is the larger half of the fix.
// Retryable reports true for it, so handler.deployOne returns a non-nil error,
// the Lambda reports a FunctionError, and the `terraform apply` that invoked it
// FAILS with the AccessDenied message attached. Without this sentinel the
// exhausted poll would unwrap to an AccessDeniedException, which Retryable
// calls permanent, which deployOne answers with `ignored` and a NIL error — so
// the apply went green while the repair deployment silently did not happen and
// nothing anywhere surfaced it. A genuine misconfiguration now fails the apply
// with the real reason instead of a green no-op.
var ErrPropagationTimeout = errors.New("iam permission did not propagate within the invocation budget")

// awaitingPropagation reports whether err is an authorization refusal on the one
// path where waiting for it to clear is the right answer.
//
// THE DEFECT. On the first `terraform apply` of a new environment,
// aws_iam_role_policy_attachment.lambda_ecs completed at 06:01:17.0126 and the
// three aws_lambda_invocation.{backend,services}_revision resources ran at
// 06:01:23.38 — 6.4 seconds later. IAM had not propagated that far, and all
// three answered:
//
//	AccessDeniedException: User: .../scratchpl_ci_lambda_dev is not authorized
//	to perform: ecs:UpdateService ... because no identity-based policy allows
//	the ecs:UpdateService action
//
// The policy was CORRECT — every later invocation succeeded. This is pure IAM
// eventual consistency, and the only thing that resolves it is time. Merely
// reclassifying AccessDenied as retryable does not help: MAX_DEPLOYMENT_RETRIES
// is 2 over a 1s base, so the whole ordinary budget is ~3s and would have been
// spent before the 6.4s mark. Hence a poll bounded by the invocation deadline
// rather than by a retry count, and a poll rather than a fixed sleep because the
// overwhelmingly common case — every apply of an environment that already exists
// — must pay nothing at all.
//
// WHY THIS IS SCOPED TO SourceTerraform, AND NOT TO AccessDenied EVERYWHERE.
// AccessDenied is classified permanent in Retryable for a stated reason, the
// same one recorded on awsecs.ErrPermanent: the ECR, SSM and S3 paths are
// invoked ASYNCHRONOUSLY by EventBridge, so a retryable verdict escapes the
// handler as an invocation error, EventBridge redelivers the event, and a
// condition that will never clear posts DEPLOYMENT_INITIATING plus
// DEPLOYMENT_FAILED to Slack on every attempt of every redelivery, forever.
// Widening this predicate to every source reintroduces exactly that storm, and
// adds up to 45s of dead time to each round of it.
//
// SourceManual would be too wide for a second reason. The generated GitHub
// Actions workflows emit DEPLOY / SERVICE_DEPLOY too, and they are routed
// SourceManual — a human who clicks deploy against a genuinely broken policy
// should be told so in a second or two, not after the Lambda has spent its
// whole 60s budget confirming it. So the discriminator is narrower than the
// detail-type: handler.manual promotes a request to SourceTerraform only when
// the event source is handler.TerraformInvocationSource(env) —
// "terraform.{env}" — which no EventBridge rule in lambda.tf accepts
// (local.ci_manual_sources_scoped and _global list only action.* and
// github.actions.*). An event carrying it therefore arrived by direct
// RequestResponse Invoke, which is aws_lambda_invocation and nothing else: a
// synchronous caller that is already blocked waiting, with no redelivery behind
// it and no Slack storm to cause. internal/boundary pins that source string in
// backend.tf and services.tf, because a silent rename there would silently
// switch this whole path back off.
func awaitingPropagation(src Source, err error) bool {
	if src != SourceTerraform || err == nil {
		return false
	}

	// UpdateService models AccessDeniedException, so an authorization refusal on
	// the service path deserialises into the typed error.
	var accessDenied *types.AccessDeniedException
	if errors.As(err, &accessDenied) {
		return true
	}

	// RegisterTaskDefinition does NOT model it — its error shapes are
	// ServerException, ClientException and InvalidParameterException — so the
	// same refusal arrives there as an unmodelled smithy.GenericAPIError
	// carrying the code as a string. Matching both is what stops the scheduled
	// task path from behaving differently for the identical AWS condition.
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "AccessDeniedException", "AccessDenied", "UnauthorizedOperation":
			return true
		}
	}
	return false
}

// Retryable reports whether retrying err could plausibly succeed.
//
// Configuration errors are the important half: an unknown identifier, a
// deleted service or a malformed parameter will fail identically on every
// attempt, and burning the whole retry budget on them is what turned one
// mis-keyed map into three identical failures in the logs.
func Retryable(err error) bool {
	if err == nil {
		return false
	}

	// A joined error (DeployAll over a fan-out) is retryable if any one of its
	// members is.
	if multi, ok := err.(interface{ Unwrap() []error }); ok { //nolint:errorlint // deliberate: top-level join only
		for _, e := range multi.Unwrap() {
			if Retryable(e) {
				return true
			}
		}
		return false
	}

	// An exhausted permission-propagation poll is retryable, and this test has
	// to come BEFORE the AccessDenied case below — the sentinel is joined to
	// the very AccessDeniedException that would otherwise be read as permanent,
	// so whichever check runs first decides. Retryable here does not mean
	// "attempt it again inside this invocation"; the poll already did that until
	// the deadline. It means "do not swallow this": deployOne returns the error,
	// the Lambda reports a FunctionError, and the apply fails loudly instead of
	// going green with nothing deployed. See ErrPropagationTimeout, and note
	// that only SourceTerraform — a synchronous aws_lambda_invocation, never an
	// EventBridge delivery — can produce it, so no redelivery follows.
	if errors.Is(err, ErrPropagationTimeout) {
		return true
	}

	// Our own request/configuration errors never become retries. awsecs.
	// ErrPermanent is the same class raised one layer down: a family whose
	// containers do not use the pushed repository, a missing service name, an
	// empty API response. None of those change between two attempts.
	if errors.Is(err, ErrUnknownTarget) || errors.Is(err, ErrInvalidRequest) || errors.Is(err, awsecs.ErrPermanent) {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Definitively non-retryable ECS errors.
	var (
		serviceNotFound *types.ServiceNotFoundException
		clusterNotFound *types.ClusterNotFoundException
		invalidParam    *types.InvalidParameterException
		clientErr       *types.ClientException
		accessDenied    *types.AccessDeniedException
		platformUnknown *types.PlatformUnknownException
		unsupported     *types.UnsupportedFeatureException
	)
	switch {
	case errors.As(err, &serviceNotFound),
		errors.As(err, &clusterNotFound),
		errors.As(err, &invalidParam),
		errors.As(err, &clientErr),
		errors.As(err, &accessDenied),
		errors.As(err, &platformUnknown),
		errors.As(err, &unsupported):
		return false
	}

	// Definitively retryable ECS errors.
	var serverErr *types.ServerException
	if errors.As(err, &serverErr) {
		return true
	}

	// Everything else the SDK models: server faults and the throttling family
	// are worth another attempt, other modelled errors are client problems.
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		if apiErr.ErrorFault() == smithy.FaultServer {
			return true
		}
		switch apiErr.ErrorCode() {
		case "ThrottlingException", "Throttling", "TooManyRequestsException",
			"RequestLimitExceeded", "RequestThrottled", "ServiceUnavailable",
			"InternalFailure", "InternalError", "RequestTimeout":
			return true
		}
		return false
	}

	// Network-level faults (dial timeouts, resets) are worth another attempt.
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// Unclassified: do NOT retry. This default is deliberate and it is the
	// opposite of what it used to be.
	//
	// Everything that can plausibly succeed on a second attempt is already
	// classified above: net.Error, smithy server faults, the throttling family
	// and types.ServerException. Below the SDK there is a second retryer —
	// aws-sdk-go-v2 retries transient transport and throttling failures inside
	// a single call before we ever see the error — so an error that reaches
	// here has already survived that.
	//
	// What actually reached here in practice was this module's own errors, and
	// retrying them is not free: each attempt re-posts DEPLOYMENT_INITIATING
	// and DEPLOYMENT_FAILED to Slack, and a retryable verdict propagates out of
	// the handler as an invocation error, which makes EventBridge redeliver the
	// event and repeat the whole thing. Three attempts inside three invocations
	// is six Slack posts for one push on a condition that will never clear.
	// That retry storm is the failure this rewrite exists to remove.
	//
	// The cost of being wrong in this direction is one missed deployment,
	// logged at ERROR with the full error and reported as `ignored`. The cost
	// of being wrong in the other direction is the storm. Prefer the former,
	// and classify the error explicitly above when a new case turns up.
	return false
}
