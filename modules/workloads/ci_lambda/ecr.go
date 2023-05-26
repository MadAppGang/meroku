package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
)

type ECRImagePushEventDetail struct {
	RepositoryName string `json:"repository-name"`
	Tag            string `json:"image-tag"`
	Action         string `json:"action-type"`
	Result         string `json:"result"`
}

func processECREvent(srv Service, ctx context.Context, e events.CloudWatchEvent) (string, error) {
	var detail ECRImagePushEventDetail
	err := json.Unmarshal(e.Detail, &detail)
	if err != nil {
		return "", fmt.Errorf("could not unmarshal event detail: %v", err)
	}

	fmt.Printf("New image pushed to ECR repository: %s with tag: %s.\n", detail.RepositoryName, detail.Tag)
	if detail.Action != "PUSH" {
		return fmt.Sprintf("Skipping event with action: %s", detail.Action), nil
	}

	if detail.Result != "SUCCESS" {
		return fmt.Sprintf("Skipping event with result: %s", detail.Result), nil
	}

	serviceName, err := getServiceNameFromRepoName(detail.RepositoryName)
	if err != nil {
		return "", fmt.Errorf("unable to extract service name from repo name: %s", detail.RepositoryName)
	}

	// Listing all task definitions with the specific family prefix
	taskList, err := srv.ListTaskDefinitions(&ecs.ListTaskDefinitionsInput{
		FamilyPrefix: &serviceName,
		Sort:         aws.String("DESC"),
	})

	if err != nil || len(taskList.TaskDefinitionArns) == 0 {
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
	clusterName := fmt.Sprintf("%s_cluster_dev", ProjectName)
	serviceName = fmt.Sprintf("%s_service_dev", serviceName)

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

	result := fmt.Sprintf("Processed ECR event and updated ECS service: %s with the latest task definition", e.ID)
	fmt.Println(result)

	return result, nil
}

func getServiceNameFromRepoName(str string) (string, error) {
	re := regexp.MustCompile(`\w+_(?P<service>\w+)`)
	match := re.FindStringSubmatch(str)
	if len(match) == 2 {
		return match[1], nil
	}
	return "", errors.New("Unable to extract service name")
}
