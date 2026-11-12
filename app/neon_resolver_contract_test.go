package main

import (
	"context"
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func qaNeonResolve(t *testing.T, env Env, st NeonStateView, api neonAPI, ssm ssmLister, phase neonPhase) NeonResolution {
	t.Helper()
	r, err := resolveNeon(context.Background(), api, ssm, env, st, phase)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func qaNeonProblem(t *testing.T, r NeonResolution, rule string) {
	t.Helper()
	if !strings.Contains(strings.Join(r.Problems, "\n"), "["+rule+"]") {
		t.Fatalf("problems=%v want %s", r.Problems, rule)
	}
	if len(r.Releases) != 0 || len(r.KeptAdd) != 0 || len(r.OutOfBandReleases) != 0 {
		t.Fatal("failed checks must not release or classify data")
	}
}

// REQ-21 / TEST-18.
func TestNeonContractSSMClashesUseCaseInsensitiveLastSegment(t *testing.T) {
	for _, tc := range []struct {
		name, path         string
		owned, skip, clash bool
	}{
		{"unowned lowercase", "/dev/myapp/backend/database_url", false, false, true},
		{"owned lowercase", "/dev/myapp/backend/database_url", true, false, false},
		{"owned uppercase", "/dev/myapp/backend/DATABASE_URL", true, false, true},
		{"nested lowercase", "/dev/myapp/backend/nested/database_url", true, false, true},
		{"mixed case", "/dev/myapp/backend/Database_Url", false, false, true},
		{"unrelated", "/dev/myapp/backend/database_password", false, false, false},
		{"no AWS skips", "/dev/myapp/backend/DATABASE_URL", false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := qaNeonState()
			st.SSMServices["backend"] = tc.owned
			var list ssmLister = func(context.Context, string) ([]string, error) { return []string{tc.path}, nil }
			if tc.skip {
				list = nil
			}
			r := qaNeonResolve(t, qaNeonEnv(t), st, qaNeonAPIReady(), list, neonPhasePre)
			if tc.clash {
				qaNeonProblem(t, r, "V-N26")
			} else if len(r.Problems) != 0 {
				t.Fatalf("safe SSM names refused: %v", r.Problems)
			}
		})
	}
}

// REQ-22 / TEST-19.
func TestNeonContractProjectIdentityDriftStopsBeforeDataRelease(t *testing.T) {
	for _, tc := range []string{"connection", "created project", "created database", "reuse to create"} {
		t.Run(tc, func(t *testing.T) {
			env := qaNeonEnv(t)
			st := qaNeonState()
			old := "fake-old-project-000000"
			switch tc {
			case "connection":
				st.Connections["old"] = NeonConnectionState{Alias: "old", ProjectID: old, BranchID: "br-fake-main", Database: "app", AdminRole: "neondb_owner"}
			case "created project":
				st.Created.Project = &NeonCreatedProject{ID: old, DefaultBranchID: "br-fake-main"}
			case "created database":
				st.Created.Databases["app"] = NeonCreatedDatabase{Name: "app", ProjectID: old, BranchID: "br-fake-main"}
			case "reuse to create":
				env.Neon = &Neon{Mode: "create", Project: &NeonProject{Name: "fake-new-project", OrgID: "org-fake-000000", RegionID: "aws-us-east-1", PGVersion: 17}}
				st.Connections["old"] = NeonConnectionState{Alias: "old", ProjectID: old, BranchID: "br-fake-main", Database: "app", AdminRole: "neondb_owner"}
			}
			st.ProjectID = old
			st.Managed[neonProjectAddress] = true
			r := qaNeonResolve(t, env, st, qaNeonAPIReady(), nil, neonPhasePre)
			qaNeonProblem(t, r, "V-N38")
		})
	}
}

// REQ-23 / TEST-20.
func TestNeonContractDefaultBranchDriftRefusesCreateAndReuse(t *testing.T) {
	for _, mode := range []string{"reuse", "create"} {
		t.Run(mode, func(t *testing.T) {
			env := qaNeonEnv(t)
			st := qaNeonState()
			pid := env.Neon.ProjectID
			st.ProjectID = pid
			st.Connections["old"] = NeonConnectionState{Alias: "old", ProjectID: pid, BranchID: "br-fake-old", Database: "app", AdminRole: "neondb_owner"}
			st.Managed[`module.neon_access_old.postgresql_role.svc["backend"]`] = true
			if mode == "create" {
				env.Neon = &Neon{Mode: "create", Project: &NeonProject{Name: "fake-project", OrgID: "org-fake-000000", RegionID: "aws-us-east-1", PGVersion: 17}}
				st.Created.Project = &NeonCreatedProject{ID: pid, OrgID: "org-fake-000000", DefaultBranchID: "br-fake-old"}
			}
			r := qaNeonResolve(t, env, st, qaNeonAPIReady(), nil, neonPhasePre)
			qaNeonProblem(t, r, "V-N39")
			for _, s := range []string{"br-fake-main", "br-fake-old", "set-default"} {
				if !strings.Contains(strings.Join(r.Problems, " "), s) {
					t.Errorf("branch remedy lacks %q", s)
				}
			}
		})
	}
}

// REQ-8 / TEST-7.
func TestNeonContractCreateReuseExistenceMatrix(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		create, inState, exists bool
		rule                    string
		outOfBand               bool
	}{
		{"new create", true, false, false, "", false},
		{"refuse adoption", true, false, true, "V-N20", false},
		{"managed exists", true, true, true, "", false},
		{"managed externally deleted", true, true, false, "", true},
		{"reuse exists", false, false, true, "", false},
		{"reuse missing", false, false, false, "V-N21", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := qaNeonEnv(t)
			env.Neon.Databases[0].Create = tc.create
			// REQ-8 / TEST-7: the managed schema must still be desired.
			// Otherwise the independent schema-removal rule correctly releases it.
			env.Workload.BackendNeon.Schemas = []string{"app", "public"}
			st := qaNeonState()
			api := qaNeonAPIReady()
			addr := fmt.Sprintf(neonDatabaseAddressFmt, "app")
			if tc.inState {
				st.ProjectID = env.Neon.ProjectID
				st.Created.Databases["app"] = NeonCreatedDatabase{Name: "app", ProjectID: env.Neon.ProjectID, BranchID: "br-fake-main", OwnerName: "neondb_owner"}
				st.Managed[addr] = true
				for _, a := range []string{`postgresql_schema.this["app"]`, `postgresql_grant.this["backend|app|table"]`, `postgresql_default_privileges.this["backend|app|table|default"]`, `postgresql_role.svc["backend"]`, `random_password.svc["backend"]`, "terraform_data.connection"} {
					st.Managed["module.neon_access_"+neonConnectionAlias("app")+"."+a] = true
				}
			}
			if !tc.exists {
				api.databases = nil
			}
			r := qaNeonResolve(t, env, st, api, nil, neonPhasePre)
			if tc.rule != "" {
				qaNeonProblem(t, r, tc.rule)
				return
			}
			if len(r.Problems) != 0 {
				t.Fatalf("unexpected problems %v", r.Problems)
			}
			if (len(r.OutOfBand) > 0) != tc.outOfBand {
				t.Fatalf("out-of-band=%v want %v", r.OutOfBand, tc.outOfBand)
			}
			if tc.outOfBand {
				if len(r.OutOfBandReleases) != 4 {
					t.Fatalf("expected database/schema/grant/default-privilege releases, got %v", r.OutOfBandReleases)
				}
				for _, a := range r.OutOfBandReleases {
					if strings.Contains(a, "postgresql_role") || strings.Contains(a, "random_password") || strings.Contains(a, "terraform_data") || strings.Contains(a, "neon_ssm") {
						t.Errorf("persistent identity wrongly released: %s", a)
					}
				}
			} else if len(r.Releases) != 0 || len(r.OutOfBandReleases) != 0 {
				t.Fatalf("unchanged/new data unexpectedly released: %+v", r)
			}
		})
	}
}

