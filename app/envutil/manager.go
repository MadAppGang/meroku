package envutil

import (
	"fmt"
	"os"
)

// Manager handles environment variable management with tracking and restoration
// This provides a safer way to temporarily modify environment variables
type Manager struct {
	original map[string]string
}

// NewManager creates a new environment variable manager
func NewManager() *Manager {
	return &Manager{
		original: make(map[string]string),
	}
}

// Set sets an environment variable and tracks the original value for restoration
func (m *Manager) Set(key, value string) error {
	// Save original value if not already saved
	if _, exists := m.original[key]; !exists {
		m.original[key] = os.Getenv(key)
	}

	if err := os.Setenv(key, value); err != nil {
		return fmt.Errorf("failed to set %s: %w", key, err)
	}

	return nil
}

// Get retrieves the current value of an environment variable
func (m *Manager) Get(key string) string {
	return os.Getenv(key)
}

// Restore restores all environment variables to their original values
func (m *Manager) Restore() {
	for key, value := range m.original {
		if value == "" {
			os.Unsetenv(key)
		} else {
			os.Setenv(key, value)
		}
	}
}

// RestoreKey restores a specific environment variable to its original value
func (m *Manager) RestoreKey(key string) {
	if value, exists := m.original[key]; exists {
		if value == "" {
			os.Unsetenv(key)
		} else {
			os.Setenv(key, value)
		}
		delete(m.original, key)
	}
}

// SetAWSProfile sets the AWS_PROFILE environment variable
func (m *Manager) SetAWSProfile(profile string) error {
	return m.Set("AWS_PROFILE", profile)
}

// SetAWSRegion sets both AWS_REGION and AWS_DEFAULT_REGION environment variables
func (m *Manager) SetAWSRegion(region string) error {
	if err := m.Set("AWS_REGION", region); err != nil {
		return err
	}
	return m.Set("AWS_DEFAULT_REGION", region)
}

// SetAWSConfig sets both AWS profile and region in one call
func (m *Manager) SetAWSConfig(profile, region string) error {
	if profile != "" {
		if err := m.SetAWSProfile(profile); err != nil {
			return err
		}
	}
	if region != "" {
		return m.SetAWSRegion(region)
	}
	return nil
}

// SetBuildEnv sets environment variables for Go cross-compilation
func (m *Manager) SetBuildEnv(goos, goarch string) error {
	if err := m.Set("GOOS", goos); err != nil {
		return err
	}
	return m.Set("GOARCH", goarch)
}
