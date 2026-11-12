package main

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v2"
)

// neonRenderKey is the envMap key env/main.hbs reads for everything Neon. It
// is set by preprocessEnvMap only when the env YAML has a top-level `neon:`
// block, so an AWS-only render never sees it.
const neonRenderKey = "neon_render"

const (
	NeonModeCreate = "create"
	NeonModeReuse  = "reuse"

	NeonAccessReadonly  = "readonly"
	NeonAccessReadwrite = "readwrite"

	// neonDefaultOwnerRole is the role Neon creates with every project. It
	// owns every database, table, sequence and schema meroku serves.
	neonDefaultOwnerRole = "neondb_owner"
	// neonDefaultPGVersion is the Postgres major version of a created project
	// whose YAML names none.
	neonDefaultPGVersion = 17
	// neonDefaultSchema is a service's schema list when it names none.
	neonDefaultSchema = "public"
	// neonProjectDatabase is the database Neon creates with every project. In
	// create mode it already exists, so it can only be read as data.
	neonProjectDatabase = "neondb"
	// neonBackendKey is the backend's key in role names, SSM paths and the
	// template input; a named service uses its name.
	neonBackendKey = "backend"
	// neonRoleNameBudget is how much of a role name the readable prefix may
	// use: 63 bytes (Postgres NAMEDATALEN-1) minus "_" and the 8-hex id.
	neonRoleNameBudget = 54
)

