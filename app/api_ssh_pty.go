package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v2"
)

// Define WebSocket upgrader for SSH connections
var sshUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for development
		// In production, you should check the origin
		return true
	},
}

// startSSHSessionPTY handles WebSocket connections for SSH sessions using PTY
func startSSHSessionPTY(w http.ResponseWriter, r *http.Request) {
	// Get parameters
	envName := r.URL.Query().Get("env")
	serviceName := r.URL.Query().Get("service")
	taskArn := r.URL.Query().Get("taskArn")
	containerName := r.URL.Query().Get("container")
	
	fmt.Printf("SSH WebSocket request: env=%s, service=%s, taskArn=%s, container=%s\n", envName, serviceName, taskArn, containerName)

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
	fmt.Printf("Attempting WebSocket upgrade for SSH connection...\n")
	conn, err := sshUpgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("WebSocket upgrade failed: %v\n", err)
		// Don't call http.Error after upgrade attempt - headers already sent
		return
	}
	fmt.Printf("WebSocket upgrade successful\n")
	defer conn.Close()

	// Set WebSocket timeouts
	conn.SetReadDeadline(time.Time{}) // No read deadline
	conn.SetWriteDeadline(time.Time{}) // No write deadline
	
	// Configure ping/pong to keep connection alive
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	
	// Handle close gracefully
	conn.SetCloseHandler(func(code int, text string) error {
		fmt.Printf("WebSocket close requested: code=%d, text=%s\n", code, text)
		return nil
	})

	// Small delay to ensure client is ready
	time.Sleep(50 * time.Millisecond)

	// Send connected message
	if err := conn.WriteJSON(SSHMessage{
		Type: "connected",
		Data: fmt.Sprintf("Connecting to %s container in task %s...", containerName, taskArn),
	}); err != nil {
		fmt.Printf("Failed to send connected message: %v\n", err)
		return
	}

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

	fmt.Printf("Executing command: aws %s\n", strings.Join(cmdArgs, " "))
	fmt.Printf("Using profile: %s\n", selectedAWSProfile)
	fmt.Printf("Cluster: %s, Container: %s\n", clusterName, containerName)

	// Create command
	var cmd *exec.Cmd
	if selectedAWSProfile != "" {
		cmd = exec.Command("aws", cmdArgs...)
		cmd.Env = append(os.Environ(), fmt.Sprintf("AWS_PROFILE=%s", selectedAWSProfile))
	} else {
		cmd = exec.Command("aws", cmdArgs...)
	}

	// Start command with PTY
	fmt.Printf("Starting PTY session...\n")
	
	// Set up to capture any immediate errors
	cmd.Stderr = cmd.Stdout // Combine stderr with stdout for PTY
	
	ptmx, err := pty.Start(cmd)
	if err != nil {
		fmt.Printf("Failed to start PTY: %v\n", err)
		conn.WriteJSON(SSHMessage{
			Type: "error",
			Data: "Failed to start PTY session: " + err.Error(),
		})
		return
	}
	fmt.Printf("PTY session started successfully\n")
	
	// Give the process a moment to start
	time.Sleep(100 * time.Millisecond)
	
	// Check if process is still running
	if cmd.ProcessState != nil {
		fmt.Printf("Process exited immediately with: %v\n", cmd.ProcessState)
		conn.WriteJSON(SSHMessage{
			Type: "error",
			Data: "SSH session failed to start. The command exited immediately.",
		})
		return
	}
	
	// Handle cleanup
	defer func() {
		fmt.Printf("Cleaning up PTY session...\n")
		if cmd.Process != nil {
			fmt.Printf("Process state: %v\n", cmd.ProcessState)
		}
		ptmx.Close()
		cmd.Wait()
		
		if err := conn.WriteJSON(SSHMessage{
			Type: "disconnected",
			Data: "Session ended",
		}); err != nil {
			fmt.Printf("Failed to send disconnected message: %v\n", err)
		}
	}()
	
	// Set initial terminal size
	if err := pty.Setsize(ptmx, &pty.Winsize{
		Rows: 24,
		Cols: 80,
	}); err != nil {
		fmt.Printf("Failed to set PTY size: %v\n", err)
	}

	// Mutex for PTY writes
	var writeMutex sync.Mutex

	// Create channels for goroutine coordination
	done := make(chan bool, 2)

	// Read from PTY and send to WebSocket
	go func() {
		buf := make([]byte, 4096) // Larger buffer for better performance
		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				if err != io.EOF {
					fmt.Printf("Error reading from PTY: %v\n", err)
					// Only send error if it's not a normal close
					if !strings.Contains(err.Error(), "file already closed") {
						conn.WriteJSON(SSHMessage{
							Type: "error",
							Data: "Error reading PTY: " + err.Error(),
						})
					}
				}
				break
			}
			if n > 0 {
				output := string(buf[:n])
				fmt.Printf("PTY output (%d bytes): %q\n", n, output)
				if err := conn.WriteJSON(SSHMessage{
					Type: "output",
					Data: output,
				}); err != nil {
					fmt.Printf("Error sending output to WebSocket: %v\n", err)
					break
				}
			}
		}
		fmt.Printf("PTY read goroutine exiting\n")
		done <- true
	}()

	// Read from WebSocket and write to PTY
	go func() {
		for {
			var msg SSHMessage
			err := conn.ReadJSON(&msg)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					fmt.Printf("WebSocket read error: %v\n", err)
				}
				break
			}

			switch msg.Type {
			case "input":
				writeMutex.Lock()
				_, err := ptmx.Write([]byte(msg.Data))
				writeMutex.Unlock()
				if err != nil {
					fmt.Printf("Error writing to PTY: %v\n", err)
					conn.WriteJSON(SSHMessage{
						Type: "error",
						Data: "Failed to write to PTY: " + err.Error(),
					})
				}
			case "resize":
				// Handle terminal resize if needed
				// Expected format: {"type": "resize", "data": {"rows": 30, "cols": 120}}
				// You can parse the data and call pty.Setsize()
			}
		}
		done <- true
	}()

	// Wait for either PTY or WebSocket to close
	<-done
	fmt.Printf("Session ended, cleaning up...\n")
}