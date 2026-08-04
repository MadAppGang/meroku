package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/stretchr/testify/require"
	"madappgang.com/infrastructure/ci_lambda/internal/config"
	"madappgang.com/infrastructure/ci_lambda/internal/deploy"
	"madappgang.com/infrastructure/ci_lambda/internal/handler"
	"madappgang.com/infrastructure/ci_lambda/internal/slack"
	"madappgang.com/infrastructure/ci_lambda/internal/testsupport"
)

// fakeDeployer resolves nothing itself: it answers from the same Config the
// handler uses, so a test cannot accidentally accept an identifier that
// production would reject.
type fakeDeployer struct {
	cfg  *config.Config
	seen []deploy.Request
	err  error
}

func (f *fakeDeployer) Deploy(_ context.Context, req deploy.Request) (deploy.Result, error) {
	f.seen = append(f.seen, req)
	if f.err != nil {
		return deploy.Result{}, f.err
	}
	target, ok := f.cfg.Target(req.ID)
	if !ok {
		return deploy.Result{}, errors.New("test bug: handler asked to deploy an unmapped identifier " + req.ID)
	}
	return deploy.Result{ID: req.ID, ServiceName: target.ServiceName, Kind: target.Kind}, nil
}

func (f *fakeDeployer) DeployAll(ctx context.Context, reqs []deploy.Request) ([]deploy.Result, error) {
	var (
		results []deploy.Result
		errs    []error
	)
	for _, r := range reqs {
		res, err := f.Deploy(ctx, r)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		results = append(results, res)
	}
	return results, errors.Join(errs...)
}

func (f *fakeDeployer) ids() []string {
	out := make([]string, 0, len(f.seen))
	for _, r := range f.seen {
		out = append(out, r.ID)
	}
	return out
}

type nopNotifier struct{ count int }

func (n *nopNotifier) Notify(context.Context, slack.Message) { n.count++ }

func newHandler(t *testing.T, overrides map[string]string) (*handler.Handler, *fakeDeployer, *nopNotifier) {
	t.Helper()
	cfg := testsupport.Config(t, overrides)
	dep := &fakeDeployer{cfg: cfg}
	notifier := &nopNotifier{}
	return handler.New(cfg, dep, notifier, testsupport.Logger()), dep, notifier
}

func event(source, detailType string, detail any) events.CloudWatchEvent {
	raw, err := json.Marshal(detail)
	if err != nil {
		panic(err)
	}
	return events.CloudWatchEvent{
		ID:         "00000000-0000-0000-0000-000000000000",
		Source:     source,
		DetailType: detailType,
		AccountID:  "000000000000",
		Region:     "us-east-1",
		Detail:     raw,
	}
}

// ---------------------------------------------------------------- ECR

func TestECRPushDeploysTheBackend(t *testing.T) {
	h, dep, _ := newHandler(t, nil)

	res, err := h.Handle(context.Background(), event("aws.ecr", "ECR Image Action", map[string]string{
		"repository-name": "acme_backend",
		"image-tag":       "abc123",
		"action-type":     "PUSH",
		"result":          "SUCCESS",
	}))

	require.NoError(t, err)
	require.Equal(t, handler.StatusDeployed, res.Status)
	require.Equal(t, []string{"backend"}, res.Deployed)
	require.Equal(t, []string{"backend"}, dep.ids())
}

func TestECRPushFansOutToEveryConsumerOfTheRepository(t *testing.T) {
	h, dep, _ := newHandler(t, nil)

	res, err := h.Handle(context.Background(), event("aws.ecr", "ECR Image Action", map[string]string{
		"repository-name": "acme_service_api",
		"image-tag":       "abc123",
		"action-type":     "PUSH",
		"result":          "SUCCESS",
	}))

	require.NoError(t, err)
	require.ElementsMatch(t, []string{"api", "reporting"}, res.Deployed,
		"one repository shared by two services deploys both")
	require.Len(t, dep.seen, 2)
}

func TestECRPushToAScheduledTaskCarriesTheImageURI(t *testing.T) {
	h, dep, _ := newHandler(t, nil)

	_, err := h.Handle(context.Background(), event("aws.ecr", "ECR Image Action", map[string]string{
		"repository-name": "acme_task_cleanup",
		"image-tag":       "v9",
		"action-type":     "PUSH",
		"result":          "SUCCESS",
	}))

	require.NoError(t, err)
	require.Len(t, dep.seen, 1)
	require.Equal(t, "task:cleanup", dep.seen[0].ID)
	require.Equal(t,
		"000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_task_cleanup:v9",
		dep.seen[0].ImageURI)
}

