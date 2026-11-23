package main

// X-Ray Transaction Search is one setting per AWS account and region: the
// trace segment destination and the Default indexing rule. modules/workloads
// turns it on for every environment that uses X-Ray, with the env YAML's
// xray.indexing_percentage (default 1). Two environments in one account and
// region with different percentages overwrite each other's rule on every
// apply; xrayIndexingConflictWarning says so before either is deployed.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

const (
	xrayDocsPath = "ai_docs/XRAY_TRANSACTION_SEARCH.md"

	// xrayDefaultIndexingPercentage is the share of spans X-Ray indexes free,
	// and what env/main.hbs renders when xray.indexing_percentage is absent.
	xrayDefaultIndexingPercentage = 1.0
)

// xrayNumber reads a YAML number: yaml.v2 decodes 1 as int and 1.5 as
// float64. Anything else, a quoted "1" included, is not a number.
func xrayNumber(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float64:
		return n, true
	case float32:
		return float64(n), true
	default:
		return 0, false
	}
}

// envUsesXRay reports whether the backend or any service that renders has
// xray_enabled: the same condition as local.xray_in_use in
// modules/workloads/xray_transaction_search.tf. A service with enabled: false
// is left out of main.tf, so it does not count.
func envUsesXRay(envMap map[string]interface{}) bool {
	if workload, ok := yamlSubMap(envMap, "workload"); ok {
		if on, _ := computeBoolField(workload, "xray_enabled"); on {
			return true
		}
	}
	for _, svc := range computeMapList(envMap, "services") {
		if enabled, has := svc["enabled"]; has && enabled != true {
			continue
		}
		if on, _ := computeBoolField(svc, "xray_enabled"); on {
			return true
		}
	}
	return false
}

// xrayIndexingPercentage is the percentage the environment's Default indexing
// rule gets: xray.indexing_percentage, or 1 when absent. ok is false for a
// value validateXRayMap refuses.
func xrayIndexingPercentage(envMap map[string]interface{}) (percentage float64, ok bool) {
	raw, present := envMap["xray"]
	if !present || raw == nil {
		return xrayDefaultIndexingPercentage, true
	}
	xray, isMap := yamlSubMap(envMap, "xray")
	if !isMap {
		return 0, false
	}
	value, present := xray["indexing_percentage"]
	if !present || value == nil {
		return xrayDefaultIndexingPercentage, true
	}
	n, isNumber := xrayNumber(value)
	if !isNumber || n < 0 || n > 100 {
		return 0, false
	}
	return n, true
}

// xrayIndexingUse is a sibling environment that uses X-Ray: where its
// Transaction Search lands and the percentage it sets.
type xrayIndexingUse struct {
	Name        string // file stem
	File        string
	AccountKeys []string
	Percentage  float64
}

// siblingXRayIndexingUses lists every other environment YAML in dir that uses
// X-Ray, with its account keys and effective indexing percentage. Files are
// read raw and never written. A sibling whose percentage its own generate
// would refuse is left out.
func siblingXRayIndexingUses(dir, self string) ([]xrayIndexingUse, error) {
	probes, err := scanSiblingProbes(dir, self)
	if err != nil {
		return nil, err
	}
	var uses []xrayIndexingUse
	for _, sp := range probes {
		data, err := os.ReadFile(filepath.Join(dir, sp.File))
		if err != nil {
			continue
		}
		var m map[string]interface{}
		if err := yaml.Unmarshal(data, &m); err != nil {
			continue
		}
		if !envUsesXRay(m) {
			continue
		}
		percentage, ok := xrayIndexingPercentage(m)
		if !ok {
			continue
		}
		uses = append(uses, xrayIndexingUse{
			Name: sp.Stem,
			File: sp.File,
			AccountKeys: awsAccountKeys(Env{
				Region:     sp.Probe.Region,
				AccountID:  sp.Probe.AccountID,
				AWSProfile: sp.Probe.AWSProfile,
			}),
			Percentage: percentage,
		})
	}
	return uses, nil
}

// accountIDFromMap reads account_id whether the YAML quotes it or not.
func accountIDFromMap(envMap map[string]interface{}) string {
	if s := stringFromMap(envMap, "account_id"); s != "" {
		return s
	}
	if n, ok := envMap["account_id"].(int); ok {
		return strconv.Itoa(n)
	}
	return ""
}

// xrayIndexingConflictWarning is the warning for an environment that uses
// X-Ray when a sibling in the same account and region also does, with a
// different indexing percentage. Silent when this environment does not use
// X-Ray or its percentage is invalid (validateXRayMap refuses that).
func xrayIndexingConflictWarning(envMap map[string]interface{}, siblings []xrayIndexingUse) (ConfigWarning, bool) {
	if !envUsesXRay(envMap) {
		return ConfigWarning{}, false
	}
	own, ok := xrayIndexingPercentage(envMap)
	if !ok {
		return ConfigWarning{}, false
	}
	ownKeys := map[string]bool{}
	for _, k := range awsAccountKeys(Env{
		Region:     stringFromMap(envMap, "region"),
		AccountID:  accountIDFromMap(envMap),
		AWSProfile: stringFromMap(envMap, "aws_profile"),
	}) {
		ownKeys[k] = true
	}
	if len(ownKeys) == 0 {
		return ConfigWarning{}, false
	}

	var conflicts []xrayIndexingUse
	for _, s := range siblings {
		if s.Percentage == own {
			continue
		}
		for _, k := range s.AccountKeys {
			if ownKeys[k] {
				conflicts = append(conflicts, s)
				break
			}
		}
	}
	if len(conflicts) == 0 {
		return ConfigWarning{}, false
	}

	var b strings.Builder
	fmt.Fprintf(&b, "This environment indexes %s of X-Ray spans (xray.indexing_percentage), and these\n", formatPercentage(own))
	fmt.Fprintf(&b, "environments in the same AWS account and region also use X-Ray with a different value:\n")
	for _, c := range conflicts {
		fmt.Fprintf(&b, "  %s (%s): %s\n", c.Name, c.File, formatPercentage(c.Percentage))
	}
	fmt.Fprintf(&b, "\nThe indexing rule is one per account and region, so each apply overwrites the\n")
	fmt.Fprintf(&b, "value the other environment set. Set the same xray.indexing_percentage in all of\n")
	fmt.Fprintf(&b, "them (absent means %s).", formatPercentage(xrayDefaultIndexingPercentage))
	return ConfigWarning{
		Code:   warnCodeXRayIndexingConflict,
		Item:   envItem(),
		Title:  "X-Ray indexing percentage differs from another environment in the same AWS account",
		Detail: b.String(),
		Docs:   docsLink(xrayDocsPath),
	}, true
}

func formatPercentage(p float64) string {
	return strconv.FormatFloat(p, 'g', -1, 64) + "%"
}
