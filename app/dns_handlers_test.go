package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The DNS endpoints must actually be registered. They were absent entirely
// before, which is why the web UI displayed hand-written guesses at DNS records.
func TestDNSRoutes_AreRegistered(t *testing.T) {
	mux := http.NewServeMux()
	registerDNSRoutes(mux)

	for _, path := range []string{"/api/dns/status", "/api/dns/parent-candidates", "/api/dns/delegate"} {
		_, pattern := mux.Handler(httptest.NewRequest(http.MethodGet, path, nil))
		if pattern == "" {
			t.Errorf("%s is not registered", path)
		}
	}
}

// Writing to live DNS must require an explicit confirm, so it can never be the
// result of a stray navigation, prefetch or double-submit.
func TestPostDNSDelegate_RequiresConfirm(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/dns/delegate",
		strings.NewReader(`{"env":"dev","profile":"mag"}`))

	postDNSDelegate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 without confirm, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "confirm") {
		t.Errorf("error should mention the confirm flag, got %s", rec.Body.String())
	}
}

// GET must not reach the write path at all.
func TestPostDNSDelegate_RejectsGET(t *testing.T) {
	rec := httptest.NewRecorder()
	postDNSDelegate(rec, httptest.NewRequest(http.MethodGet, "/api/dns/delegate", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET, got %d", rec.Code)
	}
}

// Missing env/profile must be rejected before any AWS call.
func TestPostDNSDelegate_RequiresEnvAndProfile(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/dns/delegate",
		strings.NewReader(`{"confirm":true}`))

	postDNSDelegate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 without env/profile, got %d: %s", rec.Code, rec.Body.String())
	}
}

// Read endpoints reject non-GET.
func TestDNSReadEndpoints_RejectPOST(t *testing.T) {
	for name, h := range map[string]http.HandlerFunc{
		"status":            getDNSStatus,
		"parent-candidates": getDNSParentCandidates,
	} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodPost, "/api/dns/"+name, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: expected 405 for POST, got %d", name, rec.Code)
		}
	}
}
