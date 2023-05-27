package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/aws/aws-lambda-go/events"
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

	return deploy(srv, serviceName)
}

func getServiceNameFromRepoName(str string) (string, error) {
	re := regexp.MustCompile(`\w+_(?P<service>\w+)`)
	match := re.FindStringSubmatch(str)
	if len(match) == 2 {
		return match[1], nil
	}
	return "", errors.New("Unable to extract service name")
}
