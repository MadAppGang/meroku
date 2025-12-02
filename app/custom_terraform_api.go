package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type CustomTerraformFile struct {
	Path         string `json:"path"`
	Name         string `json:"name"`
	Content      string `json:"content"`
	Scope        string `json:"scope"` // "shared" or "environment"
	LastModified string `json:"lastModified,omitempty"`
}

type BridgeVariable struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Conditional string `json:"conditional,omitempty"`
}

type CustomModuleOutput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Value       string `json:"value"`
}

type CustomModuleStatus struct {
	ModuleName    string               `json:"moduleName"`
	Deployed      bool                 `json:"deployed"`
	ResourceCount int                  `json:"resourceCount,omitempty"`
	LastApplied   string               `json:"lastApplied,omitempty"`
	Outputs       []CustomModuleOutput `json:"outputs,omitempty"`
}

type ListFilesResponse struct {
	Files []CustomTerraformFile `json:"files"`
}

type GetFileResponse struct {
	File CustomTerraformFile `json:"file"`
}

type SaveFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Scope   string `json:"scope"`
}

type DeleteFileRequest struct {
	Path  string `json:"path"`
	Scope string `json:"scope"`
}

type BridgeVariablesResponse struct {
	Variables []BridgeVariable `json:"variables"`
}

type CustomModuleStatusResponse struct {
	Modules []CustomModuleStatus `json:"modules"`
}

// listCustomTerraformFiles lists all custom Terraform files for an environment
func listCustomTerraformFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	env := r.URL.Query().Get("env")
	if env == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "env parameter is required"})
		return
	}

	// Validate env parameter to prevent path traversal
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(env) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid env parameter format"})
		return
	}

	var files []CustomTerraformFile

	// Get shared custom terraform files
	sharedDir := filepath.Join("custom", "terraform", "_shared")
	if _, err := os.Stat(sharedDir); err == nil {
		sharedFiles, err := readTerraformFiles(sharedDir, "shared")
		if err == nil {
			files = append(files, sharedFiles...)
		}
	}

	// Get environment-specific custom terraform files
	envDir := filepath.Join("custom", "terraform", env)
	if _, err := os.Stat(envDir); err == nil {
		envFiles, err := readTerraformFiles(envDir, "environment")
		if err == nil {
			files = append(files, envFiles...)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ListFilesResponse{Files: files})
}

// readTerraformFiles reads all .tf files from a directory
func readTerraformFiles(dir string, scope string) ([]CustomTerraformFile, error) {
	var files []CustomTerraformFile

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tf") {
			filePath := filepath.Join(dir, entry.Name())
			content, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}

			info, err := entry.Info()
			var lastModified string
			if err == nil {
				lastModified = info.ModTime().Format(time.RFC3339)
			}

			files = append(files, CustomTerraformFile{
				Path:         entry.Name(),
				Name:         entry.Name(),
				Content:      string(content),
				Scope:        scope,
				LastModified: lastModified,
			})
		}
	}

	return files, nil
}

// getCustomTerraformFile gets a specific custom Terraform file
func getCustomTerraformFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	env := r.URL.Query().Get("env")
	path := r.URL.Query().Get("path")
	scope := r.URL.Query().Get("scope")

	if env == "" || path == "" || scope == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "env, path, and scope parameters are required"})
		return
	}

	// Validate env parameter to prevent path traversal
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(env) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid env parameter format"})
		return
	}

	// Validate scope
	if scope != "shared" && scope != "environment" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "scope must be 'shared' or 'environment'"})
		return
	}

	// Determine file path
	var filePath string
	if scope == "shared" {
		filePath = filepath.Join("custom", "terraform", "_shared", filepath.Base(path))
	} else {
		filePath = filepath.Join("custom", "terraform", env, filepath.Base(path))
	}

	// Read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "file not found"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		}
		return
	}

	info, err := os.Stat(filePath)
	var lastModified string
	if err == nil {
		lastModified = info.ModTime().Format(time.RFC3339)
	}

	file := CustomTerraformFile{
		Path:         filepath.Base(path),
		Name:         filepath.Base(path),
		Content:      string(content),
		Scope:        scope,
		LastModified: lastModified,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GetFileResponse{File: file})
}

