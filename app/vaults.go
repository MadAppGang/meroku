package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Shared SSM vaults (modules/vaults). A vault is the SSM path
// /<env>/<project>/vault/<name>/ and the list of secret names the YAML declares
// for it. A workload connected to the vault gets each declared secret, its
// value read by ECS from /<env>/<project>/vault/<name>/<secret>:
//
//	vaults:
//	  - { name: shared, secrets: [api_key] }
//	  - { name: payments, secrets: [database_url] }
//	services:
//	  - name: api
//	    vaults:
//	      - { name: shared }                                   # access: read
//	      - { name: payments, access: read_write, prefix: PAY } # database_url -> PAY_DATABASE_URL
//
// The backend connects under workload.vaults, tasks under their own vaults key.
// Values are written from the web UI (app/api_vault_secrets.go), never by
// Terraform. See ai_docs/VAULTS.md.

// vaultConnectionsKey is the envMap key holding every connection the template
// renders, one entry per workload and vault: {workload, vault, access, prefix}.
const vaultConnectionsKey = "_vault_connections"

// vaultSecretsKey is the envMap key holding each declared vault's secret names,
// vault name -> list of names, rendered as modules/vaults' var.vaults.
const vaultSecretsKey = "_vault_secrets"

// vaultNamePattern matches modules/vaults' validation of a vault name. The name
// is a segment of the SSM path and part of the IAM policy ARNs.
var vaultNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9_-]{0,30}[a-z0-9])?$`)

// vaultSecretNamePattern is what a vault secret name may be: the last segment
// of its parameter. Lower case only, so two names can never upper-case to the
// same variable.
var vaultSecretNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// vaultSecretNameRule says vaultSecretNamePattern in words, for messages.
const vaultSecretNameRule = "use 1 to 63 lower-case letters, digits and underscores, starting with a letter"

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

// declaredVaults reads the top-level vaults list. It returns each vault's
// secret names, keyed by vault, and the problems found: a malformed or
// duplicate vault name, a missing or malformed secrets list, a malformed or
// duplicate secret name. A vault whose list has a problem is still a key, with
// the valid names only, so a connection to it is not also reported as a
// connection to an unknown vault.
func declaredVaults(envMap map[string]interface{}) (map[string][]string, []string) {
	vaults := map[string][]string{}
	raw, present := envMap["vaults"]
	if !present || raw == nil {
		return vaults, nil
	}
	list, ok := raw.([]interface{})
	if !ok {
		return vaults, []string{"vaults must be a list of { name: <vault>, secrets: [<name>, ...] }"}
	}
	var problems []string
	first := map[string]int{}
	for i, entryRaw := range list {
		where := fmt.Sprintf("vaults[%d]", i)
		entry, ok := computeNodeMap(entryRaw)
		if !ok {
			problems = append(problems, fmt.Sprintf("%s must be a mapping with a name and a secrets list", where))
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

		secrets, secretProblems := vaultSecretList(name, where, entry)
		problems = append(problems, secretProblems...)
		vaults[name] = secrets
	}
	return vaults, problems
}

// vaultSecretList reads one vault entry's secrets list. It returns the valid
// names, in YAML order, never nil, and a problem for every invalid one.
func vaultSecretList(vault, where string, entry map[string]interface{}) ([]string, []string) {
	names := []string{}
	raw, has := entry["secrets"]
	if !has || raw == nil {
		return names, []string{fmt.Sprintf(
			"vault %q has no secrets list: list its secret names under %s.secrets; the Vaults tab can import the names already in SSM", vault, where)}
	}
	var items []interface{}
	switch list := raw.(type) {
	case []interface{}:
		items = list
	case []string:
		for _, s := range list {
			items = append(items, s)
		}
	default:
		return names, []string{fmt.Sprintf("%s.secrets must be a list of secret names, not %s", where, formatYAMLValue(raw))}
	}

	var problems []string
	first := map[string]int{}
	for j, item := range items {
		s, isString := item.(string)
		if !isString || !vaultSecretNamePattern.MatchString(s) {
			problems = append(problems, fmt.Sprintf("%s.secrets[%d] %s: %s", where, j, formatYAMLValue(item), vaultSecretNameRule))
			continue
		}
		if at, dup := first[s]; dup {
			problems = append(problems, fmt.Sprintf("%s.secrets[%d] %q is already listed at index %d", where, j, s, at))
			continue
		}
		first[s] = j
		names = append(names, s)
	}
	return names, problems
}

// vaultConnection is one validated connection, with where the user wrote it.
type vaultConnection struct {
	workload string
	field    string
	vault    string
	access   string
	prefix   string
}

// vaultSecretEnvName is the variable a vault secret becomes on a workload
// connected with prefix.
func vaultSecretEnvName(prefix, secret string) string {
	if prefix == "" {
		return strings.ToUpper(secret)
	}
	return strings.ToUpper(prefix + "_" + secret)
}

// validateVaultsConfigMap checks the vaults list and every workload's
// connections, and writes the connections under vaultConnectionsKey and each
// vault's secret names under vaultSecretsKey.
//
// Errors: a malformed or duplicate vault name; a vault without a secrets list
// (the shape from before the list existed) or with a malformed or duplicate
// secret name; a connection to a vault that is not declared, the same vault
// twice on one workload, an access other than read or read_write, a prefix that
// is not upper-case letters, digits and underscores starting with a letter; and
// every name collision the declared lists make knowable here (see
// vaultSecretCollisions).
func validateVaultsConfigMap(envMap map[string]interface{}) error {
	envMap[vaultConnectionsKey] = []interface{}{}
	envMap[vaultSecretsKey] = map[string]interface{}{}

	vaults, problems := declaredVaults(envMap)
	var connections []vaultConnection

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
			if _, declared := vaults[name]; !declared {
				problems = append(problems, fmt.Sprintf(
					"%s connects vault %q, which is not in the top-level vaults list; add { name: %s, secrets: [] } there or pick another vault", where, name, name))
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
				// A non-string (a number, a list) is refused rather than read as
				// the default, which would quietly downgrade read_write to read.
				s, isString := rawAccess.(string)
				switch {
				case isString && (s == vaultAccessRead || s == vaultAccessReadWrite):
					access = s
				case isString && s == "":
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

			connections = append(connections, vaultConnection{
				workload: src.workload, field: where, vault: name, access: access, prefix: prefix,
			})
		}
	}

	problems = append(problems, vaultSecretCollisions(envMap, vaults, connections)...)

	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("vaults:\n  %s", strings.Join(problems, "\n  "))
	}

	rendered := make([]interface{}, 0, len(connections))
	for _, c := range connections {
		rendered = append(rendered, map[string]interface{}{
			"workload": c.workload,
			"vault":    c.vault,
			"access":   c.access,
			"prefix":   c.prefix,
		})
	}
	envMap[vaultConnectionsKey] = rendered

	secrets := make(map[string]interface{}, len(vaults))
	for name, list := range vaults {
		items := make([]interface{}, len(list))
		for i, s := range list {
			items[i] = s
		}
		secrets[name] = items
	}
	envMap[vaultSecretsKey] = secrets
	return nil
}

// vaultSecretCollisions reports the names a workload's container would get
// twice, now that each vault's secret names are in the YAML:
//
//   - a secret named env on a connection without a prefix, which would be ENV,
//     the variable meroku sets for every workload from <own path>/env;
//   - one variable from two connected vaults;
//   - a variable that is also in the workload's YAML env vars;
//   - a variable meroku sets for that workload (reservedWorkloadEnvTable).
//
// modules/env_secret_check refuses the same at plan, together with the
// variables the owning modules set themselves (AWS_REGION and the rest), which
// only they list.
func vaultSecretCollisions(envMap map[string]interface{}, vaults map[string][]string, connections []vaultConnection) []string {
	yamlVars := map[string]workloadEnvSource{}
	for _, src := range workloadEnvSources(envMap) {
		yamlVars[src.name] = src
	}

	type origin struct {
		field, vault, secret string
	}
	var problems []string
	byWorkload := map[string]map[string][]origin{}
	var order []string

	for _, c := range connections {
		names := byWorkload[c.workload]
		if names == nil {
			names = map[string][]origin{}
			byWorkload[c.workload] = names
			order = append(order, c.workload)
		}
		for _, secret := range vaults[c.vault] {
			envName := vaultSecretEnvName(c.prefix, secret)
			if envName == "ENV" {
				problems = append(problems, fmt.Sprintf(
					"%s connects vault %q without a prefix, and its secret %s would be the variable ENV, which meroku sets for every workload; give the connection a prefix (PAY makes it PAY_ENV) or rename the secret",
					c.field, c.vault, secret))
				continue
			}
			names[envName] = append(names[envName], origin{field: c.field, vault: c.vault, secret: secret})
		}
	}

	for _, workload := range order {
		names := byWorkload[workload]
		src := yamlVars[workload]
		ctx := workloadEnvContext{envMap: envMap, name: workload}
		envNames := make([]string, 0, len(names))
		for n := range names {
			envNames = append(envNames, n)
		}
		sort.Strings(envNames)

		for _, envName := range envNames {
			from := names[envName]
			if len(from) > 1 {
				var parts []string
				for _, o := range from {
					parts = append(parts, fmt.Sprintf("vault %q (secret %s, %s)", o.vault, o.secret, o.field))
				}
				problems = append(problems, fmt.Sprintf(
					"%s would get %s from %s; give the connections different prefixes or rename one of the secrets",
					workloadSubject(workload), envName, strings.Join(parts, " and ")))
			}
			o := from[0]
			for _, v := range src.vars {
				if v == envName {
					problems = append(problems, fmt.Sprintf(
						"%s: secret %s of vault %q would be %s, which %s also sets; remove the variable or give the connection a prefix",
						o.field, o.secret, o.vault, envName, src.field))
					break
				}
			}
			for _, row := range reservedWorkloadEnvTable {
				if row.matches(envName) && row.appliesTo(ctx) {
					problems = append(problems, fmt.Sprintf(
						"%s: secret %s of vault %q would be %s, which meroku sets for this workload (%s: %s); give the connection a prefix or rename the secret",
						o.field, o.secret, o.vault, envName, row.display(), row.setBy))
					break
				}
			}
		}
	}
	return problems
}

// workloadSubject names a workload at the start of a message.
func workloadSubject(workload string) string {
	if workload == backendWorkloadName {
		return "The backend"
	}
	return fmt.Sprintf("Workload %q", workload)
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
