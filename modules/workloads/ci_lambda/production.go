package main

import (
	"context"

	"github.com/aws/aws-lambda-go/events"
)

func processProductionDeployEvent(srv Service, ctx context.Context, e events.CloudWatchEvent) (string, error) {
	// TODO: implement
	return "not implemented yet", nil
}
