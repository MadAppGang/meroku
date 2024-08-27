package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func findFilesWithExts(exts []string) ([]string, error) {
	var files []string
	entries, err := os.ReadDir(".")
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileExt := filepath.Ext(entry.Name())
		for _, ext := range exts {
			if strings.EqualFold(fileExt, ext) {
				baseName := filepath.Base(entry.Name())
				nameWithoutExt := strings.TrimSuffix(baseName, fileExt)
				files = append(files, nameWithoutExt)
				break
			}
		}
	}

	return files, nil
}

func createFolderIfNotExists(folderPath string) error {
	// Check if the folder exists
	_, err := os.Stat(folderPath)
	if os.IsNotExist(err) {
		// Folder does not exist, create it
		err := os.MkdirAll(folderPath, os.ModePerm)
		if err != nil {
			return fmt.Errorf("failed to create folder: %v", err)
		}
	} else if err != nil {
		// Some other error occurred
		return fmt.Errorf("error checking folder: %v", err)
	}
	return nil
}
