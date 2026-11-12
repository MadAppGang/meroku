package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Deploy-time files in env/<env>/.
//
// main.tf depends only on the YAML. A connection that left the YAML still has
// roles in state, and Terraform needs a provider configuration to drop them.
// For the length of one deploy or destroy, meroku writes that configuration
// next to main.tf: neon_retired.tf holds the credentials variable (when
// main.tf lacks it) and one postgresql provider alias per retired connection;
// neon_retired_override.tf adds the postgresql entry to main.tf's
// required_providers (override files merge element by element, a second
// required_providers block in a normal file is an error) and the
// database_url names modules/workloads must stop reading. main.tf on disk is
// never edited. Both files are deleted when they are no longer needed.

const (
	neonRetiredFile         = "neon_retired.tf"
	neonRetiredOverrideFile = "neon_retired_override.tf"
	neonStep1PlanFile       = "neon-step1.tfplan"
	neonPostgresSource      = "cyrilgdn/postgresql"
	neonPostgresVersion     = "1.27.0"
)

var (
	neonReCredentialsVariable = regexp.MustCompile(`(?m)^\s*variable\s+"neon_pg_credentials"`)
	neonReAliasIdent          = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
	neonRePostgresSource      = regexp.MustCompile(`source\s*=\s*"` + regexp.QuoteMeta(neonPostgresSource) + `"`)
)

// removeNeonDeployFiles deletes the deploy-time files and a step-1 plan file
// left in envDir. A file that is not there is not an error.
func removeNeonDeployFiles(envDir string) error {
	var errs []error
	for _, name := range []string{neonRetiredFile, neonRetiredOverrideFile, neonStep1PlanFile} {
		if err := os.Remove(filepath.Join(envDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// writeNeonRetiredFiles writes neon_retired.tf and neon_retired_override.tf
// into envDir for the retired connections and the database_url names leaving
// with this run. mainTF is main.tf's content. remove deletes both files; it is
// safe to call more than once. Nothing is written when nothing is needed.
func writeNeonRetiredFiles(envDir string, retired []NeonConnectionState, ssmExclude []string, mainTF []byte) (remove func(), err error) {
	remove = func() { _ = removeNeonDeployFiles(envDir) }
	if err := removeNeonDeployFiles(envDir); err != nil {
		return remove, fmt.Errorf("neon: removing old deploy-time files: %w", err)
	}

	conns := append([]NeonConnectionState(nil), retired...)
	sort.Slice(conns, func(i, j int) bool { return conns[i].Alias < conns[j].Alias })

	if len(conns) > 0 {
		var b strings.Builder
		b.WriteString("# Written by meroku for one deploy or destroy, then deleted. Do not edit.\n")
		b.WriteString("# Provider configurations for Neon connections that left the YAML, so their\n")
		b.WriteString("# roles can be dropped.\n\n")
		if !neonReCredentialsVariable.Match(mainTF) {
			b.WriteString(neonCredentialsVariableHCL)
			b.WriteString("\n")
		}
		for _, c := range conns {
			if !neonReAliasIdent.MatchString(c.Alias) || c.Database == "" {
				return remove, fmt.Errorf("neon: the connection record of %s in state names no database", neonAccessModuleAddress(c.Alias))
			}
			b.WriteString(neonProviderHCL(c.Alias, c.Database))
			b.WriteString("\n")
		}
		if err := writeNeonHCL(envDir, neonRetiredFile, b.String()); err != nil {
			return remove, err
		}
	}

	var o strings.Builder
	if len(conns) > 0 && !neonRePostgresSource.Match(mainTF) {
		fmt.Fprintf(&o, "terraform {\n  required_providers {\n    postgresql = {\n      source  = %q\n      version = %q\n    }\n  }\n}\n", neonPostgresSource, neonPostgresVersion)
	}
	if len(ssmExclude) > 0 {
		names := append([]string(nil), ssmExclude...)
		sort.Strings(names)
		if o.Len() > 0 {
			o.WriteString("\n")
		}
		o.WriteString("module \"workloads\" {\n  ssm_exclude_names = [\n")
		for _, n := range names {
			fmt.Fprintf(&o, "    %s,\n", neonHCLString(n))
		}
		o.WriteString("  ]\n}\n")
	}
	if o.Len() > 0 {
		content := "# Written by meroku for one deploy or destroy, then deleted. Do not edit.\n\n" + o.String()
		if err := writeNeonHCL(envDir, neonRetiredOverrideFile, content); err != nil {
			return remove, err
		}
	}
	return remove, nil
}

// neonLeavingConnections are the connections in st that env no longer
// renders and whose record names a database. Their objects are still in
// state, so any plan needs their provider configuration. A connection whose
// record names no database has no provider to write, and the resolver
// reports it.
func neonLeavingConnections(env Env, st NeonStateView) ([]NeonConnectionState, error) {
	d, err := neonDesiredFromEnv(env)
	if err != nil {
		return nil, err
	}
	var out []NeonConnectionState
	for _, alias := range neonSortedConnections(st) {
		if d != nil {
			if _, ok := d.connections[alias]; ok {
				continue
			}
		}
		if c := st.Connections[alias]; c.Database != "" {
			out = append(out, c)
		}
	}
	return out, nil
}

const neonCredentialsVariableHCL = `variable "neon_pg_credentials" {
  type      = map(object({ host = string, user = string, password = string, pg_version = string }))
  default   = {}
  sensitive = true
  ephemeral = true
}
`

// neonProviderHCL is one postgresql provider alias, configured only from the
// ephemeral credentials variable.
func neonProviderHCL(alias, database string) string {
	db := neonHCLString(database)
	return fmt.Sprintf(`provider "postgresql" {
  alias            = %s
  host             = try(var.neon_pg_credentials[%s].host, "unresolved.invalid")
  port             = 5432
  database         = %s
  username         = try(var.neon_pg_credentials[%s].user, "unresolved")
  password         = try(var.neon_pg_credentials[%s].password, "")
  sslmode          = "require"
  superuser        = false
  connect_timeout  = 30
  expected_version = try(var.neon_pg_credentials[%s].pg_version, "17")
}
`, neonHCLString(alias), db, db, db, db, db)
}

// neonHCLString is s as a quoted HCL string literal.
func neonHCLString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return `"` + hclEscapeString(r.Replace(s)) + `"`
}

// writeNeonHCL checks content parses, then writes it to envDir/name.
func writeNeonHCL(envDir, name, content string) error {
	path := filepath.Join(envDir, name)
	if err := validateGeneratedHCL(path, content); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("neon: writing %s: %w", path, err)
	}
	return nil
}

// neonMainTFModules are the module names main.tf declares.
var neonReModuleBlock = regexp.MustCompile(`(?m)^\s*module\s+"([^"]+)"`)

func neonMainTFModules(mainTF []byte) map[string]bool {
	out := map[string]bool{}
	for _, m := range neonReModuleBlock.FindAllSubmatch(mainTF, -1) {
		out[string(m[1])] = true
	}
	return out
}
