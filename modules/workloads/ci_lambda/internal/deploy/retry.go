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
