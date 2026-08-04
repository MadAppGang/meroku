// Package testsupport hands every test the same environment: the one
// Terraform actually produced for the synthetic project in
// internal/boundary/testdata.
//
// No test builds its own map. The old suite did, which is how it managed to
// pass green for months while the backend deploy path was completely broken:
// the config test keyed the map on "", the handler test keyed it on "backend",
// and nothing ever compared the two.
package testsupport

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"madappgang.com/infrastructure/ci_lambda/internal/config"
)

type golden struct {
	Env map[string]string `json:"env"`
}

func goldenPath(t testing.TB) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("testsupport: cannot locate its own source file")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "boundary", "testdata", "tf_identifiers.golden.json")
}

// Env returns the captured Terraform environment, plus any overrides. An
// override with an empty value removes the variable.
func Env(t testing.TB, overrides map[string]string) map[string]string {
	t.Helper()

	raw, err := os.ReadFile(goldenPath(t))
	if err != nil {
		t.Fatalf("testsupport: %v", err)
	}
	var g golden
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatalf("testsupport: %v", err)
	}

	env := make(map[string]string, len(g.Env)+len(overrides))
	for k, v := range g.Env {
		env[k] = v
	}
	for k, v := range overrides {
		if v == "" {
			delete(env, k)
			continue
		}
		env[k] = v
	}
	return env
}

// Getenv adapts a map to the lookup function config.Load takes, so no test
// touches the process environment.
func Getenv(env map[string]string) func(string) string {
	return func(k string) string { return env[k] }
}

// Config loads the captured environment through the real loader.
func Config(t testing.TB, overrides map[string]string) *config.Config {
	t.Helper()
	cfg, err := config.Load(Getenv(Env(t, overrides)))
	if err != nil {
		t.Fatalf("testsupport: %v", err)
	}
	return cfg
}

// Logger is a logger that writes nowhere, for tests that do not assert on logs.
func Logger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}
