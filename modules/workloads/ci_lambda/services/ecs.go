package services

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
	"madappgang.com/infrastructure/ci_lambda/config"
	"madappgang.com/infrastructure/ci_lambda/utils"
)

// ECSServiceV2 provides operations for ECS deployments using actual resource names
type ECSServiceV2 struct {
	client *ecs.ECS
	config *config.Config
	logger *utils.Logger
}

// NewECSServiceV2 creates a new ECS service client with direct name lookups
func NewECSServiceV2(cfg *config.Config, logger *utils.Logger) (*ECSServiceV2, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(cfg.AWSRegion),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	return &ECSServiceV2{
		client: ecs.New(sess),
		config: cfg,
		logger: logger,
	}, nil
}

// DeploymentRequest represents a deployment operation
type DeploymentRequest struct {
	ServiceIdentifier string // Service ID from event (empty string for backend, "api" for api service, etc.)
	TaskDefinition    string // Optional: if empty, uses latest
	ForceNewDeploy    bool
	DesiredCount      *int64 // Optional: if nil, keeps current count
}

// DeploymentResult contains the result of a deployment
type DeploymentResult struct {
	ServiceIdentifier string
	ServiceName       string
	ClusterName       string
	TaskDefinition    string
	DeploymentID      string
	Status            string
	Message           string
}

// Deploy updates an ECS service with a new task definition or forces redeployment
func (s *ECSServiceV2) Deploy(req DeploymentRequest) (*DeploymentResult, error) {
	log := s.logger.WithFields(map[string]interface{}{
		"service_id": req.ServiceIdentifier,
		"action":     "deploy",
	})

	// Look up actual ECS resource names from config
	mapping, err := s.config.GetServiceMapping(req.ServiceIdentifier)
	if err != nil {
		log.Error("Service not found in configuration", map[string]interface{}{
			"error":      err.Error(),
			"service_id": req.ServiceIdentifier,
		})
		return nil, fmt.Errorf("service not found: %w", err)
	}

	clusterName := s.config.GetClusterName()
	ecsServiceName := mapping.ServiceName
	taskFamily := mapping.TaskFamily

	log.Info("Starting deployment", map[string]interface{}{
		"cluster":      clusterName,
		"ecs_service":  ecsServiceName,
		"task_family":  taskFamily,
		"service_id":   req.ServiceIdentifier,
	})

	// If task definition not specified, find the latest one
	taskDefinitionArn := req.TaskDefinition
	if taskDefinitionArn == "" {
		var err error
		taskDefinitionArn, err = s.getLatestTaskDefinition(taskFamily)
		if err != nil {
			log.Error("Failed to get latest task definition", map[string]interface{}{
				"error":       err.Error(),
				"task_family": taskFamily,
			})
			return nil, fmt.Errorf("failed to get latest task definition: %w", err)
		}
		log.Info("Using latest task definition", map[string]interface{}{
			"task_definition": taskDefinitionArn,
		})
	}

	// Dry run check
	if s.config.DryRun {
		log.Info("DRY RUN: Would deploy service", map[string]interface{}{
			"cluster":         clusterName,
			"service":         ecsServiceName,
			"task_definition": taskDefinitionArn,
			"force_new":       req.ForceNewDeploy,
		})
		return &DeploymentResult{
			ServiceIdentifier: req.ServiceIdentifier,
			ServiceName:       ecsServiceName,
			ClusterName:       clusterName,
			TaskDefinition:    taskDefinitionArn,
			Status:            "DRY_RUN",
			Message:           "Dry run successful - no actual deployment performed",
		}, nil
	}

	// Build update input
	updateInput := &ecs.UpdateServiceInput{
		Cluster:            aws.String(clusterName),
		Service:            aws.String(ecsServiceName),
		TaskDefinition:     aws.String(taskDefinitionArn),
		ForceNewDeployment: aws.Bool(req.ForceNewDeploy),
	}

	if req.DesiredCount != nil {
		updateInput.DesiredCount = req.DesiredCount
	}

	// Perform the update
	output, err := s.client.UpdateService(updateInput)
	if err != nil {
		log.Error("Failed to update service", map[string]interface{}{
			"error": err.Error(),
		})
		return nil, fmt.Errorf("failed to update ECS service: %w", err)
	}

	// Extract deployment info
	var deploymentID string
	if output.Service != nil && len(output.Service.Deployments) > 0 {
		// Get the primary deployment (most recent)
		for _, deployment := range output.Service.Deployments {
			if aws.StringValue(deployment.Status) == "PRIMARY" {
				deploymentID = aws.StringValue(deployment.Id)
				break
			}
		}
	}

	result := &DeploymentResult{
		ServiceIdentifier: req.ServiceIdentifier,
		ServiceName:       ecsServiceName,
		ClusterName:       clusterName,
		TaskDefinition:    taskDefinitionArn,
		DeploymentID:      deploymentID,
		Status:            "DEPLOYED",
		Message:           fmt.Sprintf("Successfully deployed %s to %s", ecsServiceName, clusterName),
	}

	log.Info("Deployment successful", map[string]interface{}{
		"deployment_id":   deploymentID,
		"task_definition": taskDefinitionArn,
	})

	return result, nil
}

