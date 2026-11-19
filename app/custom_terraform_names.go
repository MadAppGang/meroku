package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// Root-level names env/main.hbs declares for the capability modules.
//
// Files in custom/terraform/_shared and custom/terraform/<env> are appended to the
// generated main.tf, so they share its root namespace. A custom file that already
// declares one of these names would make Terraform fail with "Duplicate module call"
// or "Duplicate local value definition" pointing at a line of main.tf; refusing it
// at generate names the custom file and its line instead.
var reservedRootModuleNames = []string{"data_bucket", "vaults", "events", "valkey", "kms_keys"}
var reservedRootLocalNames = []string{"workload_roles", "workload_extra_env", "workload_extra_secrets"}

// customTerraformFiles lists the custom .tf files appended to main.tf for env, in
// the order getCustomTerraformContent appends them.
func customTerraformFiles(env string) []string {
	var files []string
	for _, dir := range []string{
		filepath.Join("custom", "terraform", "_shared"),
		filepath.Join("custom", "terraform", env),
	} {
		matches, err := filepath.Glob(filepath.Join(dir, "*.tf"))
		if err != nil {
			continue
		}
		files = append(files, matches...)
	}
	return files
}

// Module names reserved only while the env renders the awscc provider (an
// enhanced project bus, or one being cleaned up), so a classic env that
// already uses one of them keeps generating.
var reservedAwsccRootModuleNames = []string{"event_bus_enhanced"}
var reservedAwsccRootModulePrefixes = []string{eventSubscriberModulePrefix, eventFifoSubscriberModulePrefix}

// validateCustomTerraformRootNames refuses a custom .tf file that declares a
// module or a local with a name meroku's template uses on a classic env.
func validateCustomTerraformRootNames(env string) error {
	return validateCustomTerraformRootNamesFor(env, false)
}

// validateCustomTerraformRootNamesFor is validateCustomTerraformRootNames
// with the awscc module names reserved too when awscc is true.
func validateCustomTerraformRootNamesFor(env string, awscc bool) error {
	var problems []string
	for _, file := range customTerraformFiles(env) {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		problems = append(problems, reservedRootNameUsesFor(file, data, awscc)...)
	}
	if len(problems) == 0 {
		return nil
	}
	modules := strings.Join(reservedRootModuleNames, ", ")
	if awscc {
		modules += fmt.Sprintf("; while this env uses the enhanced event bus also %s and every name starting %s",
			strings.Join(reservedAwsccRootModuleNames, ", "), strings.Join(reservedAwsccRootModulePrefixes, " or "))
	}
	return fmt.Errorf(
		"custom terraform declares names meroku's generated main.tf uses (modules %s; locals %s). Rename yours, or delete it if meroku's feature replaces it:\n  %s",
		modules, strings.Join(reservedRootLocalNames, ", "),
		strings.Join(problems, "\n  "))
}

// reservedRootNameUses parses one custom file and reports each module block and
// locals attribute whose name is reserved, as "<file>:<line>: ...". A file that
// does not parse is left to validateGeneratedHCL, which reports it in context.
func reservedRootNameUses(filename string, data []byte) []string {
	return reservedRootNameUsesFor(filename, data, false)
}

// reservedRootNameUsesFor is reservedRootNameUses with the awscc module names
// reserved too when awscc is true.
func reservedRootNameUsesFor(filename string, data []byte, awscc bool) []string {
	file, _ := hclsyntax.ParseConfig(data, filename, hcl.InitialPos)
	if file == nil {
		return nil
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil
	}

	modules := toSet(reservedRootModuleNames)
	locals := toSet(reservedRootLocalNames)
	var prefixes []string
	if awscc {
		for _, n := range reservedAwsccRootModuleNames {
			modules[n] = true
		}
		prefixes = reservedAwsccRootModulePrefixes
	}
	reservedModule := func(name string) bool {
		if modules[name] {
			return true
		}
		for _, p := range prefixes {
			if strings.HasPrefix(name, p) {
				return true
			}
		}
		return false
	}

	var out []string
	for _, block := range body.Blocks {
		switch block.Type {
		case "module":
			if len(block.Labels) > 0 && reservedModule(block.Labels[0]) {
				out = append(out, fmt.Sprintf("%s:%d: module %q", filename, block.DefRange().Start.Line, block.Labels[0]))
			}
		case "locals":
			for name, attr := range block.Body.Attributes {
				if locals[name] {
					out = append(out, fmt.Sprintf("%s:%d: local %q", filename, attr.SrcRange.Start.Line, name))
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

func toSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, s := range items {
		out[s] = true
	}
	return out
}
