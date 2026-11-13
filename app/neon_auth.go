package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// NeonTokenSource says where a Neon API token came from.
type NeonTokenSource string

const (
	// neonTokenFromEnv is NEON_API_KEY, captured from meroku's own environment
	// at startup. Its expiry is unknown.
	neonTokenFromEnv NeonTokenSource = "env"
	// neonTokenFromNeonctl is the access token of the local Neon CLI login
	// (`neon`, formerly `neonctl`). The value is kept for the web API.
	neonTokenFromNeonctl NeonTokenSource = "neonctl"
)

// neonCLINames are the Neon CLI's binary names, in lookup order: current
// installs ship `neon`, older ones `neonctl`.
var neonCLINames = []string{"neon", "neonctl"}

// findNeonCLI returns the first Neon CLI binary name lookPath finds.
func findNeonCLI(lookPath func(string) (string, error)) (string, bool) {
	for _, name := range neonCLINames {
		if _, err := lookPath(name); err == nil {
			return name, true
		}
	}
	return "", false
}

// neonCLI is the Neon CLI binary to run and to name in printed commands: the
// one installed, else "neon".
func neonCLI() string {
	if name, ok := findNeonCLI(exec.LookPath); ok {
		return name
	}
	return neonCLINames[0]
}

// NeonToken is a Neon API token held in memory only. It is never written to a
// file, to meroku's own environment or to any log.
type NeonToken struct {
	Value  string
	Source NeonTokenSource
	// ExpiresAt is zero when the expiry is unknown (source env).
	ExpiresAt time.Time
}

// NeonAuthErrorKind says why no usable Neon token was found.
type NeonAuthErrorKind string

const (
	// neonAuthNoCredentials: the Neon CLI is installed but not logged in.
	neonAuthNoCredentials NeonAuthErrorKind = "no_credentials"
	// neonAuthNeonctlMissing: no NEON_API_KEY and no Neon CLI (neon or
	// neonctl) on PATH. The value is kept for the web API.
	neonAuthNeonctlMissing NeonAuthErrorKind = "neonctl_missing"
	// neonAuthExpired: the Neon CLI token has expired and was not refreshed.
	neonAuthExpired NeonAuthErrorKind = "expired"
	// neonAuthShort: the token is valid but has less life left than needed.
	neonAuthShort NeonAuthErrorKind = "short"
	// neonAuthRefreshFailed: waiting for expiry and refreshing did not yield
	// a usable token.
	neonAuthRefreshFailed NeonAuthErrorKind = "refresh_failed"
)

// NeonAuthError is a failure to get a usable Neon token.
type NeonAuthError struct {
	Kind NeonAuthErrorKind
	// Remaining is the token's life left, for neonAuthShort.
	Remaining time.Duration
	Err       error
}

func (e *NeonAuthError) Error() string {
	var msg string
	switch e.Kind {
	case neonAuthNoCredentials:
		msg = "neon: the Neon CLI is not logged in"
	case neonAuthNeonctlMissing:
		msg = "neon: no NEON_API_KEY and no Neon CLI (neon or neonctl) on PATH"
	case neonAuthExpired:
		msg = "neon: the Neon CLI token has expired"
	case neonAuthShort:
		msg = fmt.Sprintf("neon: the Neon token has only %s left", e.Remaining.Round(time.Second))
	case neonAuthRefreshFailed:
		msg = "neon: refreshing the Neon CLI token failed"
	default:
		msg = "neon: authentication failed"
	}
	if e.Err != nil {
		return msg + ": " + e.Err.Error()
	}
	return msg
}

func (e *NeonAuthError) Unwrap() error { return e.Err }

// neonAuthDeps is the outside world token resolution touches.
type neonAuthDeps struct {
	UserKey    func() string
	LookPath   func(string) (string, error)
	RunNeonctl func(ctx context.Context, args ...string) ([]byte, error)
	ReadFile   func(string) ([]byte, error)
	HomeDir    func() (string, error)
	Getenv     func(string) string
	Now        func() time.Time
	Tick       func(time.Duration) <-chan time.Time
}

// defaultNeonAuthDeps is the real system.
func defaultNeonAuthDeps() neonAuthDeps {
	return neonAuthDeps{
		UserKey:    func() string { return neonUserKey },
		LookPath:   exec.LookPath,
		RunNeonctl: runNeonctl,
		ReadFile:   os.ReadFile,
		HomeDir:    os.UserHomeDir,
		Getenv:     os.Getenv,
		Now:        time.Now,
		Tick:       time.Tick,
	}
}

