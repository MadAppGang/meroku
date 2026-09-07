package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"

	"github.com/aws/aws-lambda-go/events"
	"madappgang.com/infrastructure/ci_lambda/internal/deploy"
)

type ssmDetail struct {
	Operation string `json:"operation"`
	Name      string `json:"name"`
	Type      string `json:"type"`
}

// ssm handles a Parameter Store change.
//
// Resolution is a longest-prefix lookup against SSM_SERVICE_MAP. The regex it
// replaces required exactly /{env}/{project}/{x}/{y} with \w+ segments, so it
// missed scheduled tasks (/{env}/{project}/task/{name}/env, five segments) and
// every hyphenated service name.
func (h *Handler) ssm(ctx context.Context, log *slog.Logger, ev events.CloudWatchEvent) (Response, error) {
	var d ssmDetail
	if err := json.Unmarshal(ev.Detail, &d); err != nil {
		log.Warn("SSM event detail could not be parsed", "error", err)
		return ignored("unparsable SSM event detail"), nil
	}

	log = log.With("parameter", d.Name, "operation", d.Operation)

	// SSMDeployOperations is the same slice PatternContracts() publishes as what
	// the ci_ssm_change rule must select, so the filter in front of this Lambda
	// and the filter inside it cannot disagree. See eventpattern.go for why
	// Create belongs and Delete does not.
	if !slices.Contains(SSMDeployOperations, d.Operation) {
		return ignored("SSM operation " + d.Operation + " does not trigger a deployment"), nil
	}

	// Terraform creates a "{prefix}/env" placeholder per target so the path
	// exists for an operator to fill in, and creating one says nothing about the
	// configuration. On the first apply of an environment it is also emitted
	// before the ECS service it names exists.
	//
	// Create only. An Update of {prefix}/env is the main configuration path in
	// this system and must keep deploying — which is why this cannot be folded
	// into the operation test above.
	if d.Operation == SSMOperationCreate && h.cfg.IsTerraformOwnedSSMPath(d.Name) {
		log.Info("parameter is Terraform's own placeholder, not a configuration change")
		return ignored("Terraform created placeholder parameter " + d.Name), nil
	}

	id, ok := h.cfg.IdentifierForSSMPath(d.Name)
	if !ok {
		log.Info("SSM parameter is not mapped to any target in this project")
		return ignored("no target uses parameter " + d.Name), nil
	}

	// A scheduled task is skipped because this Lambda has nothing useful it
	// could do, not because the change is harmless.
	//
	// The reason this comment used to give — "a scheduled task reads its secrets
	// when it next runs" — holds for a changed VALUE and is false for an ADDED
	// parameter. Adding one changes the `secrets` LIST, which lives in the task
	// definition (modules/ecs_task/main.tf renders it from
	// data.aws_ssm_parameters_by_path.task), and no task start can conjure a
	// list entry that is not there.
	//
	// What is true is that the Lambda cannot conjure it either: its one
	// revision-producing call, awsecs.RegisterRevisionWithImage, CLONES the
	// latest ACTIVE revision and substitutes an image, so it would carry the old
	// list forward. Only Terraform can render the new one — and once it has, the
	// scheduler reaches it unaided. modules/ecs_task/main.tf targets
	// aws_ecs_task_definition.task.arn_without_revision, a family ARN resolved at
	// run time; modules/event_bridge_task/main.tf pins .arn, but the apply that
	// registers the revision rewrites that target in the same walk, because the
	// attribute it reads changed.
	//
	// So a scheduled task needs no notification, which is also why the
	// aws_lambda_invocation edge in services.tf and backend.tf exists for
	// services alone.
	if h.cfg.IsScheduledTask(id) {
		log.Info("only Terraform can change a scheduled task's secrets, and its scheduler follows", "target", id)
		return ignored("scheduled task " + id + " needs no redeployment for an SSM change"), nil
	}

	if len(h.autoDeployable(log, []string{id})) == 0 {
		return autoDeployDisabled([]string{id}), nil
	}

	log.Info("SSM change resolved", "target", id)
	return h.deployOne(ctx, log, deploy.Request{
		ID:     id,
		Reason: fmt.Sprintf("SSM parameter changed: %s", d.Name),
		Source: deploy.SourceSSM,
	})
}
