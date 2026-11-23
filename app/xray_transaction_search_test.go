package main

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// X-Ray Transaction Search: which envs use X-Ray, the xray.indexing_percentage
// block, the cross-env indexing conflict warning, the rendered template and
// the provider lock floor. All accounts and names are synthetic.

const (
	xrayTSAccount      = "000000000000"
	xrayTSOtherAccount = "111111111111"
	xrayTSRegion       = "us-east-1"
	xrayTSOtherRegion  = "eu-west-1"
)

// REQ-1 / TEST-1
func TestEnvUsesXRay(t *testing.T) {
	tests := []struct {
		name   string
		envMap map[string]interface{}
		want   bool
	}{
		{name: "empty env does not use X-Ray", envMap: map[string]interface{}{}, want: false},
		{
			name:   "backend xray_enabled true uses X-Ray",
			envMap: map[string]interface{}{"workload": map[string]interface{}{"xray_enabled": true}},
			want:   true,
		},
		{
			name:   "backend xray_enabled false does not use X-Ray",
			envMap: map[string]interface{}{"workload": map[string]interface{}{"xray_enabled": false}},
			want:   false,
		},
		{
			name: "a service with xray_enabled uses X-Ray",
			envMap: map[string]interface{}{"services": []interface{}{
				map[string]interface{}{"name": "orders"},
				map[string]interface{}{"name": "traced", "xray_enabled": true},
			}},
			want: true,
		},
		{
			name: "an explicitly enabled service with xray_enabled uses X-Ray",
			envMap: map[string]interface{}{"services": []interface{}{
				map[string]interface{}{"name": "traced", "enabled": true, "xray_enabled": true},
			}},
			want: true,
		},
		{
			name: "a disabled service with xray_enabled does not count",
			envMap: map[string]interface{}{"services": []interface{}{
				map[string]interface{}{"name": "traced", "enabled": false, "xray_enabled": true},
			}},
			want: false,
		},
		{
			name: "services without xray_enabled do not use X-Ray",
			envMap: map[string]interface{}{"services": []interface{}{
				map[string]interface{}{"name": "orders", "xray_enabled": false},
			}},
			want: false,
		},
		{
			name: "scheduled and event tasks never count",
			envMap: map[string]interface{}{
				"scheduled_tasks": []interface{}{
					map[string]interface{}{"name": "cleanup", "xray_enabled": true},
				},
				"event_processor_tasks": []interface{}{
					map[string]interface{}{"name": "onevent", "xray_enabled": true},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := envUsesXRay(tt.envMap); got != tt.want {
				t.Errorf("envUsesXRay() = %v, want %v", got, tt.want)
			}
		})
	}
}

// REQ-6, REQ-7 / TEST-2: absent = 1.
func TestXRayIndexingPercentage(t *testing.T) {
	tests := []struct {
		name   string
		xray   interface{}
		absent bool
		want   float64
		wantOK bool
	}{
		{name: "absent block defaults to 1", absent: true, want: 1, wantOK: true},
		{name: "null block defaults to 1", xray: nil, want: 1, wantOK: true},
		{name: "empty block defaults to 1", xray: map[string]interface{}{}, want: 1, wantOK: true},
		{name: "null key defaults to 1", xray: map[string]interface{}{"indexing_percentage": nil}, want: 1, wantOK: true},
		{name: "int 0 is kept", xray: map[string]interface{}{"indexing_percentage": 0}, want: 0, wantOK: true},
		{name: "float 50.5 is kept", xray: map[string]interface{}{"indexing_percentage": 50.5}, want: 50.5, wantOK: true},
		{name: "int 100 is kept", xray: map[string]interface{}{"indexing_percentage": 100}, want: 100, wantOK: true},
		{name: "string value is invalid", xray: map[string]interface{}{"indexing_percentage": "1"}, wantOK: false},
		{name: "101 is invalid", xray: map[string]interface{}{"indexing_percentage": 101}, wantOK: false},
		{name: "a string block is invalid", xray: "on", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envMap := map[string]interface{}{}
			if !tt.absent {
				envMap["xray"] = tt.xray
			}
			got, ok := xrayIndexingPercentage(envMap)
			if ok != tt.wantOK {
				t.Fatalf("xrayIndexingPercentage() ok = %v, want %v (value %v)", ok, tt.wantOK, got)
			}
			if tt.wantOK && got != tt.want {
				t.Errorf("xrayIndexingPercentage() = %v, want %v", got, tt.want)
			}
		})
	}
}

