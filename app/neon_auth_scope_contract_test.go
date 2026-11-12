package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// REQ-3 / TEST-3: expiry is checked again after time spent reviewing a plan.
func TestNeonContractLongReviewCannotStartApplyWithShortToken(t *testing.T) {
	old := currentNeonSession()
	t.Cleanup(func() { setActiveNeonSession(old) })
	now := time.Now()
	sess, clear := beginNeonSession(NeonToken{Value: "fake-review-token-do-not-use", Source: neonTokenFromNeonctl, ExpiresAt: now.Add(10 * time.Minute)}, nil)
	t.Cleanup(clear)
	if _, err := sess.terraformEnv(now); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.terraformEnv(now.Add(6 * time.Minute)); !errors.Is(err, errNeonTokenShort) {
		t.Fatalf("apply after long review: %v, want short-token refusal", err)
	}
}

// REQ-3 / TEST-4: redact before truncation and file output, not only on screen.
func TestNeonContractHandoffBriefingNeverWritesRegisteredSecrets(t *testing.T) {
	root := t.TempDir()
	fakeHandoffSystem(t, nil, "", nil)
	secret := "fake-handoff-secret-do-not-use"
	registerSecret(secret)
	ctx := sampleHandoffContext(root)
	ctx.ErrorOutput = "failure: " + secret
	ctx.Diagnostics = []string{"provider: " + secret}
	ctx.Failures[0].Detail = "password: " + secret
	result, err := LaunchClaudeHandoff(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(result.BriefingPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) || strings.Contains(result.ManualCommand, secret) {
		t.Fatal("handoff leaked a registered secret")
	}
}

func qaNeonAuthDeps(now time.Time, key, credentials string) neonAuthDeps {
	return neonAuthDeps{
		UserKey: func() string { return key },
		LookPath: func(name string) (string, error) {
			if name == "neon" {
				return "/fake/bin/neon", nil
			}
			return "", exec.ErrNotFound
		},
		RunNeonctl: func(context.Context, ...string) ([]byte, error) { return []byte("{}"), nil },
		ReadFile: func(string) ([]byte, error) {
			if credentials == "" {
				return nil, os.ErrNotExist
			}
			return []byte(credentials), nil
		},
		HomeDir: func() (string, error) { return "/fake/home", nil },
		Getenv:  func(string) string { return "" },
		Now:     func() time.Time { return now },
	}
}

// REQ-1 / TEST-1.
func TestNeonContractEnvironmentKeyWinsOverUnusableLogin(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d := qaNeonAuthDeps(now, "fake-environment-key-do-not-use", "malformed")
	d.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	tok, err := resolveNeonToken(context.Background(), d, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "fake-environment-key-do-not-use" || tok.Source != neonTokenFromEnv || !tok.ExpiresAt.IsZero() {
		t.Fatalf("wrong auth result source/expiry: %s %v", tok.Source, tok.ExpiresAt)
	}
}

// REQ-2, REQ-5 / TEST-2.
func TestNeonContractLoginExpiryAndFailures(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name      string
		remaining time.Duration
		kind      NeonAuthErrorKind
	}{
		{"usable", 10 * time.Minute, ""},
		{"exactly five minutes", 5 * time.Minute, ""},
		{"short", 4 * time.Minute, neonAuthShort},
		{"expired", -time.Minute, neonAuthExpired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]interface{}{"access_token": "fake-oauth-token-do-not-use", "expires_at": now.Add(tc.remaining).UnixMilli(), "type": "oauth"})
			d := qaNeonAuthDeps(now, "", string(raw))
			tok, err := resolveNeonToken(context.Background(), d, 5*time.Minute)
			if tc.kind == "" {
				if err != nil {
					t.Fatal(err)
				}
				if tok.Value != "fake-oauth-token-do-not-use" || tok.Source != neonTokenFromNeonctl {
					t.Fatal("wrong CLI token")
				}
				return
			}
			var ae *NeonAuthError
			if !errors.As(err, &ae) || ae.Kind != tc.kind {
				t.Fatalf("error=%v want kind %s", err, tc.kind)
			}
			if tc.kind == neonAuthShort && tok.Value == "" {
				t.Error("short-life caller needs the token to offer Continue")
			}
			if strings.Contains(err.Error(), "fake-oauth-token-do-not-use") {
				t.Error("auth error leaked token")
			}
		})
	}
	for _, tc := range []struct {
		name, raw  string
		missingCLI bool
	}{
		{"missing login", "", false}, {"malformed login", `{"access_token":"fake-oauth-token-do-not-use",`, false}, {"missing CLI", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := qaNeonAuthDeps(now, "", tc.raw)
			if tc.missingCLI {
				d.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
			}
			_, err := resolveNeonToken(context.Background(), d, 5*time.Minute)
			if err == nil {
				t.Fatal("unusable auth accepted")
			}
			var ae *NeonAuthError
			if !errors.As(err, &ae) {
				t.Fatalf("error lacks auth kind: %T", err)
			}
			if tc.missingCLI && ae.Kind != neonAuthNeonctlMissing {
				t.Fatalf("kind=%s", ae.Kind)
			}
			if tc.raw == "" && !tc.missingCLI && ae.Kind != neonAuthNoCredentials {
				t.Fatalf("kind=%s", ae.Kind)
			}
			if strings.Contains(err.Error(), "fake-oauth-token-do-not-use") {
				t.Error("auth error leaked token")
			}
		})
	}
	msg := neonNoCredentialsMessage("dev")
	for _, s := range []string{"NEON_API_KEY", "npm i -g neon", "neon login", "dev"} {
		if !strings.Contains(msg, s) {
			t.Errorf("missing login instruction %q", s)
		}
	}
}

