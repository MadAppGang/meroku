package utils_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/stretchr/testify/assert"

	"madappgang.com/infrastructure/ci_lambda/utils"
)

type MockService struct {
	listTaskDefinitionsOutput *ecs.ListTaskDefinitionsOutput
	updateServiceOutput       *ecs.UpdateServiceOutput
	listTaskDefinitionsError  error
	updateServiceError        error
}

func (m *MockService) ListTaskDefinitions(input *ecs.ListTaskDefinitionsInput) (*ecs.ListTaskDefinitionsOutput, error) {
	if m.listTaskDefinitionsError != nil {
		return nil, m.listTaskDefinitionsError
	}
	return m.listTaskDefinitionsOutput, nil
}

func (m *MockService) UpdateService(input *ecs.UpdateServiceInput) (*ecs.UpdateServiceOutput, error) {
	if m.updateServiceError != nil {
		return nil, m.updateServiceError
	}
	return m.updateServiceOutput, nil
}

func TestDeploy(t *testing.T) {
	// Save original env vars and restore after tests
	originalEnv := os.Getenv("PROJECT_ENV")
	originalProjectName := os.Getenv("PROJECT_NAME")
	defer func() {
		os.Setenv("PROJECT_ENV", originalEnv)
		os.Setenv("PROJECT_NAME", originalProjectName)
	}()

	os.Setenv("PROJECT_ENV", "test")
	os.Setenv("PROJECT_NAME", "project")

	tests := []struct {
		name           string
		serviceName    string
		mockService    *MockService
		expectedError  bool
		expectedResult string
		setup          func()
		cleanup        func()
	}{
		{
			name:          "missing PROJECT_NAME env var",
			serviceName:   "api",
			mockService:   &MockService{},
			expectedError: true,
			setup: func() {
				os.Unsetenv("PROJECT_NAME")
			},
			cleanup: func() {
				os.Setenv("PROJECT_NAME", "project")
			},
		},
		{
			name:        "successful deployment",
			serviceName: "api",
			mockService: &MockService{
				listTaskDefinitionsOutput: &ecs.ListTaskDefinitionsOutput{
					TaskDefinitionArns: []*string{
						aws.String("arn:aws:ecs:region:account:task-definition/project_service_api_test:2"),
						aws.String("arn:aws:ecs:region:account:task-definition/project_service_api_test:1"),
					},
				},
				updateServiceOutput: &ecs.UpdateServiceOutput{},
			},
			expectedError:  false,
			expectedResult: "Processed ECR event and updated ECS service: project_service_api_test with the latest task definition arn:aws:ecs:region:account:task-definition/project_service_api_test:2",
		},
		{
			name:        "no task definitions found",
			serviceName: "api",
			mockService: &MockService{
				listTaskDefinitionsOutput: &ecs.ListTaskDefinitionsOutput{
					TaskDefinitionArns: []*string{},
				},
			},
			expectedError: true,
		},
		{
			name:        "list task definitions error",
			serviceName: "api",
			mockService: &MockService{
				listTaskDefinitionsError: fmt.Errorf("AWS API error"),
			},
			expectedError: true,
		},
		{
			name:        "update service error",
			serviceName: "api",
			mockService: &MockService{
				listTaskDefinitionsOutput: &ecs.ListTaskDefinitionsOutput{
					TaskDefinitionArns: []*string{
						aws.String("arn:aws:ecs:region:account:task-definition/project_service_api_test:1"),
					},
				},
				updateServiceError: fmt.Errorf("update service failed"),
			},
			expectedError: true,
		},
		{
			name:        "backend service (empty service name)",
			serviceName: "",
			mockService: &MockService{
				listTaskDefinitionsOutput: &ecs.ListTaskDefinitionsOutput{
					TaskDefinitionArns: []*string{
						aws.String("arn:aws:ecs:region:account:task-definition/project_service_test:2"),
					},
				},
				updateServiceOutput: &ecs.UpdateServiceOutput{},
			},
			expectedError:  false,
			expectedResult: "Processed ECR event and updated ECS service: project_service_test with the latest task definition arn:aws:ecs:region:account:task-definition/project_service_test:2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}
			result, err := utils.Deploy(tt.mockService, tt.serviceName)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
			if tt.cleanup != nil {
				tt.cleanup()
			}
		})
	}
}
