package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// validateYAMLPath validates that a file path is safe for YAML file operations.
// It prevents path traversal and symlink bypass attacks by:
// 1. Rejecting paths with parent directory references (..)
// 2. Resolving symlinks to get the real path
// 3. Ensuring paths are within the project/ directory
// 4. Only allowing .yaml/.yml files
func validateYAMLPath(filePath string) error {
	// Reject paths with parent directory references (defense-in-depth)
	if strings.Contains(filePath, "..") {
		return fmt.Errorf("path traversal attempt detected: %s", filePath)
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// CRITICAL: Evaluate symlinks to get real path
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// If symlink target doesn't exist yet, validate the parent directory
		if os.IsNotExist(err) {
			// For new files, validate the directory containing the file
			dir := filepath.Dir(absPath)
			realDir, dirErr := filepath.EvalSymlinks(dir)
			if dirErr != nil && !os.IsNotExist(dirErr) {
				return fmt.Errorf("failed to resolve directory symlinks: %w", dirErr)
			}
			if dirErr == nil {
				// Use the real directory path + filename
				realPath = filepath.Join(realDir, filepath.Base(absPath))
			} else {
				// Directory also doesn't exist, use original path
				// This will fail later validation but that's correct behavior
				realPath = absPath
			}
		} else {
			return fmt.Errorf("failed to resolve symlinks: %w", err)
		}
	}

	// Get project directory
	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	projectDir, err = filepath.Abs(projectDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute project directory: %w", err)
	}

	// CRITICAL: Resolve symlinks in project directory too
	// Project directory should always exist and be resolvable
	realProjectDir, err := filepath.EvalSymlinks(projectDir)
	if err != nil {
		return fmt.Errorf("failed to resolve project directory symlinks: %w", err)
	}
	projectDir = realProjectDir

	// Only allow writes to project/ directory
	allowedDir := filepath.Join(projectDir, "project")

	// Use filepath.Clean to normalize paths before comparison
	realPath = filepath.Clean(realPath)
	allowedDir = filepath.Clean(allowedDir)

	// Check if path is inside allowed directory
	if !strings.HasPrefix(realPath, allowedDir+string(filepath.Separator)) &&
		realPath != allowedDir {
		return fmt.Errorf("path outside allowed directory (project/): %s", realPath)
	}

	// Ensure .yaml or .yml extension
	if !strings.HasSuffix(realPath, ".yaml") && !strings.HasSuffix(realPath, ".yml") {
		return fmt.Errorf("only .yaml/.yml files allowed, got: %s", realPath)
	}

	return nil
}

// Tool: read_aws_config
func (a *SSOAgent) toolReadAWSConfig(ctx context.Context, command string, agentCtx *SSOAgentContext) (*SSOAgentAction, error) {
	configPath := strings.TrimSpace(command)
	if configPath == "" {
		configPath = getAWSConfigPath()
	}

	content, err := ReadAWSConfig(configPath)
	if err != nil {
		return &SSOAgentAction{
			Type:   "read_aws_config",
			Result: fmt.Sprintf("Failed to read config: %v", err),
			Error:  err,
		}, err
	}

	// Store in context for future reference
	agentCtx.AWSConfigContent = content

	// Parse profiles and sessions
	profiles, _ := ParseAWSConfigProfiles(content)
	sessions, _ := ParseSSOSessions(content)

	result := fmt.Sprintf("Read AWS config from %s\n", configPath)
	result += fmt.Sprintf("Found %d profiles: %s\n", len(profiles), strings.Join(profiles, ", "))
	result += fmt.Sprintf("Found %d sso-sessions: %s\n", len(sessions), strings.Join(sessions, ", "))
	result += fmt.Sprintf("\nConfig content:\n%s", content)

	return &SSOAgentAction{
		Type:      "read_aws_config",
		Result:    result,
		Timestamp: time.Now(),
	}, nil
}

