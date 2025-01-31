package utils

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
)

func Deploy(srv Service, serviceName string) (string, error) {
	Env := os.Getenv("PROJECT_ENV")
	ProjectName := os.Getenv("PROJECT_NAME")
	if ProjectName == "" {
		return "", fmt.Errorf("PROJECT_NAME environment variable is not set")
	}
	// Listing all task definitions with the specific family prefix
	fmt.Printf("deploying service %s for env %s\n", serviceName, Env)

	familyPrefix := fmt.Sprintf("%s_service_%s_%s", ProjectName, serviceName, Env)
	// for backend service
	if len(serviceName) == 0 {
		familyPrefix = fmt.Sprintf("%s_service_%s", ProjectName, Env)
	}

	taskList, err := srv.ListTaskDefinitions(&ecs.ListTaskDefinitionsInput{
		FamilyPrefix: &familyPrefix,
		Sort:         aws.String("DESC"),
	})
	if err != nil {
		return "", fmt.Errorf("unable to retrieve task definitions: %v", err)
	}

	if len(taskList.TaskDefinitionArns) == 0 {
		return "", fmt.Errorf("no task definitions found for family prefix: %v", familyPrefix)
	}

	// Parsing out the latest task definition
	taskDefinitions := aws.StringValueSlice(taskList.TaskDefinitionArns)
	sort.SliceStable(taskDefinitions, func(i, j int) bool {
		return strings.Compare(taskDefinitions[i], taskDefinitions[j]) > 0
	})
	latestTaskDefinition := taskDefinitions[0]

	fmt.Println("latest task definition found: ", latestTaskDefinition)

	clusterName := fmt.Sprintf("%s_cluster_%s", ProjectName, Env)
	fmt.Printf("deploying service %s for in cluster %s\n", familyPrefix, clusterName)

	// Updating the ECS service with the latest task definition revision
	u, err := srv.UpdateService(&ecs.UpdateServiceInput{
		Service:            &familyPrefix,
		Cluster:            &clusterName,
		TaskDefinition:     &latestTaskDefinition,
		ForceNewDeployment: aws.Bool(true),
	})
	if err != nil {
		return "", fmt.Errorf("unable to update ECS service: %v", err)
	}

	fmt.Printf("updated ECS service: %s with result %+v\n", familyPrefix, u)
	result := fmt.Sprintf("Processed ECR event and updated ECS service: %s with the latest task definition %s", familyPrefix, latestTaskDefinition)
	fmt.Println(result)

	return result, nil
}