// runNeonctl runs the Neon CLI non-interactively: CI=true stops it from
// opening a browser, and the environment carries no Neon secret.
func runNeonctl(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, neonCLI(), args...)
	cmd.Env = append(childEnv(), "CI=true")
	return cmd.CombinedOutput()
}

const (
	// neonMinWebLife is the token life a web lookup needs.
	neonMinWebLife = time.Minute
	// neonExpiryPollInterval is how often the wait for expiry checks.
	neonExpiryPollInterval = 5 * time.Second
	// neonExpiryGrace is how long past the expected expiry the wait goes on.
	neonExpiryGrace = 60 * time.Second
)

// neonctlCredentialsPaths lists where the Neon CLI keeps credentials.json, in
// the order the CLI itself reads them: an explicit NEON_CONFIG_DIR (or the
// older NEONCTL_CONFIG_DIR) alone; otherwise
// ${XDG_CONFIG_HOME:-$HOME/.config}/neon, then the legacy .../neonctl.
// Named profiles and keyring logins are not read.
func neonctlCredentialsPaths(d neonAuthDeps) ([]string, error) {
	for _, key := range []string{"NEON_CONFIG_DIR", "NEONCTL_CONFIG_DIR"} {
		if dir := d.Getenv(key); dir != "" {
			return []string{filepath.Join(dir, "credentials.json")}, nil
		}
	}
	base := d.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := d.HomeDir()
		if err != nil {
			return nil, err
		}
		base = filepath.Join(home, ".config")
	}
	return []string{
		filepath.Join(base, "neon", "credentials.json"),
		filepath.Join(base, "neonctl", "credentials.json"),
	}, nil
}

// readNeonctlCredentials returns the first credentials file that exists, with
// its path. When none exists the error wraps os.ErrNotExist.
func readNeonctlCredentials(d neonAuthDeps) (string, []byte, error) {
	paths, err := neonctlCredentialsPaths(d)
	if err != nil {
		return "", nil, err
	}
	for _, path := range paths {
		data, err := d.ReadFile(path)
		if err == nil {
			return path, data, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return path, nil, err
		}
	}
	return "", nil, fmt.Errorf("no Neon CLI credentials in %s: %w", strings.Join(paths, " or "), os.ErrNotExist)
}

// readNeonctlToken reads the Neon CLI login. meroku never writes this file.
// Current files hold type, access_token, expires_at, refresh_token and
// user_id; older neonctl files hold access_token and expires_at as well.
func readNeonctlToken(d neonAuthDeps) (NeonToken, error) {
	path, data, err := readNeonctlCredentials(d)
	if err != nil {
		return NeonToken{}, err
	}
	var creds struct {
		AccessToken string          `json:"access_token"`
		ExpiresAt   json.RawMessage `json:"expires_at"`
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return NeonToken{}, fmt.Errorf("reading %s: %w", path, err)
	}
	if creds.AccessToken == "" {
		return NeonToken{}, fmt.Errorf("%s has no access_token", path)
	}
	tok := NeonToken{Value: creds.AccessToken, Source: neonTokenFromNeonctl}
	if at, ok := parseNeonExpiry(creds.ExpiresAt); ok {
		tok.ExpiresAt = at
	}
	registerSecret(tok.Value)
	return tok, nil
}

// neonEpochSecondsLimit separates the two units expires_at has been written
// in: below it the number is seconds (a date before year 5138), at or above it
// milliseconds (a date after 1973).
const neonEpochSecondsLimit = 100_000_000_000