// Tool: write_aws_config
func (a *SSOAgent) toolWriteAWSConfig(ctx context.Context, command string, agentCtx *SSOAgentContext) (*SSOAgentAction, error) {
	// command contains the full config content to write
	configContent := command

	if configContent == "" {
		return &SSOAgentAction{
			Type:   "write_aws_config",
			Result: "Error: empty config content",
			Error:  fmt.Errorf("empty config content"),
		}, fmt.Errorf("empty config content")
	}

	// Create backup
	configPath := getAWSConfigPath()
	var backupPath string

	if _, err := os.Stat(configPath); err == nil {
		// File exists, create backup in backup/ directory
		var backupErr error
		backupPath, backupErr = CreateAWSConfigBackup(configPath)
		if backupErr != nil {
			return &SSOAgentAction{
				Type:   "write_aws_config",
				Result: fmt.Sprintf("Failed to create backup: %v", backupErr),
				Error:  backupErr,
			}, backupErr
		}
	}

	// Write new config
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	if err != nil {
		return &SSOAgentAction{
			Type:   "write_aws_config",
			Result: fmt.Sprintf("Failed to write config: %v", err),
			Error:  err,
		}, err
	}

	result := fmt.Sprintf("✅ Wrote AWS config to %s\n", configPath)
	if backupPath != "" {
		result += fmt.Sprintf("   Backup saved: %s", backupPath)
	}

	return &SSOAgentAction{
		Type:      "write_aws_config",
		Result:    result,
		Timestamp: time.Now(),
	}, nil
}

// Tool: read_yaml
func (a *SSOAgent) toolReadYAML(ctx context.Context, command string, agentCtx *SSOAgentContext) (*SSOAgentAction, error) {
	yamlPath := strings.TrimSpace(command)

	content, err := os.ReadFile(yamlPath)
	if err != nil {
		return &SSOAgentAction{
			Type:   "read_yaml",
			Result: fmt.Sprintf("Failed to read YAML: %v", err),
			Error:  err,
		}, err
	}

	// Store in context cache
	if agentCtx.YAMLContent == nil {
		agentCtx.YAMLContent = make(map[string]string)
	}
	agentCtx.YAMLContent[yamlPath] = string(content)

	result := fmt.Sprintf("Read YAML from %s\n\nContent:\n%s", yamlPath, string(content))

	return &SSOAgentAction{
		Type:      "read_yaml",
		Result:    result,
		Timestamp: time.Now(),
	}, nil
}

// Tool: write_yaml (uses file_edit pattern)
func (a *SSOAgent) toolWriteYAML(ctx context.Context, command string, agentCtx *SSOAgentContext) (*SSOAgentAction, error) {
	// Parse: FILE:path|OLD:old_text|NEW:new_text
	parts := strings.Split(command, "|")
	if len(parts) != 3 {
		return &SSOAgentAction{
			Type:   "write_yaml",
			Result: "Error: invalid format, expected FILE:path|OLD:text|NEW:text",
			Error:  fmt.Errorf("invalid format"),
		}, fmt.Errorf("invalid format")
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

	// CRITICAL: Validate path before any file operations
	if err := validateYAMLPath(filePath); err != nil {
		return &SSOAgentAction{
			Type:   "write_yaml",
			Result: fmt.Sprintf("Invalid path: %v", err),
			Error:  err,
		}, err
	}

	// Create backup in backup/ directory
	backupPath, err := CreateProjectBackup(filePath)
	if err != nil {
		return &SSOAgentAction{
			Type:   "write_yaml",
			Result: fmt.Sprintf("Failed to create backup: %v", err),
			Error:  err,
		}, err
	}

	// Read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return &SSOAgentAction{
			Type:   "write_yaml",
			Result: fmt.Sprintf("Failed to read YAML: %v", err),
			Error:  err,
		}, err
	}

	// Replace
	newContent := strings.ReplaceAll(string(content), oldText, newText)

	// Write back
	// SECURITY: Use 0600 for YAML files containing infrastructure configuration
	err = os.WriteFile(filePath, []byte(newContent), 0600)
	if err != nil {
		return &SSOAgentAction{
			Type:   "write_yaml",
			Result: fmt.Sprintf("Failed to write YAML: %v", err),
			Error:  err,
		}, err
	}

	// Update cache
	if agentCtx.YAMLContent == nil {
		agentCtx.YAMLContent = make(map[string]string)
	}
	agentCtx.YAMLContent[filePath] = newContent

	result := fmt.Sprintf("✅ Updated YAML: %s\n   Backup: %s", filePath, backupPath)

	return &SSOAgentAction{
		Type:      "write_yaml",
		Result:    result,
		Timestamp: time.Now(),
	}, nil
}

