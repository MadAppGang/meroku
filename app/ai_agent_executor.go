package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// AgentExecutor handles the execution of different tool types
type AgentExecutor struct {
	context *AgentContext
}

// NewAgentExecutor creates a new executor
func NewAgentExecutor(ctx *AgentContext) *AgentExecutor {
	return &AgentExecutor{
		context: ctx,
	}
}

// ExecuteAWSCLI runs AWS CLI commands
func (e *AgentExecutor) ExecuteAWSCLI(ctx context.Context, command string) (string, error) {
	// Parse command to extract the actual AWS CLI part
	// Command might be like: "aws ecs describe-services --cluster name --services svc"

	// SECURITY: Validate command before execution
	if err := validateCommand(command); err != nil {
		return "", fmt.Errorf("command validation failed: %w", err)
	}

	// Set AWS environment variables
	env := os.Environ()
	if e.context.AWSProfile != "" {
		env = append(env, fmt.Sprintf("AWS_PROFILE=%s", e.context.AWSProfile))
	}
	if e.context.AWSRegion != "" {
		env = append(env, fmt.Sprintf("AWS_REGION=%s", e.context.AWSRegion))
		env = append(env, fmt.Sprintf("AWS_DEFAULT_REGION=%s", e.context.AWSRegion))
	}

	// Create command with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "sh", "-c", command)
	cmd.Dir = e.context.WorkingDir
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		// Include stderr in output
		if output != "" {
			output += "\n--- STDERR ---\n"
		}
		output += stderr.String()
	}

	if err != nil {
		return output, fmt.Errorf("AWS CLI command failed: %w", err)
	}

	return output, nil
}

// ExecuteShell runs arbitrary shell commands
func (e *AgentExecutor) ExecuteShell(ctx context.Context, command string) (string, error) {
	// SECURITY: Validate command before execution
	if err := validateCommand(command); err != nil {
		return "", fmt.Errorf("command validation failed: %w", err)
	}

	// Set environment variables
	env := os.Environ()
	if e.context.AWSProfile != "" {
		env = append(env, fmt.Sprintf("AWS_PROFILE=%s", e.context.AWSProfile))
	}
	if e.context.AWSRegion != "" {
		env = append(env, fmt.Sprintf("AWS_REGION=%s", e.context.AWSRegion))
	}

	// Create command with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "sh", "-c", command)
	cmd.Dir = e.context.WorkingDir
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n--- STDERR ---\n"
		}
		output += stderr.String()
	}

	if err != nil {
		return output, fmt.Errorf("shell command failed: %w", err)
	}

	return output, nil
}

// ExecuteFileEdit performs file editing operations
// Command format: "FILE:/path/to/file|OLD:old text|NEW:new text"
func (e *AgentExecutor) ExecuteFileEdit(ctx context.Context, command string) (string, error) {
	// Parse the command
	parts := strings.Split(command, "|")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid file_edit command format, expected: FILE:path|OLD:text|NEW:text")
	}

	var filePath, oldText, newText string

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "FILE:") {
			filePath = strings.TrimPrefix(part, "FILE:")
		} else if strings.HasPrefix(part, "OLD:") {
			oldText = strings.TrimPrefix(part, "OLD:")
		} else if strings.HasPrefix(part, "NEW:") {
			newText = strings.TrimPrefix(part, "NEW:")
		}
	}

	if filePath == "" {
		return "", fmt.Errorf("file path not specified")
	}

	// Handle relative paths
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(e.context.WorkingDir, filePath)
	}

	// SECURITY: Validate file path to prevent path traversal
	if err := validateFilePath(e.context.WorkingDir, filePath); err != nil {
		return "", fmt.Errorf("file path validation failed: %w", err)
	}

	// SECURITY: Validate file size to prevent memory exhaustion
	const maxFileSize = 10 * 1024 * 1024 // 10MB
	if err := validateFileSize(filePath, maxFileSize); err != nil {
		return "", fmt.Errorf("file size validation failed: %w", err)
	}

	// SECURITY: Create backup before modification
	backupPath, err := createFileBackup(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}
	if backupPath != "" {
		defer func() {
			// Log backup creation (could be captured in output)
			fmt.Printf("Created backup: %s\n", backupPath)
		}()
	}

	// Read the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Check if old text exists (if specified)
	contentStr := string(content)
	if oldText != "" {
		if !strings.Contains(contentStr, oldText) {
			return "", fmt.Errorf("old text not found in file: %s", oldText)
		}
		// Replace old with new
		contentStr = strings.ReplaceAll(contentStr, oldText, newText)
	} else {
		// Just append new text
		contentStr += newText
	}

	// Write back
	err = os.WriteFile(filePath, []byte(contentStr), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully updated %s", filePath), nil
}