// parseNeonExpiry reads expires_at: an epoch in milliseconds or seconds, as a
// JSON number or a numeric string, or an RFC 3339 time.
func parseNeonExpiry(raw json.RawMessage) (time.Time, bool) {
	if n, ok := parseEpochMillis(raw); ok {
		if n > 0 && n < neonEpochSecondsLimit {
			return time.Unix(n, 0), true
		}
		return time.UnixMilli(n), true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseEpochMillis reads a millisecond epoch written as a JSON number or a
// numeric string.
func parseEpochMillis(raw json.RawMessage) (int64, bool) {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if s == "" || s == "null" {
		return 0, false
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, true
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f), true
	}
	return 0, false
}

// resolveNeonToken finds the Neon token: NEON_API_KEY first (used as is,
// expiry unknown), else the Neon CLI login, refreshed by `<cli> me` when it
// has expired. A token with less than minRemaining left is returned together
// with a neonAuthShort error, so an interactive caller can offer to continue.
func resolveNeonToken(ctx context.Context, d neonAuthDeps, minRemaining time.Duration) (NeonToken, error) {
	if key := d.UserKey(); key != "" {
		registerSecret(key)
		return NeonToken{Value: key, Source: neonTokenFromEnv}, nil
	}
	if _, ok := findNeonCLI(d.LookPath); !ok {
		return NeonToken{}, &NeonAuthError{Kind: neonAuthNeonctlMissing}
	}

	// `<cli> me` refreshes an expired token and never opens a browser with
	// CI=true. Its failure only matters if the file is then unusable.
	_, meErr := d.RunNeonctl(ctx, "me", "-o", "json")

	tok, err := readNeonctlToken(d)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return NeonToken{}, &NeonAuthError{Kind: neonAuthNoCredentials, Err: meErr}
		}
		return NeonToken{}, &NeonAuthError{Kind: neonAuthNoCredentials, Err: err}
	}
	if tok.ExpiresAt.IsZero() {
		return tok, nil
	}
	left := tok.ExpiresAt.Sub(d.Now())
	if left <= 0 {
		return tok, &NeonAuthError{Kind: neonAuthExpired, Remaining: left, Err: meErr}
	}
	if left < minRemaining {
		return tok, &NeonAuthError{Kind: neonAuthShort, Remaining: left}
	}
	return tok, nil
}

// waitAndRefreshNeonToken waits until tok has expired, polling every
// neonExpiryPollInterval with a countdown on out, then refreshes it with
// `<cli> me` and reads the login again. It gives up neonExpiryGrace after
// the expected expiry.
func waitAndRefreshNeonToken(ctx context.Context, d neonAuthDeps, tok NeonToken, out io.Writer) (NeonToken, error) {
	if tok.ExpiresAt.IsZero() {
		return tok, nil
	}
	deadline := tok.ExpiresAt.Add(neonExpiryGrace)
	for d.Now().Before(tok.ExpiresAt) {
		if !d.Now().Before(deadline) {
			return NeonToken{}, &NeonAuthError{Kind: neonAuthRefreshFailed}
		}
		left := tok.ExpiresAt.Sub(d.Now()).Round(time.Second)
		fmt.Fprintf(out, "\r⏳ Waiting for the Neon token to expire so it can be refreshed: %s ", left)
		select {
		case <-ctx.Done():
			fmt.Fprintln(out)
			return NeonToken{}, &NeonAuthError{Kind: neonAuthRefreshFailed, Err: ctx.Err()}
		case <-d.Tick(neonExpiryPollInterval):
		}
	}
	fmt.Fprintln(out, "\r🔄 Refreshing the Neon token...                                        ")

	for {
		_, meErr := d.RunNeonctl(ctx, "me", "-o", "json")
		fresh, err := readNeonctlToken(d)
		if err == nil && (fresh.ExpiresAt.IsZero() || fresh.ExpiresAt.After(d.Now())) {
			return fresh, nil
		}
		if !d.Now().Before(deadline) {
			if err == nil {
				err = meErr
			}
			return NeonToken{}, &NeonAuthError{Kind: neonAuthRefreshFailed, Err: err}
		}
		select {
		case <-ctx.Done():
			return NeonToken{}, &NeonAuthError{Kind: neonAuthRefreshFailed, Err: ctx.Err()}
		case <-d.Tick(neonExpiryPollInterval):
		}
	}
}

// neonTokenRefresher is the refresh function a session gets: wait for the
// current Neon CLI token to expire, then refresh and re-read it.
func neonTokenRefresher(d neonAuthDeps) func(context.Context, io.Writer) (NeonToken, error) {
	return func(ctx context.Context, out io.Writer) (NeonToken, error) {
		cur, err := readNeonctlToken(d)
		if err != nil {
			return NeonToken{}, &NeonAuthError{Kind: neonAuthRefreshFailed, Err: err}
		}
		return waitAndRefreshNeonToken(ctx, d, cur, out)
	}
}

// neonNoCredentialsMessage is shown, before any Terraform runs, when Neon is
// enabled but meroku has neither NEON_API_KEY nor the Neon CLI.
func neonNoCredentialsMessage(env string) string {
	return fmt.Sprintf("❌ Neon is enabled for %s, but meroku found no Neon credentials.\n"+
		"   Either set NEON_API_KEY (for CI), or log in with the Neon CLI:\n"+
		"     npm i -g neon && neon login\n", env)
}

