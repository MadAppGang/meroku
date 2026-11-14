package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

// workload.efs[].read_only: the YAML -> main.tf hop, and what a save through the
// Workload struct does to a mount that sets it and to a mount that never did.
//
// The key becomes readOnly on the backend container's mount point
// (modules/workloads/backend.tf), and modules/workloads/variables.tf declares it
// optional(bool, false). So unlike a dropped autoscaling target this does not
// fail an apply or announce itself anywhere: the mount simply comes up
// read-write, which is the opposite of what the config asked for.
//
// The absent half matters as much as the set half. Rendering read_only = false
// for a mount that never had the key would put a line into the generated
// Terraform of every environment that mounts a filesystem, and a mount point
// change is a task-definition change: a diff nobody asked for on their next
// plan.

// efsMountWorkload returns project/dev.yaml's workload section with its efs
// mounts replaced by the given list.
//
// The shipped config mounts nothing, so this sets the whole list rather than
// editing an entry, and `{{#compare (len workload.efs) ">" 0}}` — which gates
// backend_efs_mounts — is satisfied by the overlay alone.
func efsMountWorkload(t *testing.T, mounts []interface{}) map[interface{}]interface{} {
	t.Helper()

	base, err := os.ReadFile(filepath.Join("..", "project", "dev.yaml"))
	if err != nil {
		t.Fatalf("reading project/dev.yaml: %v", err)
	}
	var doc map[interface{}]interface{}
	if err := yaml.Unmarshal(base, &doc); err != nil {
		t.Fatalf("parsing project/dev.yaml: %v", err)
	}
	workload, ok := doc["workload"].(map[interface{}]interface{})
	if !ok {
		t.Fatal("project/dev.yaml has no workload section")
	}
	workload["efs"] = mounts
	return workload
}

var (
	// The mount entries are flat object literals, so a brace pair is an entry.
	efsMountEntryRe = regexp.MustCompile(`\{[^{}]*\}`)
	efsMountNameRe  = regexp.MustCompile(`efs_name\s*=\s*"([^"]*)"`)
	efsReadOnlyRe   = regexp.MustCompile(`(?m)^\s*read_only\s*=\s*true\s*$`)
)

// efsMountEntries returns the rendered backend_efs_mounts entries, keyed by the
// filesystem they mount.
func efsMountEntries(t *testing.T, rendered string) map[string]string {
	t.Helper()

	list := argumentValue(t, workloadsModuleBlock(t, rendered), "backend_efs_mounts")
	entries := map[string]string{}
	for _, entry := range efsMountEntryRe.FindAllString(list, -1) {
		name := efsMountNameRe.FindStringSubmatch(entry)
		if name == nil {
			t.Fatalf("a backend_efs_mounts entry has no efs_name:\n%s", entry)
		}
		entries[name[1]] = entry
	}
	return entries
}

// assertEFSMountReadOnly checks both halves on one render: the mount that sets
// read_only renders it, and the mount that does not renders no read_only line at
// all. It also checks the render as a whole parses.
func assertEFSMountReadOnly(t *testing.T, rendered string) {
	t.Helper()

	if err := validateGeneratedHCL("env/dev/main.tf", rendered); err != nil {
		t.Fatalf("the rendered Terraform does not parse: %v", err)
	}
	entries := efsMountEntries(t, rendered)
	if len(entries) != 2 {
		t.Fatalf("want 2 entries in backend_efs_mounts, got %d: %v", len(entries), entries)
	}

	set := entries["uploads"]
	if !efsReadOnlyRe.MatchString(set) {
		t.Errorf("a mount that sets read_only: true must render read_only = true, got:\n%s", set)
	}
	if !strings.Contains(set, `mount_point = "/data"`) {
		t.Errorf("the mount's other keys moved while read_only was added:\n%s", set)
	}

	unset := entries["static"]
	if strings.Contains(unset, "read_only") {
		t.Errorf("a mount that never set read_only must render no read_only line, so an environment that mounts a filesystem sees no diff, got:\n%s", unset)
	}
}

func TestEFSMountReadOnlyRendersAndSurvivesASave(t *testing.T) {
	workload := efsMountWorkload(t, []interface{}{
		map[interface{}]interface{}{"name": "uploads", "mount_point": "/data", "read_only": true},
		map[interface{}]interface{}{"name": "static", "mount_point": "/static"},
	})

	assertEFSMountReadOnly(t, renderMainHBS(t, map[string]interface{}{"workload": workload}))

	// The same two mounts as saveEnvToFile writes them: unmarshalled into
	// Workload and marshalled straight back out. Rendering that rather than the
	// YAML above is what covers the struct half of the hop — a Workload with no
	// field for the key drops it here, and the second render then reports a
	// read-write mount.
	src, err := yaml.Marshal(workload)
	if err != nil {
		t.Fatalf("marshalling the workload section: %v", err)
	}
	assertEFSMountReadOnly(t, renderMainHBS(t, map[string]interface{}{"workload": roundTripWorkload(t, string(src))}))
}
