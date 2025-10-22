package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/ini.v1"
)

// ConfigWriter safely writes AWS configuration files
type ConfigWriter struct {
	configPath string
}

// NewConfigWriter creates a new config writer
func NewConfigWriter() *ConfigWriter {
	return &ConfigWriter{
		configPath: getAWSConfigPath(),
	}
}

// WriteModernSSOProfile writes a modern SSO profile configuration
func (cw *ConfigWriter) WriteModernSSOProfile(opts ModernSSOProfileOptions) error {
	// Create backup first
	if err := cw.createBackup(); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Load or create config
	cfg, err := cw.loadOrCreateConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Write sso-session section
	if err := cw.writeSSOSessionSection(cfg, opts.SSOSessionName, opts); err != nil {
		return fmt.Errorf("failed to write sso-session: %w", err)
	}

	// Write profile section
	if err := cw.writeProfileSection(cfg, opts.ProfileName, opts); err != nil {
		return fmt.Errorf("failed to write profile: %w", err)
	}

	// Save configuration
	if err := cw.saveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

// WriteLegacySSOProfile writes a legacy SSO profile configuration
func (cw *ConfigWriter) WriteLegacySSOProfile(opts LegacySSOProfileOptions) error {
	// Create backup first
	if err := cw.createBackup(); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Load or create config
	cfg, err := cw.loadOrCreateConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Get or create profile section
	sectionName := getSectionName(opts.ProfileName)
	section, err := cfg.NewSection(sectionName)
	if err != nil {
		// Section might already exist
		section = cfg.Section(sectionName)
	}

	// Write required SSO fields
	section.Key("sso_start_url").SetValue(opts.SSOStartURL)
	section.Key("sso_region").SetValue(opts.SSORegion)
	section.Key("sso_account_id").SetValue(opts.SSOAccountID)
	section.Key("sso_role_name").SetValue(opts.SSORoleName)

	// Write optional fields
	if opts.Region != "" {
		section.Key("region").SetValue(opts.Region)
	}
	if opts.Output != "" {
		section.Key("output").SetValue(opts.Output)
	}

	// Save configuration
	if err := cw.saveConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

// writeSSOS essSection writes or updates an sso-session section
func (cw *ConfigWriter) writeSSOSessionSection(cfg *ini.File, sessionName string, opts ModernSSOProfileOptions) error {
	sectionName := fmt.Sprintf("sso-session %s", sessionName)
	section, err := cfg.NewSection(sectionName)
	if err != nil {
		// Section might already exist
		section = cfg.Section(sectionName)
	}

	// Write required fields
	section.Key("sso_start_url").SetValue(opts.SSOStartURL)
	section.Key("sso_region").SetValue(opts.SSORegion)

	// Write optional fields
	if opts.SSORegistrationScopes != "" {
		section.Key("sso_registration_scopes").SetValue(opts.SSORegistrationScopes)
	} else {
		// Use default if not specified
		section.Key("sso_registration_scopes").SetValue("sso:account:access")
	}

	return nil
}

// writeProfileSection writes or updates a profile section
func (cw *ConfigWriter) writeProfileSection(cfg *ini.File, profileName string, opts ModernSSOProfileOptions) error {
	sectionName := getSectionName(profileName)
	section, err := cfg.NewSection(sectionName)
	if err != nil {
		// Section might already exist
		section = cfg.Section(sectionName)
	}

	// Write required fields
	section.Key("sso_session").SetValue(opts.SSOSessionName)
	section.Key("sso_account_id").SetValue(opts.SSOAccountID)
	section.Key("sso_role_name").SetValue(opts.SSORoleName)

	// Write optional fields
	if opts.Region != "" {
		section.Key("region").SetValue(opts.Region)
	}
	if opts.Output != "" {
		section.Key("output").SetValue(opts.Output)
	}

	return nil
}

// createBackup creates a timestamped backup of the config file
func (cw *ConfigWriter) createBackup() error {
	// Check if config file exists
	if _, err := os.Stat(cw.configPath); os.IsNotExist(err) {
		// No config to backup
		return nil
	}

	// Create backup path with timestamp
	timestamp := time.Now().Format("20060102_150405")
	backupPath := fmt.Sprintf("%s.backup.%s", cw.configPath, timestamp)

	// Read original file
	data, err := os.ReadFile(cw.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Write backup
	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write backup: %w", err)
	}

	fmt.Printf("✅ Backup created: %s\n", backupPath)
	return nil
}

// loadOrCreateConfig loads existing config or creates new one
func (cw *ConfigWriter) loadOrCreateConfig() (*ini.File, error) {
	// Check if config file exists
	if _, err := os.Stat(cw.configPath); os.IsNotExist(err) {
		// Create directory if needed
		configDir := filepath.Dir(cw.configPath)
		if err := os.MkdirAll(configDir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create .aws directory: %w", err)
		}

		// Create new empty config
		return ini.Empty(), nil
	}

	// Load existing config
	cfg, err := ini.Load(cw.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return cfg, nil
}

// saveConfig saves configuration to disk with proper permissions
func (cw *ConfigWriter) saveConfig(cfg *ini.File) error {
	// Write to temp file first (atomic write)
	tempPath := cw.configPath + ".tmp"

	if err := cfg.SaveTo(tempPath); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Set proper permissions (read/write for user only)
	if err := os.Chmod(tempPath, 0600); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, cw.configPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	fmt.Printf("✅ Configuration written to %s\n", cw.configPath)
	return nil
}

// ModernSSOProfileOptions contains options for modern SSO profile
type ModernSSOProfileOptions struct {
	ProfileName            string
	SSOSessionName         string // Name of sso-session section
	SSOStartURL            string
	SSORegion              string
	SSOAccountID           string
	SSORoleName            string
	SSORegistrationScopes  string // Optional, defaults to "sso:account:access"
	Region                 string // Optional
	Output                 string // Optional
}

// LegacySSOProfileOptions contains options for legacy SSO profile
type LegacySSOProfileOptions struct {
	ProfileName  string
	SSOStartURL  string
	SSORegion    string
	SSOAccountID string
	SSORoleName  string
	Region       string // Optional
	Output       string // Optional
}
