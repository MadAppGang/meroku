package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func dohServer(t *testing.T, status int, body string) (*httptest.Server, dohResolver) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Every one of these providers needs the JSON accept header; without it
		// some return wire format, which parses as garbage rather than erroring.
		if r.Header.Get("Accept") != "application/dns-json" {
			t.Errorf("missing JSON accept header, got %q", r.Header.Get("Accept"))
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, dohResolver{Name: "test", URL: srv.URL + "?name=%s&type=NS"}
}

func TestQueryNameserversDoH_ParsesNSAnswers(t *testing.T) {
	_, r := dohServer(t, 200, `{"Status":0,"Answer":[
		{"type":2,"data":"ns-745.awsdns-29.net."},
		{"type":2,"data":"ns-1310.awsdns-35.org."},
		{"type":1,"data":"192.0.2.1"}
	]}`)

	got, err := queryNameserversDoH(&http.Client{Timeout: 5 * time.Second}, r, "dev.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected only the NS answers, got %v", got)
	}
	// Trailing dots are stripped so the result compares against zone data.
	for _, ns := range got {
		if strings.HasSuffix(ns, ".") {
			t.Errorf("trailing dot not stripped: %q", ns)
		}
	}
}

// NXDOMAIN and "delegated but not visible here yet" are the same answer for our
// purposes, and neither is an error worth surfacing.
func TestQueryNameserversDoH_NoAnswersIsNotAnError(t *testing.T) {
	_, r := dohServer(t, 200, `{"Status":3,"Answer":[]}`)

	got, err := queryNameserversDoH(&http.Client{Timeout: 5 * time.Second}, r, "dev.example.com")
	if err != nil {
		t.Errorf("an empty answer should not be an error, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no nameservers, got %v", got)
	}
}

func TestQueryNameserversDoH_RejectsBadResponses(t *testing.T) {
	t.Run("http error", func(t *testing.T) {
		_, r := dohServer(t, 502, `nope`)
		if _, err := queryNameserversDoH(&http.Client{Timeout: 5 * time.Second}, r, "x.com"); err == nil {
			t.Error("expected an error for a non-200")
		}
	})
	t.Run("wire format instead of json", func(t *testing.T) {
		_, r := dohServer(t, 200, "\x00\x01\x81\x80binary")
		if _, err := queryNameserversDoH(&http.Client{Timeout: 5 * time.Second}, r, "x.com"); err == nil {
			t.Error("expected an error for unparseable body")
		}
	})
}

// The resolver set must be independent operators, and named by provider rather
// than IP — with DoH there is no single address being queried, and showing
// 8.8.8.8 would imply a UDP query we are specifically not making.
func TestDohResolvers_AreNamedAndDistinct(t *testing.T) {
	names := dohResolverNames()
	if len(names) < 3 {
		t.Fatalf("expected several independent resolvers, got %v", names)
	}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Errorf("duplicate resolver %q", n)
		}
		seen[n] = true
		if strings.Count(n, ".") == 3 {
			t.Errorf("%q looks like an IP; DoH queries a host, not an address", n)
		}
	}
	for _, r := range dohResolvers {
		if !strings.HasPrefix(r.URL, "https://") {
			t.Errorf("%s must be queried over HTTPS, got %q", r.Name, r.URL)
		}
		if !strings.Contains(r.URL, "%s") {
			t.Errorf("%s URL has no place for the query name", r.Name)
		}
	}
}
