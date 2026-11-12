package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Deploy-time Neon reconciliation.
//
// env/<env>/main.tf depends only on the YAML. What the YAML cannot say — what
// is already in Terraform state and what exists in Neon — is resolved here:
// readNeonState reads the Neon part of state, and resolveNeon compares it
// with the YAML and with Neon, and returns what to release from state, what
// to record as kept in Neon, which connections are retired, and the
// credentials the postgresql providers need. It never changes state, files or
// Neon itself; the caller acts on the resolution.

// terraformRunner runs one terraform command in dir and returns its output.
type terraformRunner func(ctx context.Context, purpose tfPurpose, dir string, args ...string) ([]byte, error)

// ssmLister lists every SSM parameter name under path, recursively. nil when
// meroku runs without AWS, which skips the checks that need it.
type ssmLister func(ctx context.Context, path string) ([]string, error)

// neonPhase is when resolveNeon runs: pre is before anything is released or
// planned; post is after step 1 has applied, with state read again.
type neonPhase int

const (
	neonPhasePre neonPhase = iota
	neonPhasePost
)

// Terraform addresses meroku manages in a Neon env.
const (
	neonModulePrefix       = "module.neon."
	neonAccessModulePrefix = "module.neon_access_"
	neonProjectAddress     = "module.neon.neon_project.this[0]"
	neonDatabaseAddressFmt = `module.neon.neon_database.created["%s"]`
	neonSSMParamName       = "database_url"
)

var (
	neonReSSMAddress    = regexp.MustCompile(`^module\.neon_ssm\.aws_ssm_parameter\.database_url\["([^"]+)"\]$`)
	neonReSchemaAddress = regexp.MustCompile(`^module\.neon_access_([^.\[]+)\.postgresql_schema\.this\["([^"]+)"\]$`)
	neonReAccessModule  = regexp.MustCompile(`^module\.neon_access_([^.\[]+)\.`)
	// Database-scoped addresses in a neon_access module that go with their
	// database when it was deleted outside meroku. Role, password and the
	// connection record stay.
	neonReDatabaseScoped = regexp.MustCompile(`\.(postgresql_schema|postgresql_grant|postgresql_default_privileges)\.`)
)

// neonResolveNow stamps Kept entries.
var neonResolveNow = time.Now

// NeonConnectionState is one neon_access module's terraform_data.connection:
// the database it serves, where, as which admin role, and its roles by
// service. It holds no secrets.
type NeonConnectionState struct {
	Alias     string
	Database  string
	ProjectID string
	BranchID  string
	AdminRole string
	Roles     map[string]string // svc -> role name
}

// NeonCreatedProject is neon_project.this[0] as state records it.
type NeonCreatedProject struct {
	ID              string
	Name            string
	OrgID           string
	DefaultBranchID string
}

// NeonCreatedDatabase is one neon_database.created[...] as state records it.
type NeonCreatedDatabase struct {
	Name      string
	ProjectID string
	BranchID  string
	OwnerName string
}

// NeonCreatedSet is what meroku created in Neon, read from state before any
// release. YAML flags never decide it.
type NeonCreatedSet struct {
	Project   *NeonCreatedProject
	Databases map[string]NeonCreatedDatabase // by name
}

// NeonStateView is the Neon part of one env's Terraform state.
type NeonStateView struct {
	// Managed holds every address under module.neon and every
	// module.neon_access_*; never module.neon_ssm.
	Managed map[string]bool
	// SSMServices holds each service with a
	// module.neon_ssm.aws_ssm_parameter.database_url["<svc>"]. Those are never
	// released.
	SSMServices map[string]bool
	// ProjectID is the created project's id, or else the project the
	// connections and databases record.
	ProjectID   string
	Created     NeonCreatedSet
	Connections map[string]NeonConnectionState // by alias
}

// NeonKeptEntry is one object released from state and left in Neon. Kind is
// "project", "database" or "role". Ids and names only.
type NeonKeptEntry struct {
	Kind       string    `json:"kind"`
	OrgID      string    `json:"org_id,omitempty"`
	ProjectID  string    `json:"project_id"`
	BranchID   string    `json:"branch_id,omitempty"`
	Database   string    `json:"database,omitempty"`
	Role       string    `json:"role,omitempty"`
	ReleasedAt time.Time `json:"released_at"`
}

// Kinds of NeonKeptEntry.
const (
	neonKeptProject  = "project"
	neonKeptDatabase = "database"
	neonKeptRole     = "role"
)

// PGCredential is one database's entry in TF_VAR_neon_pg_credentials: the
// default branch's direct read_write host and the admin role's login.
type PGCredential struct {
	Host      string `json:"host"`
	User      string `json:"user"`
	Password  string `json:"password"`
	PGVersion string `json:"pg_version"`
}

// neonCredentialsJSON is the value of TF_VAR_neon_pg_credentials, registered
// for redaction before it is returned.
func neonCredentialsJSON(creds map[string]PGCredential) (string, error) {
	if creds == nil {
		creds = map[string]PGCredential{}
	}
	data, err := json.Marshal(creds)
	if err != nil {
		return "", errors.New("neon: encoding the Postgres credentials failed")
	}
	registerSecret(string(data))
	return string(data), nil
}

// NeonResolution is what resolveNeon decided. A non-empty Problems means
// stop: nothing may be released or planned.
type NeonResolution struct {
	ProjectID       string
	DefaultBranchID string
	// Releases are the addresses for one `terraform state rm`.
	Releases []string
	// KeptAdd is written to the Kept file before that state rm.
	KeptAdd []NeonKeptEntry
	// OutOfBand names each object in state that was deleted outside meroku
	// (V-N29); the caller asks whether to create it again.
	OutOfBand []string
	// OutOfBandReleases are the addresses to state rm when the user chooses
	// to create the OutOfBand objects again.
	OutOfBandReleases []string
	// Notices are lines to print, such as an object released as data.
	Notices []string
	// Retired are connections in state but not in the render whose database
	// still exists: their roles are dropped in step 2.
	Retired []NeonConnectionState
	// Dropped are connections whose database is gone: their roles go to the
	// Kept file, then the module is released (its addresses are in Releases).
	Dropped []NeonConnectionState
	// SSMExclude are the database_url parameter names leaving with this
	// deploy.
	SSMExclude []string
	Problems   []string
	// Credentials are keyed by database, for current and retired connections.
	Credentials map[string]PGCredential
}

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

