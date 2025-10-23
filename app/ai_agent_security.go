package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// validateCommand validates a command before execution to prevent dangerous operations
func validateCommand(command string) error {
	// Block dangerous patterns (case-insensitive)
	dangerousPatterns := []string{
		`rm\s*-rf\s*/`,
		`rm\s*-rf\s*~`,
		`rm\s*-rf\s*\*`,
		`rm\s*-rf?\s*\.`,      // Blocks rm -rf . and rm -r .
		`rm\s*-r.*\*`,         // Blocks rm -r * variations
		`rm\s+.*-rf`,          // Catches flag order variations
		`dd\s+if=`,
		`mkfs`,
		`>/dev/sd`,
		`curl.*\|\s*sh`,
		`wget.*\|\s*bash`,
		`chmod\s*777`,
		`:(){:|:&};:`, // fork bomb
		`>\s*/dev`,
		`format\s+`,
		`fdisk`,
		`eval\s+`,
	}

	for _, pattern := range dangerousPatterns {
		// Add case-insensitive flag
		matched, _ := regexp.MatchString(`(?i)`+pattern, command)
		if matched {
			return fmt.Errorf("dangerous command pattern detected: matches '%s'", pattern)
		}
	}

	// Whitelist allowed base commands
	cmdParts := strings.Fields(command)
	if len(cmdParts) == 0 {
		return fmt.Errorf("empty command")
	}

	baseCmd := filepath.Base(cmdParts[0])
	allowedCommands := map[string]bool{
		"aws":       true,
		"terraform": true,
		"cat":       true,
		"grep":      true,
		"ls":        true,
		"echo":      true,
		"head":      true,
		"tail":      true,
		"find":      true,
		"wc":        true,
		"diff":      true,
		"git":       true,
		"pwd":       true,
		"cd":        true,
		"mkdir":     true,
		"touch":     true,
		"jq":        true,
		// SECURITY: sed and awk removed - can modify files and execute commands
	}

	if !allowedCommands[baseCmd] {
		return fmt.Errorf("command not in whitelist: %s", baseCmd)
	}

	return nil
}

// validateFilePath validates a file path to prevent path traversal attacks
func validateFilePath(basePath, filePath string) error {
	// Get absolute base path
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return fmt.Errorf("invalid base path: %w", err)
	}

	// Get absolute file path
	var absFile string
	if filepath.IsAbs(filePath) {
		absFile = filePath
		// SECURITY FIX: Reject absolute paths outside working directory
		// Check if the absolute path is within or equal to the base path
		if !strings.HasPrefix(absFile, absBase+string(filepath.Separator)) && absFile != absBase {
			return fmt.Errorf("absolute file path outside working directory: %s", filePath)
		}
	} else {
		absFile = filepath.Join(absBase, filePath)
	}

	// Clean the path to remove . and .. components
	absFile = filepath.Clean(absFile)

	// Get absolute path again after cleaning
	absFile, err = filepath.Abs(absFile)
	if err != nil {
		return fmt.Errorf("invalid file path: %w", err)
	}

	// Double-check: verify file is still within base directory after cleaning
	relPath, err := filepath.Rel(absBase, absFile)
	if err != nil {
		return fmt.Errorf("unable to determine relative path: %w", err)
	}

	// Block any path traversal attempts
	if strings.HasPrefix(relPath, "..") {
		return fmt.Errorf("file path escapes working directory: %s", filePath)
	}

	// SECURITY: Block sensitive system paths ONLY if they're outside the working directory
	// If a file is within our working directory, we trust it regardless of its absolute path
	// This allows working directories like /var/folders/... on macOS temp dirs
	if !strings.HasPrefix(absFile, absBase+string(filepath.Separator)) && absFile != absBase {
		// File is outside working directory - now check if it's a system path
		sensitivePaths := []string{
			"/etc/",
			"/usr/bin/",
			"/usr/sbin/",
			"/bin/",
			"/sbin/",
			"/var/",
			"/sys/",
			"/proc/",
		}

		for _, sensPath := range sensitivePaths {
			if strings.HasPrefix(absFile, sensPath) {
				return fmt.Errorf("access to system paths not allowed: %s", filePath)
			}
		}
	}

	return nil
}

// validateFileSize validates that a file is not too large to process
func validateFileSize(filePath string, maxSize int64) error {
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, OK
		}
		return fmt.Errorf("unable to stat file: %w", err)
	}

	if info.Size() > maxSize {
		return fmt.Errorf("file too large: %d bytes (max %d bytes)", info.Size(), maxSize)
	}

	return nil
}

// createFileBackup creates a timestamped backup of a file before modification in backup/ directory
func createFileBackup(filePath string) (string, error) {
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", nil // No file to backup
	}

	backupPath, err := CreateFileBackupWithPermissions(filePath, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}

	return backupPath, nil
}
