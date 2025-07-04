package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type NodePosition struct {
	NodeID   string  `json:"nodeId"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
}

type BoardPositions struct {
	Environment string         `json:"environment"`
	Positions   []NodePosition `json:"positions"`
}

func getPositionsFilePath(environment string) string {
	// Store positions in a .positions directory
	dir := ".positions"
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, fmt.Sprintf("%s-positions.json", environment))
}

func getNodePositions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	environment := r.URL.Query().Get("environment")
	if environment == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "environment parameter is required"})
		return
	}

	filePath := getPositionsFilePath(environment)
	
	// Check if positions file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// Return empty positions if file doesn't exist
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(BoardPositions{
			Environment: environment,
			Positions:   []NodePosition{},
		})
		return
	}

	// Read the positions file
	data, err := os.ReadFile(filePath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Failed to read positions: %v", err)})
		return
	}

	var positions BoardPositions
	if err := json.Unmarshal(data, &positions); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Failed to parse positions: %v", err)})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(positions)
}

func saveNodePositions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to read request body"})
		return
	}
	defer r.Body.Close()

	var positions BoardPositions
	if err := json.Unmarshal(body, &positions); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Invalid request body: %v", err)})
		return
	}

	if positions.Environment == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "environment is required"})
		return
	}

	filePath := getPositionsFilePath(positions.Environment)
	
	// Marshal positions to JSON with indentation for readability
	data, err := json.MarshalIndent(positions, "", "  ")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Failed to marshal positions: %v", err)})
		return
	}

	// Write to file
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Failed to save positions: %v", err)})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Positions saved successfully",
		"environment": positions.Environment,
	})
}