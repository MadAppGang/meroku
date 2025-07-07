package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"gopkg.in/yaml.v2"
	"os"
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
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TestEventResponse{
			Success: false,
			Message: "Failed to read request body",
		})
		return
	}
	defer r.Body.Close()

	var req TestEventRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TestEventResponse{
			Success: false,
			Message: fmt.Sprintf("Invalid request body: %v", err),
		})
		return
	}

	// Validate required fields
	if req.Source == "" || req.DetailType == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TestEventResponse{
			Success: false,
			Message: "source and detailType are required",
		})
		return
	}

	// If detail is not provided, create a default test event detail
	if req.Detail == nil {
		req.Detail = map[string]interface{}{
			"test": true,
			"timestamp": time.Now().Format(time.RFC3339),
			"message": "Test event from infrastructure dashboard",
		}
	}

	// Load AWS configuration
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TestEventResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to load AWS config: %v", err),
		})
		return
	}

	// Create EventBridge client
	client := eventbridge.NewFromConfig(cfg)

	// Marshal detail to JSON string
	detailJSON, err := json.Marshal(req.Detail)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TestEventResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to marshal detail: %v", err),
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
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TestEventResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to send event: %v", err),
		})
		return
	}

	// Check if the event was successfully sent
	if result.FailedEntryCount > 0 && len(result.Entries) > 0 {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TestEventResponse{
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(TestEventResponse{
		Success: true,
		EventId: eventId,
		Message: "Test event sent successfully",
	})
}

// getEventTaskInfo returns information about event tasks and their patterns
func getEventTaskInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	envName := r.URL.Query().Get("env")
	if envName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "env parameter is required"})
		return
	}

	// Load environment config
	filename := fmt.Sprintf("%s.yaml", envName)
	content, err := os.ReadFile(filename)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "environment not found"})
		return
	}

	var envConfig Env
	if err := yaml.Unmarshal(content, &envConfig); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to parse environment config"})
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(eventTasks)
}