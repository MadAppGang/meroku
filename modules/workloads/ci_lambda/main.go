package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

var (
	ProjectName     = os.Getenv("PROJECT_NAME")
	SlackWebhookURL = os.Getenv("SLACK_WEBHOOK_URL")
	Env             = os.Getenv("PROJECT_ENV")
)

func Handler(srv Service) func(ctx context.Context, e events.CloudWatchEvent) (string, error) {
	return func(ctx context.Context, e events.CloudWatchEvent) (string, error) {
		fmt.Printf("Processing request data for event %s.\n", e.ID)

		switch e.Source {
		case "aws.ecr":
			return processECREvent(srv, ctx, e)
		case "aws.ecs":
			return processECSEvent(srv, ctx, e)
		case "action.production":
			return processProductionDeployEvent(srv, ctx, e)
		case "aws.ssm":
			return processSSMEvent(srv, ctx, e)
		}

		return "", fmt.Errorf("unable to process event: %s, unsupported event source: %s", e.ID, e.Source)
	}
}

func getServiceName(str string) (string, error) {
	re := regexp.MustCompile(`\d{12}\.dkr\.ecr\.(\w|-)+\.amazonaws.com\/\w+_(?P<service>\w+)`)
	match := re.FindStringSubmatch(str)
	if len(match) == 3 {
		return match[2], nil
	}
	return "", errors.New("Unable to extract service name")
}

func main() {
	lambda.Start(Handler(NewAWSService()))
}
