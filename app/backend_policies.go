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

// customPreBackendPoliciesKey is the template flag that is true when the
// custom/pre module declares a backend_policies output.
//
// env/main.hbs merges that output into backend_policy by direct reference,
// not through try(). try() returns a wholly unknown value as soon as its
// argument is partly unknown, and a custom/pre statement whose resources name
// something created in the same apply is partly unknown at plan. The whole
// backend_policy list would then be unknown, and so would the count of the
// backend's custom policy: Terraform stops with "Invalid count argument". A
// direct reference keeps the list's length known, so the template references
// the output only when it exists.
const customPreBackendPoliciesKey = "custom_pre_backend_policies"

// customPreDeclaresBackendPolicies reports whether any custom/pre/*.tf file,
// relative to the working directory, has a top-level
// `output "backend_policies"` block. Each file is parsed as HCL, so the label
// inside a comment or a string never counts, and a label written unquoted or
// with a comment before it still does.
//
// A missing directory or an unreadable file counts as not declared. A file that
// does not parse counts as not declared too, and is named in the returned
// warnings, because the output in it would then be silently left out of the
// backend's policy. Terraform reports the parse error itself at init.
func customPreDeclaresBackendPolicies() (declared bool, warnings []string) {
	files, err := filepath.Glob(filepath.Join("custom", "pre", "*.tf"))
	if err != nil {
		return false, nil
	}
	sort.Strings(files)
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		found, parseErr := declaresBackendPoliciesOutput(file, data)
		if parseErr != "" {
			warnings = append(warnings, fmt.Sprintf(
				"%s does not parse as HCL, so a backend_policies output in it is not merged into the backend's policy: %s",
				file, parseErr))
			continue
		}
		if found {
			declared = true
		}
	}
	return declared, warnings
}

// declaresBackendPoliciesOutput parses one file and reports whether it has a
// top-level output block labelled backend_policies. parseErr is non-empty when
// the file has HCL syntax errors.
func declaresBackendPoliciesOutput(filename string, data []byte) (found bool, parseErr string) {
	file, diags := hclsyntax.ParseConfig(data, filename, hcl.InitialPos)
	if diags.HasErrors() {
		return false, diags.Error()
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return false, ""
	}
	for _, block := range body.Blocks {
		if block.Type == "output" && len(block.Labels) == 1 && block.Labels[0] == "backend_policies" {
			return true, ""
		}
	}
	return false, ""
}

// validateLegacyBackendPolicyKeysConfigMap refuses the two workload keys that
// look like backend IAM settings but are not rendered:
//
//   - workload.backend_policies. Migration v33 moves a list of statements onto
//     workload.policy; what is left at generate is a value it could not move,
//     or one written by hand afterwards.
//   - workload.policies, a list of managed policy ARNs. An env YAML names
//     what a workload may do, not the ARNs of policies in one account.
//
// Both are refused only when they hold something: an empty list grants
// nothing and older struct saves wrote `policies: []` into every file.
func validateLegacyBackendPolicyKeysConfigMap(envMap map[string]interface{}) error {
	workload, ok := asYAMLMapping(envMap["workload"])
	if !ok {
		return nil
	}

	var problems []string
	if raw, _ := workload.lookup(legacyBackendPoliciesKey); !isEmptyYAMLList(raw) {
		problems = append(problems,
			"workload.backend_policies is not read: move its statements under workload.policy "+
				"({effect, actions, resources, conditions}, effect defaults to Allow)")
	}
	if raw, _ := workload.lookup("policies"); !isEmptyYAMLList(raw) {
		problems = append(problems,
			"workload.policies (managed policy ARNs) is not supported: write the permissions as workload.policy statements, "+
				"or attach a managed policy to the backend task role from custom/post")
	}

	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("invalid backend IAM configuration:\n  %s", strings.Join(problems, "\n  "))
}

// isEmptyYAMLList reports whether a YAML value is absent, null or an empty list.
func isEmptyYAMLList(raw interface{}) bool {
	if raw == nil {
		return true
	}
	list, ok := raw.([]interface{})
	return ok && len(list) == 0
}
