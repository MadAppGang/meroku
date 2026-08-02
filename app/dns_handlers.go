package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// HTTP surface for the DNS layer.
//
// Everything here wraps functions that already existed in dns_api.go,
// dns_preflight.go and dns_delegation.go. Until now that whole subsystem was
// reachable only from the CLI/TUI — spa_server.go registered no DNS routes at
// all — which is why the web UI showed a hand-written guess at the DNS records
// instead of the real ones.
//
// Read endpoints are GET. The one endpoint that changes live DNS is POST and
// requires an explicit confirm flag, so it cannot be triggered by a stray
// navigation or prefetch.

// dnsStatusResponse mirrors dnsPreflightResult for the browser.
type dnsStatusResponse struct {
	Environment string `json:"environment"`
	Plan        string `json:"plan"`
	Reason      string `json:"reason"`

	ZoneName     string `json:"zone_name"`
	ParentDomain string `json:"parent_domain,omitempty"`
	ZoneID       string `json:"zone_id,omitempty"`

	ZoneNameservers   []string `json:"zone_nameservers,omitempty"`
	PublicNameservers []string `json:"public_nameservers,omitempty"`

	Delegated        bool `json:"delegated"`
	NeedsDelegation  bool `json:"needs_delegation"`
	ParentIsRoute53  bool `json:"parent_is_route53"`
	CanAutoDelegate  bool `json:"can_auto_delegate"`
	ZoneExists       bool `json:"zone_exists"`
	CertificateReady bool `json:"certificate_ready"`
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeAPIError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

// loadEnvForRequest resolves the ?env= parameter to a loaded environment.
func loadEnvForRequest(r *http.Request) (Env, string, error) {
	envName := r.URL.Query().Get("env")
	if envName == "" {
		envName = selectedEnvironment
	}
	if envName == "" {
		return Env{}, "", errMissingEnv
	}
	e, err := loadEnv(envName)
	return e, envName, err
}

var errMissingEnv = &apiError{"missing required query parameter: env"}

type apiError struct{ msg string }

func (e *apiError) Error() string { return e.msg }

// getDNSStatus reports the delegation state of an environment's zone.
//
// GET /api/dns/status?env=dev
func getDNSStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	e, envName, err := loadEnvForRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	res, err := checkDNSPreflight(ctx, e)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "DNS check failed: "+err.Error())
		return
	}

	resp := dnsStatusResponse{
		Environment:       envName,
		Plan:              res.Plan.String(),
		Reason:            res.Reason,
		ZoneName:          res.ZoneName,
		ParentDomain:      res.ParentDomain,
		ZoneID:            res.ZoneID,
		ZoneNameservers:   res.ZoneNameservers,
		PublicNameservers: res.PublicNameservers,
		Delegated:         res.Plan == dnsPlanNormal,
		NeedsDelegation:   res.NeedsDelegation(),
		ZoneExists:        res.ZoneID != "",
		// A certificate can only validate once delegation resolves publicly.
		CertificateReady: res.Plan == dnsPlanNormal,
	}

	// Is the parent on Route53 at all? Cheap, and it tells the UI whether to
	// offer the one-click path or the manual instructions.
	if res.ParentDomain != "" {
		if parentNS, nsErr := queryNameservers(res.ParentDomain); nsErr == nil {
			resp.ParentIsRoute53 = looksLikeRoute53(parentNS)
			resp.CanAutoDelegate = resp.ParentIsRoute53 && res.NeedsDelegation() && len(res.ZoneNameservers) > 0
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// dnsCandidateResponse is one profile's answer to "do you hold the parent zone?".
type dnsCandidateResponse struct {
	Profile       string   `json:"profile"`
	AccountID     string   `json:"account_id,omitempty"`
	ZoneID        string   `json:"zone_id,omitempty"`
	Nameservers   []string `json:"nameservers,omitempty"`
	Authoritative bool     `json:"authoritative"`
	Error         string   `json:"error,omitempty"`
}

// getDNSParentCandidates lists local AWS profiles that hold the parent zone,
// flagging which one actually serves the domain.
//
// GET /api/dns/parent-candidates?env=dev
func getDNSParentCandidates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	e, _, err := loadEnvForRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()

	res, err := checkDNSPreflight(ctx, e)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "DNS check failed: "+err.Error())
		return
	}
	if res.ParentDomain == "" {
		writeJSON(w, http.StatusOK, []dnsCandidateResponse{})
		return
	}

	publicParentNS, err := queryNameservers(res.ParentDomain)
	if err != nil || len(publicParentNS) == 0 {
		writeAPIError(w, http.StatusBadGateway,
			"could not resolve nameservers for "+res.ParentDomain+", so no candidate can be verified")
		return
	}

	profiles, err := getLocalAWSProfiles()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "could not list AWS profiles: "+err.Error())
		return
	}

	out := []dnsCandidateResponse{}
	for _, c := range scanProfilesForParentZone(ctx, profiles, res.ParentDomain, publicParentNS) {
		// Only profiles that actually hold a zone are useful to the caller.
		if c.ZoneID == "" && c.Err == nil {
			continue
		}
		item := dnsCandidateResponse{
			Profile:       c.Profile,
			AccountID:     c.AccountID,
			ZoneID:        c.ZoneID,
			Nameservers:   c.Nameservers,
			Authoritative: c.Authoritative,
		}
		if c.Err != nil {
			item.Error = shortError(c.Err)
		}
		out = append(out, item)
	}

	writeJSON(w, http.StatusOK, out)
}

