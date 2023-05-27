package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/aws/aws-lambda-go/events"
)

type SSMEventDetail struct {
	Operation   string `json:"operation"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

func processSSMEvent(srv Service, ctx context.Context, e events.CloudWatchEvent) (string, error) {
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
		fmt.Printf("env variables in SSM key %s changed (%s) for service %s", detail.Name, detail.Operation, match[1])
		return deploy(srv, match[1])
	}

	result := fmt.Sprintf("SSM parameter with key %s does not fit to any service environment, skipping", detail.Name)
	fmt.Println(result)

	return result, nil
}
