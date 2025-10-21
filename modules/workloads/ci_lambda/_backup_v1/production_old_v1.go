package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"madappgang.com/infrastructure/ci_lambda/utils"
)

type DeployEventDetail struct {
	Service string `json:"service"`
}

func processProductionDeployEvent(srv utils.Service, ctx context.Context, e events.CloudWatchEvent) (string, error) {
	var detail DeployEventDetail
	err := json.Unmarshal(e.Detail, &detail)
	if err != nil {
		return "", fmt.Errorf("could not unmarshal event detail: %v", err)
	}
	fmt.Printf("New deploy command for service %s.\n", detail.Service)
	return utils.Deploy(srv, detail.Service)
}
