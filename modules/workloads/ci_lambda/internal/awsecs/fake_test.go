package awsecs_test

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

// fakeAPI records calls and replays canned responses. Every test in this
// package runs against it: no credentials, no network.
type fakeAPI struct {
	updateIn  []*ecs.UpdateServiceInput
	updateOut *ecs.UpdateServiceOutput
	updateErr error

	describeIn  []*ecs.DescribeTaskDefinitionInput
	describeOut *ecs.DescribeTaskDefinitionOutput
	describeErr error

	registerIn  []*ecs.RegisterTaskDefinitionInput
	registerOut *ecs.RegisterTaskDefinitionOutput
	registerErr error
}

func (f *fakeAPI) UpdateService(_ context.Context, in *ecs.UpdateServiceInput, _ ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error) {
	f.updateIn = append(f.updateIn, in)
	return f.updateOut, f.updateErr
}

func (f *fakeAPI) DescribeTaskDefinition(_ context.Context, in *ecs.DescribeTaskDefinitionInput, _ ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error) {
	f.describeIn = append(f.describeIn, in)
	return f.describeOut, f.describeErr
}

func (f *fakeAPI) RegisterTaskDefinition(_ context.Context, in *ecs.RegisterTaskDefinitionInput, _ ...func(*ecs.Options)) (*ecs.RegisterTaskDefinitionOutput, error) {
	f.registerIn = append(f.registerIn, in)
	return f.registerOut, f.registerErr
}
