package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

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

	// Terraform creates these parameters itself, and deleting one is not a
	// reason to redeploy a service whose configuration has just vanished.
	if d.Operation != SSMOperationUpdate {
		return ignored("SSM operation " + d.Operation + " does not trigger a deployment"), nil
	}

	id, ok := h.cfg.IdentifierForSSMPath(d.Name)
	if !ok {
		log.Info("SSM parameter is not mapped to any target in this project")
		return ignored("no target uses parameter " + d.Name), nil
	}

	// Scheduled tasks read their SSM secrets when the task starts, so the next
	// scheduled run picks the new value up on its own. There is no service to
	// restart and a new revision would carry the same image.
	if h.cfg.IsScheduledTask(id) {
		log.Info("scheduled task picks up SSM changes on its next run", "target", id)
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
