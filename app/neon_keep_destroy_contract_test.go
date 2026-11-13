package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
)

func qaNeonKeptEntries() []NeonKeptEntry {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return []NeonKeptEntry{
		{Kind: "project", ProjectID: "fake-project-000000", OrgID: "org-fake-000000", ReleasedAt: at},
		{Kind: "database", ProjectID: "fake-project-000000", BranchID: "br-fake-main", Database: "app", ReleasedAt: at},
		{Kind: "role", ProjectID: "fake-project-000000", BranchID: "br-fake-main", Database: "app", Role: "fake_backend", ReleasedAt: at},
	}
}

// REQ-10, REQ-31 / TEST-10.
func TestNeonContractKeptFilePermissionsMergingAndEnclosingReport(t *testing.T) {
	root := t.TempDir()
	entries := qaNeonKeptEntries()
	if err := recordNeonKept(root, "dev", entries); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".meroku", "neon-kept", "dev.json")
	for _, tc := range []struct {
		path string
		mode os.FileMode
	}{{path, 0600}, {filepath.Dir(path), 0700}} {
		st, err := os.Stat(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != tc.mode {
			t.Errorf("%s mode=%o want %o", tc.path, st.Mode().Perm(), tc.mode)
		}
	}
	later := append([]NeonKeptEntry(nil), entries...)
	for i := range later {
		later[i].ReleasedAt = later[i].ReleasedAt.Add(time.Hour)
	}
	if err := recordNeonKept(root, "dev", later); err != nil {
		t.Fatal(err)
	}
	got, err := loadNeonKept(root, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("deduplicated entries=%d want 3", len(got))
	}
	for _, e := range got {
		if !e.ReleasedAt.Equal(entries[0].ReleasedAt) {
			t.Error("first release timestamp lost")
		}
	}
	report := strings.Join(neonKeptReport(got), "\n")
	if !strings.Contains(report, "projects delete") || strings.Contains(report, "databases delete") || strings.Contains(report, "roles delete") {
		t.Fatalf("enclosing report=%s", report)
	}
	if !strings.Contains(report, neonCLI()+" projects delete") {
		t.Error("delete report must use the installed CLI name")
	}
	roleReport := strings.Join(neonKeptReport(entries[2:]), "\n")
	if !strings.Contains(roleReport, neonCLI()+" roles delete") || !strings.Contains(roleReport, "fake_backend") || !strings.Contains(roleReport, "br-fake-main") {
		t.Fatalf("role report=%s", roleReport)
	}
	if err := saveNeonKept(root, "dev", nil); err != nil {
		t.Fatal(err)
	}
	got, err = loadNeonKept(root, "dev")
	if err != nil || len(got) != 0 {
		t.Fatalf("forgotten entries=%v err=%v", got, err)
	}
}

// qaNeonKeptInaccessibleAPI makes every relevant project-scoped read fail.
// A successful empty ListRoles would instead prove that a kept role is gone.
type qaNeonKeptInaccessibleAPI struct{ neonAPI }

func (a qaNeonKeptInaccessibleAPI) ListDatabases(context.Context, string, string) ([]NeonDatabaseInfo, error) {
	return nil, ErrNeonNotFound
}

func (a qaNeonKeptInaccessibleAPI) ListRoles(context.Context, string, string) ([]NeonRoleInfo, error) {
	return nil, ErrNeonNotFound
}

// REQ-10 / TEST-10.
func TestNeonContractKeptPruningRequiresProofAndRecognisesReuse(t *testing.T) {
	entries := qaNeonKeptEntries()
	api := qaNeonAPIReady()
	api.projectErr = ErrNeonNotFound
	got := pruneNeonKept(context.Background(), qaNeonKeptInaccessibleAPI{api}, nil, entries)
	if len(got) != len(entries) {
		t.Fatal("an inaccessible project (404) must not prove data deleted")
	}
	// A successful roles listing that omits the role is positive proof;
	// the existing project and database must still be retained.
	got = pruneNeonKept(context.Background(), qaNeonAPIReady(), nil, entries)
	if len(got) != 2 {
		t.Fatalf("successful absence proof must remove only the role: %+v", got)
	}
	for _, e := range got {
		if e.Kind == "role" {
			t.Fatalf("role proven absent was retained: %+v", e)
		}
	}

	env := qaNeonEnv(t)
	got = pruneNeonKept(context.Background(), nil, &env, entries)
	for _, e := range got {
		if e.Kind == "project" || (e.Kind == "database" && e.Database == "app") {
			t.Fatalf("reused-as-data entry not pruned: %+v", e)
		}
	}
}

