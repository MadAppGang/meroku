package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/aws/aws-lambda-go/events"
	"madappgang.com/infrastructure/ci_lambda/utils"
)

type SSMEventDetail struct {
	Operation   string `json:"operation"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

func processSSMEvent(srv utils.Service, ctx context.Context, e events.CloudWatchEvent) (string, error) {
	var detail SSMEventDetail
	err := json.Unmarshal(e.Detail, &detail)
	if err != nil {
		return "", fmt.Errorf("could not unmarshal event detail: %v", err)
	}
	fmt.Printf("SSM parameter change event %s for parameter %s.\n", detail.Operation, detail.Name)

	// project name:
	//"$env/$project/$service/xxxxxx"
	re := regexp.MustCompile(fmt.Sprintf(`\/?%s\/%s\/(\w+)\/\w+$`, Env, ProjectName))
	match := re.FindStringSubmatch(detail.Name)
	if len(match) == 2 {
		serviceName := match[1]
		// backend service is default service
		if serviceName == "backend" {
			serviceName = ProjectName
		}
		fmt.Printf("env variables in SSM key %s changed (%s) for service %s\n", detail.Name, detail.Operation, serviceName)
		return utils.Deploy(srv, serviceName)
	}

	result := fmt.Sprintf("SSM parameter with key %s does not fit to any service environment, skipping", detail.Name)
	fmt.Println(result)

	return result, nil
}