// REQ-2 / TEST-2.
// REQ-2 / TEST-2: a successful noninteractive refresh replaces the expired login.
func TestNeonContractExpiredLoginCanRefreshWithoutBrowser(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	refreshed := false
	d := qaNeonAuthDeps(now, "", "")
	d.RunNeonctl = func(context.Context, ...string) ([]byte, error) { refreshed = true; return []byte("{}"), nil }
	d.ReadFile = func(string) ([]byte, error) {
		token := "fake-expired-token-do-not-use"
		expiry := now.Add(-time.Minute)
		if refreshed {
			token = "fake-refreshed-token-do-not-use"
			expiry = now.Add(time.Hour)
		}
		return json.Marshal(map[string]interface{}{"access_token": token, "expires_at": expiry.UnixMilli()})
	}
	tok, err := resolveNeonToken(context.Background(), d, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "fake-refreshed-token-do-not-use" || !tok.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatal("refresh did not yield the new usable token")
	}
}

func TestNeonContractCLIAndCredentialsLookupOrder(t *testing.T) {
	for _, tc := range []struct {
		name      string
		installed map[string]bool
		want      string
		ok        bool
	}{
		{"both", map[string]bool{"neon": true, "neonctl": true}, "neon", true},
		{"legacy", map[string]bool{"neonctl": true}, "neonctl", true},
		{"none", map[string]bool{}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := findNeonCLI(func(name string) (string, error) {
				if tc.installed[name] {
					return "/fake/" + name, nil
				}
				return "", exec.ErrNotFound
			})
			if got != tc.want || ok != tc.ok {
				t.Fatalf("CLI=(%q,%v) want (%q,%v)", got, ok, tc.want, tc.ok)
			}
		})
	}
	for _, tc := range []struct {
		name string
		env  map[string]string
		want []string
	}{
		{"home", map[string]string{}, []string{"/fake/home/.config/neon/credentials.json", "/fake/home/.config/neonctl/credentials.json"}},
		{"xdg", map[string]string{"XDG_CONFIG_HOME": "/fake/xdg"}, []string{"/fake/xdg/neon/credentials.json", "/fake/xdg/neonctl/credentials.json"}},
		{"legacy override", map[string]string{"NEONCTL_CONFIG_DIR": "/fake/old"}, []string{"/fake/old/credentials.json"}},
		{"current wins", map[string]string{"NEON_CONFIG_DIR": "/fake/new", "NEONCTL_CONFIG_DIR": "/fake/old", "XDG_CONFIG_HOME": "/fake/xdg"}, []string{"/fake/new/credentials.json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := qaNeonAuthDeps(time.Now(), "", "")
			d.Getenv = func(k string) string { return tc.env[k] }
			got, err := neonctlCredentialsPaths(d)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("paths=%v want %v", got, tc.want)
			}
		})
	}
}