// saveCustomTerraformFile saves a custom Terraform file
func saveCustomTerraformFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	env := r.URL.Query().Get("env")
	if env == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "env parameter is required"})
		return
	}

	// Validate env parameter to prevent path traversal
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(env) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid env parameter format"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to read request body"})
		return
	}
	defer r.Body.Close()

	var req SaveFileRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid JSON"})
		return
	}

	// Validate scope
	if req.Scope != "shared" && req.Scope != "environment" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "scope must be 'shared' or 'environment'"})
		return
	}

	// Validate file name
	if !strings.HasSuffix(req.Path, ".tf") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "file must have .tf extension"})
		return
	}

	// Determine directory and file path
	var dir, filePath string
	if req.Scope == "shared" {
		dir = filepath.Join("custom", "terraform", "_shared")
		filePath = filepath.Join(dir, filepath.Base(req.Path))
	} else {
		dir = filepath.Join("custom", "terraform", env)
		filePath = filepath.Join(dir, filepath.Base(req.Path))
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("failed to create directory: %v", err)})
		return
	}

	// Write file
	if err := os.WriteFile(filePath, []byte(req.Content), 0644); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("failed to write file: %v", err)})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "file saved successfully"})
}

// deleteCustomTerraformFile deletes a custom Terraform file
func deleteCustomTerraformFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	env := r.URL.Query().Get("env")
	if env == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "env parameter is required"})
		return
	}

	// Validate env parameter to prevent path traversal
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(env) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid env parameter format"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to read request body"})
		return
	}
	defer r.Body.Close()

	var req DeleteFileRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid JSON"})
		return
	}

	// Validate scope
	if req.Scope != "shared" && req.Scope != "environment" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "scope must be 'shared' or 'environment'"})
		return
	}

	// Determine file path
	var filePath string
	if req.Scope == "shared" {
		filePath = filepath.Join("custom", "terraform", "_shared", filepath.Base(req.Path))
	} else {
		filePath = filepath.Join("custom", "terraform", env, filepath.Base(req.Path))
	}

	// Delete file
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "file not found"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("failed to delete file: %v", err)})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "file deleted successfully"})
}

// getBridgeVariables returns available bridge variables
func getBridgeVariables(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	env := r.URL.Query().Get("env")
	if env == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "env parameter is required"})
		return
	}

	// Validate env parameter to prevent path traversal
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(env) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid env parameter format"})
		return
	}

	// Load environment config to check conditionals
	envData, err := loadEnv(env)

	variables := []BridgeVariable{
		{Name: "local.bridge.project", Description: "Project name", Type: "string"},
		{Name: "local.bridge.env", Description: "Environment name", Type: "string"},
		{Name: "local.bridge.region", Description: "AWS region", Type: "string"},
		{Name: "local.bridge.account_id", Description: "AWS account ID", Type: "string"},
		{Name: "local.bridge.vpc_id", Description: "VPC ID", Type: "string"},
		{Name: "local.bridge.subnet_ids", Description: "Subnet IDs", Type: "list(string)"},
		{Name: "local.bridge.api_endpoint", Description: "API Gateway endpoint", Type: "string"},
		{Name: "local.bridge.api_gateway_id", Description: "API Gateway ID", Type: "string"},
		{Name: "local.bridge.ecs_cluster_arn", Description: "ECS cluster ARN", Type: "string"},
		{Name: "local.bridge.ecs_cluster_name", Description: "ECS cluster name", Type: "string"},
		{Name: "local.bridge.backend_ecr_repo_url", Description: "Backend ECR URL", Type: "string"},
		{Name: "local.bridge.backend_task_role", Description: "Backend task role ARN", Type: "string"},
	}

	// Add conditional variables based on environment config
	if err == nil {
		if envData.Domain.Enabled {
			variables = append(variables,
				BridgeVariable{Name: "local.bridge.domain_zone_id", Description: "Route53 zone ID", Type: "string", Conditional: "domain.enabled"},
				BridgeVariable{Name: "local.bridge.api_domain", Description: "Custom API domain", Type: "string", Conditional: "domain.enabled"},
			)
		}
		if envData.Postgres.Enabled {
			variables = append(variables,
				BridgeVariable{Name: "local.bridge.db_endpoint", Description: "Database endpoint", Type: "string", Conditional: "postgres.enabled"},
			)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(BridgeVariablesResponse{Variables: variables})
}

// getCustomModuleStatus returns deployment status of custom modules
func getCustomModuleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	env := r.URL.Query().Get("env")
	if env == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "env parameter is required"})
		return
	}

	// Validate env parameter to prevent path traversal
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(env) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid env parameter format"})
		return
	}

	// TODO: Parse terraform state to get actual deployment status
	// For now, return empty modules list
	modules := []CustomModuleStatus{}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CustomModuleStatusResponse{Modules: modules})
}
