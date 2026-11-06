package main

// GET /api/ses-dns?env=<name>&domain=<ses domain>[&candidate=<spf record>][&live=0]
//
// The body is sesDNSReport (ses_dns_service.go). Read-only, like
// /api/vpc-cidr: the web client saves whole YAML documents.
//
// 4xx only for the caller's own mistakes (unknown env, unknown domain); an
// AWS failure is a 200 carrying live.read_error.

import (
	"context"
	"net/http"
	"slices"
)

// sesDNSAPIReader is the seam tests replace to keep the handler off AWS.
var sesDNSAPIReader sesDNSLiveReader = readRoute53TXT

// sesDNSAPILoadEnvMap loads the requested environment's YAML as a map. A seam
// for tests.
var sesDNSAPILoadEnvMap = func(envName string) (map[string]interface{}, error) {
	path, err := resolveEnvFilePath(envName)
	if err != nil {
		return nil, err
	}
	return loadEnvToMap(path)
}

func getSESDNSReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()

	envName := q.Get("env")
	domain := q.Get("domain")
	if envName == "" || domain == "" {
		writeAPIError(w, http.StatusBadRequest, "env and domain parameters are required")
		return
	}
	names, err := listEnvironmentNames()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !slices.Contains(names, envName) {
		writeAPIError(w, http.StatusBadRequest, "unknown environment: "+envName)
		return
	}

	var candidate *string
	if q.Has("candidate") {
		c := q.Get("candidate")
		candidate = &c
	}

	envMap, err := sesDNSAPILoadEnvMap(envName)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "failed to load environment "+envName+": "+err.Error())
		return
	}

	read := sesDNSAPIReader
	if q.Get("live") == "0" || q.Get("live") == "false" {
		read = nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), sesDNSAWSTimeout)
	defer cancel()
	report, err := inspectSESDNS(ctx, envMap, domain, candidate, read)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}
