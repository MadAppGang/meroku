// Package contract holds the only two identifier strings that must agree across
// the Terraform <-> Go boundary.
//
// It deliberately contains no name *formats* (ECR repository names, SSM
// parameter paths, ECS service names). Those are produced by the Terraform
// resources that actually create the objects, and reach the Lambda as lookup
// maps in environment variables. The Lambda never derives an identifier from a
// repository name or a parameter path — it only ever looks one up. An unmapped
// key is ignored, never guessed.
//
// contract.json is read by Terraform with jsondecode(file(...)) and embedded
// here, so a change on one side is a change on both.
package contract

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

//go:embed contract.json
var contractJSON []byte

// Spec is the decoded contract.
type Spec struct {
	backendID    string
	taskIDPrefix string
}

type rawSpec struct {
	BackendID    string `json:"backend_id"`
	TaskIDPrefix string `json:"task_id_prefix"`
}

var (
	once   sync.Once
	loaded Spec
)

// Load decodes the embedded contract. The data is compiled into the binary, so
// a malformed or incomplete contract is a build defect and panics rather than
// producing a half-configured Lambda.
func Load() Spec {
	once.Do(func() {
		var raw rawSpec
		if err := json.Unmarshal(contractJSON, &raw); err != nil {
			panic(fmt.Sprintf("contract: malformed contract.json: %v", err))
		}
		if raw.BackendID == "" {
			panic("contract: backend_id must not be empty")
		}
		if raw.TaskIDPrefix == "" {
			panic("contract: task_id_prefix must not be empty")
		}
		loaded = Spec{backendID: raw.BackendID, taskIDPrefix: raw.TaskIDPrefix}
	})
	return loaded
}

// BackendID is the identifier of the main backend service. It is a real string
// ("backend"), never an empty-string sentinel.
func (s Spec) BackendID() string { return s.backendID }

// TaskIDPrefix is the prefix that distinguishes a scheduled-task identifier
// from an ECS service identifier.
func (s Spec) TaskIDPrefix() string { return s.taskIDPrefix }

// TaskID returns the identifier for a scheduled task.
func (s Spec) TaskID(name string) string { return s.taskIDPrefix + name }

// IsTaskID reports whether id is a scheduled-task identifier.
//
// This is a display/validation convenience only. Whether a target is deployed
// as a service or as a task-definition revision is decided by the "type" field
// Terraform puts in the target map, not by string inspection.
func (s Spec) IsTaskID(id string) bool { return strings.HasPrefix(id, s.taskIDPrefix) }
