package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/aws/aws-lambda-go/events"
	"madappgang.com/infrastructure/ci_lambda/internal/deploy"
)

type s3Detail struct {
	EventName         string `json:"eventName"`
	RequestParameters struct {
		BucketName string `json:"bucketName"`
		Key        string `json:"key"`
	} `json:"requestParameters"`
}

// s3 handles an env-file write.
//
// S3_SERVICE_MAP carries the backend's files as well as the per-service ones,
// keyed on the bucket name exactly as the task definitions spell it.
func (h *Handler) s3(ctx context.Context, log *slog.Logger, ev events.CloudWatchEvent) (Response, error) {
	var d s3Detail
	if err := json.Unmarshal(ev.Detail, &d); err != nil {
		log.Warn("S3 event detail could not be parsed", "error", err)
		return ignored("unparsable S3 event detail"), nil
	}

	bucket := d.RequestParameters.BucketName
	key := d.RequestParameters.Key
	log = log.With("bucket", bucket, "key", key, "s3_event", d.EventName)

	resolved := h.cfg.IdentifiersForS3(bucket, key)
	if len(resolved) == 0 {
		log.Info("S3 object is not mapped to any target in this project")
		return ignored(fmt.Sprintf("no target uses s3://%s/%s", bucket, key)), nil
	}

	// One env file is routinely shared by several services, which need not
	// share a policy.
	ids := h.autoDeployable(log, resolved)
	if len(ids) == 0 {
		return autoDeployDisabled(resolved), nil
	}

	reqs := make([]deploy.Request, 0, len(ids))
	for _, id := range ids {
		reqs = append(reqs, deploy.Request{
			ID:     id,
			Reason: fmt.Sprintf("Env file changed: s3://%s/%s", bucket, key),
			Source: deploy.SourceS3,
		})
	}

	log.Info("S3 change resolved", "targets", ids)
	return h.deployMany(ctx, log, reqs)
}
