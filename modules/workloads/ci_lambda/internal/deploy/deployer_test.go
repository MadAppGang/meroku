package deploy

// These tests drive the real Deployer. The previous suite defined its own
// copy of the branching logic and asserted against the copy, so the retry
// policy, the notification behaviour and the error paths of the type that
// actually ships were never tested at all.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	smithy "github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"
	"madappgang.com/infrastructure/ci_lambda/internal/awsecs"
	"madappgang.com/infrastructure/ci_lambda/internal/config"
	"madappgang.com/infrastructure/ci_lambda/internal/slack"
	"madappgang.com/infrastructure/ci_lambda/internal/testsupport"
)

type fakeECS struct {
	updates   []awsecs.UpdateRequest
	updateOut awsecs.UpdateResult
	updateErr error

	registers []struct{ Family, Image string }
	registerA string
	registerE error
}

func (f *fakeECS) UpdateService(_ context.Context, req awsecs.UpdateRequest) (awsecs.UpdateResult, error) {
	f.updates = append(f.updates, req)
	if f.updateErr != nil {
		return awsecs.UpdateResult{}, f.updateErr
	}
	out := f.updateOut
	if out.ServiceName == "" {
		out.ServiceName = req.ServiceName
	}
	return out, nil
}

func (f *fakeECS) RegisterRevisionWithImage(_ context.Context, family, image string) (string, error) {
	f.registers = append(f.registers, struct{ Family, Image string }{family, image})
	return f.registerA, f.registerE
}

type recordingNotifier struct{ messages []slack.Message }

func (r *recordingNotifier) Notify(_ context.Context, m slack.Message) {
	r.messages = append(r.messages, m)
}

func (r *recordingNotifier) levels() []slack.Level {
	out := make([]slack.Level, 0, len(r.messages))
	for _, m := range r.messages {
		out = append(out, m.Level)
	}
	return out
}

// newTestDeployer wires the real Deployer with a fake clock so retry tests
// finish instantly and the delays are observable.
func newTestDeployer(t *testing.T, e ECS, n slack.Notifier, overrides map[string]string) (*Deployer, *[]time.Duration) {
	t.Helper()

	cfg := testsupport.Config(t, overrides)
	d := New(cfg, e, n, testsupport.Logger())

	var slept []time.Duration
	d.sleep = func(ctx context.Context, delay time.Duration) error {
		slept = append(slept, delay)
		return ctx.Err()
	}
	d.jitter = func() float64 { return 0.5 } // no jitter, deterministic delays
	return d, &slept
}

func TestUnknownIdentifierNeverReachesAWS(t *testing.T) {
	ecsFake := &fakeECS{}
	notifier := &recordingNotifier{}
	d, slept := newTestDeployer(t, ecsFake, notifier, nil)

	_, err := d.Deploy(context.Background(), Request{ID: "ghost", Source: SourceManual})

	require.ErrorIs(t, err, ErrUnknownTarget)
	require.False(t, Retryable(err), "a configuration error must not burn the retry budget")
	require.Empty(t, ecsFake.updates)
	require.Empty(t, ecsFake.registers)
	require.Empty(t, *slept)
	require.Empty(t, notifier.messages, "nothing is announced for a target that does not exist")
}

func TestServiceDeployPassesTheFamily(t *testing.T) {
	ecsFake := &fakeECS{updateOut: awsecs.UpdateResult{
		TaskDefinition: "arn:aws:ecs:us-east-1:000000000000:task-definition/acme_service_dev:11",
		DeploymentID:   "ecs-svc/new",
	}}
	notifier := &recordingNotifier{}
	d, _ := newTestDeployer(t, ecsFake, notifier, nil)

	res, err := d.Deploy(context.Background(), Request{ID: "backend", Source: SourceECR})
	require.NoError(t, err)

	require.Len(t, ecsFake.updates, 1)
	require.Equal(t, "acme_service_dev", ecsFake.updates[0].ServiceName)
	require.Equal(t, "acme_service_dev", ecsFake.updates[0].TaskDefinition, "the family, not a revision")
	require.True(t, ecsFake.updates[0].Force)

	require.Equal(t, "backend", res.ID)
	require.Equal(t, "ecs-svc/new", res.DeploymentID)
	require.Equal(t, config.KindService, res.Kind)

	require.Equal(t, []slack.Level{slack.LevelInfo, slack.LevelSuccess}, notifier.levels())
}

