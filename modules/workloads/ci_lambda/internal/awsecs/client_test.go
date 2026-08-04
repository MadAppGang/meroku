package awsecs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/stretchr/testify/require"
	"madappgang.com/infrastructure/ci_lambda/internal/awsecs"
	"madappgang.com/infrastructure/ci_lambda/internal/testsupport"
)

func newClient(api awsecs.API, dryRun bool) *awsecs.Client {
	return awsecs.New(api, "acme_cluster_dev", dryRun, testsupport.Logger())
}

// TestUpdateServicePassesTheFamilyNotAResolvedRevision is the regression test
// for the wrong-revision bug. The Lambda used to list task definitions and sort
// the ARNs as strings, so ":9" beat ":11". It now hands ECS the bare family and
// reads back whatever ECS resolved.
func TestUpdateServicePassesTheFamilyNotAResolvedRevision(t *testing.T) {
	api := &fakeAPI{
		updateOut: &ecs.UpdateServiceOutput{
			Service: &types.Service{
				TaskDefinition: aws.String("arn:aws:ecs:us-east-1:000000000000:task-definition/acme_service_dev:11"),
				Deployments: []types.Deployment{
					{Status: aws.String("ACTIVE"), Id: aws.String("ecs-svc/old")},
					{Status: aws.String("PRIMARY"), Id: aws.String("ecs-svc/new")},
				},
			},
		},
	}

	res, err := newClient(api, false).UpdateService(context.Background(), awsecs.UpdateRequest{
		ServiceName:    "acme_service_dev",
		TaskDefinition: "acme_service_dev",
		Force:          true,
	})
	require.NoError(t, err)

	require.Len(t, api.updateIn, 1)
	require.Equal(t, "acme_service_dev", aws.ToString(api.updateIn[0].TaskDefinition),
		"the family is sent verbatim; ECS resolves the latest ACTIVE revision")
	require.Equal(t, "acme_cluster_dev", aws.ToString(api.updateIn[0].Cluster))
	require.True(t, api.updateIn[0].ForceNewDeployment)

	require.Equal(t, "arn:aws:ecs:us-east-1:000000000000:task-definition/acme_service_dev:11", res.TaskDefinition)
	require.Equal(t, "ecs-svc/new", res.DeploymentID)
}

func TestUpdateServicePropagatesErrors(t *testing.T) {
	api := &fakeAPI{updateErr: &types.ServiceNotFoundException{}}

	_, err := newClient(api, false).UpdateService(context.Background(), awsecs.UpdateRequest{
		ServiceName: "acme_service_dev",
	})
	var notFound *types.ServiceNotFoundException
	require.ErrorAs(t, err, &notFound, "the typed error must survive wrapping so retry classification works")
}

func TestUpdateServiceDryRunMakesNoCalls(t *testing.T) {
	api := &fakeAPI{}

	res, err := newClient(api, true).UpdateService(context.Background(), awsecs.UpdateRequest{
		ServiceName:    "acme_service_dev",
		TaskDefinition: "acme_service_dev",
	})
	require.NoError(t, err)
	require.Empty(t, api.updateIn)
	require.Equal(t, "DRY_RUN", res.DeploymentID)
}

func taskDefinition() *ecs.DescribeTaskDefinitionOutput {
	return &ecs.DescribeTaskDefinitionOutput{
		TaskDefinition: &types.TaskDefinition{
			Family: aws.String("acme_task_cleanup_dev"),
			ContainerDefinitions: []types.ContainerDefinition{
				{
					Name:  aws.String("app"),
					Image: aws.String("000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_task_cleanup:old"),
				},
				{
					Name:  aws.String("sidecar"),
					Image: aws.String("amazon/aws-xray-daemon"),
				},
			},
			Cpu:                   aws.String("256"),
			Memory:                aws.String("512"),
			NetworkMode:           types.NetworkModeAwsvpc,
			PidMode:               types.PidModeTask,
			IpcMode:               types.IpcModeTask,
			InferenceAccelerators: []types.InferenceAccelerator{{DeviceName: aws.String("dev1"), DeviceType: aws.String("eia2.medium")}},
			TaskRoleArn:           aws.String("arn:aws:iam::000000000000:role/acme_cleanup_task_dev"),
			ExecutionRoleArn:      aws.String("arn:aws:iam::000000000000:role/acme_exec_dev"),
			Volumes:               []types.Volume{{Name: aws.String("scratch")}},
			RuntimePlatform:       &types.RuntimePlatform{CpuArchitecture: types.CPUArchitectureArm64},
			EphemeralStorage:      &types.EphemeralStorage{SizeInGiB: 21},
			RequiresCompatibilities: []types.Compatibility{
				types.CompatibilityFargate,
			},
			PlacementConstraints: []types.TaskDefinitionPlacementConstraint{
				{Type: types.TaskDefinitionPlacementConstraintTypeMemberOf, Expression: aws.String("attribute:ecs.os-type == linux")},
			},
		},
		Tags: []types.Tag{{Key: aws.String("Project"), Value: aws.String("acme")}},
	}
}

