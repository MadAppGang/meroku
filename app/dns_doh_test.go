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
	if len(got.NS) != 2 {
		t.Fatalf("expected only the NS answers, got %v", got.NS)
	}
	// Trailing dots are stripped so the result compares against zone data.
	for _, ns := range got.NS {
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
	if len(got.NS) != 0 {
		t.Errorf("expected no nameservers, got %v", got.NS)
	}
}

// The rcode has to survive the parse. Reducing a response to its nameserver list
// threw away the one field that separates "nothing cached here yet" from "this
// resolver holds a delegation whose servers all refuse", which are minutes and
// days apart.
func TestQueryNameserversDoH_CarriesTheStatus(t *testing.T) {
	_, r := dohServer(t, 200, `{"Status":2,"Answer":[]}`)

	got, err := queryNameserversDoH(&http.Client{Timeout: 5 * time.Second}, r, "dev.example.com")
	if err != nil {
		t.Fatalf("SERVFAIL is an answer, not a transport failure: %v", err)
	}
	if got.Status != rcodeServFail {
		t.Errorf("expected the rcode to be carried out, got %d", got.Status)
	}
}

// The three verdicts, and why each answer maps where it does.
func TestClassifyNSAnswer(t *testing.T) {
	expected := []string{"ns-1036.awsdns-01.org", "ns-360.awsdns-45.com"}

	cases := []struct {
		name   string
		answer dohAnswer
		want   dohVerdict
		why    string
	}{
		{
			name:   "our nameservers",
			answer: dohAnswer{Status: rcodeNoError, NS: expected},
			want:   dohResolved,
			why:    "the delegation we wrote is what this resolver sees",
		},
		{
			name:   "order does not matter",
			answer: dohAnswer{Status: rcodeNoError, NS: []string{expected[1], expected[0]}},
			want:   dohResolved,
			why:    "an RRset is a set; resolvers may return it in any order",
		},
		{
			name: "somebody else's nameservers",
			answer: dohAnswer{Status: rcodeNoError,
				NS: []string{"ns-1222.awsdns-24.org", "ns-704.awsdns-24.net"}},
			want: dohStale,
			why:  "it is looking at a delegation, just not ours — waiting will not fix that",
		},
		{
			name:   "servfail",
			answer: dohAnswer{Status: rcodeServFail},
			want:   dohStale,
			why:    "it holds a delegation whose nameservers all refused for the zone",
		},
		{
			name:   "nxdomain",
			answer: dohAnswer{Status: 3},
			want:   dohNotYet,
			why:    "nothing cached, so the next check may well find the delegation",
		},
		{
			name:   "noerror with no answer",
			answer: dohAnswer{Status: rcodeNoError},
			want:   dohNotYet,
			why:    "NODATA is the same as NXDOMAIN here — no delegation seen yet",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyNSAnswer(c.answer, expected); got != c.want {
				t.Errorf("got %v, want %v — %s", got, c.want, c.why)
			}
		})
	}
}

// A transport failure is not a DNS answer. We learned nothing about what that
// resolver holds, so it must not be reported as stale — a red badge would be
// claiming knowledge we do not have.
func TestCheckPropagationDoH_UnreachableResolverIsNotStale(t *testing.T) {
	if got := classifyNSAnswer(dohAnswer{}, []string{"ns-1036.awsdns-01.org"}); got != dohNotYet {
		t.Errorf("a zero answer should read as not-yet, got %v", got)
	}
}

func TestCountVerdicts(t *testing.T) {
	resolved, stale := countVerdicts(map[string]dohVerdict{
		"Google":     dohStale,
		"Cloudflare": dohStale,
		"AdGuard":    dohResolved,
		"NextDNS":    dohResolved,
		"Unasked":    dohNotYet,
	})
	if resolved != 2 || stale != 2 {
		t.Errorf("got %d resolved and %d stale, want 2 and 2", resolved, stale)
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
