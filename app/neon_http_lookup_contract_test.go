package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// REQ-24 / TEST-21.
func TestNeonContractLookupRequiresLoopbackPeerHostPortAndSafeOrigin(t *testing.T) {
	old := neonLookupDeps
	t.Cleanup(func() { neonLookupDeps = old })
	env := qaNeonEnv(t)
	// REQ-24 / TEST-21: environment files are discovered in the project root.
	// LoadEnv supplies the typed settings; it does not create an env on disk.
	root := t.TempDir()
	qaNeonWrite(t, filepath.Join(root, "dev.yaml"), `schema_version: 32
project: myapp
env: dev
neon:
  mode: reuse
  project_id: fake-project-000000
  pg_version: 17
  owner_role: neondb_owner
  databases:
    - name: app
      create: false
workload:
  backend_neon:
    database: app
    access: readwrite
`)
	t.Chdir(root)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	neonLookupDeps = func() neonLookupDependencies {
		return neonLookupDependencies{
			Auth:   qaNeonAuthDeps(now, "fake-lookup-token-do-not-use", ""),
			Tokens: newNeonWebTokenCache(),
			NewAPI: func(string) neonAPI { return qaNeonAPIReady() },
			LoadEnv: func(name string) (Env, error) {
				if name != "dev" {
					return Env{}, errors.New("unknown environment")
				}
				return env, nil
			},
		}
	}
	for _, tc := range []struct {
		name, peer, host, origin, fetch, method, path string
		status                                        int
	}{
		{"local", "127.0.0.1:54000", "localhost:8080", "http://localhost:8080", "same-origin", "GET", "?env=dev", 200},
		{"direct no origin", "127.0.0.1:54000", "127.0.0.1:8080", "", "none", "GET", "?env=dev", 200},
		{"IPv6 loopback", "[::1]:54000", "[::1]:8080", "http://[::1]:8080", "same-origin", "GET", "?env=dev", 200},
		{"remote spoofing localhost", "192.0.2.10:54000", "localhost:8080", "http://localhost:8080", "same-origin", "GET", "?env=dev", 403},
		{"bad peer", "not-an-address", "localhost:8080", "", "", "GET", "?env=dev", 403},
		{"DNS rebinding", "127.0.0.1:54000", "attacker.example:8080", "", "", "GET", "?env=dev", 403},
		{"wrong listener port", "127.0.0.1:54000", "localhost:9999", "", "", "GET", "?env=dev", 403},
		{"foreign origin", "127.0.0.1:54000", "localhost:8080", "http://attacker.example", "same-origin", "GET", "?env=dev", 403},
		{"https origin", "127.0.0.1:54000", "localhost:8080", "https://localhost:8080", "same-origin", "GET", "?env=dev", 403},
		{"null origin", "127.0.0.1:54000", "localhost:8080", "null", "", "GET", "?env=dev", 403},
		{"cross site", "127.0.0.1:54000", "localhost:8080", "http://localhost:8080", "cross-site", "GET", "?env=dev", 403},
		{"same site not same origin", "127.0.0.1:54000", "localhost:8080", "http://localhost:8080", "same-site", "GET", "?env=dev", 403},
		{"missing env", "127.0.0.1:54000", "localhost:8080", "", "", "GET", "", 400},
		{"unknown env", "127.0.0.1:54000", "localhost:8080", "", "", "GET", "?env=missing", 400},
		{"method refused", "127.0.0.1:54000", "localhost:8080", "", "", "POST", "?env=dev", 405},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "http://localhost:8080/api/neon/lookup"+tc.path, nil)
			req.Host = tc.host
			req.RemoteAddr = tc.peer
			req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}))
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.fetch != "" {
				req.Header.Set("Sec-Fetch-Site", tc.fetch)
			}
			w := httptest.NewRecorder()
			getNeonLookup(w, req)
			if w.Code != tc.status {
				t.Fatalf("status=%d want %d: %s", w.Code, tc.status, w.Body.String())
			}
			if w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Error("lookup must not enable CORS")
			}
			body := w.Body.String()
			for _, s := range []string{"fake-lookup-token-do-not-use", "fake-owner-password-do-not-use", "access_token", "refresh_token", "connection_uri", "database_password"} {
				if strings.Contains(body, s) {
					t.Errorf("response leaks secret %q", s)
				}
			}
			if tc.status == 200 {
				var got neonLookupResponse
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got.Auth.State != "ok" || got.Auth.Source != "env" || got.Project == nil || got.Project.ID != "fake-project-000000" {
					t.Fatalf("response=%s", body)
				}
				if len(got.Databases) != 1 || got.Databases[0].Name != "app" {
					t.Fatalf("database choices=%+v", got.Databases)
				}
			}
		})
	}
}