// neonStateMayExist reports whether envDir's Terraform state may hold
// resources of the Neon or Postgres providers.
//
// The lock file is only the cheap pre-check: init records the providers state
// requires, but plan and apply never remove a lock entry, so the lock still
// lists them after the last Neon resource has left state. When the lock lists
// one, the state itself decides: envDir must be initialised and `state list`
// must show a module.neon or module.neon_access_ address. An uninitialised
// envDir answers false; every caller checks again after its init. A state
// that cannot be read answers true, so a failure never skips the Neon login;
// neonStateReadError says why.
//
// The state read happens once per envDir and lock file content: later calls
// in the same run reuse its answer, and an init that changes the lock reads
// again. resetNeonStateCache starts a new run.
func neonStateMayExist(envDir string) bool {
	return neonStateProbeFor(envDir).mayExist
}

// neonStateReadError is why the state read behind neonStateMayExist failed
// for envDir's current lock, or nil when it succeeded or has not run.
func neonStateReadError(envDir string) error {
	key, sum, ok := neonStateCacheKey(envDir)
	if !ok {
		return nil
	}
	neonStateCache.mu.Lock()
	defer neonStateCache.mu.Unlock()
	if e, found := neonStateCache.entries[key]; found && e.lockSum == sum {
		return e.probe.readErr
	}
	return nil
}

// neonStateProbe is one answer of neonStateMayExist.
type neonStateProbe struct {
	mayExist bool
	readErr  error
}

type neonStateCacheEntry struct {
	lockSum [sha256.Size]byte
	probe   neonStateProbe
}

// neonStateCache holds the state reads of this run, keyed by absolute
// envDir. An entry counts only while the lock file still hashes to lockSum.
var neonStateCache = struct {
	mu      sync.Mutex
	entries map[string]neonStateCacheEntry
}{entries: map[string]neonStateCacheEntry{}}

// resetNeonStateCache forgets every state read, so the next check reads the
// state again. A deploy or destroy calls it when it starts and ends: either
// may change what state holds without changing the lock.
func resetNeonStateCache() {
	neonStateCache.mu.Lock()
	defer neonStateCache.mu.Unlock()
	neonStateCache.entries = map[string]neonStateCacheEntry{}
}

// neonStateCacheKey is envDir's cache key and its lock file's hash, with ok
// false when the lock cannot be read.
func neonStateCacheKey(envDir string) (string, [sha256.Size]byte, bool) {
	data, err := os.ReadFile(filepath.Join(envDir, ".terraform.lock.hcl"))
	if err != nil {
		return "", [sha256.Size]byte{}, false
	}
	key, err := filepath.Abs(envDir)
	if err != nil {
		key = filepath.Clean(envDir)
	}
	return key, sha256.Sum256(data), true
}

func neonStateProbeFor(envDir string) neonStateProbe {
	data, err := os.ReadFile(filepath.Join(envDir, ".terraform.lock.hcl"))
	if err != nil {
		return neonStateProbe{}
	}
	if !bytes.Contains(data, []byte("kislerdm/neon")) && !bytes.Contains(data, []byte("cyrilgdn/postgresql")) {
		return neonStateProbe{}
	}
	// Not cached: the init that creates .terraform comes next, and the
	// check after it must read the state.
	if _, err := os.Stat(filepath.Join(envDir, ".terraform")); err != nil {
		return neonStateProbe{}
	}

	key, err := filepath.Abs(envDir)
	if err != nil {
		key = filepath.Clean(envDir)
	}
	sum := sha256.Sum256(data)
	neonStateCache.mu.Lock()
	defer neonStateCache.mu.Unlock()
	if e, found := neonStateCache.entries[key]; found && e.lockSum == sum {
		return e.probe
	}

	ctx, cancel := context.WithTimeout(context.Background(), neonStateListTimeout)
	defer cancel()
	var probe neonStateProbe
	if out, err := neonStateLister(ctx, envDir); isNoStateFileError(err) {
		probe = neonStateProbe{}
	} else if err != nil {
		probe = neonStateProbe{mayExist: true, readErr: err}
	} else {
		probe = neonStateProbe{mayExist: stateListHasNeon(out)}
	}
	neonStateCache.entries[key] = neonStateCacheEntry{lockSum: sum, probe: probe}
	return probe
}

// neonStateReadNotice explains, for an env without `neon:`, that a Neon
// login is asked for only because its state could not be read. "" when the
// read succeeded or did not run.
func neonStateReadNotice(env Env) string {
	if env.Neon != nil || env.Env == "" {
		return ""
	}
	err := neonStateReadError(envTerraformDir(env.Env))
	if err == nil {
		return ""
	}
	first, _, _ := strings.Cut(err.Error(), "\n")
	return fmt.Sprintf("ℹ️  meroku could not read the Terraform state of env/%s (%s), so it assumes Neon resources may still be there.\n", env.Env, first)
}

// neonStateListTimeout bounds the state read behind neonStateMayExist.
const neonStateListTimeout = 2 * time.Minute

// neonStateLister runs `terraform state list` in dir. A variable so the
// trigger can be exercised without Terraform.
var neonStateLister = func(ctx context.Context, dir string) ([]byte, error) {
	return terraformOutputRunner(ctx, tfState, dir, "state", "list")
}

// isNoStateFileError reports whether a `terraform state` command failed only
// because there is no state yet: a local backend before its first apply. An S3
// backend with no state object lists nothing instead, so both read as empty.
func isNoStateFileError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "No state file was found")
}

// stateListHasNeon reports whether `terraform state list` output holds an
// address that only a Neon or Postgres provider can manage.
func stateListHasNeon(out []byte) bool {
	for _, line := range strings.Split(string(out), "\n") {
		addr := strings.TrimSpace(line)
		if strings.HasPrefix(addr, neonModulePrefix) || strings.HasPrefix(addr, neonAccessModulePrefix) {
			return true
		}
	}
	return false
}