// neonKeyringLoginMessage is shown when a Neon CLI login succeeded but left
// no credentials file: the CLI kept the login in the system keyring, which
// meroku does not read.
func neonKeyringLoginMessage(env string) string {
	return fmt.Sprintf("❌ The Neon CLI login for %s finished, but it left no credentials file.\n"+
		"   The Neon CLI keeps some logins in the system keyring; meroku reads only the\n"+
		"   file-based login (credentials.json in the Neon CLI's config directory).\n"+
		"   Either log in so the CLI writes that file, or set NEON_API_KEY and run again.\n", env)
}

// neonStdin is the one reader every Neon prompt reads from, so input buffered
// while answering one prompt is still there for the next.
var neonStdin = bufio.NewReader(os.Stdin)

// neonAuthPrompt asks the user to pick one of options and returns its index,
// or -1 when nothing was chosen. A variable so tests can answer.
var neonAuthPrompt = func(question string, options []string) int {
	fmt.Println(question)
	for i, o := range options {
		fmt.Printf("  [%d] %s\n", i+1, o)
	}
	fmt.Print("Choose: ")
	line, err := neonStdin.ReadString('\n')
	if err != nil && line == "" {
		return -1
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(options) {
		return -1
	}
	return n - 1
}

// neonLogin runs `<cli> auth`, which opens a browser. `auth` is the command
// in both the `neon` and the older `neonctl` CLI, with `login` as its alias;
// printed text names the documented `login`. A variable so tests never start
// one.
var neonLogin = func(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, neonCLI(), "auth")
	cmd.Env = childEnv()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// ensureNeonSession resolves the token for a Neon run of env. Non-interactive
// callers get the error as is. Interactive callers are offered, for a short
// token: wait for it to expire and refresh, continue anyway, or cancel; for a
// missing or expired login: log in with the Neon CLI, or cancel. A short token is
// returned without error only when the user chose to continue. A login that
// still leaves no credentials file ends the loop with the keyring explanation
// instead of offering the same login again.
func ensureNeonSession(ctx context.Context, env Env, interactive bool) (NeonToken, error) {
	d := defaultNeonAuthDeps()
	loggedIn := false
	notice := neonStateReadNotice(env)
	for {
		tok, err := resolveNeonToken(ctx, d, neonMinTerraformLife)
		if err == nil {
			return tok, nil
		}
		var authErr *NeonAuthError
		if !errors.As(err, &authErr) {
			return NeonToken{}, err
		}
		if loggedIn && authErr.Kind == neonAuthNoCredentials {
			return NeonToken{}, fmt.Errorf("%s%w", neonKeyringLoginMessage(env.Env), err)
		}
		if authErr.Kind == neonAuthNeonctlMissing {
			return NeonToken{}, fmt.Errorf("%s%w", neonNoCredentialsMessage(env.Env), err)
		}
		if !interactive {
			if notice != "" {
				return NeonToken{}, fmt.Errorf("%s%w", notice, err)
			}
			return NeonToken{}, err
		}

		switch authErr.Kind {
		case neonAuthShort:
			mins := int(authErr.Remaining.Round(time.Minute) / time.Minute)
			choice := neonAuthPrompt(
				fmt.Sprintf("⚠️  The Neon token has %s left; one Terraform run needs %s.",
					authErr.Remaining.Round(time.Second), neonMinTerraformLife),
				[]string{
					fmt.Sprintf("Wait for it to expire, then refresh automatically (≈%d min)", mins),
					fmt.Sprintf("Continue anyway (%d min left)", mins),
					"Cancel",
				})
			switch choice {
			case 0:
				fresh, werr := waitAndRefreshNeonToken(ctx, d, tok, os.Stdout)
				if werr != nil {
					return NeonToken{}, werr
				}
				return fresh, nil
			case 1:
				return tok, nil
			default:
				return NeonToken{}, err
			}
		default:
			choice := neonAuthPrompt(
				fmt.Sprintf("%s⚠️  %s.", notice, authErr.Error()),
				[]string{fmt.Sprintf("Log in now (%s login — opens a browser)", neonCLI()), "Cancel"})
			if choice != 0 {
				return NeonToken{}, err
			}
			if lerr := neonLogin(ctx); lerr != nil {
				return NeonToken{}, &NeonAuthError{Kind: authErr.Kind, Err: lerr}
			}
			// Logged in: resolve again.
			loggedIn = true
		}
	}
}
