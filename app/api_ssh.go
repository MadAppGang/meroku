package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v2"
)

// SSHSession represents an active SSH session
type SSHSession struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	mutex  sync.Mutex
}

// Message types for WebSocket communication
type SSHMessage struct {
	Type string `json:"type"` // "input", "output", "error", "connected", "disconnected"
	Data string `json:"data"`
}

// startSSHSession handles WebSocket connections for SSH sessions
func startSSHSession(w http.ResponseWriter, r *http.Request) {
	// Get parameters
	envName := r.URL.Query().Get("env")
	serviceName := r.URL.Query().Get("service")
	taskArn := r.URL.Query().Get("taskArn")
	containerName := r.URL.Query().Get("container")

	if envName == "" || serviceName == "" || taskArn == "" {
		http.Error(w, "Missing required parameters: env, service, taskArn", http.StatusBadRequest)
		return
	}

	// Load environment config to get project name and region
	filename := fmt.Sprintf("%s.yaml", envName)
	content, err := os.ReadFile(filename)
	if err != nil {
		http.Error(w, "Environment not found: "+err.Error(), http.StatusNotFound)
		return
	}

	var envConfig Env
	if err := yaml.Unmarshal(content, &envConfig); err != nil {
		http.Error(w, "Failed to parse environment config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Get cluster name
	clusterName := fmt.Sprintf("%s_cluster_%s", envConfig.Project, envConfig.Env)

	// If container name not provided, use the service name pattern
	if containerName == "" {
		if serviceName == "backend" {
			containerName = fmt.Sprintf("%s_service_%s", envConfig.Project, envConfig.Env)
		} else {
			containerName = fmt.Sprintf("%s_service_%s_%s", envConfig.Project, serviceName, envConfig.Env)
		}
	}

	// Get AWS region from config
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(selectedAWSProfile),
	)
	if err != nil {
		http.Error(w, "Failed to load AWS config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Failed to upgrade to WebSocket: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	// Send connected message
	conn.WriteJSON(SSHMessage{
		Type: "connected",
		Data: fmt.Sprintf("Connecting to %s container in task %s...", containerName, taskArn),
	})

	// Build the ECS execute-command
	cmdArgs := []string{
		"ecs", "execute-command",
		"--cluster", clusterName,
		"--task", taskArn,
		"--container", containerName,
		"--interactive",
		"--command", "/bin/bash",
		"--region", cfg.Region,
	}

	// Add profile if specified
	var cmd *exec.Cmd
	if selectedAWSProfile != "" {
		cmd = exec.Command("aws", cmdArgs...)
		cmd.Env = append(os.Environ(), 
			fmt.Sprintf("AWS_PROFILE=%s", selectedAWSProfile),
			"AWS_PAGER=", // Disable pager
		)
	} else {
		cmd = exec.Command("aws", cmdArgs...)
		cmd.Env = append(os.Environ(), 
			"AWS_PAGER=", // Disable pager
		)
	}

	// Create pipes for stdin, stdout, stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		conn.WriteJSON(SSHMessage{
			Type: "error",
			Data: "Failed to create stdin pipe: " + err.Error(),
		})
		return
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		conn.WriteJSON(SSHMessage{
			Type: "error",
			Data: "Failed to create stdout pipe: " + err.Error(),
		})
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		conn.WriteJSON(SSHMessage{
			Type: "error",
			Data: "Failed to create stderr pipe: " + err.Error(),
		})
		return
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		conn.WriteJSON(SSHMessage{
			Type: "error",
			Data: "Failed to start ECS execute command: " + err.Error(),
		})
		return
	}

	// Create session
	session := &SSHSession{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}

	// Handle cleanup on exit
	defer func() {
		session.mutex.Lock()
		defer session.mutex.Unlock()

		if session.stdin != nil {
			session.stdin.Close()
		}
		if session.cmd != nil && session.cmd.Process != nil {
			session.cmd.Process.Kill()
			session.cmd.Wait()
		}

		conn.WriteJSON(SSHMessage{
			Type: "disconnected",
			Data: "Session ended",
		})
	}()

	// Start goroutines to handle I/O
	done := make(chan bool)
	
	// Read from stdout
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stdout.Read(buf)
			if err != nil {
				if err != io.EOF {
					conn.WriteJSON(SSHMessage{
						Type: "error",
						Data: "Error reading stdout: " + err.Error(),
					})
				}
				break
			}
			if n > 0 {
				conn.WriteJSON(SSHMessage{
					Type: "output",
					Data: string(buf[:n]),
				})
			}
		}
		done <- true
	}()

	// Read from stderr
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stderr.Read(buf)
			if err != nil {
				if err != io.EOF {
					conn.WriteJSON(SSHMessage{
						Type: "error",
						Data: "Error reading stderr: " + err.Error(),
					})
				}
				break
			}
			if n > 0 {
				conn.WriteJSON(SSHMessage{
					Type: "output",
					Data: string(buf[:n]),
				})
			}
		}
	}()

	// Read from WebSocket and write to stdin
	go func() {
		for {
			var msg SSHMessage
			err := conn.ReadJSON(&msg)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					fmt.Printf("WebSocket error: %v\n", err)
				}
				break
			}

			if msg.Type == "input" {
				session.mutex.Lock()
				if session.stdin != nil {
					_, err := session.stdin.Write([]byte(msg.Data))
					if err != nil {
						conn.WriteJSON(SSHMessage{
							Type: "error",
							Data: "Failed to write to stdin: " + err.Error(),
						})
					}
				}
				session.mutex.Unlock()
			}
		}
		done <- true
	}()

	// Wait for either stdout to close or WebSocket to disconnect
	<-done

	// Wait for the command to finish
	cmd.Wait()
}

