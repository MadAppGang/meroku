package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Secret scope for child processes.
//
// The Neon token and the Postgres credentials meroku computes reach terraform
// processes and nothing else. newTerraformCmd is the only constructor for a
// terraform process; childEnv is the environment every other child gets.

const (
	neonAPIKeyEnvName        = "NEON_API_KEY"
	neonPGCredentialsEnvName = "TF_VAR_neon_pg_credentials"
)

// tfPurpose says whether a terraform command evaluates configuration.
type tfPurpose int

const (
	// tfState commands touch no provider configuration: init, state
	// list/rm/show, output, show of a plan file, force-unlock, version. They
	// never get Neon secrets.
	tfState tfPurpose = iota
	// tfConfig commands configure providers: plan, apply, destroy, import.
	tfConfig
)

var (
	errNeonSessionRequired = errors.New("neon: this command needs a Neon login; run it from deploy or destroy")
	errNeonUpgradeRefused  = errors.New("init -upgrade refused: it could move a Neon provider off its pinned release; see NEON.md")
	errNeonTokenShort      = errors.New("neon: the Neon token has less than 5 minutes left")
)

// neonMinTerraformLife is the token life one terraform process needs.
const neonMinTerraformLife = 5 * time.Minute

// neonUserKey is NEON_API_KEY as meroku found it at startup.
var neonUserKey string

// captureNeonEnv moves NEON_API_KEY from meroku's environment into memory and
// removes it and TF_VAR_neon_pg_credentials from the environment, so no child
// process inherits either. A user-set TF_VAR_neon_pg_credentials is
// discarded: meroku computes that value itself.
func captureNeonEnv(getenv func(string) string, unsetenv func(string) error) {
	neonUserKey = getenv(neonAPIKeyEnvName)
	registerSecret(neonUserKey)
	if getenv(neonPGCredentialsEnvName) != "" {
		fmt.Fprintf(os.Stderr, "ℹ️  Ignoring %s from the environment: meroku computes it.\n", neonPGCredentialsEnvName)
	}
	_ = unsetenv(neonAPIKeyEnvName)
	_ = unsetenv(neonPGCredentialsEnvName)
}

// childEnv is meroku's environment without the Neon secret variables.
func childEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if name == neonAPIKeyEnvName || name == neonPGCredentialsEnvName {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// newTerraformCmd builds a terraform command that runs in dir. It is the only
// constructor for terraform processes, and the only place Neon secrets enter a
// child environment. Whether dir is a Neon dir is decided from the files in
// dir, never from the process working directory. Rules, in order:
//
//  1. init -upgrade in a Neon dir is refused while a Neon provider in its
//     lock is not pinned to its exact release (neonUnpinnedProviders).
//  2. tfState commands, and tfConfig commands outside a Neon dir, get
//     childEnv().
//  3. tfConfig in a Neon dir needs a session: none is errNeonSessionRequired;
//     a degraded session gets childEnv().
//  4. A token with a known expiry and less than neonMinTerraformLife left is
//     errNeonTokenShort, unless the user accepted a short token that has not
//     yet expired. Waiting is the caller's job (EnsureFresh); this never
//     blocks.
//  5. Otherwise childEnv() plus NEON_API_KEY and, once set,
//     TF_VAR_neon_pg_credentials.
func newTerraformCmd(ctx context.Context, purpose tfPurpose, dir string, args ...string) (*exec.Cmd, error) {
	if dir == "" {
		return nil, errors.New("terraform: no working directory given")
	}
	neonDir := isNeonDir(dir)
	if neonDir && isInitUpgrade(args) {
		if unpinned := neonUnpinnedProviders(dir); len(unpinned) > 0 {
			return nil, fmt.Errorf("%w (%s not pinned in main.tf)", errNeonUpgradeRefused, strings.Join(unpinned, ", "))
		}
	}

	env := childEnv()
	if purpose == tfConfig && neonDir {
		s := currentNeonSession()
		if s == nil {
			return nil, errNeonSessionRequired
		}
		extra, err := s.terraformEnv(time.Now())
		if err != nil {
			return nil, err
		}
		env = append(env, extra...)
	}

	cmd := exec.CommandContext(ctx, "terraform", args...)
	cmd.Dir = dir
	cmd.Env = env
	return cmd, nil
}

// isNeonDir reports whether dir's main.tf or neon_retired.tf uses the Neon
// provider or the Postgres credentials variable.
func isNeonDir(dir string) bool {
	for _, name := range []string{"main.tf", "neon_retired.tf"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if bytes.Contains(data, []byte("kislerdm/neon")) || bytes.Contains(data, []byte("neon_pg_credentials")) {
			return true
		}
	}
	return false
}

// neonPinnedProviders are the Neon providers and the one release each may be
// at. The render, the modules and the retired override declare them with
// these exact versions, which leaves `init -upgrade` no other release to
// choose; it can still move other providers, such as hashicorp/aws.
var neonPinnedProviders = []struct{ source, version string }{
	{"kislerdm/neon", "0.18.0"},
	{neonPostgresSource, neonPostgresVersion},
}

// neonUnpinnedProviders lists the Neon providers dir's lock file holds that
// the root configuration (main.tf, neon_retired_override.tf) does not pin to
// their exact release. `init -upgrade` would be free to move those, so it is
// refused while the list is not empty. A provider absent from the lock has
// nothing to move.
func neonUnpinnedProviders(dir string) []string {
	lock, err := os.ReadFile(filepath.Join(dir, ".terraform.lock.hcl"))
	if err != nil {
		return nil
	}
	var config []byte
	for _, name := range []string{"main.tf", neonRetiredOverrideFile} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			config = append(config, data...)
			config = append(config, '\n')
		}
	}
	var unpinned []string
	for _, p := range neonPinnedProviders {
		if !bytes.Contains(lock, []byte(`"registry.terraform.io/`+p.source+`"`)) {
			continue
		}
		pin := regexp.MustCompile(`source\s*=\s*"` + regexp.QuoteMeta(p.source) + `"\s*\n\s*version\s*=\s*"=?\s*` + regexp.QuoteMeta(p.version) + `"`)
		if !pin.Match(config) {
			unpinned = append(unpinned, p.source)
		}
	}
	return unpinned
}

