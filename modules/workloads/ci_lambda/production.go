package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
)

type DeployEventDetail struct {
	Service string `json:"service"`
}

func processProductionDeployEvent(srv Service, ctx context.Context, e events.CloudWatchEvent) (string, error) {
	var detail DeployEventDetail
	err := json.Unmarshal(e.Detail, &detail)
	if err != nil {
		return "", fmt.Errorf("could not unmarshal event detail: %v", err)
	}
	fmt.Printf("New deploy command for service %s.\n", detail.Service)
	return deploy(srv, detail.Service)
}