func TestECRPushUsesTheDigestWhenThereIsNoTag(t *testing.T) {
	h, dep, _ := newHandler(t, nil)

	digest := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	_, err := h.Handle(context.Background(), event("aws.ecr", "ECR Image Action", map[string]string{
		"repository-name": "acme_task_cleanup",
		"image-digest":    digest,
		"action-type":     "PUSH",
		"result":          "SUCCESS",
	}))

	require.NoError(t, err)
	require.Equal(t,
		"000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_task_cleanup@"+digest,
		dep.seen[0].ImageURI)
}

// TestECRPushFromAnotherProjectIsIgnored is the cross-project isolation
// regression test. The old code parsed the repository name with an unanchored,
// project-blind regex and happily deployed.
func TestECRPushFromAnotherProjectIsIgnored(t *testing.T) {
	h, dep, _ := newHandler(t, nil)

	res, err := h.Handle(context.Background(), event("aws.ecr", "ECR Image Action", map[string]string{
		"repository-name": "otherproj_service_api",
		"image-tag":       "abc123",
		"action-type":     "PUSH",
		"result":          "SUCCESS",
	}))

	require.NoError(t, err, "another project's push is not a failure of ours")
	require.Equal(t, handler.StatusIgnored, res.Status)
	require.Empty(t, dep.seen)
}

func TestECRNonPushEventsAreIgnored(t *testing.T) {
	h, dep, _ := newHandler(t, nil)

	for _, detail := range []map[string]string{
		{"repository-name": "acme_backend", "action-type": "DELETE", "result": "SUCCESS"},
		{"repository-name": "acme_backend", "action-type": "PUSH", "result": "FAILURE"},
	} {
		res, err := h.Handle(context.Background(), event("aws.ecr", "ECR Image Action", detail))
		require.NoError(t, err)
		require.Equal(t, handler.StatusIgnored, res.Status)
	}
	require.Empty(t, dep.seen)
}

func TestECRMonitoringCanBeDisabled(t *testing.T) {
	h, dep, _ := newHandler(t, map[string]string{"ENABLE_ECR_MONITORING": "false"})

	res, err := h.Handle(context.Background(), event("aws.ecr", "ECR Image Action", map[string]string{
		"repository-name": "acme_backend", "action-type": "PUSH", "result": "SUCCESS",
	}))
	require.NoError(t, err)
	require.Equal(t, handler.StatusIgnored, res.Status)
	require.Empty(t, dep.seen)
}

// ---------------------------------------------------------------- SSM

func TestSSMUpdateDeploysTheRightTarget(t *testing.T) {
	// payment-worker is deliberately absent: the fixture marks it
	// auto_deploy = false, and TestAutoDeployDisabledIsReportedByName covers it.
	cases := map[string]string{
		"/dev/acme/backend/env":    "backend",
		"/dev/acme/legacy-api/env": "legacy-api",
		"/dev/acme/task/env":       "task",
	}

	for path, want := range cases {
		h, dep, _ := newHandler(t, nil)
		res, err := h.Handle(context.Background(), event("aws.ssm", "Parameter Store Change", map[string]string{
			"operation": "Update", "name": path, "type": "SecureString",
		}))
		require.NoErrorf(t, err, "path %s", path)
		require.Equalf(t, handler.StatusDeployed, res.Status, "path %s", path)
		require.Equalf(t, []string{want}, dep.ids(), "path %s", path)
	}
}

func TestSSMNonUpdateOperationsAreIgnored(t *testing.T) {
	for _, op := range []string{"Create", "Delete", "LabelParameterVersion"} {
		h, dep, _ := newHandler(t, nil)
		res, err := h.Handle(context.Background(), event("aws.ssm", "Parameter Store Change", map[string]string{
			"operation": op, "name": "/dev/acme/backend/env",
		}))
		require.NoError(t, err)
		require.Equalf(t, handler.StatusIgnored, res.Status, "operation %s", op)
		require.Empty(t, dep.seen)
	}
}

