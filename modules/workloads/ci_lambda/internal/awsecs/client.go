// Package awsecs is the only place in this Lambda that talks to ECS.
//
// It is deliberately narrow: three API calls behind an interface, so every
// test runs with no credentials and no network.
package awsecs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// ErrPermanent marks a failure this package raised itself, about a fact that
// cannot change between two attempts of the same invocation: a missing field, a
// family whose containers do not use the repository that was pushed, an API
// response with nothing in it.
//
// It exists because these were bare fmt.Errorf values, which
// deploy.Retryable could not tell apart from a transient AWS fault and
// therefore treated as retryable. A scheduled task whose container image comes
// from somewhere else — docker_image pinned in YAML, a repository recreated
// elsewhere, a renamed container — then burned the whole retry budget and
// posted DEPLOYMENT_INITIATING plus DEPLOYMENT_FAILED to Slack on each of three
// invocations, forever, for a condition that will never clear.
//
// deploy.Retryable matches this with errors.Is and returns false.
var ErrPermanent = errors.New("permanent ecs error")

// API is the subset of the ECS client this Lambda uses.
//
// ListTaskDefinitions is deliberately absent. Resolving "the latest revision"
// client-side is what produced the ":9 beats :11" bug; ECS resolves the latest
// ACTIVE revision itself when handed a bare family name.
type API interface {
	UpdateService(context.Context, *ecs.UpdateServiceInput, ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error)
	DescribeTaskDefinition(context.Context, *ecs.DescribeTaskDefinitionInput, ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error)
	RegisterTaskDefinition(context.Context, *ecs.RegisterTaskDefinitionInput, ...func(*ecs.Options)) (*ecs.RegisterTaskDefinitionOutput, error)
}

// Client performs ECS deployments.
type Client struct {
	api     API
	cluster string
	dryRun  bool
	log     *slog.Logger
}

// New builds a Client over any API implementation.
func New(api API, cluster string, dryRun bool, log *slog.Logger) *Client {
	return &Client{api: api, cluster: cluster, dryRun: dryRun, log: log}
}

// NewFromAWSConfig builds a Client over the real ECS service client.
func NewFromAWSConfig(cfg aws.Config, cluster string, dryRun bool, log *slog.Logger) *Client {
	return New(ecs.NewFromConfig(cfg), cluster, dryRun, log)
}

// UpdateRequest asks for a rolling deployment of one ECS service.
type UpdateRequest struct {
	ServiceName string
	// TaskDefinition is normally the bare *family*: ECS then resolves the
	// latest ACTIVE revision server-side. A "family:revision" or a full ARN is
	// accepted too, for a manual pinned deploy.
	TaskDefinition string
	Force          bool
}

// UpdateResult reports what ECS actually did.
type UpdateResult struct {
	ServiceName string
	// TaskDefinition is the revision ECS resolved, read back from the response.
	TaskDefinition string
	DeploymentID   string
}

// UpdateService triggers a new deployment of a long-running ECS service.
func (c *Client) UpdateService(ctx context.Context, req UpdateRequest) (UpdateResult, error) {
	if req.ServiceName == "" {
		return UpdateResult{}, fmt.Errorf("%w: ecs: service name is required", ErrPermanent)
	}

	log := c.log.With("cluster", c.cluster, "service", req.ServiceName, "task_definition", req.TaskDefinition)

	if c.dryRun {
		log.Info("dry run: would update service")
		return UpdateResult{
			ServiceName:    req.ServiceName,
			TaskDefinition: req.TaskDefinition,
			DeploymentID:   "DRY_RUN",
		}, nil
	}

	in := &ecs.UpdateServiceInput{
		Cluster:            aws.String(c.cluster),
		Service:            aws.String(req.ServiceName),
		ForceNewDeployment: req.Force,
	}
	if req.TaskDefinition != "" {
		in.TaskDefinition = aws.String(req.TaskDefinition)
	}

	out, err := c.api.UpdateService(ctx, in)
	if err != nil {
		return UpdateResult{}, fmt.Errorf("ecs: update service %q on cluster %q: %w", req.ServiceName, c.cluster, err)
	}

	res := UpdateResult{ServiceName: req.ServiceName, TaskDefinition: req.TaskDefinition}
	if out != nil && out.Service != nil {
		if out.Service.TaskDefinition != nil {
			res.TaskDefinition = aws.ToString(out.Service.TaskDefinition)
		}
		for _, d := range out.Service.Deployments {
			if aws.ToString(d.Status) == "PRIMARY" {
				res.DeploymentID = aws.ToString(d.Id)
				break
			}
		}
	}

	log.Info("ecs service updated", "resolved_task_definition", res.TaskDefinition, "deployment_id", res.DeploymentID)
	return res, nil
}