var (
	neonReProject = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,62}$`)
	neonReOrg     = regexp.MustCompile(`^org-[a-z0-9-]{3,64}$`)
	neonReRegion  = regexp.MustCompile(`^(aws|azure)-[a-z0-9-]+$`)
	neonReIdent   = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

	neonSanitizeChars = regexp.MustCompile(`[^a-z0-9]`)
	neonSanitizeRuns  = regexp.MustCompile(`_+`)
)

// NeonProblem is one broken generate-time rule. Rule is the rule id (V-N1,
// V-N2, ...), Path the YAML path it was found at.
type NeonProblem struct {
	Rule    string
	Path    string
	Problem string
	Fix     string
}

// String renders the problem as one line: `neon: <path>: <problem> — <fix>`,
// followed by the rule id in brackets.
func (p NeonProblem) String() string {
	return fmt.Sprintf("neon: %s: %s — %s [%s]", p.Path, p.Problem, p.Fix, p.Rule)
}

// NeonConfigError carries every broken rule of one env at once.
type NeonConfigError struct {
	Problems []NeonProblem
}

func (e *NeonConfigError) Error() string {
	lines := make([]string, 0, len(e.Problems))
	for _, p := range e.Problems {
		lines = append(lines, p.String())
	}
	return "invalid Neon settings:\n  " + strings.Join(lines, "\n  ")
}

// Rules returns the distinct rule ids in the error, sorted.
func (e *NeonConfigError) Rules() []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range e.Problems {
		if !seen[p.Rule] {
			seen[p.Rule] = true
			out = append(out, p.Rule)
		}
	}
	sort.Strings(out)
	return out
}

// neonRules returns the rule ids an error from validateNeonConfigMap carries,
// or nil for a nil or unrelated error.
func neonRules(err error) []string {
	var cfgErr *NeonConfigError
	if errors.As(err, &cfgErr) {
		return cfgErr.Rules()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Snapshot
// ---------------------------------------------------------------------------

// snapshotNeonAccess copies, from envMap, exactly what the Neon rules and the
// template input read: the meroku project and env names, the top-level
// `neon:` block, the backend's `backend_neon` and env variables, and every
// service's name, `neon` block and env variables.
//
// It runs before filterDisabledItems, so a service with `enabled: false` is
// in the snapshot: switching a service off must not drop its role or its SSM
// parameter. The copy holds its own maps, so stripping the service `neon`
// keys from envMap afterwards leaves it intact.
func snapshotNeonAccess(envMap map[string]interface{}) map[string]interface{} {
	snap := map[string]interface{}{}
	for _, key := range []string{"project", "env", "neon"} {
		if v, ok := envMap[key]; ok {
			snap[key] = v
		}
	}

	if workload, ok := yamlSubMap(envMap, "workload"); ok {
		w := map[string]interface{}{}
		for _, key := range []string{"backend_neon", "backend_env_variables"} {
			if v, ok := workload[key]; ok {
				w[key] = v
			}
		}
		snap["workload"] = w
	}

	if raw, ok := envMap["services"]; ok && raw != nil {
		if list, ok := raw.([]interface{}); ok {
			services := make([]interface{}, 0, len(list))
			for _, item := range list {
				svc, ok := computeNodeMap(item)
				if !ok {
					services = append(services, item)
					continue
				}
				s := map[string]interface{}{}
				for _, key := range []string{"name", "neon", "env_vars", "env_variables"} {
					if v, ok := svc[key]; ok {
						s[key] = v
					}
				}
				services = append(services, s)
			}
			snap["services"] = services
		} else {
			snap["services"] = raw
		}
	}
	return snap
}

// stripNeonAccessKeys removes `workload.backend_neon` and every
// `services[].neon` from envMap, after the template input has been built from
// the snapshot. The template reads Neon only through neon_render.
func stripNeonAccessKeys(envMap map[string]interface{}) {
	switch w := envMap["workload"].(type) {
	case map[string]interface{}:
		delete(w, "backend_neon")
	case map[interface{}]interface{}:
		delete(w, "backend_neon")
	}
	if list, ok := envMap["services"].([]interface{}); ok {
		for _, item := range list {
			switch s := item.(type) {
			case map[string]interface{}:
				delete(s, "neon")
			case map[interface{}]interface{}:
				delete(s, "neon")
			}
		}
	}
}

// applyNeonPreprocess is the Neon part of preprocessEnvMap: snapshot,
// validate, build neon_render, strip the service `neon` keys. With no Neon
// anywhere it changes nothing in envMap.
func applyNeonPreprocess(envMap map[string]interface{}) error {
	snap := snapshotNeonAccess(envMap)
	if err := validateNeonConfigMap(snap); err != nil {
		return err
	}
	render, err := buildNeonRender(snap)
	if err != nil {
		return err
	}
	if render != nil {
		envMap[neonRenderKey] = render
	}
	stripNeonAccessKeys(envMap)
	return nil
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// neonServiceAccess is one service's raw `neon` block with where it was found.
type neonServiceAccess struct {
	key    string // "backend" or the service name
	path   string // YAML path of the block
	raw    interface{}
	envSet bool // the service sets DATABASE_URL in its env variables
}

// neonServiceAccesses lists every backend and named-service `neon` block in
// envMap, in file order, the backend first.
func neonServiceAccesses(envMap map[string]interface{}) []neonServiceAccess {
	var out []neonServiceAccess

	if workload, ok := yamlSubMap(envMap, "workload"); ok {
		if raw, present := workload["backend_neon"]; present && raw != nil {
			out = append(out, neonServiceAccess{
				key:    neonBackendKey,
				path:   "workload.backend_neon",
				raw:    raw,
				envSet: neonMapHasKey(workload["backend_env_variables"], "DATABASE_URL"),
			})
		}
	}

	if list, ok := envMap["services"].([]interface{}); ok {
		for i, item := range list {
			svc, ok := computeNodeMap(item)
			if !ok {
				continue
			}
			raw, present := svc["neon"]
			if !present || raw == nil {
				continue
			}
			name := computeStringField(svc, "name")
			label := name
			if label == "" {
				label = fmt.Sprintf("%d", i)
			}
			out = append(out, neonServiceAccess{
				key:  name,
				path: fmt.Sprintf("services[%s].neon", label),
				raw:  raw,
				envSet: neonMapHasKey(svc["env_vars"], "DATABASE_URL") ||
					neonEnvListHasName(svc["env_variables"], "DATABASE_URL"),
			})
		}
	}
	return out
}

func neonMapHasKey(raw interface{}, key string) bool {
	m, ok := computeNodeMap(raw)
	if !ok {
		return false
	}
	_, present := m[key]
	return present
}

func neonEnvListHasName(raw interface{}, name string) bool {
	list, ok := raw.([]interface{})
	if !ok {
		return false
	}
	for _, item := range list {
		if m, ok := computeNodeMap(item); ok && computeStringField(m, "name") == name {
			return true
		}
	}
	return false
}

// neonInt reads a YAML or JSON number that must be a whole number. YAML gives
// int; JSON gives float64, which counts only when it has no fraction.
func neonInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case uint64:
		return int(n), true
	case float64:
		if n == math.Trunc(n) && !math.IsInf(n, 0) {
			return int(n), true
		}
	}
	return 0, false
}

// neonString reads a string field; present reports whether the key is there
// with a non-nil value at all.
func neonString(m map[string]interface{}, key string) (value string, present bool, isString bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return "", false, false
	}
	s, isStr := v.(string)
	return s, true, isStr
}

// validateNeonConfigMap checks the generate-time Neon rules (V-N1 … V-N16,
// V-N28) on an env map and returns every problem at once as a
// *NeonConfigError, or nil.
//
// With no top-level `neon:` the only rule checked is V-N13: a backend or named
// service that asks for a Neon database in an env that declares none.
func validateNeonConfigMap(envMap map[string]interface{}) error {
	var problems []NeonProblem
	add := func(rule, path, problem, fix string) {
		problems = append(problems, NeonProblem{Rule: rule, Path: path, Problem: problem, Fix: fix})
	}

	accesses := neonServiceAccesses(envMap)

	neonRaw, hasNeon := envMap["neon"]
	if !hasNeon || neonRaw == nil {
		for _, a := range accesses {
			add("V-N13", a.path, "a Neon database is requested but the env has no top-level neon: block",
				"add a neon: block that declares the database, or remove "+a.path)
		}
		return neonResult(problems)
	}

	neon, ok := computeNodeMap(neonRaw)
	if !ok {
		add("V-N1", "neon", "must be a mapping with mode, databases and the project settings",
			"see ai_docs/NEON.md for the neon: block")
		return neonResult(problems)
	}

	mode, _, _ := neonString(neon, "mode")
	modeValid := mode == NeonModeCreate || mode == NeonModeReuse
	if !modeValid {
		problem := "mode is missing"
		if _, present := neon["mode"]; present && neon["mode"] != nil {
			problem = fmt.Sprintf("mode %s is not a mode", formatYAMLValue(neon["mode"]))
		}
		add("V-N1", "neon.mode", problem, "set mode: create (meroku creates the project) or mode: reuse (an existing project)")
	}

	_, hasProjectID := neon["project_id"]
	hasProjectID = hasProjectID && neon["project_id"] != nil
	pgRaw, hasPG := neon["pg_version"]
	hasPG = hasPG && pgRaw != nil
	projectRaw, hasProject := neon["project"]
	hasProject = hasProject && projectRaw != nil

	reusePG := 0
	switch mode {
	case NeonModeReuse:
		projectID, _, isStr := neonString(neon, "project_id")
		if !isStr || !neonReProject.MatchString(projectID) {
			add("V-N2", "neon.project_id", "reuse mode needs the id of an existing Neon project (lower-case letters, digits and hyphens)",
				fmt.Sprintf("set project_id to the project's id from the Neon console or `%s projects list`", neonCLI()))
		}
		if !hasPG {
			add("V-N2", "neon.pg_version", "reuse mode needs the project's Postgres major version",
				"set pg_version to 14, 15, 16, 17 or 18, as the project reports it")
		} else {
			v, isInt := neonInt(pgRaw)
			if isInt {
				reusePG = v
			}
			if !isInt || v < 14 || v > 18 {
				add("V-N2", "neon.pg_version", fmt.Sprintf("%s is not a supported Postgres version", formatYAMLValue(pgRaw)),
					"set pg_version to 14, 15, 16, 17 or 18, as the project reports it")
			}
		}
		if hasProject {
			add("V-N2", "neon.project", "reuse mode reads an existing project, so it takes no project block",
				"remove neon.project, or set mode: create")
		}
	case NeonModeCreate:
		if hasPG {
			add("V-N2", "neon.pg_version", "create mode takes the version from neon.project.pg_version",
				"move pg_version under neon.project")
		}
		if hasProjectID {
			add("V-N3", "neon.project_id", "create mode makes a new project, so it takes no project_id",
				"remove project_id, or set mode: reuse to use that project")
		}
		problems = append(problems, neonProjectProblems(projectRaw, hasProject)...)
	}

	if owner, present, isStr := neonString(neon, "owner_role"); present && (!isStr || (owner != "" && !neonReIdent.MatchString(owner))) {
		add("V-N15", "neon.owner_role", fmt.Sprintf("%s is not a Postgres role name", formatYAMLValue(neon["owner_role"])),
			"use lower-case letters, digits and underscores, starting with a letter or underscore (default neondb_owner)")
	}

	declared, dbProblems := neonDatabaseProblems(neon, mode)
	problems = append(problems, dbProblems...)
	databasesValid := len(dbProblems) == 0

	for _, a := range accesses {
		access, ok := computeNodeMap(a.raw)
		if !ok {
			add("V-N10", a.path, "must be a mapping with database and access", "set database: <name> and access: readonly or readwrite")
			continue
		}
		dbName, dbOK := neonAccessProblems(a.path, access, add)
		if dbOK && databasesValid && !declared[dbName] {
			add("V-N11", a.path+".database", fmt.Sprintf("database %q is not declared in neon.databases", dbName),
				"add it to neon.databases, or name a declared database")
		}
		if a.envSet {
			add("V-N14", a.path, "the service also sets DATABASE_URL in its env variables, which would hide the URL meroku writes",
				"remove DATABASE_URL from the service's env variables")
		}
	}

	if mode == NeonModeReuse && reusePG > 0 && reusePG < 15 && len(accesses) > 0 {
		add("V-N28", "neon.pg_version", fmt.Sprintf("Postgres %d lets every role create tables in public, so a service role could own tables", reusePG),
			"upgrade the project to Postgres 15 or later, or remove the service neon blocks")
	}

	if len(problems) == 0 {
		problems = append(problems, neonRoleNameProblems(envMap, accesses)...)
	}
	return neonResult(problems)
}

func neonResult(problems []NeonProblem) error {
	if len(problems) == 0 {
		return nil
	}
	return &NeonConfigError{Problems: problems}
}

// neonProjectProblems checks neon.project in create mode (V-N3).
func neonProjectProblems(raw interface{}, present bool) []NeonProblem {
	var out []NeonProblem
	add := func(path, problem, fix string) {
		out = append(out, NeonProblem{Rule: "V-N3", Path: path, Problem: problem, Fix: fix})
	}
	if !present {
		add("neon.project", "create mode needs a project block", "add neon.project with name, org_id and region_id")
		return out
	}
	project, ok := computeNodeMap(raw)
	if !ok {
		add("neon.project", "must be a mapping", "set name, org_id and region_id under neon.project")
		return out
	}
	if name, _, isStr := neonString(project, "name"); !isStr || strings.TrimSpace(name) == "" {
		add("neon.project.name", "the project needs a name", "set neon.project.name, for example <project>-<env>")
	}
	if org, _, isStr := neonString(project, "org_id"); !isStr || !neonReOrg.MatchString(org) {
		add("neon.project.org_id", "not a Neon organization id",
			fmt.Sprintf("set org_id to the org-… id from the Neon console or `%s orgs list`", neonCLI()))
	}
	if region, _, isStr := neonString(project, "region_id"); !isStr || !neonReRegion.MatchString(region) {
		add("neon.project.region_id", "not a Neon region id", "use a Neon region such as aws-us-east-1")
	}
	if pg, present := project["pg_version"]; present && pg != nil {
		if v, isInt := neonInt(pg); !isInt || v < 15 || v > 18 {
			add("neon.project.pg_version", fmt.Sprintf("%s is not a version meroku creates", formatYAMLValue(pg)),
				"set pg_version to 15, 16, 17 or 18 (default 17)")
		}
	}
	return out
}

// neonDatabaseProblems checks neon.databases (V-N8, V-N9) and returns the set
// of declared names.
func neonDatabaseProblems(neon map[string]interface{}, mode string) (map[string]bool, []NeonProblem) {
	declared := map[string]bool{}
	var out []NeonProblem

	raw, present := neon["databases"]
	if !present || raw == nil {
		return declared, nil
	}
	list, ok := raw.([]interface{})
	if !ok {
		out = append(out, NeonProblem{Rule: "V-N8", Path: "neon.databases", Problem: "must be a list",
			Fix: "list each database as - name: <name>"})
		return declared, out
	}

	for i, item := range list {
		path := fmt.Sprintf("neon.databases[%d]", i)
		db, ok := computeNodeMap(item)
		if !ok {
			out = append(out, NeonProblem{Rule: "V-N8", Path: path, Problem: "must be a mapping with a name",
				Fix: "write it as - name: <name>"})
			continue
		}
		name, _, isStr := neonString(db, "name")
		if !isStr || !neonReIdent.MatchString(name) {
			out = append(out, NeonProblem{Rule: "V-N8", Path: path + ".name",
				Problem: fmt.Sprintf("%s is not a Postgres database name", formatYAMLValue(db["name"])),
				Fix:     "use lower-case letters, digits and underscores, starting with a letter or underscore"})
			continue
		}
		if declared[name] {
			out = append(out, NeonProblem{Rule: "V-N8", Path: path + ".name",
				Problem: fmt.Sprintf("database %q is listed twice", name), Fix: "list each database once"})
			continue
		}
		declared[name] = true

		create := false
		if v, present := db["create"]; present && v != nil {
			b, isBool := v.(bool)
			if !isBool {
				out = append(out, NeonProblem{Rule: "V-N8", Path: path + ".create",
					Problem: fmt.Sprintf("%s is not true or false", formatYAMLValue(v)),
					Fix:     "set create: true (meroku creates it) or create: false (it exists)"})
				continue
			}
			create = b
		}
		if mode == NeonModeCreate && name == neonProjectDatabase && create {
			out = append(out, NeonProblem{Rule: "V-N9", Path: path + ".create",
				Problem: "a new Neon project already comes with the neondb database",
				Fix:     "set create: false on neondb"})
		}
	}
	return declared, out
}

// neonAccessProblems checks one service `neon` block (V-N10) and returns its
// database name and whether that name is valid.
func neonAccessProblems(path string, access map[string]interface{}, add func(rule, path, problem, fix string)) (string, bool) {
	dbName, _, isStr := neonString(access, "database")
	dbOK := isStr && neonReIdent.MatchString(dbName)
	if !dbOK {
		problem := "database is missing"
		if v, present := access["database"]; present && v != nil {
			problem = fmt.Sprintf("%s is not a Postgres database name", formatYAMLValue(v))
		}
		add("V-N10", path+".database", problem, "name one of the databases in neon.databases")
	}

	if a, _, isStr := neonString(access, "access"); !isStr || (a != NeonAccessReadonly && a != NeonAccessReadwrite) {
		problem := "access is missing"
		if v, present := access["access"]; present && v != nil {
			problem = fmt.Sprintf("access %s is not an access level", formatYAMLValue(v))
		}
		add("V-N10", path+".access", problem, "set access: readonly or access: readwrite")
	}

	if raw, present := access["schemas"]; present && raw != nil {
		list, ok := raw.([]interface{})
		switch {
		case !ok:
			add("V-N10", path+".schemas", "must be a list of schema names", "write schemas: [public] or leave it out")
		case len(list) == 0:
			add("V-N10", path+".schemas", "an empty list grants nothing", "list at least one schema, or leave schemas out for [public]")
		default:
			seen := map[string]bool{}
			for _, item := range list {
				s, isStr := item.(string)
				switch {
				case !isStr || !neonReIdent.MatchString(s):
					add("V-N10", path+".schemas", fmt.Sprintf("%s is not a schema name", formatYAMLValue(item)),
						"use lower-case letters, digits and underscores, starting with a letter or underscore")
				case strings.HasPrefix(s, "pg_") || s == "information_schema":
					add("V-N10", path+".schemas", fmt.Sprintf("%s is a Postgres system schema", s),
						"list only your application's schemas")
				case seen[s]:
					add("V-N10", path+".schemas", fmt.Sprintf("schema %q is listed twice", s), "list each schema once")
				}
				if isStr {
					seen[s] = true
				}
			}
		}
	}

	if raw, present := access["password_version"]; present && raw != nil {
		if v, isInt := neonInt(raw); !isInt || v < 1 {
			add("V-N10", path+".password_version", fmt.Sprintf("%s is not a whole number of 1 or more", formatYAMLValue(raw)),
				"leave it out (1), or raise it by one to rotate the password")
		}
	}
	return dbName, dbOK
}

// neonRoleNameProblems is V-N16: two services whose role names coincide on
// the same database. Different tuples differ in the 8-hex id, so this fires
// only on a 32-bit collision, or on two services with the same key.
func neonRoleNameProblems(envMap map[string]interface{}, accesses []neonServiceAccess) []NeonProblem {
	project := computeStringField(envMap, "project")
	env := computeStringField(envMap, "env")

	var out []NeonProblem
	owner := map[string]string{}
	for _, a := range accesses {
		access, ok := computeNodeMap(a.raw)
		if !ok {
			continue
		}
		db := computeStringField(access, "database")
		name := neonRoleName(project, env, a.key, db)
		if first, dup := owner[name]; dup {
			out = append(out, NeonProblem{Rule: "V-N16", Path: a.path,
				Problem: fmt.Sprintf("its role name %s is the same as %s's", name, first),
				Fix:     "rename one of the services"})
			continue
		}
		owner[name] = a.path
	}
	return out
}

// ---------------------------------------------------------------------------
// Names
// ---------------------------------------------------------------------------

// neonSanitize lower-cases s, turns every character outside [a-z0-9] into _,
// and collapses runs of _ to one.
func neonSanitize(s string) string {
	s = neonSanitizeChars.ReplaceAllString(strings.ToLower(s), "_")
	return neonSanitizeRuns.ReplaceAllString(s, "_")
}

// neonRoleName is the Postgres role of one (env, service, database). The
// readable prefix is for people; the id, from the raw names joined with NUL,
// keeps tuples apart whose prefixes coincide. The result is at most 63 bytes
// and matches ^[a-z_][a-z0-9_]*$.
func neonRoleName(project, env, svc, db string) string {
	sum := sha256.Sum256([]byte(project + "\x00" + env + "\x00" + svc + "\x00" + db))
	id := hex.EncodeToString(sum[:])[:8]

	prefix := strings.Join([]string{neonSanitize(project), neonSanitize(env), neonSanitize(svc), neonSanitize(db)}, "_")
	if prefix != "" && prefix[0] >= '0' && prefix[0] <= '9' {
		prefix = "r_" + prefix
	}
	if len(prefix) > neonRoleNameBudget {
		prefix = prefix[:neonRoleNameBudget]
	}
	return strings.TrimRight(prefix, "_") + "_" + id
}

// neonConnectionAlias is the provider alias and module suffix of a database's
// connection: readable, and unique per database name.
func neonConnectionAlias(db string) string {
	sum := md5.Sum([]byte(db))
	return "neon_" + neonSanitize(db) + "_" + hex.EncodeToString(sum[:])[:6]
}

// ---------------------------------------------------------------------------
// Defaults and the template input
// ---------------------------------------------------------------------------

// normalizeNeon returns a copy of n with the defaults the YAML may leave out:
// owner_role neondb_owner and, in create mode, project.pg_version 17. nil
// stays nil. The YAML itself is never rewritten with them.
func normalizeNeon(n *Neon) *Neon {
	if n == nil {
		return nil
	}
	out := *n
	if out.OwnerRole == "" {
		out.OwnerRole = neonDefaultOwnerRole
	}
	if n.Project != nil {
		p := *n.Project
		if out.Mode == NeonModeCreate && p.PGVersion == 0 {
			p.PGVersion = neonDefaultPGVersion
		}
		out.Project = &p
	}
	out.Databases = append([]NeonDatabase(nil), n.Databases...)
	return &out
}

// neonPGVersion is the project's Postgres major version: project.pg_version
// in create mode, pg_version in reuse mode.
func neonPGVersion(n *Neon) int {
	n = normalizeNeon(n)
	if n == nil {
		return 0
	}
	if n.Mode == NeonModeCreate {
		if n.Project == nil {
			return neonDefaultPGVersion
		}
		return n.Project.PGVersion
	}
	return n.PGVersion
}

// normalizeNeonAccess returns a copy of a with schemas [public] and
// password_version 1 when the YAML leaves them out.
func normalizeNeonAccess(a NeonAccess) NeonAccess {
	if len(a.Schemas) == 0 {
		a.Schemas = []string{neonDefaultSchema}
	} else {
		a.Schemas = append([]string(nil), a.Schemas...)
	}
	if a.PasswordVersion == 0 {
		a.PasswordVersion = 1
	}
	return a
}

// neonConfigDoc is the typed view of a snapshot, for the template input.
type neonConfigDoc struct {
	Project  string `yaml:"project"`
	Env      string `yaml:"env"`
	Neon     *Neon  `yaml:"neon"`
	Workload struct {
		BackendNeon *NeonAccess `yaml:"backend_neon"`
	} `yaml:"workload"`
	Services []struct {
		Name string      `yaml:"name"`
		Neon *NeonAccess `yaml:"neon"`
	} `yaml:"services"`
}

// neonServiceUse is one service's access with its key.
type neonServiceUse struct {
	key    string
	access NeonAccess
}

// buildNeonRender builds the `neon_render` template input from a validated
// snapshot (or a whole env map). It returns nil when the env has no top-level
// `neon:` block.
//
// Every value in it is a plain map, list, string, int or bool, so the
// template helpers (json, hclArray, hclString) render it as they render the
// rest of the YAML. Maps are keyed by name, and lists are sorted, so a render
// is the same on every run.
func buildNeonRender(envMap map[string]interface{}) (map[string]interface{}, error) {
	if raw, ok := envMap["neon"]; !ok || raw == nil {
		return nil, nil
	}

	encoded, err := yaml.Marshal(envMap)
	if err != nil {
		return nil, fmt.Errorf("neon: reading the neon settings: %w", err)
	}
	var doc neonConfigDoc
	if err := yaml.Unmarshal(encoded, &doc); err != nil {
		return nil, fmt.Errorf("neon: reading the neon settings: %w", err)
	}
	n := normalizeNeon(doc.Neon)

	var uses []neonServiceUse
	if doc.Workload.BackendNeon != nil {
		uses = append(uses, neonServiceUse{key: neonBackendKey, access: normalizeNeonAccess(*doc.Workload.BackendNeon)})
	}
	for _, s := range doc.Services {
		if s.Neon != nil {
			uses = append(uses, neonServiceUse{key: s.Name, access: normalizeNeonAccess(*s.Neon)})
		}
	}

	databases := map[string]interface{}{}
	created := map[string]bool{}
	for _, db := range n.Databases {
		databases[db.Name] = map[string]interface{}{"create": db.Create}
		if db.Create {
			created[db.Name] = true
		}
	}

	// Connections: every database a service uses, plus every database meroku
	// creates, because each created database has PUBLIC's CONNECT and
	// TEMPORARY revoked through its connection even when no service uses it.
	byDB := map[string]map[string]interface{}{}
	for name := range created {
		byDB[name] = map[string]interface{}{}
	}
	ssmServices := make([]interface{}, 0, len(uses))
	ssmKeys := make([]string, 0, len(uses))
	for _, u := range uses {
		services, ok := byDB[u.access.Database]
		if !ok {
			services = map[string]interface{}{}
			byDB[u.access.Database] = services
		}
		schemas := make([]interface{}, 0, len(u.access.Schemas))
		for _, s := range u.access.Schemas {
			schemas = append(schemas, s)
		}
		services[u.key] = map[string]interface{}{
			"schemas":          schemas,
			"access":           u.access.Access,
			"role_name":        neonRoleName(doc.Project, doc.Env, u.key, u.access.Database),
			"password_version": u.access.PasswordVersion,
		}
		ssmKeys = append(ssmKeys, u.key)
	}
	sort.Strings(ssmKeys)
	for _, k := range ssmKeys {
		ssmServices = append(ssmServices, k)
	}

	dbNames := make([]string, 0, len(byDB))
	for name := range byDB {
		dbNames = append(dbNames, name)
	}
	sort.Strings(dbNames)
	connections := make([]interface{}, 0, len(dbNames))
	for _, name := range dbNames {
		connections = append(connections, map[string]interface{}{
			"database":      name,
			"alias":         neonConnectionAlias(name),
			"revoke_public": created[name],
			"services":      byDB[name],
		})
	}

	var project interface{}
	if n.Mode == NeonModeCreate && n.Project != nil {
		project = map[string]interface{}{
			"name":       n.Project.Name,
			"org_id":     n.Project.OrgID,
			"region_id":  n.Project.RegionID,
			"pg_version": n.Project.PGVersion,
		}
	}

	return map[string]interface{}{
		"mode":         n.Mode,
		"project_id":   n.ProjectID,
		"pg_version":   neonPGVersion(n),
		"project":      project,
		"owner_role":   n.OwnerRole,
		"databases":    databases,
		"connections":  connections,
		"ssm_services": ssmServices,
	}, nil
}
