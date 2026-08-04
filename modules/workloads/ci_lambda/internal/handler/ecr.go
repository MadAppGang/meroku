package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/aws/aws-lambda-go/events"
	"madappgang.com/infrastructure/ci_lambda/internal/deploy"
)

// ecrDetail is the ECR Image Action event body.
type ecrDetail struct {
	RepositoryName string `json:"repository-name"`
	Tag            string `json:"image-tag"`
	Digest         string `json:"image-digest"`
	ActionType     string `json:"action-type"`
	Result         string `json:"result"`
}

// ecr handles an image push.
//
// The repository name is looked up in the Terraform-emitted ECR_REPO_MAP. It
// is never parsed: parsing cannot express one repository feeding several
// services (ecr_config mode=use_existing), cannot express an arbitrarily named
// manual_repo, and cannot tell another project's repository from ours.
func (h *Handler) ecr(ctx context.Context, log *slog.Logger, ev events.CloudWatchEvent) (Response, error) {
	var d ecrDetail
	if err := json.Unmarshal(ev.Detail, &d); err != nil {
		log.Warn("ECR event detail could not be parsed", "error", err)
		return ignored("unparsable ECR event detail"), nil
	}

	log = log.With("repository", d.RepositoryName, "image_tag", d.Tag)

	if d.ActionType != ECRActionTypePush || d.Result != ECRResultSuccess {
		return ignored(fmt.Sprintf("ECR action-type=%s result=%s", d.ActionType, d.Result)), nil
	}

	resolved := h.cfg.IdentifiersForRepo(d.RepositoryName)
	if len(resolved) == 0 {
		// Another project's repository in a shared account lands here.
		log.Info("ECR repository is not mapped to any target in this project")
		return ignored("no target uses repository " + d.RepositoryName), nil
	}

	// A repository can feed several targets and they need not share a policy:
	// deploy the ones that opted in, name the ones that did not.
	ids := h.autoDeployable(log, resolved)
	if len(ids) == 0 {
		return autoDeployDisabled(resolved), nil
	}

	imageURI := ecrImageURI(ev.AccountID, ev.Region, d)

	reqs := make([]deploy.Request, 0, len(ids))
	for _, id := range ids {
		req := deploy.Request{
			ID:     id,
			Reason: fmt.Sprintf("New image pushed to %s", imageURI),
			Source: deploy.SourceECR,
		}
		if h.cfg.IsScheduledTask(id) {
			// A scheduled task has no ECS service to update; the deploy is a
			// new task-definition revision carrying this image.
			req.ImageURI = imageURI
		}
		reqs = append(reqs, req)
	}

	log.Info("ECR push resolved", "targets", ids)
	return h.deployMany(ctx, log, reqs)
}

// ecrImageURI rebuilds the pushed image reference. The event carries the
// repository, tag and digest but not the registry host, which is derived from
// the account and region the event came from.
func ecrImageURI(accountID, region string, d ecrDetail) string {
	repo := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s", accountID, region, d.RepositoryName)
	switch {
	case d.Tag != "":
		return repo + ":" + d.Tag
	case d.Digest != "":
		return repo + "@" + d.Digest
	default:
		return repo
	}
}
