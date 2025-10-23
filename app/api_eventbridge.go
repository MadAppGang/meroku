package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"gopkg.in/yaml.v2"

	"madappgang.com/meroku/httputil"
	"madappgang.com/meroku/logger"
)

// TestEventRequest represents the request to send a test event
type TestEventRequest struct {
	Source     string                 `json:"source"`
	DetailType string                 `json:"detailType"`
	Detail     map[string]interface{} `json:"detail"`
}

// TestEventResponse represents the response after sending an event
type TestEventResponse struct {
	Success  bool   `json:"success"`
	EventId  string `json:"eventId,omitempty"`
	Message  string `json:"message"`
}

// sendTestEvent sends a test event to EventBridge
func sendTestEvent(w http.ResponseWriter, r *http.Request) {
	var req TestEventRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		logger.Error("Failed to decode test event request: %v", err)
		httputil.RespondJSON(w, http.StatusBadRequest, TestEventResponse{
			Success: false,
			Message: "Invalid request body",
		})
		return
	}

	// Validate required fields
	if req.Source == "" || req.DetailType == "" {
		logger.Warn("Missing required fields in test event request")
		httputil.RespondJSON(w, http.StatusBadRequest, TestEventResponse{
			Success: false,
			Message: "source and detailType are required",
		})
		return
	}

	logger.Info("Sending test event: source=%s, detailType=%s", req.Source, req.DetailType)

	// If detail is not provided, create a default test event detail
	if req.Detail == nil {
		req.Detail = map[string]interface{}{
			"test": true,
			"timestamp": time.Now().Format(time.RFC3339),
			"message": "Test event from infrastructure dashboard",
		}
	}

	// Create EventBridge client
	// TODO: Use ClientFactory in next phase when we add dependency injection
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		logger.Error("Failed to load AWS config: %v", err)
		httputil.RespondJSON(w, http.StatusInternalServerError, TestEventResponse{
			Success: false,
			Message: "Failed to load AWS configuration",
		})
		return
	}
	client := eventbridge.NewFromConfig(cfg)

	// Marshal detail to JSON string
	detailJSON, err := json.Marshal(req.Detail)
	if err != nil {
		logger.Error("Failed to marshal event detail: %v", err)
		httputil.RespondJSON(w, http.StatusInternalServerError, TestEventResponse{
			Success: false,
			Message: "Failed to marshal event detail",
		})
		return
	}

	// Create the event entry
	entry := types.PutEventsRequestEntry{
		Source:     aws.String(req.Source),
		DetailType: aws.String(req.DetailType),
		Detail:     aws.String(string(detailJSON)),
		Time:       aws.Time(time.Now()),
	}

	// Send the event
	input := &eventbridge.PutEventsInput{
		Entries: []types.PutEventsRequestEntry{entry},
	}

	result, err := client.PutEvents(ctx, input)
	if err != nil {
		logger.Error("Failed to send EventBridge event: %v", err)
		httputil.RespondJSON(w, http.StatusInternalServerError, TestEventResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to send event: %v", err),
		})
		return
	}

	// Check if the event was successfully sent
	if result.FailedEntryCount > 0 && len(result.Entries) > 0 {
		logger.Error("EventBridge event failed: %s", *result.Entries[0].ErrorMessage)
		httputil.RespondJSON(w, http.StatusInternalServerError, TestEventResponse{
			Success: false,
			Message: fmt.Sprintf("Event failed: %s", *result.Entries[0].ErrorMessage),
		})
		return
	}

	// Get event ID if available
	eventId := ""
	if len(result.Entries) > 0 && result.Entries[0].EventId != nil {
		eventId = *result.Entries[0].EventId
	}

	logger.Success("Test event sent successfully: eventId=%s", eventId)
	httputil.RespondJSON(w, http.StatusOK, TestEventResponse{
		Success: true,
		EventId: eventId,
		Message: "Test event sent successfully",
	})
}

// getEventTaskInfo returns information about event tasks and their patterns
func getEventTaskInfo(w http.ResponseWriter, r *http.Request) {
	envName, ok := httputil.RequiredQueryParam(w, r, "env")
	if !ok {
		return
	}

	logger.Info("Fetching event task info for env=%s", envName)

	// Load environment config
	filename := fmt.Sprintf("%s.yaml", envName)
	content, err := os.ReadFile(filename)
	if err != nil {
		logger.Error("Environment not found: %s", envName)
		httputil.RespondError(w, http.StatusNotFound, "environment not found")
		return
	}

	var envConfig Env
	if err := yaml.Unmarshal(content, &envConfig); err != nil {
		logger.Error("Failed to parse environment config for %s: %v", envName, err)
		httputil.RespondError(w, http.StatusInternalServerError, "failed to parse environment config")
		return
	}

	// Extract event task information
	type EventTaskInfo struct {
		Name        string   `json:"name"`
		RuleName    string   `json:"ruleName"`
		Sources     []string `json:"sources"`
		DetailTypes []string `json:"detailTypes"`
		DockerImage string   `json:"dockerImage,omitempty"`
	}

	eventTasks := make([]EventTaskInfo, 0)
	for _, task := range envConfig.EventProcessorTasks {
		eventTasks = append(eventTasks, EventTaskInfo{
			Name:        task.Name,
			RuleName:    task.RuleName,
			Sources:     task.Sources,
			DetailTypes: task.DetailTypes,
			DockerImage: task.ExternalDockerImage,
		})
	}

	logger.Success("Retrieved %d event tasks for env=%s", len(eventTasks), envName)
	httputil.RespondJSON(w, http.StatusOK, eventTasks)
}