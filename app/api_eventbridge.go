package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
)

// TestEventRequest represents the request to send a test event.
//
// Env names the environment whose YAML, AWS profile and region are used.
// EventBusName is "default" (also when empty) or the environment's project bus,
// <project>-<env>; any other bus is refused.
type TestEventRequest struct {
	Env          string                 `json:"env"`
	EventBusName string                 `json:"eventBusName,omitempty"`
	Source       string                 `json:"source"`
	DetailType   string                 `json:"detailType"`
	Detail       map[string]interface{} `json:"detail"`
}

// TestEventResponse represents the response after sending an event
type TestEventResponse struct {
	Success bool   `json:"success"`
	EventId string `json:"eventId,omitempty"`
	Message string `json:"message"`
}

// eventBridgeAPI is the part of the EventBridge client Test Event uses.
type eventBridgeAPI interface {
	PutEvents(ctx context.Context, in *eventbridge.PutEventsInput, optFns ...func(*eventbridge.Options)) (*eventbridge.PutEventsOutput, error)
	DescribeEventBus(ctx context.Context, in *eventbridge.DescribeEventBusInput, optFns ...func(*eventbridge.Options)) (*eventbridge.DescribeEventBusOutput, error)
}

// newEventBridgeClient builds a client for the environment's own AWS profile and
// region, never the shell's ambient credentials: the test event must land in the
// account the environment deploys to. A variable so tests can replace it.
var newEventBridgeClient = func(ctx context.Context, e Env) (eventBridgeAPI, error) {
	var opts []func(*config.LoadOptions) error
	if e.AWSProfile != "" {
		opts = append(opts, config.WithSharedConfigProfile(e.AWSProfile))
	}
	if e.Region != "" {
		opts = append(opts, config.WithRegion(e.Region))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return eventbridge.NewFromConfig(cfg), nil
}

// loadEnvForTestEvent loads <project dir>/<env>.yaml. A variable so tests can
// replace it.
var loadEnvForTestEvent = loadEnv

// projectEventBusName is the name modules/events gives the project bus.
func projectEventBusName(e Env) string {
	return e.Project + "-" + e.Env
}

// eventBusEnabledIn reports event_bus.enabled for a loaded environment.
func eventBusEnabledIn(e Env) bool {
	return e.EventBus != nil && e.EventBus.Enabled != nil && *e.EventBus.Enabled
}

// errTestEventBus is a refusal of the requested bus; the handler answers 400.
var errTestEventBus = errors.New("test event bus refused")

// resolveTestEventBus checks the requested bus against the environment: the
// default bus always, the project bus only when event_bus.enabled is true.
func resolveTestEventBus(e Env, requested string) (string, error) {
	switch requested {
	case "", eventBusDefault:
		return eventBusDefault, nil
	case projectEventBusName(e):
		if !eventBusEnabledIn(e) {
			return "", fmt.Errorf("%w: %s is the project bus, but event_bus.enabled is not true in %s.yaml", errTestEventBus, requested, e.Env)
		}
		return requested, nil
	}
	return "", fmt.Errorf("%w: %q is not a bus of this environment; use default or %s", errTestEventBus, requested, projectEventBusName(e))
}

func writeTestEventResponse(w http.ResponseWriter, status int, resp TestEventResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

// sendTestEvent sends a test event to EventBridge
//
// POST /api/eventbridge/send-test-event
func sendTestEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeTestEventResponse(w, http.StatusBadRequest, TestEventResponse{Message: "Failed to read request body"})
		return
	}
	defer r.Body.Close()

	var req TestEventRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeTestEventResponse(w, http.StatusBadRequest, TestEventResponse{Message: fmt.Sprintf("Invalid request body: %v", err)})
		return
	}

	if req.Source == "" || req.DetailType == "" {
		writeTestEventResponse(w, http.StatusBadRequest, TestEventResponse{Message: "source and detailType are required"})
		return
	}
	envName := req.Env
	if envName == "" {
		envName = selectedEnvironment
	}
	if envName == "" {
		writeTestEventResponse(w, http.StatusBadRequest, TestEventResponse{Message: "env is required"})
		return
	}

	e, err := loadEnvForTestEvent(envName)
	if err != nil {
		writeTestEventResponse(w, http.StatusNotFound, TestEventResponse{Message: fmt.Sprintf("Environment %s not found: %v", envName, err)})
		return
	}

	busName, err := resolveTestEventBus(e, req.EventBusName)
	if err != nil {
		writeTestEventResponse(w, http.StatusBadRequest, TestEventResponse{Message: err.Error()})
		return
	}

	if req.Detail == nil {
		req.Detail = map[string]interface{}{
			"test":      true,
			"timestamp": time.Now().Format(time.RFC3339),
			"message":   "Test event from infrastructure dashboard",
		}
	}
	detailJSON, err := json.Marshal(req.Detail)
	if err != nil {
		writeTestEventResponse(w, http.StatusInternalServerError, TestEventResponse{Message: fmt.Sprintf("Failed to marshal detail: %v", err)})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	client, err := newEventBridgeClient(ctx, e)
	if err != nil {
		writeTestEventResponse(w, http.StatusInternalServerError, TestEventResponse{Message: fmt.Sprintf("Failed to load AWS config for profile %q: %v", e.AWSProfile, err)})
		return
	}

	// PutEvents to a bus that does not exist can report success and drop the
	// event, so the project bus is looked up first.
	if busName != eventBusDefault {
		if _, err := client.DescribeEventBus(ctx, &eventbridge.DescribeEventBusInput{Name: aws.String(busName)}); err != nil {
			var notFound *types.ResourceNotFoundException
			if errors.As(err, &notFound) {
				writeTestEventResponse(w, http.StatusConflict, TestEventResponse{
					Message: fmt.Sprintf("The bus %s does not exist yet; apply the %s environment first", busName, e.Env),
				})
				return
			}
			writeTestEventResponse(w, http.StatusInternalServerError, TestEventResponse{Message: fmt.Sprintf("Failed to look up the bus %s: %v", busName, err)})
			return
		}
	}

	result, err := client.PutEvents(ctx, &eventbridge.PutEventsInput{
		Entries: []types.PutEventsRequestEntry{{
			EventBusName: aws.String(busName),
			Source:       aws.String(req.Source),
			DetailType:   aws.String(req.DetailType),
			Detail:       aws.String(string(detailJSON)),
			Time:         aws.Time(time.Now()),
		}},
	})
	if err != nil {
		writeTestEventResponse(w, http.StatusInternalServerError, TestEventResponse{Message: fmt.Sprintf("Failed to send event: %v", err)})
		return
	}
	if result.FailedEntryCount > 0 && len(result.Entries) > 0 {
		writeTestEventResponse(w, http.StatusInternalServerError, TestEventResponse{
			Message: fmt.Sprintf("Event failed: %s", aws.ToString(result.Entries[0].ErrorMessage)),
		})
		return
	}

	eventID := ""
	if len(result.Entries) > 0 {
		eventID = aws.ToString(result.Entries[0].EventId)
	}
	writeTestEventResponse(w, http.StatusOK, TestEventResponse{
		Success: true,
		EventId: eventID,
		Message: fmt.Sprintf("Test event sent to the %s bus", busName),
	})
}

