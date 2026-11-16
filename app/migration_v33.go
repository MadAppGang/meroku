package main

import (
	"fmt"
	"io"
)

// legacyBackendPoliciesKey is the workload key that once described the
// backend's IAM statements but was never rendered. workload.policy is the key
// the template reads.
const legacyBackendPoliciesKey = "backend_policies"

// migrateToV33 moves workload.backend_policies onto workload.policy.
//
// backend_policies was declared in the model and documented, but no template
// line ever read it, so its statements were never granted. Each entry is an
// {actions, resources} map, which is a valid `policy` statement as it stands:
// a statement without an effect is an Allow.
//
//   - No workload, or no backend_policies key: nothing changes.
//   - backend_policies is empty (or `backend_policies:` with no value): the
//     key is dropped.
//   - backend_policies is a list: its entries are appended after any existing
//     policy statements, and the key is dropped. The moved statements are
//     granted from the next apply on, so the migration says so.
//   - backend_policies is not a list, or policy is present and not a list:
//     both are left as they are with a warning, and generate refuses them
//     (validateLegacyBackendPolicyKeysConfigMap, validateWorkloadPolicyConfigMap).
//
// The version is not set here. applyMigrations owns schema_version.
func migrateToV33(data map[string]interface{}, out io.Writer) error {
	fmt.Fprintln(out, "  → Migrating to v33: Moving workload.backend_policies onto workload.policy")

	workload, ok := asYAMLMapping(data["workload"])
	if !ok {
		fmt.Fprintln(out, "    ℹ️  No workload configuration to migrate")
		return nil
	}

	legacyRaw, hasLegacy := workload.lookup(legacyBackendPoliciesKey)
	if !hasLegacy {
		fmt.Fprintln(out, "    ✓ workload.backend_policies is not set, nothing to migrate")
		return nil
	}

	legacy, ok := legacyRaw.([]interface{})
	if !ok && legacyRaw != nil {
		fmt.Fprintln(out, "    ⚠️  workload.backend_policies is not a list of {actions, resources} statements; left in place.")
		fmt.Fprintln(out, "       Move its statements to workload.policy by hand: generate refuses workload.backend_policies.")
		return nil
	}

	if len(legacy) == 0 {
		workload.remove(legacyBackendPoliciesKey)
		fmt.Fprintln(out, "    ✓ Removed an empty workload.backend_policies")
		return nil
	}

	existingRaw, _ := workload.lookup("policy")
	existing, ok := existingRaw.([]interface{})
	if !ok && existingRaw != nil {
		fmt.Fprintln(out, "    ⚠️  workload.policy is not a list, so workload.backend_policies cannot be added to it; both left in place.")
		fmt.Fprintln(out, "       Fix workload.policy, then move the backend_policies statements into it: generate refuses both as they are.")
		return nil
	}

	merged := make([]interface{}, 0, len(existing)+len(legacy))
	merged = append(merged, existing...)
	merged = append(merged, legacy...)
	workload.set("policy", merged)
	workload.remove(legacyBackendPoliciesKey)

	fmt.Fprintf(out, "    ✓ Moved %d statement(s) from workload.backend_policies to workload.policy\n", len(legacy))
	fmt.Fprintln(out, "    ⚠️  These statements were never granted before and are granted from the next apply: review them in the plan.")
	return nil
}

// set writes value at key, replacing any value already there.
func (m yamlMapping) set(key string, value interface{}) {
	if m.anyKeyed != nil {
		m.anyKeyed[key] = value
		return
	}
	m.stringKeyed[key] = value
}

// remove deletes key. A missing key is not an error.
func (m yamlMapping) remove(key string) {
	if m.anyKeyed != nil {
		delete(m.anyKeyed, key)
		return
	}
	delete(m.stringKeyed, key)
}
