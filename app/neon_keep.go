package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Neon data meroku created and then let go of.
//
// meroku never deletes a Neon project or database. When one it created leaves
// the YAML, or the env is destroyed, it is released from Terraform state and
// recorded in the Kept file, <project root>/.meroku/neon-kept/<env>.json, so
// the user can delete it by hand or reuse it. Service roles left behind in
// Neon (their database gone, or a destroy without a Neon login) are recorded
// there too. The file holds ids and names only.

const neonKeptDirName = "neon-kept"

// neonKeptPath is the Kept file of env under projectRoot.
func neonKeptPath(projectRoot, env string) string {
	return filepath.Join(projectRoot, ".meroku", neonKeptDirName, env+".json")
}

// neonKeptKey identifies an entry; ReleasedAt is not part of it.
func neonKeptKey(e NeonKeptEntry) string {
	return strings.Join([]string{e.Kind, e.ProjectID, e.BranchID, e.Database, e.Role}, "\x00")
}

// loadNeonKept reads env's Kept file. A missing file is no entries.
func loadNeonKept(projectRoot, env string) ([]NeonKeptEntry, error) {
	path := neonKeptPath(projectRoot, env)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("neon: reading %s: %w", path, err)
	}
	var entries []NeonKeptEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("neon: %s is not a list of kept Neon objects: %w", path, err)
	}
	return entries, nil
}

// recordNeonKept merges add into env's Kept file. An entry already there
// keeps its first release time. The write is atomic: a failure leaves the
// previous file, and the caller must then release nothing.
func recordNeonKept(projectRoot, env string, add []NeonKeptEntry) error {
	if len(add) == 0 {
		return nil
	}
	entries, err := loadNeonKept(projectRoot, env)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, e := range entries {
		seen[neonKeptKey(e)] = true
	}
	for _, e := range add {
		if seen[neonKeptKey(e)] {
			continue
		}
		seen[neonKeptKey(e)] = true
		entries = append(entries, e)
	}
	return saveNeonKept(projectRoot, env, entries)
}