// expectedNeonAddresses are the step-1 targets: the created project and every
// created database. Empty in reuse mode with no created database.
func expectedNeonAddresses(n *Neon) []string {
	n = normalizeNeon(n)
	if n == nil {
		return nil
	}
	var out []string
	if n.Mode == NeonModeCreate {
		out = append(out, neonProjectAddress)
	}
	var dbs []string
	for _, db := range n.Databases {
		if db.Create {
			dbs = append(dbs, fmt.Sprintf(neonDatabaseAddressFmt, db.Name))
		}
	}
	sort.Strings(dbs)
	return append(out, dbs...)
}

// neonAccessModuleAddress is the module address of a connection alias.
func neonAccessModuleAddress(alias string) string {
	return neonAccessModulePrefix + alias
}

// Shapes of `terraform show -json` on state.
type neonTFState struct {
	Values *struct {
		RootModule neonTFModule `json:"root_module"`
	} `json:"values"`
}

type neonTFModule struct {
	Address      string           `json:"address"`
	Resources    []neonTFResource `json:"resources"`
	ChildModules []neonTFModule   `json:"child_modules"`
}

type neonTFResource struct {
	Address         string                 `json:"address"`
	Mode            string                 `json:"mode"`
	Type            string                 `json:"type"`
	Name            string                 `json:"name"`
	Index           json.RawMessage        `json:"index"`
	Values          map[string]interface{} `json:"values"`
	SensitiveValues interface{}            `json:"sensitive_values"`
}

// readNeonState reads the Neon part of the state in dir: `state list`, and
// `show -json` when Neon addresses exist. Every sensitive value in the state
// JSON is registered for redaction before anything else reads it. Errors
// never quote terraform's output.
func readNeonState(ctx context.Context, tf terraformRunner, dir string) (NeonStateView, error) {
	view := NeonStateView{
		Managed:     map[string]bool{},
		SSMServices: map[string]bool{},
		Created:     NeonCreatedSet{Databases: map[string]NeonCreatedDatabase{}},
		Connections: map[string]NeonConnectionState{},
	}

	out, err := tf(ctx, tfState, dir, "state", "list")
	if isNoStateFileError(err) {
		return view, nil
	}
	if err != nil {
		return view, fmt.Errorf("neon: reading Terraform state (state list) failed: %s", redactSecrets(err.Error()))
	}
	for _, line := range strings.Split(string(out), "\n") {
		addr := strings.TrimSpace(line)
		if addr == "" {
			continue
		}
		if m := neonReSSMAddress.FindStringSubmatch(addr); m != nil {
			view.SSMServices[m[1]] = true
			continue
		}
		if strings.HasPrefix(addr, neonModulePrefix) || strings.HasPrefix(addr, neonAccessModulePrefix) {
			view.Managed[addr] = true
		}
	}
	if len(view.Managed) == 0 {
		return view, nil
	}

	out, err = tf(ctx, tfState, dir, "show", "-json")
	if err != nil {
		return view, fmt.Errorf("neon: reading Terraform state (show -json) failed: %s", redactSecrets(err.Error()))
	}
	var st neonTFState
	if err := json.Unmarshal(out, &st); err != nil {
		return view, errors.New("neon: Terraform state (show -json) is not the JSON meroku expects")
	}
	if st.Values == nil {
		return view, nil
	}

	var resources []neonTFResource
	collectNeonResources(st.Values.RootModule, &resources)
	for _, r := range resources {
		registerNeonSensitive(r.Values, r.SensitiveValues)
	}

	roleNames := map[string]map[string]string{} // alias -> svc -> role
	for _, r := range resources {
		if r.Mode != "managed" {
			continue
		}
		switch {
		case r.Address == neonProjectAddress && r.Type == "neon_project":
			view.Created.Project = &NeonCreatedProject{
				ID:              neonValueString(r.Values, "id"),
				Name:            neonValueString(r.Values, "name"),
				OrgID:           neonValueString(r.Values, "org_id"),
				DefaultBranchID: neonValueString(r.Values, "default_branch_id"),
			}
		case strings.HasPrefix(r.Address, neonModulePrefix) && r.Type == "neon_database" && r.Name == "created":
			name := neonValueString(r.Values, "name")
			if name == "" {
				name = neonIndexKey(r.Index)
			}
			view.Created.Databases[name] = NeonCreatedDatabase{
				Name:      name,
				ProjectID: neonValueString(r.Values, "project_id"),
				BranchID:  neonValueString(r.Values, "branch_id"),
				OwnerName: neonValueString(r.Values, "owner_name"),
			}
		case r.Type == "terraform_data" && r.Name == "connection":
			alias := neonAliasOf(r.Address)
			if alias == "" {
				continue
			}
			view.Connections[alias] = neonConnectionFromInput(alias, r.Values["input"])
		case r.Type == "postgresql_role" && r.Name == "svc":
			alias := neonAliasOf(r.Address)
			svc := neonIndexKey(r.Index)
			if alias == "" || svc == "" {
				continue
			}
			if roleNames[alias] == nil {
				roleNames[alias] = map[string]string{}
			}
			roleNames[alias][svc] = neonValueString(r.Values, "name")
		}
	}

	// A neon_access module in state always has an entry, even one applied
	// only in part, so retirement sees it.
	for addr := range view.Managed {
		if m := neonReAccessModule.FindStringSubmatch(addr); m != nil {
			if _, ok := view.Connections[m[1]]; !ok {
				view.Connections[m[1]] = NeonConnectionState{Alias: m[1], Roles: map[string]string{}}
			}
		}
	}
	for alias, roles := range roleNames {
		c := view.Connections[alias]
		if c.Roles == nil {
			c.Roles = map[string]string{}
		}
		for svc, role := range roles {
			if _, ok := c.Roles[svc]; !ok && role != "" {
				c.Roles[svc] = role
			}
		}
		view.Connections[alias] = c
	}

	view.ProjectID = view.primaryProjectID()
	return view, nil
}

func collectNeonResources(m neonTFModule, out *[]neonTFResource) {
	*out = append(*out, m.Resources...)
	for _, child := range m.ChildModules {
		collectNeonResources(child, out)
	}
}

