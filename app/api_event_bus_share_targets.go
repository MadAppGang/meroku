package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// resolveShareTargetsEnvFile finds the env file the share targets are listed
// for. A variable so tests can replace it.
var resolveShareTargetsEnvFile = resolveEnvFilePath

func writeShareTargetsJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// getEventBusShareTargets lists the sibling environments of the same project
// an event_bus.share entry can name with env, each with its status (ok,
// ambiguous, no_account, own_account) and the files that declare it. It uses
// the same rule generate applies to share entries, eventBusShareTargets, on
// the directory of the env's own file.
//
// The env file is read raw for project and account_id: no migration, no
// backup, no rewrite.
//
// GET /api/event-bus/share-targets?env=dev
func getEventBusShareTargets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	envName := r.URL.Query().Get("env")
	if envName == "" {
		writeShareTargetsJSON(w, http.StatusBadRequest, ErrorResponse{Error: "env parameter is required"})
		return
	}
	if !validEventAPIEnvName(envName) {
		writeShareTargetsJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid env name: use letters, digits, hyphens and underscores"})
		return
	}

	file, err := resolveShareTargetsEnvFile(envName)
	if err != nil {
		writeShareTargetsJSON(w, http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}

	data, err := os.ReadFile(file)
	if err != nil {
		writeShareTargetsJSON(w, http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("failed to read %s: %v", file, err)})
		return
	}
	var own vpcCIDRProbe
	if err := yaml.Unmarshal(data, &own); err != nil {
		writeShareTargetsJSON(w, http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("failed to parse %s: %v", file, err)})
		return
	}

	targets, err := eventBusShareTargets(filepath.Dir(file), envName, own.Env, own.Project, own.AccountID)
	if err != nil {
		writeShareTargetsJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	if targets == nil {
		targets = []shareTarget{}
	}
	writeShareTargetsJSON(w, http.StatusOK, targets)
}