// REQ-10 / TEST-10: observable external Terraform state remains intact after a failed Kept write.
func TestNeonContractFailedKeptWriteDoesNotReleaseData(t *testing.T) {
	root := t.TempDir()
	qaNeonWrite(t, filepath.Join(root, ".meroku"), "not a directory")
	state := map[string]bool{neonProjectAddress: true}
	tf := func(_ context.Context, _ tfPurpose, _ string, args ...string) ([]byte, error) {
		if len(args) > 1 && args[0] == "state" && args[1] == "rm" {
			for _, a := range args[2:] {
				delete(state, a)
			}
			return nil, nil
		}
		return nil, fmt.Errorf("unsupported offline command %v", args)
	}
	err := neonRelease(context.Background(), &bytes.Buffer{}, tf, root, "dev", root, qaNeonKeptEntries(), []string{neonProjectAddress})
	if err == nil {
		t.Fatal("failed Kept write was ignored")
	}
	if !state[neonProjectAddress] {
		t.Fatal("data released despite failed Kept write")
	}
}

func TestNeonContractPreventDestroyDiagnosticNamesOnlyNeonData(t *testing.T) {
	// REQ-31 / TEST-10.
	for _, addr := range []string{neonProjectAddress, `module.neon.neon_database.created["app"]`} {
		output := "Error: Instance cannot be destroyed\nResource " + addr + " has lifecycle.prevent_destroy set, but the plan calls for this resource to be destroyed."
		got, ok := neonPreventDestroyError(output)
		if !ok || got != addr {
			t.Fatalf("diagnostic=(%q,%v) want %q", got, ok, addr)
		}
		msg := neonRefusalMessage(addr, "project/dev.yaml")
		if !strings.Contains(msg, "project/dev.yaml") || !strings.Contains(msg, addr) {
			t.Fatalf("refusal missing context: %s", msg)
		}
	}
	if _, ok := neonPreventDestroyError("Resource aws_s3_bucket.fake has lifecycle.prevent_destroy set"); ok {
		t.Error("unrelated AWS resource treated as Neon data")
	}
}

