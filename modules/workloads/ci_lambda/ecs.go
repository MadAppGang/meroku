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
var successTmpl, _ = template.New("success").Parse(errorJson)

type ECSServiceDeployEvent struct {
	EventType    string `json:"eventType"`
	EventName    string `json:"eventName"`
	Reason       string `json:"reason"`
	DeploymentID string `json:"deploymentId"`
}

type templateData struct {
	Service string
	Reason  string
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
	fmt.Printf("New ECS deployment event: %s with resource: %s.\n", detail.EventType, resource)

	if detail.EventType != "SERVICE_DEPLOYMENT_COMPLETED" &&
		detail.EventType != "SERVICE_DEPLOYMENT_FAILED" {
		return "", fmt.Errorf("skipping event type: %s", detail.EventType)
	}

	data := templateData{
		Service: resource,
		Reason:  detail.Reason,
	}

	var payload bytes.Buffer

	t := successTmpl
	if detail.EventType == "SERVICE_DEPLOYMENT_FAILED" {
		t = errorTmpl
	}

	if err := t.Execute(&payload, data); err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, SlackWebhookURL, bytes.NewReader(payload.Bytes()))
	if err != nil {
		return "", err
	}

	if req.Response.StatusCode < 200 || req.Response.StatusCode > 299 {
		return "", fmt.Errorf("could not send slack message: %s", req.Response.Status)
	}

	return fmt.Sprintf("sent slack message for %s", detail.EventType), nil
}