func TestManualDeployCanPinARevision(t *testing.T) {
	ecsFake := &fakeECS{}
	d, _ := newTestDeployer(t, ecsFake, &recordingNotifier{}, nil)

	_, err := d.Deploy(context.Background(), Request{
		ID:             "backend",
		TaskDefinition: "acme_service_dev:7",
		Source:         SourceManual,
	})
	require.NoError(t, err)
	require.Equal(t, "acme_service_dev:7", ecsFake.updates[0].TaskDefinition)
}

func TestScheduledTaskRegistersARevision(t *testing.T) {
	ecsFake := &fakeECS{registerA: "arn:aws:ecs:us-east-1:000000000000:task-definition/acme_task_cleanup_dev:12"}
	d, _ := newTestDeployer(t, ecsFake, &recordingNotifier{}, nil)

	image := "000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_task_cleanup:new"
	res, err := d.Deploy(context.Background(), Request{ID: "task:cleanup", ImageURI: image, Source: SourceECR})
	require.NoError(t, err)

	require.Empty(t, ecsFake.updates, "a scheduled task has no ECS service to update")
	require.Len(t, ecsFake.registers, 1)
	require.Equal(t, "acme_task_cleanup_dev", ecsFake.registers[0].Family)
	require.Equal(t, image, ecsFake.registers[0].Image)
	require.Equal(t, config.KindScheduledTask, res.Kind)
}

func TestScheduledTaskWithoutAnImageIsARequestError(t *testing.T) {
	ecsFake := &fakeECS{}
	d, slept := newTestDeployer(t, ecsFake, &recordingNotifier{}, nil)

	_, err := d.Deploy(context.Background(), Request{ID: "task:cleanup", Source: SourceManual})
	require.ErrorIs(t, err, ErrInvalidRequest)
	require.False(t, Retryable(err))
	require.Empty(t, ecsFake.registers)
	require.Empty(t, *slept)
}

func TestRetryableFailureIsRetriedWithGrowingDelays(t *testing.T) {
	ecsFake := &fakeECS{updateErr: &types.ServerException{}}
	notifier := &recordingNotifier{}
	d, slept := newTestDeployer(t, ecsFake, notifier, map[string]string{
		"MAX_DEPLOYMENT_RETRIES": "3",
		"RETRY_BASE_DELAY_MS":    "100",
	})

	_, err := d.Deploy(context.Background(), Request{ID: "backend", Source: SourceECR})
	require.Error(t, err)

	require.Len(t, ecsFake.updates, 4, "one attempt plus three retries")
	require.Equal(t, []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}, *slept,
		"genuinely exponential, not the linear sequence the old comment claimed")

	require.Equal(t, []slack.Level{slack.LevelInfo, slack.LevelError}, notifier.levels())
}

func TestNonRetryableFailureFailsOnTheFirstAttempt(t *testing.T) {
	ecsFake := &fakeECS{updateErr: &types.ServiceNotFoundException{}}
	d, slept := newTestDeployer(t, ecsFake, &recordingNotifier{}, nil)

	_, err := d.Deploy(context.Background(), Request{ID: "backend", Source: SourceECR})
	require.Error(t, err)
	require.Len(t, ecsFake.updates, 1)
	require.Empty(t, *slept)
}

