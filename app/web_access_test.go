package main

// REGRESSION: the local web server answered other origins, other hosts and foreign WebSocket upgrades.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

const (
	webAccessLoopbackRemote = "127.0.0.1:54000"
	webAccessLoopbackHost   = "127.0.0.1:8080"
	webAccessLANRemote      = "192.168.1.50:5000"
	webAccessLANHost        = "192.168.1.10:8080"
	webAccessForeignOrigin  = "https://evil.example"
)

// webAccessSetup runs the test in an empty project directory, so
// /api/environments answers 200 with no AWS or network access, and puts the
// server back in local mode afterwards.
func webAccessSetup(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	configureWebAccess("127.0.0.1")
	t.Cleanup(func() { configureWebAccess("127.0.0.1") })
}

func webAccessRequest(method, target, remote, host string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.RemoteAddr = remote
	req.Host = host
	return req
}

func webAccessServe(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mainRouter().ServeHTTP(rec, req)
	return rec
}

func expectWebAccessStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d (body %q)", rec.Code, want, strings.TrimSpace(rec.Body.String()))
	}
}

func expectNoCORSHeaders(t *testing.T, h http.Header) {
	t.Helper()
	for _, name := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Methods",
		"Access-Control-Allow-Credentials",
	} {
		if v := h.Get(name); v != "" {
			t.Errorf("%s = %q, want no such header", name, v)
		}
	}
}

// ---------------------------------------------------------------------------
// Local mode
// ---------------------------------------------------------------------------

func TestWebAccess_LocalRefusesForeignOrigin(t *testing.T) {
	webAccessSetup(t)
	req := webAccessRequest(http.MethodGet, "/api/environments", webAccessLoopbackRemote, webAccessLoopbackHost)
	req.Header.Set("Origin", webAccessForeignOrigin)

	rec := webAccessServe(req)

	expectNoCORSHeaders(t, rec.Header())
	expectWebAccessStatus(t, rec, http.StatusForbidden)
}

func TestWebAccess_LocalPreflightFromForeignOriginGetsNoCORS(t *testing.T) {
	webAccessSetup(t)
	req := webAccessRequest(http.MethodOptions, "/api/ssm/parameter", webAccessLoopbackRemote, webAccessLoopbackHost)
	req.Header.Set("Origin", webAccessForeignOrigin)
	req.Header.Set("Access-Control-Request-Method", http.MethodPut)
	req.Header.Set("Access-Control-Request-Headers", "content-type")

	rec := webAccessServe(req)

	expectNoCORSHeaders(t, rec.Header())
	if rec.Code == http.StatusNoContent {
		t.Fatalf("status = 204, a preflight from %s must not succeed", webAccessForeignOrigin)
	}
}

func TestWebAccess_LocalRefusesNonLoopbackHost(t *testing.T) {
	webAccessSetup(t)
	req := webAccessRequest(http.MethodGet, "/api/environments", webAccessLoopbackRemote, "evil.example:8080")

	expectWebAccessStatus(t, webAccessServe(req), http.StatusForbidden)
}

func TestWebAccess_LocalRefusesLANRemoteAddr(t *testing.T) {
	webAccessSetup(t)
	req := webAccessRequest(http.MethodGet, "/api/environments", webAccessLANRemote, webAccessLoopbackHost)

	expectWebAccessStatus(t, webAccessServe(req), http.StatusForbidden)
}

func TestWebAccess_LocalRefusesCrossSiteFetch(t *testing.T) {
	webAccessSetup(t)
	req := webAccessRequest(http.MethodGet, "/api/environments", webAccessLoopbackRemote, webAccessLoopbackHost)
	req.Header.Set("Sec-Fetch-Site", "cross-site")

	expectWebAccessStatus(t, webAccessServe(req), http.StatusForbidden)
}

func TestWebAccess_LocalGuardsSPARoot(t *testing.T) {
	webAccessSetup(t)
	req := webAccessRequest(http.MethodGet, "/", webAccessLoopbackRemote, webAccessLoopbackHost)
	req.Header.Set("Origin", webAccessForeignOrigin)

	expectWebAccessStatus(t, webAccessServe(req), http.StatusForbidden)
}

// dialWebSocket opens a WebSocket to path on server and returns the handshake
// response status. A 101 means the upgrade went through.
func dialWebSocket(t *testing.T, server *httptest.Server, path string, header http.Header) int {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + path
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if conn != nil {
		conn.Close()
	}
	if resp == nil {
		t.Fatalf("dial %s: no handshake response: %v", path, err)
	}
	return resp.StatusCode
}

