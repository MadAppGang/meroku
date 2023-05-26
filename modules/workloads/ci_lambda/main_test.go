package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/stretchr/testify/assert"
)

type MockService struct {
	usi *ecs.UpdateServiceInput
}

func (s *MockService) ListTaskDefinitions(input *ecs.ListTaskDefinitionsInput) (*ecs.ListTaskDefinitionsOutput, error) {
	return &ecs.ListTaskDefinitionsOutput{
		TaskDefinitionArns: []*string{
			aws.String("arn:aws:ecs:us-east-1:798135304365:task-definition/backend:3"),
		},
	}, nil
}

func (s *MockService) UpdateService(input *ecs.UpdateServiceInput) (*ecs.UpdateServiceOutput, error) {
	s.usi = input
	return &ecs.UpdateServiceOutput{}, nil
}

func Test_handleRequestECR(t *testing.T) {
	ProjectName = "chubby"
	var e events.CloudWatchEvent
	err := json.Unmarshal([]byte(ecr_event), &e)
	assert.NoError(t, err)
	assert.Equal(t, "012345678912", e.AccountID)
	assert.Equal(t, "aws.ecr", e.Source)

	srv := MockService{}
	handler := Handler(&srv)
	result, err := handler(context.TODO(), e)
	assert.NoError(t, err)
	assert.Contains(t, result, "Processed ECR event and updated ECS service:")

	assert.NotNil(t, srv.usi)
	assert.Equal(t, "backend_service_dev", *srv.usi.Service)
	assert.Equal(t, "chubby_cluster_dev", *srv.usi.Cluster)
	assert.Equal(t, "arn:aws:ecs:us-east-1:798135304365:task-definition/backend:3", *srv.usi.TaskDefinition)
}

const ecr_event = `
{
  "version": "0",
  "id": "01234567-0123-0123-0123-012345678912",
  "detail-type": "ECR Image Action",
  "source": "aws.ecr",
  "account": "012345678912",
  "time": "2017-10-06T19:49:24Z",
  "region": "us-west-2",
  "resources": [
    "arn:aws:ecr:us-west-2:012345678912:repository/my-repo"
  ],
  "detail": {
    "action-type": "PUSH",
    "result": "SUCCESS",
    "repository-name": "chubby_backend",
    "image-tag": "latest",
    "image-digest": "sha256:0123456789abcdef0123456789abcdef",
    "actor": "012345678912"
  }
}
`

const ecs_event_success = `
{
   "version": "0",
   "id": "ddca6449-b258-46c0-8653-e0e3aEXAMPLE",
   "detail-type": "ECS Deployment State Change",
   "source": "aws.ecs",
   "account": "111122223333",
   "time": "2020-05-23T12:31:14Z",
   "region": "us-west-2",
   "resources": [ 
        "arn:aws:ecs:us-west-2:111122223333:service/default/servicetest"
   ],
   "detail": {
        "eventType": "INFO", 
        "eventName": "SERVICE_DEPLOYMENT_COMPLETED",
        "deploymentId": "ecs-svc/123",
        "updatedAt": "2020-05-23T11:11:11Z",
        "reason": "ECS deployment deploymentID completed."
   }
}
`

const ecs_event_failed = `
{
   "version": "0",
   "id": "ddca6449-b258-46c0-8653-e0e3aEXAMPLE",
   "detail-type": "ECS Deployment State Change",
   "source": "aws.ecs",
   "account": "111122223333",
   "time": "2020-05-23T12:31:14Z",
   "region": "us-west-2",
   "resources": [ 
        "arn:aws:ecs:us-west-2:111122223333:service/default/servicetest"
   ],
   "detail": {
        "eventType": "ERROR", 
        "eventName": "SERVICE_DEPLOYMENT_FAILED",
        "deploymentId": "ecs-svc/123",
        "updatedAt": "2020-05-23T11:11:11Z",
        "reason": "ECS deployment circuit breaker: task failed to start."
   }
}
`
