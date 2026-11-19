package main

// GET /api/events/deployed?env=<name>
//
// What the last deploy left of the env's events resources, for the web UI's
// event editors: which consumers have an ordered (FIFO) queue set deployed,
// and which principals the enhanced bus is shared with. It answers from what
// generate decides from, through the same functions: the Terraform state
// whenever a render would depend on it (eventsStateRequired), else the
// events markers of the previous env/<env>/main.tf. Reading the state may
// call S3 or `terraform state pull`; nothing is written.
//
//	200 {"known": bool, "fifoConsumers": [...], "sharedPrincipals": [...], "subscribePrincipals": [...]}
//	404 {"error": "unknown environment: <name>"}
//
// known is false when neither answered: the state was needed and could not be
// read, there is no previous render and no backend to read, or the YAML does
// not load. The lists are then empty and say nothing.

import (
	"context"
	"net/http"
)

// eventsDeployedReport is the body of GET /api/events/deployed.
type eventsDeployedReport struct {
	Known bool `json:"known"`
	// FifoConsumers are the workloads whose FIFO queue set is deployed,
	// active or retired (fifo_retired).
	FifoConsumers []string `json:"fifoConsumers"`
	// SharedPrincipals are the principals the bus is shared with now, any
	// access: account:<id>, ou:<id> or organization.
	SharedPrincipals []string `json:"sharedPrincipals"`
	// SubscribePrincipals is the part of SharedPrincipals whose deployed
	// grant lets them create subscribers (access subscribe or full).
	SubscribePrincipals []string `json:"subscribePrincipals"`
}

func getEventsDeployed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
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
		writeAPIError(w, http.StatusNotFound, "unknown environment: "+envName)
		return
	}
	writeJSON(w, http.StatusOK, deployedEventsReport(r.Context(), envName))
}

// deployedEventsReport computes the report for env from its YAML on disk.
// ctx is the request's, so a request the browser cancels stops its state
// read.
func deployedEventsReport(ctx context.Context, env string) (report eventsDeployedReport) {
	unknown := eventsDeployedReport{FifoConsumers: []string{}, SharedPrincipals: []string{}, SubscribePrincipals: []string{}}
	report = unknown
	// The probe runs type assertions on a hand-edited YAML; a panic in one
	// must not take down the web server.
	defer func() {
		if recover() != nil {
			report = unknown
		}
	}()

	envFile, envMap, err := loadEnvMapReadOnly(env)
	if err != nil {
		return unknown
	}
	prepareEventInputs(envMap, env, envFile)
	prepareEventStateProbe(ctx, envMap, env)
	d := readEventDeployed(envMap)
	if !d.fromState() && (d.state.Needed || !d.hasPrevious) {
		return unknown
	}
	return eventsDeployedReport{
		Known:               true,
		FifoConsumers:       d.deployedFifoConsumers(),
		SharedPrincipals:    d.sharedPrincipals(),
		SubscribePrincipals: d.subscribePrincipals(),
	}
}