// registerNeonSensitive registers every string in values that its
// sensitive_values mask marks true, and every string under an attribute whose
// name says password.
func registerNeonSensitive(values interface{}, mask interface{}) {
	switch m := mask.(type) {
	case bool:
		if m {
			registerAllStrings(values)
		}
	case map[string]interface{}:
		if v, ok := values.(map[string]interface{}); ok {
			for k, sub := range m {
				registerNeonSensitive(v[k], sub)
			}
		}
	case []interface{}:
		if v, ok := values.([]interface{}); ok {
			for i, sub := range m {
				if i < len(v) {
					registerNeonSensitive(v[i], sub)
				}
			}
		}
	}
	if v, ok := values.(map[string]interface{}); ok {
		for k, sub := range v {
			if strings.Contains(strings.ToLower(k), "password") {
				registerAllStrings(sub)
			}
		}
	}
}

func registerAllStrings(v interface{}) {
	switch x := v.(type) {
	case string:
		registerSecret(x)
	case map[string]interface{}:
		for _, sub := range x {
			registerAllStrings(sub)
		}
	case []interface{}:
		for _, sub := range x {
			registerAllStrings(sub)
		}
	}
}

func neonValueString(values map[string]interface{}, key string) string {
	if s, ok := values[key].(string); ok {
		return s
	}
	return ""
}

// neonIndexKey is a resource's for_each key (or count index) as a string.
func neonIndexKey(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	return ""
}

// neonAliasOf is the connection alias of an address inside a neon_access
// module, or "".
func neonAliasOf(addr string) string {
	if m := neonReAccessModule.FindStringSubmatch(addr); m != nil {
		return m[1]
	}
	return ""
}

// neonConnectionFromInput reads terraform_data.connection's input. A dynamic
// value may come wrapped as {"value": …, "type": …}.
func neonConnectionFromInput(alias string, raw interface{}) NeonConnectionState {
	c := NeonConnectionState{Alias: alias, Roles: map[string]string{}}
	in, ok := raw.(map[string]interface{})
	if !ok {
		return c
	}
	if inner, ok := in["value"].(map[string]interface{}); ok && len(in) <= 2 {
		if _, typed := in["type"]; typed {
			in = inner
		}
	}
	c.Database = neonValueString(in, "database")
	if c.Database == "" {
		c.Database = neonValueString(in, "key")
	}
	c.ProjectID = neonValueString(in, "project_id")
	c.BranchID = neonValueString(in, "branch_id")
	c.AdminRole = neonValueString(in, "admin_role")
	if roles, ok := in["roles"].(map[string]interface{}); ok {
		for svc, v := range roles {
			if s, ok := v.(string); ok && s != "" {
				c.Roles[svc] = s
			}
		}
	}
	return c
}

// recordedProjectIDs are the distinct project ids state records, sorted.
func (v NeonStateView) recordedProjectIDs() []string {
	seen := map[string]bool{}
	if v.Created.Project != nil && v.Created.Project.ID != "" {
		seen[v.Created.Project.ID] = true
	}
	for _, db := range v.Created.Databases {
		if db.ProjectID != "" {
			seen[db.ProjectID] = true
		}
	}
	for _, c := range v.Connections {
		if c.ProjectID != "" {
			seen[c.ProjectID] = true
		}
	}
	return neonSortedSet(seen)
}

// recordedBranchIDs are the distinct branch ids state records for project,
// sorted.
func (v NeonStateView) recordedBranchIDs(project string) []string {
	seen := map[string]bool{}
	for _, db := range v.Created.Databases {
		if db.BranchID != "" && (db.ProjectID == project || db.ProjectID == "") {
			seen[db.BranchID] = true
		}
	}
	for _, c := range v.Connections {
		if c.BranchID != "" && (c.ProjectID == project || c.ProjectID == "") {
			seen[c.BranchID] = true
		}
	}
	return neonSortedSet(seen)
}

func (v NeonStateView) primaryProjectID() string {
	if v.Created.Project != nil && v.Created.Project.ID != "" {
		return v.Created.Project.ID
	}
	if ids := v.recordedProjectIDs(); len(ids) > 0 {
		return ids[0]
	}
	return ""
}

func neonSortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------
// What the YAML asks for
// ---------------------------------------------------------------------------

// neonDesired is the render's view of an env: its connections by alias, the
// services with a database_url, and the databases.
type neonDesired struct {
	neon        *Neon // normalized
	connections map[string]neonDesiredConnection
	ssm         map[string]bool
	databases   map[string]NeonDatabase
}

type neonDesiredConnection struct {
	Database string
	// Schemas holds every schema a service of this connection lists.
	Schemas map[string]bool
}

// neonDesiredFromEnv builds the desired view through buildNeonRender, the
// same function the template input comes from. nil when the env has no
// `neon:`.
func neonDesiredFromEnv(env Env) (*neonDesired, error) {
	if env.Neon == nil {
		return nil, nil
	}
	services := make([]interface{}, 0, len(env.Services))
	for _, s := range env.Services {
		services = append(services, map[string]interface{}{"name": s.Name, "neon": s.Neon})
	}
	render, err := buildNeonRender(map[string]interface{}{
		"project":  env.Project,
		"env":      env.Env,
		"neon":     env.Neon,
		"workload": map[string]interface{}{"backend_neon": env.Workload.BackendNeon},
		"services": services,
	})
	if err != nil {
		return nil, err
	}

	d := &neonDesired{
		neon:        normalizeNeon(env.Neon),
		connections: map[string]neonDesiredConnection{},
		ssm:         map[string]bool{},
		databases:   map[string]NeonDatabase{},
	}
	for _, db := range d.neon.Databases {
		d.databases[db.Name] = db
	}
	if render == nil {
		return d, nil
	}
	conns, _ := render["connections"].([]interface{})
	for _, raw := range conns {
		c, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		alias, _ := c["alias"].(string)
		dc := neonDesiredConnection{Schemas: map[string]bool{}}
		dc.Database, _ = c["database"].(string)
		svcs, _ := c["services"].(map[string]interface{})
		for _, sraw := range svcs {
			s, ok := sraw.(map[string]interface{})
			if !ok {
				continue
			}
			schemas, _ := s["schemas"].([]interface{})
			for _, sc := range schemas {
				if name, ok := sc.(string); ok {
					dc.Schemas[name] = true
				}
			}
		}
		d.connections[alias] = dc
	}
	ssm, _ := render["ssm_services"].([]interface{})
	for _, raw := range ssm {
		if s, ok := raw.(string); ok {
			d.ssm[s] = true
		}
	}
	return d, nil
}

