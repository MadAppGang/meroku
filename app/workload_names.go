package main

import (
	"fmt"
	"sort"
	"strings"
)

// The workload namespace.
//
// The backend, every service, every scheduled task and every event task is a
// workload, and a workload's name is used as a key in places that do not know its
// kind: its folder in the data bucket (<name>/), the for_each keys of the
// capability modules (local.workload_roles in env/main.hbs), and the references
// other YAML fields make to it (data_bucket.shared[].read_write, ...). So names
// are unique across all kinds, and a few are reserved:
//
//   - backend: the backend's own name everywhere above.
//   - task:    scheduled and event tasks keep their SSM parameters under
//     /<env>/<project>/task/<name>. A service reads its path
//     /<env>/<project>/<service> recursively, so a service named "task" would read
//     every task's parameters as its own secrets.
//   - vault:   the same, for the vault paths /<env>/<project>/vault/<name>.
//   - shared:  the data bucket's shared folders live under shared/, and a
//     workload named "shared" would own them all as its own folder.

// workloadNamesKey is the envMap key holding every workload name, backend first,
// then services, scheduled tasks and event tasks in YAML order. Written by
// validateWorkloadNamesConfigMap.
const workloadNamesKey = "_workload_names"

// disabledWorkloadNamesKey is the envMap key holding the names of the services
// and tasks that are switched off (enabled: false). Written by
// collectDisabledWorkloadNames, before filterDisabledItems removes them.
const disabledWorkloadNamesKey = "_disabled_workload_names"

// backendWorkloadName is the name the backend has as a workload.
const backendWorkloadName = "backend"

// reservedWorkloadNames are the names no service or task may take.
var reservedWorkloadNames = []string{backendWorkloadName, "vault", "task", "shared"}

// workloadLists are the YAML lists whose items are workloads, in the order their
// names are listed.
var workloadLists = []string{"services", "scheduled_tasks", "event_processor_tasks"}

// collectDisabledWorkloadNames records the names of the services and tasks with
// enabled: false. It must run before filterDisabledItems, which removes them, so
// that a reference to a switched-off workload can be dropped with a warning
// instead of being refused as a name that does not exist.
func collectDisabledWorkloadNames(envMap map[string]interface{}) {
	var names []interface{}
	for _, key := range workloadLists {
		items, _ := envMap[key].([]interface{})
		for _, raw := range items {
			item, ok := computeNodeMap(raw)
			if !ok {
				continue
			}
			if enabled, has := item["enabled"]; has && enabled == false {
				if name := stringFromMap(item, "name"); name != "" {
					names = append(names, name)
				}
			}
		}
	}
	envMap[disabledWorkloadNamesKey] = names
}

// validateWorkloadNamesConfigMap refuses a service or task named like another
// workload of any kind, or with a reserved name, and records every workload name
// under workloadNamesKey for the validators that resolve references.
func validateWorkloadNamesConfigMap(envMap map[string]interface{}) error {
	names := []interface{}{backendWorkloadName}
	seen := map[string]string{backendWorkloadName: "the backend"}
	var problems []string

	for _, key := range workloadLists {
		items, _ := envMap[key].([]interface{})
		for i, raw := range items {
			item, ok := computeNodeMap(raw)
			if !ok {
				continue
			}
			name := stringFromMap(item, "name")
			if name == "" {
				continue
			}
			where := fmt.Sprintf("%s[%d] '%s'", key, i, name)
			if isReservedWorkloadName(name) {
				problems = append(problems, fmt.Sprintf(
					"%s: the name %q is reserved; rename the workload", where, name))
				continue
			}
			if other, dup := seen[name]; dup {
				problems = append(problems, fmt.Sprintf(
					"%s: %s already has this name; workload names are unique across services, scheduled tasks and event tasks, so rename one of them", where, other))
				continue
			}
			seen[name] = where
			names = append(names, name)
		}
	}

	envMap[workloadNamesKey] = names

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf(
		"workload names: %s are reserved, and every name is used as a key for the workload's SSM path and its data bucket folder.\n  %s",
		strings.Join(quoteAll(reservedWorkloadNames), ", "), strings.Join(problems, "\n  "))
}

func isReservedWorkloadName(name string) bool {
	for _, r := range reservedWorkloadNames {
		if name == r {
			return true
		}
	}
	return false
}

// workloadNameSet returns the names written by validateWorkloadNamesConfigMap.
func workloadNameSet(envMap map[string]interface{}) map[string]bool {
	return stringSetFromEnvMap(envMap, workloadNamesKey)
}

// disabledWorkloadNameSet returns the names written by
// collectDisabledWorkloadNames.
func disabledWorkloadNameSet(envMap map[string]interface{}) map[string]bool {
	return stringSetFromEnvMap(envMap, disabledWorkloadNamesKey)
}

func stringSetFromEnvMap(envMap map[string]interface{}, key string) map[string]bool {
	out := map[string]bool{}
	items, _ := envMap[key].([]interface{})
	for _, raw := range items {
		if s, ok := raw.(string); ok {
			out[s] = true
		}
	}
	return out
}

// workloadReference is how a YAML field that names a workload resolves.
type workloadReference int

const (
	workloadReferenceKnown    workloadReference = iota // a deployed workload
	workloadReferenceDisabled                          // switched off: drop it, with a warning
	workloadReferenceUnknown                           // names nothing: an error
)

// resolveWorkloadReference classifies a reference to a workload by name.
func resolveWorkloadReference(envMap map[string]interface{}, name string) workloadReference {
	if workloadNameSet(envMap)[name] {
		return workloadReferenceKnown
	}
	if disabledWorkloadNameSet(envMap)[name] {
		return workloadReferenceDisabled
	}
	return workloadReferenceUnknown
}

// generateWarningsKey is the envMap key holding warnings found while
// preprocessing. applyTemplate prints them; render tests read them.
const generateWarningsKey = "_generate_warnings"

// addGenerateWarning records a warning for applyTemplate to print.
func addGenerateWarning(envMap map[string]interface{}, warning string) {
	existing, _ := envMap[generateWarningsKey].([]string)
	envMap[generateWarningsKey] = append(existing, warning)
}

// generateWarnings returns the warnings recorded while preprocessing, sorted.
func generateWarnings(envMap map[string]interface{}) []string {
	warnings, _ := envMap[generateWarningsKey].([]string)
	out := append([]string(nil), warnings...)
	sort.Strings(out)
	return out
}