// isInitUpgrade reports whether args are `init` with -upgrade.
func isInitUpgrade(args []string) bool {
	if len(args) == 0 || args[0] != "init" {
		return false
	}
	for _, a := range args[1:] {
		switch a {
		case "-upgrade", "--upgrade", "-upgrade=true", "--upgrade=true":
			return true
		}
	}
	return false
}

// workingDir is the process working directory for call sites that run
// terraform where meroku currently is (env/<env> during a deploy). Captured
// explicitly so newTerraformCmd always gets a directory.
func workingDir() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

type neonSessionMode int

const (
	sessionEmpty neonSessionMode = iota
	sessionToken
	sessionDegraded
)

// neonSession holds the Neon token and the Postgres credentials for the
// terraform processes of one Neon run.
type neonSession struct {
	mu   sync.Mutex
	mode neonSessionMode
	tok  NeonToken
	// refresh waits for an expiring token to expire, then refreshes it.
	refresh func(ctx context.Context, out io.Writer) (NeonToken, error)
	// credsJSON is the value of TF_VAR_neon_pg_credentials; "" until
	// SetCredentials.
	credsJSON string
	// AcceptShort records that the user chose to continue with a token that
	// has less than the minimum life left.
	AcceptShort bool
}

// activeNeonSession is the process-wide holder.
var activeNeonSession struct {
	mu sync.Mutex
	s  *neonSession
}

func currentNeonSession() *neonSession {
	activeNeonSession.mu.Lock()
	defer activeNeonSession.mu.Unlock()
	return activeNeonSession.s
}

func setActiveNeonSession(s *neonSession) {
	activeNeonSession.mu.Lock()
	defer activeNeonSession.mu.Unlock()
	activeNeonSession.s = s
}

// beginNeonSession makes a token session the active one. clear empties the
// holder. A token that already has less than neonMinTerraformLife left was
// accepted by the user (ensureNeonSession returns one only after "Continue
// anyway"), so the session starts with AcceptShort set.
func beginNeonSession(tok NeonToken, refresh func(context.Context, io.Writer) (NeonToken, error)) (*neonSession, func()) {
	registerSecret(tok.Value)
	s := &neonSession{mode: sessionToken, tok: tok, refresh: refresh}
	if !tok.ExpiresAt.IsZero() && time.Until(tok.ExpiresAt) < neonMinTerraformLife {
		s.AcceptShort = true
	}
	setActiveNeonSession(s)
	return s, func() { clearNeonSession(s) }
}

