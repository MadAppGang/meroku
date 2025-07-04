package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v2"
	"os"
)

// constructLogGroupName determines the correct log group name based on service type
func constructLogGroupName(envConfig Env, serviceName string) string {
	// Check if it's the backend service
	if serviceName == "backend" {
		return fmt.Sprintf("%s_backend_%s", envConfig.Project, envConfig.Env)
	}
	
	// Check if it's a scheduled task
	for _, task := range envConfig.ScheduledTasks {
		if task.Name == serviceName {
			return fmt.Sprintf("%s_task_%s_%s", envConfig.Project, serviceName, envConfig.Env)
		}
	}
	
	// Check if it's an event processor task
	for _, task := range envConfig.EventProcessorTasks {
		if task.Name == serviceName {
			return fmt.Sprintf("%s_task_%s_%s", envConfig.Project, serviceName, envConfig.Env)
		}
	}
	
	// Default to service pattern for regular services
	return fmt.Sprintf("%s_service_%s_%s", envConfig.Project, serviceName, envConfig.Env)
}

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
	Level     string `json:"level"`
	Stream    string `json:"stream"`
}

// LogsResponse represents the response for logs endpoint
type LogsResponse struct {
	ServiceName string     `json:"serviceName"`
	Logs        []LogEntry `json:"logs"`
	NextToken   string     `json:"nextToken,omitempty"`
}

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for development
		// In production, you should check the origin
		return true
	},
}

