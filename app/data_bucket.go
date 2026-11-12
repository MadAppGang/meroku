package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// The private data bucket (modules/data_bucket). The bucket always exists; the
// optional data_bucket block only lists shared folders:
//
//	data_bucket:
//	  shared:
//	    - { name: uploads, read_write: [api, backend], read: [reports] }
//
// See ai_docs/DATA_BUCKET.md.

// dataBucketSharedKey is the envMap key holding the shared folders the template
// renders, with references to switched-off workloads removed.
const dataBucketSharedKey = "_data_bucket_shared"

// dataBucketSharedNamePattern matches modules/data_bucket's validation of
// var.shared[].name. The name becomes shared/<name>/ in the bucket and
// DATA_BUCKET_SHARED_<NAME> in the container, with - turned into _.
var dataBucketSharedNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`)

// validateDataBucketConfigMap checks data_bucket.shared and writes the normalised
// list under dataBucketSharedKey. Errors: a malformed or duplicate folder name, a
// workload listed twice for one folder, and a workload name that does not exist.
// A switched-off workload is dropped from the folder with a warning.
func validateDataBucketConfigMap(envMap map[string]interface{}) error {
	envMap[dataBucketSharedKey] = []interface{}{}

	raw, present := envMap["data_bucket"]
	if !present || raw == nil {
		return nil
	}
	block, ok := computeNodeMap(raw)
	if !ok {
		return fmt.Errorf("data_bucket must be a mapping with an optional `shared` list")
	}
	sharedRaw, present := block["shared"]
	if !present || sharedRaw == nil {
		return nil
	}
	shared, ok := sharedRaw.([]interface{})
	if !ok {
		return fmt.Errorf("data_bucket.shared must be a list of folders")
	}

	var problems []string
	normalised := make([]interface{}, 0, len(shared))
	seenNames := map[string]int{}

	for i, entryRaw := range shared {
		where := fmt.Sprintf("data_bucket.shared[%d]", i)
		entry, ok := computeNodeMap(entryRaw)
		if !ok {
			problems = append(problems, fmt.Sprintf("%s must be a mapping with name, read_write and read", where))
			continue
		}
		name := stringFromMap(entry, "name")
		switch {
		case name == "":
			problems = append(problems, fmt.Sprintf("%s has no name", where))
			continue
		case !dataBucketSharedNamePattern.MatchString(name):
			problems = append(problems, fmt.Sprintf(
				"%s name %q: use 1 to 32 lower-case letters, digits and hyphens, starting and ending with a letter or a digit", where, name))
			continue
		}
		if first, dup := seenNames[name]; dup {
			problems = append(problems, fmt.Sprintf("%s name %q is already used by data_bucket.shared[%d]", where, name, first))
			continue
		}
		seenNames[name] = i
		where = fmt.Sprintf("%s ('%s')", where, name)

		listed := map[string]string{}
		lists := map[string][]interface{}{}
		for _, field := range []string{"read_write", "read"} {
			names, err := workloadNameList(entry[field])
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s.%s %v", where, field, err))
				continue
			}
			kept := []interface{}{}
			for _, w := range names {
				if other, dup := listed[w]; dup {
					problems = append(problems, fmt.Sprintf(
						"%s.%s lists %q, which is already in %s; list a workload once per folder (read_write includes read)", where, field, w, other))
					continue
				}
				listed[w] = field
				switch resolveWorkloadReference(envMap, w) {
				case workloadReferenceKnown:
					kept = append(kept, w)
				case workloadReferenceDisabled:
					addGenerateWarning(envMap, fmt.Sprintf(
						"%s.%s lists %q, which is switched off (enabled: false); it gets no access to shared/%s/ until it is switched on", where, field, w, name))
				default:
					problems = append(problems, fmt.Sprintf(
						"%s.%s lists %q, which is not a workload; use backend or the name of a service, scheduled task or event task", where, field, w))
				}
			}
			lists[field] = kept
		}

		normalised = append(normalised, map[string]interface{}{
			"name":       name,
			"read_write": lists["read_write"],
			"read":       lists["read"],
		})
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("data_bucket:\n  %s", strings.Join(problems, "\n  "))
	}
	envMap[dataBucketSharedKey] = normalised
	return nil
}

// workloadNameList reads a YAML list of workload names.
func workloadNameList(raw interface{}) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("must be a list of workload names")
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok || s == "" {
			return nil, fmt.Errorf("must hold workload names, got %v", formatYAMLValue(item))
		}
		out = append(out, s)
	}
	return out, nil
}
