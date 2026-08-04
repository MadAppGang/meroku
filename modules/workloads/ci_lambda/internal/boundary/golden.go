// Package boundary holds the Terraform <-> Go boundary checks.
//
// The mechanism is a committed golden file: `terraform apply` on the synthetic
// project in testdata/tfgolden produces the exact maps Terraform ships to the
// Lambda, and an always-on Go test feeds those maps to the real config loader
// and the real resolvers. No terraform binary is needed to run the check, so it
// cannot silently skip; regenerating the golden file needs terraform, and drift
// between the file and the module is a hard failure.
package boundary

import (
	"encoding/json"
	"fmt"
	"os"
)

// GoldenPath is the committed capture of Terraform's output.
const GoldenPath = "testdata/tf_identifiers.golden.json"

// Golden is the whole captured payload.
type Golden struct {
	Identifiers Identifiers       `json:"identifiers"`
	Inputs      Inputs            `json:"inputs"`
	Env         map[string]string `json:"env"`
}

// Identifiers is what the tf_identifiers module computes.
type Identifiers struct {
	BackendID    string              `json:"backend_id"`
	ServiceIDs   map[string]string   `json:"service_ids"`
	TaskIDs      map[string]string   `json:"task_ids"`
	ECRRepoIDs   map[string][]string `json:"ecr_repo_ids"`
	SSMPrefixIDs map[string]string   `json:"ssm_prefix_ids"`
	AutoDeploy   map[string]bool     `json:"auto_deploy"`
}

// Inputs are the fixture's own arguments, captured alongside the result.
//
// Without them a test asserting anything about a disabled target would have to
// name that target itself, and would then keep passing after the fixture stopped
// containing one. Capturing what Terraform was *given* lets the assertions be
// derived rather than restated.
type Inputs struct {
	Project           string          `json:"project"`
	BackendAutoDeploy bool            `json:"backend_auto_deploy"`
	ServiceAutoDeploy map[string]bool `json:"service_auto_deploy"`
	TaskAutoDeploy    map[string]bool `json:"task_auto_deploy"`
}

// LoadGolden reads the committed golden file.
func LoadGolden(path string) (*Golden, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var g Golden
	if err := json.Unmarshal(raw, &g); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &g, nil
}

// Canonicalise renders a decoded payload deterministically, so a regenerated
// capture can be byte-compared against the committed one.
func Canonicalise(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