// Tool: ask_choice
func (a *SSOAgent) toolAskChoice(ctx context.Context, command string, agentCtx *SSOAgentContext) (*SSOAgentAction, error) {
	// Parse: QUESTION:text|OPTIONS:opt1,opt2,opt3
	parts := strings.Split(command, "|")
	if len(parts) != 2 {
		return &SSOAgentAction{
			Type:   "ask_choice",
			Result: "Error: invalid format, expected QUESTION:text|OPTIONS:opt1,opt2,opt3",
			Error:  fmt.Errorf("invalid format"),
		}, fmt.Errorf("invalid format")
	}

	var question string
	var options []string

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "QUESTION:") {
			question = strings.TrimPrefix(part, "QUESTION:")
		} else if strings.HasPrefix(part, "OPTIONS:") {
			optStr := strings.TrimPrefix(part, "OPTIONS:")
			options = strings.Split(optStr, ",")
			for i := range options {
				options[i] = strings.TrimSpace(options[i])
			}
		}
	}

	selected, err := AskChoice(question, options)
	if err != nil {
		return &SSOAgentAction{
			Type:      "ask_choice",
			Question:  question,
			Result:    fmt.Sprintf("User cancelled selection: %v", err),
			Error:     err,
			Timestamp: time.Now(),
		}, err
	}

	return &SSOAgentAction{
		Type:      "ask_choice",
		Question:  question,
		Answer:    selected,
		Result:    fmt.Sprintf("User selected: %s", selected),
		Timestamp: time.Now(),
	}, nil
}

// Tool: ask_confirm
func (a *SSOAgent) toolAskConfirm(ctx context.Context, command string, agentCtx *SSOAgentContext) (*SSOAgentAction, error) {
	question := strings.TrimSpace(command)

	confirmed, err := AskConfirm(question)
	if err != nil {
		return &SSOAgentAction{
			Type:      "ask_confirm",
			Question:  question,
			Result:    fmt.Sprintf("User cancelled confirmation: %v", err),
			Error:     err,
			Timestamp: time.Now(),
		}, err
	}

	return &SSOAgentAction{
		Type:      "ask_confirm",
		Question:  question,
		Answer:    fmt.Sprintf("%v", confirmed),
		Result:    fmt.Sprintf("User confirmed: %v", confirmed),
		Timestamp: time.Now(),
	}, nil
}

// Tool: ask_input
func (a *SSOAgent) toolAskInput(ctx context.Context, command string, agentCtx *SSOAgentContext) (*SSOAgentAction, error) {
	// Parse: QUESTION:text|VALIDATOR:type|PLACEHOLDER:text
	parts := strings.Split(command, "|")

	var question, validator, placeholder string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "QUESTION:") {
			question = strings.TrimPrefix(part, "QUESTION:")
		} else if strings.HasPrefix(part, "VALIDATOR:") {
			validator = strings.TrimPrefix(part, "VALIDATOR:")
		} else if strings.HasPrefix(part, "PLACEHOLDER:") {
			placeholder = strings.TrimPrefix(part, "PLACEHOLDER:")
		}
	}

	if question == "" {
		return &SSOAgentAction{
			Type:   "ask_input",
			Result: "Error: question not specified",
			Error:  fmt.Errorf("question not specified"),
		}, fmt.Errorf("question not specified")
	}

	input, err := AskInput(question, placeholder, validator)
	if err != nil {
		return &SSOAgentAction{
			Type:      "ask_input",
			Question:  question,
			Result:    fmt.Sprintf("User cancelled input: %v", err),
			Error:     err,
			Timestamp: time.Now(),
		}, err
	}

	return &SSOAgentAction{
		Type:      "ask_input",
		Question:  question,
		Answer:    input,
		Result:    fmt.Sprintf("User entered: %s", input),
		Timestamp: time.Now(),
	}, nil
}