// ---------------------------------------------------------------------------
// Neon lookups, cached for one resolution
// ---------------------------------------------------------------------------

type neonProjectLookup struct {
	info  NeonProjectInfo
	found bool
}

type neonLookups struct {
	api       neonAPI
	projects  map[string]neonProjectLookup
	branches  map[string]string // project -> default branch id
	databases map[string][]NeonDatabaseInfo
	hosts     map[string]string
}

func newNeonLookups(api neonAPI) *neonLookups {
	return &neonLookups{
		api:       api,
		projects:  map[string]neonProjectLookup{},
		branches:  map[string]string{},
		databases: map[string][]NeonDatabaseInfo{},
		hosts:     map[string]string{},
	}
}

// project returns the project and whether Neon has it. A 404 is not found;
// any other failure is an error.
func (l *neonLookups) project(ctx context.Context, id string) (NeonProjectInfo, bool, error) {
	if p, ok := l.projects[id]; ok {
		return p.info, p.found, nil
	}
	info, err := l.api.GetProject(ctx, id)
	switch {
	case errors.Is(err, ErrNeonNotFound):
		l.projects[id] = neonProjectLookup{}
		return NeonProjectInfo{}, false, nil
	case err != nil:
		return NeonProjectInfo{}, false, err
	}
	l.projects[id] = neonProjectLookup{info: info, found: true}
	return info, true, nil
}

// defaultBranch is the project's current default branch.
func (l *neonLookups) defaultBranch(ctx context.Context, project string) (string, error) {
	if b, ok := l.branches[project]; ok {
		return b, nil
	}
	branches, err := l.api.ListBranches(ctx, project)
	if err != nil {
		return "", err
	}
	for _, b := range branches {
		if b.Default {
			l.branches[project] = b.ID
			return b.ID, nil
		}
	}
	return "", fmt.Errorf("neon: project %s has no default branch", project)
}

// databasesOn lists a branch's databases. found is false on a 404 (the
// project or branch is gone).
func (l *neonLookups) databasesOn(ctx context.Context, project, branch string) ([]NeonDatabaseInfo, bool, error) {
	key := project + "/" + branch
	if dbs, ok := l.databases[key]; ok {
		return dbs, dbs != nil, nil
	}
	dbs, err := l.api.ListDatabases(ctx, project, branch)
	switch {
	case errors.Is(err, ErrNeonNotFound):
		l.databases[key] = nil
		return nil, false, nil
	case err != nil:
		return nil, false, err
	}
	if dbs == nil {
		dbs = []NeonDatabaseInfo{}
	}
	l.databases[key] = dbs
	return dbs, true, nil
}

// readWriteHost is the direct host of the branch's read_write endpoint, or
// "" when it has none.
func (l *neonLookups) readWriteHost(ctx context.Context, project, branch string) (string, error) {
	key := project + "/" + branch
	if h, ok := l.hosts[key]; ok {
		return h, nil
	}
	eps, err := l.api.ListEndpoints(ctx, project, branch)
	if err != nil {
		return "", err
	}
	host := ""
	for _, e := range eps {
		if e.Type == "read_write" && e.Host != "" {
			host = e.Host
			break
		}
	}
	l.hosts[key] = host
	return host, nil
}

func neonFindDatabase(dbs []NeonDatabaseInfo, name string) (NeonDatabaseInfo, bool) {
	for _, d := range dbs {
		if d.Name == name {
			return d, true
		}
	}
	return NeonDatabaseInfo{}, false
}

// ---------------------------------------------------------------------------
// Resolution
// ---------------------------------------------------------------------------

// neonProblems collects deploy-time problems in the generate-time format.
type neonProblems struct{ list []NeonProblem }

func (p *neonProblems) add(rule, path, problem, fix string) {
	p.list = append(p.list, NeonProblem{Rule: rule, Path: path, Problem: problem, Fix: fix})
}

func (p *neonProblems) any() bool { return len(p.list) > 0 }

func (p *neonProblems) strings() []string {
	out := make([]string, 0, len(p.list))
	for _, x := range p.list {
		out = append(out, x.String())
	}
	return out
}

// resolveNeon compares the env's YAML with state (st) and with Neon.
//
// pre runs before anything is released or planned, in this order: V-N38
// (project identity), V-N39 (default branch), V-N26 (SSM name clash; skipped
// when listSSM is nil), each stopping the resolution on a problem; then V-N36,
// V-N35, V-N33, V-N20, V-N21 and V-N29 (OutOfBand), and the releases and Kept
// entries of the D2 table.
//
// post runs on state read after step 1: V-N21, V-N23, V-N24, V-N25, the
// retired and dropped connections, SSMExclude and the credentials.
func resolveNeon(ctx context.Context, api neonAPI, listSSM ssmLister, env Env, st NeonStateView, phase neonPhase) (NeonResolution, error) {
	d, err := neonDesiredFromEnv(env)
	if err != nil {
		return NeonResolution{}, err
	}
	if d == nil && len(st.Managed) == 0 && len(st.SSMServices) == 0 && len(st.Connections) == 0 {
		return NeonResolution{}, nil
	}
	if api == nil {
		return NeonResolution{}, errors.New("neon: resolving Neon needs a Neon login")
	}
	lk := newNeonLookups(api)

	switch phase {
	case neonPhasePre:
		return resolveNeonPre(ctx, lk, listSSM, env, d, st)
	case neonPhasePost:
		return resolveNeonPost(ctx, lk, env, d, st)
	}
	return NeonResolution{}, fmt.Errorf("neon: unknown resolution phase %d", phase)
}

// neonYAMLProject is the project the YAML names: the reuse project_id, or in
// create mode the project state holds ("" when none yet).
func neonYAMLProject(d *neonDesired, st NeonStateView) string {
	if d.neon.Mode == NeonModeReuse {
		return d.neon.ProjectID
	}
	if st.Created.Project != nil {
		return st.Created.Project.ID
	}
	return ""
}