// REQ-8 / TEST-8.
func TestNeonContractRemovingNeonKeepsDataButNotSSMState(t *testing.T) {
	var env Env
	if err := yaml.Unmarshal([]byte("project: myapp\nenv: dev\n"), &env); err != nil {
		t.Fatal(err)
	}
	st := qaNeonState()
	pid := "fake-project-000000"
	st.ProjectID = pid
	st.Created.Project = &NeonCreatedProject{ID: pid, OrgID: "org-fake-000000", DefaultBranchID: "br-fake-main"}
	st.Created.Databases["app"] = NeonCreatedDatabase{Name: "app", ProjectID: pid, BranchID: "br-fake-main"}
	st.Managed[neonProjectAddress] = true
	st.Managed[fmt.Sprintf(neonDatabaseAddressFmt, "app")] = true
	st.SSMServices["backend"] = true
	r := qaNeonResolve(t, env, st, qaNeonAPIReady(), nil, neonPhasePre)
	if len(r.Problems) != 0 || len(r.Releases) != 2 || len(r.KeptAdd) != 2 {
		t.Fatalf("removal=%+v", r)
	}
	for _, addr := range r.Releases {
		if !strings.HasPrefix(addr, "module.neon.") {
			t.Errorf("released non-data address %s", addr)
		}
	}
	post := qaNeonResolve(t, env, st, qaNeonAPIReady(), nil, neonPhasePost)
	if !reflect.DeepEqual(post.SSMExclude, []string{"/dev/myapp/backend/database_url"}) {
		t.Fatalf("removed SSM exclusions=%v", post.SSMExclude)
	}
}

