package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
)

func deploy(srv Service, serviceName string) (string, error) {
	// Listing all task definitions with the specific family prefix
  fmt.Printf("deploying service %s for env %s", serviceName, Env)

	familyPrefix := fmt.Sprintf("%s_service_%s", serviceName, Env)

	taskList, err := srv.ListTaskDefinitions(&ecs.ListTaskDefinitionsInput{
		FamilyPrefix: &familyPrefix,
		Sort:         aws.String("DESC"),
	})

  if len(taskList.TaskDefinitionArns) == 0 {
    return "", fmt.Errorf("not task definitions for family prefix: %v", familyPrefix)
  }

	if err != nil { 
		return "", fmt.Errorf("unable to retrieve task definitions: %v", err)
	}

	// Parsing out the latest task definition
	taskDefinitions := aws.StringValueSlice(taskList.TaskDefinitionArns)
	sort.SliceStable(taskDefinitions, func(i, j int) bool {
		return strings.Compare(taskDefinitions[i], taskDefinitions[j]) > 0
	})
	latestTaskDefinition := taskDefinitions[0]

	if err != nil {
		return "", fmt.Errorf("unable to extract service name from arn: %s", latestTaskDefinition)
	}
	clusterName := fmt.Sprintf("%s_cluster_%s", ProjectName, Env)
	serviceName = fmt.Sprintf("%s_service_%s", serviceName, Env)

	// Updating the ECS service with the latest task definition revision
	_, err = srv.UpdateService(&ecs.UpdateServiceInput{
		Service:            &serviceName,
		Cluster:            &clusterName,
		TaskDefinition:     &latestTaskDefinition,
		ForceNewDeployment: aws.Bool(true),
	})

	if err != nil {
		return "", fmt.Errorf("unable to update ECS service: %v", err)
	}

	result := fmt.Sprintf("Processed ECR event and updated ECS service: %s with the latest task definition %s", serviceName, latestTaskDefinition)
	fmt.Println(result)

	return result, nil
}