func TestSSMChangeForAScheduledTaskDoesNotRedeploy(t *testing.T) {
	h, dep, _ := newHandler(t, nil)

	res, err := h.Handle(context.Background(), event("aws.ssm", "Parameter Store Change", map[string]string{
		"operation": "Update", "name": "/dev/acme/task/cleanup/env",
	}))
	require.NoError(t, err)
	require.Equal(t, handler.StatusIgnored, res.Status)
	require.Empty(t, dep.seen, "a scheduled task reads its secrets when it next runs")
}

func TestSSMUnknownParameterIsIgnored(t *testing.T) {
	h, dep, _ := newHandler(t, nil)

	res, err := h.Handle(context.Background(), event("aws.ssm", "Parameter Store Change", map[string]string{
		"operation": "Update", "name": "/dev/otherproj/backend/env",
	}))
	require.NoError(t, err)
	require.Equal(t, handler.StatusIgnored, res.Status)
	require.Empty(t, dep.seen)
}

// ---------------------------------------------------------------- S3

// TestS3WriteDeploysEveryConsumerOfTheFile also covers a partial fan-out:
// shared.env is consumed by reporting (auto_deploy = true) and by
// payment-worker (false), and one disabled consumer must not hold up the other.
func TestS3WriteDeploysEveryConsumerOfTheFile(t *testing.T) {
	h, _, _ := newHandler(t, nil)

	res, err := h.Handle(context.Background(), event("aws.s3", "AWS API Call via CloudTrail", map[string]any{
		"eventName": "PutObject",
		"requestParameters": map[string]string{
			"bucketName": "acme-config",
			"key":        "shared.env",
		},
	}))

	require.NoError(t, err)
	require.ElementsMatch(t, []string{"reporting"}, res.Deployed)
}

// TestS3BackendEnvFileDeploysTheBackend is the regression test for the backend
// half of the S3 path: the rules were created from the backend file list while
// the map was built from the per-service list only, so a backend env change
// invoked the Lambda and then found nothing to do.
func TestS3BackendEnvFileDeploysTheBackend(t *testing.T) {
	h, dep, _ := newHandler(t, nil)

	res, err := h.Handle(context.Background(), event("aws.s3", "AWS API Call via CloudTrail", map[string]any{
		"eventName": "PutObject",
		"requestParameters": map[string]string{
			"bucketName": "acme-config",
			"key":        "backend.env",
		},
	}))

	require.NoError(t, err)
	require.Equal(t, []string{"backend"}, res.Deployed)
	require.Equal(t, []string{"backend"}, dep.ids())
}

func TestS3UnknownFileIsIgnored(t *testing.T) {
	h, dep, _ := newHandler(t, nil)

	res, err := h.Handle(context.Background(), event("aws.s3", "AWS API Call via CloudTrail", map[string]any{
		"eventName": "PutObject",
		"requestParameters": map[string]string{
			"bucketName": "someone-elses-bucket",
			"key":        "backend.env",
		},
	}))
	require.NoError(t, err)
	require.Equal(t, handler.StatusIgnored, res.Status)
	require.Empty(t, dep.seen)
}

// ---------------------------------------------------------------- manual

// TestManualDeployAcceptsEveryContractTheGeneratorsEmit pins the payloads the
// emitters actually produce today, and the legacy ones still in the wild.
//
// The fixture project is "acme" in "dev", so every case here has to be
// something a dev environment's EventBridge rule would really deliver.
func TestManualDeployAcceptsEveryContractTheGeneratorsEmit(t *testing.T) {
	cases := []struct {
		name       string
		source     string
		detailType string
		detail     map[string]string
		want       string
	}{
		{
			// web/src/components/Sidebar.tsx
			name: "Sidebar backend workflow", source: "action.dev", detailType: "DEPLOY",
			detail: map[string]string{"service": "backend", "project": "acme"}, want: "backend",
		},
		{
			// web/src/components/ServiceCICDConfiguration.tsx
			name: "per-service workflow", source: "github.actions.dev", detailType: "SERVICE_DEPLOY",
			detail: map[string]string{
				"service": "api", "project": "acme", "env": "dev", "trigger": "github",
			},
			want: "api",
		},
		{
			// receipts/github/prod-deploy.yml, with PROJECT/ENV substituted.
			name: "shipped deploy receipt", source: "action.dev", detailType: "DEPLOY",
			detail: map[string]string{"service": "backend", "project": "acme", "env": "dev"},
			want:   "backend",
		},
		{
			// Pipelines generated before project/env were added. Their rule is
			// environment-scoped by source, so accepting them is safe.
			name: "legacy payload, no project", source: "action.dev", detailType: "DEPLOY",
			detail: map[string]string{"service": "backend"}, want: "backend",
		},
		{
			// The environment-agnostic source. Its rule requires both fields.
			name: "fully scoped payload on a global source", source: "action.deploy", detailType: "DEPLOY",
			detail: map[string]string{"service": "payment-worker", "project": "acme", "env": "dev"},
			want:   "payment-worker",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, dep, _ := newHandler(t, nil)
			res, err := h.Handle(context.Background(), event(c.source, c.detailType, c.detail))
			require.NoError(t, err)
			require.Equal(t, handler.StatusDeployed, res.Status)
			require.Equal(t, []string{c.want}, dep.ids())
		})
	}
}

