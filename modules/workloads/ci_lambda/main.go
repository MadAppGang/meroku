// Command ci_lambda is the deployment trigger for a meroku project.
//
// It reacts to ECR pushes, SSM parameter changes, S3 env-file writes, manual
// DEPLOY events and ECS deployment state changes, and turns them into an ECS
// UpdateService or a new task-definition revision. It is fire-and-forget: no
// deployment waiter, which is why a 60s function timeout is enough.
package main

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"madappgang.com/infrastructure/ci_lambda/contract"
	"madappgang.com/infrastructure/ci_lambda/internal/awsecs"
	"madappgang.com/infrastructure/ci_lambda/internal/config"
	"madappgang.com/infrastructure/ci_lambda/internal/deploy"
	"madappgang.com/infrastructure/ci_lambda/internal/handler"
	"madappgang.com/infrastructure/ci_lambda/internal/slack"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		startInert(newLogger(slog.LevelInfo), "configuration could not be loaded", err)
		return
	}

	log := newLogger(cfg.LogLevel).With("project", cfg.Project, "env", cfg.Env)

	// The contract check reports, it does not gate. Refusing to start would
	// turn one bad map entry into "every matching event in the account is
	// retried by the async invoke path" — the exact failure this Lambda's
	// error policy exists to avoid. Alarm on event=contract_selfcheck_failed.
	if problems := cfg.SelfCheck(contract.Load().BackendID()); len(problems) > 0 {
		log.Error("contract self-check failed; unresolvable events will be ignored",
			"event", "contract_selfcheck_failed",
			"problem_count", len(problems),
			"problems", problems)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
	if err != nil {
		startInert(log, "AWS configuration could not be loaded", err)
		return
	}

	notifier := slack.New(cfg.SlackWebhookURL, cfg.Env, log)
	ecsClient := awsecs.NewFromAWSConfig(awsCfg, cfg.Cluster, cfg.DryRun, log)
	deployer := deploy.New(cfg, ecsClient, notifier, log)
	h := handler.New(cfg, deployer, notifier, log)

	log.Info("ci lambda ready",
		"cluster", cfg.Cluster,
		"region", cfg.Region,
		"targets", len(cfg.Targets),
		"ecr_repos", len(cfg.ECRRepos),
		"ssm_prefixes", len(cfg.SSMPrefixes),
		"dry_run", cfg.DryRun)

	lambda.Start(h.Handle)
}

// startInert serves every event as `ignored` and logs the reason, instead of
// exiting.
//
// os.Exit(1) during initialization looks like the loud option and is the
// opposite. EventBridge invokes this function asynchronously, so a failed
// invocation is redelivered twice more — and an initialization failure fails
// *every* invocation, so a permanently bad configuration turns into a permanent
// retry storm across every rule this project owns, every one of them charged
// and logged three times. That is precisely the behaviour the comment on
// SelfCheck refuses to accept for a bad map entry; the two policies now agree.
//
// Visibility is not lost: the reason is logged at ERROR once at init and again
// on every invocation, with a stable `event` key to alarm on. What is lost is
// EventBridge's reason to retry.
func startInert(log *slog.Logger, message string, cause error) {
	log.Error(message+"; every event will be ignored",
		"event", "configuration_invalid",
		"error", cause)

	lambda.Start(inertHandler(log, cause))
}

// inertHandler is what startInert serves. Split out so a test can drive it
// without starting the Lambda runtime.
func inertHandler(log *slog.Logger, cause error) func(context.Context, events.CloudWatchEvent) (handler.Response, error) {
	return func(context.Context, events.CloudWatchEvent) (handler.Response, error) {
		log.Error("event ignored: this Lambda is not configured to do anything",
			"event", "configuration_invalid",
			"error", cause)
		return handler.Response{
			Status: handler.StatusIgnored,
			Detail: "ci lambda is misconfigured: " + cause.Error(),
		}, nil
	}
}

// newLogger returns a JSON slog logger whose output stays parsable by the
// CloudWatch Logs Insights queries this project uses: a `timestamp` field, a
// lowercase `level`, and a `message`.
//
// Attributes are flat. The old logger nested everything under `fields.*`; any
// saved query on `fields.x` needs updating to `x`.
func newLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) > 0 {
				return a
			}
			switch a.Key {
			case slog.TimeKey:
				a.Key = "timestamp"
			case slog.MessageKey:
				a.Key = "message"
			case slog.LevelKey:
				if lvl, ok := a.Value.Any().(slog.Level); ok {
					a.Value = slog.StringValue(strings.ToLower(lvl.String()))
				}
			}
			return a
		},
	}))
}