func TestWebAccess_LocalRefusesForeignWebSocketUpgrade(t *testing.T) {
	webAccessSetup(t)
	server := httptest.NewServer(mainRouter())
	t.Cleanup(server.Close)

	paths := []string{
		"/ws/logs?env=missing&service=backend",
		"/ws/ssh-pty?env=missing&service=backend&taskArn=arn:aws:ecs:us-east-1:000000000000:task/fake",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			header := http.Header{"Origin": []string{webAccessForeignOrigin}}
			status := dialWebSocket(t, server, path, header)
			if status == http.StatusSwitchingProtocols {
				t.Fatalf("status = 101 Switching Protocols, want 403 for Origin %s", webAccessForeignOrigin)
			}
			if status != http.StatusForbidden {
				t.Fatalf("status = %d, want 403 for Origin %s", status, webAccessForeignOrigin)
			}
		})
	}
}

func TestWebAccess_LocalAllowsLoopbackClients(t *testing.T) {
	webAccessSetup(t)
	server := httptest.NewServer(mainRouter())
	t.Cleanup(server.Close)

	cases := map[string]string{
		"no origin":        "",
		"vite dev origin":  "http://localhost:5173",
		"same-origin page": server.URL,
	}
	for name, origin := range cases {
		t.Run(name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, server.URL+"/api/environments", nil)
			if err != nil {
				t.Fatal(err)
			}
			if origin != "" {
				req.Header.Set("Origin", origin)
			}
			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200 for a loopback client (Origin %q)", resp.StatusCode, origin)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Remote mode (--listen on a non-loopback host)
// ---------------------------------------------------------------------------

const webAccessCookie = "meroku_token"

func webAccessRemoteSetup(t *testing.T) string {
	t.Helper()
	webAccessSetup(t)
	token := configureWebAccess("192.168.1.10")
	if token == "" {
		t.Fatal("configureWebAccess(192.168.1.10) returned no token, want a random token for a non-loopback host")
	}
	return token
}

func TestWebAccess_ConfigureReturnsNoTokenForLoopback(t *testing.T) {
	webAccessSetup(t)
	for _, host := range []string{"127.0.0.1", "localhost", "::1"} {
		if token := configureWebAccess(host); token != "" {
			t.Errorf("configureWebAccess(%q) = %q, want no token", host, token)
		}
	}
	a := configureWebAccess("0.0.0.0")
	b := configureWebAccess("0.0.0.0")
	if a == "" || a == b {
		t.Errorf("configureWebAccess(0.0.0.0) tokens %q, %q: want a fresh random token each time", a, b)
	}
}

func TestWebAccess_RemoteRefusesRequestWithoutToken(t *testing.T) {
	webAccessRemoteSetup(t)
	req := webAccessRequest(http.MethodGet, "/api/environments", webAccessLANRemote, webAccessLANHost)

	expectWebAccessStatus(t, webAccessServe(req), http.StatusForbidden)
}

func TestWebAccess_RemoteAcceptsTokenCookie(t *testing.T) {
	token := webAccessRemoteSetup(t)
	req := webAccessRequest(http.MethodGet, "/api/environments", webAccessLANRemote, webAccessLANHost)
	req.AddCookie(&http.Cookie{Name: webAccessCookie, Value: token})

	expectWebAccessStatus(t, webAccessServe(req), http.StatusOK)
}

func TestWebAccess_RemoteTokenQuerySetsCookieAndRedirects(t *testing.T) {
	token := webAccessRemoteSetup(t)
	req := webAccessRequest(http.MethodGet, "/?token="+url.QueryEscape(token), webAccessLANRemote, webAccessLANHost)

	rec := webAccessServe(req)

	if rec.Code != http.StatusFound && rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 302 or 303 redirect", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != "/" {
		t.Errorf("Location = %q, want %q", loc, "/")
	}
	if strings.Contains(loc, token) {
		t.Errorf("Location %q carries the token", loc)
	}

	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == webAccessCookie {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatalf("no %s cookie set; Set-Cookie = %q", webAccessCookie, rec.Header().Values("Set-Cookie"))
	}
	if cookie.Value != token {
		t.Errorf("cookie value = %q, want the token", cookie.Value)
	}
	if !cookie.HttpOnly {
		t.Error("cookie is not HttpOnly")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie SameSite = %v, want Strict", cookie.SameSite)
	}
}

func TestWebAccess_RemoteRefusesWrongToken(t *testing.T) {
	webAccessRemoteSetup(t)
	req := webAccessRequest(http.MethodGet, "/api/environments", webAccessLANRemote, webAccessLANHost)
	req.AddCookie(&http.Cookie{Name: webAccessCookie, Value: "fake-token-do-not-use"})

	expectWebAccessStatus(t, webAccessServe(req), http.StatusForbidden)
}

func TestWebAccess_RemoteRefusesCrossOriginWithRightCookie(t *testing.T) {
	token := webAccessRemoteSetup(t)
	for _, origin := range []string{webAccessForeignOrigin, "http://192.168.1.10:9999", "https://" + webAccessLANHost} {
		t.Run(origin, func(t *testing.T) {
			req := webAccessRequest(http.MethodGet, "/api/environments", webAccessLANRemote, webAccessLANHost)
			req.AddCookie(&http.Cookie{Name: webAccessCookie, Value: token})
			req.Header.Set("Origin", origin)

			expectWebAccessStatus(t, webAccessServe(req), http.StatusForbidden)
		})
	}
}

func TestWebAccess_RemoteWebSocketNeedsToken(t *testing.T) {
	token := webAccessRemoteSetup(t)
	server := httptest.NewServer(mainRouter())
	t.Cleanup(server.Close)
	path := "/ws/logs?env=missing&service=backend"

	t.Run("with cookie", func(t *testing.T) {
		header := http.Header{
			"Origin": []string{server.URL},
			"Cookie": []string{webAccessCookie + "=" + token},
		}
		if status := dialWebSocket(t, server, path, header); status == http.StatusForbidden {
			t.Fatalf("status = 403, want the upgrade to pass the guard with the right cookie")
		}
	})
	t.Run("without cookie", func(t *testing.T) {
		header := http.Header{"Origin": []string{server.URL}}
		if status := dialWebSocket(t, server, path, header); status != http.StatusForbidden {
			t.Fatalf("status = %d, want 403 without the token", status)
		}
	})
}

// ---------------------------------------------------------------------------
// Bind address
// ---------------------------------------------------------------------------

func TestWebAccess_ListenAddrIsLoopbackByDefault(t *testing.T) {
	webAccessSetup(t)
	if got, want := webListenAddr("8080"), "127.0.0.1:8080"; got != want {
		t.Fatalf("webListenAddr(8080) = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Token query on non-page requests
// ---------------------------------------------------------------------------

func TestWebAccess_RemoteTokenQueryWebSocketIsNotRedirected(t *testing.T) {
	token := webAccessRemoteSetup(t)
	server := httptest.NewServer(mainRouter())
	t.Cleanup(server.Close)
	path := "/ws/logs?env=missing&service=backend&token=" + url.QueryEscape(token)

	header := http.Header{"Origin": []string{server.URL}}
	status := dialWebSocket(t, server, path, header)
	if status == http.StatusSeeOther || status == http.StatusForbidden {
		t.Fatalf("status = %d, want the upgrade to reach the handler with the token query", status)
	}
}

func TestWebAccess_RemoteTokenQueryAPICallIsNotRedirected(t *testing.T) {
	token := webAccessRemoteSetup(t)
	req := webAccessRequest(http.MethodGet, "/api/environments?token="+url.QueryEscape(token), webAccessLANRemote, webAccessLANHost)

	rec := webAccessServe(req)

	if loc := rec.Header().Get("Location"); loc != "" {
		t.Errorf("Location = %q, want no redirect", loc)
	}
	expectWebAccessStatus(t, rec, http.StatusOK)
}

func TestWebAccess_RemoteTokenRedirectStaysOnServer(t *testing.T) {
	token := webAccessRemoteSetup(t)
	req := webAccessRequest(http.MethodGet, "/", webAccessLANRemote, webAccessLANHost)
	req.URL.Path = "//evil.example/x"
	req.URL.RawQuery = "token=" + url.QueryEscape(token)

	rec := webAccessServe(req)

	if got, want := rec.Header().Get("Location"), "/evil.example/x"; got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// --listen validation
// ---------------------------------------------------------------------------

func TestWebAccess_ValidateListenHost(t *testing.T) {
	accept := []string{"127.0.0.1", "localhost", "::1", "[::1]", "0.0.0.0", "::", "192.168.1.10", "my-box.local"}
	for _, v := range accept {
		if err := validateListenHost(v); err != nil {
			t.Errorf("validateListenHost(%q) = %v, want nil", v, err)
		}
	}
	const portMsg = "--listen takes an address without a port; use --web-port for the port"
	reject := map[string]string{
		"0.0.0.0:8080": portMsg,
		"[::1]:80":     portMsg,
		"localhost:80": portMsg,
		"a b":          "--listen: invalid address a b",
		"":             "--listen: invalid address ",
		"-bad.example": "--listen: invalid address -bad.example",
		"http://x":     "--listen: invalid address http://x",
	}
	for v, want := range reject {
		err := validateListenHost(v)
		if err == nil {
			t.Errorf("validateListenHost(%q) = nil, want an error", v)
			continue
		}
		if err.Error() != want {
			t.Errorf("validateListenHost(%q) = %q, want %q", v, err.Error(), want)
		}
	}
}