// REQ-26 / TEST-23: validate generated files as HCL, not formatting snapshots.
func TestNeonContractRetiredFilesPinProviderAndLeaveMainUntouched(t *testing.T) {
	dir := t.TempDir()
	main := []byte("terraform {}\nmodule \"workloads\" { source = \"./fake\" }\n")
	qaNeonWrite(t, filepath.Join(dir, "main.tf"), string(main))
	retired := []NeonConnectionState{{Alias: "neon_app_deadbe", Database: "app", ProjectID: "fake-project-000000", BranchID: "br-fake-main", AdminRole: "neondb_owner"}}
	remove, err := writeNeonRetiredFiles(dir, retired, []string{"/dev/myapp/backend/database_url"}, main)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(remove)
	parser := hclparse.NewParser()
	parse := func(name string) *hcl.File {
		t.Helper()
		file, diags := parser.ParseHCLFile(filepath.Join(dir, name))
		if diags.HasErrors() {
			t.Fatal(diags.Error())
		}
		return file
	}
	retiredFile := parse("neon_retired.tf")
	schema := &hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{{Type: "variable", LabelNames: []string{"name"}}, {Type: "provider", LabelNames: []string{"name"}}}}
	body, diags := retiredFile.Body.Content(schema)
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}
	foundProvider, foundVariable := false, false
	for _, block := range body.Blocks {
		attrs, diags := block.Body.JustAttributes()
		if diags.HasErrors() {
			t.Fatal(diags.Error())
		}
		if block.Type == "variable" && block.Labels[0] == "neon_pg_credentials" {
			foundVariable = true
			for _, k := range []string{"ephemeral", "sensitive"} {
				a := attrs[k]
				if a == nil {
					t.Fatalf("credentials variable lacks %s", k)
				}
				v, d := a.Expr.Value(nil)
				if d.HasErrors() || !v.RawEquals(cty.True) {
					t.Fatalf("%s must be true", k)
				}
			}
		}
		if block.Type == "provider" {
			foundProvider = true
			a := attrs["alias"]
			if a == nil {
				t.Fatal("missing alias")
			}
			v, d := a.Expr.Value(nil)
			if d.HasErrors() || !v.RawEquals(cty.StringVal("neon_app_deadbe")) {
				t.Fatal("provider alias is not a quoted matching literal")
			}
		}
	}
	if !foundProvider || !foundVariable {
		t.Fatal("retired provider/ephemeral variable missing")
	}
	parse("neon_retired_override.tf")
	override, err := os.ReadFile(filepath.Join(dir, "neon_retired_override.tf"))
	if err != nil {
		t.Fatal(err)
	}
	// Exact provider metadata are part of the public format. Whitespace is not.
	text := strings.Join(strings.Fields(string(override)), " ")
	for _, s := range []string{`source = "cyrilgdn/postgresql"`, `version = "1.27.0"`, "ssm_exclude_names", "/dev/myapp/backend/database_url"} {
		if !strings.Contains(text, s) {
			t.Errorf("override missing %q: %s", s, text)
		}
	}
	if strings.Contains(string(override), "~>") {
		t.Error("retired provider must be pinned exactly")
	}
	actual, err := os.ReadFile(filepath.Join(dir, "main.tf"))
	if err != nil || !bytes.Equal(actual, main) {
		t.Fatal("main.tf changed")
	}
	remove()
	remove()
	for _, name := range []string{"neon_retired.tf", "neon_retired_override.tf"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s survived cleanup: %v", name, err)
		}
	}
}

