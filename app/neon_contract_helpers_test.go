package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v2"
)

// Synthetic fixtures only. These helpers operate at the supplied file/API seams.
func qaNeonWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func qaNeonMap(t *testing.T, text string) map[string]interface{} {
	t.Helper()
	var v map[string]interface{}
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func qaNeonEnv(t *testing.T) Env {
	t.Helper()
	var env Env
	if err := yaml.Unmarshal([]byte(`project: myapp
env: dev
neon:
  mode: reuse
  project_id: fake-project-000000
  pg_version: 17
  owner_role: neondb_owner
  databases:
    - name: app
      create: false
workload:
  backend_neon:
    database: app
    access: readwrite
`), &env); err != nil {
		t.Fatal(err)
	}
	return env
}

func qaNeonJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

type qaNeonAPI struct {
	project    NeonProjectInfo
	branches   []NeonBranchInfo
	databases  []NeonDatabaseInfo
	roles      []NeonRoleInfo
	projectErr error
}

func qaNeonAPIReady() *qaNeonAPI {
	return &qaNeonAPI{
		project:   NeonProjectInfo{ID: "fake-project-000000", Name: "fake-project", OrgID: "org-fake-000000", RegionID: "aws-us-east-1", PGVersion: 17, DefaultBranchID: "br-fake-main"},
		branches:  []NeonBranchInfo{{ID: "br-fake-main", Name: "main", Default: true}},
		databases: []NeonDatabaseInfo{{Name: "app", OwnerName: "neondb_owner", BranchID: "br-fake-main"}},
		roles:     []NeonRoleInfo{{Name: "neondb_owner"}},
	}
}
func (a *qaNeonAPI) GetProject(context.Context, string) (NeonProjectInfo, error) {
	return a.project, a.projectErr
}
func (a *qaNeonAPI) ListProjects(context.Context, string) ([]NeonProjectInfo, error) {
	if a.projectErr != nil {
		return nil, a.projectErr
	}
	return []NeonProjectInfo{a.project}, nil
}
func (a *qaNeonAPI) ListOrganizations(context.Context) ([]NeonOrg, error) {
	return []NeonOrg{{ID: "org-fake-000000", Name: "Fake organization"}}, nil
}
func (a *qaNeonAPI) ListBranches(context.Context, string) ([]NeonBranchInfo, error) {
	return a.branches, nil
}
func (a *qaNeonAPI) ListEndpoints(context.Context, string, string) ([]NeonEndpointInfo, error) {
	return []NeonEndpointInfo{{ID: "ep-fake", BranchID: "br-fake-main", Host: "ep-fake.example.invalid", Type: "read_write"}}, nil
}
func (a *qaNeonAPI) ListDatabases(context.Context, string, string) ([]NeonDatabaseInfo, error) {
	return a.databases, nil
}
func (a *qaNeonAPI) ListRoles(context.Context, string, string) ([]NeonRoleInfo, error) {
	return a.roles, nil
}
func (a *qaNeonAPI) RevealRolePassword(context.Context, string, string, string) (string, error) {
	return "fake-owner-password-do-not-use", nil
}

func qaNeonState() NeonStateView {
	return NeonStateView{
		Managed: map[string]bool{}, SSMServices: map[string]bool{},
		Created:     NeonCreatedSet{Databases: map[string]NeonCreatedDatabase{}},
		Connections: map[string]NeonConnectionState{},
	}
}