// dnsDelegateRequest is the body of POST /api/dns/delegate.
type dnsDelegateRequest struct {
	Env     string `json:"env"`
	Profile string `json:"profile"`
	// Confirm must be true. Writing to live DNS should never be the accidental
	// result of a navigation, prefetch or double-submit.
	Confirm bool `json:"confirm"`
}

type dnsDelegateResponse struct {
	Written     bool     `json:"written"`
	Verified    bool     `json:"verified"`
	ZoneName    string   `json:"zone_name"`
	ParentZone  string   `json:"parent_zone_id"`
	Nameservers []string `json:"nameservers"`
	Message     string   `json:"message"`
}

// postDNSDelegate writes the NS delegation record into the parent zone.
//
// POST /api/dns/delegate  {"env":"dev","profile":"mag","confirm":true}
func postDNSDelegate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req dnsDelegateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if !req.Confirm {
		writeAPIError(w, http.StatusBadRequest,
			"refusing to modify live DNS without \"confirm\": true")
		return
	}
	if req.Env == "" || req.Profile == "" {
		writeAPIError(w, http.StatusBadRequest, "both \"env\" and \"profile\" are required")
		return
	}

	e, err := loadEnv(req.Env)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "could not load environment: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Minute)
	defer cancel()

	res, err := checkDNSPreflight(ctx, e)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "DNS check failed: "+err.Error())
		return
	}
	if res.ZoneID == "" || len(res.ZoneNameservers) == 0 {
		writeAPIError(w, http.StatusConflict,
			"the hosted zone for "+res.ZoneName+" does not exist yet — deploy it first")
		return
	}
	if res.ParentDomain == "" {
		writeAPIError(w, http.StatusBadRequest,
			res.ZoneName+" is a root domain; delegate it at your registrar")
		return
	}

	publicParentNS, err := queryNameservers(res.ParentDomain)
	if err != nil || len(publicParentNS) == 0 {
		writeAPIError(w, http.StatusBadGateway,
			"could not resolve nameservers for "+res.ParentDomain)
		return
	}

	// Re-verify the chosen profile server-side. The client is not trusted to
	// have picked an authoritative zone — writing into a same-named zone in an
	// unrelated account would silently do nothing.
	var chosen parentZoneCandidate
	for _, c := range scanProfilesForParentZone(ctx, []string{req.Profile}, res.ParentDomain, publicParentNS) {
		chosen = c
	}
	if chosen.ZoneID == "" {
		writeAPIError(w, http.StatusBadRequest,
			"profile "+req.Profile+" has no hosted zone for "+res.ParentDomain)
		return
	}
	if !chosen.Authoritative {
		writeAPIError(w, http.StatusConflict,
			"the "+res.ParentDomain+" zone in profile "+req.Profile+
				" does not match public DNS, so writing to it would have no effect")
		return
	}

	err = applyDelegation(delegationRequest{
		ParentProfile: chosen.Profile,
		ParentZoneID:  chosen.ZoneID,
		Subdomain:     res.ZoneName,
		Nameservers:   res.ZoneNameservers,
	})
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, err.Error())
		return
	}

	resp := dnsDelegateResponse{
		Written:     true,
		ZoneName:    res.ZoneName,
		ParentZone:  chosen.ZoneID,
		Nameservers: res.ZoneNameservers,
	}

	// Give propagation a short window, but do not hold the request open for the
	// full five minutes the CLI allows.
	waitCtx, cancelWait := context.WithTimeout(ctx, 90*time.Second)
	defer cancelWait()
	if _, ok := waitForDelegation(waitCtx, res.ZoneName, res.ZoneNameservers, 10*time.Second); ok {
		resp.Verified = true
		resp.Message = res.ZoneName + " is now delegated and certificate validation can proceed."
		if err := recordDelegation(res.ParentDomain, res.ZoneName, e.AccountID, res.ZoneID, res.ZoneNameservers); err != nil {
			resp.Message += " (could not record it in " + DNSConfigFile + ": " + err.Error() + ")"
		}
	} else {
		resp.Message = "Record written. Resolvers have not picked it up yet — re-check status in a minute."
	}

	writeJSON(w, http.StatusOK, resp)
}

// registerDNSRoutes wires the DNS endpoints into the SPA server.
func registerDNSRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/dns/status", corsMiddleware(getDNSStatus))
	mux.HandleFunc("/api/dns/parent-candidates", corsMiddleware(getDNSParentCandidates))
	mux.HandleFunc("/api/dns/delegate", corsMiddleware(postDNSDelegate))
}