// getLatestTaskDefinition finds the latest task definition for a family
func (s *ECSServiceV2) getLatestTaskDefinition(taskFamily string) (string, error) {
	s.logger.Debug("Listing task definitions", map[string]interface{}{
		"family": taskFamily,
	})

	output, err := s.client.ListTaskDefinitions(&ecs.ListTaskDefinitionsInput{
		FamilyPrefix: aws.String(taskFamily),
		Sort:         aws.String("DESC"),
		MaxResults:   aws.Int64(10), // Get top 10 to ensure we have the latest
	})
	if err != nil {
		return "", fmt.Errorf("failed to list task definitions: %w", err)
	}

	if len(output.TaskDefinitionArns) == 0 {
		return "", fmt.Errorf("no task definitions found for family: %s", taskFamily)
	}

	// Sort to ensure we get the latest (already sorted DESC by AWS, but being defensive)
	taskDefinitions := aws.StringValueSlice(output.TaskDefinitionArns)
	sort.SliceStable(taskDefinitions, func(i, j int) bool {
		return strings.Compare(taskDefinitions[i], taskDefinitions[j]) > 0
	})

	latestTaskDef := taskDefinitions[0]

	s.logger.Debug("Found latest task definition", map[string]interface{}{
		"task_definition": latestTaskDef,
		"total_found":     len(taskDefinitions),
	})

	return latestTaskDef, nil
}

// DescribeService gets detailed information about an ECS service
func (s *ECSServiceV2) DescribeService(serviceIdentifier string) (*ecs.Service, error) {
	serviceName, err := s.config.GetServiceName(serviceIdentifier)
	if err != nil {
		return nil, err
	}

	clusterName := s.config.GetClusterName()

	output, err := s.client.DescribeServices(&ecs.DescribeServicesInput{
		Cluster:  aws.String(clusterName),
		Services: []*string{aws.String(serviceName)},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe service: %w", err)
	}

	if len(output.Services) == 0 {
		return nil, fmt.Errorf("service not found: %s", serviceName)
	}

	return output.Services[0], nil
}

// ListAllServices returns information about all configured services
func (s *ECSServiceV2) ListAllServices() (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for _, serviceID := range s.config.ListAllServices() {
		result[serviceID] = s.config.GetServiceInfo(serviceID)
	}

	return result, nil
}

// VerifyServiceExists checks if a service exists in ECS
func (s *ECSServiceV2) VerifyServiceExists(serviceIdentifier string) error {
	_, err := s.DescribeService(serviceIdentifier)
	return err
}