func TestManualDeployForAnotherProjectOrEnvironmentIsIgnored(t *testing.T) {
	for _, detail := range []map[string]string{
		{"service": "backend", "project": "otherproj"},
		{"service": "backend", "env": "prod"},
	} {
		h, dep, _ := newHandler(t, nil)
		res, err := h.Handle(context.Background(), event("action.deploy", "DEPLOY", detail))
		require.NoError(t, err)
		require.Equal(t, handler.StatusIgnored, res.Status)
		require.Empty(t, dep.seen)
	}
}

// TestManualDeployFromAnotherProjectOnAProductionSourceIsIgnored is the H4
// regression test.
//
// Two meroku projects in one AWS account both have a production environment, so
// both production rules accept "action.production" — no source list can
// separate them. The only thing that can is detail.project, which the shipped
// emitters now send. This is the case that used to deploy the wrong project's
// backend.
func TestManualDeployFromAnotherProjectOnAProductionSourceIsIgnored(t *testing.T) {
	// The fixture project is "acme"; pretend this Lambda is its production one.
	h, dep, _ := newHandler(t, map[string]string{"PROJECT_ENV": "production"})

	res, err := h.Handle(context.Background(), event("action.production", "DEPLOY", map[string]string{
		"service": "backend",
		"project": "otherproj",
		"env":     "production",
	}))

	require.NoError(t, err, "another project's deploy is not a failure of ours")
	require.Equal(t, handler.StatusIgnored, res.Status)
	require.Contains(t, res.Detail, "otherproj")
	require.Empty(t, dep.seen, "a mismatched project must never reach the deployer")
}

// TestManualDeployFromAnotherEnvironmentIsIgnored is the other half of H4: a
// production deploy that names its environment cannot deploy dev, even if the
// event somehow reaches the dev Lambda.
func TestManualDeployFromAnotherEnvironmentIsIgnored(t *testing.T) {
	h, dep, _ := newHandler(t, nil) // fixture env is "dev"

	res, err := h.Handle(context.Background(), event("action.production", "DEPLOY", map[string]string{
		"service": "backend",
		"project": "acme",
		"env":     "production",
	}))

	require.NoError(t, err)
	require.Equal(t, handler.StatusIgnored, res.Status)
	require.Contains(t, res.Detail, "production")
	require.Empty(t, dep.seen)
}

func TestManualDeployWithoutAServiceIsIgnored(t *testing.T) {
	h, dep, _ := newHandler(t, nil)

	res, err := h.Handle(context.Background(), event("action.deploy", "DEPLOY", map[string]string{"reason": "nothing"}))
	require.NoError(t, err)
	require.Equal(t, handler.StatusIgnored, res.Status)
	require.Empty(t, dep.seen)
}

func TestManualDeployOfAnUnknownServiceIsIgnoredNotRetried(t *testing.T) {
	cfg := testsupport.Config(t, nil)
	dep := &fakeDeployer{cfg: cfg, err: deploy.ErrUnknownTarget}
	h := handler.New(cfg, dep, &nopNotifier{}, testsupport.Logger())

	res, err := h.Handle(context.Background(), event("action.deploy", "DEPLOY", map[string]string{"service": "ghost"}))
	require.NoError(t, err, "EventBridge must not retry an identifier that will never resolve")
	require.Equal(t, handler.StatusIgnored, res.Status)
}

// ---------------------------------------------------------------- auto_deploy