func qaNeonEnvValue(env []string, key string) (string, bool) {
	for _, v := range env {
		if strings.HasPrefix(v, key+"=") {
			return strings.TrimPrefix(v, key+"="), true
		}
	}
	return "", false
}

// REQ-3 / TEST-3.
func TestNeonContractCaptureUnsetsAndChildEnvStripsBothSecrets(t *testing.T) {
	oldKey := neonUserKey
	t.Cleanup(func() { neonUserKey = oldKey })
	t.Setenv("NEON_API_KEY", "fake-captured-key-do-not-use")
	t.Setenv("TF_VAR_neon_pg_credentials", "fake-user-creds-do-not-use")
	t.Setenv("QA_NEON_PUBLIC", "retained")
	captureNeonEnv(os.Getenv, os.Unsetenv)
	if os.Getenv("NEON_API_KEY") != "" || os.Getenv("TF_VAR_neon_pg_credentials") != "" {
		t.Fatal("startup secrets remain in process environment")
	}
	if neonUserKey != "fake-captured-key-do-not-use" {
		t.Fatal("startup key not retained for auth")
	}
	// A dependency may put them back; childEnv must still strip both.
	t.Setenv("NEON_API_KEY", "fake-reinserted-key-do-not-use")
	t.Setenv("TF_VAR_neon_pg_credentials", "fake-reinserted-creds-do-not-use")
	env := childEnv()
	for _, key := range []string{"NEON_API_KEY", "TF_VAR_neon_pg_credentials"} {
		if _, ok := qaNeonEnvValue(env, key); ok {
			t.Errorf("%s leaked to generic child", key)
		}
	}
	if v, _ := qaNeonEnvValue(env, "QA_NEON_PUBLIC"); v != "retained" {
		t.Error("ordinary environment dropped")
	}
}