// EventTaskRuleInfo is one rule of an event task, with the bus it is on.
type EventTaskRuleInfo struct {
	Name         string   `json:"name"`
	Sources      []string `json:"sources"`
	DetailTypes  []string `json:"detailTypes"`
	EventBusName string   `json:"eventBusName"`
}

// EventTaskInfo is an event task and its rules. RuleName, Sources and
// DetailTypes repeat the first rule, for callers that read one rule.
type EventTaskInfo struct {
	Name        string              `json:"name"`
	RuleName    string              `json:"ruleName"`
	Sources     []string            `json:"sources"`
	DetailTypes []string            `json:"detailTypes"`
	Rules       []EventTaskRuleInfo `json:"rules"`
	DockerImage string              `json:"dockerImage,omitempty"`
}

// eventTaskRulesInfo lists a task's rules, the legacy single rule included, with
// each rule's bus resolved as app/events.go resolves it.
func eventTaskRulesInfo(e Env, task EventProcessorTask) []EventTaskRuleInfo {
	busFor := func(bus string) string {
		switch bus {
		case eventBusDefault:
			return eventBusDefault
		case eventBusProject:
			return projectEventBusName(e)
		}
		if eventBusEnabledIn(e) {
			return projectEventBusName(e)
		}
		return eventBusDefault
	}
	rules := make([]EventTaskRuleInfo, 0, len(task.Rules)+1)
	if len(task.Rules) == 0 && task.RuleName != "" {
		rules = append(rules, EventTaskRuleInfo{
			Name: task.RuleName, Sources: task.Sources, DetailTypes: task.DetailTypes, EventBusName: busFor(""),
		})
	}
	for _, r := range task.Rules {
		rules = append(rules, EventTaskRuleInfo{
			Name: r.Name, Sources: r.Sources, DetailTypes: r.DetailTypes, EventBusName: busFor(r.Bus),
		})
	}
	return rules
}

// getEventTaskInfo returns information about event tasks and their patterns
//
// GET /api/eventbridge/event-tasks?env=dev
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

	e, err := loadEnvForTestEvent(envName)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "environment not found"})
		return
	}

	eventTasks := make([]EventTaskInfo, 0, len(e.EventProcessorTasks))
	for _, task := range e.EventProcessorTasks {
		info := EventTaskInfo{
			Name:        task.Name,
			Rules:       eventTaskRulesInfo(e, task),
			DockerImage: task.ExternalDockerImage,
		}
		if len(info.Rules) > 0 {
			info.RuleName = info.Rules[0].Name
			info.Sources = info.Rules[0].Sources
			info.DetailTypes = info.Rules[0].DetailTypes
		}
		eventTasks = append(eventTasks, info)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(eventTasks)
}
