package main

// X-Ray Transaction Search is one setting per AWS account and region: the
// trace segment destination and the Default indexing rule. modules/workloads
// turns it on, with the env YAML's xray.indexing_percentage (default 1), for
// an environment where
//
//   - a rendered workload has xray_enabled (the sidecar), always;
//   - a rendered workload has xray_spans (the span grant without a sidecar),
//     unless xray.transaction_search is false;
//   - xray.transaction_search is true, with no workload asking.
//
// xray.transaction_search: false leaves it to a sibling environment, the
// console or another stack that already owns it; validateXRayMap refuses it
// while a workload has xray_enabled. envUsesTransactionSearch is that rule.
//
// Two environments in one account and region with different percentages
// overwrite each other's rule on every apply; xrayIndexingConflictWarning says
// so before either is deployed. A percentage in an environment that does not
// turn Transaction Search on renders into nothing;
// xrayIndexingUnusedWarning says so.

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

// renderedServices is every service that reaches main.tf: one with enabled
// present and not true is left out of it, so it counts for nothing the module
// derives.
func renderedServices(envMap map[string]interface{}) []map[string]interface{} {
	var out []map[string]interface{}
	for _, svc := range computeMapList(envMap, "services") {
		if enabled, has := svc["enabled"]; has && enabled != true {
			continue
		}
		out = append(out, svc)
	}
	return out
}

// xrayEnabledKey names the first YAML key that gives this environment the
// X-Ray sidecar: workload.xray_enabled, or services["<name>"].xray_enabled of
// a rendered service. ok is false when there is none.
func xrayEnabledKey(envMap map[string]interface{}) (key string, ok bool) {
	if workload, isMap := yamlSubMap(envMap, "workload"); isMap {
		if on, _ := computeBoolField(workload, "xray_enabled"); on {
			return "workload.xray_enabled", true
		}
	}
	for _, svc := range renderedServices(envMap) {
		if on, _ := computeBoolField(svc, "xray_enabled"); on {
			return fmt.Sprintf("services[%q].xray_enabled", computeStringField(svc, "name")), true
		}
	}
	return "", false
}

// envUsesXRay reports whether the backend or any service that renders has
// xray_enabled, the sidecar: the same condition as local.xray_in_use in
// modules/workloads/xray_transaction_search.tf.
func envUsesXRay(envMap map[string]interface{}) bool {
	_, ok := xrayEnabledKey(envMap)
	return ok
}

// envUsesXRaySpans reports whether the backend or any service that renders
// has xray_spans: the same condition as local.xray_spans_in_use in
// modules/workloads/xray_transaction_search.tf. Values are read as
// envUsesXRay reads them.
func envUsesXRaySpans(envMap map[string]interface{}) bool {
	if workload, ok := yamlSubMap(envMap, "workload"); ok {
		if on, _ := computeBoolField(workload, "xray_spans"); on {
			return true
		}
	}
	for _, svc := range renderedServices(envMap) {
		if on, _ := computeBoolField(svc, "xray_spans"); on {
			return true
		}
	}
	return false
}

// xrayTransactionSearchSetting reads xray.transaction_search. set is false
// when the key, or the xray block, is absent: the module then derives it.
func xrayTransactionSearchSetting(envMap map[string]interface{}) (value, set bool) {
	xray, ok := yamlSubMap(envMap, "xray")
	if !ok {
		return false, false
	}
	return computeBoolField(xray, "transaction_search")
}

// envUsesTransactionSearch reports whether this environment renders the
// Transaction Search resources. It is local.transaction_search_on in
// modules/workloads/xray_transaction_search.tf, rule for rule. Both sides are
// checked against testdata/xray_transaction_search_vectors.json: the Go side
// by evaluating every vector, the Terraform side by a tftest run per vector
// whose inputs and assertion are written by hand. The Go check on the tftest
// file matches run names only.
func envUsesTransactionSearch(envMap map[string]interface{}) bool {
	if envUsesXRay(envMap) {
		return true
	}
	if value, set := xrayTransactionSearchSetting(envMap); set {
		return value
	}
	return envUsesXRaySpans(envMap)
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

// xrayIndexingUse is a sibling environment that turns on Transaction Search:
// where it lands and the percentage it sets.
type xrayIndexingUse struct {
	Name        string // file stem
	File        string
	AccountKeys []string
	Percentage  float64
}

// siblingXRayIndexingUses lists every other environment YAML in dir that
// turns on Transaction Search (envUsesTransactionSearch), with its account
// keys and effective indexing percentage. Files are read raw and never
// written. A sibling whose percentage its own generate would refuse is left
// out.
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
		if !envUsesTransactionSearch(m) {
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

// xrayIndexingConflictWarning is the warning for an environment that turns
// on Transaction Search when a sibling in the same account and region also
// does, with a different indexing percentage. Silent when this environment
// does not turn it on or its percentage is invalid (validateXRayMap refuses
// that).
func xrayIndexingConflictWarning(envMap map[string]interface{}, siblings []xrayIndexingUse) (ConfigWarning, bool) {
	if !envUsesTransactionSearch(envMap) {
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
	fmt.Fprintf(&b, "environments in the same AWS account and region also turn on X-Ray Transaction Search\n")
	fmt.Fprintf(&b, "with a different value:\n")
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

// xrayIndexingUnusedWarning is the warning for an xray.indexing_percentage
// that renders into no rule, because this environment does not turn on
// Transaction Search. Silent when the key is absent or null.
func xrayIndexingUnusedWarning(envMap map[string]interface{}) (ConfigWarning, bool) {
	xray, ok := yamlSubMap(envMap, "xray")
	if !ok {
		return ConfigWarning{}, false
	}
	if value, present := xray["indexing_percentage"]; !present || value == nil {
		return ConfigWarning{}, false
	}
	if envUsesTransactionSearch(envMap) {
		return ConfigWarning{}, false
	}

	var b strings.Builder
	fmt.Fprintf(&b, "xray.indexing_percentage sets the share of spans X-Ray Transaction Search indexes,\n")
	fmt.Fprintf(&b, "but this environment does not turn Transaction Search on, so no indexing rule is\n")
	fmt.Fprintf(&b, "created from it.\n\n")
	if value, set := xrayTransactionSearchSetting(envMap); set && !value {
		fmt.Fprintf(&b, "xray.transaction_search is false: set the percentage in the environment or stack\n")
		fmt.Fprintf(&b, "that owns Transaction Search in this account and region, and remove it here.")
	} else {
		fmt.Fprintf(&b, "Turn Transaction Search on (xray_spans or xray_enabled on a workload, or\n")
		fmt.Fprintf(&b, "xray.transaction_search: true), or remove xray.indexing_percentage.")
	}
	return ConfigWarning{
		Code:   warnCodeXRayIndexingUnused,
		Item:   envItem(),
		Title:  "xray.indexing_percentage is set but this environment does not turn on X-Ray Transaction Search",
		Detail: b.String(),
		Docs:   docsLink(xrayDocsPath),
	}, true
}

func formatPercentage(p float64) string {
	return strconv.FormatFloat(p, 'g', -1, 64) + "%"
}