// getSSHCapability checks if a service/task supports SSH (ECS Execute Command)
func getSSHCapability(w http.ResponseWriter, r *http.Request) {
	envName := r.URL.Query().Get("env")
	serviceName := r.URL.Query().Get("service")
	taskArn := r.URL.Query().Get("taskArn")

	if envName == "" || serviceName == "" || taskArn == "" {
		http.Error(w, "Missing required parameters", http.StatusBadRequest)
		return
	}

	// Load environment config
	filename := fmt.Sprintf("%s.yaml", envName)
	content, err := os.ReadFile(filename)
	if err != nil {
		http.Error(w, "Environment not found", http.StatusNotFound)
		return
	}

	var envConfig Env
	if err := yaml.Unmarshal(content, &envConfig); err != nil {
		http.Error(w, "Failed to parse environment config", http.StatusInternalServerError)
		return
	}

	// Get cluster name
	clusterName := fmt.Sprintf("%s_cluster_%s", envConfig.Project, envConfig.Env)

	// Load AWS config
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(selectedAWSProfile),
	)
	if err != nil {
		http.Error(w, "Failed to load AWS config", http.StatusInternalServerError)
		return
	}

	ecsClient := ecs.NewFromConfig(cfg)

	// Describe the task to check if execute command is enabled
	describeTasksInput := &ecs.DescribeTasksInput{
		Cluster: &clusterName,
		Tasks:   []string{taskArn},
	}

	describeTasksOutput, err := ecsClient.DescribeTasks(ctx, describeTasksInput)
	if err != nil {
		http.Error(w, "Failed to describe task: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := struct {
		Enabled bool   `json:"enabled"`
		Reason  string `json:"reason,omitempty"`
	}{
		Enabled: false,
		Reason:  "Task not found",
	}

	if len(describeTasksOutput.Tasks) > 0 {
		task := describeTasksOutput.Tasks[0]
		if task.EnableExecuteCommand {
			response.Enabled = true
			response.Reason = ""
		} else {
			response.Reason = "Execute command is not enabled for this task"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}