// Tool: web_search (integrates with existing web search)
func (a *SSOAgent) toolWebSearch(ctx context.Context, query string, agentCtx *SSOAgentContext) (*SSOAgentAction, error) {
	// Use existing web search implementation
	results, err := ExecuteWebSearch(ctx, query)
	if err != nil {
		return &SSOAgentAction{
			Type:      "web_search",
			Result:    fmt.Sprintf("Search failed: %v", err),
			Error:     err,
			Timestamp: time.Now(),
		}, err
	}

	return &SSOAgentAction{
		Type:      "web_search",
		Result:    results,
		Timestamp: time.Now(),
	}, nil
}

// Tool: aws_validate
func (a *SSOAgent) toolAWSValidate(ctx context.Context, command string, agentCtx *SSOAgentContext) (*SSOAgentAction, error) {
	// Parse command: sso_login|profile:dev
	//            or: credentials|profile:dev|account:123456789012
	//            or: cli_version

	if command == "cli_version" {
		// Check AWS CLI version
		err := a.inspector.CheckAWSCLI()
		if err != nil {
			return &SSOAgentAction{
				Type:      "aws_validate",
				Result:    fmt.Sprintf("AWS CLI check failed: %v", err),
				Error:     err,
				Timestamp: time.Now(),
			}, err
		}
		return &SSOAgentAction{
			Type:      "aws_validate",
			Result:    "AWS CLI v2+ is installed",
			Timestamp: time.Now(),
		}, nil
	}

	parts := strings.Split(command, "|")
	validationType := parts[0]

	var profile, expectedAccount string
	for _, part := range parts[1:] {
		if strings.HasPrefix(part, "profile:") {
			profile = strings.TrimPrefix(part, "profile:")
		} else if strings.HasPrefix(part, "account:") {
			expectedAccount = strings.TrimPrefix(part, "account:")
		}
	}

	if validationType == "sso_login" {
		// Test SSO login
		autoLogin := NewAutoLogin(profile)
		err := autoLogin.Login()
		if err != nil {
			return &SSOAgentAction{
				Type:      "aws_validate",
				Result:    fmt.Sprintf("SSO login failed: %v", err),
				Error:     err,
				Timestamp: time.Now(),
			}, err
		}
		return &SSOAgentAction{
			Type:      "aws_validate",
			Result:    fmt.Sprintf("✅ SSO login successful for profile '%s'", profile),
			Timestamp: time.Now(),
		}, nil

	} else if validationType == "credentials" {
		// Validate credentials
		autoLogin := NewAutoLogin(profile)
		result, err := autoLogin.ValidateCredentials(expectedAccount, agentCtx.Region)
		if err != nil {
			return &SSOAgentAction{
				Type:      "aws_validate",
				Result:    fmt.Sprintf("Credential validation failed: %v", err),
				Error:     err,
				Timestamp: time.Now(),
			}, err
		}

		if !result.Success {
			return &SSOAgentAction{
				Type:      "aws_validate",
				Result:    "Credential validation failed",
				Error:     fmt.Errorf("validation failed"),
				Timestamp: time.Now(),
			}, fmt.Errorf("validation failed")
		}

		return &SSOAgentAction{
			Type:      "aws_validate",
			Result:    fmt.Sprintf("✅ Credentials valid. Account: %s, ARN: %s", result.AccountID, result.ARN),
			Timestamp: time.Now(),
		}, nil
	}

	return &SSOAgentAction{
		Type:      "aws_validate",
		Result:    "Unknown validation type",
		Error:     fmt.Errorf("unknown validation type: %s", validationType),
		Timestamp: time.Now(),
	}, fmt.Errorf("unknown validation type")
}

// Helper function to copy file
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// CRITICAL: Create with restricted permissions (0600 = owner only)
	// This protects AWS config backups containing SSO session tokens
	destFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	// Ensure data is written to disk
	return destFile.Sync()
}
