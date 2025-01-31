package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/events"
)

var ProjectName = os.Getenv("PROJECT_NAME")

type ECRImagePushEventDetail struct {
	RepositoryName string `json:"repository-name"`
	Tag            string `json:"image-tag"`
	Action         string `json:"action-type"`
	Result         string `json:"result"`
}

func ProcessECREvent(srv Service, _ context.Context, e events.CloudWatchEvent) (string, error) {
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

	serviceName, err := GetServiceNameFromRepoName(detail.RepositoryName, ProjectName)
	if err != nil {
		return "", fmt.Errorf("Unable to extract service name from repo name, assuming it not a service: %s", detail.RepositoryName)
	}

	return Deploy(srv, serviceName)
}