// REQ-6 / TEST-3, TEST-4
func TestValidateXRayMap(t *testing.T) {
	tests := []struct {
		name    string
		xray    interface{}
		absent  bool
		wantErr bool
	}{
		{name: "absent block is ok", absent: true},
		{name: "null block is ok", xray: nil},
		{name: "empty block is ok", xray: map[string]interface{}{}},
		{name: "null key is ok", xray: map[string]interface{}{"indexing_percentage": nil}},
		{name: "int 0 is ok", xray: map[string]interface{}{"indexing_percentage": 0}},
		{name: "int 1 is ok", xray: map[string]interface{}{"indexing_percentage": 1}},
		{name: "int 100 is ok", xray: map[string]interface{}{"indexing_percentage": 100}},
		{name: "float 0.0 is ok", xray: map[string]interface{}{"indexing_percentage": 0.0}},
		{name: "float 50.5 is ok", xray: map[string]interface{}{"indexing_percentage": 50.5}},
		{name: "float 100.0 is ok", xray: map[string]interface{}{"indexing_percentage": 100.0}},

		{name: "a string block is refused", xray: "enabled", wantErr: true},
		{name: "a list block is refused", xray: []interface{}{map[string]interface{}{"indexing_percentage": 1}}, wantErr: true},
		{name: "a bool block is refused", xray: true, wantErr: true},
		{name: "the string \"1\" is refused", xray: map[string]interface{}{"indexing_percentage": "1"}, wantErr: true},
		{name: "a bool value is refused", xray: map[string]interface{}{"indexing_percentage": true}, wantErr: true},
		{name: "int -1 is refused", xray: map[string]interface{}{"indexing_percentage": -1}, wantErr: true},
		{name: "float -0.5 is refused", xray: map[string]interface{}{"indexing_percentage": -0.5}, wantErr: true},
		{name: "float 100.01 is refused", xray: map[string]interface{}{"indexing_percentage": 100.01}, wantErr: true},
		{name: "int 101 is refused", xray: map[string]interface{}{"indexing_percentage": 101}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envMap := map[string]interface{}{"project": "myapp", "env": "dev"}
			if !tt.absent {
				envMap["xray"] = tt.xray
			}
			err := validateXRayMap(envMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateXRayMap() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// REQ-6 / TEST-5
func TestValidateXRayMapNamesUnknownKey(t *testing.T) {
	envMap := map[string]interface{}{
		"xray": map[string]interface{}{"sampling_rate": 5},
	}
	err := validateXRayMap(envMap)
	if err == nil {
		t.Fatal("validateXRayMap() accepted an unknown key sampling_rate")
	}
	if !strings.Contains(err.Error(), "sampling_rate") {
		t.Errorf("error does not name the unknown key sampling_rate: %v", err)
	}
}

func xrayTSEnvMap(account, region string, backendXRay bool, xray interface{}) map[string]interface{} {
	envMap := map[string]interface{}{
		"project":    "myapp",
		"env":        "dev",
		"account_id": account,
		"region":     region,
		"workload":   map[string]interface{}{"xray_enabled": backendXRay},
	}
	if xray != nil {
		envMap["xray"] = xray
	}
	return envMap
}

func xrayTSUse(name, account, region string, percentage float64) xrayIndexingUse {
	return xrayIndexingUse{
		Name:        name,
		File:        name + ".yaml",
		AccountKeys: awsAccountKeys(Env{Region: region, AccountID: account}),
		Percentage:  percentage,
	}
}

// REQ-7 / TEST-6 to TEST-12
func TestXRayIndexingConflictWarning(t *testing.T) {
	if warnCodeXRayIndexingConflict != "xray_indexing_conflict" {
		t.Fatalf("warnCodeXRayIndexingConflict = %q, want xray_indexing_conflict", warnCodeXRayIndexingConflict)
	}
	pct := func(v interface{}) map[string]interface{} {
		return map[string]interface{}{"indexing_percentage": v}
	}

	tests := []struct {
		name        string
		envMap      map[string]interface{}
		siblings    []xrayIndexingUse
		wantWarning bool
		wantNames   []string
	}{
		{
			name:        "same account and region, default 1 vs 5 warns",
			envMap:      xrayTSEnvMap(xrayTSAccount, xrayTSRegion, true, nil),
			siblings:    []xrayIndexingUse{xrayTSUse("prod", xrayTSAccount, xrayTSRegion, 5)},
			wantWarning: true,
			wantNames:   []string{"prod"},
		},
		{
			name:   "every conflicting sibling is named",
			envMap: xrayTSEnvMap(xrayTSAccount, xrayTSRegion, true, pct(10)),
			siblings: []xrayIndexingUse{
				xrayTSUse("prod", xrayTSAccount, xrayTSRegion, 5),
				xrayTSUse("staging", xrayTSAccount, xrayTSRegion, 20),
			},
			wantWarning: true,
			wantNames:   []string{"prod", "staging"},
		},
		{
			name:     "absent equals an explicit 1, no warning",
			envMap:   xrayTSEnvMap(xrayTSAccount, xrayTSRegion, true, nil),
			siblings: []xrayIndexingUse{xrayTSUse("prod", xrayTSAccount, xrayTSRegion, 1)},
		},
		{
			name:     "equal explicit percentages, no warning",
			envMap:   xrayTSEnvMap(xrayTSAccount, xrayTSRegion, true, pct(50.5)),
			siblings: []xrayIndexingUse{xrayTSUse("prod", xrayTSAccount, xrayTSRegion, 50.5)},
		},
		{
			name:     "a different account, no warning",
			envMap:   xrayTSEnvMap(xrayTSAccount, xrayTSRegion, true, nil),
			siblings: []xrayIndexingUse{xrayTSUse("prod", xrayTSOtherAccount, xrayTSRegion, 5)},
		},
		{
			name:     "a different region, no warning",
			envMap:   xrayTSEnvMap(xrayTSAccount, xrayTSRegion, true, nil),
			siblings: []xrayIndexingUse{xrayTSUse("prod", xrayTSAccount, xrayTSOtherRegion, 5)},
		},
		{
			name:     "this env without X-Ray, no warning",
			envMap:   xrayTSEnvMap(xrayTSAccount, xrayTSRegion, false, nil),
			siblings: []xrayIndexingUse{xrayTSUse("prod", xrayTSAccount, xrayTSRegion, 5)},
		},
		{
			name:   "no siblings, no warning",
			envMap: xrayTSEnvMap(xrayTSAccount, xrayTSRegion, true, pct(5)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warning, ok := xrayIndexingConflictWarning(tt.envMap, tt.siblings)
			if ok != tt.wantWarning {
				t.Fatalf("xrayIndexingConflictWarning() ok = %v, want %v (warning %#v)", ok, tt.wantWarning, warning)
			}
			if !tt.wantWarning {
				return
			}
			if warning.Code != warnCodeXRayIndexingConflict {
				t.Errorf("warning code = %q, want %q", warning.Code, warnCodeXRayIndexingConflict)
			}
			for _, name := range tt.wantNames {
				if !strings.Contains(warning.Detail, name) {
					t.Errorf("warning detail does not name conflicting env %q: %q", name, warning.Detail)
				}
			}
		})
	}
}

// xrayTSWriteEnv writes <dir>/<name>.yaml with the given body.
func xrayTSWriteEnv(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name+".yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

const xrayTSDevYAML = `project: myapp
env: dev
account_id: "000000000000"
region: us-east-1
workload:
  xray_enabled: true
`

// REQ-7 / TEST-13
func TestSiblingXRayIndexingUsesReadsOtherFiles(t *testing.T) {
	dir := t.TempDir()
	xrayTSWriteEnv(t, dir, "dev", xrayTSDevYAML)
	xrayTSWriteEnv(t, dir, "prod", `project: myapp
env: prod
account_id: "000000000000"
region: us-east-1
workload:
  xray_enabled: true
xray:
  indexing_percentage: 5
`)

	uses, err := siblingXRayIndexingUses(dir, "dev")
	if err != nil {
		t.Fatalf("siblingXRayIndexingUses() error = %v", err)
	}

	var prod *xrayIndexingUse
	for i := range uses {
		if uses[i].Name == "dev" {
			t.Errorf("siblingXRayIndexingUses() returned the env itself: %#v", uses[i])
		}
		if uses[i].Name == "prod" {
			prod = &uses[i]
		}
	}
	if prod == nil {
		t.Fatalf("siblingXRayIndexingUses() did not return prod: %#v", uses)
	}
	if prod.Percentage != 5 {
		t.Errorf("prod percentage = %v, want 5", prod.Percentage)
	}
	if filepath.Base(prod.File) != "prod.yaml" {
		t.Errorf("prod file = %q, want prod.yaml", prod.File)
	}
	wantKeys := awsAccountKeys(Env{Region: xrayTSRegion, AccountID: xrayTSAccount})
	if !reflect.DeepEqual(prod.AccountKeys, wantKeys) {
		t.Errorf("prod account keys = %v, want %v", prod.AccountKeys, wantKeys)
	}
}

// REQ-7 / TEST-14 to TEST-18: through envLevelWarnings with real files.
func TestEnvLevelWarningsXRayIndexingConflict(t *testing.T) {
	devMap := func() map[string]interface{} {
		return xrayTSEnvMap(xrayTSAccount, xrayTSRegion, true, nil)
	}
	noVPCSiblings := func(dir, self string) ([]vpcCIDRUse, error) { return nil, nil }

	tests := []struct {
		name        string
		prodYAML    string
		useReader   bool
		emptyFile   bool
		wantWarning bool
	}{
		{
			name: "prod in the same account and region at 5 warns",
			prodYAML: `project: myapp
env: prod
account_id: "000000000000"
region: us-east-1
workload:
  xray_enabled: true
xray:
  indexing_percentage: 5
`,
			useReader:   true,
			wantWarning: true,
		},
		{
			name: "a service alone makes prod use X-Ray",
			prodYAML: `project: myapp
env: prod
account_id: "000000000000"
region: us-east-1
services:
  - name: traced
    xray_enabled: true
xray:
  indexing_percentage: 5
`,
			useReader:   true,
			wantWarning: true,
		},
		{
			name: "prod without X-Ray at 5 does not warn",
			prodYAML: `project: myapp
env: prod
account_id: "000000000000"
region: us-east-1
workload:
  xray_enabled: false
xray:
  indexing_percentage: 5
`,
			useReader: true,
		},
		{
			name: "prod in another region does not warn",
			prodYAML: `project: myapp
env: prod
account_id: "000000000000"
region: eu-west-1
workload:
  xray_enabled: true
xray:
  indexing_percentage: 5
`,
			useReader: true,
		},
		{
			name: "prod at an explicit 1 does not warn",
			prodYAML: `project: myapp
env: prod
account_id: "000000000000"
region: us-east-1
workload:
  xray_enabled: true
xray:
  indexing_percentage: 1
`,
			useReader: true,
		},
		{
			name: "a nil XRaySiblings skips the check",
			prodYAML: `project: myapp
env: prod
account_id: "000000000000"
region: us-east-1
workload:
  xray_enabled: true
xray:
  indexing_percentage: 5
`,
			useReader: false,
		},
		{
			name: "an empty EnvFile skips the check",
			prodYAML: `project: myapp
env: prod
account_id: "000000000000"
region: us-east-1
workload:
  xray_enabled: true
xray:
  indexing_percentage: 5
`,
			useReader: true,
			emptyFile: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			devFile := xrayTSWriteEnv(t, dir, "dev", xrayTSDevYAML)
			xrayTSWriteEnv(t, dir, "prod", tt.prodYAML)

			in := warningInputs{
				EnvName:  "dev",
				EnvFile:  devFile,
				Siblings: noVPCSiblings,
			}
			if tt.emptyFile {
				in.EnvFile = ""
			}
			if tt.useReader {
				in.XRaySiblings = siblingXRayIndexingUses
			}

			warnings := envLevelWarnings(devMap(), in)
			got := hasConfigWarningCode(warnings, warnCodeXRayIndexingConflict)
			if got != tt.wantWarning {
				t.Fatalf("xray_indexing_conflict present = %v, want %v: %#v", got, tt.wantWarning, warnings)
			}
			if tt.wantWarning {
				for _, w := range warnings {
					if w.Code == warnCodeXRayIndexingConflict && !strings.Contains(w.Detail, "prod") {
						t.Errorf("warning detail does not name prod: %q", w.Detail)
					}
				}
			}
		})
	}
}

var xrayTSPercentageLine = regexp.MustCompile(`^xray_indexing_percentage\s*=\s*(\S+)\s*$`)

// xrayTSRenderedPercentages returns the value of every rendered
// `xray_indexing_percentage = ...` line.
func xrayTSRenderedPercentages(rendered string) []string {
	var values []string
	for _, line := range strings.Split(rendered, "\n") {
		if m := xrayTSPercentageLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			values = append(values, m[1])
		}
	}
	return values
}

// REQ-5 / TEST-19, TEST-20
func TestTemplate_XRayIndexingPercentage(t *testing.T) {
	tests := []struct {
		name   string
		xray   interface{}
		absent bool
		want   string
	}{
		{name: "absent block renders 1", absent: true, want: "1"},
		{name: "empty block renders 1", xray: map[string]interface{}{}, want: "1"},
		{name: "explicit 50.5 is rendered", xray: map[string]interface{}{"indexing_percentage": 50.5}, want: "50.5"},
		{name: "explicit 0 is rendered, not the default", xray: map[string]interface{}{"indexing_percentage": 0}, want: "0"},
		{name: "explicit 100 is rendered", xray: map[string]interface{}{"indexing_percentage": 100}, want: "100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := minimalEnvMap("")
			if tt.absent {
				delete(env, "xray")
			} else {
				env["xray"] = tt.xray
			}
			out := renderMainTemplateWithCustomPre(t, env, false, false)

			values := xrayTSRenderedPercentages(out)
			if len(values) == 0 {
				t.Fatal("rendered main.tf has no `xray_indexing_percentage =` argument")
			}
			for _, v := range values {
				if v != tt.want {
					t.Errorf("xray_indexing_percentage = %s, want %s", v, tt.want)
				}
			}
		})
	}
}

var xrayTSProviderBlock = regexp.MustCompile(`(?s)\{[^{}]*"hashicorp/aws"[^{}]*\}`)

// REQ-5 / TEST-21
func TestTemplate_AWSProviderConstraintIs647(t *testing.T) {
	out := renderMainTemplateWithCustomPre(t, minimalEnvMap(""), false, false)

	blocks := xrayTSProviderBlock.FindAllString(out, -1)
	if len(blocks) == 0 {
		t.Fatal("rendered main.tf has no required_providers entry for hashicorp/aws")
	}
	version := regexp.MustCompile(`version\s*=\s*"~> 6\.47"`)
	for _, block := range blocks {
		if !version.MatchString(block) {
			t.Errorf("hashicorp/aws constraint is not \"~> 6.47\":\n%s", block)
		}
	}
}

// REQ-8 / TEST-22
func TestLockedProviderBehindMinorFloor(t *testing.T) {
	tests := []struct {
		name        string
		lock        string
		constraint  string
		want        bool
		wantMajor   int
		wantMinor   int
		wantHasMin  bool
		lockedMajor int
		lockedMinor int
	}{
		{"6.30.0 vs ~> 6.47 is behind", "6.30.0", "~> 6.47", true, 6, 47, true, 6, 30},
		{"6.47.0 vs ~> 6.47 is not behind", "6.47.0", "~> 6.47", false, 6, 47, true, 6, 47},
		{"6.73.1 vs ~> 6.47 is not behind", "6.73.1", "~> 6.47", false, 6, 47, true, 6, 73},
		{"6.30.0 vs ~> 6.0 is not behind", "6.30.0", "~> 6.0", false, 6, 0, true, 6, 30},
		{"5.99.0 vs ~> 6.0 is behind", "5.99.0", "~> 6.0", true, 6, 0, true, 5, 99},
		{"7.1.0 vs ~> 6.47 is not behind", "7.1.0", "~> 6.47", false, 6, 47, true, 7, 1},
		{"6.30.0 vs ~> 6 without a minor is not behind", "6.30.0", "~> 6", false, 6, 0, false, 6, 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lock, main := writeLockFixtures(t, tt.lock, tt.constraint)
			behind, locked, wanted := lockedProviderMajorBehind(lock, main)
			if behind != tt.want {
				t.Errorf("behind = %v, want %v (locked %+v, wanted %+v)", behind, tt.want, locked, wanted)
			}
			if wanted.Major != tt.wantMajor || wanted.HasMinor != tt.wantHasMin {
				t.Errorf("wanted = %+v, want major %d HasMinor %v", wanted, tt.wantMajor, tt.wantHasMin)
			}
			if tt.wantHasMin && wanted.Minor != tt.wantMinor {
				t.Errorf("wanted minor = %d, want %d", wanted.Minor, tt.wantMinor)
			}
			if locked.Major != tt.lockedMajor || locked.Minor != tt.lockedMinor {
				t.Errorf("locked = %+v, want %d.%d", locked, tt.lockedMajor, tt.lockedMinor)
			}
		})
	}
}

// REQ-8 / TEST-23
func TestLockedProviderBehindUnreadableFiles(t *testing.T) {
	t.Run("unparseable lock file is not behind", func(t *testing.T) {
		lock, main := writeLockFixtures(t, "", "~> 6.47")
		if err := os.WriteFile(lock, []byte("this is { not hcl at all"), 0o644); err != nil {
			t.Fatal(err)
		}
		if behind, _, _ := lockedProviderMajorBehind(lock, main); behind {
			t.Error("an unparseable lock file counted as behind")
		}
	})

	t.Run("unparseable lock version is not behind", func(t *testing.T) {
		lock, main := writeLockFixtures(t, "not-a-version", "~> 6.47")
		if behind, _, _ := lockedProviderMajorBehind(lock, main); behind {
			t.Error("a lock with an unparseable version counted as behind")
		}
	})

	t.Run("missing main.tf is not behind", func(t *testing.T) {
		lock, _ := writeLockFixtures(t, "6.30.0", "~> 6.47")
		missing := filepath.Join(t.TempDir(), "main.tf")
		if behind, _, _ := lockedProviderMajorBehind(lock, missing); behind {
			t.Error("a missing main.tf counted as behind")
		}
	})

	t.Run("missing lock file is not behind", func(t *testing.T) {
		lock, main := writeLockFixtures(t, "", "~> 6.47")
		if behind, _, _ := lockedProviderMajorBehind(lock, main); behind {
			t.Error("a missing lock file counted as behind")
		}
	})
}
