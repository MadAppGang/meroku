package deploy

// The IAM permission-propagation poll, and the scoping that keeps it off every
// other path.
//
// The race these tests pin, from the run that produced the code: on the first
// `terraform apply` of a new environment,
// aws_iam_role_policy_attachment.lambda_ecs completed at 06:01:17.0126 and the
// three aws_lambda_invocation.{backend,services}_revision resources ran at
// 06:01:23.38 — 6.4s later — and all three got
//
//	AccessDeniedException: User: .../scratchpl_ci_lambda_dev is not authorized
//	to perform: ecs:UpdateService ... because no identity-based policy allows
//	the ecs:UpdateService action
//
// against a policy that was correct and that worked on every later invocation.
// retry.go classified that as permanent, handler.deployOne answered `ignored`
// with a NIL error, and the apply went GREEN with the repair deployment never
// performed and nothing anywhere surfacing it.

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	smithy "github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"
	"madappgang.com/infrastructure/ci_lambda/internal/awsecs"
	"madappgang.com/infrastructure/ci_lambda/internal/slack"
)

// accessDenied is the refusal as the ECS SDK models it for UpdateService,
// message and all.
func accessDenied() error {
	return &types.AccessDeniedException{
		Message: aws.String("User: arn:aws:sts::000000000000:assumed-role/acme_ci_lambda_dev/x " +
			"is not authorized to perform: ecs:UpdateService on resource: " +
			"arn:aws:ecs:us-east-1:000000000000:service/acme_cluster_dev/acme_service_dev " +
			"because no identity-based policy allows the ecs:UpdateService action"),
	}
}

