package main

// GET /api/vpc-cidr?env=<name>[&aws=1][&count=<1..10>][&candidate=<string>]
//
// The body is vpcCIDRReport (vpc_cidr_service.go); the contract is
// ai-docs/sessions/.../api-contract.md. Read-only: there is no POST, because
// the web client saves whole YAML documents and would overwrite a server-side
// write with the copy it already holds.
//
// Status codes follow api_compute.go: 4xx only for the caller's own mistakes
// (unknown env, bad count); an AWS failure is a 200 carrying awsNote.

import (
	"context"
	"net/http"
	"slices"
	"strconv"
)

const (
	vpcCIDRDefaultSuggestions = 5
	vpcCIDRMaxSuggestions     = 10
)

// vpcCIDRAPIDeps is the seam tests replace to keep the handler off AWS.
var vpcCIDRAPIDeps = defaultVPCCIDRDeps

// vpcCIDRAPILoadEnv loads the requested environment. A seam for tests.
var vpcCIDRAPILoadEnv = loadEnv

func getVPCCIDRReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()

	envName := q.Get("env")
	if envName == "" {
		writeAPIError(w, http.StatusBadRequest, "env parameter is required")
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

	count := vpcCIDRDefaultSuggestions
	if raw := q.Get("count"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "count must be a whole number between 1 and 10")
			return
		}
		count = min(max(n, 1), vpcCIDRMaxSuggestions)
	}

	withAWS := q.Get("aws") == "1" || q.Get("aws") == "true"

	var candidate *string
	if q.Has("candidate") {
		c := q.Get("candidate")
		candidate = &c
	}

	env, err := vpcCIDRAPILoadEnv(envName)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "failed to load environment "+envName+": "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), vpcCIDRAWSTimeout)
	defer cancel()
	report := inspectVPCCIDR(ctx, vpcCIDRAPIDeps(), env, vpcCIDRInspectOptions{
		EnvName:   envName,
		Dir:       ".",
		WithAWS:   withAWS, // ignored by inspectVPCCIDR when candidate is set
		Count:     count,
		Candidate: candidate,
	})
	writeJSON(w, http.StatusOK, report)
}
