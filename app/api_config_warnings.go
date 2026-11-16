package main

// GET /api/config-warnings?env=<name>
//
// The body is ConfigWarningReport (config_warnings.go): every configuration
// warning for the env, computed from its YAML on disk at request time, so the
// next GET after a save reflects the edit. Read-only: it never writes or
// migrates the file, never prints, and never calls AWS or the network.
//
// Status codes follow api_vpc_cidr.go: 4xx only for the caller's own mistakes
// (no env, unknown env); a YAML generate would refuse is a 200 carrying
// configError.

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// configWarningsAPIInputs builds the inputs for the env-level sources. A seam
// for tests. The web report has no region_drift: the server's AWS_REGION is
// not the shell any deploy runs in.
var configWarningsAPIInputs = webWarningInputs

func getConfigWarnings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	envName := r.URL.Query().Get("env")
	if envName == "" {
		writeAPIError(w, http.StatusBadRequest, "env parameter is required")
		return
	}
	exists, err := configWarningsEnvExists(envName)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		writeAPIError(w, http.StatusBadRequest, "unknown environment: "+envName)
		return
	}

	writeJSON(w, http.StatusOK, collectEnvConfigWarnings(envName, configWarningsAPIInputs(envName)))
}

// configWarningsEnvExists reports whether envName is an environment of the
// project. A listed environment exists, and so does a name whose file is
// present as <name>.yaml or project/<name>.yaml even when that file does not
// parse: a YAML that cannot be read is what the report's configError is for.
//
// A name that could reach outside the project directory is never an
// environment, so raw input never builds such a path.
func configWarningsEnvExists(envName string) (bool, error) {
	if envName == "." || envName == ".." || envName == "dns" || strings.ContainsAny(envName, `/\`) {
		return false, nil
	}
	names, err := listEnvironmentNames()
	if err != nil {
		return false, err
	}
	if slices.Contains(names, envName) {
		return true, nil
	}
	for _, path := range []string{envName + ".yaml", filepath.Join("project", envName+".yaml")} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return true, nil
		}
	}
	return false, nil
}