func TestNeonContractSameProjectCreateToReuseReleasesOnlyProject(t *testing.T) {
	// REQ-8 / TEST-8.
	env := qaNeonEnv(t)
	env.Neon.Databases[0].Create = true
	st := qaNeonState()
	pid := env.Neon.ProjectID
	st.ProjectID = pid
	st.Created.Project = &NeonCreatedProject{ID: pid, DefaultBranchID: "br-fake-main"}
	st.Created.Databases["app"] = NeonCreatedDatabase{Name: "app", ProjectID: pid, BranchID: "br-fake-main"}
	st.Managed[neonProjectAddress] = true
	st.Managed[fmt.Sprintf(neonDatabaseAddressFmt, "app")] = true
	r := qaNeonResolve(t, env, st, qaNeonAPIReady(), nil, neonPhasePre)
	if len(r.Problems) != 0 || !reflect.DeepEqual(r.Releases, []string{neonProjectAddress}) || len(r.KeptAdd) != 0 {
		t.Fatalf("same-id flip=%+v", r)
	}
}

// REQ-30 / TEST-24. The injected lister cannot execute Terraform.
func TestNeonContractStateProbeRechecksAfterInitLockChangeAndNewRun(t *testing.T) {
	// REQ-30 / TEST-24.
	old := neonStateLister
	t.Cleanup(func() { neonStateLister = old; resetNeonStateCache() })
	resetNeonStateCache()
	dir := t.TempDir()
	lock := filepath.Join(dir, ".terraform.lock.hcl")
	qaNeonWrite(t, lock, `provider "registry.terraform.io/kislerdm/neon" { version = "0.18.0" }`)
	current := "module.neon.neon_project.this[0]"
	neonStateLister = func(context.Context, string) ([]byte, error) { return []byte(current), nil }
	if neonStateMayExist(dir) {
		t.Fatal("uninitialized checkout must be checked again after init")
	}
	if err := os.Mkdir(filepath.Join(dir, ".terraform"), 0700); err != nil {
		t.Fatal(err)
	}
	if !neonStateMayExist(dir) {
		t.Fatal("init did not enable state discovery")
	}
	current = "aws_s3_bucket.fake"
	qaNeonWrite(t, lock, `# init changed the lock
provider "registry.terraform.io/kislerdm/neon" { version = "0.18.0" }`)
	if neonStateMayExist(dir) {
		t.Fatal("changed lock did not refresh state discovery")
	}
	current = "module.neon.neon_project.this[0]"
	resetNeonStateCache()
	if !neonStateMayExist(dir) {
		t.Fatal("new run reused stale no-Neon answer")
	}
}

// REQ-30 / TEST-24.
func TestNeonContractStateProbeIgnoresStaleLocksAndFailsClosed(t *testing.T) {
	old := neonStateLister
	t.Cleanup(func() { neonStateLister = old; resetNeonStateCache() })
	for _, tc := range []struct {
		name, out               string
		initialised, fail, want bool
	}{
		{"not initialised", "module.neon.neon_project.this[0]\n", false, false, false},
		{"stale lock", "aws_s3_bucket.fake\n", true, false, false},
		{"data remains", "module.neon.neon_project.this[0]\n", true, false, true},
		{"roles remain", `module.neon_access_old.postgresql_role.svc["backend"]`, true, false, true},
		{"only SSM remains", `module.neon_ssm.aws_ssm_parameter.database_url["backend"]`, true, false, false},
		{"unreadable", "", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetNeonStateCache()
			dir := t.TempDir()
			qaNeonWrite(t, filepath.Join(dir, ".terraform.lock.hcl"), `provider "registry.terraform.io/kislerdm/neon" { version = "0.18.0" }`)
			if tc.initialised {
				if err := os.Mkdir(filepath.Join(dir, ".terraform"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			neonStateLister = func(context.Context, string) ([]byte, error) {
				if tc.fail {
					return nil, errors.New("fake backend unavailable")
				}
				return []byte(tc.out), nil
			}
			if got := neonStateMayExist(dir); got != tc.want {
				t.Fatalf("state may exist=%v want %v", got, tc.want)
			}
			if tc.fail && neonStateReadError(dir) == nil {
				t.Error("fail-closed decision must explain the read error")
			}
		})
	}
}