// REQ-3 / TEST-3: commands are constructed, never executed.
func TestNeonContractOnlyConfigCommandsInTheirNeonDirReceiveSecrets(t *testing.T) {
	old := currentNeonSession()
	setActiveNeonSession(nil)
	t.Cleanup(func() { setActiveNeonSession(old) })
	neonDir, awsDir := t.TempDir(), t.TempDir()
	qaNeonWrite(t, filepath.Join(neonDir, "main.tf"), `variable "neon_pg_credentials" {
 default = {}
 sensitive = true
 ephemeral = true
}
`)
	qaNeonWrite(t, filepath.Join(awsDir, "main.tf"), "terraform {}\n")
	_, err := newTerraformCmd(context.Background(), tfConfig, neonDir, "plan")
	if !errors.Is(err, errNeonSessionRequired) {
		t.Fatalf("no session error=%v", err)
	}
	sess, clear := beginNeonSession(NeonToken{Value: "fake-tf-key-do-not-use", Source: neonTokenFromEnv}, nil)
	t.Cleanup(clear)
	for _, tc := range []struct {
		name, dir string
		purpose   tfPurpose
		args      []string
		secrets   bool
	}{
		{"Neon plan", neonDir, tfConfig, []string{"plan"}, true},
		{"state show", neonDir, tfState, []string{"show", "-json"}, false},
		{"AWS-only plan", awsDir, tfConfig, []string{"plan"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd, err := newTerraformCmd(context.Background(), tc.purpose, tc.dir, tc.args...)
			if err != nil {
				t.Fatal(err)
			}
			if cmd.Dir != tc.dir {
				t.Fatalf("dir=%q want %q", cmd.Dir, tc.dir)
			}
			key, ok := qaNeonEnvValue(cmd.Env, "NEON_API_KEY")
			if ok != tc.secrets || (ok && key != "fake-tf-key-do-not-use") {
				t.Fatal("incorrect token scope")
			}
			if _, ok = qaNeonEnvValue(cmd.Env, "TF_VAR_neon_pg_credentials"); ok {
				t.Fatal("unset credentials must be omitted")
			}
		})
	}
	sess.SetCredentials(`{"app":{"host":"ep-fake.example.invalid","user":"neondb_owner","password":"fake-pg-password-do-not-use","pg_version":"17"}}`)
	cmd, err := newTerraformCmd(context.Background(), tfConfig, neonDir, "apply", "tfplan")
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := qaNeonEnvValue(cmd.Env, "TF_VAR_neon_pg_credentials"); !ok || !strings.Contains(v, "fake-pg-password-do-not-use") {
		t.Fatal("config child lacks supplied credentials")
	}
	if strings.Contains(strings.Join(cmd.Args, " "), "fake-pg-password") || strings.Contains(strings.Join(cmd.Args, " "), "fake-tf-key") {
		t.Fatal("secrets entered argv")
	}
	clear()
	_, err = newTerraformCmd(context.Background(), tfConfig, neonDir, "plan")
	if !errors.Is(err, errNeonSessionRequired) {
		t.Fatalf("cleared session remains usable: %v", err)
	}
	_, end := beginDegradedNeonSession()
	t.Cleanup(end)
	cmd, err = newTerraformCmd(context.Background(), tfConfig, neonDir, "destroy", "-refresh=false")
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"NEON_API_KEY", "TF_VAR_neon_pg_credentials"} {
		if _, ok := qaNeonEnvValue(cmd.Env, k); ok {
			t.Errorf("degraded destroy received %s", k)
		}
	}
}

// REQ-20 / TEST-17: the latest contract allows pinned upgrades, not blanket refusal.
func TestNeonContractUpgradeRefusesOnlyUnconstrainedStateProviders(t *testing.T) {
	for _, tc := range []struct {
		name, pinFile, pin string
		refused            bool
	}{
		{"state-only", "", "", true},
		{"range in main", "main.tf", "~> 1.27", true},
		{"exact in main", "main.tf", "1.27.0", false},
		{"exact in override", "neon_retired_override.tf", "1.27.0", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			qaNeonWrite(t, filepath.Join(dir, "main.tf"), `variable "neon_pg_credentials" { default = {} }`+"\n")
			qaNeonWrite(t, filepath.Join(dir, ".terraform.lock.hcl"), `provider "registry.terraform.io/cyrilgdn/postgresql" { version = "1.27.0" }`+"\n")
			if tc.pinFile != "" {
				qaNeonWrite(t, filepath.Join(dir, tc.pinFile), `terraform {
 required_providers {
  postgresql = {
   source = "cyrilgdn/postgresql"
   version = "`+tc.pin+`"
  }
 }
}
variable "neon_pg_credentials" { default = {} }
`)
			}
			cmd, err := newTerraformCmd(context.Background(), tfState, dir, "init", "-upgrade", "-input=false")
			if tc.refused {
				if !errors.Is(err, errNeonUpgradeRefused) {
					t.Fatalf("error=%v want upgrade refusal", err)
				}
			} else if err != nil || cmd == nil {
				t.Fatalf("exactly pinned upgrade refused: %v", err)
			}
		})
	}
}