// TestRegisterRevisionCarriesEveryField is the regression test for the silent
// loss of PidMode, IpcMode and InferenceAccelerators on every scheduled-task
// deploy.
func TestRegisterRevisionCarriesEveryField(t *testing.T) {
	api := &fakeAPI{
		describeOut: taskDefinition(),
		registerOut: &ecs.RegisterTaskDefinitionOutput{
			TaskDefinition: &types.TaskDefinition{
				TaskDefinitionArn: aws.String("arn:aws:ecs:us-east-1:000000000000:task-definition/acme_task_cleanup_dev:12"),
			},
		},
	}

	arn, err := newClient(api, false).RegisterRevisionWithImage(context.Background(),
		"acme_task_cleanup_dev",
		"000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_task_cleanup:new")
	require.NoError(t, err)
	require.Equal(t, "arn:aws:ecs:us-east-1:000000000000:task-definition/acme_task_cleanup_dev:12", arn)

	require.Len(t, api.describeIn, 1)
	require.Equal(t, "acme_task_cleanup_dev", aws.ToString(api.describeIn[0].TaskDefinition),
		"an exact family name, not a FamilyPrefix that also matches sibling families")

	require.Len(t, api.registerIn, 1)
	in := api.registerIn[0]

	require.Equal(t, types.PidModeTask, in.PidMode)
	require.Equal(t, types.IpcModeTask, in.IpcMode)
	require.Len(t, in.InferenceAccelerators, 1)
	require.Equal(t, "256", aws.ToString(in.Cpu))
	require.Equal(t, "512", aws.ToString(in.Memory))
	require.Equal(t, types.NetworkModeAwsvpc, in.NetworkMode)
	require.Len(t, in.Volumes, 1)
	require.Len(t, in.PlacementConstraints, 1)
	require.Len(t, in.RequiresCompatibilities, 1)
	require.NotNil(t, in.RuntimePlatform)
	require.NotNil(t, in.EphemeralStorage)
	require.Equal(t, "arn:aws:iam::000000000000:role/acme_cleanup_task_dev", aws.ToString(in.TaskRoleArn))
	require.Equal(t, "arn:aws:iam::000000000000:role/acme_exec_dev", aws.ToString(in.ExecutionRoleArn))
	require.Len(t, in.Tags, 1)

	// Only the container whose repository matches is swapped.
	require.Equal(t, "000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_task_cleanup:new",
		aws.ToString(in.ContainerDefinitions[0].Image))
	require.Equal(t, "amazon/aws-xray-daemon", aws.ToString(in.ContainerDefinitions[1].Image))
}

// TestRegisterRevisionRefusesToRegisterAnIdenticalCopy pins the case where no
// container matches: re-registering would look like a successful deploy and
// change nothing.
//
// The error *class* matters as much as the error. This condition is a
// configuration fact — the family's containers come from somewhere else — so it
// will fail identically on every attempt and every redelivery. Asserting only
// that it errors is what let it be classified retryable.
func TestRegisterRevisionRefusesToRegisterAnIdenticalCopy(t *testing.T) {
	api := &fakeAPI{describeOut: taskDefinition()}

	_, err := newClient(api, false).RegisterRevisionWithImage(context.Background(),
		"acme_task_cleanup_dev",
		"000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_task_other:new")
	require.ErrorContains(t, err, "no container")
	require.ErrorIs(t, err, awsecs.ErrPermanent)
	require.Empty(t, api.registerIn)
}