// saveNeonKept replaces env's Kept file with entries; no entries removes it.
// Directory 0700, file 0600, under the git-ignored .meroku/.
func saveNeonKept(projectRoot, env string, entries []NeonKeptEntry) error {
	path := neonKeptPath(projectRoot, env)
	if len(entries) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("neon: removing %s: %w", path, err)
		}
		return nil
	}
	if _, err := ensureMerokuDir(projectRoot); err != nil {
		return fmt.Errorf("neon: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("neon: creating %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("neon: securing %s: %w", dir, err)
	}
	sortNeonKept(entries)
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("neon: encoding the Kept file: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "."+env+".json.*")
	if err != nil {
		return fmt.Errorf("neon: writing %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("neon: writing %s: %w", path, err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("neon: writing %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("neon: writing %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("neon: writing %s: %w", path, err)
	}
	return nil
}

func sortNeonKept(entries []NeonKeptEntry) {
	rank := map[string]int{neonKeptProject: 0, neonKeptDatabase: 1, neonKeptRole: 2}
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.ProjectID != b.ProjectID {
			return a.ProjectID < b.ProjectID
		}
		if rank[a.Kind] != rank[b.Kind] {
			return rank[a.Kind] < rank[b.Kind]
		}
		if a.Database != b.Database {
			return a.Database < b.Database
		}
		return a.Role < b.Role
	})
}

// pruneNeonKept drops the entries env uses as data again and those a
// successful Neon read proves gone. A failed read, including a 404 from a
// token that cannot see the project, proves nothing and prunes nothing.
// api may be nil (no Neon login): then only the data-again rule applies.
func pruneNeonKept(ctx context.Context, api neonAPI, env *Env, entries []NeonKeptEntry) []NeonKeptEntry {
	var n *Neon
	if env != nil {
		n = normalizeNeon(env.Neon)
	}
	projectsByOrg := map[string]map[string]bool{} // org -> project ids, nil when the list failed
	listed := map[string]bool{}
	projectVisible := map[string]bool{}
	projectChecked := map[string]bool{}
	databases := map[string]map[string]bool{} // project/branch -> names, nil when the list failed
	roles := map[string]map[string]bool{}

	projectOK := func(id string) bool {
		if !projectChecked[id] {
			projectChecked[id] = true
			_, err := api.GetProject(ctx, id)
			projectVisible[id] = err == nil
		}
		return projectVisible[id]
	}

	var out []NeonKeptEntry
	for _, e := range entries {
		if neonKeptUsedAgain(n, e) {
			continue
		}
		if api == nil {
			out = append(out, e)
			continue
		}
		gone := false
		switch e.Kind {
		case neonKeptProject:
			if e.OrgID == "" {
				break
			}
			if !listed[e.OrgID] {
				listed[e.OrgID] = true
				if ps, err := api.ListProjects(ctx, e.OrgID); err == nil {
					ids := map[string]bool{}
					for _, p := range ps {
						ids[p.ID] = true
					}
					projectsByOrg[e.OrgID] = ids
				}
			}
			if ids := projectsByOrg[e.OrgID]; ids != nil {
				gone = !ids[e.ProjectID]
			}
		case neonKeptDatabase:
			if e.BranchID == "" || !projectOK(e.ProjectID) {
				break
			}
			key := e.ProjectID + "/" + e.BranchID
			if _, ok := databases[key]; !ok {
				databases[key] = nil
				if dbs, err := api.ListDatabases(ctx, e.ProjectID, e.BranchID); err == nil {
					names := map[string]bool{}
					for _, d := range dbs {
						names[d.Name] = true
					}
					databases[key] = names
				}
			}
			if names := databases[key]; names != nil {
				gone = !names[e.Database]
			}
		case neonKeptRole:
			if e.BranchID == "" {
				break
			}
			key := e.ProjectID + "/" + e.BranchID
			if _, ok := roles[key]; !ok {
				roles[key] = nil
				if rs, err := api.ListRoles(ctx, e.ProjectID, e.BranchID); err == nil {
					names := map[string]bool{}
					for _, r := range rs {
						names[r.Name] = true
					}
					roles[key] = names
				}
			}
			if names := roles[key]; names != nil {
				gone = !names[e.Role]
			}
		}
		if !gone {
			out = append(out, e)
		}
	}
	return out
}

// neonKeptUsedAgain reports whether the YAML uses a kept project or database
// as data again.
func neonKeptUsedAgain(n *Neon, e NeonKeptEntry) bool {
	if n == nil || n.Mode != NeonModeReuse || n.ProjectID != e.ProjectID {
		return false
	}
	switch e.Kind {
	case neonKeptProject:
		return true
	case neonKeptDatabase:
		for _, db := range n.Databases {
			if db.Name == e.Database && !db.Create {
				return true
			}
		}
	}
	return false
}

// neonKeptReport is the printed report for entries. Only the most enclosing
// object is shown: a database or role whose project is kept too is left out.
// Empty when there is nothing to report.
func neonKeptReport(entries []NeonKeptEntry) []string {
	if len(entries) == 0 {
		return nil
	}
	sorted := append([]NeonKeptEntry(nil), entries...)
	sortNeonKept(sorted)
	projects := map[string]bool{}
	for _, e := range sorted {
		if e.Kind == neonKeptProject {
			projects[e.ProjectID] = true
		}
	}
	cli := neonCLI()
	var cmds []string
	reuse := ""
	for _, e := range sorted {
		switch e.Kind {
		case neonKeptProject:
			cmds = append(cmds, fmt.Sprintf("  %s projects delete %s   # project", cli, e.ProjectID))
			if reuse == "" {
				reuse = e.ProjectID
			}
		case neonKeptDatabase:
			if projects[e.ProjectID] {
				continue
			}
			cmds = append(cmds, fmt.Sprintf("  %s databases delete %s --project-id %s --branch %s   # database", cli, e.Database, e.ProjectID, e.BranchID))
			if reuse == "" {
				reuse = e.ProjectID
			}
		case neonKeptRole:
			if projects[e.ProjectID] {
				continue
			}
			cmds = append(cmds, fmt.Sprintf("  %s roles delete %s --project-id %s --branch %s   # role", cli, e.Role, e.ProjectID, e.BranchID))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	lines := []string{
		"Kept in Neon — meroku never deletes Neon data. To delete it, run by hand. This deletes the",
		"data permanently, and meroku does not check whether another env uses it:",
	}
	lines = append(lines, cmds...)
	if reuse != "" {
		lines = append(lines,
			fmt.Sprintf("To use it again instead, set mode: reuse with project_id: %s, and create: false on", reuse),
			"its databases.")
	}
	return lines
}

// printNeonKeptReport prints env's Kept report, if any, from the file.
func printNeonKeptReport(projectRoot, env string) {
	entries, err := loadNeonKept(projectRoot, env)
	if err != nil {
		fmt.Printf("⚠️  %v\n", err)
		return
	}
	lines := neonKeptReport(entries)
	if len(lines) == 0 {
		return
	}
	fmt.Println()
	for _, l := range lines {
		fmt.Println(l)
	}
	fmt.Println()
}

// handleNeonCommand is `meroku neon kept <env> [--forget]`, run from the
// project root. It prints the Kept file, or with --forget deletes it.
func handleNeonCommand(args []string) error {
	if len(args) < 2 || args[0] != "kept" {
		return errors.New("usage: meroku neon kept <env> [--forget]")
	}
	env := args[1]
	forget := false
	for _, a := range args[2:] {
		switch a {
		case "--forget", "-forget":
			forget = true
		default:
			return fmt.Errorf("unknown option %q; usage: meroku neon kept <env> [--forget]", a)
		}
	}
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("reading the current directory: %w", err)
	}
	if forget {
		if err := saveNeonKept(root, env, nil); err != nil {
			return err
		}
		fmt.Printf("Forgot the Neon objects kept for %s. They are still in Neon.\n", env)
		return nil
	}
	entries, err := loadNeonKept(root, env)
	if err != nil {
		return err
	}
	lines := neonKeptReport(entries)
	if len(lines) == 0 {
		fmt.Printf("Nothing kept in Neon for %s.\n", env)
		return nil
	}
	for _, l := range lines {
		fmt.Println(l)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Terraform's refusal to destroy Neon data
// ---------------------------------------------------------------------------

var neonPreventDestroyRe = regexp.MustCompile(`Resource\s+(module\.neon\.[^\s,]+)\s+has\s+lifecycle\.prevent_destroy\s+set`)

// neonPreventDestroyError finds Terraform's prevent_destroy refusal for an
// address under module.neon in a failed plan's output.
func neonPreventDestroyError(output string) (address string, ok bool) {
	clean := stripAnsiEscapeCodes(output)
	if !strings.Contains(clean, "Instance cannot be destroyed") {
		return "", false
	}
	m := neonPreventDestroyRe.FindStringSubmatch(clean)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// neonRefusalMessage explains a refused replacement of a created project or
// database and how to get out of it.
func neonRefusalMessage(address, envFile string) string {
	fields := "neon.owner_role"
	if strings.HasPrefix(address, "module.neon.neon_project.") {
		fields = "neon.project.org_id, neon.project.region_id, neon.project.pg_version and neon.owner_role"
	}
	return fmt.Sprintf("neon: this change would replace %s and delete its data. meroku never deletes Neon data. "+
		"Revert %s. To start over, remove it from %s and deploy (meroku keeps it and prints the command to delete it), "+
		"delete it, then add it back.", address, fields, envFile)
}

// ---------------------------------------------------------------------------
// Service roles in use, and search_path changes
// ---------------------------------------------------------------------------

var neonRoleInUseRe = regexp.MustCompile(`role "([^"]+)" cannot be dropped because some objects depend on it`)

// neonRolesInUse are the roles a failed Terraform run could not drop because
// objects still depend on them (SQLSTATE 2BP01).
func neonRolesInUse(output string) []string {
	clean := stripAnsiEscapeCodes(output)
	seen := map[string]bool{}
	var roles []string
	for _, m := range neonRoleInUseRe.FindAllStringSubmatch(clean, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			roles = append(roles, m[1])
		}
	}
	sort.Strings(roles)
	return roles
}

// neonTerminateSQL ends every session of role.
func neonTerminateSQL(role string) string {
	return fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE usename = '%s';", strings.ReplaceAll(role, "'", "''"))
}

// neonRoleInUseRemedy is what to do when DROP ROLE failed with 2BP01: the
// role still has sessions (in a reused database, temp tables held by pooled
// connections). Empty when output shows no such failure.
func neonRoleInUseRemedy(output, ownerRole string) string {
	roles := neonRolesInUse(output)
	if len(roles) == 0 {
		return ""
	}
	if ownerRole == "" {
		ownerRole = neonDefaultOwnerRole
	}
	var b strings.Builder
	b.WriteString("neon: Postgres refused to drop ")
	b.WriteString(strings.Join(roles, ", "))
	b.WriteString(" (SQLSTATE 2BP01): open sessions of the role still hold objects, such as temporary\n")
	b.WriteString("tables kept alive by pooled connections. Connect to that database as ")
	b.WriteString(ownerRole)
	b.WriteString(" and run:\n")
	for _, r := range roles {
		b.WriteString("  ")
		b.WriteString(neonTerminateSQL(r))
		b.WriteString("\n")
	}
	b.WriteString("then run meroku again.\n")
	return b.String()
}

// neonSearchPathChanges are the service roles whose search_path the plan
// changes in place.
func neonSearchPathChanges(changes []ResourceChange) []string {
	var roles []string
	for _, rc := range changes {
		if rc.Type != "postgresql_role" || !strings.HasPrefix(rc.Address, neonAccessModulePrefix) {
			continue
		}
		if len(rc.Change.Actions) != 1 || rc.Change.Actions[0] != "update" {
			continue
		}
		before, _ := json.Marshal(rc.Change.Before["search_path"])
		after, _ := json.Marshal(rc.Change.After["search_path"])
		if string(before) == string(after) {
			continue
		}
		name, _ := rc.Change.After["name"].(string)
		if name == "" {
			name, _ = rc.Change.Before["name"].(string)
		}
		if name != "" {
			roles = append(roles, name)
		}
	}
	sort.Strings(roles)
	return roles
}

// neonSearchPathNotice tells the user when a changed search_path reaches
// pooled connections, and how to apply it at once. Empty without roles.
func neonSearchPathNotice(roles []string, ownerRole string) string {
	if len(roles) == 0 {
		return ""
	}
	if ownerRole == "" {
		ownerRole = neonDefaultOwnerRole
	}
	var b strings.Builder
	b.WriteString("ℹ️  The schemas (search_path) of ")
	b.WriteString(strings.Join(roles, ", "))
	b.WriteString(" changed. The services connect through Neon's pooled host,\n")
	b.WriteString("   and the change reaches pooled connections once Neon's pooler recycles its server\n")
	b.WriteString("   connections. To apply it now, connect as ")
	b.WriteString(ownerRole)
	b.WriteString(" and run:\n")
	for _, r := range roles {
		b.WriteString("     ")
		b.WriteString(neonTerminateSQL(r))
		b.WriteString("\n")
	}
	return b.String()
}