func TestRetriesStopWhenTheInvocationDeadlineCannotFitAnother(t *testing.T) {
	ecsFake := &fakeECS{updateErr: &types.ServerException{}}
	d, slept := newTestDeployer(t, ecsFake, &recordingNotifier{}, map[string]string{
		"MAX_DEPLOYMENT_RETRIES": "5",
		"RETRY_BASE_DELAY_MS":    "5000",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := d.Deploy(ctx, Request{ID: "backend", Source: SourceECR})
	require.Error(t, err)
	require.Len(t, ecsFake.updates, 1)
	require.Empty(t, *slept, "no 5s sleep inside a 3s budget")
}

// TestSlackOutageDoesNotAffectTheDeployment uses the real notifier against a
// dead endpoint.
func TestSlackOutageDoesNotAffectTheDeployment(t *testing.T) {
	ecsFake := &fakeECS{updateOut: awsecs.UpdateResult{DeploymentID: "ecs-svc/new"}}
	dead := slack.New("http://127.0.0.1:1/hook", "dev", testsupport.Logger())
	d, _ := newTestDeployer(t, ecsFake, dead, nil)

	res, err := d.Deploy(context.Background(), Request{ID: "backend", Source: SourceECR})
	require.NoError(t, err)
	require.Equal(t, "ecs-svc/new", res.DeploymentID)
	require.Len(t, ecsFake.updates, 1)
}

func TestDeployAllDoesNotStopAtTheFirstFailure(t *testing.T) {
	ecsFake := &fakeECS{}
	d, _ := newTestDeployer(t, ecsFake, &recordingNotifier{}, nil)

	results, err := d.DeployAll(context.Background(), []Request{
		{ID: "api", Source: SourceS3},
		{ID: "ghost", Source: SourceS3},
		{ID: "payment-worker", Source: SourceS3},
	})

	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnknownTarget)
	require.Len(t, results, 2, "the two real targets are still deployed")
	require.Len(t, ecsFake.updates, 2)
	require.False(t, Retryable(err), "a joined error of only configuration errors is not retryable")
}

func TestDeployAllIsRetryableWhenAnyMemberIs(t *testing.T) {
	joined := errors.Join(
		fmt.Errorf("%w: %q", ErrUnknownTarget, "ghost"),
		&types.ServerException{},
	)
	require.True(t, Retryable(joined), "one transient member is reason enough to retry the invocation")

	onlyConfig := errors.Join(
		fmt.Errorf("%w: %q", ErrUnknownTarget, "ghost"),
		fmt.Errorf("%w: no image", ErrInvalidRequest),
	)
	require.False(t, Retryable(onlyConfig))
}

func TestBackoffIsExponentialAndCapped(t *testing.T) {
	base := time.Second

	require.Equal(t, time.Duration(0), backoff(base, 0, 0.5))
	require.Equal(t, 1*time.Second, backoff(base, 1, 0.5))
	require.Equal(t, 2*time.Second, backoff(base, 2, 0.5))
	require.Equal(t, 4*time.Second, backoff(base, 3, 0.5))
	require.Equal(t, maxBackoff, backoff(base, 10, 0.5), "a single sleep never eats the whole invocation")

	// Jitter stays inside +/-20%.
	require.Equal(t, 800*time.Millisecond, backoff(base, 1, 0))
	require.InDelta(t, float64(1200*time.Millisecond), float64(backoff(base, 1, 1)), float64(time.Millisecond))
}

func TestRetryableClassification(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unknown target", ErrUnknownTarget, false},
		{"invalid request", ErrInvalidRequest, false},
		{"context cancelled", context.Canceled, false},
		{"deadline exceeded", context.DeadlineExceeded, false},
		{"service not found", &types.ServiceNotFoundException{}, false},
		{"cluster not found", &types.ClusterNotFoundException{}, false},
		{"invalid parameter", &types.InvalidParameterException{}, false},
		{"client exception", &types.ClientException{}, false},
		{"access denied", &types.AccessDeniedException{}, false},
		{"server exception", &types.ServerException{}, true},
		{"throttling", &smithy.GenericAPIError{Code: "ThrottlingException", Fault: smithy.FaultClient}, true},
		{"modelled client error", &smithy.GenericAPIError{Code: "ValidationError", Fault: smithy.FaultClient}, false},
		{"network error", &net.DNSError{IsTimeout: true}, true},

		// awsecs raises these about facts that cannot change between attempts.
		{"awsecs permanent", awsecs.ErrPermanent, false},

		// The default. Retrying an error nothing above recognises bought one
		// more attempt and cost a Slack pair per attempt plus an EventBridge
		// redelivery of the whole event.
		{"unclassified", errors.New("something odd"), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, Retryable(c.err))
		})
	}
}