// RegisterRevisionWithImage clones the active revision of a task-definition
// family, swaps the image of every container that already points at the same
// repository, and registers the result.
//
// Every field the API returns is carried forward — including PidMode, IpcMode
// and InferenceAccelerators, whose omission silently stripped those settings on
// every scheduled-task deploy.
func (c *Client) RegisterRevisionWithImage(ctx context.Context, family, imageURI string) (string, error) {
	if family == "" {
		return "", fmt.Errorf("%w: ecs: task definition family is required", ErrPermanent)
	}
	if imageURI == "" {
		return "", fmt.Errorf("%w: ecs: image URI is required to register a new revision of %q", ErrPermanent, family)
	}

	log := c.log.With("task_family", family, "image", imageURI)

	if c.dryRun {
		log.Info("dry run: would register a new task definition revision")
		return family + ":DRY_RUN", nil
	}

	// DescribeTaskDefinition with a bare family name returns the latest ACTIVE
	// revision by exact family match — no prefix semantics, no client sorting.
	desc, err := c.api.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: aws.String(family),
		Include:        []types.TaskDefinitionField{types.TaskDefinitionFieldTags},
	})
	if err != nil {
		return "", fmt.Errorf("ecs: describe task definition %q: %w", family, err)
	}
	if desc == nil || desc.TaskDefinition == nil {
		return "", fmt.Errorf("%w: ecs: describe task definition %q returned no definition", ErrPermanent, family)
	}
	td := desc.TaskDefinition

	newRepo, _ := SplitImageRef(imageURI)
	containers := make([]types.ContainerDefinition, len(td.ContainerDefinitions))
	matched := 0
	for i, container := range td.ContainerDefinitions {
		containers[i] = container
		if container.Image == nil {
			continue
		}
		currentRepo, _ := SplitImageRef(aws.ToString(container.Image))
		if currentRepo != newRepo {
			continue
		}
		containers[i].Image = aws.String(imageURI)
		matched++
		log.Info("container image updated",
			"container", aws.ToString(container.Name),
			"old_image", aws.ToString(container.Image),
			"new_image", imageURI)
	}

	// Re-registering an identical definition would look like a successful
	// deploy and change nothing at all.
	if matched == 0 {
		return "", fmt.Errorf(
			"%w: ecs: no container in family %q uses repository %q; refusing to register an identical revision",
			ErrPermanent, family, newRepo)
	}

	in := &ecs.RegisterTaskDefinitionInput{
		Family:                  td.Family,
		ContainerDefinitions:    containers,
		Cpu:                     td.Cpu,
		EnableFaultInjection:    td.EnableFaultInjection,
		EphemeralStorage:        td.EphemeralStorage,
		ExecutionRoleArn:        td.ExecutionRoleArn,
		InferenceAccelerators:   td.InferenceAccelerators,
		IpcMode:                 td.IpcMode,
		Memory:                  td.Memory,
		NetworkMode:             td.NetworkMode,
		PidMode:                 td.PidMode,
		PlacementConstraints:    td.PlacementConstraints,
		ProxyConfiguration:      td.ProxyConfiguration,
		RequiresCompatibilities: td.RequiresCompatibilities,
		RuntimePlatform:         td.RuntimePlatform,
		TaskRoleArn:             td.TaskRoleArn,
		Volumes:                 td.Volumes,
	}
	if len(desc.Tags) > 0 {
		in.Tags = desc.Tags
	}

	out, err := c.api.RegisterTaskDefinition(ctx, in)
	if err != nil {
		return "", fmt.Errorf("ecs: register task definition revision for %q: %w", family, err)
	}
	if out == nil || out.TaskDefinition == nil {
		return "", fmt.Errorf("%w: ecs: register task definition for %q returned no definition", ErrPermanent, family)
	}

	arn := aws.ToString(out.TaskDefinition.TaskDefinitionArn)
	log.Info("task definition revision registered", "task_definition_arn", arn, "containers_updated", matched)
	return arn, nil
}
