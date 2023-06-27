package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
)

//go:embed slack.message.error.json.tmpl
var errorJson string
var errorTmpl, _ = template.New("error").Parse(errorJson)

//go:embed slack.message.success.json.tmpl
var successJson string
var successTmpl, _ = template.New("success").Parse(successJson)

//go:embed slack.message.info.json.tmpl
var infoJson string
var infoTmpl, _ = template.New("info").Parse(infoJson)

type ECSServiceDeployEvent struct {
	EventType    ECSEventType `json:"eventType"` // INFO or
	EventName    ECSEventName `json:"eventName"`
	Reason       string       `json:"reason"`
	DeploymentID string       `json:"deploymentId"`
}

type (
	ECSEventType string
	ECSEventName string
)

const (
	ECSEventTypeError                             ECSEventType = "ERROR"
	ECSEventTypeInfo                              ECSEventType = "INFO"
	ECSEventTypeWarn                              ECSEventType = "WARN"
	ECSEventNameInProgress                        ECSEventName = "SERVICE_DEPLOYMENT_IN_PROGRESS"
	ECSEventNameCompleted                         ECSEventName = "SERVICE_DEPLOYMENT_COMPLETED"
	ECSEventNameFailed                            ECSEventName = "SERVICE_DEPLOYMENT_FAILED"
	ECSEventNameServiceSteady                     ECSEventName = "SERVICE_STEADY_STATE"        // service got to desired capacity state, and consider green
	ECSEventNameServiceTaskImpaired               ECSEventName = "SERVICE_TASK_START_IMPAIRED" // The service is unable to consistently start tasks successfully.
	ECSEventNameServiceDiscoveryInstanceUnhealthy ECSEventName = "SERVICE_DISCOVERY_INSTANCE_UNHEALTHY"
)

type templateData struct {
	Env       string
	Service   string
	Reason    string
	StateName string
}

func processECSEvent(srv Service, ctx context.Context, e events.CloudWatchEvent) (string, error) {
	if len(SlackWebhookURL) == 0 {
		return "no webhook setup, ignoring service deployment event", nil
	}

	var detail ECSServiceDeployEvent
	err := json.Unmarshal(e.Detail, &detail)
	if err != nil {
		return "", fmt.Errorf("could not unmarshal event detail: %v", err)
	}

	resource := ""
	if len(e.Resources) > 0 {
		resource = e.Resources[0]
	}
	fmt.Printf("New ECS deployment event type: %s, with name: %s with resource: %s.\n", detail.EventType, detail.EventName, resource)

	data := templateData{
		Service:   resource,
		Reason:    detail.Reason,
		StateName: string(detail.EventName),
		Env:       Env,
	}

	var payload bytes.Buffer
	var t *template.Template
	switch detail.EventName {
	case ECSEventNameFailed:
		t = errorTmpl
	case ECSEventNameCompleted:
		t = successTmpl
	case ECSEventNameServiceSteady:
		return "Ignoring SERVICE_STEADY_STATE, as it produces too much noise!", nil
	default:
		t = infoTmpl
	}

	if err := t.Execute(&payload, data); err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, SlackWebhookURL, bytes.NewReader(payload.Bytes()))
	if err != nil {
		return "", err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("could not send slack message: %s", resp.Status)
	}

	result := fmt.Sprintf("sent slack message for %s and %s.", detail.EventType, detail.EventName)
	fmt.Println(result)

	return result, nil
}

// Service steady state
// {
//     "version": "0",
//     "id": "af3c496d-f4a8-65d1-70f4-a69d52e9b584",
//     "detail-type": "ECS Service Action",
//     "source": "aws.ecs",
//     "account": "111122223333",
//     "time": "2019-11-19T19:27:22Z",
//     "region": "us-west-2",
//     "resources": [
//         "arn:aws:ecs:us-west-2:111122223333:service/default/servicetest"
//     ],
//     "detail": {
//         "eventType": "INFO",
//         "eventName": "SERVICE_STEADY_STATE",
//         "clusterArn": "arn:aws:ecs:us-west-2:111122223333:cluster/default",
//         "createdAt": "2019-11-19T19:27:22.695Z"
//     }
// }