// REQ-9, REQ-23 / TEST-9: in-memory state acts as the external Terraform backend.
func TestNeonContractDestroyPrepKeepsDataAndStopsOnBranchDrift(t *testing.T) {
	for _, tc := range []struct {
		name            string
		degraded, drift bool
	}{
		{"normal", false, false}, {"degraded", true, false}, {"branch drift", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "env", "dev")
			qaNeonWrite(t, filepath.Join(dir, "main.tf"), "terraform {}\n")
			// An initialized AWS-only root models removal of neon: followed by generate.
			if err := os.MkdirAll(filepath.Join(dir, ".terraform", "modules"), 0700); err != nil {
				t.Fatal(err)
			}
			qaNeonWrite(t, filepath.Join(dir, ".terraform", "modules", "modules.json"), `{"Modules":[{"Key":"","Source":"","Dir":"."}]}`)
			pid := "fake-project-000000"
			branch := "br-fake-main"
			if tc.drift {
				branch = "br-fake-old"
			}
			alias := "neon_app_deadbe"
			prefix := "module.neon_access_" + alias + "."
			resources := map[string]map[string]interface{}{
				neonProjectAddress:                         {"id": pid, "name": "fake-project", "org_id": "org-fake-000000", "default_branch_id": branch},
				`module.neon.neon_database.created["app"]`: {"name": "app", "project_id": pid, "branch_id": branch, "owner_name": "neondb_owner"},
				prefix + "terraform_data.connection":       {"input": map[string]interface{}{"database": "app", "project_id": pid, "branch_id": branch, "admin_role": "neondb_owner", "roles": map[string]string{"backend": "fake_backend"}}},
				prefix + `postgresql_schema.this["extra"]`: {"name": "extra"},
				prefix + `postgresql_role.svc["backend"]`:  {"name": "fake_backend"},
			}
			state := func() string {
				var neon, access []interface{}
				for addr, values := range resources {
					tail := strings.TrimPrefix(addr, prefix)
					kind := strings.Split(tail, ".")[0]
					if strings.HasPrefix(addr, "module.neon.") {
						tail = strings.TrimPrefix(addr, "module.neon.")
						kind = strings.Split(tail, ".")[0]
					}
					resource := map[string]interface{}{"address": addr, "mode": "managed", "type": kind, "name": strings.Split(strings.Split(tail, ".")[1], "[")[0], "values": values}
					if strings.Contains(addr, "neon_database") {
						resource["index"] = "app"
					}
					if strings.HasPrefix(addr, "module.neon.") {
						neon = append(neon, resource)
					} else {
						access = append(access, resource)
					}
				}
				return qaNeonJSON(t, map[string]interface{}{"values": map[string]interface{}{"root_module": map[string]interface{}{"child_modules": []interface{}{map[string]interface{}{"address": "module.neon", "resources": neon}, map[string]interface{}{"address": "module.neon_access_" + alias, "resources": access}}}}})
			}
			tf := func(_ context.Context, _ tfPurpose, _ string, args ...string) ([]byte, error) {
				switch {
				case len(args) >= 2 && args[0] == "state" && args[1] == "list":
					keys := []string{}
					for a := range resources {
						keys = append(keys, a)
					}
					sort.Strings(keys)
					return []byte(strings.Join(keys, "\n")), nil
				case len(args) >= 2 && args[0] == "show" && args[1] == "-json":
					return []byte(state()), nil
				case len(args) >= 2 && args[0] == "state" && args[1] == "rm":
					for _, a := range args[2:] {
						delete(resources, a)
					}
					return nil, nil
				case len(args) > 0 && args[0] == "init":
					return nil, nil
				default:
					return nil, fmt.Errorf("offline state backend cannot execute %v", args)
				}
			}
			old := currentNeonSession()
			t.Cleanup(func() { setActiveNeonSession(old) })
			var sess *neonSession
			var end func()
			if tc.degraded {
				sess, end = beginDegradedNeonSession()
			} else {
				sess, end = beginNeonSession(NeonToken{Value: "fake-destroy-token-do-not-use", Source: neonTokenFromEnv}, nil)
			}
			t.Cleanup(end)
			target := handoffTarget{ProjectRoot: root, EnvName: "dev"}
			original := qaNeonJSON(t, resources)
			_, cleanup, err := neonDestroyPrep(context.Background(), &bytes.Buffer{}, target, tf, qaNeonAPIReady(), sess)
			if cleanup != nil {
				t.Cleanup(cleanup)
			}
			if tc.drift {
				if err == nil || !strings.Contains(err.Error(), "V-N39") {
					t.Fatalf("branch drift error=%v", err)
				}
				if qaNeonJSON(t, resources) != original {
					t.Fatal("branch drift changed external state")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for addr := range resources {
				if strings.HasPrefix(addr, "module.neon.") || strings.Contains(addr, "postgresql_schema") || tc.degraded {
					t.Errorf("data/schema/degraded-role address not released: %s", addr)
				}
			}
			if !tc.degraded && len(resources) != 2 {
				t.Fatalf("normal destroy prep must retain role/connection for Terraform teardown: %v", resources)
			}
			entries, err := loadNeonKept(root, "dev")
			if err != nil {
				t.Fatal(err)
			}
			kinds := map[string]int{}
			for _, e := range entries {
				kinds[e.Kind]++
			}
			if kinds["project"] != 1 || kinds["database"] != 1 || (tc.degraded && kinds["role"] != 1) {
				t.Fatalf("Kept data incomplete: %+v", entries)
			}
			// Lock-release restarts must not lose earlier Kept entries.
			_, again, err := neonDestroyPrep(context.Background(), &bytes.Buffer{}, target, tf, qaNeonAPIReady(), sess)
			if again != nil {
				t.Cleanup(again)
			}
			if err != nil {
				t.Fatal(err)
			}
			after, err := loadNeonKept(root, "dev")
			if err != nil {
				t.Fatal(err)
			}
			if len(after) != len(entries) {
				t.Fatal("repeat prep changed Kept entries")
			}
		})
	}
}
