package main

// Access rules for the web server. Every route, /api, /ws and the SPA, passes
// through webAccessGuard in mainRouter.
//
// Local mode (the default, listening on 127.0.0.1): a request is answered only
// when it arrived from a loopback address, its Host is localhost, 127.0.0.1 or
// [::1] on the port meroku serves, its Origin (when sent) is a loopback http
// origin and its Sec-Fetch-Site (when sent) is same-origin or none. RemoteAddr
// refuses other machines; the Host and Origin rules refuse pages from other
// sites and DNS-rebinding pages in a local browser. The web UI's dev server
// proxies from its own loopback port, which these rules accept.
//
// Remote mode (--listen on any other address): every request carries the
// per-run token, as the meroku_token cookie or a token query parameter; its
// Origin, when sent, is exactly http://<Host>; and Sec-Fetch-Site follows the
// rule above. Opening the printed URL sets the cookie and redirects to the
// same path without the token.
//
// No route sends CORS headers: the UI is served from the same origin.

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
)

const (
	webAccessCookieName = "meroku_token"
	webAccessQueryName  = "token"
	webDefaultListen    = "127.0.0.1"
)

// webAccessToken is the token a remote browser must present. Empty means the
// server is reachable from this machine only.
var webAccessToken string

// webListenHost is the host the web server listens on, without brackets.
var webListenHost = webDefaultListen

// configureWebAccess records the host the web server listens on and returns
// the access token a remote browser needs, or "" for a loopback host.
func configureWebAccess(listenHost string) string {
	host := strings.Trim(strings.TrimSpace(listenHost), "[]")
	if host == "" {
		host = webDefaultListen
	}
	webListenHost = host
	if isLoopbackHostName(host) {
		webAccessToken = ""
		return ""
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	webAccessToken = hex.EncodeToString(buf)
	return webAccessToken
}

// validateListenHost reports whether value names an interface for --listen:
// an IP address (IPv6 optionally in brackets) or a hostname, with no port.
func validateListenHost(value string) error {
	if _, port, err := net.SplitHostPort(value); err == nil && port != "" && strings.Trim(port, "0123456789") == "" {
		return errors.New("--listen takes an address without a port; use --web-port for the port")
	}
	host := value
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	if net.ParseIP(host) != nil || isHostName(host) {
		return nil
	}
	return fmt.Errorf("--listen: invalid address %s", value)
}

// isHostName reports whether s is a DNS hostname: dot-separated labels of
// letters, digits and hyphens, none starting or ending with a hyphen.
func isHostName(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(s, "."), ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

// webListenAddr is the address the web server listens on for port.
func webListenAddr(port string) string {
	return net.JoinHostPort(webListenHost, port)
}

// webRemoteMode reports whether requests need the access token.
func webRemoteMode() bool {
	return webAccessToken != ""
}

// webRequestAllowed reports whether r may reach any route of the web server.
// See the file comment for the rules.
func webRequestAllowed(r *http.Request) bool {
	if !webRemoteMode() {
		return localRequestAllowed(r)
	}
	if !secFetchSiteAllowed(r) {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
		return false
	}
	if c, err := r.Cookie(webAccessCookieName); err == nil && webTokenMatches(c.Value) {
		return true
	}
	return webTokenMatches(r.URL.Query().Get(webAccessQueryName))
}

// localRequestAllowed reports whether r was sent to meroku on this machine by
// a page meroku serves.
func localRequestAllowed(r *http.Request) bool {
	remote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	if ip := net.ParseIP(remote); ip == nil || !ip.IsLoopback() {
		return false
	}
	if !secFetchSiteAllowed(r) {
		return false
	}
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil || !isLoopbackHostName(host) {
		return false
	}
	// The port the request reached, when the server recorded it. A Host on
	// another port names some other server.
	if addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		if _, localPort, err := net.SplitHostPort(addr.String()); err == nil && localPort != port {
			return false
		}
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	// The web UI's dev server proxies /api from its own loopback port, so an
	// Origin is checked for being loopback, not for matching Host's port.
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" {
		return false
	}
	return isLoopbackHostName(u.Hostname())
}

// secFetchSiteAllowed refuses requests a browser marks as sent by another
// site. Clients that send no Sec-Fetch-Site header pass.
func secFetchSiteAllowed(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
		return true
	}
	return false
}

