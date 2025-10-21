package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"madappgang.com/infrastructure/ci_lambda/utils"
)

type S3File struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
}

type ServiceConfig map[string][]S3File

var serviceConfig ServiceConfig

func init() {
	configJSON := os.Getenv("SERVICE_CONFIG")
	if configJSON == "" {
		panic("SERVICE_CONFIG environment variable not set")
	}

	if err := json.Unmarshal([]byte(configJSON), &serviceConfig); err != nil {
		panic(fmt.Sprintf("Failed to parse SERVICE_CONFIG: %v", err))
	}
}

// Add new function
type UpdateResult struct {
	Message string
	Error   error
}

func processS3Event(srv utils.Service, ctx context.Context, e events.CloudWatchEvent) (string, error) {
	var s3Event struct {
		RequestParameters map[string]string `json:"requestParameters"`
	}

	if err := json.Unmarshal(e.Detail, &s3Event); err != nil {
		return "", fmt.Errorf("failed to parse S3 event: %v", err)
	}

	bucketName := s3Event.RequestParameters["bucketName"]
	objectKey := s3Event.RequestParameters["key"]

	// Collect results for all affected services
	var results []UpdateResult
	// Find service associated with this file
	for service, files := range serviceConfig {
		for _, file := range files {
			if file.Bucket == bucketName && file.Key == objectKey {
				message, err := utils.Deploy(srv, service)
				result := UpdateResult{
					Message: message,
					Error:   err,
				}
				results = append(results, result)
			}
		}
	}

	if len(results) == 0 {
		return "", fmt.Errorf("no service found for env file s3://%s/%s", bucketName, objectKey)
	}

	r, err := json.Marshal(results)
	if err != nil {
		return "", fmt.Errorf("failed to marshal results: %v", err)
	}
	return string(r), nil
}