func resolveNeonPre(ctx context.Context, lk *neonLookups, listSSM ssmLister, env Env, d *neonDesired, st NeonStateView) (NeonResolution, error) {
	var res NeonResolution
	var probs neonProblems
	stop := func() (NeonResolution, error) {
		res.Problems = probs.strings()
		return res, nil
	}

	// V-N38: the YAML's project must be the one every recorded id names.
	want := ""
	if d != nil {
		want = neonYAMLProject(d, st)
		newName := want
		if newName == "" && d.neon.Project != nil {
			newName = fmt.Sprintf("a new project %q", d.neon.Project.Name)
		}
		for _, old := range st.recordedProjectIDs() {
			if old == want {
				continue
			}
			probs.add("V-N38", "neon",
				fmt.Sprintf("`neon` now names project %s, but state holds %s", newName, old),
				fmt.Sprintf("Revert it. To move this env: remove `neon:` and deploy (meroku keeps %s and prints how to delete it), then add the new project.", old))
		}
		if probs.any() {
			return stop()
		}
		res.ProjectID = want
	}

	// V-N39: every recorded branch must be the project's current default.
	checkProjects := st.recordedProjectIDs()
	if d != nil {
		checkProjects = nil
		if want != "" {
			checkProjects = []string{want}
		}
	}
	for _, project := range checkProjects {
		_, found, err := lk.project(ctx, project)
		if err != nil {
			return res, err
		}
		if !found {
			// Deleted outside meroku: V-N29 or V-N21 below.
			continue
		}
		def, err := lk.defaultBranch(ctx, project)
		if err != nil {
			return res, err
		}
		if d != nil {
			res.DefaultBranchID = def
		}
		for _, old := range st.recordedBranchIDs(project) {
			if old == def {
				continue
			}
			probs.add("V-N39", "neon",
				fmt.Sprintf("the default branch of %s is now %s, but meroku's roles and databases are on %s", project, def, old),
				fmt.Sprintf("Make %s the default branch again (`%s branches set-default %s --project-id %s`), then run meroku again.", old, neonCLI(), old, project))
		}
	}
	if probs.any() {
		return stop()
	}

	// V-N26: no other DATABASE_URL under a service's SSM path.
	if d != nil && listSSM != nil {
		for _, svc := range neonSortedSet(d.ssm) {
			prefix := fmt.Sprintf("/%s/%s/%s/", env.Env, env.Project, svc)
			own := prefix + neonSSMParamName
			names, err := listSSM(ctx, prefix)
			if err != nil {
				return res, fmt.Errorf("neon: listing SSM parameters under %s failed: %s", prefix, redactSecrets(err.Error()))
			}
			sort.Strings(names)
			for _, name := range names {
				last := name[strings.LastIndex(name, "/")+1:]
				if !strings.EqualFold(last, neonSSMParamName) {
					continue
				}
				if name == own && st.SSMServices[svc] {
					continue
				}
				probs.add("V-N26", neonServicePath(svc),
					fmt.Sprintf("SSM parameter %s would also become DATABASE_URL in the container", name),
					fmt.Sprintf("delete or rename it; meroku writes %s itself", own))
			}
		}
		if probs.any() {
			return stop()
		}
	}

	if d == nil {
		// `neon:` removed: release everything under module.neon, keep every
		// created object, release every schema.
		res.Releases = neonReleaseSet(st, func(addr string) bool {
			return strings.HasPrefix(addr, neonModulePrefix) || neonReSchemaAddress.MatchString(addr)
		})
		res.KeptAdd = neonKeptAllCreated(st)
		return res, nil
	}

	n := d.neon

	// V-N36: roles are dropped by the admin role that created them.
	for _, alias := range neonSortedConnections(st) {
		c := st.Connections[alias]
		if c.AdminRole != "" && c.AdminRole != n.OwnerRole {
			probs.add("V-N36", "neon.owner_role",
				fmt.Sprintf("owner_role is %s, but the roles of database %s were created by %s", n.OwnerRole, c.Database, c.AdminRole),
				fmt.Sprintf("set owner_role back to %s, or remove the services' neon blocks and deploy that change first", c.AdminRole))
		}
	}

	projectFound := false
	var projectInfo NeonProjectInfo
	if want != "" {
		info, found, err := lk.project(ctx, want)
		if err != nil {
			return res, err
		}
		projectInfo, projectFound = info, found
	}

	switch n.Mode {
	case NeonModeCreate:
		// V-N35: a create-mode env never lands on an existing project.
		if st.Created.Project == nil && n.Project != nil {
			projects, err := lk.api.ListProjects(ctx, n.Project.OrgID)
			if err != nil {
				return res, err
			}
			for _, p := range projects {
				if p.Name == n.Project.Name {
					probs.add("V-N35", "neon.project.name",
						fmt.Sprintf("a project named %s (%s) already exists in %s", p.Name, p.ID, n.Project.OrgID),
						fmt.Sprintf("set mode: reuse with project_id: %s, or choose another name", p.ID))
				}
			}
		}
		// V-N29, project: created by meroku, deleted outside it.
		if st.Created.Project != nil && !projectFound {
			res.OutOfBand = append(res.OutOfBand, "project "+st.Created.Project.ID)
			res.OutOfBandReleases = append(res.OutOfBandReleases, neonReleaseSet(st, func(addr string) bool {
				return strings.HasPrefix(addr, neonModulePrefix) || strings.HasPrefix(addr, neonAccessModulePrefix)
			})...)
		}
	case NeonModeReuse:
		if !projectFound {
			probs.add("V-N21", "neon.project_id",
				fmt.Sprintf("project %s does not exist in Neon, or this login cannot see it", n.ProjectID),
				"check project_id and the Neon login")
		} else if projectInfo.PGVersion != n.PGVersion {
			probs.add("V-N33", "neon.pg_version",
				fmt.Sprintf("pg_version is %d, but project %s runs Postgres %d", n.PGVersion, n.ProjectID, projectInfo.PGVersion),
				fmt.Sprintf("set pg_version: %d", projectInfo.PGVersion))
		}
	}

	// Databases: V-N20, V-N21, V-N29 against the default branch.
	if projectFound && res.DefaultBranchID != "" {
		dbs, found, err := lk.databasesOn(ctx, want, res.DefaultBranchID)
		if err != nil {
			return res, err
		}
		if found {
			for _, name := range neonSortedDatabases(d) {
				db := d.databases[name]
				_, inState := st.Created.Databases[name]
				_, exists := neonFindDatabase(dbs, name)
				switch {
				case db.Create && inState && !exists:
					res.OutOfBand = append(res.OutOfBand, "database "+name)
					res.OutOfBandReleases = append(res.OutOfBandReleases, fmt.Sprintf(neonDatabaseAddressFmt, name))
					module := neonAccessModuleAddress(neonConnectionAlias(name)) + "."
					res.OutOfBandReleases = append(res.OutOfBandReleases, neonReleaseSet(st, func(addr string) bool {
						return strings.HasPrefix(addr, module) && neonReDatabaseScoped.MatchString(addr)
					})...)
				case db.Create && !inState && exists:
					probs.add("V-N20", fmt.Sprintf("neon.databases[%s]", name),
						fmt.Sprintf("%s already exists in Neon. meroku will not adopt existing data", name),
						fmt.Sprintf("set create: false, or delete it first: %s databases delete %s --project-id %s --branch %s", neonCLI(), name, want, res.DefaultBranchID))
				case !db.Create && !exists:
					probs.add("V-N21", fmt.Sprintf("neon.databases[%s]", name),
						fmt.Sprintf("database %s does not exist on the default branch of %s", name, want),
						"create it in Neon, or set create: true")
				}
			}
		}
	}
	if probs.any() {
		return stop()
	}

	// Releases (D2): the project flipped to data, created databases removed
	// or flipped to data, schemas no longer listed.
	releases := map[string]bool{}
	if st.Created.Project != nil && n.Mode == NeonModeReuse && st.Managed[neonProjectAddress] {
		releases[neonProjectAddress] = true
		res.Notices = append(res.Notices, fmt.Sprintf("No longer managed by meroku (kept in Neon, now used as data): project %s", st.Created.Project.ID))
	}
	var removed []string
	for _, name := range neonSortedCreated(st) {
		addr := fmt.Sprintf(neonDatabaseAddressFmt, name)
		db, inYAML := d.databases[name]
		switch {
		case !inYAML:
			releases[addr] = true
			removed = append(removed, name)
		case !db.Create:
			releases[addr] = true
			res.Notices = append(res.Notices, fmt.Sprintf("No longer managed by meroku (kept in Neon, now used as data): database %s", name))
		}
	}
	for addr := range st.Managed {
		m := neonReSchemaAddress.FindStringSubmatch(addr)
		if m == nil {
			continue
		}
		if dc, ok := d.connections[m[1]]; !ok || !dc.Schemas[m[2]] {
			releases[addr] = true
		}
	}
	res.Releases = neonSortedSet(releases)
	res.KeptAdd = neonKeptDatabases(st, removed)
	return res, nil
}