// beginDegradedNeonSession makes a session with no token and no credentials
// the active one.
func beginDegradedNeonSession() (*neonSession, func()) {
	s := &neonSession{mode: sessionDegraded}
	setActiveNeonSession(s)
	return s, func() { clearNeonSession(s) }
}

// clearNeonSession empties the holder if s is still the active session, and
// drops the secrets s held.
func clearNeonSession(s *neonSession) {
	activeNeonSession.mu.Lock()
	if activeNeonSession.s == s {
		activeNeonSession.s = nil
	}
	activeNeonSession.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = sessionEmpty
	s.tok = NeonToken{}
	s.credsJSON = ""
}

// SetCredentials stores the Postgres credentials JSON for later terraform
// processes.
func (s *neonSession) SetCredentials(json string) {
	registerSecret(json)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.credsJSON = json
}

// terraformEnv is what a tfConfig command in a Neon dir adds to childEnv(),
// following newTerraformCmd's rules 3 to 5.
func (s *neonSession) terraformEnv(now time.Time) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch s.mode {
	case sessionDegraded:
		return nil, nil
	case sessionToken:
	default:
		return nil, errNeonSessionRequired
	}
	if !s.tok.ExpiresAt.IsZero() {
		left := s.tok.ExpiresAt.Sub(now)
		accepted := s.AcceptShort && left > 0
		if left < neonMinTerraformLife && !accepted {
			return nil, errNeonTokenShort
		}
	}
	env := []string{neonAPIKeyEnvName + "=" + s.tok.Value}
	if s.credsJSON != "" {
		env = append(env, neonPGCredentialsEnvName+"="+s.credsJSON)
	}
	return env, nil
}

// EnsureFresh makes sure the token outlives the next terraform process. A
// token with less than neonMinTerraformLife left is refreshed with the
// session's refresh function, which waits for it to expire and writes its
// countdown to out. With AcceptShort, only a token that has actually expired
// is refreshed. A token of unknown expiry (NEON_API_KEY) is used as is.
// Callers run it inside their tea.Cmd or flow step, before newTerraformCmd.
func (s *neonSession) EnsureFresh(ctx context.Context, out io.Writer) error {
	s.mu.Lock()
	if s.mode != sessionToken || s.tok.ExpiresAt.IsZero() {
		s.mu.Unlock()
		return nil
	}
	left := time.Until(s.tok.ExpiresAt)
	if left >= neonMinTerraformLife || (s.AcceptShort && left > 0) {
		s.mu.Unlock()
		return nil
	}
	refresh := s.refresh
	s.mu.Unlock()

	if refresh == nil {
		kind := neonAuthShort
		if left <= 0 {
			kind = neonAuthExpired
		}
		return &NeonAuthError{Kind: kind, Remaining: left}
	}
	if out == nil {
		out = io.Discard
	}
	// The lock is not held while refresh waits, so newTerraformCmd never
	// blocks on it.
	tok, err := refresh(ctx, out)
	if err != nil {
		var authErr *NeonAuthError
		if errors.As(err, &authErr) {
			return err
		}
		return &NeonAuthError{Kind: neonAuthRefreshFailed, Remaining: left, Err: err}
	}
	registerSecret(tok.Value)
	if !tok.ExpiresAt.IsZero() {
		if newLeft := time.Until(tok.ExpiresAt); newLeft < neonMinTerraformLife {
			return &NeonAuthError{Kind: neonAuthShort, Remaining: newLeft}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mode != sessionToken {
		// Cleared while refreshing: the run is over.
		return errNeonSessionRequired
	}
	s.tok = tok
	s.AcceptShort = false
	return nil
}

// ensureActiveNeonSessionFresh runs EnsureFresh on the active session, if
// there is one.
func ensureActiveNeonSessionFresh(ctx context.Context, out io.Writer) error {
	if s := currentNeonSession(); s != nil {
		return s.EnsureFresh(ctx, out)
	}
	return nil
}

// activeNeonTokenSource is the source of the active session's token, or ""
// when there is none.
func activeNeonTokenSource() NeonTokenSource {
	s := currentNeonSession()
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mode != sessionToken {
		return ""
	}
	return s.tok.Source
}