// TestPermanentECSErrorsAreNotRetryable is the H1 regression test.
//
// These are the errors awsecs raises itself. Each was a bare fmt.Errorf and so
// fell through to the unclassified default, which used to be `true`. The
// zero-container case is the one that bites in production: a scheduled task
// with docker_image pinned in YAML, a repository recreated in another registry,
// or a renamed app container hits it on every single push, forever.
func TestPermanentECSErrorsAreNotRetryable(t *testing.T) {
	ecsFake := &fakeECS{registerE: fmt.Errorf(
		"%w: ecs: no container in family %q uses repository %q; refusing to register an identical revision",
		awsecs.ErrPermanent, "acme_task_cleanup_dev", "acme_task_cleanup")}
	notifier := &recordingNotifier{}
	d, slept := newTestDeployer(t, ecsFake, notifier, map[string]string{"MAX_DEPLOYMENT_RETRIES": "3"})

	_, err := d.Deploy(context.Background(), Request{
		ID:       "task:cleanup",
		ImageURI: "000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_task_cleanup:v9",
		Source:   SourceECR,
	})

	require.Error(t, err)
	require.ErrorIs(t, err, awsecs.ErrPermanent)
	require.False(t, Retryable(err), "a permanent configuration fact must not be retried")
	require.Len(t, ecsFake.registers, 1, "one attempt, not four")
	require.Empty(t, *slept)
	require.Equal(t, []slack.Level{slack.LevelInfo, slack.LevelError}, notifier.levels(),
		"exactly one initiating post and one failure post")
}

// TestPermanentECSErrorReachesTheRealClient checks the wrapping at the source,
// not a hand-written copy of it: the real awsecs.Client must produce an error
// that deploy classifies as permanent.
func TestPermanentECSErrorFromTheRealClientIsNotRetryable(t *testing.T) {
	client := awsecs.New(nil, "acme_cluster_dev", false, testsupport.Logger())

	_, err := client.UpdateService(context.Background(), awsecs.UpdateRequest{})
	require.ErrorIs(t, err, awsecs.ErrPermanent)
	require.False(t, Retryable(err))

	_, err = client.RegisterRevisionWithImage(context.Background(), "", "img")
	require.ErrorIs(t, err, awsecs.ErrPermanent)
	require.False(t, Retryable(err))

	_, err = client.RegisterRevisionWithImage(context.Background(), "fam", "")
	require.ErrorIs(t, err, awsecs.ErrPermanent)
	require.False(t, Retryable(err))
}

func TestRetryableSeesThroughWrapping(t *testing.T) {
	wrapped := fmt.Errorf("ecs: update service: %w", &types.ServerException{})
	require.True(t, Retryable(wrapped))

	var typed error = &types.ServiceNotFoundException{}
	require.False(t, Retryable(errors.Join(typed)))

	permanent := fmt.Errorf("deployment of %q failed: %w", "task:cleanup",
		fmt.Errorf("%w: no container matched", awsecs.ErrPermanent))
	require.False(t, Retryable(permanent), "%%w keeps the sentinel reachable through every wrap")
}