// ExecuteTerraformApply runs terraform apply
func (e *AgentExecutor) ExecuteTerraformApply(ctx context.Context, command string) (string, error) {
	// Extract environment from command or use default
	env := e.context.Environment
	if strings.Contains(command, "-chdir=") {
		// Parse chdir flag if present
		parts := strings.Fields(command)
		for _, part := range parts {
			if strings.HasPrefix(part, "-chdir=") {
				// Extract directory
				// This is just for reference, we'll use environment-based path
			}
		}
	}

	// Build terraform directory path
	tfDir := filepath.Join(e.context.WorkingDir, "env", env)
	if _, err := os.Stat(tfDir); os.IsNotExist(err) {
		return "", fmt.Errorf("terraform directory does not exist: %s", tfDir)
	}

	// Create command with timeout (terraform apply can take a while)
	cmdCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	// Add auto-approve flag if not present
	if !strings.Contains(command, "-auto-approve") {
		command += " -auto-approve"
	}

	// SECURITY: Validate terraform command before execution
	if err := validateCommand(command); err != nil {
		return "", fmt.Errorf("command validation failed: %w", err)
	}

	cmd := exec.CommandContext(cmdCtx, "sh", "-c", command)
	cmd.Dir = tfDir

	// Set AWS environment
	cmdEnv := os.Environ()
	if e.context.AWSProfile != "" {
		cmdEnv = append(cmdEnv, fmt.Sprintf("AWS_PROFILE=%s", e.context.AWSProfile))
	}
	if e.context.AWSRegion != "" {
		cmdEnv = append(cmdEnv, fmt.Sprintf("AWS_REGION=%s", e.context.AWSRegion))
	}
	cmd.Env = cmdEnv

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n--- STDERR ---\n"
		}
		output += stderr.String()
	}

	if err != nil {
		return output, fmt.Errorf("terraform apply failed: %w", err)
	}

	return output, nil
}

// ExecuteTerraformPlan runs terraform plan
func (e *AgentExecutor) ExecuteTerraformPlan(ctx context.Context, command string) (string, error) {
	// Build terraform directory path
	env := e.context.Environment
	tfDir := filepath.Join(e.context.WorkingDir, "env", env)
	if _, err := os.Stat(tfDir); os.IsNotExist(err) {
		return "", fmt.Errorf("terraform directory does not exist: %s", tfDir)
	}

	// Create command with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "sh", "-c", command)
	cmd.Dir = tfDir

	// Set AWS environment
	cmdEnv := os.Environ()
	if e.context.AWSProfile != "" {
		cmdEnv = append(cmdEnv, fmt.Sprintf("AWS_PROFILE=%s", e.context.AWSProfile))
	}
	if e.context.AWSRegion != "" {
		cmdEnv = append(cmdEnv, fmt.Sprintf("AWS_REGION=%s", e.context.AWSRegion))
	}
	cmd.Env = cmdEnv

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n--- STDERR ---\n"
		}
		output += stderr.String()
	}

	if err != nil {
		return output, fmt.Errorf("terraform plan failed: %w", err)
	}

	return output, nil
}

// ExecuteWebSearch performs a web search using DuckDuckGo
func (e *AgentExecutor) ExecuteWebSearch(ctx context.Context, query string) (string, error) {
	// Create timeout context
	searchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Execute search
	output, err := ExecuteWebSearch(searchCtx, query)
	if err != nil {
		return "", fmt.Errorf("web search failed: %w", err)
	}

	return output, nil
}
