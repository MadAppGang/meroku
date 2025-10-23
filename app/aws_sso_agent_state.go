package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	StateVersion = "1.0"
	StateDir     = "/tmp"
)

// SSOAgentState represents persisted agent state
type SSOAgentState struct {
	Version         string            `json:"version"`
	ProfileName     string            `json:"profile_name"`
	SaveTime        time.Time         `json:"save_time"`
	Context         *SSOAgentContext  `json:"context"`
	IsComplete      bool              `json:"is_complete"`
	CompletionMsg   string            `json:"completion_message"`
	TotalIterations int               `json:"total_iterations"`
	RunNumber       int               `json:"run_number"`
}

// GetStateFilePath generates state file path for a profile
func GetStateFilePath(profileName string) string {
	timestamp := time.Now().Format("20060102")
	filename := fmt.Sprintf("meroku_sso_state_%s_%s.json", profileName, timestamp)
	return filepath.Join(StateDir, filename)
}

// SaveState persists agent state to disk
func SaveState(state *SSOAgentState, filepath string) error {
	state.Version = StateVersion
	state.SaveTime = time.Now()

	// Marshal to JSON with indentation for readability
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Write with restricted permissions (owner only)
	err = os.WriteFile(filepath, data, 0600)
	if err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// LoadState loads agent state from disk
func LoadState(filepath string) (*SSOAgentState, error) {
	// Check if file exists
	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		return nil, fmt.Errorf("state file does not exist: %s", filepath)
	}

	// Read file
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	// Unmarshal
	var state SSOAgentState
	err = json.Unmarshal(data, &state)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	// Verify version compatibility
	if state.Version != StateVersion {
		return nil, fmt.Errorf("incompatible state version: %s (expected %s)", state.Version, StateVersion)
	}

	return &state, nil
}

// CleanupStateFile deletes state file after successful completion
func CleanupStateFile(filepath string) error {
	if filepath == "" {
		return nil // Nothing to clean up
	}

	err := os.Remove(filepath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove state file: %w", err)
	}

	return nil
}

// ListStateFiles finds all state files for debugging
func ListStateFiles() ([]string, error) {
	pattern := filepath.Join(StateDir, "meroku_sso_state_*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to list state files: %w", err)
	}
	return matches, nil
}
