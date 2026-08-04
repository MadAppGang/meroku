package contract_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"madappgang.com/infrastructure/ci_lambda/contract"
)

func TestLoadValues(t *testing.T) {
	s := contract.Load()

	require.Equal(t, "backend", s.BackendID(), "the backend identifier is the literal string \"backend\" everywhere")
	require.Equal(t, "task:", s.TaskIDPrefix())
	require.Equal(t, "task:cleanup", s.TaskID("cleanup"))
	require.Equal(t, "task:nightly-report", s.TaskID("nightly-report"))
}

func TestIsTaskID(t *testing.T) {
	s := contract.Load()

	require.True(t, s.IsTaskID("task:cleanup"))
	require.False(t, s.IsTaskID("backend"))
	require.False(t, s.IsTaskID("payment-worker"))
	require.False(t, s.IsTaskID("tasks"))
}

// TestContractJSONHasExactlyTheAgreedKeys pins the file shape. Terraform reads
// the same file with jsondecode(); adding a key here without adding it there
// (or vice versa) is the class of drift this file exists to prevent.
func TestContractJSONHasExactlyTheAgreedKeys(t *testing.T) {
	b, err := os.ReadFile("contract.json")
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	require.ElementsMatch(t, []string{"backend_id", "task_id_prefix"}, keys,
		"contract.json carries identifiers only — name formats belong to the Terraform resources that create the objects")
}