// neonReleaseSet is every managed address keep accepts, sorted.
func neonReleaseSet(st NeonStateView, keep func(string) bool) []string {
	var out []string
	for addr := range st.Managed {
		if keep(addr) {
			out = append(out, addr)
		}
	}
	sort.Strings(out)
	return out
}

// neonKeptAllCreated is the Kept entries for the created project and every
// created database, all released together.
func neonKeptAllCreated(st NeonStateView) []NeonKeptEntry {
	var out []NeonKeptEntry
	if p := st.Created.Project; p != nil && st.Managed[neonProjectAddress] {
		out = append(out, NeonKeptEntry{Kind: neonKeptProject, OrgID: p.OrgID, ProjectID: p.ID, BranchID: p.DefaultBranchID, ReleasedAt: neonResolveNow().UTC()})
	}
	return append(out, neonKeptDatabases(st, neonSortedCreated(st))...)
}

// neonKeptDatabases is the Kept entries for the named created databases.
func neonKeptDatabases(st NeonStateView, databases []string) []NeonKeptEntry {
	now := neonResolveNow().UTC()
	orgID := ""
	if st.Created.Project != nil {
		orgID = st.Created.Project.OrgID
	}
	var out []NeonKeptEntry
	for _, name := range databases {
		db := st.Created.Databases[name]
		out = append(out, NeonKeptEntry{Kind: neonKeptDatabase, OrgID: orgID, ProjectID: db.ProjectID, BranchID: db.BranchID, Database: name, ReleasedAt: now})
	}
	return out
}