// getServiceLogs retrieves recent logs for a service
func getServiceLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	envName := r.URL.Query().Get("env")
	serviceName := r.URL.Query().Get("service")
	limit := r.URL.Query().Get("limit")
	nextToken := r.URL.Query().Get("nextToken")

	if envName == "" || serviceName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "env and service parameters are required"})
		return
	}

	// Default limit
	logLimit := int32(100)
	if limit != "" {
		fmt.Sscanf(limit, "%d", &logLimit)
		if logLimit > 1000 {
			logLimit = 1000 // Max limit
		}
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

	// Construct log group name based on service type
	logGroupName := constructLogGroupName(envConfig, serviceName)

	// Get AWS config
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to load AWS config"})
		return
	}

	// Get logs from CloudWatch
	cwClient := cloudwatchlogs.NewFromConfig(cfg)
	
	// Get log streams
	streamsInput := &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: aws.String(logGroupName),
		OrderBy:      "LastEventTime",
		Descending:   aws.Bool(true),
		Limit:        aws.Int32(5), // Get last 5 streams
	}

	streamsResult, err := cwClient.DescribeLogStreams(ctx, streamsInput)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("failed to get log streams: %v", err)})
		return
	}

	logs := []LogEntry{}
	var outputNextToken string

	if len(streamsResult.LogStreams) > 0 {
		// Build filter pattern to search across streams
		streamNames := []string{}
		for _, stream := range streamsResult.LogStreams {
			if stream.LogStreamName != nil {
				streamNames = append(streamNames, *stream.LogStreamName)
			}
		}

		// Query logs - get latest logs across all time
		filterInput := &cloudwatchlogs.FilterLogEventsInput{
			LogGroupName:   aws.String(logGroupName),
			Limit:          aws.Int32(logLimit),
			LogStreamNames: streamNames,
		}

		if nextToken != "" {
			filterInput.NextToken = aws.String(nextToken)
		}

		filterResult, err := cwClient.FilterLogEvents(ctx, filterInput)
		if err == nil {
			for _, event := range filterResult.Events {
				if event.Message != nil && event.Timestamp != nil {
					// Parse log level from message
					level := "info"
					message := *event.Message
					messageLower := strings.ToLower(message)
					
					if strings.Contains(messageLower, "error") || strings.Contains(messageLower, "exception") {
						level = "error"
					} else if strings.Contains(messageLower, "warn") {
						level = "warning"
					} else if strings.Contains(messageLower, "debug") {
						level = "debug"
					}

					logs = append(logs, LogEntry{
						Timestamp: time.Unix(*event.Timestamp/1000, 0).Format(time.RFC3339),
						Message:   strings.TrimSpace(message),
						Level:     level,
						Stream:    *event.LogStreamName,
					})
				}
			}

			if filterResult.NextToken != nil {
				outputNextToken = *filterResult.NextToken
			}
		}
	}

	// Sort logs by timestamp (newest first)
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].Timestamp > logs[j].Timestamp
	})

	response := LogsResponse{
		ServiceName: serviceName,
		Logs:        logs,
		NextToken:   outputNextToken,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// streamServiceLogs streams logs in real-time via WebSocket
func streamServiceLogs(w http.ResponseWriter, r *http.Request) {
	envName := r.URL.Query().Get("env")
	serviceName := r.URL.Query().Get("service")

	if envName == "" || serviceName == "" {
		http.Error(w, "env and service parameters are required", http.StatusBadRequest)
		return
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Failed to upgrade to WebSocket", http.StatusBadRequest)
		return
	}
	defer conn.Close()

	// Load environment config
	filename := fmt.Sprintf("%s.yaml", envName)
	content, err := os.ReadFile(filename)
	if err != nil {
		conn.WriteJSON(map[string]string{"error": "environment not found"})
		return
	}

	var envConfig Env
	if err := yaml.Unmarshal(content, &envConfig); err != nil {
		conn.WriteJSON(map[string]string{"error": "failed to parse environment config"})
		return
	}

	// Construct log group name based on service type
	logGroupName := constructLogGroupName(envConfig, serviceName)

	// Get AWS config
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		conn.WriteJSON(map[string]string{"error": "failed to load AWS config"})
		return
	}

	cwClient := cloudwatchlogs.NewFromConfig(cfg)

	// Send initial connection success message
	conn.WriteJSON(map[string]interface{}{
		"type":    "connected",
		"message": "Connected to log stream",
	})

	// Keep track of the last timestamp - start from 24 hours ago to get recent logs
	lastTimestamp := time.Now().Add(-24 * time.Hour).Unix() * 1000

	// Create a ticker for periodic log fetching
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Channel to handle client disconnection
	done := make(chan struct{})
	
	// Read messages from client (for ping/pong and close detection)
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				close(done)
				return
			}
		}
	}()

	// Main loop to fetch and stream logs
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			// Get latest log streams
			streamsInput := &cloudwatchlogs.DescribeLogStreamsInput{
				LogGroupName: aws.String(logGroupName),
				OrderBy:      "LastEventTime",
				Descending:   aws.Bool(true),
				Limit:        aws.Int32(3), // Get last 3 streams
			}

			streamsResult, err := cwClient.DescribeLogStreams(ctx, streamsInput)
			if err != nil {
				conn.WriteJSON(map[string]string{"error": fmt.Sprintf("failed to get log streams: %v", err)})
				continue
			}

			if len(streamsResult.LogStreams) > 0 {
				// Build stream names
				streamNames := []string{}
				for _, stream := range streamsResult.LogStreams {
					if stream.LogStreamName != nil {
						streamNames = append(streamNames, *stream.LogStreamName)
					}
				}

				// Query new logs since last timestamp
				filterInput := &cloudwatchlogs.FilterLogEventsInput{
					LogGroupName:   aws.String(logGroupName),
					StartTime:      aws.Int64(lastTimestamp + 1),
					LogStreamNames: streamNames,
					Limit:          aws.Int32(50),
				}

				filterResult, err := cwClient.FilterLogEvents(ctx, filterInput)
				if err == nil && len(filterResult.Events) > 0 {
					newLogs := []LogEntry{}
					
					for _, event := range filterResult.Events {
						if event.Message != nil && event.Timestamp != nil {
							// Update last timestamp
							if *event.Timestamp > lastTimestamp {
								lastTimestamp = *event.Timestamp
							}

							// Parse log level
							level := "info"
							message := *event.Message
							messageLower := strings.ToLower(message)
							
							if strings.Contains(messageLower, "error") || strings.Contains(messageLower, "exception") {
								level = "error"
							} else if strings.Contains(messageLower, "warn") {
								level = "warning"
							} else if strings.Contains(messageLower, "debug") {
								level = "debug"
							}

							newLogs = append(newLogs, LogEntry{
								Timestamp: time.Unix(*event.Timestamp/1000, 0).Format(time.RFC3339),
								Message:   strings.TrimSpace(message),
								Level:     level,
								Stream:    *event.LogStreamName,
							})
						}
					}

					// Send new logs to client
					if len(newLogs) > 0 {
						err = conn.WriteJSON(map[string]interface{}{
							"type": "logs",
							"data": newLogs,
						})
						if err != nil {
							return
						}
					}
				}
			}
		}
	}
}