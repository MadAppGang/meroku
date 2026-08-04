//go:build tfgolden

// Regeneration and drift check for the committed golden file.
//
//	go test -tags tfgolden ./internal/boundary/            # fail on drift
//	go test -tags tfgolden ./internal/boundary/ -update    # rewrite the file
//
// This is the only test that shells out to terraform, and it is not the gate:
// boundary_test.go is, and that one always runs. Keeping regeneration separate
// means a machine without terraform loses the ability to *update* the capture,
// never the ability to *check* it.
package boundary_test

import (
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"madappgang.com/infrastructure/ci_lambda/internal/boundary"
)

var update = flag.Bool("update", false, "rewrite the committed golden file from terraform output")

func TestGoldenMatchesTerraform(t *testing.T) {
	tf, err := exec.LookPath("terraform")
	require.NoError(t, err, "terraform is required to regenerate or drift-check the golden file")

	fixture, err := filepath.Abs(filepath.Join("testdata", "tfgolden"))
	require.NoError(t, err)

	env := append(os.Environ(),
		"TF_IN_AUTOMATION=1",
		"CHECKPOINT_DISABLE=1",
		"TF_DATA_DIR="+t.TempDir(),
	)

	run := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command(tf, args...)
		cmd.Dir = fixture
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "terraform %v failed:\n%s", args, out)
		return out
	}

	// The fixture is run in place, not copied to a temp directory: its module
	// source is relative, and a copy would resolve it against the temp dir and
	// fail to find the module at all. Only the state files land here, and they
	// are removed again below (and gitignored).
	t.Cleanup(func() {
		_ = os.Remove(filepath.Join(fixture, "terraform.tfstate"))
		_ = os.Remove(filepath.Join(fixture, "terraform.tfstate.backup"))
	})

	// The module has no providers, so this needs neither credentials nor
	// network access.
	run("init", "-backend=false", "-input=false", "-no-color")
	run("apply", "-auto-approve", "-input=false", "-no-color")

	// Naming a single output makes `-json` emit the bare value.
	raw := run("output", "-json", "-no-color", "golden")

	var value any
	require.NoError(t, json.Unmarshal(raw, &value))
	require.NotNil(t, value, "terraform produced no `golden` output:\n%s", raw)

	got, err := boundary.Canonicalise(value)
	require.NoError(t, err)

	goldenPath := filepath.Join("testdata", "tf_identifiers.golden.json")

	if *update {
		require.NoError(t, os.WriteFile(goldenPath, got, 0o644))
		t.Logf("wrote %s (%d bytes)", goldenPath, len(got))
		return
	}

	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	require.Equal(t, string(want), string(got),
		"%s is out of date with tf_identifiers; regenerate with: go test -tags tfgolden ./internal/boundary/ -update",
		goldenPath)
}
