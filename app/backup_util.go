package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// BackupConfig holds configuration for creating backups
type BackupConfig struct {
	// SourcePath is the full path to the file to backup
	SourcePath string
	// BackupDir is the directory where backups should be stored
	// If empty, uses a 'backup' subdirectory relative to the source file
	BackupDir string
	// Permissions for the backup file (e.g., 0644, 0600)
	Permissions os.FileMode
	// PreserveDirectory whether to preserve the original directory structure in backup folder
	PreserveDirectory bool
}

// BackupResult contains information about the created backup
type BackupResult struct {
	BackupPath string
	Timestamp  string
	Success    bool
	Error      error
}

// CreateBackup creates a timestamped backup of a file in a centralized backup directory
func CreateBackup(config BackupConfig) (*BackupResult, error) {
	result := &BackupResult{
		Timestamp: time.Now().Format("20060102_150405"),
	}

	// Read source file
	sourceData, err := os.ReadFile(config.SourcePath)
	if err != nil {
		result.Error = fmt.Errorf("failed to read source file: %w", err)
		return result, result.Error
	}

	// Determine backup directory
	backupDir := config.BackupDir
	if backupDir == "" {
		// Use backup folder in the same directory as source file
		sourceDir := filepath.Dir(config.SourcePath)
		backupDir = filepath.Join(sourceDir, "backup")
	}

	// Create backup directory if it doesn't exist
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		result.Error = fmt.Errorf("failed to create backup directory: %w", err)
		return result, result.Error
	}

	// Construct backup filename
	var backupPath string
	if config.PreserveDirectory {
		// Preserve directory structure
		sourceAbs, err := filepath.Abs(config.SourcePath)
		if err != nil {
			result.Error = fmt.Errorf("failed to get absolute path: %w", err)
			return result, result.Error
		}
		relPath, err := filepath.Rel(filepath.Dir(backupDir), sourceAbs)
		if err != nil {
			result.Error = fmt.Errorf("failed to get relative path: %w", err)
			return result, result.Error
		}
		backupPath = filepath.Join(backupDir, relPath)
	} else {
		// Just use filename
		sourceName := filepath.Base(config.SourcePath)
		backupPath = filepath.Join(backupDir, sourceName)
	}

	// Add timestamp to filename
	backupPath = fmt.Sprintf("%s.backup_%s", backupPath, result.Timestamp)

	// Handle collision by adding microseconds
	if _, err := os.Stat(backupPath); err == nil {
		microTimestamp := time.Now().Format("20060102_150405.000000")
		backupPath = fmt.Sprintf("%s.backup_%s", filepath.Join(backupDir, filepath.Base(config.SourcePath)), microTimestamp)
	}

	// Create backup directory structure if needed
	backupFileDir := filepath.Dir(backupPath)
	if err := os.MkdirAll(backupFileDir, 0755); err != nil {
		result.Error = fmt.Errorf("failed to create backup file directory: %w", err)
		return result, result.Error
	}

	// Set permissions (default to 0644 if not specified)
	permissions := config.Permissions
	if permissions == 0 {
		permissions = 0644
	}

	// Write backup file
	if err := os.WriteFile(backupPath, sourceData, permissions); err != nil {
		result.Error = fmt.Errorf("failed to write backup file: %w", err)
		return result, result.Error
	}

	result.BackupPath = backupPath
	result.Success = true
	return result, nil
}

// CreateProjectBackup creates a backup for project YAML files in project/backup/
func CreateProjectBackup(yamlPath string) (string, error) {
	config := BackupConfig{
		SourcePath:  yamlPath,
		BackupDir:   filepath.Join(filepath.Dir(yamlPath), "backup"),
		Permissions: 0644,
	}

	result, err := CreateBackup(config)
	if err != nil {
		return "", err
	}

	return result.BackupPath, nil
}

// CreateAWSConfigBackup creates a backup for AWS config files (~/.aws/config) with restricted permissions
func CreateAWSConfigBackup(configPath string) (string, error) {
	config := BackupConfig{
		SourcePath:  configPath,
		BackupDir:   filepath.Join(filepath.Dir(configPath), "backup"),
		Permissions: 0600, // More restrictive for AWS credentials
	}

	result, err := CreateBackup(config)
	if err != nil {
		return "", err
	}

	return result.BackupPath, nil
}

// CreateFileBackupWithPermissions creates a backup with custom permissions
func CreateFileBackupWithPermissions(filePath string, permissions os.FileMode) (string, error) {
	config := BackupConfig{
		SourcePath:  filePath,
		BackupDir:   filepath.Join(filepath.Dir(filePath), "backup"),
		Permissions: permissions,
	}

	result, err := CreateBackup(config)
	if err != nil {
		return "", err
	}

	return result.BackupPath, nil
}

// copyFileToBackup is a helper that copies a file to the backup directory
// Deprecated: Use CreateBackup instead
func copyFileToBackup(src, backupPath string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	return nil
}