// TestAutoDeployDisabledIsReportedByName is the whole reason auto_deploy is a
// flag in a map rather than an omission from one.
//
// If Terraform simply left a disabled target out of ECR_REPO_MAP, this Lambda's
// only honest answer would be "no target uses repository
// acme_service_payment-worker" — which is untrue, indistinguishable from a
// typo'd repository name, and sends whoever is on call hunting for a naming bug
// that does not exist. The target stays in every map, the push still invokes the
// Lambda, and the response names the actual reason.
func TestAutoDeployDisabledIsReportedByName(t *testing.T) {
	const disabled = "payment-worker"

	t.Run("ECR push", func(t *testing.T) {
		h, dep, notifier := newHandler(t, nil)

		res, err := h.Handle(context.Background(), event("aws.ecr", "ECR Image Action", map[string]string{
			"repository-name": "acme_service_" + disabled,
			"image-tag":       "v1",
			"action-type":     "PUSH",
			"result":          "SUCCESS",
		}))

		require.NoError(t, err, "a policy decision is not something to retry for hours")
		require.Equal(t, handler.StatusIgnored, res.Status)
		require.Equal(t, "auto_deploy is disabled for "+disabled, res.Detail)
		require.NotContains(t, res.Detail, "no target uses",
			"a disabled target must never be reported as an unmapped repository")
		require.Empty(t, dep.seen)
		require.Zero(t, notifier.count, "nothing was deployed, so nothing is announced")
	})

	t.Run("SSM update", func(t *testing.T) {
		h, dep, _ := newHandler(t, nil)

		res, err := h.Handle(context.Background(), event("aws.ssm", "Parameter Store Change", map[string]string{
			"operation": "Update", "name": "/dev/acme/" + disabled + "/env",
		}))

		require.NoError(t, err)
		require.Equal(t, handler.StatusIgnored, res.Status)
		require.Equal(t, "auto_deploy is disabled for "+disabled, res.Detail)
		require.Empty(t, dep.seen)
	})

	t.Run("S3 env file", func(t *testing.T) {
		// api.env belongs to api alone, so swap the fixture's map for one where
		// the only consumer of a file is the disabled service.
		h, dep, _ := newHandler(t, map[string]string{
			"S3_SERVICE_MAP": `{"payment-worker":[{"bucket":"acme-config","key":"worker.env"}]}`,
		})

		res, err := h.Handle(context.Background(), event("aws.s3", "AWS API Call via CloudTrail", map[string]any{
			"eventName": "PutObject",
			"requestParameters": map[string]string{
				"bucketName": "acme-config",
				"key":        "worker.env",
			},
		}))

		require.NoError(t, err)
		require.Equal(t, handler.StatusIgnored, res.Status)
		require.Equal(t, "auto_deploy is disabled for "+disabled, res.Detail)
		require.Empty(t, dep.seen)
	})

	// The button still works. auto_deploy answers "may an event redeploy this
	// on its own?" — turning off automatic deploys in prod must not also take
	// away the ability to deploy prod on purpose.
	t.Run("manual deploy is unaffected", func(t *testing.T) {
		h, dep, _ := newHandler(t, nil)

		res, err := h.Handle(context.Background(), event("action.deploy", "DEPLOY", map[string]string{
			"service": disabled, "project": "acme", "env": "dev",
		}))

		require.NoError(t, err)
		require.Equal(t, handler.StatusDeployed, res.Status)
		require.Equal(t, []string{disabled}, dep.ids())
	})
}

// TestAutoDeployDisabledScheduledTaskIsReportedByName covers the target kind
// that motivated the setting: outside dev nothing can reach a scheduled task at
// all, so a task map that lists them says more than it can deliver. With the
// flag shipped, a push to a task repository answers with the policy instead of
// with silence.
func TestAutoDeployDisabledScheduledTaskIsReportedByName(t *testing.T) {
	h, dep, _ := newHandler(t, nil)

	res, err := h.Handle(context.Background(), event("aws.ecr", "ECR Image Action", map[string]string{
		"repository-name": "acme_task_archive",
		"image-tag":       "v1",
		"action-type":     "PUSH",
		"result":          "SUCCESS",
	}))

	require.NoError(t, err)
	require.Equal(t, handler.StatusIgnored, res.Status)
	require.Equal(t, "auto_deploy is disabled for task:archive", res.Detail)
	require.Empty(t, dep.seen)
}