func TestNeonContractLookupWithoutAuthReturnsSafeBanner(t *testing.T) {
	// REQ-24 / TEST-21.
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d := qaNeonAuthDeps(now, "", "")
	deps := neonLookupDependencies{Auth: d, Tokens: newNeonWebTokenCache(), NewAPI: func(string) neonAPI { return qaNeonAPIReady() }}
	got := lookupNeon(context.Background(), deps, qaNeonEnv(t), neonLookupRequest{})
	if got.Auth.State != "no_credentials" {
		t.Fatalf("auth=%+v", got.Auth)
	}
	if got.Project != nil || len(got.Databases) != 0 {
		t.Fatal("unauthenticated lookup exposed live data")
	}
}

// REQ-25 / TEST-22: the client sees only this local HTTP server.
func TestNeonContractHTTPClientReadsProjectAndBothBranchFormats(t *testing.T) {
	for _, field := range []string{"primary", "default"} {
		t.Run(field, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("client attempted a write: %s", r.Method)
					w.WriteHeader(405)
					return
				}
				if r.Header.Get("Authorization") != "Bearer fake-http-token-do-not-use" {
					t.Error("missing API auth header")
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/projects/fake-project-000000":
					fmt.Fprint(w, `{"project":{"id":"fake-project-000000","name":"fake-project","org_id":"org-fake-000000","region_id":"aws-us-east-1","pg_version":17}}`)
				case "/projects/fake-project-000000/branches":
					fmt.Fprintf(w, `{"branches":[{"id":"br-old","name":"old","%s":false},{"id":"br-main","name":"main","%s":true}]}`, field, field)
				default:
					w.WriteHeader(404)
					fmt.Fprint(w, `{"message":"not found"}`)
				}
			}))
			defer server.Close()
			api := newNeonHTTPClient(server.URL, "fake-http-token-do-not-use", server.Client())
			p, err := api.GetProject(context.Background(), "fake-project-000000")
			if err != nil {
				t.Fatal(err)
			}
			if p.ID != "fake-project-000000" || p.PGVersion != 17 {
				t.Fatalf("decoded project=%+v", p)
			}
			bs, err := api.ListBranches(context.Background(), p.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(bs) != 2 || bs[0].Default || !bs[1].Default || bs[1].ID != "br-main" {
				t.Fatalf("decoded branches=%+v", bs)
			}
			_, err = api.GetProject(context.Background(), "missing")
			if !errors.Is(err, ErrNeonNotFound) {
				t.Fatalf("404 error=%v", err)
			}
		})
	}
}

func TestNeonContractHTTPClientRetriesBoundedlyAndRedactsErrors(t *testing.T) {
	// REQ-25 / TEST-22.
	for _, code := range []int{401, 429, 500} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			attempts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts++
				w.WriteHeader(code)
				fmt.Fprint(w, `{"message":"fake-http-retry-token-do-not-use failure"}`)
			}))
			defer server.Close()
			api := newNeonHTTPClient(server.URL, "fake-http-retry-token-do-not-use", server.Client())
			api.sleep = func(context.Context, time.Duration) error { return nil }
			_, err := api.GetProject(context.Background(), "fake-project-000000")
			if err == nil {
				t.Fatal("failed request accepted")
			}
			want := 3
			if code == 401 {
				want = 1
			}
			// Boundaries are observable HTTP attempts, not private helper/mock invocations.
			if attempts != want {
				t.Fatalf("HTTP attempts=%d want %d", attempts, want)
			}
			if strings.Contains(err.Error(), "fake-http-retry-token-do-not-use") {
				t.Fatal("API error leaked auth token")
			}
		})
	}
}

func TestNeonContractRevealPasswordRegistersRedactionAndCancellationStopsRequest(t *testing.T) {
	// REQ-3, REQ-25 / TEST-22.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/reveal_password") {
			w.WriteHeader(405)
			return
		}
		fmt.Fprint(w, `{"password":"fake-revealed-password-do-not-use"}`)
	}))
	defer server.Close()
	api := newNeonHTTPClient(server.URL, "fake-http-token-do-not-use", server.Client())
	p, err := api.RevealRolePassword(context.Background(), "fake-project-000000", "br-main", "neondb_owner")
	if err != nil {
		t.Fatal(err)
	}
	if p != "fake-revealed-password-do-not-use" {
		t.Fatal("wrong revealed password")
	}
	if strings.Contains(redactSecrets("password "+p), p) {
		t.Fatal("revealed password not registered for redaction")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = api.GetProject(ctx, "fake-project-000000")
	if err == nil {
		t.Fatal("cancelled request succeeded")
	}
}