// isLoopbackHostName reports whether host (no port, no brackets) is a name
// that always means this machine.
func isLoopbackHostName(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// webTokenMatches compares candidate with the access token in constant time.
func webTokenMatches(candidate string) bool {
	if webAccessToken == "" || candidate == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(webAccessToken)) == 1
}

// webAccessGuard answers 403 to any request webRequestAllowed refuses. In
// remote mode a page navigation (a GET outside /api/ and /ws/ that is not a
// WebSocket upgrade) carrying the token in its query gets the token as a
// cookie and a redirect to the same path without it. Other requests with the
// token query go straight to the handler.
func webAccessGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !webRequestAllowed(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		cleanPath := path.Clean("/" + strings.TrimLeft(r.URL.Path, "/"))
		if webRemoteMode() && r.Method == http.MethodGet && isPageNavigation(r, cleanPath) {
			q := r.URL.Query()
			if webTokenMatches(q.Get(webAccessQueryName)) {
				http.SetCookie(w, &http.Cookie{
					Name:     webAccessCookieName,
					Value:    webAccessToken,
					Path:     "/",
					HttpOnly: true,
					SameSite: http.SameSiteStrictMode,
				})
				q.Del(webAccessQueryName)
				target := url.URL{Path: cleanPath, RawQuery: q.Encode()}
				http.Redirect(w, r, target.String(), http.StatusSeeOther)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// isPageNavigation reports whether r, whose cleaned path is cleanPath, is a
// browser loading a page rather than an API call or a WebSocket upgrade.
func isPageNavigation(r *http.Request, cleanPath string) bool {
	if strings.HasPrefix(cleanPath, "/api/") || strings.HasPrefix(cleanPath, "/ws/") {
		return false
	}
	return !isWebSocketUpgrade(r)
}

// isWebSocketUpgrade reports whether r asks to switch to the WebSocket
// protocol: a Connection header with the "upgrade" token and Upgrade:
// websocket, both case-insensitive.
func isWebSocketUpgrade(r *http.Request) bool {
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return false
	}
	for _, v := range r.Header.Values("Connection") {
		for _, token := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
				return true
			}
		}
	}
	return false
}

// webServerURLs returns the URL to show the user and the URL to open in a
// browser on this machine. In remote mode both carry the token.
func webServerURLs(port string) (shown, open string) {
	if !webRemoteMode() {
		u := "http://localhost:" + port
		return u, u
	}
	suffix := "/?" + webAccessQueryName + "=" + webAccessToken
	host := webListenHost
	openHost := webListenHost
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		host = webMachineAddress()
		openHost = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port) + suffix,
		"http://" + net.JoinHostPort(openHost, port) + suffix
}

// webURLWithoutToken is u with the token query removed, for display.
func webURLWithoutToken(u string) string {
	base, _, _ := strings.Cut(u, "/?"+webAccessQueryName+"=")
	return base
}

// webMachineAddress is an address other machines can likely reach this one
// on: the first non-loopback IPv4 address of an interface, else the hostname.
func webMachineAddress() string {
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipNet, ok := a.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	if name, err := os.Hostname(); err == nil && name != "" {
		return name
	}
	return "localhost"
}

// printWebAccessURL prints the address to open in remote mode, the one place
// the token is shown. In local mode it prints nothing.
func printWebAccessURL(shown string) {
	if !webRemoteMode() {
		return
	}
	fmt.Printf("\n🌐 Web app: %s\n", shown)
	fmt.Printf("   The token in this URL is required; anyone with the URL gets access.\n\n")
}