// TestTerraformDeployPollsThroughAnIAMPropagationRace is the primary fix.
//
// Two refusals then success, and the point is the third call. The retry budget
// is left at its production value (MAX_DEPLOYMENT_RETRIES=2 over a 1s base, so
// ~3s in total), which is why merely reclassifying AccessDenied as retryable
// would not have fixed the real incident — it was still refusing at 6.4s. What
// makes this pass is the separate, deadline-bounded poll.
func TestTerraformDeployPollsThroughAnIAMPropagationRace(t *testing.T) {
	ecsFake := &fakeECS{
		updateErr:  accessDenied(),
		updateErrN: 2,
		updateOut:  awsecs.UpdateResult{DeploymentID: "ecs-svc/new"},
	}
	notifier := &recordingNotifier{}
	d, slept := newTestDeployer(t, ecsFake, notifier, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	res, err := d.Deploy(ctx, Request{ID: "backend", Source: SourceTerraform, Reason: "revision 4"})

	require.NoError(t, err, "the permission arrived; the deployment must go through")
	require.Equal(t, "ecs-svc/new", res.DeploymentID)
	require.Greater(t, len(ecsFake.updates), 1,
		"a single UpdateService call means no poll happened and the race is still live")
	require.Len(t, ecsFake.updates, 3, "two refusals waited out, the third call succeeded")
	require.Equal(t, []time.Duration{time.Second, 2 * time.Second}, *slept,
		"the poll ramps rather than sleeping a guessed constant")
	require.Equal(t, []slack.Level{slack.LevelInfo, slack.LevelSuccess}, notifier.levels(),
		"a race that resolves is a successful deployment, not a failure anyone hears about")
}

// TestTerraformDeployHasNoOverheadWhenThePermissionIsAlreadyInEffect is the
// reason this is a poll and not a time_sleep. Every apply of an environment
// that already exists — the overwhelmingly common case — must pay nothing.
func TestTerraformDeployHasNoOverheadWhenThePermissionIsAlreadyInEffect(t *testing.T) {
	ecsFake := &fakeECS{updateOut: awsecs.UpdateResult{DeploymentID: "ecs-svc/new"}}
	d, slept := newTestDeployer(t, ecsFake, &recordingNotifier{}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, err := d.Deploy(ctx, Request{ID: "backend", Source: SourceTerraform})
	require.NoError(t, err)
	require.Len(t, ecsFake.updates, 1)
	require.Empty(t, *slept, "not one second of added latency on the common path")
}

// TestTerraformDeployFailsTheApplyWhenPermissionNeverArrives removes the
// silence, which is the larger half of the defect.
//
// A poll that gives up must produce an error deploy.Retryable calls retryable,
// because that is the only thing that makes handler.deployOne return a non-nil
// error, the Lambda report a FunctionError, and the `terraform apply` FAIL. A
// genuine misconfiguration then fails the apply with the AccessDenied message
// attached instead of the green no-op the old classification produced.
func TestTerraformDeployFailsTheApplyWhenPermissionNeverArrives(t *testing.T) {
	ecsFake := &fakeECS{updateErr: accessDenied()}
	notifier := &recordingNotifier{}
	d, slept := newTestDeployer(t, ecsFake, notifier, nil)

	const budget = 60 * time.Second // aws_lambda_function.lambda_deploy timeout
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()

	_, err := d.Deploy(ctx, Request{ID: "backend", Source: SourceTerraform})

	require.Error(t, err)
	require.ErrorIs(t, err, ErrPropagationTimeout)
	require.True(t, Retryable(err),
		"an exhausted poll MUST be retryable: that is what turns it into a non-nil error out of "+
			"deployOne and fails the apply. Report it permanent and the apply goes green with "+
			"nothing deployed, which is the defect this fixes")

	var denied *types.AccessDeniedException
	require.ErrorAs(t, err, &denied, "the real reason must survive the wrap")
	require.Contains(t, err.Error(), "ecs:UpdateService",
		"the operator reading the failed apply needs the AWS message, not a sentinel")

	// Bounded by the invocation deadline, not by a constant.
	var total time.Duration
	for _, s := range *slept {
		total += s
	}
	require.Less(t, total, budget, "the poll must never sleep past the invocation deadline")
	require.LessOrEqual(t, total, budget-2*time.Second, "and must leave fitsDeadline's call budget")
	require.Greater(t, total, 45*time.Second, "but must use the budget it has, not give up early")
	require.Len(t, ecsFake.updates, len(*slept)+1, "one call per wait, plus the first")

	require.Equal(t, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}, (*slept)[:3],
		"1s, 2s, 4s, then hold")
	for _, s := range (*slept)[3:] {
		require.Equal(t, 4*time.Second, s, "held at 4s, so a 60s budget is ~15 ECS calls not ~55")
	}

	require.Equal(t, []slack.Level{slack.LevelInfo, slack.LevelError}, notifier.levels(),
		"one initiating post and one failure post; the invocation is synchronous, so nothing "+
			"redelivers it and there is no storm to cause")
}

// TestNonTerraformSourcesStillTreatAccessDeniedAsPermanent is the regression
// guard for the scoping, and it is not optional.
//
// ECR, SSM and S3 are invoked ASYNCHRONOUSLY by EventBridge. A retryable verdict
// there escapes the handler as an invocation error, EventBridge redelivers the
// event, and every attempt posts DEPLOYMENT_INITIATING plus DEPLOYMENT_FAILED to
// Slack — so a condition that will never clear becomes an unbounded storm. That
// is precisely the behaviour the comments on awsecs.ErrPermanent and on
// Retryable's unclassified default exist to remove. SourceManual is in the list
// for a second reason: the generated GitHub Actions workflows and the web UI's
// deploy button route through it, and a human clicking deploy against a
// genuinely broken policy should be told so in a second, not after 45s.
//
// MUTATION: delete the `src != SourceTerraform` guard in awaitingPropagation and
// every subtest here fails — sixteen UpdateService calls instead of one, and the
// error comes back retryable.
func TestNonTerraformSourcesStillTreatAccessDeniedAsPermanent(t *testing.T) {
	for _, src := range []Source{SourceECR, SourceSSM, SourceS3, SourceManual} {
		t.Run(string(src), func(t *testing.T) {
			ecsFake := &fakeECS{updateErr: accessDenied()}
			notifier := &recordingNotifier{}
			d, slept := newTestDeployer(t, ecsFake, notifier, nil)

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			_, err := d.Deploy(ctx, Request{ID: "backend", Source: src})

			require.Error(t, err)
			require.Len(t, ecsFake.updates, 1, "one attempt, no poll")
			require.Empty(t, *slept, "not one second of waiting")
			require.False(t, Retryable(err),
				"AccessDenied outside the Terraform path stays permanent; retrying it is the "+
					"Slack storm awsecs.ErrPermanent exists to prevent")
			require.NotErrorIs(t, err, ErrPropagationTimeout)
			require.Equal(t, []slack.Level{slack.LevelInfo, slack.LevelError}, notifier.levels(),
				"one initiating post and one failure post per event — not sixteen invocations' worth")
		})
	}
}

// TestScheduledTaskPropagationUsesTheUnmodelledErrorShape covers the other API.
//
// RegisterTaskDefinition does not model AccessDeniedException — its error shapes
// are ServerException, ClientException and InvalidParameterException — so the
// identical AWS refusal arrives as an unmodelled smithy.GenericAPIError carrying
// the code as a string. Matching only *types.AccessDeniedException would make
// the scheduled-task path behave differently for the same condition.
func TestScheduledTaskPropagationUsesTheUnmodelledErrorShape(t *testing.T) {
	ecsFake := &fakeECS{registerE: &smithy.GenericAPIError{
		Code:    "AccessDeniedException",
		Message: "is not authorized to perform: ecs:RegisterTaskDefinition",
		Fault:   smithy.FaultClient,
	}}
	d, slept := newTestDeployer(t, ecsFake, &recordingNotifier{}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	_, err := d.Deploy(ctx, Request{
		ID:       "task:cleanup",
		ImageURI: "000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_task_cleanup:v9",
		Source:   SourceTerraform,
	})

	require.ErrorIs(t, err, ErrPropagationTimeout)
	require.Greater(t, len(ecsFake.registers), 1, "the unmodelled shape must poll too")
	require.NotEmpty(t, *slept)
}

// TestPropagationPredicateIsScopedAndShapeAware is the unit test for the
// predicate: the source half and the error half, separately.
func TestPropagationPredicateIsScopedAndShapeAware(t *testing.T) {
	every := []Source{SourceECR, SourceSSM, SourceS3, SourceManual, SourceTerraform}

	denials := []struct {
		name string
		err  error
	}{
		{"modelled AccessDeniedException", accessDenied()},
		{"wrapped", fmt.Errorf("ecs: update service %q: %w", "acme_service_dev", accessDenied())},
		{"unmodelled AccessDeniedException", &smithy.GenericAPIError{Code: "AccessDeniedException", Fault: smithy.FaultClient}},
		{"AccessDenied", &smithy.GenericAPIError{Code: "AccessDenied", Fault: smithy.FaultClient}},
		{"UnauthorizedOperation", &smithy.GenericAPIError{Code: "UnauthorizedOperation", Fault: smithy.FaultClient}},
	}
	for _, c := range denials {
		for _, src := range every {
			t.Run(c.name+"/"+string(src), func(t *testing.T) {
				require.Equal(t, src == SourceTerraform, awaitingPropagation(src, c.err),
					"only a synchronous aws_lambda_invocation may wait out a propagation race")
			})
		}
	}

	// Not a propagation race, whatever the source. These must keep failing fast
	// on the Terraform path too, or a real misconfiguration costs a whole
	// invocation before the apply is told what is wrong.
	others := []struct {
		name string
		err  error
	}{
		{"nil", nil},
		{"service not found", &types.ServiceNotFoundException{}},
		{"invalid parameter", &types.InvalidParameterException{}},
		{"server exception", &types.ServerException{}},
		{"throttling", &smithy.GenericAPIError{Code: "ThrottlingException", Fault: smithy.FaultClient}},
		{"unknown target", ErrUnknownTarget},
		{"awsecs permanent", awsecs.ErrPermanent},
		{"unclassified", errors.New("something odd")},
	}
	for _, c := range others {
		t.Run("not a denial/"+c.name, func(t *testing.T) {
			require.False(t, awaitingPropagation(SourceTerraform, c.err))
		})
	}
}

func TestPropagationDelayRampsThenHolds(t *testing.T) {
	require.Equal(t, 1*time.Second, propagationDelay(1, 0.5))
	require.Equal(t, 2*time.Second, propagationDelay(2, 0.5))
	require.Equal(t, 4*time.Second, propagationDelay(3, 0.5))
	require.Equal(t, 4*time.Second, propagationDelay(4, 0.5), "held, not doubled")
	require.Equal(t, 4*time.Second, propagationDelay(99, 0.5))

	// Jittered, because one apply fires the backend invocation and every
	// service invocation at once and they must not poll in lockstep.
	require.Equal(t, 800*time.Millisecond, propagationDelay(1, 0))
	require.InDelta(t, float64(1200*time.Millisecond), float64(propagationDelay(1, 1)), float64(time.Millisecond))
}

// TestPropagationTimeoutIsRetryableThroughItsCause guards the ORDERING inside
// Retryable. The sentinel is joined to the very AccessDeniedException the poll
// gave up on, so a check placed after the AccessDenied case would answer
// "permanent" and restore the green-apply silence.
func TestPropagationTimeoutIsRetryableThroughItsCause(t *testing.T) {
	require.True(t, Retryable(ErrPropagationTimeout))

	joined := fmt.Errorf("%w: deployment of %q failed after 16 attempt(s): %w",
		ErrPropagationTimeout, "backend", accessDenied())
	require.True(t, Retryable(joined),
		"the sentinel must win over the AccessDeniedException it wraps")

	aborted := fmt.Errorf("%w: retry aborted: %w", ErrPropagationTimeout, context.DeadlineExceeded)
	require.True(t, Retryable(aborted),
		"a poll cut short by the deadline is still a deployment that did not happen")
}

// TestTerraformSourceKeepsTheOrdinaryRetryBudgetForEverythingElse keeps the two
// budgets separate.
//
// The Terraform path is allowed to poll a propagation race until the deadline;
// it is NOT allowed to turn every other failure into a 55-second wait. A server
// fault on that path still gets exactly MAX_DEPLOYMENT_RETRIES and no
// propagation verdict, so nothing about ordinary transient handling changed
// because the source did.
func TestTerraformSourceKeepsTheOrdinaryRetryBudgetForEverythingElse(t *testing.T) {
	ecsFake := &fakeECS{updateErr: &types.ServerException{}}
	d, slept := newTestDeployer(t, ecsFake, &recordingNotifier{}, map[string]string{
		"MAX_DEPLOYMENT_RETRIES": "2",
		"RETRY_BASE_DELAY_MS":    "1000",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, err := d.Deploy(ctx, Request{ID: "backend", Source: SourceTerraform})

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrPropagationTimeout, "no poll happened, so no propagation verdict")
	require.Len(t, ecsFake.updates, 3, "one attempt plus MAX_DEPLOYMENT_RETRIES, unchanged")
	require.Equal(t, []time.Duration{time.Second, 2 * time.Second}, *slept)
}