// TestPermanentErrorsAreTyped enumerates every failure this package raises
// itself. All of them were bare fmt.Errorf values, indistinguishable from a
// transient AWS fault to anything downstream.
func TestPermanentErrorsAreTyped(t *testing.T) {
	t.Run("no service name", func(t *testing.T) {
		_, err := newClient(&fakeAPI{}, false).UpdateService(context.Background(), awsecs.UpdateRequest{})
		require.ErrorIs(t, err, awsecs.ErrPermanent)
	})

	t.Run("no family", func(t *testing.T) {
		_, err := newClient(&fakeAPI{}, false).RegisterRevisionWithImage(context.Background(), "", "img")
		require.ErrorIs(t, err, awsecs.ErrPermanent)
	})

	t.Run("no image", func(t *testing.T) {
		_, err := newClient(&fakeAPI{}, false).RegisterRevisionWithImage(context.Background(), "fam", "")
		require.ErrorIs(t, err, awsecs.ErrPermanent)
	})

	t.Run("describe returned nothing", func(t *testing.T) {
		api := &fakeAPI{describeOut: &ecs.DescribeTaskDefinitionOutput{}}
		_, err := newClient(api, false).RegisterRevisionWithImage(context.Background(), "fam", "repo:tag")
		require.ErrorIs(t, err, awsecs.ErrPermanent)
	})

	t.Run("register returned nothing", func(t *testing.T) {
		api := &fakeAPI{
			describeOut: taskDefinition(),
			registerOut: &ecs.RegisterTaskDefinitionOutput{},
		}
		_, err := newClient(api, false).RegisterRevisionWithImage(context.Background(),
			"acme_task_cleanup_dev",
			"000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_task_cleanup:new")
		require.ErrorIs(t, err, awsecs.ErrPermanent)
	})
}

func TestRegisterRevisionHandlesDigestReferences(t *testing.T) {
	desc := taskDefinition()
	desc.TaskDefinition.ContainerDefinitions[0].Image = aws.String(
		"000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_task_cleanup@sha256:" +
			"1111111111111111111111111111111111111111111111111111111111111111")

	api := &fakeAPI{
		describeOut: desc,
		registerOut: &ecs.RegisterTaskDefinitionOutput{
			TaskDefinition: &types.TaskDefinition{TaskDefinitionArn: aws.String("arn:task:2")},
		},
	}

	newImage := "000000000000.dkr.ecr.us-east-1.amazonaws.com/acme_task_cleanup@sha256:" +
		"2222222222222222222222222222222222222222222222222222222222222222"

	_, err := newClient(api, false).RegisterRevisionWithImage(context.Background(), "acme_task_cleanup_dev", newImage)
	require.NoError(t, err)
	require.Equal(t, newImage, aws.ToString(api.registerIn[0].ContainerDefinitions[0].Image))
}

func TestRegisterRevisionRequiresAnImage(t *testing.T) {
	api := &fakeAPI{}
	_, err := newClient(api, false).RegisterRevisionWithImage(context.Background(), "acme_task_cleanup_dev", "")
	require.ErrorContains(t, err, "image URI is required")
	require.Empty(t, api.describeIn)
}

func TestRegisterRevisionPropagatesDescribeErrors(t *testing.T) {
	api := &fakeAPI{describeErr: errors.New("boom")}
	_, err := newClient(api, false).RegisterRevisionWithImage(context.Background(), "acme_task_cleanup_dev", "repo:tag")
	require.ErrorContains(t, err, "boom")
}

func TestRegisterRevisionDryRunMakesNoCalls(t *testing.T) {
	api := &fakeAPI{}
	arn, err := newClient(api, true).RegisterRevisionWithImage(context.Background(), "acme_task_cleanup_dev", "repo:tag")
	require.NoError(t, err)
	require.Equal(t, "acme_task_cleanup_dev:DRY_RUN", arn)
	require.Empty(t, api.describeIn)
	require.Empty(t, api.registerIn)
}
