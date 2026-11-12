package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Shared SSM vaults (modules/vaults). A vault is the SSM path
// /<env>/<project>/vault/<name>/; a workload connected to it gets every parameter
// directly under it as a secret:
//
//	vaults: [{ name: shared }, { name: payments }]
//	services:
//	  - name: api
//	    vaults:
//	      - { name: shared }                                   # access: read
//	      - { name: payments, access: read_write, prefix: PAY } # database_url -> PAY_DATABASE_URL
//
// The backend connects under workload.vaults, tasks under their own vaults key.
// See ai_docs/VAULTS.md.

// vaultConnectionsKey is the envMap key holding every connection the template
// renders, one entry per workload and vault: {workload, vault, access, prefix}.
const vaultConnectionsKey = "_vault_connections"

// vaultNamePattern matches modules/vaults' validation of a vault name. The name
// is a segment of the SSM path and part of the IAM policy ARNs.
var vaultNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9_-]{0,30}[a-z0-9])?$`)

// vaultPrefixPattern is what a connection's prefix may be. The prefix and an
// underscore go before each secret name, which is upper-cased.
var vaultPrefixPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

const (
	vaultAccessRead      = "read"
	vaultAccessReadWrite = "read_write"
)

// vaultConnectionSource is one workload's YAML vaults list and where it lives.
type vaultConnectionSource struct {
	workload string
	field    string
	raw      interface{}
}

// vaultConnectionSources lists every workload's vaults key, backend first.
func vaultConnectionSources(envMap map[string]interface{}) []vaultConnectionSource {
	var out []vaultConnectionSource
	if workload, ok := yamlSubMap(envMap, "workload"); ok {
		out = append(out, vaultConnectionSource{workload: backendWorkloadName, field: "workload.vaults", raw: workload["vaults"]})
	}
	for _, key := range workloadLists {
		items, _ := envMap[key].([]interface{})
		for i, raw := range items {
			item, ok := computeNodeMap(raw)
			if !ok {
				continue
			}
			name := stringFromMap(item, "name")
			if name == "" {
				continue
			}
			out = append(out, vaultConnectionSource{
				workload: name,
				field:    fmt.Sprintf("%s[%d].vaults ('%s')", key, i, name),
				raw:      item["vaults"],
			})
		}
	}
	return out
}

// declaredVaults reads the top-level vaults list. It returns the set of names
// and the problems found: a malformed or duplicate name.
func declaredVaults(envMap map[string]interface{}) (map[string]bool, []string) {
	names := map[string]bool{}
	raw, present := envMap["vaults"]
	if !present || raw == nil {
		return names, nil
	}
	list, ok := raw.([]interface{})
	if !ok {
		return names, []string{"vaults must be a list of { name: <vault> }"}
	}
	var problems []string
	first := map[string]int{}
	for i, entryRaw := range list {
		where := fmt.Sprintf("vaults[%d]", i)
		entry, ok := computeNodeMap(entryRaw)
		if !ok {
			problems = append(problems, fmt.Sprintf("%s must be a mapping with a name", where))
			continue
		}
		name := stringFromMap(entry, "name")
		switch {
		case name == "":
			problems = append(problems, fmt.Sprintf("%s has no name", where))
			continue
		case !vaultNamePattern.MatchString(name):
			problems = append(problems, fmt.Sprintf(
				"%s name %q: use 1 to 32 lower-case letters, digits, hyphens and underscores, starting and ending with a letter or a digit", where, name))
			continue
		}
		if at, dup := first[name]; dup {
			problems = append(problems, fmt.Sprintf("%s name %q is already used by vaults[%d]", where, name, at))
			continue
		}
		first[name] = i
		names[name] = true
	}
	return names, problems
}

// validateVaultsConfigMap checks the vaults list and every workload's
// connections, and writes the connections under vaultConnectionsKey. Errors: a
// malformed or duplicate vault name, a connection to a vault that is not
// declared, the same vault twice on one workload, an access other than read or
// read_write, and a prefix that is not upper-case letters, digits and
// underscores starting with a letter.
func validateVaultsConfigMap(envMap map[string]interface{}) error {
	envMap[vaultConnectionsKey] = []interface{}{}

	vaults, problems := declaredVaults(envMap)
	connections := []interface{}{}

	for _, src := range vaultConnectionSources(envMap) {
		if src.raw == nil {
			continue
		}
		list, ok := src.raw.([]interface{})
		if !ok {
			problems = append(problems, fmt.Sprintf("%s must be a list of { name, access, prefix }", src.field))
			continue
		}
		seen := map[string]int{}
		for i, entryRaw := range list {
			where := fmt.Sprintf("%s[%d]", src.field, i)
			entry, ok := computeNodeMap(entryRaw)
			if !ok {
				problems = append(problems, fmt.Sprintf("%s must be a mapping with name, access and prefix", where))
				continue
			}
			name := stringFromMap(entry, "name")
			if name == "" {
				problems = append(problems, fmt.Sprintf("%s has no vault name", where))
				continue
			}
			if !vaults[name] {
				problems = append(problems, fmt.Sprintf(
					"%s connects vault %q, which is not in the top-level vaults list; add { name: %s } there or pick another vault", where, name, name))
				continue
			}
			if at, dup := seen[name]; dup {
				problems = append(problems, fmt.Sprintf(
					"%s connects vault %q, already connected at index %d; connect a vault once per workload (read_write includes read)", where, name, at))
				continue
			}
			seen[name] = i

			access := vaultAccessRead
			if rawAccess, has := entry["access"]; has && rawAccess != nil {
				s, _ := rawAccess.(string)
				switch s {
				case vaultAccessRead, vaultAccessReadWrite:
					access = s
				case "":
				default:
					problems = append(problems, fmt.Sprintf(
						"%s access %s: use read or read_write", where, formatYAMLValue(rawAccess)))
					continue
				}
			}

			prefix := ""
			if rawPrefix, has := entry["prefix"]; has && rawPrefix != nil {
				s, ok := rawPrefix.(string)
				if !ok || (s != "" && !vaultPrefixPattern.MatchString(s)) {
					problems = append(problems, fmt.Sprintf(
						"%s prefix %s: use upper-case letters, digits and underscores, starting with a letter (PAY turns database_url into PAY_DATABASE_URL)", where, formatYAMLValue(rawPrefix)))
					continue
				}
				prefix = s
			}

			connections = append(connections, map[string]interface{}{
				"workload": src.workload,
				"vault":    name,
				"access":   access,
				"prefix":   prefix,
			})
		}
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("vaults:\n  %s", strings.Join(problems, "\n  "))
	}
	envMap[vaultConnectionsKey] = connections
	return nil
}

// moveServiceVaultsOut deletes each service's vaults key once it is normalised
// under vaultConnectionsKey. The services list renders through
// {{{array services}}}, which does not escape HCL template openers, so nothing a
// capability module reads travels through it.
func moveServiceVaultsOut(envMap map[string]interface{}) {
	services, _ := envMap["services"].([]interface{})
	for _, raw := range services {
		switch service := raw.(type) {
		case map[string]interface{}:
			delete(service, "vaults")
		case map[interface{}]interface{}:
			delete(service, "vaults")
		}
	}
}
