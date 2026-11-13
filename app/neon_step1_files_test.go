package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A connection that left the YAML still has objects in state. Every plan,
// step 1's targeted one included, needs its provider configuration, so the
// files that carry it must exist before step 1 runs.
func TestNeonLeavingConnectionsAreTheStateConnectionsTheYAMLDropped(t *testing.T) {
	env := Env{Project: "myapp", Env: "dev"}
	env.Neon = &Neon{Mode: NeonModeCreate, Databases: []NeonDatabase{{Name: "app", Create: true}}}
	env.Workload.BackendNeon = &NeonAccess{Database: "app", Access: "readwrite"}

	d, err := neonDesiredFromEnv(env)
	if err != nil {
		t.Fatal(err)
	}
	var kept string
	for alias := range d.connections {
		kept = alias
	}
	st := NeonStateView{Connections: map[string]NeonConnectionState{
		kept:               {Alias: kept, Database: "app"},
		"neon_app2_ec8d83": {Alias: "neon_app2_ec8d83", Database: "app2"},
		"neon_gone_000000": {Alias: "neon_gone_000000"},
	}}

	got, err := neonLeavingConnections(env, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Alias != "neon_app2_ec8d83" {
		t.Fatalf("leaving = %+v, want only neon_app2_ec8d83 (the kept one is in the YAML, the other names no database)", got)
	}
}

func TestNeonLeavingConnectionsWithNeonRemovedFromTheYAML(t *testing.T) {
	st := NeonStateView{Connections: map[string]NeonConnectionState{
		"neon_app_d2a57d": {Alias: "neon_app_d2a57d", Database: "app"},
	}}
	got, err := neonLeavingConnections(Env{Project: "myapp", Env: "dev"}, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("leaving = %+v, want the one connection", got)
	}
}

func TestNeonRetiredFilesCarryTheLeavingProvider(t *testing.T) {
	dir := t.TempDir()
	remove, err := writeNeonRetiredFiles(dir, []NeonConnectionState{{Alias: "neon_app2_ec8d83", Database: "app2"}}, nil, []byte("module \"neon\" {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	defer remove()
	b, err := os.ReadFile(filepath.Join(dir, neonRetiredFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `alias            = "neon_app2_ec8d83"`) {
		t.Fatalf("%s lacks the provider alias:\n%s", neonRetiredFile, b)
	}
}