func resolveNeonPost(ctx context.Context, lk *neonLookups, env Env, d *neonDesired, st NeonStateView) (NeonResolution, error) {
	res := NeonResolution{Credentials: map[string]PGCredential{}}
	var probs neonProblems
	stop := func() (NeonResolution, error) {
		res.Problems = probs.strings()
		return res, nil
	}

	if d != nil {
		n := d.neon
		project := neonYAMLProject(d, st)
		if project == "" {
			probs.add("V-N21", "neon.project", "the project is not in Terraform state after step 1", "run meroku again")
			return stop()
		}
		_, found, err := lk.project(ctx, project)
		if err != nil {
			return res, err
		}
		if !found {
			probs.add("V-N21", "neon.project_id",
				fmt.Sprintf("project %s does not exist in Neon, or this login cannot see it", project),
				"check project_id and the Neon login")
			return stop()
		}
		branch, err := lk.defaultBranch(ctx, project)
		if err != nil {
			return res, err
		}
		res.ProjectID, res.DefaultBranchID = project, branch

		dbs, _, err := lk.databasesOn(ctx, project, branch)
		if err != nil {
			return res, err
		}
		for _, name := range neonSortedDatabases(d) {
			db, ok := neonFindDatabase(dbs, name)
			if !ok {
				probs.add("V-N21", fmt.Sprintf("neon.databases[%s]", name),
					fmt.Sprintf("database %s does not exist on the default branch of %s", name, project),
					"create it in Neon, or set create: true")
				continue
			}
			if db.OwnerName != n.OwnerRole {
				probs.add("V-N23", fmt.Sprintf("neon.databases[%s]", name),
					fmt.Sprintf("database %s is owned by %s, not by owner_role %s; grants by a non-owner do nothing", name, db.OwnerName, n.OwnerRole),
					fmt.Sprintf("set owner_role: %s, or make %s the owner of %s", db.OwnerName, n.OwnerRole, name))
			}
		}

		cred, ok, err := neonCredential(ctx, lk, &probs, project, branch, n.OwnerRole, strconv.Itoa(neonPGVersion(n)))
		if err != nil {
			return res, err
		}
		if probs.any() {
			return stop()
		}
		if ok {
			for _, alias := range neonSortedDesired(d) {
				res.Credentials[d.connections[alias].Database] = cred
			}
		}
	}

	// Connections in state that the render no longer has.
	for _, alias := range neonSortedConnections(st) {
		if d != nil {
			if _, ok := d.connections[alias]; ok {
				continue
			}
		}
		c := st.Connections[alias]
		alive, info, err := neonConnectionAlive(ctx, lk, c)
		if err != nil {
			return res, err
		}
		if !alive {
			res.Dropped = append(res.Dropped, c)
			module := neonAccessModuleAddress(alias) + "."
			res.Releases = append(res.Releases, neonReleaseSet(st, func(addr string) bool {
				return strings.HasPrefix(addr, module)
			})...)
			res.KeptAdd = append(res.KeptAdd, neonKeptRoles(st, c, info)...)
			continue
		}
		res.Retired = append(res.Retired, c)
		admin := c.AdminRole
		if admin == "" {
			admin = neonDefaultOwnerRole
		}
		pgv := strconv.Itoa(info.PGVersion)
		if d != nil && c.ProjectID == res.ProjectID {
			pgv = strconv.Itoa(neonPGVersion(d.neon))
		}
		cred, ok, err := neonCredential(ctx, lk, &probs, c.ProjectID, c.BranchID, admin, pgv)
		if err != nil {
			return res, err
		}
		if ok {
			res.Credentials[c.Database] = cred
		}
	}
	sort.Strings(res.Releases)

	// database_url parameters leaving with this deploy.
	for _, svc := range neonSortedSet(st.SSMServices) {
		if d == nil || !d.ssm[svc] {
			res.SSMExclude = append(res.SSMExclude, fmt.Sprintf("/%s/%s/%s/%s", env.Env, env.Project, svc, neonSSMParamName))
		}
	}

	if probs.any() {
		return stop()
	}
	return res, nil
}

// neonConnectionAlive reports whether a connection's database still exists
// on its recorded branch. A missing project, branch or database is not alive.
func neonConnectionAlive(ctx context.Context, lk *neonLookups, c NeonConnectionState) (bool, NeonProjectInfo, error) {
	if c.Database == "" || c.ProjectID == "" || c.BranchID == "" {
		return false, NeonProjectInfo{}, nil
	}
	info, found, err := lk.project(ctx, c.ProjectID)
	if err != nil || !found {
		return false, info, err
	}
	dbs, found, err := lk.databasesOn(ctx, c.ProjectID, c.BranchID)
	if err != nil || !found {
		return false, info, err
	}
	_, ok := neonFindDatabase(dbs, c.Database)
	return ok, info, nil
}

// neonCredential resolves the direct read_write host and role's password on
// a branch. A missing endpoint is V-N24 and an unrevealable password V-N25,
// both added to probs; ok is false then.
func neonCredential(ctx context.Context, lk *neonLookups, probs *neonProblems, project, branch, role, pgVersion string) (PGCredential, bool, error) {
	host, err := lk.readWriteHost(ctx, project, branch)
	if err != nil {
		return PGCredential{}, false, err
	}
	if host == "" {
		probs.add("V-N24", "neon",
			fmt.Sprintf("branch %s of %s has no read_write endpoint", branch, project),
			"add a read_write compute endpoint to the default branch in Neon")
		return PGCredential{}, false, nil
	}
	password, err := lk.api.RevealRolePassword(ctx, project, branch, role)
	if err != nil {
		probs.add("V-N25", "neon.owner_role",
			fmt.Sprintf("the password of %s on %s cannot be revealed (%s)", role, project, redactSecrets(err.Error())),
			"check owner_role and that the Neon login may read the project's roles")
		return PGCredential{}, false, nil
	}
	registerSecret(password)
	return PGCredential{Host: host, User: role, Password: password, PGVersion: pgVersion}, true, nil
}

// neonKeptRoles is one Kept entry per role of a dropped connection.
func neonKeptRoles(st NeonStateView, c NeonConnectionState, info NeonProjectInfo) []NeonKeptEntry {
	now := neonResolveNow().UTC()
	orgID := info.OrgID
	if orgID == "" && st.Created.Project != nil {
		orgID = st.Created.Project.OrgID
	}
	roles := make([]string, 0, len(c.Roles))
	for _, r := range c.Roles {
		roles = append(roles, r)
	}
	sort.Strings(roles)
	out := make([]NeonKeptEntry, 0, len(roles))
	for _, r := range roles {
		out = append(out, NeonKeptEntry{Kind: neonKeptRole, OrgID: orgID, ProjectID: c.ProjectID, BranchID: c.BranchID, Database: c.Database, Role: r, ReleasedAt: now})
	}
	return out
}

func neonServicePath(svc string) string {
	if svc == neonBackendKey {
		return "workload.backend_neon"
	}
	return fmt.Sprintf("services[%s].neon", svc)
}

func neonSortedConnections(st NeonStateView) []string {
	out := make([]string, 0, len(st.Connections))
	for alias := range st.Connections {
		out = append(out, alias)
	}
	sort.Strings(out)
	return out
}

func neonSortedCreated(st NeonStateView) []string {
	out := make([]string, 0, len(st.Created.Databases))
	for name := range st.Created.Databases {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func neonSortedDatabases(d *neonDesired) []string {
	out := make([]string, 0, len(d.databases))
	for name := range d.databases {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func neonSortedDesired(d *neonDesired) []string {
	out := make([]string, 0, len(d.connections))
	for alias := range d.connections {
		out = append(out, alias)
	}
	sort.Strings(out)
	return out
}
