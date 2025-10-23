package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/ini.v1"
)

// ReadAWSConfig reads the entire AWS config file
func ReadAWSConfig(configPath string) (string, error) {
	// Resolve ~ to home directory
	if strings.HasPrefix(configPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		configPath = filepath.Join(home, configPath[2:])
	}

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return "", fmt.Errorf("config file does not exist: %s", configPath)
	}

	// Read file
	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %w", err)
	}

	return string(content), nil
}

// ParseAWSConfigProfiles extracts all profile names from config content
func ParseAWSConfigProfiles(content string) ([]string, error) {
	cfg, err := ini.Load([]byte(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	profiles := []string{}
	for _, section := range cfg.Sections() {
		name := section.Name()
		if name == "DEFAULT" || name == "default" {
			profiles = append(profiles, "default")
		} else if strings.HasPrefix(name, "profile ") {
			profileName := strings.TrimPrefix(name, "profile ")
			profiles = append(profiles, profileName)
		}
	}

	return profiles, nil
}

// ParseSSOSessions extracts all sso-session names from config content
func ParseSSOSessions(content string) ([]string, error) {
	cfg, err := ini.Load([]byte(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	sessions := []string{}
	for _, section := range cfg.Sections() {
		name := section.Name()
		if strings.HasPrefix(name, "sso-session ") {
			sessionName := strings.TrimPrefix(name, "sso-session ")
			sessions = append(sessions, sessionName)
		}
	}

	return sessions, nil
}

// GetProfileSection extracts a specific profile section as a map
func GetProfileSection(content, profileName string) (map[string]string, error) {
	cfg, err := ini.Load([]byte(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	sectionName := "profile " + profileName
	if profileName == "default" {
		sectionName = "default"
	}

	section, err := cfg.GetSection(sectionName)
	if err != nil {
		return nil, fmt.Errorf("profile not found: %s", profileName)
	}

	result := make(map[string]string)
	for _, key := range section.Keys() {
		result[key.Name()] = key.String()
	}

	return result, nil
}

// GetSSOSessionSection extracts a specific sso-session section
func GetSSOSessionSection(content, sessionName string) (map[string]string, error) {
	cfg, err := ini.Load([]byte(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	sectionName := "sso-session " + sessionName
	section, err := cfg.GetSection(sectionName)
	if err != nil {
		return nil, fmt.Errorf("sso-session not found: %s", sessionName)
	}

	result := make(map[string]string)
	for _, key := range section.Keys() {
		result[key.Name()] = key.String()
	}

	return result, nil
}
