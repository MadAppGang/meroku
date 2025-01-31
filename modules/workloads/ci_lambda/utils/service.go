package utils

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
)

type Service interface {
	ListTaskDefinitions(*ecs.ListTaskDefinitionsInput) (*ecs.ListTaskDefinitionsOutput, error)
	UpdateService(*ecs.UpdateServiceInput) (*ecs.UpdateServiceOutput, error)
}

type AWSService struct {
	e *ecs.ECS
}

func NewAWSService() *AWSService {
	sess := session.Must(session.NewSession())
	svc := ecs.New(sess)
	return &AWSService{e: svc}
}

func (s *AWSService) ListTaskDefinitions(input *ecs.ListTaskDefinitionsInput) (*ecs.ListTaskDefinitionsOutput, error) {
	return s.e.ListTaskDefinitions(input)
}

func (s *AWSService) UpdateService(input *ecs.UpdateServiceInput) (*ecs.UpdateServiceOutput, error) {
	return s.e.UpdateService(input)
}
