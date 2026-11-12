package main

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Serverless Valkey (modules/valkey). The optional valkey block:
//
//	valkey:
//	  enabled: true
//	  engine_version: "8"   # 7 | 8 | 9
//	  max_storage_gb: 1     # a ceiling on stored data, never a minimum
//	  access: [api, nightly]
//
// Access is by security group only: each workload in access may reach the cache
// and gets VALKEY_HOST, VALKEY_PORT and VALKEY_URL. See ai_docs/VALKEY.md.

// valkeyKey is the envMap key holding the normalised block the template renders:
// engine_version as a string, max_storage_gb as an integer and access with
// references to switched-off workloads removed. Written only when valkey is
// enabled.
const valkeyKey = "_valkey"

// valkeyEngineVersions are the major versions serverless Valkey offers.
var valkeyEngineVersions = []string{"7", "8", "9"}

const (
	valkeyDefaultEngineVersion = "8"
	valkeyDefaultMaxStorageGB  = 1
)

// validateValkeyConfigMap checks an enabled valkey block and writes the
// normalised form under valkeyKey. Errors: an engine version other than 7, 8 or
// 9, a storage cap below 1 GB or not a whole number, a workload listed twice, and
// a name that is not a workload. A switched-off workload is dropped with a
// warning, and an enabled cache no workload can reach is a warning.
func validateValkeyConfigMap(envMap map[string]interface{}) error {
	delete(envMap, valkeyKey)

	raw, present := envMap["valkey"]
	if !present || raw == nil {
		return nil
	}
	block, ok := computeNodeMap(raw)
	if !ok {
		return fmt.Errorf("valkey must be a mapping with enabled, engine_version, max_storage_gb and access")
	}
	if enabled, _ := block["enabled"].(bool); !enabled {
		return nil
	}

	var problems []string

	version, err := valkeyEngineVersion(block["engine_version"])
	if err != nil {
		problems = append(problems, err.Error())
	}
	storage, err := valkeyMaxStorageGB(block["max_storage_gb"])
	if err != nil {
		problems = append(problems, err.Error())
	}

	access := []interface{}{}
	names, err := workloadNameList(block["access"])
	if err != nil {
		problems = append(problems, fmt.Sprintf("valkey.access %v", err))
	}
	seen := map[string]bool{}
	for _, w := range names {
		if seen[w] {
			problems = append(problems, fmt.Sprintf("valkey.access lists %q more than once; list each workload once", w))
			continue
		}
		seen[w] = true
		switch resolveWorkloadReference(envMap, w) {
		case workloadReferenceKnown:
			access = append(access, w)
		case workloadReferenceDisabled:
			addGenerateWarning(envMap, fmt.Sprintf(
				"valkey.access lists %q, which is switched off (enabled: false); it cannot reach Valkey until it is switched on", w))
		default:
			problems = append(problems, fmt.Sprintf(
				"valkey.access lists %q, which is not a workload; use backend or the name of a service, scheduled task or event task", w))
		}
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("valkey:\n  %s", strings.Join(problems, "\n  "))
	}

	if len(access) == 0 {
		addGenerateWarning(envMap,
			"valkey is enabled but valkey.access lists no workload, so nothing can connect to it; add the workloads that use it, or set enabled: false to stop paying for it")
	}

	envMap[valkeyKey] = map[string]interface{}{
		"engine_version": version,
		"max_storage_gb": storage,
		"access":         access,
	}
	return nil
}

// valkeyEngineVersion reads engine_version, which YAML may give as a string or a
// number. Absent means the default.
func valkeyEngineVersion(raw interface{}) (string, error) {
	var version string
	switch v := raw.(type) {
	case nil:
		return valkeyDefaultEngineVersion, nil
	case string:
		version = strings.TrimSpace(v)
	case int:
		version = strconv.Itoa(v)
	case int64:
		version = strconv.FormatInt(v, 10)
	case uint64:
		version = strconv.FormatUint(v, 10)
	case float64:
		version = strconv.FormatFloat(v, 'f', -1, 64)
	default:
		version = formatYAMLValue(v)
	}
	for _, allowed := range valkeyEngineVersions {
		if version == allowed {
			return version, nil
		}
	}
	return "", fmt.Errorf("valkey.engine_version %q: use \"7\", \"8\" or \"9\"", version)
}

// valkeyMaxStorageGB reads max_storage_gb, a whole number of at least 1. Absent
// means the default.
func valkeyMaxStorageGB(raw interface{}) (int, error) {
	var gb float64
	switch v := raw.(type) {
	case nil:
		return valkeyDefaultMaxStorageGB, nil
	case int:
		gb = float64(v)
	case int64:
		gb = float64(v)
	case uint64:
		gb = float64(v)
	case float64:
		gb = v
	default:
		return 0, fmt.Errorf("valkey.max_storage_gb must be a whole number of GB, at least 1; got %v", formatYAMLValue(raw))
	}
	if gb < 1 || gb != math.Trunc(gb) {
		return 0, fmt.Errorf("valkey.max_storage_gb must be a whole number of GB, at least 1; got %v", formatYAMLValue(raw))
	}
	return int(gb), nil
}