// TestAutoDeployFanOutDeploysOnlyTheEnabledConsumers pins the partial case: one
// repository feeds several targets (ecr_config mode = use_existing) and they
// need not share a policy.
func TestAutoDeployFanOutDeploysOnlyTheEnabledConsumers(t *testing.T) {
	h, dep, _ := newHandler(t, map[string]string{
		// api and reporting share acme_service_api in the fixture; disable one.
		"AUTO_DEPLOY_MAP": `{"backend":true,"api":false,"reporting":true,"payment-worker":false,` +
			`"legacy-api":true,"task":true,"task:cleanup":true,"task:archive":false}`,
	})

	res, err := h.Handle(context.Background(), event("aws.ecr", "ECR Image Action", map[string]string{
		"repository-name": "acme_service_api",
		"image-tag":       "v1",
		"action-type":     "PUSH",
		"result":          "SUCCESS",
	}))

	require.NoError(t, err)
	require.Equal(t, handler.StatusDeployed, res.Status)
	require.Equal(t, []string{"reporting"}, res.Deployed)
	require.Equal(t, []string{"reporting"}, dep.ids(), "the disabled consumer must not reach the deployer")
}

// TestAbsentAutoDeployMapDeploysEverything is the upgrade path: a Lambda whose
// Terraform state predates the setting must behave exactly as it does today.
func TestAbsentAutoDeployMapDeploysEverything(t *testing.T) {
	h, dep, _ := newHandler(t, map[string]string{"AUTO_DEPLOY_MAP": ""})

	res, err := h.Handle(context.Background(), event("aws.ecr", "ECR Image Action", map[string]string{
		"repository-name": "acme_service_payment-worker",
		"image-tag":       "v1",
		"action-type":     "PUSH",
		"result":          "SUCCESS",
	}))

	require.NoError(t, err)
	require.Equal(t, handler.StatusDeployed, res.Status)
	require.Equal(t, []string{"payment-worker"}, dep.ids())
}

// ---------------------------------------------------------------- ECS state

func TestECSStateOnlyNotifies(t *testing.T) {
	h, dep, notifier := newHandler(t, nil)

	res, err := h.Handle(context.Background(), event("aws.ecs", "ECS Deployment State Change", map[string]string{
		"eventType": "INFO", "eventName": "SERVICE_DEPLOYMENT_COMPLETED", "deploymentId": "ecs-svc/new",
	}))

	require.NoError(t, err)
	require.Equal(t, handler.StatusNotified, res.Status)
	require.Equal(t, 1, notifier.count)
	require.Empty(t, dep.seen, "an ECS state change never deploys")
}

func TestECSSteadyStateIsSuppressed(t *testing.T) {
	h, _, notifier := newHandler(t, nil)

	res, err := h.Handle(context.Background(), event("aws.ecs", "ECS Service Action", map[string]string{
		"eventType": "INFO", "eventName": "SERVICE_STEADY_STATE",
	}))
	require.NoError(t, err)
	require.Equal(t, handler.StatusIgnored, res.Status)
	require.Zero(t, notifier.count)
}

// ---------------------------------------------------------------- routing

func TestUnknownSourceIsIgnoredNotRetried(t *testing.T) {
	h, dep, _ := newHandler(t, nil)

	res, err := h.Handle(context.Background(), event("aws.cloudtrail", "Something Else", map[string]string{}))
	require.NoError(t, err, "an unroutable event must not be retried for hours")
	require.Equal(t, handler.StatusIgnored, res.Status)
	require.Empty(t, dep.seen)
}

func TestUnparsableDetailIsIgnoredNotRetried(t *testing.T) {
	h, _, _ := newHandler(t, nil)

	for _, source := range []string{"aws.ecr", "aws.ssm", "aws.s3", "aws.ecs"} {
		ev := event(source, "whatever", map[string]string{})
		ev.Detail = json.RawMessage(`["not", "an", "object"]`)

		res, err := h.Handle(context.Background(), ev)
		require.NoErrorf(t, err, "source %s", source)
		require.Equalf(t, handler.StatusIgnored, res.Status, "source %s", source)
	}
}

// TestRetryableAWSFailureIsReturned is the other half of the error policy: an
// error that a retry could fix must reach EventBridge.
func TestRetryableAWSFailureIsReturned(t *testing.T) {
	cfg := testsupport.Config(t, nil)
	dep := &fakeDeployer{cfg: cfg, err: &types.ServerException{}}
	h := handler.New(cfg, dep, &nopNotifier{}, testsupport.Logger())

	_, err := h.Handle(context.Background(), event("aws.ecr", "ECR Image Action", map[string]string{
		"repository-name": "acme_backend", "action-type": "PUSH", "result": "SUCCESS",
	}))
	require.Error(t, err)
}
